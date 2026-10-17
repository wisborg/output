package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

// plot runs the command on data with args and the chart drawn in braille,
// 60 by 16 characters, and is what it wrote.
func plot(t *testing.T, data string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	args = append([]string{"--terminal", "braille", "--width", "60", "--height", "16"}, args...)
	err = run(args, strings.NewReader(data), &out, &errOut, noEnv)
	return out.String(), errOut.String(), err
}

// The rows of a file written newest first are drawn oldest first: a price
// that rose is drawn rising, low on the left and high on the right, in one
// line.
func TestPlotPutsTheRowsInOrder(t *testing.T) {
	data := "Date,Close\n2024-01-05,50\n2024-01-04,40\n2024-01-03,30\n2024-01-02,20\n2024-01-01,10\n"
	out, _, err := plot(t, data)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if lines[0] != "Close against Date" {
		t.Errorf("title %q", lines[0])
	}
	first, last := -1, -1
	var drawn []bool // whether each character column of the plot has a dot
	for r, l := range lines[1 : len(lines)-2] {
		// The gutter's labels are ASCII, so the axis's byte index is its
		// rune index too.
		plot := []rune(l)[strings.IndexAny(l, "┤│")+1:]
		if drawn == nil {
			drawn = make([]bool, len(plot))
		}
		for c, ch := range plot {
			drawn[c] = drawn[c] || ch != '\u2800'
		}
		if plot[0] != '\u2800' && first < 0 {
			first = r
		}
		if plot[len(plot)-1] != '\u2800' && last < 0 {
			last = r
		}
	}
	if first < 0 || last < 0 || last >= first {
		t.Errorf("the line starts on row %d and ends on row %d; it should end higher:\n%s", first, last, out)
	}
	// Joined from end to end: rows taken in the file's order would put
	// each dot in its place but join none of them, the line running
	// backwards along the axis.
	for c, ok := range drawn {
		if !ok {
			t.Errorf("column %d of the plot is blank; the line is broken:\n%s", c, out)
			break
		}
	}
	if !strings.Contains(lines[len(lines)-1], "1 Jan 2024") {
		t.Errorf("the x axis %q does not begin with the first day", lines[len(lines)-1])
	}
}

// Without --x, a first column of numbers in order is the axis, and one out
// of order is a reading like the others, plotted against the row.
func TestPlotChoosesTheAxis(t *testing.T) {
	out, _, err := plot(t, "year,rain\n2001,5\n2002,7\n2003,6\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "rain against year\n") {
		t.Errorf("years in order not the axis:\n%s", out)
	}
	out, _, err = plot(t, "a,b\n5,1\n3,2\n9,3\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "a and b against row\n") {
		t.Errorf("numbers out of order taken for the axis:\n%s", out)
	}
	out, _, err = plot(t, "year,rain\n2001,5\n2002,7\n", "--x", "0", "--y", "rain", "--title", "Rain")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "Rain\n") || strings.Contains(out, "2001") {
		t.Errorf("--x 0 --title Rain:\n%s", out)
	}
}

// What is left out is said: a cell that is not a number, a row without an
// x.
func TestPlotSaysWhatItLeftOut(t *testing.T) {
	data := "date,v\n2024-01-01,1\n2024-01-02,n.d.\n,3\n2024-01-04,4\n2024-01-05,5\n"
	_, stderr, err := plot(t, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"v": 1 cell not number, left as gaps`, `"date": 1 row with no date, left out`} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr %q does not say %q", stderr, want)
		}
	}
}

func TestPlotRefuses(t *testing.T) {
	for _, c := range []struct {
		data string
		args []string
		want string
	}{
		{"a,b\n1,2\n", []string{"--y", "c"}, `no column "c"`},
		{"name,v\nx,1\ny,2\n", []string{"--y", "name"}, "holds text, not numbers"},
		{"name,v\nx,1\ny,2\n", []string{"--x", "name"}, "neither numbers nor dates"},
		{"d,v\n03/04/2024,1\n05/06/2024,2\n", nil, "--dates dmy or --dates mdy"},
		{"a,b\nx,y\n", nil, "no column holds numbers"},
		{"a,b\n1,2\n", []string{"--y", "b,b"}, "twice"},
		{"a,b\n1,2\n", []string{"--header", "maybe"}, "--header"},
		{"a,b\n1,2\n", []string{"--out", "chart.jpg"}, "name it .png"},
	} {
		_, _, err := plot(t, c.data, c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q %v: %v, want an error saying %q", c.data, c.args, err, c.want)
		}
	}
}

// --list says what each column holds.
func TestPlotList(t *testing.T) {
	out, _, err := plot(t, "date,v,note\n2024-01-01,1,a\n2024-01-02,,b\n", "--list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"1  date    date    2 of 2", "2  v       number  1 of 2", "3  note    text    2 of 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("--list does not say %q:\n%s", want, out)
		}
	}
}

// --out writes a PNG of the default size, taller for each further column.
func TestPlotWritesAPNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chart.png")
	var stdout, stderr bytes.Buffer
	err := run([]string{"--out", path}, strings.NewReader("x,a,b\n1,2,3\n2,3,4\n3,1,5\n"), &stdout, &stderr, noEnv)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != imageWidth || cfg.Height != imageHeightFor(2) {
		t.Errorf("%d by %d, want %d by %d", cfg.Width, cfg.Height, imageWidth, imageHeightFor(2))
	}
	if !strings.Contains(stderr.String(), path) {
		t.Errorf("stderr %q does not name the file", stderr.String())
	}
}

// A flag after the file is a flag, as it is before it.
func TestPlotFlagsAfterTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.csv")
	if err := os.WriteFile(path, []byte("date,v,note\n2024-01-01,1,a\n2024-01-02,2,b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := plot(t, "", path, "--list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2  v       number  2 of 2") {
		t.Errorf("--list after the file did not list it:\n%s", out)
	}
}

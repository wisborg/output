package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/wisborg/output/chart/picture"
	"github.com/wisborg/output/internal/cli"
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

// Every example CSV in the repository draws, in braille and as a PNG: an
// example that no longer reads is worse than none. Drawn narrower than
// its title, each line is at most the width in a terminal's columns --
// summer-weather.csv's title, in Japanese, came out 59 columns of 50 when
// a title was cut by runes.
func TestExamples(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no examples/*.csv")
	}
	const width = 50
	for _, path := range paths {
		var out, errOut bytes.Buffer
		if err := run([]string{"--terminal", "braille", "--width", strconv.Itoa(width), "--height", "20", path}, strings.NewReader(""), &out, &errOut, noEnv); err != nil {
			t.Errorf("%s in braille: %v", path, err)
			continue
		}
		lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
		if len(lines) < 10 {
			t.Errorf("%s: %d lines, want a chart:\n%s", path, len(lines), out.String())
		}
		// In a CJK locale runewidth counts the axes' box-drawing characters
		// two columns each, which the braille rows are not laid out by; the
		// title is cut by runewidth's measure in any locale.
		if runewidth.DefaultCondition.EastAsianWidth {
			lines = lines[:1]
		}
		for _, l := range lines {
			if w := runewidth.StringWidth(l); w > width {
				t.Errorf("%s: a line %d columns wide, past %d: %q", path, w, width, l)
			}
		}
		png := filepath.Join(t.TempDir(), "chart.png")
		if err := run([]string{"--out", png, path}, strings.NewReader(""), &out, &errOut, noEnv); err != nil {
			t.Errorf("%s as a PNG: %v", path, err)
		} else if fi, err := os.Stat(png); err != nil || fi.Size() == 0 {
			t.Errorf("%s as a PNG: nothing written (%v)", path, err)
		}
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

// Help asked for is written to standard output, where a pager or grep
// finds it, and is not an error to report; a mistake in a flag is a
// UsageError, which the command reports in a line and exits 2 for.
func TestPlotHelpAndMistakes(t *testing.T) {
	out, errOut, err := plot(t, "", "--help")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(out, "usage: plot") || errOut != "" {
		t.Errorf("--help: %v, stdout %d bytes, stderr %q", err, len(out), errOut)
	}
	for _, args := range [][]string{{"--bogus"}, {"--smooth", "-3"}, {"--width", "-5"}, {"--header", "maybe"}, {"--delimiter", "ab"}} {
		_, errOut, err := plot(t, "a,b\n1,2\n", args...)
		var ue *cli.UsageError
		if !errors.As(err, &ue) {
			t.Errorf("%q: %v, want a UsageError", args, err)
		}
		if errOut != "" {
			t.Errorf("%q: wrote %q beside the error", args, errOut)
		}
	}
}

// --list lines its columns up in a terminal's columns, a CJK name included,
// which tabwriter counted in runes.
func TestPlotListAlignsWideNames(t *testing.T) {
	out, _, err := plot(t, "a,日本語,c\n1,2,3\n2,3,4\n", "--list")
	if err != nil {
		t.Fatal(err)
	}
	at := -1
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n")[2:] {
		col := runewidth.StringWidth(l[:strings.Index(l, "number")])
		if at >= 0 && col != at {
			t.Errorf("\"holds\" at column %d and %d:\n%s", at, col, out)
		}
		at = col
	}
}

// --theme sets the picture's palette: light and dark as asked, and auto
// light for a PNG, which goes on a page, and dark in a terminal that does
// not say its background is light -- as the dark terminals that show
// pictures are -- light in one whose COLORFGBG says so.
func TestPlotTheme(t *testing.T) {
	data := "x,a\n1,2\n2,3\n3,1\n"
	background := func(args ...string) color.Color {
		t.Helper()
		path := filepath.Join(t.TempDir(), "c.png")
		var stdout, stderr bytes.Buffer
		if err := run(append([]string{"--out", path}, args...), strings.NewReader(data), &stdout, &stderr, noEnv); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		return img.At(1, 1)
	}
	same := func(a, b color.Color) bool {
		r1, g1, b1, _ := a.RGBA()
		r2, g2, b2, _ := b.RGBA()
		return r1 == r2 && g1 == g2 && b1 == b2
	}
	for _, c := range []struct {
		args []string
		want color.Color
	}{
		{nil, picture.Light.Background},
		{[]string{"--theme", "light"}, picture.Light.Background},
		{[]string{"--theme", "dark"}, picture.Dark.Background},
	} {
		if got := background(c.args...); !same(got, c.want) {
			t.Errorf("%q: background %v, want %v", c.args, got, c.want)
		}
	}
	for _, c := range []struct {
		env  string
		want picture.Palette
	}{{"", picture.Dark}, {"0;15", picture.Light}, {"15;0", picture.Dark}} {
		d := drawing{theme: "auto", getenv: func(k string) string {
			if k == "COLORFGBG" {
				return c.env
			}
			return ""
		}}
		if got, err := d.palette(true); err != nil || got != c.want {
			t.Errorf("auto in a terminal with COLORFGBG=%q: %v, %v", c.env, got.Background, err)
		}
	}
}

// A file of a header and nothing under it says so, rather than that no
// column holds numbers.
func TestPlotHeaderOnly(t *testing.T) {
	if _, _, err := plot(t, "a,b\n", "--header", "yes"); err == nil || !strings.Contains(err.Error(), "no rows") {
		t.Errorf("%v, want an error saying there are no rows", err)
	}
}

// A number that could have been read the other way is noted, with the flag
// that reads it so; --decimal takes only its three values.
func TestPlotDecimal(t *testing.T) {
	_, errOut, err := plot(t, "t\tv\n1\t1,234\n2\t2,000\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "--decimal comma") {
		t.Errorf("stderr %q does not say how to read 1,234 the other way", errOut)
	}
	_, _, err = plot(t, "t,v\n1,2\n", "--decimal", "dot")
	var ue *cli.UsageError
	if !errors.As(err, &ue) {
		t.Errorf("--decimal dot: %v, want a UsageError", err)
	}
}

// The command line's mistakes are UsageErrors, which exit 2, the same in
// both commands: two files, an unknown --terminal, a --theme checked even
// where the chart is drawn in braille. --width 0 is the default, here as in
// diagram. And a file of a header alone says it has no rows.
func TestPlotUsageConsistency(t *testing.T) {
	var ue *cli.UsageError
	for _, args := range [][]string{{"a.csv", "b.csv"}, {"--terminal", "sixel"}, {"--theme", "pink"}} {
		if _, _, err := plot(t, "x,y\n1,2\n2,3\n", args...); !errors.As(err, &ue) {
			t.Errorf("%q: %v, want a UsageError", args, err)
		}
	}
	if _, _, err := plot(t, "x,y\n1,2\n2,3\n", "--width", "0"); err != nil {
		t.Errorf("--width 0: %v", err)
	}
	if _, _, err := plot(t, "x,y\n"); err == nil || !strings.Contains(err.Error(), "no rows") {
		t.Errorf("a header alone: %v, want an error saying there are no rows", err)
	}
}

// Timestamps as logs and databases write them are dates, read at the wall
// clock they say: an offset without its colon, a space before the zone or
// its name, Go's own time.Time.String, PostgreSQL's hours-only offset.
func TestPlotReadsLogTimestamps(t *testing.T) {
	for _, d := range []string{
		"2024-03-05T14:30:15+0100",
		"2024-03-05 14:30:15 +0100",
		"2024-03-05 14:30:15 +01:00",
		"2024-03-05 14:30:15 UTC",
		"2024-03-05 14:30:15.123 +0100 CET",
		"2024-03-05 14:30:15+01",
	} {
		c := classify([]string{d, d}, datesAuto)
		if c.kind != dateColumn || c.wrong != 0 {
			t.Errorf("%q: %v with %d wrong", d, c.kind, c.wrong)
			continue
		}
		if v, _ := c.value(d); time.Unix(int64(v), 0).UTC().Hour() != 14 {
			t.Errorf("%q read at %v, not at its wall clock's 14:30", d, time.Unix(int64(v), 0).UTC())
		}
	}
}

// --x-date reads an x column of numbers as dates -- seconds or
// milliseconds since 1970, or eight digits -- which no cell can say for
// itself; the axis is then a calendar's.
func TestPlotXDate(t *testing.T) {
	for _, c := range []struct {
		mode, data string
	}{
		{"unix", "t,v\n1709649015,1\n1709735415,2\n1709821815,3\n"},
		{"unix-ms", "t,v\n1709649015000,1\n1709735415000,2\n1709821815000,3\n"},
		{"yyyymmdd", "t,v\n20240305,1\n20240306,2\n20240307,3\n"},
	} {
		out, _, err := plot(t, c.data, "--x-date", c.mode)
		if err != nil {
			t.Fatalf("%s: %v", c.mode, err)
		}
		if !strings.Contains(out, "Mar") {
			t.Errorf("%s: no date on the axis:\n%s", c.mode, out)
		}
	}
	var ue *cli.UsageError
	if _, _, err := plot(t, "t,v\n1,2\n", "--x-date", "epoch"); !errors.As(err, &ue) {
		t.Errorf("--x-date epoch: %v, want a UsageError", err)
	}
}

// --smooth n averages each reading over n rows, the row and as many
// either side, and says so: an even n is the odd number after it, which
// it always averaged over, where the title said n.
func TestSmoothSaysHowManyRows(t *testing.T) {
	s := series{yNames: []string{"a"}}
	for i := range 9 {
		y := 0.0
		if i == 4 {
			y = 15
		}
		s.rows = append(s.rows, seriesRow{x: float64(i), ys: []float64{y}, ok: []bool{true}})
	}
	for _, c := range []struct{ smooth, rows int }{{2, 3}, {3, 3}, {4, 5}, {7, 7}} {
		ch, err := buildChart(s, chartOptions{smooth: c.smooth}, 100, 4, 4)
		if err != nil {
			t.Fatal(err)
		}
		if want := fmt.Sprintf("averaged over %d rows", c.rows); !strings.HasSuffix(ch.Title, want) {
			t.Errorf("--smooth %d: title %q, want it to end %q", c.smooth, ch.Title, want)
		}
		if got, want := ch.Strips[0].Segments[0][4].Y, 15/float64(c.rows); math.Abs(got-want) > 1e-9 {
			t.Errorf("--smooth %d: the spike of 15 averaged to %v, want 15 over %d rows, %v", c.smooth, got, c.rows, want)
		}
	}
}

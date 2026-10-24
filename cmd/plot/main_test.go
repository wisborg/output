package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

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

// Every example CSV and JSON Lines file in the repository draws, in braille
// and as a PNG: an example that no longer reads is worse than none. Drawn
// narrower than its title, each line is at most the width in a terminal's
// columns -- summer-weather.csv's title, in Japanese, came out 59 columns
// of 50 when a title was cut by runes.
func TestExamples(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.csv"))
	if err != nil {
		t.Fatal(err)
	}
	jsonl, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 || len(jsonl) == 0 {
		t.Fatal("no examples/*.csv or no examples/*.jsonl")
	}
	paths = append(paths, jsonl...)
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

// A braille chart of more than a million characters, or a PNG of more
// than picture.DefaultMaxPixels, is refused with what it would take and
// the flag that draws it; --high-memory does, as far as Go can allocate,
// where a refusal names no flag, since none would help.
func TestPlotHighMemory(t *testing.T) {
	data := "x,a\n1,2\n2,3\n3,1\n"
	_, _, err := plot(t, data, "--width", "50000", "--height", "24")
	if err == nil || !strings.Contains(err.Error(), "--high-memory draws it") || !strings.Contains(err.Error(), "would take about 30 MB") {
		t.Errorf("50000 by 24 characters: %v", err)
	}
	out, _, err := plot(t, data, "--width", "50000", "--height", "24", "--high-memory")
	if err != nil {
		t.Errorf("50000 by 24 characters with --high-memory: %v", err)
	} else if lines := strings.Split(strings.TrimRight(out, "\n"), "\n"); len(lines) != 24 || utf8.RuneCountInString(lines[3]) != 50000 {
		t.Errorf("50000 by 24 characters with --high-memory drew %d lines", len(lines))
	}
	_, _, err = plot(t, data, "--width", "1000000000", "--height", "1000000", "--high-memory")
	if err == nil || strings.Contains(err.Error(), "--high-memory") {
		t.Errorf("10^15 characters with --high-memory: %v", err)
	}
	png := filepath.Join(t.TempDir(), "c.png")
	var stdout, stderr bytes.Buffer
	err = run([]string{"--out", png, "--width", "100000", "--height", "100000"}, strings.NewReader(data), &stdout, &stderr, noEnv)
	if !errors.Is(err, picture.ErrTooLarge) || !strings.Contains(err.Error(), "--high-memory draws it") {
		t.Errorf("a PNG of 10^10 pixels: %v", err)
	}
	err = run([]string{"--out", png, "--width", "2097152", "--height", "1048576", "--high-memory"}, strings.NewReader(data), &stdout, &stderr, noEnv)
	// Refused by the ceiling Go's allocation sets, not by the default.
	if !errors.Is(err, picture.ErrTooLarge) || strings.Contains(err.Error(), "--high-memory") || strings.Contains(err.Error(), "the 67 million") {
		t.Errorf("a PNG of 2^41 pixels with --high-memory: %v", err)
	}
}

// --log draws each y axis by its logarithm, labelled at powers of ten; a
// reading at or below zero has no place on it, and is a gap that plot
// says, never a dot along the bottom; a column with nothing above zero is
// refused, saying why.
func TestPlotLog(t *testing.T) {
	data := "n,latency\n1,0.8\n2,3\n3,12\n4,0\n5,95\n6,400\n7,2100\n8,-1\n9,9800\n10,51000\n"
	out, errOut, err := plot(t, data, "--log")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"10k┤", "100┤", "  1┤"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q on the axis:\n%s", want, out)
		}
	}
	if !strings.Contains(errOut, `"latency": 2 cells at or below zero, left as gaps on a log axis`) {
		t.Errorf("stderr %q does not say what was left out", errOut)
	}
	// Linear, the same file has no such labels: the flag is what made them.
	if lin, _, _ := plot(t, data); strings.Contains(lin, "10k┤") {
		t.Errorf("a linear axis labelled as a log one:\n%s", lin)
	}
	if _, _, err := plot(t, "n,v\n1,0\n2,-3\n", "--log"); err == nil || !strings.Contains(err.Error(), "above zero") {
		t.Errorf("a column with nothing above zero: %v", err)
	}
}

// A file of JSON objects is read as JSON Lines without being told, and
// plotted by its keys; a line not an object is left out and said. The
// flags for CSV alone are refused for it rather than ignored, and --input
// csv reads it as CSV. A JSON array says what plot reads instead.
func TestPlotJSONLines(t *testing.T) {
	const data = "\xef\xbb\xbf{\"ts\":\"2024-03-05T14:30:00Z\",\"cpu\":{\"user\":0.2}}\n" +
		"{\"ts\":\"2024-03-05T14:31:00Z\",\"cpu\":{\"user\":0.5}}\n" +
		"{\"ts\":\"2024-03-05T14:32\n" +
		"{\"ts\":\"2024-03-05T14:33:00Z\",\"cpu\":{\"user\":0.3}}\n"
	out, errOut, err := plot(t, data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "cpu.user against ts") {
		t.Errorf("want cpu.user plotted against ts:\n%s", out)
	}
	if errOut != "line 3: not a JSON object, left out\n" {
		t.Errorf("stderr %q, want the line left out said", errOut)
	}
	out, errOut, err = plot(t, data, "--list", "--input", "jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if errOut != "line 3: not a JSON object, left out\n" {
		t.Errorf("--list: stderr %q, want the line left out said there too", errOut)
	}
	if !strings.Contains(out, "1  ts        date    3 of 3") || !strings.Contains(out, "2  cpu.user  number  3 of 3") {
		t.Errorf("--list:\n%s", out)
	}

	// A CSV's flags are refused for JSON Lines, but not at their defaults,
	// which a wrapper passing every flag says; --decimal, --dates apply.
	var ue *cli.UsageError
	for _, args := range [][]string{{"--delimiter", ","}, {"--header", "no"}, {"--input", "xml"}} {
		if _, _, err := plot(t, data, args...); !errors.As(err, &ue) {
			t.Errorf("%q: %v, want a UsageError", args, err)
		} else if args[0] != "--input" && !strings.Contains(err.Error(), "--input csv") {
			t.Errorf("%q: %v, want it to say --input csv reads the file as CSV", args, err)
		}
	}
	for _, args := range [][]string{{"--delimiter", "auto"}, {"--header", "auto"}, {"--decimal", "point"}, {"--decimal", "auto"}, {"--dates", "dmy"}} {
		if _, _, err := plot(t, data, args...); err != nil {
			t.Errorf("%q: %v", args, err)
		}
	}
	// With --input jsonl typed, the refusal does not send the reader to
	// --input csv, and comes before the input is read: a stream from a
	// program still running was read to its end first.
	// A flag mistyped is said before a file is opened: with the file
	// mistyped too, the missing file was said, exit 1, not the flag.
	for _, args := range [][]string{{"--input", "jsn"}, {"--decimal", "dot"}, {"--header", "maybe"}} {
		if _, _, err := plot(t, "", append(args, filepath.Join(t.TempDir(), "nosuch.jsonl"))...); !errors.As(err, &ue) {
			t.Errorf("%q and a file missing: %v, want a UsageError", args, err)
		}
	}
	var out2, errOut2 bytes.Buffer
	err = run([]string{"--input", "jsonl", "--delimiter", ","}, unreadable{t}, &out2, &errOut2, noEnv)
	if !errors.As(err, &ue) || strings.Contains(err.Error(), "--input csv") {
		t.Errorf("--input jsonl --delimiter ,: %v, want a UsageError not naming --input csv", err)
	}

	if out, _, err := plot(t, data, "--list", "--input", "csv"); err != nil || !strings.Contains(out, `{"ts":"2024-03-05T14:30:00Z"`) {
		t.Errorf("--input csv: %v, want the objects read as CSV cells:\n%s", err, out)
	}
	if _, _, err := plot(t, "a,b\n1,2\n", "--input", "jsonl"); err == nil || !strings.Contains(err.Error(), "line 1: not a JSON object") {
		t.Errorf("CSV as --input jsonl: %v", err)
	}
	if _, _, err := plot(t, `[{"a":1},{"a":2}]`); err == nil || !strings.Contains(err.Error(), "JSON Lines") {
		t.Errorf("a JSON array: %v", err)
	}
	// The byte-order mark is stripped once, before either reader: a CSV's
	// first name does not begin with it.
	if out, _, err := plot(t, "\xef\xbb\xbfdate,v\n2024-01-01,1\n2024-01-02,2\n", "--list"); err != nil || !strings.Contains(out, "1  date ") || strings.Contains(out, "\ufeff") {
		t.Errorf("a CSV after a byte-order mark: %v\n%s", err, out)
	}
}

// unreadable is an input a test must not read.
type unreadable struct{ t *testing.T }

func (u unreadable) Read([]byte) (int, error) {
	u.t.Error("the input was read")
	return 0, io.EOF
}

// A log as Python writes one holds NaN and Infinity, which JSON has no
// word for: read as the words, as a CSV's cells are, NaN is a gap and
// Infinity a cell not a number, and the rest of the line is kept. A log
// written where a comma is the decimal mark, in its strings, is read so
// -- its JSON numbers are left be -- or as --decimal says. A key past the
// columns kept, asked for, is said to be left out.
func TestPlotJSONLinesAsWritten(t *testing.T) {
	out, _, err := plot(t, "{\"t\":1,\"v\":NaN,\"w\":1}\n{\"t\":2,\"v\":2,\"w\":2}\n{\"t\":3,\"v\":3,\"w\":3}\n{\"t\":4,\"v\":-Infinity,\"w\":4,\"s\":\"NaN\"}\n", "--list")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]string{{"2", "v", "number", "(1", "cell", "not)", "3", "of", "4"}, {"3", "w", "number", "4", "of", "4"}} {
		if !slices.ContainsFunc(strings.Split(out, "\n"), func(l string) bool { return slices.Equal(strings.Fields(l), want) }) {
			t.Errorf("NaN and Infinity: want a line %q in\n%s", want, out)
		}
	}

	const comma = "{\"t\":1,\"v\":\"1,250\",\"n\":1.250}\n{\"t\":2,\"v\":\"0,5\",\"n\":2}\n"
	tb, err := readJSONLines([]byte(comma), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if got := grid(tb); got[0][1] != "1.250" || got[0][2] != "1.250" || got[1][1] != "0.5" {
		t.Errorf("strings with a decimal comma: %q, want 1,250 and 0,5 read with it and the number 1.250 left be", got)
	}
	if tb, err = readJSONLines([]byte(comma), decimalPoint); err != nil || tb.cell(0, 1) != "1,250" {
		t.Errorf("--decimal point: %v, %q", err, tb.cell(0, 1))
	}

	// And through the command: --decimal point reads "1,5" as no number.
	const strs = "{\"v\":\"1,5\"}\n{\"v\":\"2,5\"}\n{\"v\":\"3,5\"}\n"
	for mark, kind := range map[string]string{"auto": "number", "point": "text"} {
		out, _, err := plot(t, strs, "--list", "--decimal", mark)
		if err != nil || !strings.Contains(out, "1  v       "+kind) {
			t.Errorf("--decimal %s: %v, want v %s:\n%s", mark, err, kind, out)
		}
	}

	var b strings.Builder
	for i := range maxJSONColumns + 1 {
		fmt.Fprintf(&b, "{\"t\":%d,\"k%d\":1}\n", i, i)
	}
	if _, _, err := plot(t, b.String(), "--y", fmt.Sprint("k", maxJSONColumns)); err == nil || !strings.Contains(err.Error(), "left out") {
		t.Errorf("--y naming a key left out: %v, want it said to be left out", err)
	}
}

// Command plot charts columns of a CSV file -- prices against dates, rain
// against months, any column of numbers against another or against its
// row -- in the terminal or as a PNG.
//
//	plot prices.csv
//	plot --y close,volume prices.csv
//	plot --x month --y rain --out rain.png weather.tsv
//	some-program | plot --smooth 7
//
// It reads CSV: values delimited by commas, tabs, semicolons or bars -- it
// tells which by looking -- quoted where they hold the delimiter, with or
// without a first row naming the columns, and with its numbers' fractions
// marked by a point or a comma, which it tells for the whole file from the
// numbers in it. Other formats may come; the
// reading is one file of this command, and the chart does not care where
// its numbers came from.
//
// It draws with github.com/wisborg/output/chart: in iTerm2, WezTerm, kitty
// and Ghostty a picture the width of the window; in any other terminal, or
// into a pipe, in braille; with --out, a PNG.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/font/opentype"

	"github.com/wisborg/output/chart/picture"
	"github.com/wisborg/output/chart/terminal"
	"github.com/wisborg/output/internal/cli"
	tbl "github.com/wisborg/output/table"
)

func main() {
	cli.Exit("plot", run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}

// maxDefaultColumns is the most columns plotted when --y does not name
// them: more strips than this are each too short to read, and a file of
// fifty columns is better listed than drawn.
const maxDefaultColumns = 6

// options are the command's flags.
type options struct {
	x, y          string
	list          bool
	delimiter     string
	header        string
	dates         string
	decimal       string
	xDate         string
	out           string
	width, height int
	terminal      string
	theme         string
	title         string
	smooth        int
	robust        bool
	fonts         []string
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) error {
	var o options
	fs := flag.NewFlagSet("plot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		cli.PrintDefaults(fs)
	}
	fs.StringVar(&o.x, "x", "", "the column to plot against, by name or number from 1, or 0 for the row (default: the first column, where it holds dates or numbers in order; the row otherwise)")
	fs.StringVar(&o.y, "y", "", "the columns to plot, by name or number from 1, comma-separated, each in a strip of its own (default: every column of numbers but x, up to "+strconv.Itoa(maxDefaultColumns)+")")
	fs.BoolVar(&o.list, "list", false, "list the columns, what each holds and how many values, and plot nothing")
	fs.StringVar(&o.delimiter, "delimiter", "auto", "what separates values: auto, tab, or a character such as , ; or |")
	fs.StringVar(&o.header, "header", string(headerAuto), "whether the first row names the columns: auto, yes or no")
	fs.StringVar(&o.dates, "dates", string(datesAuto), "how a date in numbers alone, 03/04/2024, orders day and month: auto, dmy or mdy; auto tells from a day past the 12th")
	fs.StringVar(&o.xDate, "x-date", "", "read the x column as dates written as numbers: unix (seconds since 1970), unix-ms (milliseconds) or yyyymmdd (20240305), which a number alone cannot say it is")
	fs.StringVar(&o.decimal, "decimal", string(decimalAuto), "how the file marks a number's fraction: auto, point (1,234.5) or comma (1.234,5); auto tells from the file's numbers, and where they cannot say, takes a comma in a file delimited by semicolons and a point otherwise")
	fs.StringVar(&o.out, "out", "", "write the chart to this PNG rather than drawing it in the terminal")
	fs.IntVar(&o.width, "width", 0, "the chart's width: in characters in the terminal (default, or 0: the terminal's, or 100); in pixels in a PNG (default, or 0: "+strconv.Itoa(imageWidth)+")")
	fs.IntVar(&o.height, "height", 0, "the chart's height: in lines in the terminal (default, or 0: 24, and 10 more for each further column, held to the terminal's); in pixels in a PNG (default, or 0: "+strconv.Itoa(imageHeight)+", and 300 more for each further column)")
	fs.StringVar(&o.terminal, "terminal", string(terminal.Auto), "how the chart is shown in the terminal: auto (a picture where the terminal shows one, braille otherwise), iterm, kitty or braille")
	fs.StringVar(&o.theme, "theme", "auto", "the picture's palette: auto (light for a PNG; the terminal's own in a terminal), light or dark")
	fs.StringVar(&o.title, "title", "", "the chart's title (default: what is plotted against what)")
	fs.IntVar(&o.smooth, "smooth", 0, "average each value with those of the rows either side of it, this many rows in all -- the odd number after an even one -- never across a gap")
	fs.BoolVar(&o.robust, "robust", false, "set each y axis from the middle 98% of its values, so a few outliers do not flatten the rest; those beyond are drawn at the edge")
	fs.Func("font", "a TrueType or OpenType font for the letters the built-in font lacks, in a picture; repeat for several", func(s string) error {
		o.fonts = append(o.fonts, s)
		return nil
	})
	files, err := cli.Parse(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		// Asked for, so on standard output, where a pager or grep finds
		// it.
		fs.SetOutput(stdout)
		fs.Usage()
		return err
	}
	if err != nil {
		return err
	}
	if len(files) > 1 {
		return cli.Usagef("one file at a time: %s", strings.Join(files, " "))
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	// Every flag is checked whichever way the chart is drawn: --theme pink
	// with the chart in braille once drew it and exited 0, and the same
	// command with --out was refused.
	if _, _, err := terminal.Choose(terminal.Protocol(o.terminal), getenv, false); err != nil {
		return cli.Usagef("--terminal %v", err)
	}
	switch o.theme {
	case "auto", "light", "dark":
	default:
		return cli.Usagef("--theme %q: use auto, light or dark", o.theme)
	}
	for _, f := range []struct {
		name string
		v    int
	}{{"width", o.width}, {"height", o.height}, {"smooth", o.smooth}} {
		if f.v < 0 {
			return cli.Usagef("--%s %d: a number from 0", f.name, f.v)
		}
	}

	in, name := stdin, "standard input"
	if len(files) == 1 && files[0] != "-" {
		f, err := os.Open(files[0])
		if err != nil {
			return err
		}
		defer f.Close()
		in, name = f, files[0]
	}
	delim, err := parseDelimiter(o.delimiter)
	if err != nil {
		return err
	}
	header := headerMode(o.header)
	if header != headerAuto && header != headerYes && header != headerNo {
		return cli.Usagef("--header %q: use auto, yes or no", o.header)
	}
	order := dateOrder(o.dates)
	if order != datesAuto && order != datesDMY && order != datesMDY {
		return cli.Usagef("--dates %q: use auto, dmy or mdy", o.dates)
	}
	mark := decimalMark(o.decimal)
	if mark != decimalAuto && mark != decimalPoint && mark != decimalComma {
		return cli.Usagef("--decimal %q: use auto, point or comma", o.decimal)
	}
	t, err := readTable(in, delim, header, order, mark)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if len(t.rows) == 0 && !o.list {
		// Said as it is: "no column holds numbers" sent the reader looking
		// at columns when the file has no rows at all.
		return fmt.Errorf("%s: a header and no rows of values to plot", name)
	}
	cols := make([]column, len(t.names))
	for j := range t.names {
		cells := make([]string, len(t.rows))
		for i, rec := range t.rows {
			cells[i] = rec[j]
		}
		cols[j] = classify(cells, order)
	}
	if o.list {
		return writeColumns(stdout, t, cols)
	}

	xLayout := ""
	if o.xDate != "" {
		l, ok := xDateLayout(o.xDate)
		if !ok {
			return cli.Usagef("--x-date %q: use unix, unix-ms or yyyymmdd", o.xDate)
		}
		xLayout = l
	}
	x, err := chooseX(t, cols, o.x)
	if err != nil {
		return err
	}
	if xLayout != "" {
		if x < 0 {
			return cli.Usagef("--x-date reads the x column as dates, and the chart is against the row: say --x")
		}
		cells := make([]string, len(t.rows))
		for i, rec := range t.rows {
			cells[i] = rec[x]
		}
		if cols[x] = asDates(cells, xLayout); cols[x].values == cols[x].wrong {
			return fmt.Errorf("--x-date %s: no cell of %q reads as one", o.xDate, t.names[x])
		}
	}
	ys, err := chooseY(t, cols, x, o.y)
	if err != nil {
		return err
	}
	notes := t.notes
	if x >= 0 && cols[x].ambiguous {
		return fmt.Errorf("the dates in %q read as well with the day first as the month: say --dates dmy or --dates mdy", t.names[x])
	}
	ycols := make([]column, len(ys))
	for k, y := range ys {
		ycols[k] = cols[y]
		if w := cols[y].wrong; w > 0 {
			notes = append(notes, fmt.Sprintf("%q: %s not %s, left as gaps", t.names[y], cells(w), cols[y].kind))
		}
	}
	s := readSeries(t, x, cols[max(x, 0)], ys, ycols)
	if s.noX > 0 {
		notes = append(notes, fmt.Sprintf("%q: %s no %s, left out", t.names[x], rowsOf(s.noX), cols[x].kind))
	}
	copts := chartOptions{title: o.title, smooth: o.smooth, robust: o.robust}
	fonts, err := loadFonts(o.fonts)
	if err != nil {
		return err
	}
	d := drawing{series: s, opts: copts, fonts: fonts, theme: o.theme, getenv: getenv}
	if o.out != "" {
		err = d.writePNG(o.out, o.width, o.height)
	} else {
		err = d.writeTerminal(stdout, stderr, terminal.Protocol(o.terminal), o.width, o.height, set["width"], set["height"])
	}
	if err != nil {
		return err
	}
	if o.out != "" {
		fmt.Fprintf(stderr, "chart %s\n", o.out)
	}
	for _, n := range notes {
		fmt.Fprintln(stderr, n)
	}
	return nil
}

const usage = `plot charts columns of a CSV file against another column, or against the
row, in the terminal or as a PNG.

usage: plot [flags] [FILE]

FILE is read as CSV -- delimited by commas, tabs, semicolons or bars --
with or without a first row of names; without FILE, or with -, standard
input is. Each column plotted is a strip of its own, on its own scale,
sharing the x axis. An empty cell, NA or null is a gap in the line, never
a zero. A number may be written with a decimal point, 1,234.5, or a decimal
comma, 1.234,5: which, is told for the whole file from its numbers, or
said with --decimal.

Flags:
`

// chooseX is the column --x names, or -1 for the row. Without --x it is the
// first column where that holds dates, or numbers in order -- years, a
// count, a sequence -- and another column holds numbers to plot against
// it; otherwise the row. A first column of numbers out of order is a
// reading like the rest, not an axis.
func chooseX(t *table, cols []column, spec string) (int, error) {
	if spec == "0" {
		return -1, nil
	}
	if spec != "" {
		x, err := findColumn(t.names, spec)
		if err != nil {
			return 0, fmt.Errorf("--x: %w", err)
		}
		if cols[x].kind == textColumn {
			return 0, fmt.Errorf("--x: %q holds neither numbers nor dates to plot against", t.names[x])
		}
		return x, nil
	}
	if len(cols) < 2 {
		return -1, nil
	}
	others := false
	for _, c := range cols[1:] {
		others = others || c.kind == numberColumn
	}
	switch {
	case !others:
		return -1, nil
	case cols[0].kind == dateColumn:
		return 0, nil
	case cols[0].kind == numberColumn && ordered(t, 0, cols[0]):
		return 0, nil
	}
	return -1, nil
}

// ordered says column j's values run one way: never down, or never up.
func ordered(t *table, j int, c column) bool {
	up, down := true, true
	prev, have := 0.0, false
	for _, rec := range t.rows {
		v, ok := c.value(rec[j])
		if !ok {
			continue
		}
		if have {
			up, down = up && v >= prev, down && v <= prev
		}
		prev, have = v, true
	}
	return up || down
}

// chooseY are the columns --y names, or every column of numbers but x.
func chooseY(t *table, cols []column, x int, spec string) ([]int, error) {
	var ys []int
	if spec != "" {
		for part := range strings.SplitSeq(spec, ",") {
			y, err := findColumn(t.names, part)
			if err != nil {
				return nil, fmt.Errorf("--y: %w", err)
			}
			if cols[y].kind != numberColumn {
				return nil, fmt.Errorf("--y: %q holds %s, not numbers", t.names[y], cols[y].kind)
			}
			for _, had := range ys {
				if had == y {
					return nil, fmt.Errorf("--y names %q twice", t.names[y])
				}
			}
			ys = append(ys, y)
		}
		return ys, nil
	}
	for j, c := range cols {
		if j != x && c.kind == numberColumn {
			ys = append(ys, j)
		}
	}
	switch {
	case len(ys) == 0:
		return nil, errors.New("no column holds numbers to plot; --list says what each holds")
	case len(ys) > maxDefaultColumns:
		return nil, fmt.Errorf("%d columns hold numbers, too many to plot at once: choose with --y; --list lists them", len(ys))
	}
	return ys, nil
}

// writeColumns lists t's columns: number, name, what each holds and how
// many of its rows hold a value. Through the table package, which measures
// a column's names in a terminal's columns: tabwriter counted runes, and a
// name in CJK pushed the columns after it out of line.
func writeColumns(w io.Writer, t *table, cols []column) error {
	tb := tbl.New(tbl.Column{Header: "#", Align: tbl.Right}, tbl.Column{Header: "column"}, tbl.Column{Header: "holds"}, tbl.Column{Header: "values"})
	for j, c := range cols {
		holds := c.kind.String()
		if c.wrong > 0 {
			holds += fmt.Sprintf(" (%s not)", cells(c.wrong))
		}
		tb.MustAppend(j+1, t.names[j], holds, fmt.Sprintf("%d of %d", c.values, len(t.rows)))
	}
	return tb.Render(w, tbl.Style{Spacing: 2})
}

func cells(n int) string {
	if n == 1 {
		return "1 cell"
	}
	return strconv.Itoa(n) + " cells"
}

func rowsOf(n int) string {
	if n == 1 {
		return "1 row with"
	}
	return strconv.Itoa(n) + " rows with"
}

// loadFonts reads the fonts --font names.
func loadFonts(paths []string) ([]*opentype.Font, error) {
	var fonts []*opentype.Font
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("--font: %w", err)
		}
		f, err := opentype.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("--font %s: %w", p, err)
		}
		fonts = append(fonts, f)
	}
	return fonts, nil
}

// The size of a PNG where --width and --height do not say: wide, as a chart
// along an axis reads best, and at a size a screen shows whole -- for one
// column, and 300 pixels more for each further one.
const (
	imageWidth  = 1600
	imageHeight = 600
)

func imageHeightFor(strips int) int { return imageHeight + 300*max(strips-1, 0) }

// terminalMaxWidth is the widest picture drawn for the terminal, in
// pixels: one the size of a screen of two pixels to a point is seconds to
// draw and megabytes to send, for a picture the terminal shows no sharper.
const terminalMaxWidth = 3200.0

// drawing is a series to draw, and how.
type drawing struct {
	series series
	opts   chartOptions
	fonts  []*opentype.Font
	theme  string
	getenv func(string) string
}

// palette is the picture's palette for --theme: for auto, light for a PNG,
// read on a page, and in a terminal the terminal's own, as COLORFGBG says
// it -- dark where it does not say.
func (d drawing) palette(inTerminal bool) (picture.Palette, error) {
	switch d.theme {
	case "light":
		return picture.Light, nil
	case "dark":
		return picture.Dark, nil
	case "auto":
		if inTerminal && !terminal.LightBackground(d.getenv) {
			return picture.Dark, nil
		}
		return picture.Light, nil
	}
	return picture.Palette{}, cli.Usagef("--theme %q: use auto, light or dark", d.theme)
}

// picture is the chart drawn width by height pixels.
func (d drawing) picture(width, height int, inTerminal bool) (*image.RGBA, error) {
	pal, err := d.palette(inTerminal)
	if err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("a chart %d by %d pixels has nothing to draw on", width, height)
	}
	// Ticks are spaced in proportion to the text, so a picture drawn at
	// twice the pixels for a sharp terminal gets the same ticks, not twice
	// as many; a strip's by its share of the height.
	n := len(d.series.yNames)
	text := picture.TextPx(width, height, n)
	strip := float64(height) / float64(n)
	c, err := buildChart(d.series, d.opts, width, max(int(float64(width)/(text*9)), 2), max(int(strip/(text*4.5)), 2))
	if err != nil {
		return nil, err
	}
	return picture.Draw(c, width, height, picture.Options{Palette: pal, Fonts: d.fonts})
}

// writePNG writes the chart to path, width by height pixels or the default
// size where either is zero.
func (d drawing) writePNG(path string, width, height int) error {
	if !strings.EqualFold(filepath.Ext(path), ".png") {
		return cli.Usagef("--out %q: a chart is written as a PNG; name it .png", path)
	}
	if width == 0 {
		width = imageWidth
	}
	if height == 0 {
		height = imageHeightFor(len(d.series.yNames))
	}
	img, err := d.picture(width, height, false)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return f.Close()
}

// writeTerminal draws the chart on out, as a picture or in braille as
// terminal.Choose decides for mode, the size of the terminal or of width
// and height in characters where set.
func (d drawing) writeTerminal(out, errOut io.Writer, mode terminal.Protocol, width, height int, widthSet, heightSet bool) error {
	f, isFile := out.(*os.File)
	ts, tty := terminal.Size{}, false
	if isFile {
		ts, tty = terminal.SizeOf(f)
	}
	how, note, err := terminal.Choose(mode, d.getenv, tty)
	if err != nil {
		return cli.Usagef("--terminal %v", err)
	}
	if note != "" {
		fmt.Fprintln(errOut, note+"; --terminal iterm or kitty draws one anyway")
	}
	n := len(d.series.yNames)
	cols, rows := ts.Cols, ts.Rows
	if !tty {
		cols, rows = envInt(d.getenv, "COLUMNS", 100), envInt(d.getenv, "LINES", 0)
	}
	w, h := cols, 24+10*(n-1)
	if rows > 0 {
		h = min(h, rows-3)
	}
	// 0 is the default, as it is for a PNG, and as diagram has it.
	if widthSet && width > 0 {
		w = width
	}
	if heightSet && height > 0 {
		h = height
	}
	if how == terminal.Braille {
		return d.writeBraille(out, w, h)
	}
	// As many pixels as the characters cover, twice over so a screen of two
	// pixels to a point draws it sharp, in a PNG's proportions, no taller
	// than the rows less one for the prompt. A terminal that does not say
	// its size in pixels is taken to have characters ten by twenty.
	cellW, cellH := 10.0, 20.0
	if ts.Width > 0 && ts.Height > 0 && ts.Cols > 0 && ts.Rows > 0 {
		cellW, cellH = float64(ts.Width)/float64(ts.Cols), float64(ts.Height)/float64(ts.Rows)
	}
	scale := min(1, terminalMaxWidth/(2*cellW*float64(w)))
	pw := int(2 * cellW * float64(w) * scale)
	ph := min(pw*imageHeightFor(n)/imageWidth, int(2*cellH*float64(max(h-1, 1))*scale))
	img, err := d.picture(pw, ph, true)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return terminal.Write(out, how, buf.Bytes(), w)
}

// writeBraille draws the chart in braille on out, width by height
// characters.
func (d drawing) writeBraille(out io.Writer, width, height int) error {
	// Each strip's rows, as Braille shares them out, ticked about every
	// three, and the x axis about every fourteen characters: a date's
	// label is longer than a number's.
	n := len(d.series.yNames)
	rows := height - 3
	if n > 1 {
		rows = (height - 3 - n) / n
	}
	c, err := buildChart(d.series, d.opts, 2*width, max(width/14, 2), max(rows/3, 3))
	if err != nil {
		return err
	}
	lines := c.Braille(width, height)
	if lines == nil {
		return fmt.Errorf("%d by %d characters is too small for a chart", width, height)
	}
	for _, l := range lines {
		fmt.Fprintln(out, l)
	}
	return nil
}

func envInt(getenv func(string) string, name string, fallback int) int {
	if n, err := strconv.Atoi(getenv(name)); err == nil && n > 0 {
		return n
	}
	return fallback
}

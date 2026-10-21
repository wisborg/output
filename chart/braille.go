package chart

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/clipperhouse/uax29/v2/graphemes"
	"github.com/mattn/go-runewidth"
)

// Chart is what a chart draws: one strip per metric, stacked, sharing the
// x axis beneath the last of them, under one title.
//
// Several metrics are stacked rather than drawn on one plot with an axis
// each side: two scales over one line leave the reader working out which
// line reads against which, and a third has nowhere to go. A strip each
// keeps every line on its own scale, and the shared x axis keeps a moment
// in one strip directly above the same moment in the next -- which is the
// point of charting them together.
type Chart struct {
	// Title names what is drawn, against what: "power (W) against time".
	// It is written over the chart.
	Title string
	// X is the scale the strips share; see ExactScale.
	X Scale
	// Strips are the metrics, top to bottom.
	Strips []Strip
	// Bands are the stretches of the x axis the chart marks, behind every
	// strip in a picture and in rows beneath the x axis in text; see Band.
	Bands []Band
	// BandKinds are the kinds of band, which a Band names by its index
	// here: what each is called, and how it is marked.
	BandKinds []BandKind
}

// Strip is one metric of a chart: its readings and the scale they are
// drawn on.
type Strip struct {
	// Title names the metric, "power (W)": written over its strip where a
	// chart has several, and in the chart's Title where it has one.
	Title string
	// Y is the strip's scale; see NewScale.
	Y Scale
	// Segments are the readings, as Segments and Smooth leave them.
	Segments [][]Point
}

// brailleBits is the bit each dot of a braille cell sets, by row and
// column: the cell is two dots wide and four tall, and Unicode numbers the
// first six dots down the left column and then the right, and the last two
// across the bottom.
var brailleBits = [4][2]rune{{0x01, 0x08}, {0x02, 0x10}, {0x04, 0x20}, {0x40, 0x80}}

const brailleBlank = rune(0x2800)

// Braille is the chart drawn in text: each strip's line in braille dots,
// two by four to a character, with its y scale's labels to its left, the
// strips one above the other beneath the title, and the x scale's labels
// beneath the last. Where there are several strips, each is headed by its
// own title, and its bands are marked in a row a kind beneath the x
// labels, over a legend. width and height are the whole drawing's in
// characters, labels and bands and all -- a title or a legend longer than
// the width is cut to it -- and the strips share the height evenly. A drawing
// too small to give each strip at least two rows is none, and so is a chart
// Check refuses.
//
// Text is the chart's lowest resolution, and the one place a renderer's
// choices are seen dot by dot, so it follows the model to the letter: a
// column's mean is its dot, a line joins two columns only where Bin joined
// them, and a column without a reading is left blank -- never dropped to
// the bottom, which would read as a zero.
func (c Chart) Braille(width, height int) []string {
	n := len(c.Strips)
	if n == 0 || c.Check() != nil {
		return nil
	}
	// One gutter for every strip, the widest's, so the plots line up and a
	// moment in one is above the same moment in the next.
	gutter := 0
	for _, s := range c.Strips {
		for _, v := range s.Y.Ticks() {
			gutter = max(gutter, utf8.RuneCountInString(s.Y.Label(v)))
		}
	}
	headed := 0
	if n > 1 {
		headed = 1
	}
	// Above the strips the title, below them the axis and its labels,
	// and below those the bands.
	fixed := 3 + n*headed + BandRows(len(c.UsedBandKinds()))
	cols, rows := width-gutter-1, (height-fixed)/n
	if cols < 4 || rows < 2 {
		return nil
	}
	spare := height - fixed - n*rows
	pad := strings.Repeat(" ", gutter)
	out := []string{cut(c.Title, width)}
	for i, s := range c.Strips {
		if headed > 0 {
			out = append(out, cut(pad+"│"+s.Title, width))
		}
		// The rows left over when the height does not divide go to the
		// first strips, a row each.
		r := rows
		if i < spare {
			r++
		}
		out = append(out, s.braille(c.X, gutter, cols, r)...)
	}
	axis, xLabels := c.xAxis(cols)
	out = append(out, pad+"└"+axis, pad+" "+xLabels)
	return append(out, c.brailleBands(gutter, cols)...)
}

// Check says what is wrong with a chart that cannot be drawn truthfully, or
// nil: a scale left at its zero value, which places every reading at once
// and so none -- an empty frame that looks like a chart of nothing.
// Braille draws such a chart as nothing, as it draws one too small, and the
// picture package refuses it with this error.
func (c Chart) Check() error {
	if len(c.Strips) == 0 {
		return errors.New("chart: no strips to draw")
	}
	if err := c.X.fault(); err != nil {
		return fmt.Errorf("chart: the X scale %w", err)
	}
	for i, s := range c.Strips {
		if err := s.Y.fault(); err != nil {
			return fmt.Errorf("chart: strip %d (%q): its Y scale %w", i+1, s.Title, err)
		}
	}
	return nil
}

// fault is what is wrong with s as a scale to draw on, or nil: not set,
// not numbers, or running from Min to a Max no greater -- an inverted
// scale is Inverted, with Min below Max still. Each was "not set" once,
// which sent a caller who had set the scale backwards looking for where
// they had forgotten to.
func (s Scale) fault() error {
	switch {
	case s == (Scale{}):
		return errors.New("is not set")
	case !finite(s.Min) || !finite(s.Max):
		return fmt.Errorf("runs from %v to %v, which are not both numbers", s.Min, s.Max)
	case !(s.Max > s.Min):
		return fmt.Errorf("runs from %v to %v: Max must be above Min (Inverted turns a scale round)", s.Min, s.Max)
	}
	return nil
}

// cut is s made printable and cut to at most n terminal columns, measured
// by runewidth.StringWidth as table and progress measure theirs. It cuts
// between characters -- grapheme clusters, measured whole -- and never
// writes half of one: counted in runes, a title in CJK came out twice the
// width asked for, and a family of emoji cut part way left a joiner
// dangling.
//
// Printable: a character hidden has, or a byte that is not UTF-8, is
// written as U+FFFD. A title, a strip's name and a band kind's are text someone
// else may have written -- plot names its strips after a CSV's column
// headings -- and an escape in one written to a terminal is a command to it:
// a heading holding "\x1b]0;...\x07" retitled the window when the file was
// plotted.
func cut(s string, n int) string {
	if !utf8.ValidString(s) || strings.ContainsFunc(s, hidden) {
		s = strings.Map(func(r rune) rune { // a byte that is not UTF-8 comes as U+FFFD
			if hidden(r) {
				return '\uFFFD'
			}
			return r
		}, s)
	}
	if runewidth.StringWidth(s) <= n {
		return s
	}
	used, end := 0, 0
	g := graphemes.FromString(s)
	for g.Next() {
		w := runewidth.StringWidth(g.Value())
		if used+w > n {
			break
		}
		used += w
		end = g.End()
	}
	return s[:end]
}

// hidden is whether r is written as U+FFFD: a control character, a line or
// paragraph separator, or a format character -- a bidi override, a
// zero-width space, a byte-order mark. A format character draws nothing,
// and a terminal's reader cannot see it: U+202E turns the rest of a line
// round, so "exe.txt" in a name reads as something else, and a zero-width
// space makes two names that look the same differ. The joiners U+200C and
// U+200D are kept: they are part of the text they are in -- an emoji
// family, a Persian word -- and a width measures it with them.
func hidden(r rune) bool {
	switch {
	case unicode.IsControl(r), r == '\u2028', r == '\u2029':
		return true
	case r == '\u200C', r == '\u200D':
		return false
	}
	return unicode.Is(unicode.Cf, r)
}

// braille is the strip's rows: its line drawn cols characters wide and
// rows tall against x, its y labels in a gutter gutter characters wide.
func (s Strip) braille(x Scale, gutter, cols, rows int) []string {
	w, h := cols*2, rows*4
	dots := make([][]bool, h)
	for i := range dots {
		dots[i] = make([]bool, w)
	}
	row := func(y float64) int {
		f, _ := s.Y.Frac(y)
		return int((1-f)*float64(h-1) + 0.5)
	}
	prev, prevRow := -1, 0
	first, last := x.Ends()
	for x, col := range Bin(s.Segments, first, last, w) {
		if !col.Ok {
			continue
		}
		r := row(col.Mean)
		if col.Joined && prev >= 0 {
			line(dots, prev, prevRow, x, r)
		}
		dots[r][x] = true
		prev, prevRow = x, r
	}

	// The y labels sit on the rows of character their ticks fall in; a
	// tick sharing a row with one already labelled is not labelled, rather
	// than the two written over each other.
	labels := make([]string, rows)
	for _, v := range s.Y.Ticks() {
		r := row(v) / 4
		if labels[r] == "" {
			labels[r] = s.Y.Label(v)
		}
	}

	out := make([]string, 0, rows)
	for r := range rows {
		var b strings.Builder
		b.WriteString(strings.Repeat(" ", gutter-utf8.RuneCountInString(labels[r])))
		b.WriteString(labels[r])
		if labels[r] != "" {
			b.WriteRune('┤')
		} else {
			b.WriteRune('│')
		}
		for x := range cols {
			ch := brailleBlank
			for dy := range 4 {
				for dx := range 2 {
					if dots[r*4+dy][x*2+dx] {
						ch |= brailleBits[dy][dx]
					}
				}
			}
			b.WriteRune(ch)
		}
		out = append(out, b.String())
	}
	return out
}

// xAxis is the x axis drawn cols characters long, a mark at each tick, and
// the line of labels beneath it: each label centred on its tick where it
// fits, the first starting at its tick and none past the end, and any that
// would touch the one before it left out.
func (c Chart) xAxis(cols int) (axis, labels string) {
	marks := []rune(strings.Repeat("─", cols))
	text := []rune(strings.Repeat(" ", cols))
	end := -2 // the last character a label used: none yet
	for _, v := range c.X.ticksLeftToRight() {
		f, _ := c.X.Frac(v)
		at := min(int(f*float64(cols*2-1))/2, cols-1)
		marks[at] = '┬'
		l := []rune(c.X.Label(v))
		start := max(at-len(l)/2, 0)
		if start+len(l) > cols {
			start = cols - len(l)
		}
		if start <= end+1 || start < 0 {
			// Touching the label before it, or wider than the axis.
			continue
		}
		copy(text[start:], l)
		end = start + len(l) - 1
	}
	return string(marks), strings.TrimRight(string(text), " ")
}

// line sets the dots from (x0, y0) to (x1, y1), x0 left of x1, so that
// every column between holds one and every row the line crosses in a
// column is set: a steep rise between two columns is drawn as the rise it
// is, not as two dots far apart.
func line(dots [][]bool, x0, y0, x1, y1 int) {
	for x := x0; x <= x1; x++ {
		// The rows the line spans across this column: from where it
		// enters to where it leaves.
		from := y0 + (y1-y0)*(2*(x-x0)-1)/(2*max(x1-x0, 1))
		to := y0 + (y1-y0)*(2*(x-x0)+1)/(2*max(x1-x0, 1))
		if x == x0 {
			from = y0
		}
		if x == x1 {
			to = y1
		}
		lo, hi := min(from, to), max(from, to)
		lo, hi = max(lo, min(y0, y1)), min(hi, max(y0, y1))
		for y := lo; y <= hi; y++ {
			dots[y][x] = true
		}
	}
}

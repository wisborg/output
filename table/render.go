package table

import (
	"fmt"
	"io"
	"strings"
)

// DefaultSpacing is the gap, in spaces, between adjacent columns of an
// unframed table. Three is wide enough that a right-aligned column and the
// left-aligned one beside it do not read as a single run of text, which two
// spaces does at small column widths.
const DefaultSpacing = 3

// Style controls how a table is drawn. The zero Style is a valid, unframed,
// single-line table with DefaultSpacing between columns -- so Render(w,
// table.Style{}) is the plain rendering, and each field turns on one
// departure from it.
type Style struct {
	// Frame draws rules and pipes around every cell, in the style of the
	// mysql client:
	//
	//	+--------+-------+
	//	| offset | score |
	//	+--------+-------+
	//	| +2.75s | 0.366 |
	//	+--------+-------+
	Frame bool

	// Spacing is the number of spaces between adjacent columns when Frame
	// is false. 0 means DefaultSpacing; use a negative value for no gap at
	// all, which is otherwise unreachable.
	Spacing int

	// Multiline splits cell values on "\n" and gives each line its own
	// physical row, keeping the columns aligned:
	//
	//	name    detail
	//	------  --------------
	//	first   line one
	//	        line two
	//
	// Without it a value containing a newline is written through as-is,
	// which breaks the alignment of everything after it. It is off by
	// default because detecting newlines costs a scan of every cell and
	// most tables have none.
	Multiline bool
}

// spacing resolves Spacing's 0-means-default convention.
func (s Style) spacing() int {
	if s.Spacing == 0 {
		return DefaultSpacing
	}
	if s.Spacing < 0 {
		return 0
	}
	return s.Spacing
}

// String renders the table in the default style. It makes *Table satisfy
// fmt.Stringer, so a table can be passed straight to fmt.Println.
//
// Errors from the underlying write cannot occur against a strings.Builder,
// so this signature has nothing to report; use Render to write to a real
// io.Writer and see its error.
func (t *Table) String() string {
	var b strings.Builder
	// strings.Builder never returns an error from Write.
	_ = t.Render(&b, Style{})
	return b.String()
}

// Render writes the table to w in the given style.
//
// A table with no rows renders as nothing at all -- not a header, not an
// empty frame. A caller that wants "no results" said out loud should say it
// themselves, because only they know what the absence means; a bare header
// over nothing reads as though the data failed to load.
func (t *Table) Render(w io.Writer, style Style) error {
	lay := t.layout(style)
	if lay == nil {
		return nil
	}

	bw := &errWriter{w: w}
	if style.Frame {
		lay.rule(bw)
	}
	lay.line(bw, lay.header)
	lay.rule(bw)

	// Starts true because the header rule has just been drawn: a separator
	// added before any row would otherwise double it.
	prevRule := true
	for _, e := range lay.body {
		if e.rule {
			// Collapse runs of separators, and drop one that would sit
			// directly under the header rule that was just drawn.
			if !prevRule {
				lay.rule(bw)
				prevRule = true
			}
			continue
		}
		prevRule = false
		for _, physical := range e.lines {
			lay.line(bw, physical)
		}
	}
	if style.Frame && !prevRule {
		lay.rule(bw)
	}
	return bw.err
}

// layout is a table resolved for one particular Style: every cell already
// formatted and split into physical lines, and every column width already
// measured. Rendering is then pure assembly, which is what keeps Render
// itself readable.
type layout struct {
	widths []int
	// align is snapshotted from Table.Columns, so that a Style's rendering
	// cannot be changed halfway through by a caller mutating the table.
	align   []Align
	header  []string // the heading text of each column
	body    []bodyRow
	style   Style
	spacing int
	// widthOf is Table.width, carried along so padding is measured with
	// exactly the function that measured the columns. Two different
	// measures here is a bug that shows only on non-ASCII data.
	widthOf func(string) int
}

// bodyRow is one entry resolved for rendering: either a rule, or the
// physical lines a data row expands to. lines[i][j] is column j's text on
// physical line i, already padded-to-width at render time rather than here.
type bodyRow struct {
	rule  bool
	lines [][]string
}

// layout formats every cell and measures every column. It returns nil when
// there is nothing to draw.
func (t *Table) layout(style Style) *layout {
	if len(t.Columns) == 0 {
		return nil
	}
	hasRow := false
	for _, e := range t.entries {
		if !e.rule {
			hasRow = true
			break
		}
	}
	if !hasRow {
		return nil
	}

	lay := &layout{
		widths:  make([]int, len(t.Columns)),
		align:   make([]Align, len(t.Columns)),
		header:  make([]string, len(t.Columns)),
		style:   style,
		spacing: style.spacing(),
		widthOf: t.width,
	}
	for i, c := range t.Columns {
		h := t.truncate(c.Header, c.MaxWidth)
		lay.header[i] = h
		lay.widths[i] = t.width(h)
		lay.align[i] = c.Align
	}

	for _, e := range t.entries {
		if e.rule {
			lay.body = append(lay.body, bodyRow{rule: true})
			continue
		}
		// cells[i] is column i's text split into its physical lines.
		cells := make([][]string, len(t.Columns))
		height := 1
		for i := range t.Columns {
			var raw string
			if i < len(e.cells) {
				raw = t.format(t.Columns[i], e.cells[i])
			}
			lines := []string{raw}
			if style.Multiline {
				lines = strings.Split(raw, "\n")
			}
			for j, l := range lines {
				l = t.truncate(l, t.Columns[i].MaxWidth)
				lines[j] = l
				if n := t.width(l); n > lay.widths[i] {
					lay.widths[i] = n
				}
			}
			cells[i] = lines
			if len(lines) > height {
				height = len(lines)
			}
		}
		// Transpose to physical lines, padding short cells with blanks so
		// every physical line has one entry per column.
		physical := make([][]string, height)
		for r := 0; r < height; r++ {
			physical[r] = make([]string, len(t.Columns))
			for i := range t.Columns {
				if r < len(cells[i]) {
					physical[r][i] = cells[i][r]
				}
			}
		}
		lay.body = append(lay.body, bodyRow{lines: physical})
	}
	return lay
}

// format turns one cell value into text, before truncation.
func (t *Table) format(c Column, v any) string {
	if c.Format == "" {
		return fmt.Sprint(v)
	}
	return fmt.Sprintf(c.Format, v)
}

// truncate cuts s to at most maxWidth display columns, dropping any final
// character that would straddle the limit rather than emitting half of it.
// maxWidth <= 0 means no limit.
func (t *Table) truncate(s string, maxWidth int) string {
	if maxWidth <= 0 || t.width(s) <= maxWidth {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := t.width(string(r))
		if used+rw > maxWidth {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	return b.String()
}

// line writes one physical line: cells padded to their column widths and
// joined with the style's separators.
func (l *layout) line(w *errWriter, cells []string) {
	var b strings.Builder
	for i, width := range l.widths {
		var cell string
		if i < len(cells) {
			cell = cells[i]
		}
		if l.style.Frame {
			b.WriteString("| ")
		} else if i > 0 {
			b.WriteString(strings.Repeat(" ", l.spacing))
		}
		b.WriteString(l.pad(cell, width, i))
		if l.style.Frame {
			b.WriteString(" ")
		}
	}
	if l.style.Frame {
		b.WriteString("|")
		w.line(b.String())
		return
	}
	// Unframed, the padding on the last column is invisible but real, and
	// trailing whitespace is noise in a diff, in a terminal selection, and
	// in a golden-file test. Framed output keeps it, because there the
	// closing pipe has to land in the same place on every line.
	w.line(strings.TrimRight(b.String(), " "))
}

// pad aligns cell within width display columns.
func (l *layout) pad(cell string, width, col int) string {
	// The measured width comes from the layout, which used Table.width; the
	// same measure has to be used here or padding and measurement disagree.
	n := width - l.widthOf(cell)
	if n <= 0 {
		return cell
	}
	align := Left
	if col < len(l.align) {
		align = l.align[col]
	}
	switch align {
	case Right:
		return strings.Repeat(" ", n) + cell
	case Center:
		left := n / 2
		return strings.Repeat(" ", left) + cell + strings.Repeat(" ", n-left)
	default:
		return cell + strings.Repeat(" ", n)
	}
}

// rule writes a horizontal rule matching the current style.
func (l *layout) rule(w *errWriter) {
	var b strings.Builder
	if l.style.Frame {
		for _, width := range l.widths {
			b.WriteString("+")
			b.WriteString(strings.Repeat("-", width+2))
		}
		b.WriteString("+")
		w.line(b.String())
		return
	}
	// Unframed, the rule runs unbroken through the inter-column gaps: a
	// dashed underline with holes in it reads as several rules rather than
	// one, and the eye uses it to find the top of the data.
	total := 0
	for i, width := range l.widths {
		total += width
		if i > 0 {
			total += l.spacing
		}
	}
	b.WriteString(strings.Repeat("-", total))
	w.line(b.String())
}

// errWriter accumulates the first write error so the render path can stay
// free of error checks on every line, in the manner of bufio.Writer.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) line(s string) {
	if e.err != nil {
		return
	}
	_, e.err = io.WriteString(e.w, s+"\n")
}

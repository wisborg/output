// Package table builds tabular data and renders it as aligned text.
//
// A Table is a set of Columns plus the rows added to them. Cell values are
// stored as given -- an int stays an int -- and are only turned into text
// when the table is rendered. That ordering is deliberate: a value formatted
// on the way in cannot be recovered, so keeping it typed is what leaves room
// for renderers other than text (CSV first, since it shares this row model)
// without a change to how tables are built.
//
// The zero value of a Table is not useful; construct one with New.
//
//	t := table.New(
//		table.Column{Header: "offset", Align: table.Right, Format: "%+.2fs"},
//		table.Column{Header: "score", Align: table.Right, Format: "%.3f"},
//		table.Column{Header: "note"},
//	)
//	t.Append(2.75, 0.366, "the corner")
//	fmt.Println(t)
//
// # Display width, not byte length
//
// Column widths are measured in terminal columns, via Table.Width. The
// default handles East Asian wide characters, combining marks and emoji
// correctly; len() and utf8.RuneCountInString both get at least one of those
// wrong, and the symptom is a table that looks fine until someone's data is
// not ASCII. See Table.Width to substitute a cheaper measure.
package table

import (
	"fmt"

	"github.com/mattn/go-runewidth"
)

// Align is a column's horizontal alignment. The zero value is Left, so a
// Column literal that says nothing about alignment gets the conventional
// default for text.
type Align int

const (
	// Left pads on the right. The zero value, and the right default for
	// text: ragged right edges are easy to read down.
	Left Align = iota
	// Right pads on the left. Use it for numbers, where digits lining up
	// under each other is the whole point of a table.
	Right
	// Center splits the padding, with any odd column going to the right.
	Center
)

// String renders the alignment's name, for diagnostics and test failures.
func (a Align) String() string {
	switch a {
	case Left:
		return "left"
	case Right:
		return "right"
	case Center:
		return "center"
	default:
		return fmt.Sprintf("Align(%d)", int(a))
	}
}

// Column describes one column: its heading and how its cells are turned into
// text. Every field is safe to change after the Table is built and before it
// is rendered -- nothing about a column is baked into the rows, because cells
// are only formatted at render time. Changing Align on a table you have
// already filled is a supported operation, not a trick:
//
//	t.Columns[1].Align = table.Right
type Column struct {
	// Header is the column heading. It participates in the column's width,
	// so a heading longer than every value sets that column's width.
	Header string

	// Align is how cells AND the heading are positioned within the column.
	//
	// Note the heading follows the column, rather than always being
	// left-aligned: over a right-aligned numeric column, a left-aligned
	// heading floats away from the digits it names and is measurably
	// harder to scan. This is a deliberate difference from some other
	// table renderers.
	Align Align

	// Format is a fmt verb applied to each cell value, such as "%.2f" or
	// "%+d" or "%q". Empty means the value is rendered with %v.
	//
	// This is a fmt verb rather than a bespoke format language because Go
	// programmers already know fmt, already have its documentation, and
	// get its whole vocabulary -- width, precision, sign, base -- without
	// this package reimplementing any of it. A verb that does not match
	// the value's type produces fmt's own %!v(...) marker in the cell,
	// which is visible in the output rather than silently wrong.
	Format string

	// MaxWidth truncates a rendered cell to this many display columns.
	// 0 (the zero value) means unlimited.
	//
	// Truncation is by display width, so a wide character that would
	// straddle the limit is dropped rather than half-printed. It is hard
	// truncation with no ellipsis: an ellipsis would have to be counted
	// against MaxWidth, which makes the limit mean two different things
	// depending on whether it was hit.
	MaxWidth int
}

// Table is a set of columns and the rows added to them. It is not safe for
// concurrent use; build a Table on one goroutine and render it there, or
// guard it yourself.
type Table struct {
	// Columns describes the columns. It is exported so a caller can adjust
	// a column between building the table and rendering it -- typically
	// alignment, once the data has shown what a column actually holds.
	//
	// Appending to or truncating this slice after rows exist is allowed
	// but is unlikely to be what you want: Render pads rows that are too
	// short and ignores cells with no column, so the result is a table
	// with blank or missing data rather than an error.
	Columns []Column

	// Width measures a string's width in terminal columns. It defaults to
	// runewidth.StringWidth, which is correct for East Asian wide
	// characters, combining marks and emoji.
	//
	// Substitute a cheaper measure when the data is known to be simple and
	// the dependency's cost is not wanted -- utf8.RuneCountInString is
	// right for ASCII and precomposed Latin text, and len() is right for
	// ASCII alone. Both under-measure CJK by half and mis-measure emoji,
	// which shows up as a ragged right edge rather than as an error.
	//
	// Nil is treated as the default rather than panicking, so a Table{}
	// assembled without New still renders.
	Width func(string) int

	// entries holds rows and separators in the order they were added.
	// Separators are kept in the same sequence rather than as a set of row
	// indices, so that a separator stays attached to the position it was
	// added at no matter what is appended afterwards.
	entries []entry
}

// entry is one line's worth of table content: either a data row or a
// separator rule. A struct with a flag beats two parallel slices here
// because the interleaved ORDER is the thing being represented.
type entry struct {
	cells []any
	rule  bool
}

// New returns a Table with the given columns.
func New(columns ...Column) *Table {
	return &Table{
		Columns: columns,
		Width:   runewidth.StringWidth,
	}
}

// Append adds a row. The number of cells must match the number of columns.
//
// It returns an error rather than panicking or quietly padding, because a
// wrong cell count is a caller bug that a table would otherwise absorb into
// plausible-looking output -- a shifted column reads as bad data, not as a
// mistake in the code that produced it.
func (t *Table) Append(cells ...any) error {
	if len(cells) != len(t.Columns) {
		return fmt.Errorf("table: row has %d cells, want %d (one per column)", len(cells), len(t.Columns))
	}
	t.entries = append(t.entries, entry{cells: cells})
	return nil
}

// MustAppend is Append, panicking on a cell-count mismatch. Use it for rows
// built from literals in the same function as the New call, where the count
// is checked by reading the code and an error return is noise.
func (t *Table) MustAppend(cells ...any) {
	if err := t.Append(cells...); err != nil {
		panic(err)
	}
}

// AppendSeparator adds a horizontal rule after the rows added so far. Several
// in a row collapse to one when rendered; a leading or trailing one is
// dropped, so callers can add separators unconditionally in a loop.
func (t *Table) AppendSeparator() {
	t.entries = append(t.entries, entry{rule: true})
}

// Rows reports how many data rows have been added, not counting separators.
func (t *Table) Rows() int {
	n := 0
	for _, e := range t.entries {
		if !e.rule {
			n++
		}
	}
	return n
}

// Reset removes every row and separator, keeping the columns. It is for
// reusing a Table across several renders of the same shape.
func (t *Table) Reset() {
	t.entries = nil
}

// width measures s, resolving a nil Table.Width to the default.
func (t *Table) width(s string) int {
	if t.Width == nil {
		return runewidth.StringWidth(s)
	}
	return t.Width(s)
}

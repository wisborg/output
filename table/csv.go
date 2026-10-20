package table

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"unicode/utf8"
)

// CSVStyle controls CSV output. The zero value writes a header row and
// comma-separated fields, which is what almost every consumer expects.
type CSVStyle struct {
	// OmitHeader leaves out the header row. Use it when appending to a file
	// that already has one.
	OmitHeader bool

	// Comma is the field delimiter. 0 means ',' -- so the zero CSVStyle is
	// ordinary CSV, and this only has to be set to depart from it (e.g. '\t'
	// for TSV, or ';' where a decimal comma is in use). A quote, a line
	// break or a rune that is not valid cannot delimit anything, and
	// WriteCSV refuses one before writing a byte, with ErrBadDelimiter.
	Comma rune
}

// ErrBadDelimiter is what WriteCSV's error wraps when CSVStyle.Comma cannot
// delimit a field: a quote, a line break, or a rune that is not valid. It is
// the caller's style at fault rather than the writer, so a program that
// takes the delimiter from its user can tell the two apart with errors.Is
// and say which flag to change.
var ErrBadDelimiter = errors.New("table: cannot delimit CSV")

// WriteCSV writes the table as CSV.
//
// It shares the Table's rows with the text renderer and differs from it only
// where the two media genuinely differ:
//
//   - Align, MaxWidth and Style have no effect. Padding and truncation are
//     concessions to a fixed-width display; a CSV is data, and silently
//     shortening a value on the way into a file someone will compute with is
//     a good way to lose the value. Column.Format DOES apply, because that is
//     the caller saying how the value should be written, not how wide it may
//     be.
//   - Separators are skipped. A horizontal rule groups rows for a reader;
//     there is no row in a CSV that could carry one.
//   - A table with no rows still writes its header, where the text renderer
//     writes nothing at all. The reasons point in opposite directions: a
//     header over blank space misleads a human into thinking data failed to
//     load, while a program reading the CSV usually needs the header line to
//     know the shape of what it got, and a zero-byte file breaks parsers that
//     require one.
//
// Cells containing the delimiter, quotes or newlines are quoted by
// encoding/csv, so Style.Multiline has no CSV equivalent and needs none.
// A row that is one empty field -- an empty cell, or header, in a table of
// one column -- is written as "", not as a blank line, since a reader
// skips a blank line and the row with it.
//
// Values are written exactly as Column.Format makes them, and that includes
// a nil cell, which fmt writes as "<nil>": give the column a value of your
// own for "none" where the file is read by a program. Nor is a value
// starting with "=", "+", "-" or "@" defused: a spreadsheet opening the
// file may take one for a formula. Changing either would change the data,
// which a CSV must not.
func (t *Table) WriteCSV(w io.Writer, style CSVStyle) error {
	if c := style.Comma; c != 0 && (c == '"' || c == '\r' || c == '\n' || !utf8.ValidRune(c) || c == utf8.RuneError) {
		// Checked here, before anything is written: encoding/csv checks
		// it only on writing a record, so with no rows to write and no
		// header the same style was fine, and with rows it was not.
		return fmt.Errorf("%w with %q", ErrBadDelimiter, c)
	}
	if len(t.Columns) == 0 {
		// encoding/csv would write a bare newline for an empty record.
		// Nothing is the honest output for a table with no columns.
		return nil
	}

	cw := csv.NewWriter(w)
	if style.Comma != 0 {
		cw.Comma = style.Comma
	}
	write := func(record []string) error {
		if len(record) != 1 || record[0] != "" {
			return cw.Write(record)
		}
		// encoding/csv writes one empty field as a bare newline, which
		// its own reader, and most others, skip as a blank line: a table
		// of one column lost every row whose cell was empty. It has no
		// way to quote a field that does not need it, so the row goes
		// past it, after what it holds: a write to w for each such row.
		cw.Flush()
		if err := cw.Error(); err != nil {
			return err
		}
		_, err := io.WriteString(w, `""`+"\n")
		return err
	}

	if !style.OmitHeader {
		record := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			record[i] = c.Header
		}
		if err := write(record); err != nil {
			return err
		}
	}

	for _, e := range t.entries {
		if e.rule {
			continue
		}
		record := make([]string, len(t.Columns))
		for i := range t.Columns {
			if i < len(e.cells) {
				record[i] = t.format(t.Columns[i], e.cells[i])
			}
		}
		if err := write(record); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

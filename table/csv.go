package table

import (
	"encoding/csv"
	"io"
)

// CSVStyle controls CSV output. The zero value writes a header row and
// comma-separated fields, which is what almost every consumer expects.
type CSVStyle struct {
	// OmitHeader leaves out the header row. Use it when appending to a file
	// that already has one.
	OmitHeader bool

	// Comma is the field delimiter. 0 means ',' -- so the zero CSVStyle is
	// ordinary CSV, and this only has to be set to depart from it (e.g. '\t'
	// for TSV, or ';' where a decimal comma is in use).
	Comma rune
}

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
func (t *Table) WriteCSV(w io.Writer, style CSVStyle) error {
	if len(t.Columns) == 0 {
		// encoding/csv would write a bare newline for an empty record.
		// Nothing is the honest output for a table with no columns.
		return nil
	}

	cw := csv.NewWriter(w)
	if style.Comma != 0 {
		cw.Comma = style.Comma
	}

	if !style.OmitHeader {
		record := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			record[i] = c.Header
		}
		if err := cw.Write(record); err != nil {
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
		if err := cw.Write(record); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// Package output writes one result in whichever shape was asked for: aligned
// text, CSV, JSON or YAML.
//
// The premise is that a program builds two representations of the same
// result and lets the requested Format choose between them:
//
//   - a rich object, for JSON and YAML, where nesting, types and optional
//     fields all survive; and
//   - a deliberately simplified table, for text and CSV, where a person
//     reading a terminal wants a handful of columns and not the object.
//
// The two are built independently, on purpose. The alternative -- deriving
// the table from the object, or the object from the table -- forces the
// richer shape through the poorer one and makes both worse: JSON grows
// stringly-typed cells and pre-formatted numbers, while the table grows
// columns nobody wanted to read. Writing both is a few lines in the program
// that has the data, and each comes out shaped for its reader.
//
//	doc := output.Document{Data: result, Table: summary}
//	if err := doc.Write(os.Stdout, format); err != nil {
//		return err
//	}
//
// A Document only needs the representation the chosen format uses: JSON of a
// Document with no Table is fine, and CSV of one with no Data is fine.
// Asking for a format whose representation is missing is an error naming the
// format, and nothing is written.
//
// WriteJSON and WriteYAML are the same encoders without the Document, for a
// program that has only the object.
package output

import (
	"errors"
	"fmt"
	"io"

	"github.com/wisborg/output/table"
)

// The sentinels every error from this package can be tested for with
// errors.Is. Each returned error wraps one of these and adds what was
// actually wrong -- which format was asked for, or what was typed.
var (
	// ErrNoData is returned when a format that writes Document.Data is
	// asked for and Data is nil.
	ErrNoData = errors.New("output: no Data")
	// ErrNoTable is returned when a format that writes Document.Table is
	// asked for and Table is nil.
	ErrNoTable = errors.New("output: no Table")
	// ErrUnknownFormat is the sentinel behind every "this is not a format
	// I know" error, whether the format arrived as a string from a flag or
	// as an out-of-range Format value.
	ErrUnknownFormat = errors.New("output: unknown format")
)

// Document is one result in both of its representations, plus the styles
// each format is written in. The zero Document holds neither representation
// and can be written in no format; fill in the ones your program produces.
type Document struct {
	// Data is the object written by JSON and YAML. nil means "not
	// supplied", and asking for one of those formats is then an error
	// wrapping ErrNoData rather than a document reading "null".
	//
	// Only a nil interface counts as absent. Data holding a TYPED nil
	// pointer -- a (*Result)(nil) -- is a value, and marshals as null in
	// both formats, because at that point the program has said what it has
	// and null is the accurate answer.
	Data any

	// Table is the table written by Text and CSV. nil means "not
	// supplied", and asking for one of those formats is then an error
	// wrapping ErrNoTable.
	//
	// A table with no rows is NOT the same thing: it is a supplied answer
	// that happens to be empty, and it follows the table package's own
	// rules -- text writes nothing at all, CSV still writes its header.
	Table *table.Table

	// TextStyle is the style Text is rendered in. The zero value is the
	// plain unframed table.
	TextStyle table.Style

	// CSVStyle is the style CSV is written in. The zero value is ordinary
	// comma-separated data with a header row.
	CSVStyle table.CSVStyle

	// JSONStyle is the style JSON is written in. The zero value is
	// indented, without HTML escaping.
	JSONStyle JSONStyle

	// YAMLStyle is the style YAML is written in. The zero value is
	// two-space indentation.
	YAMLStyle YAMLStyle
}

// Write writes the document to w in format f.
//
// Only the representation f needs is required: JSON and YAML use Data, text
// and CSV use Table. A missing one is an error wrapping ErrNoData or
// ErrNoTable that names the format asked for, and nothing is written -- the
// check happens before the first byte, so a caller that reports the error
// and exits has not already put half a document on the terminal. The same
// holds for a value the encoder rejects part-way through: JSON and YAML both
// write all of a document or none of it.
//
// A Format outside the defined set is an error wrapping ErrUnknownFormat.
// There is deliberately no fallback to text: the format usually comes from a
// flag, and a program that prints a table when its caller asked for JSON has
// produced output that cannot be parsed and no message saying why.
//
// The receiver is a value because Write reads the Document and changes
// nothing in it. Copying the struct copies the Table pointer, not the table.
//
// Whatever the format, the output is either empty or ends in exactly one
// newline.
func (d Document) Write(w io.Writer, f Format) error {
	switch f {
	case Text:
		if d.Table == nil {
			return missing(ErrNoTable, f)
		}
		return d.Table.Render(w, d.TextStyle)
	case CSV:
		if d.Table == nil {
			return missing(ErrNoTable, f)
		}
		return d.Table.WriteCSV(w, d.CSVStyle)
	case JSON:
		if d.Data == nil {
			return missing(ErrNoData, f)
		}
		return WriteJSON(w, d.Data, d.JSONStyle)
	case YAML:
		if d.Data == nil {
			return missing(ErrNoData, f)
		}
		return WriteYAML(w, d.Data, d.YAMLStyle)
	default:
		return fmt.Errorf("%w %d (accepted: %s)", ErrUnknownFormat, int(f), acceptedNames())
	}
}

// missing reports that the format asked for needs a representation this
// Document does not have. It exists because the four branches of Write would
// otherwise repeat the same line, and two of those messages drifting apart
// would be a subtler bug than the duplication that allowed it.
func missing(sentinel error, f Format) error {
	return fmt.Errorf("%w to write as %s", sentinel, f)
}

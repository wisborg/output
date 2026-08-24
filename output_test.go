package output

import (
	"errors"
	"strings"
	"testing"

	"github.com/wisborg/output/table"
)

// The marker strings appear in exactly one of a Document's two
// representations, so any test can tell from the output which one was
// written -- including the case where a dispatch arm writes the wrong one
// and produces a perfectly well-formed document of the wrong thing.
const (
	objectMarker = "only_in_the_object"
	tableMarker  = "only_in_the_table"
)

type result struct {
	Clip string `json:"clip" yaml:"clip"`
	// Checksum is in the object and not in the table: detail a machine
	// wants and a person reading a terminal does not.
	Checksum string `json:"checksum" yaml:"checksum"`
}

// sampleDocument builds the two representations DIFFERENTLY on purpose. The
// object carries a field the table lacks, and the table carries a column the
// object lacks -- which is the whole premise of the package, and the only
// way a test can prove which representation an output came from.
func sampleDocument() Document {
	t := table.New(
		table.Column{Header: "clip"},
		table.Column{Header: "note"}, // no counterpart in the object
	)
	t.MustAppend("corner_1", tableMarker)
	return Document{
		Data:  result{Clip: "corner_1", Checksum: objectMarker},
		Table: t,
	}
}

// TestDocument_WriteUsesTheRepresentationTheFormatNeeds is the central test
// of the package. Each format must write ITS representation and only its
// representation: a dispatch arm wired to the wrong one still produces valid
// output of the right general shape, which no "returns no error" check and
// no well-formedness check would catch.
func TestDocument_WriteUsesTheRepresentationTheFormatNeeds(t *testing.T) {
	for _, tc := range []struct {
		format     Format
		want, deny string
	}{
		{Text, tableMarker, objectMarker},
		{CSV, tableMarker, objectMarker},
		{JSON, objectMarker, tableMarker},
		{YAML, objectMarker, tableMarker},
	} {
		var b strings.Builder
		if err := sampleDocument().Write(&b, tc.format); err != nil {
			t.Errorf("%v: %v", tc.format, err)
			continue
		}
		got := b.String()
		if !strings.Contains(got, tc.want) {
			t.Errorf("%v output does not contain %q:\n%s", tc.format, tc.want, got)
		}
		if strings.Contains(got, tc.deny) {
			t.Errorf("%v wrote the other representation (found %q):\n%s", tc.format, tc.deny, got)
		}
	}
}

// TestDocument_WriteEndsInOneNewline is the invariant shared by all four
// writers: output is either empty or ends in exactly one newline, so a
// program can print something after it without gluing the two together or
// leaving a blank line.
func TestDocument_WriteEndsInOneNewline(t *testing.T) {
	for _, f := range Formats() {
		var b strings.Builder
		if err := sampleDocument().Write(&b, f); err != nil {
			t.Errorf("%v: %v", f, err)
			continue
		}
		got := b.String()
		if got == "" {
			continue
		}
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("%v output does not end in a newline: %q", f, got)
		}
		if strings.HasSuffix(got, "\n\n") {
			t.Errorf("%v output ends in a blank line: %q", f, got)
		}
	}
}

func TestDocument_MissingTableIsAnError(t *testing.T) {
	for _, f := range []Format{Text, CSV} {
		var b strings.Builder
		d := Document{Data: result{Clip: "c", Checksum: objectMarker}} // Table nil
		err := d.Write(&b, f)
		if !errors.Is(err, ErrNoTable) {
			t.Errorf("%v: err = %v, want one wrapping ErrNoTable", f, err)
		}
		if !strings.Contains(err.Error(), f.String()) {
			t.Errorf("%v: error does not name the format: %q", f, err.Error())
		}
		if b.Len() != 0 {
			t.Errorf("%v: wrote %d bytes before failing: %q", f, b.Len(), b.String())
		}
	}
}

func TestDocument_MissingDataIsAnError(t *testing.T) {
	for _, f := range []Format{JSON, YAML} {
		var b strings.Builder
		d := Document{Table: sampleDocument().Table} // Data nil
		err := d.Write(&b, f)
		if !errors.Is(err, ErrNoData) {
			t.Errorf("%v: err = %v, want one wrapping ErrNoData", f, err)
		}
		if !strings.Contains(err.Error(), f.String()) {
			t.Errorf("%v: error does not name the format: %q", f, err.Error())
		}
		if b.Len() != 0 {
			t.Errorf("%v: wrote %d bytes before failing: %q", f, b.Len(), b.String())
		}
	}
}

// TestDocument_OnlyTheNeededRepresentationIsRequired is the other half of
// the two nil tests: a program that only produces an object must still be
// able to write JSON, and one that only produces a table must still be able
// to write CSV. Validating both representations for every format would make
// the package unusable for either.
func TestDocument_OnlyTheNeededRepresentationIsRequired(t *testing.T) {
	dataOnly := Document{Data: result{Clip: "c", Checksum: objectMarker}}
	for _, f := range []Format{JSON, YAML} {
		var b strings.Builder
		if err := dataOnly.Write(&b, f); err != nil {
			t.Errorf("%v with a nil Table: %v", f, err)
		} else if !strings.Contains(b.String(), objectMarker) {
			t.Errorf("%v with a nil Table wrote %q", f, b.String())
		}
	}

	tableOnly := Document{Table: sampleDocument().Table}
	for _, f := range []Format{Text, CSV} {
		var b strings.Builder
		if err := tableOnly.Write(&b, f); err != nil {
			t.Errorf("%v with nil Data: %v", f, err)
		} else if !strings.Contains(b.String(), tableMarker) {
			t.Errorf("%v with nil Data wrote %q", f, b.String())
		}
	}
}

// TestDocument_EmptyTableIsNotAMissingTable stops "nil means empty" from
// creeping in. A table with no rows is a supplied answer that happens to be
// empty, and it keeps the table package's own asymmetry: text writes
// nothing, CSV still writes its header.
func TestDocument_EmptyTableIsNotAMissingTable(t *testing.T) {
	empty := table.New(table.Column{Header: "clip"}, table.Column{Header: "note"})
	d := Document{Table: empty}

	var text strings.Builder
	if err := d.Write(&text, Text); err != nil {
		t.Fatalf("Text of an empty table: %v", err)
	}
	if text.String() != "" {
		t.Errorf("Text of a zero-row table = %q, want nothing at all", text.String())
	}

	var csv strings.Builder
	if err := d.Write(&csv, CSV); err != nil {
		t.Fatalf("CSV of an empty table: %v", err)
	}
	if csv.String() != "clip,note\n" {
		t.Errorf("CSV of a zero-row table = %q, want its header", csv.String())
	}
}

// TestDocument_TypedNilDataIsAValue records where the line between "no
// value" and "a value that is nil" falls: only a nil interface is absent. A
// typed nil pointer is the program saying its result is nothing, and null is
// the accurate rendering of that.
func TestDocument_TypedNilDataIsAValue(t *testing.T) {
	var typed *result
	d := Document{Data: typed}

	var b strings.Builder
	if err := d.Write(&b, JSON); err != nil {
		t.Fatalf("JSON of a typed nil pointer: %v", err)
	}
	if b.String() != "null\n" {
		t.Errorf("JSON of a typed nil pointer = %q, want %q", b.String(), "null\n")
	}

	b.Reset()
	if err := d.Write(&b, YAML); err != nil {
		t.Fatalf("YAML of a typed nil pointer: %v", err)
	}
	if b.String() != "null\n" {
		t.Errorf("YAML of a typed nil pointer = %q, want %q", b.String(), "null\n")
	}
}

func TestDocument_UnknownFormatIsAnError(t *testing.T) {
	var b strings.Builder
	err := sampleDocument().Write(&b, Format(99))
	if !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("err = %v, want one wrapping ErrUnknownFormat", err)
	}
	if !strings.Contains(err.Error(), "99") {
		t.Errorf("error does not name the value: %q", err.Error())
	}
	for _, known := range Formats() {
		if !strings.Contains(err.Error(), known.String()) {
			t.Errorf("error does not offer %q as an alternative: %q", known.String(), err.Error())
		}
	}
	if b.Len() != 0 {
		t.Errorf("wrote %d bytes for an unknown format: %q", b.Len(), b.String())
	}
}

// TestDocument_StylesReachTheirFormat catches a two-character omission --
// passing table.Style{} instead of d.TextStyle -- whose output looks
// entirely correct. Each case asserts a mark that only appears when the
// style was actually passed through.
func TestDocument_StylesReachTheirFormat(t *testing.T) {
	nested := map[string]any{"outer": map[string]any{"inner": 1}}

	for _, tc := range []struct {
		name   string
		doc    Document
		format Format
		check  func(t *testing.T, got string)
	}{
		{
			name:   "TextStyle.Frame",
			doc:    withStyle(sampleDocument(), func(d *Document) { d.TextStyle = table.Style{Frame: true} }),
			format: Text,
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "+---") {
					t.Errorf("TextStyle did not reach the renderer; no frame in:\n%s", got)
				}
			},
		},
		{
			name:   "CSVStyle.Comma",
			doc:    withStyle(sampleDocument(), func(d *Document) { d.CSVStyle = table.CSVStyle{Comma: '\t'} }),
			format: CSV,
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "clip\tnote") {
					t.Errorf("CSVStyle did not reach the writer; no tabs in:\n%q", got)
				}
			},
		},
		{
			name:   "JSONStyle.Compact",
			doc:    withStyle(sampleDocument(), func(d *Document) { d.JSONStyle = JSONStyle{Compact: true} }),
			format: JSON,
			check: func(t *testing.T, got string) {
				if n := strings.Count(got, "\n"); n != 1 {
					t.Errorf("JSONStyle did not reach the encoder; %d newlines in:\n%s", n, got)
				}
			},
		},
		{
			name: "YAMLStyle.Indent",
			doc: withStyle(Document{Data: nested}, func(d *Document) {
				d.YAMLStyle = YAMLStyle{Indent: 4}
			}),
			format: YAML,
			check: func(t *testing.T, got string) {
				if !strings.Contains(got, "\n    inner:") {
					t.Errorf("YAMLStyle did not reach the encoder; want a four-space indent in:\n%s", got)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b strings.Builder
			if err := tc.doc.Write(&b, tc.format); err != nil {
				t.Fatalf("Write: %v", err)
			}
			tc.check(t, b.String())
		})
	}

	// The same YAML document at the default style must NOT be four-space
	// indented, or the case above would pass with the style discarded.
	var b strings.Builder
	if err := (Document{Data: nested}).Write(&b, YAML); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !strings.Contains(b.String(), "\n  inner:") {
		t.Errorf("default YAMLStyle is not two-space indented:\n%s", b.String())
	}
}

// withStyle returns d with one field set, so the table above can stay a list
// of literals.
func withStyle(d Document, set func(*Document)) Document {
	set(&d)
	return d
}

func TestDocument_PropagatesWriteErrors(t *testing.T) {
	for _, f := range Formats() {
		err := sampleDocument().Write(&failWriter{n: 0}, f)
		if err == nil {
			t.Errorf("%v: a failing writer produced no error", f)
			continue
		}
		if !errors.Is(err, errWrite) {
			t.Errorf("%v: err = %v, want one wrapping %v", f, err, errWrite)
		}
	}
}

// TestDocument_StyleErrorsWriteNothing is the style fields' half of the
// nothing-written rule. A YAMLStyle the emitter would not honour is refused
// before encoding starts, so a caller that reports the error and exits has
// not put a differently-indented document on the terminal first.
func TestDocument_StyleErrorsWriteNothing(t *testing.T) {
	d := sampleDocument()
	d.YAMLStyle = YAMLStyle{Indent: 12}

	var b strings.Builder
	if err := d.Write(&b, YAML); err == nil {
		t.Fatal("an out-of-range YAMLStyle.Indent produced no error")
	}
	if b.Len() != 0 {
		t.Errorf("wrote %d bytes despite the error: %q", b.Len(), b.String())
	}
}

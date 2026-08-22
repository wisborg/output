package table

import (
	"errors"
	"strings"
	"testing"
)

func csvOf(t *testing.T, tbl *Table, style CSVStyle) string {
	t.Helper()
	var b strings.Builder
	if err := tbl.WriteCSV(&b, style); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	return b.String()
}

func TestWriteCSV_Default(t *testing.T) {
	check(t, csvOf(t, sample(t), CSVStyle{}), strings.Join([]string{
		"offset,score,note",
		"+2.75s,0.366,the corner",
		"-19.80s,0.198,decoy",
	}, "\n")+"\n")
}

func TestWriteCSV_OmitHeader(t *testing.T) {
	got := csvOf(t, sample(t), CSVStyle{OmitHeader: true})
	if strings.Contains(got, "offset") {
		t.Errorf("header written despite OmitHeader:\n%s", got)
	}
	if !strings.HasPrefix(got, "+2.75s,") {
		t.Errorf("output should start with the first data row, got %q", got)
	}
}

func TestWriteCSV_Comma(t *testing.T) {
	tbl := New(Column{Header: "a"}, Column{Header: "b"})
	tbl.MustAppend("x", "y")
	check(t, csvOf(t, tbl, CSVStyle{Comma: '\t'}), "a\tb\nx\ty\n")
	check(t, csvOf(t, tbl, CSVStyle{Comma: ';'}), "a;b\nx;y\n")
}

// TestWriteCSV_IgnoresAlignAndMaxWidth pins the deliberate difference from
// the text renderer. Padding is a display concession, and truncating a value
// on its way into a file someone will compute with loses data silently --
// the one failure a data format must not have.
func TestWriteCSV_IgnoresAlignAndMaxWidth(t *testing.T) {
	tbl := New(
		Column{Header: "wide", Align: Right, MaxWidth: 3},
		Column{Header: "n", Align: Center},
	)
	tbl.MustAppend("abcdefgh", 1)

	got := csvOf(t, tbl, CSVStyle{})
	if !strings.Contains(got, "abcdefgh") {
		t.Errorf("MaxWidth truncated a CSV value; got %q", got)
	}
	if strings.Contains(got, " ") {
		t.Errorf("alignment padding leaked into the CSV; got %q", got)
	}
}

// TestWriteCSV_AppliesFormat is the other half of the previous test: Format
// is the caller stating how a value should be written, not how wide it may
// be, so unlike Align and MaxWidth it does apply.
func TestWriteCSV_AppliesFormat(t *testing.T) {
	tbl := New(Column{Header: "v", Format: "%.2f"})
	tbl.MustAppend(3.14159)
	check(t, csvOf(t, tbl, CSVStyle{}), "v\n3.14\n")
}

func TestWriteCSV_SkipsSeparators(t *testing.T) {
	tbl := New(Column{Header: "n"})
	tbl.MustAppend(1)
	tbl.AppendSeparator()
	tbl.MustAppend(2)
	check(t, csvOf(t, tbl, CSVStyle{}), "n\n1\n2\n")
}

// TestWriteCSV_QuotesAwkwardValues is really a check that encoding/csv is
// doing the escaping rather than this package hand-rolling it -- which is
// why Style.Multiline has no CSV counterpart and needs none.
func TestWriteCSV_QuotesAwkwardValues(t *testing.T) {
	tbl := New(Column{Header: "v"})
	tbl.MustAppend("has,comma")
	tbl.MustAppend("has\nnewline")
	tbl.MustAppend(`has"quote`)

	got := csvOf(t, tbl, CSVStyle{})
	for _, want := range []string{`"has,comma"`, "\"has\nnewline\"", `"has""quote"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestWriteCSV_NoRowsStillWritesTheHeader is the asymmetry with the text
// renderer, which writes nothing at all. A program reading the CSV generally
// needs the header line to know the shape of what it got.
func TestWriteCSV_NoRowsStillWritesTheHeader(t *testing.T) {
	tbl := New(Column{Header: "a"}, Column{Header: "b"})
	check(t, csvOf(t, tbl, CSVStyle{}), "a,b\n")

	if got := tbl.String(); got != "" {
		t.Errorf("the TEXT renderer must still write nothing for an empty table, got %q", got)
	}
}

func TestWriteCSV_NoColumnsWritesNothing(t *testing.T) {
	if got := csvOf(t, New(), CSVStyle{}); got != "" {
		t.Errorf("column-less table wrote %q, want the empty string", got)
	}
}

// TestWriteCSV_PropagatesWriteErrors checks the error path that is easy to
// drop: encoding/csv buffers, so csv.Writer.Write returns nil for a small
// table and the failure only appears at Flush, via csv.Writer.Error. Code
// that checked Write's error and forgot Error would pass every other test
// here while silently discarding a failed write.
//
// Only one underlying Write happens for a table this size, which is why
// there is a single case rather than a sweep.
func TestWriteCSV_PropagatesWriteErrors(t *testing.T) {
	if err := sample(t).WriteCSV(&failWriter{n: 0}, CSVStyle{}); !errors.Is(err, errWrite) {
		t.Errorf("err = %v, want %v", err, errWrite)
	}
}

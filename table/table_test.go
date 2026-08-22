package table

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// check compares multi-line output against a want string, reporting the two
// side by side with visible line boundaries. A bare "got != want" on a table
// is unreadable, and trailing-space differences are invisible without the
// pipes.
func check(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	t.Errorf("rendered table differs:")
	for i := 0; i < max(len(gl), len(wl)); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		mark := "  "
		if g != w {
			mark = "->"
		}
		t.Errorf("  %s got |%s|", mark, g)
		t.Errorf("     want |%s|", w)
	}
}

func sample(t *testing.T) *Table {
	t.Helper()
	tbl := New(
		Column{Header: "offset", Align: Right, Format: "%+.2fs"},
		Column{Header: "score", Align: Right, Format: "%.3f"},
		Column{Header: "note"},
	)
	tbl.MustAppend(2.75, 0.366, "the corner")
	tbl.MustAppend(-19.8, 0.198, "decoy")
	return tbl
}

func TestRender_Default(t *testing.T) {
	want := strings.Join([]string{
		" offset   score   note",
		"----------------------------",
		" +2.75s   0.366   the corner",
		"-19.80s   0.198   decoy",
	}, "\n") + "\n"
	check(t, sample(t).String(), want)
}

// TestAlignCanChangeAfterRowsAreAdded is the property that motivated storing
// cell values untouched: alignment is applied at render time, so it can be
// decided after the data has shown what a column actually holds. A design
// that formatted and padded on Append could not do this without re-walking
// and re-formatting every row.
func TestAlignCanChangeAfterRowsAreAdded(t *testing.T) {
	tbl := New(Column{Header: "n"}, Column{Header: "word"})
	tbl.MustAppend(1, "a")
	tbl.MustAppend(1000, "bbbb")

	check(t, tbl.String(), strings.Join([]string{
		"n      word",
		"-----------",
		"1      a",
		"1000   bbbb",
	}, "\n")+"\n")

	tbl.Columns[0].Align = Right
	tbl.Columns[1].Align = Center

	check(t, tbl.String(), strings.Join([]string{
		"   n   word",
		"-----------",
		"   1    a",
		"1000   bbbb",
	}, "\n")+"\n")
}

func TestRender_Frame(t *testing.T) {
	tbl := New(Column{Header: "a"}, Column{Header: "b", Align: Right})
	tbl.MustAppend("x", 12)

	var b strings.Builder
	if err := tbl.Render(&b, Style{Frame: true}); err != nil {
		t.Fatal(err)
	}
	check(t, b.String(), strings.Join([]string{
		"+---+----+",
		"| a |  b |",
		"+---+----+",
		"| x | 12 |",
		"+---+----+",
	}, "\n")+"\n")
}

func TestRender_Spacing(t *testing.T) {
	tbl := New(Column{Header: "a"}, Column{Header: "b"})
	tbl.MustAppend("x", "y")

	var one, none strings.Builder
	if err := tbl.Render(&one, Style{Spacing: 1}); err != nil {
		t.Fatal(err)
	}
	check(t, one.String(), "a b\n---\nx y\n")

	// A negative Spacing means zero, which the 0-means-default convention
	// would otherwise make unreachable.
	if err := tbl.Render(&none, Style{Spacing: -1}); err != nil {
		t.Fatal(err)
	}
	check(t, none.String(), "ab\n--\nxy\n")
}

func TestRender_Multiline(t *testing.T) {
	tbl := New(Column{Header: "name"}, Column{Header: "detail"})
	tbl.MustAppend("first", "line one\nline two")
	tbl.MustAppend("second", "single")

	var b strings.Builder
	if err := tbl.Render(&b, Style{Multiline: true}); err != nil {
		t.Fatal(err)
	}
	check(t, b.String(), strings.Join([]string{
		"name     detail",
		"-----------------",
		"first    line one",
		"         line two",
		"second   single",
	}, "\n")+"\n")
}

// TestRender_MultilineOffMeasuresTheWholeValue documents the deliberate
// difference: without Style.Multiline a newline is not special, so the value
// is written through as-is. Callers get the alignment they asked for.
func TestRender_MultilineOffMeasuresTheWholeValue(t *testing.T) {
	tbl := New(Column{Header: "detail"})
	tbl.MustAppend("one\ntwo")
	if got := tbl.String(); !strings.Contains(got, "one\ntwo") {
		t.Errorf("expected the raw newline to survive, got %q", got)
	}
}

func TestSeparators(t *testing.T) {
	tbl := New(Column{Header: "n"})
	tbl.AppendSeparator() // leading: dropped, it would double the header rule
	tbl.MustAppend(1)
	tbl.AppendSeparator()
	tbl.AppendSeparator() // a run collapses to one
	tbl.MustAppend(2)

	check(t, tbl.String(), strings.Join([]string{
		"n",
		"-",
		"1",
		"-",
		"2",
	}, "\n")+"\n")
}

// TestSeparatorsKeepTheirPosition is why separators are stored in sequence
// with the rows rather than as a set of row indices: a positional index has
// to be maintained as rows arrive, and gets it wrong the moment anything is
// inserted or the table is reused.
func TestSeparatorsKeepTheirPosition(t *testing.T) {
	tbl := New(Column{Header: "n"})
	tbl.MustAppend(1)
	tbl.AppendSeparator()
	tbl.MustAppend(2)
	tbl.MustAppend(3)

	check(t, tbl.String(), "n\n-\n1\n-\n2\n3\n")
}

func TestMaxWidth_TruncatesByDisplayWidth(t *testing.T) {
	tbl := New(Column{Header: "s", MaxWidth: 5})
	tbl.MustAppend("abcdefgh")
	tbl.MustAppend("ab")
	check(t, tbl.String(), "s\n-----\nabcde\nab\n")
}

// TestMaxWidth_DropsAStraddlingWideRune checks that truncation never emits
// half a character: a two-column rune that would cross the limit is dropped
// whole, leaving the cell one column short rather than one column over.
func TestMaxWidth_DropsAStraddlingWideRune(t *testing.T) {
	tbl := New(Column{Header: "s", MaxWidth: 3})
	tbl.MustAppend("日本語") // 3 runes, 6 columns; only one fits in 3
	got := strings.Split(tbl.String(), "\n")[2]
	if got != "日" {
		t.Errorf("truncated cell = %q, want %q", got, "日")
	}
	if w := runewidth.StringWidth(got); w > 3 {
		t.Errorf("truncated cell is %d columns wide, over the MaxWidth of 3", w)
	}
}

// TestWidth_WideCharactersKeepColumnsAligned is the test the whole width
// mechanism exists for. It asserts the invariant directly -- every rendered
// line occupies the same number of terminal columns -- rather than comparing
// against a golden string, because a golden string encodes a specific right
// answer while this encodes WHY it is right. Measuring with len() or
// utf8.RuneCountInString fails this: both under-measure CJK.
func TestWidth_WideCharactersKeepColumnsAligned(t *testing.T) {
	tbl := New(Column{Header: "lang"}, Column{Header: "text"})
	tbl.MustAppend("cjk", "日本語")
	tbl.MustAppend("ascii", "abcdef")
	tbl.MustAppend("emoji", "👍")
	tbl.MustAppend("accent", "café")

	var b strings.Builder
	if err := tbl.Render(&b, Style{Frame: true}); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	first := runewidth.StringWidth(lines[0])
	for i, l := range lines {
		if w := runewidth.StringWidth(l); w != first {
			t.Errorf("line %d is %d columns wide, want %d (all lines must match)\n%s",
				i, w, first, b.String())
		}
	}
}

// TestWidth_IsSubstitutable pins that Table.Width is genuinely the single
// measure used, so a caller who does not want the runewidth dependency's
// behaviour can replace it and have the whole table follow. If padding used
// a different measure from column sizing, this would still line up on ASCII
// and quietly disagree here.
func TestWidth_IsSubstitutable(t *testing.T) {
	tbl := New(Column{Header: "t"})
	tbl.Width = utf8.RuneCountInString
	tbl.MustAppend("日本語")  // 3 runes under this measure, 6 columns under the default
	tbl.MustAppend("abcd") // 4 runes: this should therefore set the width

	lines := strings.Split(strings.TrimRight(tbl.String(), "\n"), "\n")
	if got := len(lines[1]); got != 4 {
		t.Errorf("rule is %d chars, want 4 -- Table.Width was not used for column sizing", got)
	}
}

func TestWidth_NilFallsBackToTheDefault(t *testing.T) {
	tbl := &Table{Columns: []Column{{Header: "t"}}} // no New, so Width is nil
	tbl.MustAppend("日本語")
	lines := strings.Split(strings.TrimRight(tbl.String(), "\n"), "\n")
	if got := len(lines[1]); got != 6 {
		t.Errorf("rule is %d dashes, want 6 -- a nil Width must resolve to runewidth", got)
	}
}

func TestAppend_WrongCellCountIsAnError(t *testing.T) {
	tbl := New(Column{Header: "a"}, Column{Header: "b"})
	err := tbl.Append("only one")
	if err == nil {
		t.Fatal("expected an error for a short row, got nil")
	}
	if !strings.Contains(err.Error(), "1 cells") || !strings.Contains(err.Error(), "want 2") {
		t.Errorf("error should name both counts, got %q", err)
	}
	if tbl.Rows() != 0 {
		t.Errorf("a rejected row must not be stored; Rows() = %d", tbl.Rows())
	}
}

func TestMustAppend_PanicsOnWrongCellCount(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustAppend did not panic on a short row")
		}
	}()
	New(Column{Header: "a"}, Column{Header: "b"}).MustAppend("only one")
}

// TestRender_NoRowsWritesNothing pins the choice that an empty table produces
// no output at all -- not a lone header. A header over nothing reads as data
// that failed to load; only the caller knows what "no rows" means for them.
func TestRender_NoRowsWritesNothing(t *testing.T) {
	tbl := New(Column{Header: "a"})
	if got := tbl.String(); got != "" {
		t.Errorf("empty table rendered %q, want the empty string", got)
	}
	tbl.AppendSeparator() // separators alone are still not rows
	if got := tbl.String(); got != "" {
		t.Errorf("separator-only table rendered %q, want the empty string", got)
	}
}

func TestRender_NoColumnsWritesNothing(t *testing.T) {
	if got := New().String(); got != "" {
		t.Errorf("column-less table rendered %q, want the empty string", got)
	}
}

// TestRender_UnframedHasNoTrailingWhitespace matters for golden-file tests,
// diffs and terminal selection, none of which show the difference but all of
// which are affected by it.
func TestRender_UnframedHasNoTrailingWhitespace(t *testing.T) {
	tbl := New(Column{Header: "wide-header"}, Column{Header: "b"})
	tbl.MustAppend("x", "y")
	for i, l := range strings.Split(strings.TrimRight(tbl.String(), "\n"), "\n") {
		if strings.HasSuffix(l, " ") {
			t.Errorf("line %d has trailing whitespace: %q", i, l)
		}
	}
}

func TestFormat_VerbIsApplied(t *testing.T) {
	tbl := New(Column{Header: "v", Format: "%q"})
	tbl.MustAppend("hi")
	if got := tbl.String(); !strings.Contains(got, `"hi"`) {
		t.Errorf("Format verb not applied, got %q", got)
	}
}

// TestFormat_MismatchedVerbIsVisible: a verb that does not suit the value
// produces fmt's own marker in the cell. That is deliberate -- it is wrong
// on screen where someone will see it, rather than silently plausible.
func TestFormat_MismatchedVerbIsVisible(t *testing.T) {
	tbl := New(Column{Header: "v", Format: "%.2f"})
	tbl.MustAppend("not a number")
	if got := tbl.String(); !strings.Contains(got, "%!f") {
		t.Errorf("expected a visible fmt error marker in the cell, got %q", got)
	}
}

func TestReset_KeepsColumns(t *testing.T) {
	tbl := sample(t)
	tbl.Reset()
	if tbl.Rows() != 0 {
		t.Errorf("Rows() = %d after Reset, want 0", tbl.Rows())
	}
	if len(tbl.Columns) != 3 {
		t.Errorf("Reset dropped columns: %d remain, want 3", len(tbl.Columns))
	}
	tbl.MustAppend(1.0, 2.0, "reused")
	if !strings.Contains(tbl.String(), "reused") {
		t.Error("table not usable after Reset")
	}
}

// failWriter fails on the nth write, to check that a write error surfaces
// instead of being swallowed by the errWriter that keeps Render readable.
type failWriter struct {
	n int
}

var errWrite = errors.New("write failed")

func (f *failWriter) Write(p []byte) (int, error) {
	f.n--
	if f.n < 0 {
		return 0, errWrite
	}
	return len(p), nil
}

func TestRender_PropagatesWriteErrors(t *testing.T) {
	for _, after := range []int{0, 1, 3} {
		if err := sample(t).Render(&failWriter{n: after}, Style{}); !errors.Is(err, errWrite) {
			t.Errorf("write failing after %d lines: err = %v, want %v", after, err, errWrite)
		}
	}
}

func TestAlign_String(t *testing.T) {
	for a, want := range map[Align]string{Left: "left", Right: "right", Center: "center", Align(9): "Align(9)"} {
		if got := a.String(); got != want {
			t.Errorf("Align(%d).String() = %q, want %q", int(a), got, want)
		}
	}
}

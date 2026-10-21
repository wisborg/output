package table

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/output/internal/fuzzcorpus"
)

func markdownOf(t *testing.T, tbl *Table, style MarkdownStyle) string {
	t.Helper()
	var b strings.Builder
	if err := tbl.WriteMarkdown(&b, style); err != nil {
		t.Fatalf("WriteMarkdown: %v", err)
	}
	return b.String()
}

// pipes is how many of line's pipes end a cell: those not escaped, after
// an even run of backslashes.
func pipes(line string) int {
	n, slashes := 0, 0
	for _, r := range line {
		if r == '|' && slashes%2 == 0 {
			n++
		}
		if r == '\\' {
			slashes++
		} else {
			slashes = 0
		}
	}
	return n
}

// Whatever a cell holds -- a pipe, a line break, markup, an escape for the
// terminal, wide characters -- the table keeps its shape: a line for the
// headings, one of dashes and one a row, each with a cell a column, and
// every line as wide as the others, so the source lines up too. A pipe or a
// newline let through would end a cell or the table part way, and the
// rendered table would lose or shift every cell after it.
func TestWriteMarkdown_CellsNeverBreakTheTable(t *testing.T) {
	tbl := New(Column{Header: "na|me"}, Column{Header: "n", Align: Right}, Column{Header: "note", Align: Center})
	for _, v := range []string{
		"a|b", `a\|b`, `a\\|b`, "one\ntwo\r\nthree\n\n", "*bold* _it_ ~x~", "[x](http://e.com) <b>&amp;",
		"`code`", "$x$", "\x1b]0;t\x07", "日本語", "👨‍👩‍👧", "trailing\\", "",
	} {
		tbl.MustAppend(v, 1, v)
	}
	tbl.AppendSeparator()
	for _, style := range []MarkdownStyle{{}, {Markup: true}} {
		got := markdownOf(t, tbl, style)
		lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if len(lines) != tbl.Rows()+2 {
			t.Errorf("markup %v: %d lines for %d rows:\n%s", style.Markup, len(lines), tbl.Rows(), got)
		}
		for _, l := range lines {
			if n := pipes(l); n != len(tbl.Columns)+1 {
				t.Errorf("markup %v: %d cell-ending pipes, want %d: %q", style.Markup, n, len(tbl.Columns)+1, l)
			}
			if w := tbl.width(l); w != tbl.width(lines[0]) {
				t.Errorf("markup %v: a line %d columns wide where the first is %d: %q", style.Markup, w, tbl.width(lines[0]), l)
			}
			if strings.ContainsAny(l, "\x1b\x07\r") {
				t.Errorf("markup %v: a control character in %q", style.Markup, l)
			}
		}
		if !strings.Contains(got, "| one<br>two<br>three ") {
			t.Errorf("markup %v: line breaks not written as <br>:\n%s", style.Markup, got)
		}
	}
}

// Each character that would be markup is escaped, so a cell reads as it
// was written: a name with stars in it is not set in italics, a link is
// not a link. An underscore inside a word is never emphasis, and is left
// alone, so snake_case reads as it does.
func TestWriteMarkdown_EscapesMarkup(t *testing.T) {
	tbl := New(Column{Header: "v"})
	for _, c := range []string{`\`, "`", "*", "_", "[", "]", "<", ">", "|", "~", "&", "$"} {
		tbl.MustAppend("a " + c + " b")
	}
	tbl.MustAppend("snake_case_name")
	tbl.MustAppend("1.5 -2 2024-01-02 a/b (x) #1 !")
	got := markdownOf(t, tbl, MarkdownStyle{})
	for _, c := range []string{`\`, "`", "*", "_", "[", "]", "<", ">", "|", "~", "&", "$"} {
		if !strings.Contains(got, "a \\"+c+" b") {
			t.Errorf("%q not escaped in:\n%s", c, got)
		}
	}
	for _, want := range []string{"snake_case_name", "1.5 -2 2024-01-02 a/b (x) #1 !"} {
		if !strings.Contains(got, want) {
			t.Errorf("%q escaped where nothing needed it:\n%s", want, got)
		}
	}
}

// With Markup, a cell is written as the caller's own Markdown -- a link
// stays a link -- but a pipe is still escaped, as it would end the cell;
// one the caller escaped already is left, since a second backslash would
// escape the first and let the pipe end the cell after all.
func TestWriteMarkdown_Markup(t *testing.T) {
	tbl := New(Column{Header: "link"})
	tbl.MustAppend("[docs](https://example.com) `a|b`")
	tbl.MustAppend(`a\|b`)
	got := markdownOf(t, tbl, MarkdownStyle{Markup: true})
	for _, want := range []string{"[docs](https://example.com) `a\\|b`", `| a\|b `} {
		if !strings.Contains(got, want) {
			t.Errorf("no %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, `\\|`) {
		t.Errorf("an escaped pipe escaped again:\n%s", got)
	}
}

// The row of dashes says each column's alignment, and is three dashes at
// the least; MaxWidth does not cut a cell, as a renderer wraps it; Format
// applies.
func TestWriteMarkdown_AlignmentMaxWidthFormat(t *testing.T) {
	tbl := New(
		Column{Header: "l"},
		Column{Header: "r", Align: Right, Format: "%.1f"},
		Column{Header: "c", Align: Center, MaxWidth: 3},
	)
	tbl.MustAppend("x", 2.25, "longer than three")
	check(t, markdownOf(t, tbl, MarkdownStyle{}), strings.Join([]string{
		"| l   |   r |         c         |",
		"| --- | --: | :---------------: |",
		"| x   | 2.2 | longer than three |",
		"",
	}, "\n"))
}

// A table with no rows writes nothing, as Render does: a heading over
// nothing reads as data that failed to load. Nor does one with no columns.
func TestWriteMarkdown_NothingToWrite(t *testing.T) {
	if got := markdownOf(t, New(Column{Header: "a"}), MarkdownStyle{}); got != "" {
		t.Errorf("no rows: %q", got)
	}
	if got := markdownOf(t, New(), MarkdownStyle{}); got != "" {
		t.Errorf("no columns: %q", got)
	}
}

func TestWriteMarkdown_PropagatesWriteErrors(t *testing.T) {
	for _, after := range []int{0, 1, 2} {
		if err := sample(t).WriteMarkdown(&failWriter{n: after}, MarkdownStyle{}); !errors.Is(err, errWrite) {
			t.Errorf("failing after %d writes: %v, want %v", after, err, errWrite)
		}
	}
}

// FuzzWriteMarkdown writes a cell a stranger may have written, as a
// heading and in both columns of a table of each alignment, as text and as
// the caller's markup, and holds what
// TestWriteMarkdown_CellsNeverBreakTheTable holds: a line a row, a
// cell-ending pipe a column and one more, every line as wide as the first,
// and no control character.
func FuzzWriteMarkdown(f *testing.F) {
	for _, s := range []string{"a|b", `a\|b`, `\\|`, "x\ny", "*_`[]<>~&$", "日本語", "👨‍👩‍👧", "\x1b[31m", "a\\", "_snake_", ""} {
		f.Add(s)
	}
	f.Fuzz(checkMarkdown)
}

// TestFuzzCorpus runs checkMarkdown on every input a search kept, in
// testdata/corpus: make fuzz-replay.
func TestFuzzCorpus(t *testing.T) {
	for name, s := range fuzzcorpus.Strings(t, filepath.Join("testdata", "corpus", "FuzzWriteMarkdown")) {
		t.Run(name, func(t *testing.T) { checkMarkdown(t, s) })
	}
}

// checkMarkdown is FuzzWriteMarkdown's check of one cell.
func checkMarkdown(t *testing.T, s string) {
	for _, align := range []Align{Left, Right, Center} {
		for _, style := range []MarkdownStyle{{}, {Markup: true}} {
			tbl := New(Column{Header: s, Align: align}, Column{Header: "b"})
			tbl.MustAppend(s, "x")
			tbl.MustAppend("y", s)
			var w strings.Builder
			if err := tbl.WriteMarkdown(&w, style); err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSuffix(w.String(), "\n"), "\n")
			if len(lines) != 4 {
				t.Fatalf("%d lines for two rows:\n%s", len(lines), w.String())
			}
			for _, l := range lines {
				if n := pipes(l); n != 3 {
					t.Fatalf("%d cell-ending pipes, want 3: %q", n, l)
				}
				if tbl.width(l) != tbl.width(lines[0]) {
					t.Fatalf("lines of different widths:\n%s", w.String())
				}
				if strings.ContainsFunc(l, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
					t.Fatalf("a control character in %q", l)
				}
			}
		}
	}
}

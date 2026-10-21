package table

import (
	"io"
	"strings"
	"unicode"
)

// MarkdownStyle controls Markdown output. The zero value writes every cell
// and heading as text, which is what a table of someone else's data needs.
type MarkdownStyle struct {
	// Markup writes cells and headings as Markdown, for a caller that puts
	// a link or `code` in a cell on purpose. They are written as they are,
	// but for a pipe, which would end the cell, and a line break, which
	// would end the table: those are written as WriteMarkdown always writes
	// them. Never set it for a cell someone else wrote: see WriteMarkdown.
	Markup bool
}

// WriteMarkdown writes the table as a GitHub-flavoured Markdown table:
// a heading row, a row of dashes saying how each column is aligned, and a
// row for each of the table's rows, between pipes.
//
// It is the text table's rules for a reader, in a document a renderer
// draws, and differs from Render only where the two differ:
//
//   - Align sets each column's alignment in the rendered table, and pads
//     the cells so that the source lines up as well, measured as Render
//     measures, with Table.Width.
//   - MaxWidth has no effect. A renderer wraps a long cell rather than
//     running it off the edge, so cutting it would lose text for nothing.
//     Column.Format applies, as everywhere.
//   - Separators are skipped: a Markdown table has no rule between rows.
//   - A line break in a cell or a heading is written as <br>, which GitHub
//     and most renderers draw as one, since a newline would end the table;
//     trailing ones are dropped, as Style.Multiline drops them.
//
// A table with no rows writes nothing at all, as Render does and for the
// same reason: a heading over nothing reads as data that failed to load.
//
// A cell is often someone else's data, and in Markdown its text is markup:
// "*", "_" and "~" in a name would set the rest in italics or strike it out,
// "[x](url)" is a link, "<img>" is HTML, and "|" ends the cell. So each of
//
//	\ ` * _ [ ] < > | ~ & $
//
// in a cell or a heading is written escaped with a backslash, which every
// CommonMark renderer reads as the character itself -- but for an underscore
// between letters or digits, which is never emphasis, so snake_case stays as
// it reads. Other punctuation is left alone, so a number, a date or a path
// reads in the source as it will be rendered. What no escape can stop is
// left as the renderer has it: a bare URL becomes a link on GitHub, and
// "@name" and "#12" mention a person and an issue there. A control
// character, a format character and a byte that is not UTF-8 are written as
// U+FFFD, as Render writes them.
func (t *Table) WriteMarkdown(w io.Writer, style MarkdownStyle) error {
	if len(t.Columns) == 0 || t.Rows() == 0 {
		return nil
	}
	cell := func(v string) string {
		v = strings.TrimRight(strings.ReplaceAll(v, "\r\n", "\n"), "\n")
		lines := strings.Split(v, "\n")
		for i, l := range lines {
			lines[i] = markdownText(printable(l), style.Markup)
		}
		return strings.Join(lines, "<br>")
	}

	widths := make([]int, len(t.Columns))
	headings := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		headings[i] = cell(c.Header)
		// A row of dashes is three at the least; one with a colon at each
		// end, for a centred column, has a dash between them.
		widths[i] = max(t.width(headings[i]), 3)
	}
	var rows [][]string
	for _, e := range t.entries {
		if e.rule {
			continue
		}
		row := make([]string, len(t.Columns))
		for i := range t.Columns {
			if i < len(e.cells) {
				row[i] = cell(t.format(t.Columns[i], e.cells[i]))
			}
			widths[i] = max(widths[i], t.width(row[i]))
		}
		rows = append(rows, row)
	}

	lay := &layout{widths: widths, align: make([]Align, len(t.Columns)), widthOf: t.width}
	for i, c := range t.Columns {
		lay.align[i] = c.Align
	}
	line := func(cells []string) string {
		var b strings.Builder
		b.WriteString("|")
		for i, c := range cells {
			b.WriteString(" ")
			b.WriteString(lay.pad(c, widths[i], i))
			b.WriteString(" |")
		}
		return b.String()
	}
	bw := &errWriter{w: w}
	bw.line(line(headings))
	dashes := make([]string, len(t.Columns))
	for i, width := range widths {
		switch lay.align[i] {
		case Right:
			dashes[i] = strings.Repeat("-", width-1) + ":"
		case Center:
			dashes[i] = ":" + strings.Repeat("-", width-2) + ":"
		default:
			dashes[i] = strings.Repeat("-", width)
		}
	}
	bw.line(line(dashes))
	for _, row := range rows {
		bw.line(line(row))
	}
	return bw.err
}

// markdownText is s written for a Markdown table's cell: each character
// that would be markup escaped, or with markup, the pipe alone, which
// would end the cell either way -- and not one escaped already, after an
// odd run of backslashes, where a second backslash would escape the first
// and leave the pipe to end the cell.
func markdownText(s string, markup bool) string {
	special := `\` + "`" + `*_[]<>|~&$`
	if markup {
		special = "|"
	}
	if !strings.ContainsAny(s, special) {
		return s
	}
	var b strings.Builder
	slashes := 0 // the backslashes just before r, in markup
	rs := []rune(s)
	for i, r := range rs {
		escape := strings.ContainsRune(special, r) && !(markup && slashes%2 == 1)
		// An underscore between letters or digits is never emphasis --
		// CommonMark's rule for "_", not for "*" -- so a snake_case name
		// is left as it reads.
		if r == '_' && i > 0 && i < len(rs)-1 && word(rs[i-1]) && word(rs[i+1]) {
			escape = false
		}
		if escape {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
		if r == '\\' {
			slashes++
		} else {
			slashes = 0
		}
	}
	return b.String()
}

// word is a letter or a digit, as CommonMark counts one either side of an
// underscore.
func word(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

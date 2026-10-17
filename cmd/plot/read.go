package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
)

// table is a file's cells as text: its columns' names, and its rows, each
// as long as the longest -- a short row's missing cells are empty, which a
// column reads as no value.
type table struct {
	names []string
	rows  [][]string
}

// delimiters are the delimiters sniffDelimiter tells apart, in the order it
// prefers them where two fit equally: a tab, which nobody types into a
// value, before a comma, which a value may hold quoted.
var delimiters = []rune{'\t', ',', ';', '|'}

// sniffDelimiter is the delimiter of data, by its first lines: the one of
// delimiters each line holds the same number of, outside quotes -- the
// most of them, where several do. A file none fits is one column, and
// reads the same with any delimiter; a comma is as good as another.
//
// A count that differs from line to line rules a delimiter out: a comma
// file's lines each hold the same commas, where the commas inside a tab
// file's values vary with the values.
func sniffDelimiter(data []byte) rune {
	var lines []string
	for line := range strings.SplitSeq(string(data), "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" {
			lines = append(lines, line)
		}
		if len(lines) == 20 {
			break
		}
	}
	best, bestN := ',', 0
	for _, d := range delimiters {
		n := -1
		for _, l := range lines {
			c := countOutsideQuotes(l, d)
			if n >= 0 && c != n {
				n = 0
				break
			}
			n = c
		}
		if n > bestN {
			best, bestN = d, n
		}
	}
	return best
}

// countOutsideQuotes is how many times d appears in line outside
// double-quoted values.
func countOutsideQuotes(line string, d rune) int {
	n, quoted := 0, false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case r == d && !quoted:
			n++
		}
	}
	return n
}

// parseDelimiter is the delimiter -delimiter names: "auto", which is zero,
// for sniffDelimiter to find; "tab" or "\t"; or the one character.
func parseDelimiter(s string) (rune, error) {
	switch s {
	case "auto":
		return 0, nil
	case "tab", `\t`, "\t":
		return '\t', nil
	}
	if r := []rune(s); len(r) == 1 && r[0] != '"' && r[0] != '\n' && r[0] != '\r' {
		return r[0], nil
	}
	return 0, fmt.Errorf("-delimiter %q: use auto, tab or a single character such as , ; or |", s)
}

// headerMode is -header: whether the first row names the columns.
type headerMode string

const (
	headerAuto headerMode = "auto"
	headerYes  headerMode = "yes"
	headerNo   headerMode = "no"
)

// readTable reads delimited text from r -- delimited by delim, or by what
// sniffDelimiter finds where it is zero -- with values in double quotes
// where they hold the delimiter, a quote or a line break, as RFC 4180 has
// them. header says whether the first row names the columns; for auto,
// hasHeader decides. Columns without names are called "column 1",
// "column 2" and on.
//
// The reader forgives what spreadsheets and hand-written files do: rows of
// different lengths, a quote inside an unquoted value, spaces after a
// delimiter, and the byte-order mark Excel begins a UTF-8 file with.
func readTable(r io.Reader, delim rune, header headerMode, dates dateOrder) (*table, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if delim == 0 {
		delim = sniffDelimiter(data)
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = delim
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	cr.TrimLeadingSpace = true
	records, err := cr.ReadAll()
	if err != nil {
		var pe *csv.ParseError
		if errors.As(err, &pe) {
			return nil, fmt.Errorf("line %d: %w", pe.Line, pe.Err)
		}
		return nil, err
	}
	if len(records) == 0 {
		return nil, errors.New("no rows to plot")
	}
	width := 0
	for _, rec := range records {
		width = max(width, len(rec))
	}
	for i, rec := range records {
		for j := range rec {
			rec[j] = strings.TrimSpace(rec[j])
		}
		for len(rec) < width {
			rec = append(rec, "")
		}
		records[i] = rec
	}
	t := &table{rows: records}
	if header == headerYes || header == headerAuto && hasHeader(records, dates) {
		t.names, t.rows = records[0], records[1:]
	}
	for j := range width {
		if j >= len(t.names) {
			t.names = append(t.names, "")
		}
		if t.names[j] == "" {
			t.names[j] = fmt.Sprintf("column %d", j+1)
		}
	}
	return t, nil
}

// hasHeader says the first of records names the columns: some column's
// first cell is text where the cells beneath it are numbers or dates. A
// file whose every column is text has no way to tell, and is taken to have
// one too -- the names cost a row of text, which could not be plotted
// anyway, where taking a header for data would plot it.
func hasHeader(records [][]string, dates dateOrder) bool {
	if len(records) < 2 {
		return false
	}
	allText := true
	for j, first := range records[0] {
		if missing(first) {
			continue
		}
		if _, ok := parseNumber(first); ok {
			allText = false
			continue
		}
		if _, ok := parseDateAny(first, dates); ok {
			allText = false
			continue
		}
		cells := make([]string, 0, len(records)-1)
		for _, rec := range records[1:] {
			cells = append(cells, rec[j])
		}
		if c := classify(cells, dates); c.kind != textColumn {
			return true
		}
	}
	return allText
}

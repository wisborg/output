package main

import (
	"bytes"
	"cmp"
	"encoding/csv"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/wisborg/output/internal/cli"
)

// table is a file's cells as text: its columns' names, and its rows, held
// one of two ways. A CSV's rows are rows: a cell a column, a short row's
// missing cells empty. A JSON Lines file's are sparse: each row only the
// cells its line has a key for, by column, as a log keyed by user names
// one column of a thousand a line, and held a cell a column a line, empty
// all but one, it took 2.9 GB for 5 MB. Missing cells are empty, which a
// column reads as no value. Read them through rowCount, cell and
// eachColumn, which answer the same of both.
type table struct {
	names  []string
	rows   [][]string
	sparse [][]sparseCell // JSON Lines' rows, in place of rows; each by column
	notes  []string       // what reading it guessed or left out, for plot to say
	// A JSON Lines file's strings are kept as written, and each column's
	// are read with readMark[j]: by what its own strings say of the mark,
	// seen[j], until settleMarks decides it for the columns plotted.
	seen     []marks
	readMark []decimalMark
	// dropped are the keys a JSON Lines file has past maxJSONColumns,
	// which are no column, so that asking for one can say so, and the
	// note can count them.
	//
	// It holds every such key, without a limit: a known limit, kept. A log
	// keyed by user with millions of users holds millions of names. The
	// whole file is in memory already, and the names are no larger than
	// it, so this at most doubles what reading takes; a bounded set would
	// miscount the keys left out, and say of a key past its bound that
	// there is no such column, or the file would be read twice to answer.
	dropped map[string]bool
}

// nameColumns names each of names that is empty "column N", N its place
// from 1 -- or, where a column has that name already, as a JSON key may,
// "column N, unnamed", and "column N, unnamed 2" and on where that is
// taken too: two columns named alike, the second could not be asked for
// by name.
func nameColumns(names []string) {
	taken := map[string]bool{}
	for _, n := range names {
		taken[n] = true
	}
	for j, n := range names {
		if n != "" {
			continue
		}
		name := fmt.Sprintf("column %d", j+1)
		for k := 1; taken[name]; k++ {
			name = fmt.Sprintf("column %d, unnamed", j+1)
			if k > 1 {
				name += " " + strconv.Itoa(k)
			}
		}
		names[j], taken[name] = name, true
	}
}

// sparseCell is one cell of a sparse row: its column, its text, and
// whether JSON wrote it as a string, which alone a decimal comma rewrites.
// The column is an int32, which maxJSONColumns is far inside, so that a
// cell is 24 bytes rather than 32: most of a log's memory is its cells.
type sparseCell struct {
	col   int32
	str   bool
	value string
}

// rowCount is how many rows t has.
func (t *table) rowCount() int {
	if t.sparse != nil {
		return len(t.sparse)
	}
	return len(t.rows)
}

// cell is row i's cell in column j: empty past a row's end, or where a
// sparse row has none.
func (t *table) cell(i, j int) string {
	if t.sparse != nil {
		row := t.sparse[i]
		if k, ok := slices.BinarySearchFunc(row, int32(j), func(c sparseCell, j int32) int { return cmp.Compare(c.col, j) }); ok {
			return t.text(row[k])
		}
		return ""
	}
	if row := t.rows[i]; j < len(row) {
		return row[j]
	}
	return ""
}

// text is c as a column reads it: a string written with a decimal comma
// rewritten with a point where its column is read with one, and JSON's own
// numbers, which have a point, as they are.
func (t *table) text(c sparseCell) string {
	if c.str && t.readMark[c.col] == decimalComma {
		return commaToPoint(c.value)
	}
	return c.value
}

// settleMarks decides the decimal mark of a JSON Lines file's strings
// from the columns used alone, and reads each of them with it. mark is
// --decimal, which where given has decided it already.
//
// A column's kind and counts, from classify, stand: one read now with
// another mark than its own strings' had strings that showed none, so each
// is a number read either way or neither, and only its value changes,
// which cell gives.
//
// From the columns used alone, not the file: a log's strings are fields
// of their own, ids, tags, versions, and a "3,4" among a line's ids made
// every string a decimal comma's, its "1.234" 1234 without a word, while
// a "2.5" in one field and a "1,2" in another refused a log plotting
// neither. Among the columns used, a point in one and a comma in another
// -- or both in one -- are refused rather than guessed between: "1.234"
// is 1.234 or 1234 by which, and a chart a thousand times out is worse
// than none. Where they show no mark, it is a point, as JSON's own
// numbers have, and a string that reads as two numbers is said.
func (t *table) settleMarks(used []int, mark decimalMark) (note string, err error) {
	if t.sparse == nil || mark != decimalAuto {
		return "", nil
	}
	m := marks{unit: "line", names: t.names}
	for _, j := range used {
		m = m.add(t.seen[j])
	}
	decided, note, err := m.decide(decimalPoint)
	if err != nil {
		return "", err
	}
	for _, j := range used {
		t.readMark[j] = decided
	}
	return note, nil
}

// markNotes are notes of a JSON Lines file's columns whose strings are
// written with both a decimal point and a comma: --list reads each with a
// point, and says so, as plotted it would be refused. Where --decimal says
// the mark, there are none: t.seen is not kept, and a note that the
// column waits for --decimal would be untrue. Each names its column once,
// at its head, and its cells by their lines alone.
func (t *table) markNotes() []string {
	var notes []string
	for j, m := range t.seen {
		if m.point != nil && m.comma != nil {
			notes = append(notes, fmt.Sprintf("%q: numbers written both ways, %q %s with a decimal point and %q %s with a comma; listed with a point, and plotted only once --decimal says which",
				t.names[j], m.point.cell, m.where(m.point), m.comma.cell, m.where(m.comma)))
		}
	}
	return notes
}

// eachColumn calls f with each column and the cells of it that are not
// empty, in the rows' order: what classify needs, which an empty cell
// tells nothing. A sparse table's columns are gathered in one pass over
// the cells it holds -- asked a cell at a time, a thousand columns of
// 200,000 rows were 200 million lookups, of cells nearly all empty -- and
// a CSV's a column at a time, holding no more than one at once.
func (t *table) eachColumn(f func(j int, cells []string)) {
	if t.sparse != nil {
		// Counted first, so each column is made its size: grown by
		// append, the slack took as much again as the cells.
		counts := make([]int, len(t.names))
		for _, row := range t.sparse {
			for _, c := range row {
				if c.value != "" {
					counts[c.col]++
				}
			}
		}
		cols := make([][]string, len(t.names))
		for j, n := range counts {
			cols[j] = make([]string, 0, n)
		}
		for _, row := range t.sparse {
			for _, c := range row {
				if c.value != "" {
					cols[c.col] = append(cols[c.col], t.text(c))
				}
			}
		}
		for j, cells := range cols {
			f(j, cells)
		}
		return
	}
	for j := range t.names {
		f(j, t.column(j))
	}
}

// column is column j's cells that are not empty, in the rows' order.
func (t *table) column(j int) []string {
	var cells []string
	for i := range t.rowCount() {
		if c := t.cell(i, j); c != "" {
			cells = append(cells, c)
		}
	}
	return cells
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
//
// Where semicolons fit too, they win over commas however many commas there
// are. A file delimited by semicolons is one written where a comma is the
// decimal point, and with no header, "1,5;2,5" holds two commas a line to
// one semicolon -- read as commas, it was three columns of fragments. A
// comma file whose every line holds the same semicolons is much the rarer.
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
	semicolons := false
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
		if d == ';' && n > 0 {
			semicolons = true
		}
	}
	if best == ',' && semicolons {
		return ';'
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

// parseDelimiter is the delimiter --delimiter names: "auto", which is zero,
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
	return 0, cli.Usagef("--delimiter %q: use auto, tab or a single character such as , ; or |", s)
}

// headerMode is --header: whether the first row names the columns.
type headerMode string

const (
	headerAuto headerMode = "auto"
	headerYes  headerMode = "yes"
	headerNo   headerMode = "no"
)

// csvDecimal is the decimal mark a CSV delimited by delim is guessed to
// have where nothing in it says: a comma where it is delimited by
// semicolons, the delimiter of the places that write one, and a point
// otherwise.
func csvDecimal(delim rune) decimalMark {
	if delim == ';' {
		return decimalComma
	}
	return decimalPoint
}

// readTable reads delimited text from data -- delimited by delim, or by what
// sniffDelimiter finds where it is zero -- with values in double quotes
// where they hold the delimiter, a quote or a line break, as RFC 4180 has
// them. header says whether the first row names the columns; for auto,
// hasHeader decides. Columns without names are called "column 1",
// "column 2" and on.
//
// The reader forgives what spreadsheets and hand-written files do: rows of
// different lengths, a quote inside an unquoted value, spaces after a
// delimiter. (The byte-order mark Excel begins a UTF-8 file with is run's
// to strip, before it tells CSV from JSON Lines.) A file written with a
// decimal comma -- "1,5", "1.234,5" -- is read as one, as decideDecimal
// decides from mark and the file's own cells, and its numbers are
// rewritten with points.
func readTable(data []byte, delim rune, header headerMode, dates dateOrder, mark decimalMark) (*table, error) {
	if delim == 0 {
		delim = sniffDelimiter(data)
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.Comma = delim
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	// Not for a tab: encoding/csv trims a leading tab even where the tab is
	// the delimiter, so a tab file's empty cell -- two tabs in a row --
	// vanished and every value after it moved a column left, to be plotted
	// as the column before it. For a space it is wanted: a file aligned with
	// runs of spaces has one delimiter between its columns, not one a space
	// and empty cells between. Each cell is trimmed below anyway; this is
	// for a quoted value after ", ", and for those runs.
	cr.TrimLeadingSpace = delim != '\t'
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
	decided, note, err := decideDecimal(records, csvDecimal(delim), mark)
	if err != nil {
		return nil, err
	}
	if decided == decimalComma {
		for _, rec := range records {
			for j := range rec {
				rec[j] = commaToPoint(rec[j])
			}
		}
	}
	t := &table{rows: records}
	if note != "" {
		t.notes = append(t.notes, note)
	}
	if header == headerYes || header == headerAuto && hasHeader(records, dates) {
		t.names, t.rows = records[0], records[1:]
	}
	for len(t.names) < width {
		t.names = append(t.names, "")
	}
	nameColumns(t.names)
	return t, nil
}

// hasHeader says the first of records names the columns: some column's
// first cell is text where the cells beneath it are numbers or dates. A
// file whose every column is text has no way to tell, and is taken to have
// one too -- the names cost a row of text, which could not be plotted
// anyway, where taking a header for data would plot it.
//
// A file of one row is a header with nothing beneath it where every cell
// of the row is text -- "x,y" -- and data otherwise: names alone are no
// readings, and taken for a row of them, plot said no column held numbers
// rather than that the file had no rows.
func hasHeader(records [][]string, dates dateOrder) bool {
	if len(records) == 1 {
		for _, cell := range records[0] {
			if _, ok := parseNumber(cell); ok {
				return false
			}
			if _, ok := parseDateAny(cell, dates); ok {
				return false
			}
		}
		return true
	}
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

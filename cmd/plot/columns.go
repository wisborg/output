package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// missingWords are the cells that say a value is missing, as files spell
// it -- besides an empty cell, which says it by saying nothing.
var missingWords = map[string]bool{"na": true, "n/a": true, "nan": true, "null": true, "none": true, "-": true, "--": true, "?": true}

// missing says cell is no value at all: empty, or one of missingWords in
// any case. It is a gap in the line, never a zero.
func missing(cell string) bool {
	return cell == "" || missingWords[strings.ToLower(cell)]
}

// grouped is a number written with commas between its thousands,
// "1,234,567.89", as a spreadsheet exports a formatted cell where the
// decimal mark is a point. Its first group is not 0: "0,250" is no
// thousands.
var grouped = regexp.MustCompile(`^[+-]?[1-9]\d{0,2}(,\d{3})+(\.\d+)?$`)

// commaNumber is a number written with a decimal comma, "1,5", "0,25" or
// "-1.234.567,89": its thousands, if marked at all, marked by points.
var commaNumber = regexp.MustCompile(`^[+-]?(\d+|[1-9]\d{0,2}(\.\d{3})+)(,\d+)?$`)

// decimalMark is --decimal: how a file writes the fraction of a number,
// which a cell alone often cannot say. "1,234" is twelve hundred and
// thirty-four where a point marks the fraction and commas the thousands, as
// in English, and one point two three four where a comma marks it and
// points the thousands, as in most of Europe; "1.234" the other way round.
type decimalMark string

const (
	decimalAuto  decimalMark = "auto"
	decimalPoint decimalMark = "point"
	decimalComma decimalMark = "comma"
)

// asPoint is cell read as a number written with a decimal point: "1.5",
// "1,234,567.8".
func asPoint(cell string) (float64, bool) { return parseNumber(cell) }

// asComma is cell read as a number written with a decimal comma: "1,5",
// "1.234,5"; or one that has neither mark, "12" or "1e6".
func asComma(cell string) (float64, bool) {
	switch {
	case commaNumber.MatchString(cell):
		cell = strings.Replace(strings.ReplaceAll(cell, ".", ""), ",", ".", 1)
	case strings.ContainsAny(cell, ".,"):
		return 0, false
	}
	return parseNumber(cell)
}

// commaToPoint is cell, a number written with a decimal comma, written with
// a point -- "1.234,5" is "1234.5" -- or cell as it was, where it is no
// number written so.
func commaToPoint(cell string) string {
	if !commaNumber.MatchString(cell) {
		return cell
	}
	return strings.Replace(strings.ReplaceAll(cell, ".", ""), ",", ".", 1)
}

// sighting is a cell that says something of the file's decimal mark, and
// the row it is on, counting from 1, to name it by.
type sighting struct {
	cell string
	row  int
}

// decideDecimal is the decimal mark the file of records is written with:
// mark where the caller gave one; else the mark its cells show; else a
// comma where the file is delimited by semicolons -- the delimiter of the
// places that write one -- and a point otherwise.
//
// It is decided for the file, not for each column or each cell. One program
// in one place wrote it, and read cell by cell, "1,234,567" was thousands
// while "1,234" beside it was one point two three four. A cell shows the
// mark where it reads as a number only one way: "1,5", "0,25", "1.234,5",
// "1.234.567" only with a comma; "1.5", "1,234,567", "1,234.5" only with a
// point. Cells that show both marks are a file at odds with itself, which
// is refused rather than read half and half. Where nothing showed it and a
// cell read as two different numbers -- "1,234" -- the guess is said in
// note, with the flag that reads it the other way.
func decideDecimal(records [][]string, delim rune, mark decimalMark) (decided decimalMark, note string, err error) {
	if mark != decimalAuto {
		return mark, "", nil
	}
	var point, comma, either *sighting
	for i, rec := range records {
		for _, cell := range rec {
			if missing(cell) {
				continue
			}
			p, pok := asPoint(cell)
			c, cok := asComma(cell)
			seen := &sighting{cell, i + 1}
			switch {
			case pok && !cok && point == nil:
				point = seen
			case cok && !pok && comma == nil:
				comma = seen
			case pok && cok && p != c && either == nil:
				either = seen
			}
		}
	}
	switch {
	case point != nil && comma != nil:
		return "", "", fmt.Errorf("the numbers are written both ways: %q on row %d with a decimal point, %q on row %d with a decimal comma; say --decimal point or --decimal comma",
			point.cell, point.row, comma.cell, comma.row)
	case point != nil:
		return decimalPoint, "", nil
	case comma != nil:
		return decimalComma, "", nil
	}
	decided, other := decimalPoint, decimalComma
	if delim == ';' {
		decided, other = decimalComma, decimalPoint
	}
	if either != nil {
		v, _ := asPoint(either.cell)
		if decided == decimalComma {
			v, _ = asComma(either.cell)
		}
		note = fmt.Sprintf("%q read as %s, with a decimal %s; --decimal %s reads it the other way",
			either.cell, strconv.FormatFloat(v, 'f', -1, 64), decided, other)
	}
	return decided, note, nil
}

// parseNumber is cell as a number: as Go reads a float -- "12", "-0.5",
// "1e6" -- or with commas between its thousands. Infinity and NaN are no
// number to draw, and a cell of either is not one. A file written with a
// decimal comma has been rewritten with points, by commaToPoint, before
// its cells are read.
func parseNumber(cell string) (float64, bool) {
	if grouped.MatchString(cell) {
		cell = strings.ReplaceAll(cell, ",", "")
	}
	v, err := strconv.ParseFloat(cell, 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// dateOrder is --dates: how a date written in numbers alone -- 03/04/2024,
// 03.04.2024 -- orders its day and month, which nothing in the date says.
type dateOrder string

const (
	datesAuto dateOrder = "auto"
	datesDMY  dateOrder = "dmy"
	datesMDY  dateOrder = "mdy"
)

// isoLayouts are the date layouts that say which part is which: ISO 8601's
// in its forms, with or without a time and a zone, and dates whose month is
// a word.
var isoLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05.999999999Z07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006-01",
	"2006/1/2 15:04:05",
	"2006/1/2 15:04",
	"2006/1/2",
	"2 Jan 2006 15:04:05",
	"2 Jan 2006 15:04",
	"2 Jan 2006",
	"2-Jan-2006",
	"2 January 2006",
	"Jan 2, 2006 15:04:05",
	"Jan 2, 2006",
	"January 2, 2006",
	"Jan 2006",
	"January 2006",
	time.RFC1123Z,
	time.RFC1123,
}

// numericLayouts are the dates written in numbers alone with the day first,
// for datesDMY; datesMDY swaps day and month in each.
var numericLayouts = []string{
	"2/1/2006 15:04:05", "2/1/2006 15:04", "2/1/2006",
	"2.1.2006 15:04:05", "2.1.2006 15:04", "2.1.2006",
	"2-1-2006 15:04:05", "2-1-2006 15:04", "2-1-2006",
}

// layoutsFor are the layouts a date is read by in order: isoLayouts, and
// numericLayouts with the day first or the month or, for auto, both.
func layoutsFor(order dateOrder) []string {
	ls := append([]string(nil), isoLayouts...)
	if order != datesMDY {
		ls = append(ls, numericLayouts...)
	}
	if order != datesDMY {
		for _, l := range numericLayouts {
			ls = append(ls, monthFirst(l))
		}
	}
	return ls
}

// monthFirst is a numericLayouts layout with its day and month swapped.
func monthFirst(layout string) string {
	date, clock, _ := strings.Cut(layout, " ")
	sep := date[1:2]
	date = "1" + sep + "2" + sep + "2006"
	if clock != "" {
		return date + " " + clock
	}
	return date
}

// parseDate is cell read by layout as a moment on a Date axis: the clock
// on the wall as written, in seconds since 1970, whatever zone the cell
// names. A file is plotted as it reads, and a chart of a day in New York
// whose axis said 14:00 for a reading written 09:00 would read wrong to
// everyone who wrote it.
func parseDate(cell, layout string) (float64, bool) {
	t, err := time.Parse(layout, cell)
	if err != nil {
		return 0, false
	}
	wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.UTC)
	return float64(wall.Unix()) + float64(t.Nanosecond())/1e9, true
}

// parseDateAny is cell read by the first of order's layouts that reads it.
func parseDateAny(cell string, order dateOrder) (float64, bool) {
	for _, l := range layoutsFor(order) {
		if v, ok := parseDate(cell, l); ok {
			return v, true
		}
	}
	return 0, false
}

// columnKind is what a column holds.
type columnKind int

const (
	textColumn columnKind = iota
	numberColumn
	dateColumn
)

func (k columnKind) String() string {
	return [...]string{"text", "number", "date"}[k]
}

// column is what classify finds a column to hold: its kind, the layout its
// dates are written in, how many of its cells hold a value and how many of
// those are not of its kind -- a footnote in a column of numbers, "n.d.".
type column struct {
	kind          columnKind
	layout        string
	values, wrong int
	// ambiguous says the column's dates are written in numbers alone
	// and read as well with the day first as the month: --dates must say.
	ambiguous bool
}

// classify is what cells hold. A column most of whose values are numbers
// is a column of numbers, and the rest are gaps; most of whose values are
// dates written one way, a column of dates. Anything else is text, which
// is not plotted.
//
// Most, and not all, because a file someone typed holds a "n.d." or a
// "see note 3" among the readings, and refusing the column for one of them
// would refuse the file. Each such cell is counted, so a reader is told
// how many were left out rather than finding a gap they cannot explain.
//
// A column of dates is read by one layout -- the one that reads the most of
// them -- since mixing two would read the same cell two ways. Where a date
// reads with the day first exactly as often as with the month, nothing in
// the column says which it is, and it is ambiguous.
func classify(cells []string, order dateOrder) column {
	var vals []string
	for _, c := range cells {
		if !missing(c) {
			vals = append(vals, c)
		}
	}
	col := column{values: len(vals)}
	if len(vals) == 0 {
		return col
	}
	numbers := 0
	for _, v := range vals {
		if _, ok := parseNumber(v); ok {
			numbers++
		}
	}
	if 2*numbers > len(vals) {
		col.kind, col.wrong = numberColumn, len(vals)-numbers
		return col
	}
	// The layouts worth counting are those that read one of the first
	// values: trying every layout on every cell would cost thirty parses a
	// cell for nothing.
	candidates := map[string]bool{}
	for _, v := range vals[:min(len(vals), 5)] {
		for _, l := range layoutsFor(order) {
			if _, ok := parseDate(v, l); ok {
				candidates[l] = true
			}
		}
	}
	best, bestN, tied := "", 0, false
	for _, l := range layoutsFor(order) {
		if !candidates[l] {
			continue
		}
		n := 0
		for _, v := range vals {
			if _, ok := parseDate(v, l); ok {
				n++
			}
		}
		switch {
		case n > bestN:
			best, bestN, tied = l, n, false
		case n == bestN && n > 0 && dayFirstTwin(l) == best:
			tied = true
		}
	}
	if 2*bestN > len(vals) {
		col.kind, col.layout, col.wrong, col.ambiguous = dateColumn, best, len(vals)-bestN, tied
	}
	return col
}

// dayFirstTwin is the day-first layout l is the month-first form of, and ""
// where l is no month-first layout: the layout l ties with when a column
// reads the same both ways.
func dayFirstTwin(l string) string {
	for _, d := range numericLayouts {
		if monthFirst(d) == l {
			return d
		}
	}
	return ""
}

// value is cell as a reading of a column of kind col: its number or its
// date, or that it has none.
func (col column) value(cell string) (float64, bool) {
	if missing(cell) {
		return 0, false
	}
	switch col.kind {
	case numberColumn:
		return parseNumber(cell)
	case dateColumn:
		return parseDate(cell, col.layout)
	}
	return 0, false
}

// findColumn is the column spec names: a column's name, exactly or in
// another case, or its number counting from 1. A name wins over a number,
// so a column called "2024" is found by its name.
func findColumn(names []string, spec string) (int, error) {
	spec = strings.TrimSpace(spec)
	for j, n := range names {
		if n == spec {
			return j, nil
		}
	}
	for j, n := range names {
		if strings.EqualFold(n, spec) {
			return j, nil
		}
	}
	if n, err := strconv.Atoi(spec); err == nil && n >= 1 && n <= len(names) {
		return n - 1, nil
	}
	return 0, fmt.Errorf("no column %q: the columns are %s", spec, quoteList(names))
}

// quoteList is names quoted and joined for a message.
func quoteList(names []string) string {
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = strconv.Quote(n)
	}
	return strings.Join(q, ", ")
}

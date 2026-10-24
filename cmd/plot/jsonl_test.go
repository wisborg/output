package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// A column for each key, in the order the keys are first met, whichever
// line first has them; a key a line lacks is an empty cell, and so is
// null. A nested object's keys are named by their path, an array's
// elements by their index, and a number is kept as written.
func TestReadJSONLines(t *testing.T) {
	tb, err := readJSONLines([]byte(`{"t":1,"v":1.50,"cpu":{"user":0.2},"load":[3,4]}
{"t":2,"v":null,"note":" hi ","ok":true}

{"t":3,"v":2e3,"cpu":{"user":0.3}}
`), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"t", "v", "cpu.user", "load[0]", "load[1]", "note", "ok"}; !reflect.DeepEqual(tb.names, want) {
		t.Errorf("names %q, want %q", tb.names, want)
	}
	want := [][]string{
		{"1", "1.50", "0.2", "3", "4", "", ""},
		{"2", "", "", "", "", "hi", "true"},
		{"3", "2e3", "0.3", "", "", "", ""},
	}
	if got := grid(tb); !reflect.DeepEqual(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	if n := len(tb.rows[2]); n != 3 {
		t.Errorf("the last row holds %d cells, want 3: as far as its last key, not padded to every column", n)
	}
	if len(tb.notes) != 0 {
		t.Errorf("notes %q, want none: a blank line is no row and no fault", tb.notes)
	}
}

// grid is tb's cells as cell reads them, every row as wide as the names.
func grid(tb *table) [][]string {
	g := make([][]string, len(tb.rows))
	for i := range tb.rows {
		for j := range tb.names {
			g[i] = append(g[i], tb.cell(i, j))
		}
	}
	return g
}

// A key written twice in one object is one column, with its last value,
// as encoding/json reads it, and so is a dotted key beside the nested path
// named alike; each loses a value, which is said. An empty key is a column
// numbered as a CSV's unnamed one is.
func TestReadJSONLinesKeys(t *testing.T) {
	tb, err := readJSONLines([]byte(`{"a":1,"":5,"a":2}`+"\n"+`{"a":3}`+"\n"+`{"b.c":1,"b":{"c":4}}`+"\n"), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "column 2", "b.c"}; !reflect.DeepEqual(tb.names, want) {
		t.Errorf("names %q, want %q", tb.names, want)
	}
	if want := [][]string{{"2", "5", ""}, {"3", "", ""}, {"", "", "4"}}; !reflect.DeepEqual(grid(tb), want) {
		t.Errorf("rows %q, want %q", grid(tb), want)
	}
	if want := []string{"2 lines giving a column two values, the last kept, the first line 1"}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}
	tb, err = readJSONLines([]byte(`{"x":1,"":{"x":2}}`), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"line 1: a column given two values, the last kept"}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("an object under the empty key: notes %q, want %q", tb.notes, want)
	}
}

// A log keyed by user has a key a line; past maxJSONColumns the keys are
// left out and said, and no row holds more cells than its own keys reach.
func TestReadJSONLinesHoldsItsColumns(t *testing.T) {
	var b strings.Builder
	const lines = 3 * maxJSONColumns
	for i := range lines {
		fmt.Fprintf(&b, `{"t":%d,"counts":{"user%d":1}}`+"\n", i, i)
	}
	tb, err := readJSONLines([]byte(b.String()), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.names) != maxJSONColumns || len(tb.rows) != lines {
		t.Errorf("%d columns and %d rows, want %d and %d", len(tb.names), len(tb.rows), maxJSONColumns, lines)
	}
	cells := 0
	for _, row := range tb.rows {
		cells += len(row)
	}
	// Row i reaches column i+1, 2 cells to maxJSONColumns, while there
	// are columns to make, and holds t's alone after.
	if want := (2+maxJSONColumns)*(maxJSONColumns-1)/2 + (lines - maxJSONColumns + 1); cells != want {
		t.Errorf("%d cells held, want %d", cells, want)
	}
	if want := []string{fmt.Sprintf("%d keys past the first %d left out, the first \"counts.user%d\"", lines-maxJSONColumns+1, maxJSONColumns, maxJSONColumns-1)}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}
}

// A line that is not one object -- cut short, text, an array, an object
// with more after it, one nested past maxJSONDepth -- is left out and said
// with the first of its numbers; a file with no object is refused, naming
// the first line.
func TestReadJSONLinesLeavesOutWhatIsNoObject(t *testing.T) {
	deep := strings.Repeat(`{"a":`, maxJSONDepth+1) + "1" + strings.Repeat("}", maxJSONDepth+1)
	tb, err := readJSONLines([]byte(`{"v":1}
{"v":2
starting up
[1,2]
{"v":3} {"v":4}
`+deep+`
{"v":5}`), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"1"}, {"5"}}; !reflect.DeepEqual(grid(tb), want) {
		t.Errorf("rows %q, want %q", grid(tb), want)
	}
	if want := []string{"5 lines not a JSON object, left out, the first line 2"}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}

	// As deep as maxJSONDepth is read.
	shallow := strings.Repeat(`{"a":`, maxJSONDepth) + "1" + strings.Repeat("}", maxJSONDepth)
	if tb, err := readJSONLines([]byte(shallow), decimalAuto); err != nil || len(tb.rows) != 1 || len(tb.notes) != 0 {
		t.Errorf("%d deep: %v, want one row", maxJSONDepth, err)
	}

	tb, err = readJSONLines([]byte("{\"v\":1}\n{\"v\"\n"), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"line 2: not a JSON object, left out"}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}

	if _, err := readJSONLines([]byte("\n{\"v\"\n"), decimalAuto); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("no object: %v, want an error naming line 2", err)
	}
	if _, err := readJSONLines([]byte("\n\n"), decimalAuto); err == nil {
		t.Error("blank lines alone: no error")
	}
}

// auto takes a file for JSON Lines where at least half its first lines
// are whole objects, whatever the first is -- a log read from its middle
// begins with the end of a line -- and anything else for CSV. JSON that is
// not JSON Lines is refused, saying jq -c makes it so where it would.
func TestSniffInput(t *testing.T) {
	for _, c := range []struct {
		name, data string
		want       inputMode
	}{
		{"objects", "{\"a\":1}\n{\"a\":2}\n", inputJSONL},
		{"after blank lines", "\n  \n{\"a\":1}\n", inputJSONL},
		{"first line cut short", "{\"a\":\n{\"a\":2}\n", inputJSONL},
		{"read from the middle", "s\":200}\n{\"a\":1}\n{\"a\":2}\n", inputJSONL},
		{"after a banner", "service starting\n{\"a\":1}\n{\"a\":2}\n", inputJSONL},
		{"one object in three", "x\ny\n{\"a\":1}\n", inputCSV},
		{"CSV", "a,b\n1,2\n", inputCSV},
		{"a brace no line closes", "{a},b\n1,2\n", inputCSV},
		{"brackets not JSON", "[a],b\n1,2\n", inputCSV},
		{"a brace and JSON after", "{\n\"a\": 1\n} x\n", inputCSV},
		{"NaN in most lines", "{\"v\":NaN}\n{\"v\":NaN}\n{\"v\":1}\n", inputJSONL},
		{"one bracketed line", "[a]\n", inputCSV},
	} {
		got, err := sniffInput([]byte(c.data))
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v, want %q", c.name, got, err, c.want)
		}
	}
	for _, c := range []struct{ name, data, want, not string }{
		{"an array a line", "[1,2]\n[3,4]\n[5,NaN]\n", "an array a line", "jq"},
		{"an array of objects", " [{\"a\":1},{\"a\":2}]\n", "jq -c '.[]'", ""},
		{"an array of numbers", "[1,2,3]", "one JSON array", "jq"},
		{"objects over lines", "{\n  \"a\": 1\n}\n{\n  \"a\": 2\n}\n", "jq -c .", ""},
	} {
		_, err := sniffInput([]byte(c.data))
		if err == nil || !strings.Contains(err.Error(), c.want) || c.not != "" && strings.Contains(err.Error(), c.not) {
			t.Errorf("%s: %v, want an error saying %q and not %q", c.name, err, c.want, c.not)
		}
	}
}

// An array's elements past maxJSONArray are left out and said, and what
// follows the array is still a column: a sample of readings took every
// column, and the latency after it was lost.
func TestReadJSONLinesCutsLongArrays(t *testing.T) {
	var b strings.Builder
	for i := range 3 {
		fmt.Fprintf(&b, `{"t":%d,"samples":[%s],"latency":%d}`+"\n", i, strings.Repeat("1,", 1200)+"1", i)
	}
	tb, err := readJSONLines([]byte(b.String()), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(tb.names); n != maxJSONArray+2 || tb.names[n-1] != "latency" || tb.cell(2, n-1) != "2" {
		t.Errorf("%d columns, the last %q holding %q: want %d, latency holding 2", n, tb.names[n-1], tb.cell(2, n-1), maxJSONArray+2)
	}
	if want := []string{fmt.Sprintf("3 lines with an array's elements past the first %d left out, the first line 1, \"samples\"", maxJSONArray)}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}
}

// NaN and Infinity alone are read as words; inside a string, or as part of
// a longer word, they are left as they are.
func TestAllowNonFinite(t *testing.T) {
	for in, want := range map[string]string{
		`{"a":NaN,"b":[Infinity,-Infinity]}`: `{"a":"NaN","b":["Infinity","-Infinity"]}`,
		`{"a":"NaN \" NaN","b":1}`:           `{"a":"NaN \" NaN","b":1}`,
		`{"NaN":1,"a":NaNx,"b":xNaN}`:        `{"NaN":1,"a":NaNx,"b":xNaN}`,
		`{"a":1}`:                            `{"a":1}`,
	} {
		if got := string(allowNonFinite([]byte(in))); got != want {
			t.Errorf("%s: %s, want %s", in, got, want)
		}
	}
}

// An empty name is "column N", and where a column already has that name,
// as a JSON key may, "column N, unnamed", so that each can be asked for.
func TestNameColumns(t *testing.T) {
	names := []string{"t", "", "column 2", ""}
	nameColumns(names)
	if want := []string{"t", "column 2, unnamed", "column 2", "column 4"}; !reflect.DeepEqual(names, want) {
		t.Errorf("%q, want %q", names, want)
	}
}

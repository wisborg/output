package main

import (
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
`))
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
	if !reflect.DeepEqual(tb.rows, want) {
		t.Errorf("rows %q, want %q", tb.rows, want)
	}
	if len(tb.notes) != 0 {
		t.Errorf("notes %q, want none: a blank line is no row and no fault", tb.notes)
	}
}

// A key written twice in one object is one column, with its last value,
// as encoding/json reads it; an empty key is a column numbered as a CSV's
// unnamed one is.
func TestReadJSONLinesKeys(t *testing.T) {
	tb, err := readJSONLines([]byte(`{"a":1,"":5,"a":2}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "column 2"}; !reflect.DeepEqual(tb.names, want) {
		t.Errorf("names %q, want %q", tb.names, want)
	}
	if want := [][]string{{"2", "5"}}; !reflect.DeepEqual(tb.rows, want) {
		t.Errorf("rows %q, want %q", tb.rows, want)
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
` + deep + `
{"v":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"1"}, {"5"}}; !reflect.DeepEqual(tb.rows, want) {
		t.Errorf("rows %q, want %q", tb.rows, want)
	}
	if want := []string{"5 lines not a JSON object, left out, the first line 2"}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}

	// As deep as maxJSONDepth is read.
	shallow := strings.Repeat(`{"a":`, maxJSONDepth) + "1" + strings.Repeat("}", maxJSONDepth)
	if tb, err := readJSONLines([]byte(shallow)); err != nil || len(tb.rows) != 1 || len(tb.notes) != 0 {
		t.Errorf("%d deep: %v, want one row", maxJSONDepth, err)
	}

	tb, err = readJSONLines([]byte("{\"v\":1}\n{\"v\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"line 2: not a JSON object, left out"}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}

	if _, err := readJSONLines([]byte("\n{\"v\"\n")); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("no object: %v, want an error naming line 2", err)
	}
	if _, err := readJSONLines([]byte("\n\n")); err == nil {
		t.Error("blank lines alone: no error")
	}
}

// auto takes a file beginning with an object, one of whose first lines
// reads as one, for JSON Lines, and anything else for CSV; a JSON array is
// refused, saying what plot reads instead.
func TestSniffInput(t *testing.T) {
	for _, c := range []struct {
		name, data string
		want       inputMode
	}{
		{"objects", "{\"a\":1}\n{\"a\":2}\n", inputJSONL},
		{"after blank lines", "\n  \n{\"a\":1}\n", inputJSONL},
		{"first line cut short", "{\"a\":\n{\"a\":2}\n", inputJSONL},
		{"CSV", "a,b\n1,2\n", inputCSV},
		{"a brace no line closes", "{a},b\n1,2\n", inputCSV},
		{"brackets not JSON", "[a],b\n1,2\n", inputCSV},
	} {
		got, err := sniffInput([]byte(c.data))
		if err != nil || got != c.want {
			t.Errorf("%s: %q, %v, want %q", c.name, got, err, c.want)
		}
	}
	if _, err := sniffInput([]byte(" [{\"a\":1},{\"a\":2}]\n")); err == nil || !strings.Contains(err.Error(), "jq -c") {
		t.Errorf("an array: %v, want an error saying how to make JSON Lines of it", err)
	}
}

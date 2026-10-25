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
	if n := len(tb.sparse[1]); n != 4 {
		t.Errorf("the second row holds %d cells, want 4: its own keys', null's among them, not one a column", n)
	}
	if len(tb.notes) != 0 {
		t.Errorf("notes %q, want none: a blank line is no row and no fault", tb.notes)
	}
}

// grid is tb's cells as cell reads them, every row as wide as the names.
func grid(tb *table) [][]string {
	g := make([][]string, tb.rowCount())
	for i := range tb.rowCount() {
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
	if want := []string{"2 lines: a column given two values, the last kept; the first line 1"}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}
	// The last value replaces the whole of the first, in the first's
	// place: a.x of the first object was kept beside a.y of the last, and
	// a's 1 beside a.b, with no word. In an object nested, and in one too
	// large to look for repeats pair by pair, the same.
	many := `{"k0":0`
	for i := 1; i < 20; i++ {
		many += fmt.Sprintf(`,"k%d":%d`, i, i)
	}
	many += `,"k3":{"z":9}}`
	for _, c := range []struct {
		line  string
		names []string
		row   []string
	}{
		{`{"a":{"x":1},"t":0,"a":{"y":2}}`, []string{"a.y", "t"}, []string{"2", "0"}},
		{`{"a":1,"a":{"b":2}}`, []string{"a.b"}, []string{"2"}},
		{`{"a":{"b":2},"a":1}`, []string{"a"}, []string{"1"}},
		{`{"o":{"k":1,"j":3,"k":2}}`, []string{"o.k", "o.j"}, []string{"2", "3"}},
		{many, nil, nil},
	} {
		tb, err := readJSONLines([]byte(c.line), decimalAuto)
		if err != nil {
			t.Fatal(err)
		}
		if c.names == nil {
			if len(tb.names) != 20 || tb.names[3] != "k3.z" || tb.cell(0, 3) != "9" {
				t.Errorf("%s: names %q, want k3.z fourth, holding 9", c.line, tb.names)
			}
		} else if !reflect.DeepEqual(tb.names, c.names) || !reflect.DeepEqual(grid(tb)[0], c.row) {
			t.Errorf("%s: names %q, row %q, want %q, %q", c.line, tb.names, grid(tb)[0], c.names, c.row)
		}
		if want := []string{"line 1: a column given two values, the last kept"}; !reflect.DeepEqual(tb.notes, want) {
			t.Errorf("%s: notes %q, want %q", c.line, tb.notes, want)
		}
	}

	// The line's own fields hold the kept value once, not once a repeat,
	// which readJSONLines would merge without a sign.
	if w, err := walkLine([]byte(`{"a":1,"t":0,"a":2,"a":3}`)); err != nil || !reflect.DeepEqual(w.fields, []field{{name: "a", value: "3"}, {name: "t", value: "0"}}) || !w.twice {
		t.Errorf("fields %+v, %v, want a 3 once, then t", w.fields, err)
	}

	// The key "" holds an object whose x is ".x", not the line's own x.
	tb, err = readJSONLines([]byte(`{"x":1,"":{"x":2}}`), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"x", ".x"}; !reflect.DeepEqual(tb.names, want) || len(tb.notes) != 0 {
		t.Errorf("an object under the empty key: names %q, notes %q, want %q and none", tb.names, tb.notes, want)
	}
}

// Whether a cell is a string is its last value's to say, and only the
// strings the rows hold decide the decimal mark: a number after a string,
// the key written twice, was rewritten with a decimal comma, 1.500 as
// 1500, and a string replaced decided a mark no cell is written in.
func TestReadJSONLinesStringsAsLeft(t *testing.T) {
	tb, err := readJSONLines([]byte(`{"a":"1,5"}`+"\n"+`{"a":"x","a":1.500}`+"\n"+`{"a":"2,5"}`+"\n"+`{"a.b":"1,5","a":{"b":1.500}}`+"\n"), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if got := grid(tb); got[0][0] != "1.5" || got[1][0] != "1.500" || got[2][0] != "2.5" || got[3][1] != "1.500" {
		t.Errorf("rows %q, want the strings read with a comma and the numbers 1.500 left be", got)
	}
	if _, err := readJSONLines([]byte(`{"v":"1,5","v":2}`+"\n"+`{"w":"0.5"}`+"\n"), decimalAuto); err != nil {
		t.Errorf("a decimal comma only in a string replaced: %v, want no file written both ways", err)
	}
	// JSON's numbers say nothing of the mark: 1.5 beside "0,5" is no file
	// written both ways.
	if tb, err := readJSONLines([]byte(`{"n":1.5,"s":"0,5"}`+"\n"), decimalAuto); err != nil || tb.cell(0, 1) != "0.5" {
		t.Errorf("a number and a string with a comma: %v, want the string read with it", err)
	}
}

// The decimal mark of a JSON Lines file's strings is the columns plotted's
// to say, and theirs alone: a "3,4" among the ids made "1.234" 1234, and a
// "2.5" in one field with a "1,2" in another refused a log plotting
// neither. Among the columns plotted, a point in one and a comma in
// another, or both in one, are refused, naming each; where nothing says,
// a point, and a string that reads two ways said.
func TestSettleMarks(t *testing.T) {
	for _, c := range []struct {
		name, data string
		args       []string
		want, err  string // in standard error, or the error
	}{
		{"ids do not decide", `{"t":1,"d":"1.234","ids":"3,4"}` + "\n" + `{"t":2,"d":"2.468","ids":"5"}`, []string{"--y", "d"}, `"1.234" in "d" on line 1 read as 1.234, with a decimal point`, ""},
		{"fields not plotted", `{"a":1,"tags":"1,2"}` + "\n" + `{"a":2,"ver":"2.5"}`, []string{"--y", "a"}, "", ""},
		{"plotted fields at odds", `{"t":1,"a":"1,5","b":"2.5"}` + "\n" + `{"t":2,"a":"2,5","b":"3.5"}`, []string{"--y", "a,b"}, "", `"2.5" in "b" on line 1 with a decimal point, "1,5" in "a" on line 1 with a decimal comma`},
		{"one field at odds", `{"t":1,"v":"0,5"}` + "\n" + `{"t":2,"v":"1.5"}` + "\n" + `{"t":3,"v":"2.5"}`, []string{"--y", "v"}, "", `in "v" on line 2`},
		{"told", `{"t":1,"v":"0,5"}` + "\n" + `{"t":2,"v":"1.5"}` + "\n" + `{"t":3,"v":"2.5"}`, []string{"--y", "v", "--decimal", "point"}, "", ""},
		{"a key that was empty", `{"t":1,"":"0,5","b":"1.5"}` + "\n" + `{"t":2,"":"1,5","b":"2.5"}`, []string{"--y", "column 2,b"}, "", `in "column 2" on line 1 with a decimal comma`},
		{"x decides too", `{"x":"0,5","v":"1.234"}` + "\n" + `{"x":"1,5","v":"2.345"}`, []string{"--x", "x", "--y", "v"}, "", ""},
	} {
		_, errOut, err := plot(t, c.data, c.args...)
		switch {
		case c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)):
			t.Errorf("%s: %v, want an error holding %q", c.name, err, c.err)
		case c.err == "" && err != nil:
			t.Errorf("%s: %v", c.name, err)
		case c.want != "" && !strings.Contains(errOut, c.want):
			t.Errorf("%s: stderr %q, want %q", c.name, errOut, c.want)
		case c.want == "" && c.err == "" && strings.Contains(errOut, "decimal"):
			t.Errorf("%s: stderr %q, want no word of the decimal mark", c.name, errOut)
		}
	}
	// With x's comma deciding, v's "1.234" is 1234: plotted, its axis
	// reaches a thousand.
	out, _, err := plot(t, `{"x":"0,5","v":"1.234"}`+"\n"+`{"x":"1,5","v":"2.345"}`, "--x", "x", "--y", "v")
	if err != nil || !strings.Contains(out, "2000┤") {
		t.Errorf("v read with x's comma: %v, want its axis in thousands:\n%s", err, out)
	}
	// And --list, reading each column by its own strings, says the one at
	// odds rather than refusing.
	// It names the column once, at its head, and its cells by line.
	_, errOut, err := plot(t, `{"v":"0,5"}`+"\n"+`{"v":"1.5"}`+"\n", "--list")
	if err != nil || !strings.Contains(errOut, `"v": numbers written both ways, "1.5" on line 2 with a decimal point`) {
		t.Errorf("--list of a column at odds: %v, stderr %q", err, errOut)
	}
	// A column whose strings show no mark but read two ways is listed
	// with a point, and the guess said, as a CSV's is.
	if _, errOut, err := plot(t, `{"v":"1,234"}`+"\n"+`{"v":"2"}`+"\n", "--list"); err != nil || !strings.Contains(errOut, `"v": "1,234" on line 1 read as 1234, with a decimal point; --decimal comma reads it the other way`) {
		t.Errorf("--list of a column guessed at: %v, stderr %q", err, errOut)
	}
	// With --decimal saying, there is nothing to say: the note claimed a
	// point and that --decimal was still wanted.
	if _, errOut, err := plot(t, `{"v":"0,5"}`+"\n"+`{"v":"1.5"}`+"\n", "--list", "--decimal", "comma"); err != nil || errOut != "" {
		t.Errorf("--list --decimal comma of a column at odds: %v, stderr %q, want none", err, errOut)
	}

	// x chosen for being in order, read with a point, as its own strings
	// show no mark, is in order no longer read with the comma the y
	// shows: 12, 1.234, 2. Refused, not plotted against; with --x, it is.
	const unordered = `{"x":"12","y":"0,5"}` + "\n" + `{"x":"1,234","y":"0,7"}` + "\n" + `{"x":"2,000","y":"0,9"}` + "\n"
	if _, _, err := plot(t, unordered); err == nil || !strings.Contains(err.Error(), `"x" is in order read with a decimal point`) || !strings.Contains(err.Error(), "say --x") {
		t.Errorf("x out of order once the mark is settled: %v, want it refused", err)
	}
	if _, _, err := plot(t, unordered, "--x", "x"); err != nil {
		t.Errorf("x out of order, with --x x: %v", err)
	}
	if out, _, err := plot(t, unordered, "--x", "0"); err != nil || !strings.Contains(out, "x and y against row") {
		t.Errorf("x out of order, with --x 0: %v\n%s", err, out)
	}
}

// A log keyed by user has a key a line; past maxJSONColumns the keys are
// left out and said, and a row holds only the cells its own keys have:
// after the first thousand lines brought a thousand users, each line naming
// one of them held a cell a column to that user's, some 500 a line, and 5
// MB of log took 2.9 GB.
func TestReadJSONLinesHoldsItsColumns(t *testing.T) {
	var b strings.Builder
	const lines = 3 * maxJSONColumns
	for i := range lines {
		fmt.Fprintf(&b, `{"t":%d,"counts":{"user%d":1}}`+"\n", i, i)
	}
	for i := range lines {
		fmt.Fprintf(&b, `{"t":%d,"counts":{"user%d":1}}`+"\n", i, (i*7919)%(maxJSONColumns-1))
	}
	tb, err := readJSONLines([]byte(b.String()), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if len(tb.names) != maxJSONColumns || tb.rowCount() != 2*lines {
		t.Errorf("%d columns and %d rows, want %d and %d", len(tb.names), tb.rowCount(), maxJSONColumns, 2*lines)
	}
	for i, row := range tb.sparse {
		if len(row) > 2 {
			t.Fatalf("row %d holds %d cells, want its own keys', 2 at most", i, len(row))
		}
	}
	if got, want := tb.cell(2*lines-1, 1+((lines-1)*7919)%(maxJSONColumns-1)), "1"; got != want {
		t.Errorf("the last row's user: %q, want %q", got, want)
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
	if want := []string{"4 lines: not a JSON object, left out; the first line 2", fmt.Sprintf("line 6: nested past %d deep, left out", maxJSONDepth)}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}
	if _, err := readJSONLines([]byte(deep), decimalAuto); err == nil || !strings.Contains(err.Error(), "nested past") {
		t.Errorf("only a line too deep: %v, want it said to be nested too deep", err)
	}

	// As deep as maxJSONDepth is read.
	shallow := strings.Repeat(`{"a":`, maxJSONDepth) + "1" + strings.Repeat("}", maxJSONDepth)
	if tb, err := readJSONLines([]byte(shallow), decimalAuto); err != nil || tb.rowCount() != 1 || len(tb.notes) != 0 {
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

// auto takes a file for JSON Lines where any of its first 20 lines is a
// whole object holding a value, whatever the rest are -- a log read from its
// middle begins with the end of a line, and a Python log has a traceback
// after each object -- and anything else for CSV. JSON that is not JSON
// Lines is refused, saying jq -c makes it so where it would.
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
		{"one object in three", "x\ny\n{\"a\":1}\n", inputJSONL},
		// A Python log with a traceback after each object: a quarter of
		// its lines objects, and read as CSV of their fragments.
		{"tracebacks between objects", strings.Repeat("{\"ts\":1,\"v\":2}\nTraceback (most recent call last):\n  File \"a.py\", line 1\nValueError: x\n", 5), inputJSONL},
		// Three whole lines and then cut ones decode as three objects,
		// and were refused as objects over several lines.
		{"whole lines, then cut ones", "{\"v\":1}\n{\"v\":2}\n{\"v\":3}\n{\"v\"\n{\"v\"\n{\"v\"\n{\"v\"\n", inputJSONL},
		// An empty object gives no column: a CSV of payloads, rows of
		// {}, was read as JSON Lines of nothing.
		{"a column of empty objects", "payload\n{}\n{}\n", inputCSV},
		{"an object holding only an empty one", "x\n{\"a\":{}}\n", inputCSV},
		{"a CSV cell holding an object", "a,b\n\"{\"\"x\"\":1}\",2\n", inputCSV},
		{"CSV", "a,b\n1,2\n", inputCSV},
		{"a brace no line closes", "{a},b\n1,2\n", inputCSV},
		{"brackets not JSON", "[a],b\n1,2\n", inputCSV},
		{"a brace and JSON after", "{\n\"a\": 1\n} x\n", inputCSV},
		{"NaN in most lines", "{\"v\":NaN}\n{\"v\":NaN}\n{\"v\":1}\n", inputJSONL},
		{"one bracketed line", "[a]\n", inputCSV},
		{"an array and CSV after", "[1],b\n1,2\n", inputCSV},
		{"objects too deep", strings.Repeat(strings.Repeat(`{"a":`, maxJSONDepth+1)+"1"+strings.Repeat("}", maxJSONDepth+1)+"\n", 3), inputJSONL},
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
		// Each holds a line that is a whole object, which read as JSON
		// Lines would be one row of each.
		{"an array of objects a line", "[\n{\"a\":1},\n{\"a\":2}\n]\n", "jq -c '.[]'", ""},
		{"a pretty object holding one", "{\n  \"a\": 1,\n  \"b\":\n{\"c\":2}\n}\n", "jq -c .", ""},
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
	if want := []string{fmt.Sprintf("3 lines: an array cut to its first %d elements, the first \"samples\"; the first line 1", maxJSONArray)}; !reflect.DeepEqual(tb.notes, want) {
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
		`{NaN:1,"b":[1,NaN],"c":{"d":NaN}}`:  `{NaN:1,"b":[1,"NaN"],"c":{"d":"NaN"}}`,
		`{"a":1,Infinity:2}`:                 `{"a":1,Infinity:2}`,
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
	names = []string{"", "column 1", "column 1, unnamed"}
	nameColumns(names)
	if want := []string{"column 1, unnamed 2", "column 1", "column 1, unnamed"}; !reflect.DeepEqual(names, want) {
		t.Errorf("%q, want %q", names, want)
	}
}

// A key left out past maxJSONColumns is said to be, though its name is a
// number that could be a column's: --y 5 plotted the fifth column.
func TestFindColumnSaysAKeyLeftOut(t *testing.T) {
	tb := &table{names: []string{"t", "a", "b", "c", "d", "e"}, dropped: map[string]bool{"5": true}}
	if _, err := findColumn(tb, "5"); err == nil || !strings.Contains(err.Error(), "left out") {
		t.Errorf("--y 5, a key left out: %v", err)
	}
	if j, err := findColumn(tb, "4"); err != nil || j != 3 {
		t.Errorf("--y 4: %d, %v, want the fourth column", j, err)
	}
	// Nor is a key left out found as another in a different case.
	tb = &table{names: []string{"t", "latency"}, dropped: map[string]bool{"Latency": true}}
	if _, err := findColumn(tb, "Latency"); err == nil || !strings.Contains(err.Error(), "left out") {
		t.Errorf("--y Latency, left out beside latency: %v", err)
	}
	if j, err := findColumn(tb, "LATENCY"); err != nil || j != 1 {
		t.Errorf("--y LATENCY: %d, %v, want latency, in another case", j, err)
	}
}

// A line whose key is NaN is no JSON, and is left out, not read as an
// object with a column "NaN".
func TestReadJSONLinesNaNKey(t *testing.T) {
	tb, err := readJSONLines([]byte(`{"v":1}`+"\n"+`{NaN:1,"b":2}`+"\n"), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"v"}; !reflect.DeepEqual(tb.names, want) || len(tb.notes) != 1 {
		t.Errorf("names %q, notes %q, want %q and line 2 left out", tb.names, tb.notes, want)
	}
}

// A table answers the same through rowCount, cell and eachColumn whether
// its rows are a CSV's or JSON Lines' sparse ones.
func TestTableSparseAndDense(t *testing.T) {
	dense := &table{names: []string{"a", "b", "c"}, rows: [][]string{{"1", "", "x"}, {"2"}, {"", "5", "y"}}}
	sparse := &table{names: []string{"a", "b", "c"}, sparse: [][]sparseCell{
		{{col: 0, value: "1"}, {col: 2, value: "x"}},
		{{col: 0, value: "2"}},
		{{col: 0, value: ""}, {col: 1, value: "5"}, {col: 2, value: "y"}},
	}}
	columns := func(tb *table) [][]string {
		var cols [][]string
		tb.eachColumn(func(j int, cells []string) { cols = append(cols, cells) })
		return cols
	}
	if !reflect.DeepEqual(grid(dense), grid(sparse)) || dense.rowCount() != sparse.rowCount() {
		t.Errorf("cells: %q and %q", grid(dense), grid(sparse))
	}
	want := [][]string{{"1", "2"}, {"5"}, {"x", "y"}}
	if !reflect.DeepEqual(columns(dense), want) || !reflect.DeepEqual(columns(sparse), want) {
		t.Errorf("columns: %q and %q, want %q", columns(dense), columns(sparse), want)
	}
	if !reflect.DeepEqual(sparse.column(2), want[2]) {
		t.Errorf("column 3: %q", sparse.column(2))
	}
}

// One line's note names the line, and an array cut there by its name.
func TestReadJSONLinesSaysOneLine(t *testing.T) {
	tb, err := readJSONLines([]byte(`{"t":1}`+"\n"+`{"t":2,"s":[`+strings.Repeat("1,", maxJSONArray)+`1]}`+"\n"), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{fmt.Sprintf("line 2: \"s\" cut to its first %d elements", maxJSONArray)}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}
}

// One key past the columns kept is "1 key", not "1 keys".
func TestReadJSONLinesOneKeyLeftOut(t *testing.T) {
	var b strings.Builder
	for i := range maxJSONColumns + 1 {
		fmt.Fprintf(&b, "{\"k%d\":1}\n", i)
	}
	tb, err := readJSONLines([]byte(b.String()), decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{fmt.Sprintf("1 key past the first %d left out, \"k%d\"", maxJSONColumns, maxJSONColumns)}; !reflect.DeepEqual(tb.notes, want) {
		t.Errorf("notes %q, want %q", tb.notes, want)
	}
}

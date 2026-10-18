package main

import (
	"reflect"
	"strings"
	"testing"
)

// The delimiter is the one every line holds the same number of, outside
// quotes; a comma inside a tab file's values does not win it for commas.
func TestSniffDelimiter(t *testing.T) {
	for _, c := range []struct {
		name, data string
		want       rune
	}{
		{"comma", "a,b,c\n1,2,3\n4,5,6\n", ','},
		{"tab", "a\tb\n1\t2\n", '\t'},
		{"semicolon", "a;b\n1,5;2\n3,25;4\n", ';'},
		{"bar", "a|b|c\n1|2|3\n", '|'},
		{"quoted commas", "name,price\n\"Smith, J\",3\n\"Doe, A\",4\n", ','},
		{"commas in a tab file", "note\tv\nhi, there\t1\nyes\t2\n", '\t'},
		{"one column", "v\n1\n2\n", ','},
		{"CRLF", "a;b\r\n1;2\r\n", ';'},
		{"decimal commas, no header", "1,5;2,5\n2,5;3,5\n", ';'},
	} {
		if got := sniffDelimiter([]byte(c.data)); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// A first row of names over numbers is a header; a first row of numbers is
// data, and the columns are numbered.
func TestReadTableFindsTheHeader(t *testing.T) {
	tb, err := readTable(strings.NewReader("\xef\xbb\xbfdate, close\n2024-01-02,10\n2024-01-03,11\n"), 0, headerAuto, datesAuto, decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"date", "close"}; !reflect.DeepEqual(tb.names, want) {
		t.Errorf("names %q, want %q (the byte-order mark and the space after the comma gone)", tb.names, want)
	}
	if len(tb.rows) != 2 {
		t.Errorf("%d rows, want 2", len(tb.rows))
	}

	tb, err = readTable(strings.NewReader("1,2\n3,4\n"), 0, headerAuto, datesAuto, decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"column 1", "column 2"}; !reflect.DeepEqual(tb.names, want) || len(tb.rows) != 2 {
		t.Errorf("names %q and %d rows, want %q and 2: a row of numbers is data", tb.names, len(tb.rows), want)
	}

	tb, err = readTable(strings.NewReader("1,2\n3,4\n"), 0, headerYes, datesAuto, decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1", "2"}; !reflect.DeepEqual(tb.names, want) {
		t.Errorf("--header yes: names %q, want %q", tb.names, want)
	}
}

// Rows of different lengths are padded with empty cells, which are gaps,
// and a quoted value holds the delimiter.
func TestReadTableForgivesRaggedRows(t *testing.T) {
	tb, err := readTable(strings.NewReader("a,b,c\n1,\"2,000\"\n4,5,6,7\n"), 0, headerAuto, datesAuto, decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"1", "2,000", "", ""}, {"4", "5", "6", "7"}}
	if !reflect.DeepEqual(tb.rows, want) {
		t.Errorf("rows %q, want %q", tb.rows, want)
	}
	if tb.names[3] != "column 4" {
		t.Errorf("the unnamed fourth column is %q", tb.names[3])
	}
}

func TestParseDelimiter(t *testing.T) {
	for in, want := range map[string]rune{"auto": 0, "tab": '\t', `\t`: '\t', ";": ';', "|": '|'} {
		if got, err := parseDelimiter(in); err != nil || got != want {
			t.Errorf("%q: %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "ab", `"`} {
		if _, err := parseDelimiter(in); err == nil {
			t.Errorf("%q taken as a delimiter", in)
		}
	}
}

// In a tab file an empty cell is two tabs in a row, and the values after it
// stay in their own columns rather than each moving one left.
func TestReadTableKeepsATabFilesEmptyCells(t *testing.T) {
	tb, err := readTable(strings.NewReader("a\tb\tc\n1\t\t3\n\t5\t6\n"), 0, headerAuto, datesAuto, decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"1", "", "3"}, {"", "5", "6"}}
	if !reflect.DeepEqual(tb.rows, want) {
		t.Errorf("rows %q, want %q", tb.rows, want)
	}
}

// The decimal mark is the file's, decided once from its own numbers: a
// comma where a cell reads as a number only with one -- "0,5", "1.234,5" --
// and a point where one reads only with a point -- "1.5", "1,234,567".
// Where none says, a semicolon file writes a decimal comma and any other a
// point, and a cell that read as two numbers is noted. Decided cell by cell
// as it once was, a tab file from an English spreadsheet read "1,234" as
// one point two three four while "1,234,567" beside it was thousands.
func TestReadTableDecidesTheDecimalMarkForTheFile(t *testing.T) {
	for _, c := range []struct {
		name, data string
		mark       decimalMark
		want       [][]string
		note       string
	}{
		{"semicolons", "t;v\n1;1,250\n2;-1.234,5\n3;2,5\n", decimalAuto, [][]string{{"1", "1.250"}, {"2", "-1234.5"}, {"3", "2.5"}}, ""},
		{"no header", "1,5;2,5\n2,5;3,5\n", decimalAuto, [][]string{{"1.5", "2.5"}, {"2.5", "3.5"}}, ""},
		{"tabs with a decimal comma", "t\tv\n1\t0,5\n", decimalAuto, [][]string{{"1", "0.5"}}, ""},
		{"tabs with thousands", "t\tv\n1\t999\n2\t1,234\n3\t1,234,567\n", decimalAuto, [][]string{{"1", "999"}, {"2", "1,234"}, {"3", "1,234,567"}}, ""},
		{"evidence in another column", "a\tb\n1,234\t0,5\n", decimalAuto, [][]string{{"1.234", "0.5"}}, ""},
		{"points as thousands", "t;v\n1;1.234\n2;0,5\n", decimalAuto, [][]string{{"1", "1234"}, {"2", "0.5"}}, ""},
		{"commas", "t,v\n1,\"1,250\"\n", decimalAuto, [][]string{{"1", "1,250"}}, `"1,250" read as 1250, with a decimal point; --decimal comma reads it the other way`},
		{"ambiguous semicolons", "t;v\n1;1,234\n", decimalAuto, [][]string{{"1", "1.234"}}, `"1,234" read as 1.234, with a decimal comma; --decimal point reads it the other way`},
		{"told", "t;v\n1;1,234\n", decimalPoint, [][]string{{"1", "1,234"}}, ""},
		{"text", "t;v\n1;a,b\n", decimalAuto, [][]string{{"1", "a,b"}}, ""},
		{"dates and versions", "d;v\n03.04.2024;1\n10.0.0.1;2\n", decimalAuto, [][]string{{"03.04.2024", "1"}, {"10.0.0.1", "2"}}, ""},
	} {
		tb, err := readTable(strings.NewReader(c.data), 0, headerAuto, datesAuto, c.mark)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !reflect.DeepEqual(tb.rows, c.want) {
			t.Errorf("%s: rows %q, want %q", c.name, tb.rows, c.want)
		}
		if note := strings.Join(tb.notes, "; "); note != c.note {
			t.Errorf("%s: note %q, want %q", c.name, note, c.note)
		}
	}
	if v, ok := parseNumber("1,250"); !ok || v != 1250 {
		t.Errorf("a grouped number reads %v, %v", v, ok)
	}
}

// A file whose numbers are written both ways is at odds with itself, and is
// refused with a cell of each kind and the flag that settles it, rather
// than read half one way and half the other.
func TestReadTableRefusesTwoDecimalMarks(t *testing.T) {
	for _, data := range []string{
		"v;w\n1,234,567;x\n1,234;x\n5,5;x\n",
		"v;w\n1.234;x\n2.5;x\n0,5;x\n",
	} {
		_, err := readTable(strings.NewReader(data), 0, headerAuto, datesAuto, decimalAuto)
		if err == nil || !strings.Contains(err.Error(), "both ways") || !strings.Contains(err.Error(), "--decimal") {
			t.Errorf("%q: %v, want an error saying the numbers are written both ways", data, err)
		}
	}
}

// A file aligned with runs of spaces, read with --delimiter ' ', has one
// delimiter between its columns however many spaces there are: not a
// phantom column of empty cells for the extra ones, which once pushed one
// row's reading under it.
func TestReadTableMergesRunsOfSpaces(t *testing.T) {
	tb, err := readTable(strings.NewReader("a  b\n1  10\n10 20\n2  30\n"), ' ', headerAuto, datesAuto, decimalAuto)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"1", "10"}, {"10", "20"}, {"2", "30"}}
	if !reflect.DeepEqual(tb.names, []string{"a", "b"}) || !reflect.DeepEqual(tb.rows, want) {
		t.Errorf("names %q, rows %q; want [a b] and %q", tb.names, tb.rows, want)
	}
}

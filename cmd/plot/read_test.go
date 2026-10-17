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
	tb, err := readTable(strings.NewReader("\xef\xbb\xbfdate, close\n2024-01-02,10\n2024-01-03,11\n"), 0, headerAuto, datesAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"date", "close"}; !reflect.DeepEqual(tb.names, want) {
		t.Errorf("names %q, want %q (the byte-order mark and the space after the comma gone)", tb.names, want)
	}
	if len(tb.rows) != 2 {
		t.Errorf("%d rows, want 2", len(tb.rows))
	}

	tb, err = readTable(strings.NewReader("1,2\n3,4\n"), 0, headerAuto, datesAuto)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"column 1", "column 2"}; !reflect.DeepEqual(tb.names, want) || len(tb.rows) != 2 {
		t.Errorf("names %q and %d rows, want %q and 2: a row of numbers is data", tb.names, len(tb.rows), want)
	}

	tb, err = readTable(strings.NewReader("1,2\n3,4\n"), 0, headerYes, datesAuto)
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
	tb, err := readTable(strings.NewReader("a,b,c\n1,\"2,000\"\n4,5,6,7\n"), 0, headerAuto, datesAuto)
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
	tb, err := readTable(strings.NewReader("a\tb\tc\n1\t\t3\n\t5\t6\n"), 0, headerAuto, datesAuto)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"1", "", "3"}, {"", "5", "6"}}
	if !reflect.DeepEqual(tb.rows, want) {
		t.Errorf("rows %q, want %q", tb.rows, want)
	}
}

// A file delimited by semicolons, or tabs, writes its numbers with decimal
// commas: "1,250" there is one and a quarter, and a headerless file of them
// is told from a comma file though each line holds as many commas as
// semicolons. In a comma file a quoted "1,250" is still twelve hundred and
// fifty.
func TestReadTableReadsDecimalCommas(t *testing.T) {
	for _, c := range []struct {
		name, data string
		want       [][]string
	}{
		{"semicolons", "t;v\n1;1,250\n2;-1.234,5\n3;2,5\n", [][]string{{"1", "1.250"}, {"2", "-1234.5"}, {"3", "2.5"}}},
		{"no header", "1,5;2,5\n2,5;3,5\n", [][]string{{"1.5", "2.5"}, {"2.5", "3.5"}}},
		{"tabs", "t\tv\n1\t0,5\n", [][]string{{"1", "0.5"}}},
		{"commas", "t,v\n1,\"1,250\"\n", [][]string{{"1", "1,250"}}},
		{"text", "t;v\n1;a,b\n", [][]string{{"1", "a,b"}}},
	} {
		tb, err := readTable(strings.NewReader(c.data), 0, headerAuto, datesAuto)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !reflect.DeepEqual(tb.rows, c.want) {
			t.Errorf("%s: rows %q, want %q", c.name, tb.rows, c.want)
		}
	}
	if v, ok := parseNumber("1,250"); !ok || v != 1250 {
		t.Errorf("a grouped number reads %v, %v", v, ok)
	}
}

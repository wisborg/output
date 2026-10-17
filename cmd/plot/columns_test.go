package main

import (
	"testing"
	"time"
)

func TestParseNumber(t *testing.T) {
	for in, want := range map[string]float64{"12": 12, "-0.5": -0.5, "1e3": 1000, "1,234,567.5": 1234567.5, "+3": 3} {
		if got, ok := parseNumber(in); !ok || got != want {
			t.Errorf("%q: %v, %v; want %v", in, got, ok, want)
		}
	}
	for _, in := range []string{"Inf", "NaN", "1,23", "12,3456", "abc", "3.4.2024"} {
		if v, ok := parseNumber(in); ok {
			t.Errorf("%q read as the number %v", in, v)
		}
	}
}

// A missing value is said in the ways files say it, and is never a number.
func TestMissing(t *testing.T) {
	for _, in := range []string{"", "NA", "n/a", "NULL", "NaN", "-"} {
		if !missing(in) {
			t.Errorf("%q not taken as missing", in)
		}
	}
	if missing("0") {
		t.Error("0 taken as missing: a zero is a reading")
	}
}

// A column of mostly numbers is numbers, its odd cell counted; dates are
// read by the layout that reads most of them.
func TestClassify(t *testing.T) {
	c := classify([]string{"1", "2", "n.d.", "", "4"}, datesAuto)
	if c.kind != numberColumn || c.values != 4 || c.wrong != 1 {
		t.Errorf("%+v, want numbers, 4 values, 1 not a number", c)
	}
	if c := classify([]string{"a", "b", "1"}, datesAuto); c.kind != textColumn {
		t.Errorf("mostly text read as %v", c.kind)
	}
	if c := classify([]string{"2024-01-02", "2024-01-03", "2024-01-04T10:00:00Z"}, datesAuto); c.kind != dateColumn || c.wrong != 1 {
		t.Errorf("%+v, want dates by one layout, the other counted", c)
	}
	if c := classify(nil, datesAuto); c.kind != textColumn || c.values != 0 {
		t.Errorf("an empty column is %+v", c)
	}
}

// Dates in numbers alone are told day-first or month-first by a day past
// the 12th; where none is, -dates must say, and does.
func TestClassifyNumericDates(t *testing.T) {
	dmy := classify([]string{"03.04.2024", "25.04.2024"}, datesAuto)
	if dmy.kind != dateColumn || dmy.ambiguous {
		t.Fatalf("%+v, want unambiguous dates", dmy)
	}
	if v, _ := dmy.value("03.04.2024"); time.Unix(int64(v), 0).UTC().Month() != time.April {
		t.Errorf("03.04.2024 read as %v, want the 3rd of April: 25.04 says the day comes first", time.Unix(int64(v), 0).UTC())
	}
	mdy := classify([]string{"4/3/2024", "4/25/2024"}, datesAuto)
	if v, _ := mdy.value("4/3/2024"); mdy.ambiguous || time.Unix(int64(v), 0).UTC().Day() != 3 {
		t.Errorf("4/3/2024 read as %v, want the 3rd of April: 4/25 says the month comes first", time.Unix(int64(v), 0).UTC())
	}
	if c := classify([]string{"03/04/2024", "05/06/2024"}, datesAuto); !c.ambiguous {
		t.Errorf("%+v: dates that read either way not called ambiguous", c)
	}
	c := classify([]string{"03/04/2024", "05/06/2024"}, datesMDY)
	if v, _ := c.value("03/04/2024"); c.ambiguous || time.Unix(int64(v), 0).UTC().Month() != time.March {
		t.Errorf("-dates mdy: %+v reads 03/04/2024 as %v", c, time.Unix(int64(v), 0).UTC())
	}
}

// A date is placed by its clock as written, whatever zone it names.
func TestParseDateKeepsTheWallClock(t *testing.T) {
	v, ok := parseDate("2024-03-05T09:30:00-05:00", time.RFC3339Nano)
	if !ok {
		t.Fatal("not read")
	}
	if got := time.Unix(int64(v), 0).UTC().Format("15:04"); got != "09:30" {
		t.Errorf("09:30 in New York placed at %s", got)
	}
}

func TestFindColumn(t *testing.T) {
	names := []string{"Date", "Close", "2024"}
	for spec, want := range map[string]int{"Close": 1, "close": 1, "1": 0, "2024": 2, " Date ": 0} {
		if got, err := findColumn(names, spec); err != nil || got != want {
			t.Errorf("%q: %d, %v; want %d", spec, got, err, want)
		}
	}
	for _, spec := range []string{"Open", "0", "4"} {
		if _, err := findColumn(names, spec); err == nil {
			t.Errorf("%q found", spec)
		}
	}
}

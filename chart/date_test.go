package chart

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// A Date axis ticks where a calendar does and writes each tick as briefly as
// the tick beside it allows, its first in full, from hours to decades.
func TestDateTicks(t *testing.T) {
	at := func(s string) float64 {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return float64(v.Unix())
	}
	for _, c := range []struct {
		from, to string
		want     []string
	}{
		{"2024-03-05T09:00:00Z", "2024-03-05T17:30:00Z", []string{"5 Mar 10:00", "12:00", "14:00", "16:00"}},
		{"2024-03-04T18:00:00Z", "2024-03-06T06:00:00Z", []string{"4 Mar 18:00", "5 Mar", "06:00", "12:00", "18:00", "6 Mar", "06:00"}},
		{"2024-03-01T00:00:00Z", "2024-03-20T00:00:00Z", []string{"1 Mar 2024", "6 Mar", "11 Mar", "16 Mar"}},
		{"2024-01-10T00:00:00Z", "2024-04-20T00:00:00Z", []string{"Feb 2024", "Mar", "Apr"}},
		{"2023-03-10T00:00:00Z", "2024-11-20T00:00:00Z", []string{"Jul 2023", "2024", "Jul"}},
		{"1990-01-01T00:00:00Z", "2024-11-20T00:00:00Z", []string{"1990", "2000", "2010", "2020"}},
	} {
		s := ExactScale(at(c.from), at(c.to), 6, Date)
		var labels []string
		for _, v := range s.Ticks() {
			labels = append(labels, s.Label(v))
		}
		if !reflect.DeepEqual(labels, c.want) {
			t.Errorf("%s to %s: ticks %q, want %q", c.from, c.to, labels, c.want)
		}
	}
}

// Ticks a month apart fall on each month's first, though months differ in
// length: counting a month as thirty-odd days would drift a day or two off
// it within the year.
func TestDateTicksOnTheFirst(t *testing.T) {
	from := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s := ExactScale(float64(from.Unix()), float64(to.Unix()), 12, Date)
	ticks := s.Ticks()
	if len(ticks) != 13 {
		t.Fatalf("%d ticks over a year a month apart, want 13", len(ticks))
	}
	for _, v := range ticks {
		if d := time.Unix(int64(v), 0).UTC(); d.Day() != 1 || d.Hour() != 0 {
			t.Errorf("a tick on %v, not a first of the month", d)
		}
	}
}

// Ticks five and ten days apart fall on the same days of every month --
// the 1st, 6th, 11th, 16th, 21st and 26th; the 1st, 11th and 21st -- so
// each month's begin on its first however long the month before it was.
// Ticks two days apart, which a day divides, are every other midnight.
func TestDateTicksStepsOfDays(t *testing.T) {
	day := func(y int, m time.Month, d int) float64 {
		return float64(time.Date(y, m, d, 0, 0, 0, 0, time.UTC).Unix())
	}
	for _, c := range []struct {
		name     string
		from, to float64
		ticks    int
		days     []int
		firsts   []time.Month
	}{
		{"five days", day(2024, 2, 1), day(2024, 3, 31), 12, []int{1, 6, 11, 16, 21, 26}, []time.Month{2, 3}},
		{"ten days", day(2024, 1, 1), day(2024, 4, 30), 12, []int{1, 11, 21}, []time.Month{1, 2, 3, 4}},
	} {
		s := ExactScale(c.from, c.to, c.ticks, Date)
		seen := map[time.Month]bool{}
		allowed, missing := map[int]bool{}, map[int]bool{}
		for _, d := range c.days {
			allowed[d], missing[d] = true, true
		}
		for _, v := range s.Ticks() {
			d := time.Unix(int64(v), 0).UTC()
			if !allowed[d.Day()] || d.Hour() != 0 {
				t.Errorf("%s: a tick on %v", c.name, d)
			}
			if d.Day() == 1 {
				seen[d.Month()] = true
			}
			delete(missing, d.Day())
		}
		if len(missing) > 0 {
			t.Errorf("%s: no tick on the days %v of any month", c.name, missing)
		}
		for _, m := range c.firsts {
			if !seen[m] {
				t.Errorf("%s: no tick on the first of %v in %v", c.name, m, s.Ticks())
			}
		}
	}
	s := ExactScale(day(2024, 3, 1), day(2024, 3, 21), 10, Date)
	ticks := s.Ticks()
	if len(ticks) < 2 {
		t.Fatalf("two days: ticks %v", ticks)
	}
	for k, v := range ticks {
		if d := time.Unix(int64(v), 0).UTC(); d.Hour() != 0 {
			t.Errorf("two days: a tick at %v", d)
		}
		if k > 0 && v-ticks[k-1] != 2*86400 {
			t.Errorf("two days: ticks %v apart", time.Duration(v-ticks[k-1])*time.Second)
		}
	}
}

// A Date scale reaching past the years a calendar writes -- seconds that
// overflow an int64, or a span of ten thousand years -- has no ticks rather
// than a calendar stepped towards it for ever, and a reading there is
// written as the number it is. The span inside them still ticks.
func TestDateTicksEnd(t *testing.T) {
	for _, s := range []Scale{
		ExactScale(0, 1e19, 10, Date),
		ExactScale(-1e19, 0, 10, Date),
		NewScale(0, 4e17, 6, Date),
	} {
		done := make(chan []float64)
		go func() { done <- s.Ticks() }()
		select {
		case ticks := <-done:
			if ticks != nil {
				t.Errorf("%+v: %d ticks past the calendar", s, len(ticks))
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%+v: counting its ticks did not end", s)
		}
		past := s.Max
		if s.Min < firstDate {
			past = s.Min
		}
		if l := s.Label(past); !strings.Contains(l, "e+") {
			t.Errorf("%+v: %v, past the calendar, labelled %q", s, past, l)
		}
	}
	whole := ExactScale(float64(time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC).Unix()), float64(time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC).Unix()), 6, Date)
	if n := len(whole.Ticks()); n < 2 || n > 20 {
		t.Errorf("years 1 to 9999: %d ticks", n)
	}
}

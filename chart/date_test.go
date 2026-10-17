package chart

import (
	"reflect"
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

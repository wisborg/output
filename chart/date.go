package chart

import (
	"math"
	"strconv"
	"time"
)

// The lengths a Date scale's steps are measured in, in seconds. A month and
// a year are their average lengths: they choose a step, and the ticks then
// fall on the calendar's own firsts of the month, however long each is.
const (
	day   = 86400.0
	month = 365.2425 * day / 12
	year  = 365.2425 * day
)

// firstDate and lastDate are the first and the last second of the years a
// Date scale writes as dates, 1 to 9999: a calendar's, in seconds since
// 1970 began.
const (
	firstDate = -62135596800
	lastDate  = 253402300799
)

// dateSteps are the steps a Date scale ticks at, up to a year: a clock's,
// to half a day; a day and two; ten days, on the 1st, 11th and 21st of each
// month; and a month, two, three and six, on their firsts.
var dateSteps = []float64{1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600, 43200,
	day, 2 * day, 5 * day, 10 * day, month, 2 * month, 3 * month, 6 * month}

// dateStep is the smallest step of a Date scale at least raw: one of
// dateSteps, or a round number of years.
func dateStep(raw float64) float64 {
	for _, s := range dateSteps {
		if s >= raw*(1-1e-9) {
			return s
		}
	}
	return niceStep(raw/year, Number) * year
}

// dateTicks are a Date scale's ticks from lo to hi at step: every multiple
// of the step since 1970 for a step of up to two days, which a day divides
// evenly or is a multiple of; for five days and ten, the days of each
// month dateSteps names, so every month's ticks begin on its 1st however
// long the month before it was; the first of every month, two, three or six of them, counted from
// January; and the first of every year, or every two, five, ten or more,
// counted from year nought -- 2000, 2010, 2020.
//
// A scale reaching outside the years 1 to 9999 has no calendar ticks: a
// float64 of seconds there is past what time.Time and int64 can say, and
// stepping a calendar towards it never arrived -- past int64 the time
// wrapped negative, and the loop below ran for ever. Nor are more than
// maxTicks given, as for any scale.
func dateTicks(lo, hi, step float64) []float64 {
	if lo < firstDate || hi > lastDate {
		return nil
	}
	if step <= 2*day {
		return multiples(math.Ceil(lo/step-1e-9), math.Floor((hi+1e-6)/step), step)
	}
	from := time.Unix(int64(math.Floor(lo)), 0).UTC()
	var ticks []float64
	add := func(t time.Time) {
		if v := float64(t.Unix()); v >= lo && v <= hi && len(ticks) <= maxTicks {
			ticks = append(ticks, v)
		}
	}
	defer func() {
		if len(ticks) > maxTicks {
			ticks = nil
		}
	}()
	switch {
	case step < month*0.9:
		days := []int{1, 11, 21}
		if step < 7*day {
			days = []int{1, 6, 11, 16, 21, 26}
		}
		for t := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); float64(t.Unix()) <= hi; t = t.AddDate(0, 1, 0) {
			for _, d := range days {
				add(t.AddDate(0, 0, d-1))
			}
		}
	case step < year*0.9:
		every := int(math.Round(step / month))
		for t := time.Date(from.Year(), 1, 1, 0, 0, 0, 0, time.UTC); float64(t.Unix()) <= hi; t = t.AddDate(0, every, 0) {
			add(t)
		}
	default:
		every := int(math.Round(step / year))
		y := from.Year() / every * every
		for t := time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC); float64(t.Unix()) <= hi; t = t.AddDate(every, 0, 0) {
			add(t)
		}
	}
	return ticks
}

// dateLabel is v, a tick of a Date scale stepping by step, as the axis
// writes it: as short as the step allows, since a reader takes what does
// not change from the tick beside it. A year's step writes the year,
// "2024"; a month's the month, "Mar", and January's the year it begins,
// "2024"; a day's the day, "2 Mar"; anything shorter the time, "15:30" --
// "15:30:45" under a minute -- and midnight's the day it begins.
//
// first is the axis's first tick, which writes what the others leave out
// -- "Mar 2024", "2 Mar 2024", "2 Mar 15:30" -- so a reader does not have
// to know from elsewhere which year, or day, the chart is of.
func dateLabel(v, step float64, first bool) string {
	if v < firstDate || v > lastDate {
		// No date: written as the number it is, rather than as whatever
		// an int64 overflowed into.
		return strconv.FormatFloat(v, 'g', 6, 64)
	}
	t := time.Unix(int64(math.Round(v)), 0).UTC()
	midnight := t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0
	switch {
	case step >= year*0.9:
		return t.Format("2006")
	case step >= month*0.9:
		if first && t.Month() != time.January {
			return t.Format("Jan 2006")
		}
		if t.Month() == time.January {
			return t.Format("2006")
		}
		return t.Format("Jan")
	case step >= day:
		if first {
			return t.Format("2 Jan 2006")
		}
		return t.Format("2 Jan")
	}
	clock := "15:04"
	if step < 60 {
		clock = "15:04:05"
	}
	switch {
	case first:
		return t.Format("2 Jan " + clock)
	case midnight:
		return t.Format("2 Jan")
	}
	return t.Format(clock)
}

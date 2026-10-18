package chart

import (
	"fmt"
	"math"
	"sort"
)

// RobustQuantile is how much of the readings at either end RobustRange
// leaves outside the range: a hundredth at the bottom and a hundredth at
// the top.
//
// A power meter reads zero for every second the rider coasts and spikes to
// three times the ride's average on a sprint, and a scale stretched to
// include both draws the hour between them as a flat band a few pixels
// tall. Which is exactly what the first prototype of this chart did. A
// hundredth is enough to leave dropouts and spikes out while keeping
// anything held for longer than a minute of an hour-and-a-half's readings.
const RobustQuantile = 0.01

// RobustRange is the span of segments' readings with RobustQuantile of them
// left out at either end, and whether there are any readings at all. A
// reading that is not a finite number is no reading, as in Segments.
func RobustRange(segments [][]Point) (lo, hi float64, ok bool) {
	var ys []float64
	for _, seg := range segments {
		for _, p := range seg {
			if finite(p.Y) {
				ys = append(ys, p.Y)
			}
		}
	}
	if len(ys) == 0 {
		return 0, 0, false
	}
	sort.Float64s(ys)
	at := func(q float64) float64 { return ys[int(math.Round(q*float64(len(ys)-1)))] }
	return at(RobustQuantile), at(1 - RobustQuantile), true
}

// Kind is what a scale's numbers are, which decides the steps it ticks at
// and how it writes them.
type Kind int

const (
	// Number is a quantity, ticked at 1, 2 or 5 times a power of ten.
	Number Kind = iota
	// Clock is a duration in seconds, ticked at the steps a clock reads in
	// -- 15 s, 5 min, 1 h -- and written as one: "1:05", "1:30:00". A pace
	// is one too: seconds a kilometre.
	Clock
	// Date is a moment, in seconds since 1970 began -- Unix time -- read
	// in UTC: ticked at the steps a calendar reads in, from seconds to
	// days, the first of a month, of a quarter, of a year, and written as
	// a calendar does: "15:30", "2 Mar", "Mar", "2024". See Scale.Ticks
	// and Scale.Label. Only the years 1 to 9999 are written as dates; a
	// scale reaching past them has no ticks.
	Date
)

// Scale is one axis of a chart: the span it shows, the step between its
// ticks, and whether it runs backwards.
type Scale struct {
	// Min and Max are the axis's ends: each on a tick for a scale from
	// NewScale, wherever the readings end for one from ExactScale.
	Min, Max float64
	// Step is the distance between ticks.
	Step float64
	Kind Kind
	// Inverted puts Min at the top or the right: a pace's axis, on which
	// a smaller number is faster and should read as higher.
	Inverted bool
}

// NewScale is a scale for readings from lo to hi -- a chart's y axis --
// with at most maxTicks steps between its ends, widened at either end to
// the next tick.
//
// The ends are rounded out to ticks because an axis that ends between them
// leaves its reader estimating the last stretch, and the readings are first
// given a fiftieth of their span of room either side, so the line does not
// run along the frame at its highest and lowest. That room never takes a
// scale of readings that are none of them negative below zero: a heart
// rate's axis marked -20 is a claim about something that cannot be. A span
// of nothing -- a reading that never changed -- is opened to a twentieth
// of the reading either side, or one, so the line is drawn level in the
// middle rather than on a scale of zero height.
//
// lo and hi are held within a quarter of the largest float64 either side of
// zero, and a NaN is taken for zero, so the
// scale's own arithmetic stays finite; a reading beyond it is drawn at the
// axis's end, as any reading outside a scale is.
func NewScale(lo, hi float64, maxTicks int, kind Kind) Scale {
	lo, hi = held(lo), held(hi)
	if hi < lo {
		lo, hi = hi, lo
	}
	nonNegative := lo >= 0
	if hi-lo <= 0 {
		open := max(math.Abs(lo)*0.05, 1)
		lo, hi = lo-open, hi+open
	}
	pad := (hi - lo) / 50
	lo, hi = lo-pad, hi+pad
	if nonNegative {
		lo = max(lo, 0)
	}
	step := niceStep(max((hi-lo)/float64(max(maxTicks, 1)), finestStep(lo, hi)), kind)
	return Scale{
		Min:  math.Floor(lo/step) * step,
		Max:  math.Ceil(hi/step) * step,
		Step: step,
		Kind: kind,
	}
}

// ExactScale is a scale from exactly lo to hi -- a chart's x axis -- ticked
// at round steps within it, at most maxTicks of them.
//
// The x axis is not rounded out as the y axis is: it spans the series,
// and an hour and seventeen minutes drawn on an axis to an hour and a half
// is a chart with an eighth of it empty for no reason. Its ends need no
// tick, since a reader knows where the series began and the last tick is
// near enough its end. Its ends are held as NewScale's are.
func ExactScale(lo, hi float64, maxTicks int, kind Kind) Scale {
	lo, hi = held(lo), held(hi)
	if hi < lo {
		lo, hi = hi, lo
	}
	if hi <= lo {
		// One past lo, or for a lo so large that one past it is lo itself
		// -- a timestamp in nanoseconds -- the least step it can tick at.
		hi = lo + max(1, finestStep(lo, lo))
	}
	step := niceStep(max((hi-lo)/float64(max(maxTicks, 1)), finestStep(lo, hi)), kind)
	return Scale{Min: lo, Max: hi, Step: step, Kind: kind}
}

// limit is the largest magnitude a scale's ends are held to: a quarter of
// the largest float64, so that the span between them, and a fiftieth of it
// either side, are numbers too. Readings of 1e308 and -1e308 had a span of
// +Inf, and every end and step from it was NaN: three different readings
// drawn as one level line along the top.
const limit = math.MaxFloat64 / 4

// held is v within ±limit, and a NaN as zero.
func held(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return min(max(v, -limit), limit)
}

// finite says v is a number: neither NaN nor an infinity.
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// finestStep is the least step a scale from lo to hi may tick at: 2^-40 of
// its larger end's magnitude. A float64 has 52 bits of fraction, so ticks
// that fine are still distinct numbers, still a whole count of steps from
// nought, and written with no more digits than the number holds. Finer --
// readings of nanoseconds since 1970 a few hundred apart -- and the ticks
// were the same number many times over, written as ".2, .5, .6" where the
// step was 0.2, and counting them never ended, since past 2^53 adding one
// to a float64 does not change it.
func finestStep(lo, hi float64) float64 {
	return max(math.Abs(lo), math.Abs(hi)) * 0x1p-40
}

// maxTicks is the most ticks a scale gives. A scale from NewScale or
// ExactScale has about the number it was asked for; this bounds one put
// together by hand with a Step too small for its span.
const maxTicks = 10000

// clockSteps are the steps a clock's axis ticks at, in seconds, up to a day.
var clockSteps = []float64{1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 10800, 21600, 43200, 86400}

// niceStep is the smallest round step of kind at least raw.
func niceStep(raw float64, kind Kind) float64 {
	if kind == Date {
		return dateStep(raw)
	}
	if kind == Clock {
		for _, s := range clockSteps {
			if s >= raw {
				return s
			}
		}
		return niceStep(raw/86400, Number) * 86400
	}
	if raw <= 0 {
		return 1
	}
	exp := math.Pow(10, math.Floor(math.Log10(raw)))
	for _, m := range []float64{1, 2, 5, 10} {
		if m*exp >= raw*(1-1e-9) {
			return m * exp
		}
	}
	return 10 * exp
}

// Ticks are the values the scale marks: every multiple of Step from Min to
// Max -- or, for a Date scale whose step is ten days or more, the calendar's
// own marks, which are not evenly spaced: a month is 28 to 31 days. A scale
// that would have more than ten thousand ticks, or whose numbers are not
// finite, has none.
func (s Scale) Ticks() []float64 {
	if !(s.Step > 0) || !(s.Max >= s.Min) || !finite(s.Min) || !finite(s.Max) {
		return nil
	}
	if s.Kind == Date {
		return dateTicks(s.Min, s.Max, s.Step)
	}
	return multiples(math.Ceil(s.Min/s.Step-1e-9), math.Floor(s.Max/s.Step+1e-9), s.Step)
}

// multiples are i times step for every whole i from first to last, or none
// where there would be more than maxTicks. Each is a multiple of step
// rather than the last plus step, so a step of 0.1 does not drift into
// 0.30000000000000004 by the third, and i is counted from first by an
// integer, since past 2^53 a float64 counter stops counting.
func multiples(first, last, step float64) []float64 {
	n := last - first + 1
	if !(n >= 1) || n > maxTicks {
		return nil
	}
	ticks := make([]float64, 0, int(n))
	for k := range int(n) {
		ticks = append(ticks, (first+float64(k))*step)
	}
	return ticks
}

// Frac is where v lies along the scale, from 0 at the bottom or the left to
// 1 at the top or the right, and whether it lay outside the scale and was
// held at its edge. A NaN, which lies nowhere, is held at the bottom or the
// left.
func (s Scale) Frac(v float64) (f float64, clipped bool) {
	if math.IsNaN(v) {
		return 0, true
	}
	f = (v - s.Min) / (s.Max - s.Min)
	if math.IsNaN(f) {
		// A scale of no span, or one whose ends are not numbers.
		return 0, true
	}
	if s.Inverted {
		f = 1 - f
	}
	if f < 0 || f > 1 {
		return min(max(f, 0), 1), true
	}
	return f, false
}

// Label is v as the scale writes it: a number with as many decimals as its
// step needs -- in thousands, millions or more, "250k", "4M", where the
// step is a hundred thousand or more, and as a power of ten, "4.5e+17",
// past a thousand trillion -- or a clock -- minutes and seconds, or hours and minutes on an
// axis that reaches an hour, with seconds where the step is under a minute
// -- or a date, as briefly as the tick before it allows: "2 Mar", then
// "6 Mar"; "Mar 2024", then "Apr"; the first tick of the axis in full.
func (s Scale) Label(v float64) string {
	if s.Kind == Clock {
		return clockLabel(v, s.Max >= 3600, s.Step < 60)
	}
	if s.Kind == Date {
		ticks := s.Ticks()
		return dateLabel(v, s.Step, len(ticks) > 0 && v == ticks[0])
	}
	if v == 0 {
		v = 0 // no "-0"
	}
	// An axis stepping by a hundred thousand or more writes its numbers in
	// thousands, millions, billions -- "250k", "4M" -- since a column of
	// "4000000" is a gutter as wide as the zeros and as slow to read. The
	// unit is the axis's largest number's, so every label on it has the
	// same: "0.5M, 1.0M, 1.5M", not "500k, 1.0M".
	top := max(math.Abs(s.Min), math.Abs(s.Max), math.Abs(v))
	if top >= 1e15 && v != 0 {
		// Past thousands of trillions no suffix is one a reader knows, and
		// written in full a number of 1e300 is a gutter three hundred
		// digits wide: write it as a power of ten, with the digits that
		// tell one tick from the next.
		digits := int(math.Floor(math.Log10(top))-math.Floor(math.Log10(s.Step))) + 1
		return fmt.Sprintf("%.*e", min(max(digits-1, 0), 14), v)
	}
	for _, u := range []struct {
		div    float64
		suffix string
	}{{1e12, "T"}, {1e9, "G"}, {1e6, "M"}, {1e3, "k"}} {
		if s.Step >= 1e5 && top >= u.div && v != 0 {
			step := s.Step / u.div
			decimals := max(0, int(-math.Floor(math.Log10(step)+1e-9)))
			return fmt.Sprintf("%.*f%s", decimals, v/u.div, u.suffix)
		}
	}
	decimals := max(0, int(-math.Floor(math.Log10(s.Step)+1e-9)))
	return fmt.Sprintf("%.*f", decimals, v)
}

func clockLabel(v float64, hours, seconds bool) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	total := int(math.Round(v))
	h, m, sec := total/3600, total%3600/60, total%60
	switch {
	case hours && seconds:
		return fmt.Sprintf("%s%d:%02d:%02d", sign, h, m, sec)
	case hours:
		return fmt.Sprintf("%s%d:%02d", sign, h, m)
	}
	return fmt.Sprintf("%s%d:%02d", sign, total/60, sec)
}

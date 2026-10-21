package chart

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"
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
	// Log is a quantity drawn by its logarithm, so that each power of ten
	// takes the same height: latency, a price over decades, anything that
	// grows by multiplying, where a linear axis draws all but the largest
	// readings as a line along the bottom. It is for a strip's Y scale --
	// Bin divides the x axis evenly -- and its Min must be above zero.
	//
	// Its Step is in powers of ten: 1 ticks at every power, 2 at every
	// other, and so on; a Step below 1 ticks at 1, 2 and 5 times every
	// power, which are near enough a third of a power apart. Labels are
	// written as briefly as the tick allows: "0.01", "5", "200", "1k",
	// "50M", "1e+18".
	//
	// A reading at or below zero has no place on it, and is drawn at the
	// bottom as any reading below a scale is: a reading drawn where it is
	// not. Leave such a reading out -- Point.Ok false -- so that it is a
	// gap, as plot does, and say so.
	Log
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
// of the reading either side, or one, so the line is drawn level rather
// than on a scale of zero height: in the middle where the scale opens both
// ways, and nearer the bottom where it cannot go below zero -- a reading of
// nought all the way, zero power on a walk, on the axis itself.
//
// lo and hi are held within a quarter of the largest float64 either side of
// zero, and a NaN is taken for zero, so the
// scale's own arithmetic stays finite; a reading beyond it is drawn at the
// axis's end, as any reading outside a scale is.
//
// A Log scale is rounded out to its ticks too, its room either side a
// fiftieth of its span in powers of ten, and a span of nothing is opened
// to half the reading below and half again above. Its lo must be above
// zero: a lo that is not makes a scale Check refuses, saying so. It
// reaches no lower than 1e-300.
func NewScale(lo, hi float64, maxTicks int, kind Kind) Scale {
	lo, hi = held(lo), held(hi)
	if hi < lo {
		lo, hi = hi, lo
	}
	if kind == Log {
		return logScale(lo, hi, maxTicks)
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
	if kind == Log {
		if hi <= lo {
			hi = lo * 10
		}
		return Scale{Min: lo, Max: hi, Step: logStep(math.Log10(hi)-math.Log10(lo), maxTicks), Kind: Log}
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
	if s.Kind == Log {
		return logTicks(s.Min, s.Max, s.Step)
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

// Ends are the values at the scale's two ends, the first at the bottom or
// the left: Min and Max, or Max and Min where the scale is Inverted. Bin
// takes them so, to bin an inverted x axis from its right.
func (s Scale) Ends() (first, last float64) {
	if s.Inverted {
		return s.Max, s.Min
	}
	return s.Min, s.Max
}

// ticksLeftToRight are the ticks in the order they are drawn, from the
// bottom or the left: reversed, on an inverted scale.
func (s Scale) ticksLeftToRight() []float64 {
	ticks := s.Ticks()
	if s.Inverted {
		slices.Reverse(ticks)
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
	switch {
	case s.Kind != Log:
		f = (v - s.Min) / (s.Max - s.Min)
	case v > 0:
		f = (math.Log10(v) - math.Log10(s.Min)) / (math.Log10(s.Max) - math.Log10(s.Min))
	default:
		f = math.Inf(-1) // at or below zero: below every log scale
	}
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
// past a thousand trillion -- or on a Log scale as briefly as the tick allows,
// "0.02", "500", "2k", "1e+18" -- or a clock -- minutes and seconds, or hours and minutes on an
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
	if s.Kind == Log {
		return logLabel(v)
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

// logScale is NewScale's scale of kind Log, for lo no greater than hi. Its
// arithmetic is in powers of ten, so readings from 1e-300 to 1e300 have a
// span of 600 rather than one that overflows.
func logScale(lo, hi float64, maxTicks int) Scale {
	if !(lo > 0) {
		return Scale{Min: lo, Max: hi, Step: 1, Kind: Log} // which fault refuses
	}
	a, b := max(math.Log10(lo), logFloor), max(math.Log10(hi), logFloor)
	if b <= a {
		open := math.Log10(1.5)
		a, b = a-open, b+open
	}
	pad := (b - a) / 50
	a, b = a-pad, b+pad
	step := logStep(b-a, maxTicks)
	s := Scale{Step: step, Kind: Log}
	if step >= 1 {
		// Out to whole powers of ten, not to whole steps: readings from 1
		// to 1e30 ticked every ten powers were drawn from 1e-10 to 1e40.
		s.Min, s.Max = pow10(math.Floor(a)), pow10(math.Ceil(b))
	} else {
		s.Min, s.Max = below125(a), above125(b)
	}
	return s
}

// logFloor is the least power of ten a Log scale reaches down to, 1e-300:
// a reading below it is drawn at the bottom, as one above limit is drawn
// at the top. The float64s below 1e-308 hold fewer digits the smaller
// they are, and at the least of them 1, 2 and 5 times a power of ten were
// one number: a scale from 1e-323 to 1e-323.
const logFloor = -300

// pow10 is ten to the e, held within the float64s, past the largest at
// the largest.
func pow10(e float64) float64 {
	return min(math.Pow(10, e), math.MaxFloat64)
}

// logStep is the Step of a Log scale powers powers of ten wide with at
// most maxTicks ticks: 1, 2 and 5 times each power of ten where there is
// room for three a power, otherwise every one, two, five or ten powers or
// more.
func logStep(powers float64, maxTicks int) float64 {
	if powers*3 <= float64(max(maxTicks, 1)) {
		return 1.0 / 3
	}
	return max(1, niceStep(powers/float64(max(maxTicks, 1)), Number))
}

// mantissas are the multiples of each power of ten a Log scale with a
// Step below 1 ticks at.
var mantissas = []float64{1, 2, 5}

// below125 is the largest of 1, 2 or 5 times a power of ten no greater
// than ten to the e, and above125 the least no less than it, each held as
// pow10 holds.
func below125(e float64) float64 {
	p := math.Floor(e)
	for i := len(mantissas) - 1; i >= 0; i-- {
		if m := math.Log10(mantissas[i]); p+m <= e+1e-9 {
			return pow10(p) * mantissas[i]
		}
	}
	return pow10(p)
}

func above125(e float64) float64 {
	p := math.Floor(e)
	for _, m := range mantissas {
		if p+math.Log10(m) >= e-1e-9 {
			return min(pow10(p)*m, math.MaxFloat64)
		}
	}
	return pow10(p + 1)
}

// logTicks are a Log scale's ticks from lo to hi: see Log. None where lo is
// not above zero, or there would be more than maxTicks.
func logTicks(lo, hi, step float64) []float64 {
	if !(lo > 0) {
		return nil
	}
	from, to := math.Log10(lo), math.Log10(hi)
	if to-from > maxTicks {
		return nil
	}
	var ticks []float64
	if step >= 1 {
		for k := math.Ceil(from/step - 1e-9); k <= math.Floor(to/step+1e-9); k++ {
			ticks = append(ticks, math.Pow(10, k*step))
		}
		return ticks
	}
	for e := math.Floor(from); e <= math.Floor(to); e++ {
		p := math.Pow(10, e)
		for _, m := range mantissas {
			if v := m * p; v >= lo*(1-1e-9) && v <= hi*(1+1e-9) {
				ticks = append(ticks, v)
			}
		}
	}
	return ticks
}

// logLabel is v as a Log scale writes it: to three figures at the most,
// in thousands, millions, billions or trillions -- "2k", "50M" -- from a
// thousand, and as a power of ten from a thousand trillion, or below a
// ten-thousandth, where the zeros are what make it long.
func logLabel(v float64) string {
	a := math.Abs(v)
	switch {
	case a == 0:
		return "0"
	case a >= 1e15 || a < 1e-4:
		// To three figures, as the others: a tick near the largest
		// float64, a power of ten times 5 a hair off, wrote all seventeen.
		m, e, _ := strings.Cut(strconv.FormatFloat(v, 'e', 2, 64), "e")
		if strings.Contains(m, ".") {
			m = strings.TrimRight(strings.TrimRight(m, "0"), ".")
		}
		return m + "e" + e
	}
	for _, u := range []struct {
		div    float64
		suffix string
	}{{1e12, "T"}, {1e9, "G"}, {1e6, "M"}, {1e3, "k"}} {
		if a >= u.div {
			return figures(v/u.div) + u.suffix
		}
	}
	return figures(v)
}

// figures is v to three significant figures at the most, with no zeros at
// its end: "2", "31.6", "0.05".
func figures(v float64) string {
	s, _ := strconv.ParseFloat(strconv.FormatFloat(v, 'g', 3, 64), 64)
	return strconv.FormatFloat(s, 'f', -1, 64)
}

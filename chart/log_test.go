package chart

import (
	"math"
	"strings"
	"testing"
	"time"
)

// On a Log scale each power of ten takes the same height, a reading at or
// below zero is held at the bottom -- below every log scale -- and an
// inverted one turns round as any scale does.
func TestLogFrac(t *testing.T) {
	s := Scale{Min: 1, Max: 1e6, Step: 1, Kind: Log}
	for k := range 7 {
		if f, clipped := s.Frac(math.Pow(10, float64(k))); math.Abs(f-float64(k)/6) > 1e-12 || clipped {
			t.Errorf("1e%d at %v (clipped %v), want %v", k, f, clipped, float64(k)/6)
		}
	}
	for _, v := range []float64{0, -5} {
		if f, clipped := s.Frac(v); f != 0 || !clipped {
			t.Errorf("%v at %v (clipped %v), want held at the bottom", v, f, clipped)
		}
	}
	s.Inverted = true
	if f, _ := s.Frac(1e6); f != 0 {
		t.Errorf("the top of an inverted log scale at %v, want 0", f)
	}
	if f, _ := s.Frac(0); f != 1 {
		t.Errorf("zero on an inverted log scale at %v, want held at the top, its low end", f)
	}
}

// A Log scale is ticked at 1, 2 and 5 times each power of ten where there
// is room, at every power -- or every few -- where there is not, and each
// tick written as briefly as it allows.
func TestLogTicksAndLabels(t *testing.T) {
	for _, c := range []struct {
		lo, hi float64
		want   string
	}{
		{0.003, 0.2, "0.002 0.005 0.01 0.02 0.05 0.1 0.2 0.5"},
		{5, 5, "2 5 10"},
		{1, 1.3e6, "0.1 1 10 100 1k 10k 100k 1M 10M"},
		{1, 1e30, "1 100k 10G 1e+15 1e+20 1e+25 1e+30"},
		{2e307, 4e307, "1e+307 2e+307 5e+307"},
		{3e-9, 4e-7, "2e-09 5e-09 1e-08 2e-08 5e-08 1e-07 2e-07 5e-07"},
	} {
		s := NewScale(c.lo, c.hi, 8, Log)
		var labels []string
		for _, v := range s.Ticks() {
			labels = append(labels, s.Label(v))
		}
		if got := strings.Join(labels, " "); got != c.want {
			t.Errorf("%v to %v: %s, want %s", c.lo, c.hi, got, c.want)
		}
		if !(s.Min <= c.lo && s.Max >= c.hi) {
			t.Errorf("%v to %v: a scale from %v to %v, short of the readings", c.lo, c.hi, s.Min, s.Max)
		}
	}
}

// Whatever positive readings a Log scale is made for -- from the least
// float64 to the largest -- its ends are positive numbers, it has ticks,
// and every reading from 1e-300 up to limit is on it.
func TestLogScaleExtremes(t *testing.T) {
	tiny, huge := math.SmallestNonzeroFloat64, math.MaxFloat64
	for _, r := range [][2]float64{{tiny, huge}, {tiny, tiny}, {huge, huge}, {1e-300, 1e300}, {1, 1}, {7, 3}} {
		for _, max := range []int{1, 6, 100} {
			s := NewScale(r[0], r[1], max, Log)
			if err := s.fault(); err != nil {
				t.Errorf("%v: %v", r, err)
				continue
			}
			if !(s.Min > 0) || !finite(s.Max) || len(s.Ticks()) == 0 {
				t.Errorf("%v, %d ticks: from %v to %v, ticks %v", r, max, s.Min, s.Max, s.Ticks())
			}
			for _, v := range r {
				if _, clipped := s.Frac(v); clipped && v <= limit && v >= 1e-300 {
					t.Errorf("%v, %d ticks: %v is off a scale from %v to %v", r, max, v, s.Min, s.Max)
				}
			}
		}
	}
}

// A Log scale's Min must be above zero, and a Log scale is for a strip's
// Y: Check refuses either otherwise, saying why, rather than drawing every
// reading at an edge.
func TestLogScaleRefused(t *testing.T) {
	c := ramp(1000, 1000)
	c.Strips[0].Y = NewScale(0, 100, 4, Log)
	if err := c.Check(); err == nil || !strings.Contains(err.Error(), "above zero") {
		t.Errorf("a log scale from 0: %v", err)
	}
	c = ramp(1000, 1000)
	c.X = Scale{Min: 1, Max: 100, Step: 1, Kind: Log}
	if err := c.Check(); err == nil || !strings.Contains(err.Error(), "Y scale") {
		t.Errorf("a log X scale: %v", err)
	}
}

// A reading that multiplies by the same factor each second is a straight
// line on a log scale: the dots of each column rise by the same number of
// rows, give or take one, from the bottom left to the top right.
func TestBrailleLogIsStraight(t *testing.T) {
	var ps []Point
	for i := range 101 {
		ps = append(ps, Point{X: float64(i), T: float64(i), Y: math.Pow(10, float64(i)/20), Ok: true})
	}
	c := Chart{X: ExactScale(0, 100, 4, Number), Strips: []Strip{{Y: Scale{Min: 1, Max: 1e5, Step: 1, Kind: Log}, Segments: Segments(ps, time.Second)}}}
	lines := c.Braille(60, 24)
	// The dot rows, down the page, of the first dot in each column.
	var tops []int
	for col := 0; ; col++ {
		found, ok := -1, false
		for y, l := range lines {
			r := []rune(l)
			x := strings.IndexRune(l, '┤')
			if x < 0 {
				x = strings.IndexRune(l, '│')
			}
			if x < 0 || len([]rune(l[:x]))+1+col >= len(r) {
				continue
			}
			ok = true
			if d := r[len([]rune(l[:x]))+1+col]; d > 0x2800 && d < 0x2900 {
				found = y
				break
			}
		}
		if !ok {
			break
		}
		if found >= 0 {
			tops = append(tops, found)
		}
	}
	if len(tops) < 40 {
		t.Fatalf("dots in %d columns:\n%s", len(tops), strings.Join(lines, "\n"))
	}
	// Rows fall from the bottom to the top at an even rate: each column's
	// row within one of the straight line between the first and the last.
	first, last := tops[0], tops[len(tops)-1]
	for i, y := range tops {
		want := float64(first) + float64(last-first)*float64(i)/float64(len(tops)-1)
		if math.Abs(float64(y)-want) > 1 {
			t.Fatalf("column %d's dot in row %d, %0.1f off a straight line:\n%s", i, y, math.Abs(float64(y)-want), strings.Join(lines, "\n"))
		}
	}
	if !(first > last) {
		t.Errorf("the line does not rise: row %d to row %d", first, last)
	}
}

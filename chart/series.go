// Package chart draws a chart of a series of readings -- one or several,
// stacked -- against time or any other x axis, in the terminal or as a
// picture, without a plotting library's idea of what a chart should claim.
//
// A chart is drawn at very different resolutions -- a picture some thousand
// pixels wide, a terminal's braille dots two to a character -- and the
// decisions that make it truthful must not differ between them. So they are
// made here, once, and a renderer only turns the result into marks:
//
//   - which readings a line may join (Segments): never across a stretch
//     without readings long enough to show, because a line drawn there
//     claims readings nobody took -- and a missing reading is never a zero;
//   - what each column of the drawing shows (Bin), however many readings
//     fall in it;
//   - the scale (RobustRange, NewScale), set from where most readings lie
//     so that a dropout or a spike does not flatten the rest of the trace;
//   - the ticks a reader measures it by, at round numbers or round times.
//
// Braille draws a Chart as text, which any terminal shows. The picture
// package draws it as an image, and the terminal package shows that image
// in a terminal that can show one.
//
// The package imports only the standard library: a program that draws
// charts in text links nothing else.
package chart

import (
	"math"
	"time"
)

// Point is one sample of a series: where it lies on the x axis, when it was
// taken, and its reading -- or that it has none.
//
// X and T are separate because the axis is not always time. A chart of a
// run against distance still smooths over time and still breaks its line
// where the recording stopped, and neither can be read from distance:
// standing still covers none. Where the axis is time, X and T are the
// same.
type Point struct {
	// X is the point's place on the x axis, in the axis's own units.
	X float64
	// T is when the sample was taken, in seconds from any fixed start: it
	// orders the points, and measures gaps and smoothing windows.
	T float64
	// Y is the reading, meaningful only where Ok.
	Y float64
	// Ok says the sample carries the reading. A sample without it is a
	// break in the line, never a zero.
	Ok bool
}

// Segments splits points, in time order, into the runs a line may join:
// the readings, broken wherever two in a row are more than maxGap apart in
// time.
//
// A sample without the reading is time without one, no different from time
// without a sample: the watch that dropped its heart rate strap for a
// second and the watch that recorded nothing for a second say the same
// thing, and both are a break only when they last long enough to be one. A
// line straight across ten minutes nobody measured is a confident claim
// about them, which the chart must not make; across one second, at a
// resolution where a second is a fraction of a pixel, a break would claim
// more than the data does, and shred a line with occasional dropouts into
// pieces. So the tolerance is the caller's, and a caller picks it from the
// recording's own rhythm and the chart's resolution. A maxGap of zero or
// less joins only readings recorded at the same instant.
//
// A sample whose X, T or reading is not a finite number -- a NaN, an
// infinity -- is taken as one without the reading, though Ok says it has
// one. Neither has a place on an axis, and an infinity in a smoothing
// window makes every reading the window reaches NaN.
func Segments(points []Point, maxGap time.Duration) [][]Point {
	var out [][]Point
	var run []Point
	gap := maxGap.Seconds()
	for _, p := range points {
		if !p.Ok || !finite(p.X) || !finite(p.T) || !finite(p.Y) {
			continue
		}
		if len(run) > 0 && p.T-run[len(run)-1].T > max(gap, 0) {
			out = append(out, run)
			run = nil
		}
		run = append(run, p)
	}
	if len(run) > 0 {
		out = append(out, run)
	}
	return out
}

// Smooth is each segment with every reading replaced by the mean of the
// readings within half of window either side of it in time, never reaching
// into another segment. A window of zero or less smooths nothing.
//
// It is centred, so a step in the readings stays where it happened rather
// than trailing behind it, and it is by time rather than by sample count, so
// a recording at one sample a second and one at one every few smooth alike.
func Smooth(segments [][]Point, window time.Duration) [][]Point {
	half := window.Seconds() / 2
	if half <= 0 {
		return segments
	}
	out := make([][]Point, len(segments))
	for i, seg := range segments {
		smoothed := make([]Point, len(seg))
		// A window sliding along the segment: lo and hi bound the samples
		// within half either side of the current one, and sum their
		// readings, so the whole segment is one pass.
		lo, hi, sum := 0, 0, 0.0
		for j, p := range seg {
			for hi < len(seg) && seg[hi].T <= p.T+half {
				sum += seg[hi].Y
				hi++
			}
			stale := false
			for seg[lo].T < p.T-half {
				sum -= seg[lo].Y
				// A reading far larger than what is left cancels it to
				// noise: 1e17 out of a sum of ones left -0.5 behind.
				stale = stale || math.Abs(seg[lo].Y) > 1024*math.Abs(sum)
				lo++
			}
			if stale || math.IsInf(sum, 0) {
				sum = 0
				for _, q := range seg[lo:hi] {
					sum += q.Y
				}
			}
			p.Y = sum / float64(hi-lo)
			if math.IsInf(sum, 0) {
				// Readings near the largest float64 overflow a sum of
				// them: their mean, a share of each, does not.
				p.Y = 0
				for _, q := range seg[lo:hi] {
					p.Y += q.Y / float64(hi-lo)
				}
			}
			smoothed[j] = p
		}
		out[i] = smoothed
	}
	return out
}

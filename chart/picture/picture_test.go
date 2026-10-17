package picture

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/wisborg/output/chart"
)

// ramp is a chart of a reading rising from 0 to 100 over 100 s, a reading a
// second, without readings from gapFrom to gapTo seconds.
func ramp(gapFrom, gapTo float64) chart.Chart {
	var ps []chart.Point
	for i := range 101 {
		t := float64(i)
		ps = append(ps, chart.Point{X: t, T: t, Y: t, Ok: t < gapFrom || t >= gapTo})
	}
	return chart.Chart{
		Title: "ramp",
		X:     chart.ExactScale(0, 100, 5, chart.Clock),
		Strips: []chart.Strip{{Title: "ramp", Y: chart.NewScale(0, 100, 4, chart.Number),
			Segments: chart.Segments(ps, time.Second)}},
	}
}

// stack is the unbroken ramp over one with a gap.
func stack(gapFrom, gapTo float64) chart.Chart {
	c := ramp(1000, 1000)
	gapped := ramp(gapFrom, gapTo).Strips[0]
	gapped.Title = "gapped"
	c.Strips = append(c.Strips, gapped)
	return c
}

func chartImage(t *testing.T, c chart.Chart, w, h int) *image.RGBA {
	t.Helper()
	img, err := Draw(c, w, h, Options{})
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// near says a and b are within a few levels of each other in every
// channel: the line's edges are blended into what is behind them.
func near(a, b color.Color, tol int) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	d := func(x, y uint32) bool { return int(x>>8)-int(y>>8) <= tol && int(y>>8)-int(x>>8) <= tol }
	return d(ar, br) && d(ag, bg) && d(ab, bb)
}

// accentIn counts the pixels of the line's colour in the columns from x0 up
// to x1.
func accentIn(img *image.RGBA, x0, x1 int) int {
	return accentWithin(img, x0, x1, 0, img.Bounds().Dy())
}

// accentWithin counts them in the rows from y0 up to y1 too.
func accentWithin(img *image.RGBA, x0, x1, y0, y1 int) int {
	n := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if near(img.At(x, y), Light.Line, 12) {
				n++
			}
		}
	}
	return n
}

// The picture is the size asked for, on the theme's background, with the
// line drawn in its accent.
func TestImageDrawsTheLine(t *testing.T) {
	img := chartImage(t, ramp(1000, 1000), 800, 300)
	if b := img.Bounds(); b.Dx() != 800 || b.Dy() != 300 {
		t.Fatalf("a picture %v, want 800 by 300", b)
	}
	if !near(img.At(799, 299), Light.Background, 0) {
		t.Errorf("the corner is %v, not the background", img.At(799, 299))
	}
	if accentIn(img, 0, 800) < 300 {
		t.Error("the line is not drawn")
	}
}

// A stretch without readings has no line and no band in it: not a line
// across, and nothing at the bottom where a zero would be.
func TestImageLeavesAGapEmpty(t *testing.T) {
	whole := chartImage(t, ramp(1000, 1000), 800, 300)
	gapped := chartImage(t, ramp(40, 60), 800, 300)
	// The plot spans most of the picture; 45 to 55 s of 100 is well inside
	// the gap whatever its margins.
	x0, x1 := 800*47/100, 800*53/100
	if accentIn(whole, x0, x1) == 0 {
		t.Fatal("the unbroken ramp draws nothing where the gap would be; the test is measuring the wrong place")
	}
	if n := accentIn(gapped, x0, x1); n != 0 {
		t.Errorf("%d pixels of line drawn where there were no readings", n)
	}
}

// A picture too small to hold a plot, a chart with no strips and one with a
// scale left unset are each an error. They were a nil picture and no error,
// which a caller encoding the picture dereferenced, or -- for a forgotten
// scale -- an empty frame passed off as a chart of nothing.
func TestImageRefuses(t *testing.T) {
	noX := ramp(1000, 1000)
	noX.X = chart.Scale{}
	noY := ramp(1000, 1000)
	noY.Strips[0].Y = chart.Scale{}
	for _, c := range []struct {
		name string
		c    chart.Chart
		w, h int
		want string
	}{
		{"too small", ramp(1000, 1000), 40, 30, "too small"},
		{"no size", ramp(1000, 1000), 0, 600, "too small"},
		{"no strips", chart.Chart{X: chart.ExactScale(0, 1, 5, chart.Number)}, 800, 600, "no strips"},
		{"no X scale", noX, 800, 600, "X scale"},
		{"no Y scale", noY, 800, 600, "Y scale"},
	} {
		img, err := Draw(c.c, c.w, c.h, Options{})
		if err == nil || !strings.Contains(err.Error(), c.want) || img != nil {
			t.Errorf("%s: %v, %v; want no picture and an error saying %q", c.name, img != nil, err, c.want)
		}
	}
	if lines := noY.Braille(80, 20); lines != nil {
		t.Errorf("braille drew a chart with no Y scale:\n%s", strings.Join(lines, "\n"))
	}
}

// A palette that sets some colours takes the rest from Light: one setting
// its line alone filled the picture with a nil background, and panicked.
func TestPartPalette(t *testing.T) {
	blue := color.RGBA{0, 0, 0xFF, 0xFF}
	img, err := Draw(ramp(1000, 1000), 400, 300, Options{Palette: Palette{Line: blue}})
	if err != nil {
		t.Fatal(err)
	}
	if got := img.At(1, 1); !near(got, Light.Background, 2) {
		t.Errorf("the background is %v, want Light's", got)
	}
}

// A faded colour is the colour at its own alpha times the fade: read as
// premultiplied, a translucent colour came out darker than it is.
func TestFadedKeepsTheColour(t *testing.T) {
	half := color.NRGBA{R: 200, G: 100, B: 50, A: 128}
	if got := faded(half, 0.5); got.R != 200 || got.G != 100 || got.B != 50 || got.A != 64 {
		t.Errorf("faded %v by a half: %v", half, got)
	}
}

// A band written To before From is the same stretch, drawn in the picture as
// braille marks it.
func TestReversedBandIsDrawn(t *testing.T) {
	c := ramp(1000, 1000)
	c.BandKinds = []chart.BandKind{{Name: "climbs", Mark: '╱', Colour: color.RGBA{0, 0xC0, 0, 0xFF}}}
	c.Bands = []chart.Band{{From: 60, To: 40}}
	img := chartImage(t, c, 800, 600)
	green := 0
	for x := 0; x < 800; x++ {
		if _, g, _, _ := img.At(x, 300).RGBA(); g>>8 > 0xA0 {
			if r, _, b, _ := img.At(x, 300).RGBA(); r>>8 < 0x60 && b>>8 < 0x60 {
				green++
			}
		}
	}
	if green == 0 {
		t.Error("the reversed band is not drawn")
	}
}

// Two strips are stacked, the upper above the middle of the picture and
// the lower below it, each drawing its own line; a gap in the lower is
// empty there only.
func TestImageStacksStrips(t *testing.T) {
	img := chartImage(t, stack(40, 60), 800, 600)
	x0, x1 := 800*47/100, 800*53/100
	if accentWithin(img, x0, x1, 0, 300) == 0 {
		t.Error("the upper strip draws nothing over the lower strip's gap")
	}
	if accentWithin(chartImage(t, stack(1000, 1000), 800, 600), x0, x1, 300, 600) == 0 {
		t.Fatal("the lower strip draws nothing there even without its gap; the test is measuring the wrong place")
	}
	if n := accentWithin(img, x0, x1, 300, 600); n != 0 {
		t.Errorf("%d pixels of line in the lower strip's gap", n)
	}
}

// A band colours its stretch of the plot from top to bottom, behind the
// line, and nothing outside it; the legend names it, on the title's line.
func TestImageBands(t *testing.T) {
	plain := ramp(1000, 1000)
	banded := ramp(1000, 1000)
	banded.BandKinds = []chart.BandKind{
		{Name: "intervals", Colour: Faded(Light.Line, 0.14)},
		{Name: "climbs", Colour: Faded(Light.Dim, 0.2)},
		{Name: "descents", Colour: Faded(color.RGBA{0x5B, 0x3D, 0xE0, 0xFF}, 0.14)},
	}
	banded.Bands = []chart.Band{{From: 20, To: 30, Kind: 0}, {From: 60, To: 70, Kind: 1}, {From: 80, To: 90, Kind: 2}}
	a, b := chartImage(t, plain, 800, 300), chartImage(t, banded, 800, 300)
	bg := Light.Background
	// Near the top of the plot, where the ramp has not yet risen, at 25 s
	// and 65 s -- each inside its band -- and at 45 s, outside both.
	// The plot runs from the y axis -- the first ink from the left on a
	// row between two of its labels -- to the margin, a text size from the
	// right.
	x := func(sec float64) int {
		for px := range 800 {
			if !near(a.At(px, 120), bg, 0) {
				return px + int(sec/100*float64(800-px-int(TextPx(800, 300, 1))))
			}
		}
		t.Fatal("no y axis")
		return 0
	}
	for _, sec := range []float64{25, 65, 85} {
		if !near(a.At(x(sec), 60), bg, 0) {
			t.Fatalf("the chart without bands is not background at %.0f s; the test is measuring the wrong place", sec)
		}
		if near(b.At(x(sec), 60), bg, 2) {
			t.Errorf("no band at %.0f s", sec)
		}
		if near(b.At(x(sec), 250), bg, 2) {
			t.Errorf("the band at %.0f s does not reach the bottom of the plot", sec)
		}
	}
	for _, pair := range [][2]float64{{25, 65}, {25, 85}, {65, 85}} {
		if near(b.At(x(pair[0]), 60), b.At(x(pair[1]), 60), 2) {
			t.Errorf("the bands at %.0f s and %.0f s are alike", pair[0], pair[1])
		}
	}
	if !near(b.At(x(45), 60), bg, 0) {
		t.Error("shaded outside the bands")
	}
	inked := func(img *image.RGBA) int {
		n := 0
		for y := range 25 {
			for px := 600; px < 800; px++ {
				if !near(img.At(px, y), bg, 8) {
					n++
				}
			}
		}
		return n
	}
	if inked(a) != 0 || inked(b) < 50 {
		t.Errorf("the title line's right end has %d pixels inked without bands and %d with, want none and a legend", inked(a), inked(b))
	}
}

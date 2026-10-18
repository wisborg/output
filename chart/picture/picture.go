// Package picture draws a chart.Chart as an image: each strip's line over
// a faint band of each column's lowest to highest reading, on a grid at
// the scales' ticks, the strips one above the other sharing the x axis
// beneath the last, its bands shaded behind them -- labelled as the
// chart's braille is labelled.
//
// It draws with golang.org/x/image alone, through the module's internal
// canvas: its vector rasteriser for the shapes, and Go Mono, embedded, for
// the text, with any fonts the caller passes to fall back on for the
// letters Go Mono lacks. Go Mono is monospaced on purpose: a chart's labels
// are numbers, and in a proportional face their widths differ digit by
// digit.
//
// A program that draws charts in text does not import this package, and so
// does not link golang.org/x/image.
package picture

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"slices"

	"golang.org/x/image/font/opentype"

	"github.com/wisborg/output/chart"
	"github.com/wisborg/output/internal/canvas"
)

// Palette is the colours a picture is drawn in.
type Palette struct {
	// Background fills the picture.
	Background color.Color
	// Foreground is the titles.
	Foreground color.Color
	// Dim is the chrome: the axes, the labels, and -- faded -- the grid.
	Dim color.Color
	// Line is the readings: the line, and -- faded -- the band of each
	// column's lowest to highest behind it.
	Line color.Color
}

// Light is a palette for a page: dark ink on paper, the line in a warm red.
var Light = Palette{
	Background: color.RGBA{0xF5, 0xF4, 0xF1, 0xFF},
	Foreground: color.RGBA{0x16, 0x16, 0x1A, 0xFF},
	Dim:        color.RGBA{0x7A, 0x7A, 0x84, 0xFF},
	Line:       color.RGBA{0xC4, 0x37, 0x14, 0xFF},
}

// Dark is a palette for a dark window: light ink on near-black, the line
// in a bright orange.
var Dark = Palette{
	Background: color.RGBA{0x0E, 0x0E, 0x10, 0xFF},
	Foreground: color.RGBA{0xF2, 0xF2, 0xF4, 0xFF},
	Dim:        color.RGBA{0x8A, 0x8A, 0x96, 0xFF},
	Line:       color.RGBA{0xFF, 0x5A, 0x36, 0xFF},
}

// Options are how a picture is drawn. The zero value is the Light palette
// and Go Mono alone.
type Options struct {
	// Palette is the picture's colours; the zero value is Light.
	Palette Palette
	// Fonts are fonts to fall back on: a letter Go Mono lacks -- it has
	// Latin, Greek and Cyrillic -- is drawn from the first of these that
	// has it.
	Fonts []*opentype.Font
}

// MaxPixels is the most pixels a picture Draw draws may have: 2^26, a
// quarter of a gigabyte of RGBA, 8000 by 8000, as diagram/picture's. A size
// asked for in a flag -- plot --width 100000 --height 100000 -- asked for
// forty gigabytes, and the program swapped for minutes, or died, rather
// than saying why.
const MaxPixels = 1 << 26

// Draw is c drawn as a picture width by height pixels. A chart without
// strips, one chart.Chart.Check refuses, a picture too small to hold a
// plot, and one of more than MaxPixels are each an error, never a nil
// picture or an empty frame.
//
// Text is sized from the picture's width, and its height shared between
// its strips, so that a stack of several reads at the size one does; see
// TextPx. A caller choosing how many ticks a scale gets for a picture of
// a given size spaces them by it.
func Draw(c chart.Chart, width, height int, opts Options) (*image.RGBA, error) {
	n := len(c.Strips)
	if n == 0 {
		return nil, errors.New("picture: a chart with no strips")
	}
	if err := c.Check(); err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("picture: %d by %d pixels is too small for a chart", width, height)
	}
	if float64(width)*float64(height) > MaxPixels {
		return nil, fmt.Errorf("picture: %d by %d pixels is more than %d million", width, height, MaxPixels/1000000)
	}
	pal := opts.Palette.filled()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	cv, err := canvas.New(img, opts.Fonts)
	if err != nil {
		return nil, err
	}
	px := TextPx(width, height, n)

	// The plots' box: room above for the title, to the left for the y
	// labels -- one gutter for every strip, the widest's, so their plots
	// line up -- below for the x labels, and a margin all round.
	margin := px
	gutter := 0.0
	for _, s := range c.Strips {
		for _, v := range s.Y.Ticks() {
			w, _, err := cv.Measure(s.Y.Label(v), px)
			if err != nil {
				return nil, err
			}
			gutter = math.Max(gutter, w)
		}
	}
	all := box{x: margin + gutter + px/2, y: margin + px*2}
	all.w = float64(width) - all.x - margin
	all.h = float64(height) - all.y - margin - px*2
	// Where there are several strips, each is headed by its title, in a
	// gap that clears the lowest y label of the strip above and the
	// highest of its own, each of which reaches half a line beyond its
	// strip.
	head := 0.0
	if n > 1 {
		head = px * 2.6
	}
	stripH := (all.h - float64(n)*head) / float64(n)
	if all.w < 20 || stripH < 20 {
		return nil, fmt.Errorf("picture: %d by %d pixels is too small for a chart", width, height)
	}
	cv.Fill(pal.Background)
	xAt := func(v float64) float64 {
		f, _ := c.X.Frac(v)
		return all.x + f*all.w
	}

	grid := faded(pal.Dim, 0.25)
	thin := math.Max(1, px/16)
	for i, s := range c.Strips {
		b := box{x: all.x, y: all.y + float64(i)*(head+stripH) + head, w: all.w, h: stripH}
		if n > 1 {
			_ = cv.Text(s.Title, b.x, b.y-head/2, 0, canvas.Centre, px, pal.Foreground)
		}
		for _, band := range c.Bands {
			if band.Kind < 0 || band.Kind >= len(c.BandKinds) {
				continue
			}
			from, to := band.Span()
			x0, x1 := math.Max(math.Min(xAt(from), xAt(to)), b.x), math.Min(math.Max(xAt(from), xAt(to)), b.x+b.w)
			if x1 > x0 {
				cv.Rect(x0, b.y, math.Max(x1-x0, 1), b.h, bandColour(c.BandKinds[band.Kind], pal))
			}
		}
		for _, v := range c.X.Ticks() {
			x := xAt(v)
			cv.Polyline([]float64{x, x}, []float64{b.y, b.y + b.h}, thin, grid)
		}
		drawStrip(cv, s, b, c.X, px, thin, grid, pal)
	}

	bottom := all.y + all.h
	right := -math.MaxFloat64 // where the last x label ended
	ticks := c.X.Ticks()
	if c.X.Inverted {
		slices.Reverse(ticks) // in the order they are drawn, left to right
	}
	for _, v := range ticks {
		label := c.X.Label(v)
		w, _, _ := cv.Measure(label, px)
		// Centred on its tick, kept clear of the y labels -- the first
		// starts at the axis rather than under the lowest of them -- and
		// inside the picture, and left out if it would touch the label
		// before it.
		left := math.Min(math.Max(xAt(v)-w/2, all.x), float64(width)-margin-w)
		if left < right+px {
			continue
		}
		_ = cv.Text(label, left, bottom+px*0.9, 0, 0, px, pal.Dim)
		right = left + w
	}
	cv.Polyline([]float64{all.x, all.x + all.w}, []float64{bottom, bottom}, thin, pal.Dim)
	titleX := all.x - gutter - px/2
	_ = cv.Text(c.Title, titleX, margin, 0, 0, px*1.15, pal.Foreground)
	tw, _, _ := cv.Measure(c.Title, px*1.15)
	legend(cv, c, titleX+tw+px*2, float64(width)-margin, margin, px, thin, pal)
	return img, nil
}

// TextPx is the size of a chart's text, width by height pixels and strips
// strips tall: a thirty-second of its shorter side -- of each strip's
// share of the height, where there are several, so a stack reads at the
// size of one -- and never under ten pixels.
func TextPx(width, height, strips int) float64 {
	h := float64(height)
	if strips > 1 {
		h = h * 2 / float64(strips+1)
	}
	return math.Max(math.Min(float64(width), h)/32, 10)
}

// box is a rectangle of the picture, in pixels.
type box struct{ x, y, w, h float64 }

// drawStrip draws s in b against x: its gridlines and y labels, its axis,
// and its readings -- a column a pixel, the band behind and the line over
// it. A column without a reading draws neither, and the line runs from
// column to column only where the model joined them.
func drawStrip(cv *canvas.Canvas, s chart.Strip, b box, x chart.Scale, px, thin float64, grid color.Color, pal Palette) {
	yAt := func(v float64) float64 {
		f, _ := s.Y.Frac(v)
		return b.y + (1-f)*b.h
	}
	for _, v := range s.Y.Ticks() {
		y := yAt(v)
		cv.Polyline([]float64{b.x, b.x + b.w}, []float64{y, y}, thin, grid)
		_ = cv.Text(s.Y.Label(v), b.x-px/2, y, 1, canvas.Centre, px, pal.Dim)
	}
	cv.Polyline([]float64{b.x, b.x}, []float64{b.y, b.y + b.h}, thin, pal.Dim)

	spread := faded(pal.Line, 0.22)
	var xs, ys []float64
	flush := func() {
		if len(xs) == 1 {
			// A lone reading is no line: a dot, so it is not lost.
			cv.Disc(xs[0], ys[0], thin*1.5, pal.Line)
		}
		cv.Polyline(xs, ys, math.Max(1.5, px/8), pal.Line)
		xs, ys = xs[:0], ys[:0]
	}
	first, last := x.Ends()
	for i, col := range chart.Bin(s.Segments, first, last, int(b.w)) {
		if !col.Ok {
			continue
		}
		cx := b.x + float64(i) + 0.5
		lo, hi := yAt(col.Min), yAt(col.Max)
		cv.Rect(cx-0.5, math.Min(lo, hi), 1, math.Max(math.Abs(lo-hi), 1), spread)
		if !col.Joined {
			flush()
		}
		xs, ys = append(xs, cx), append(ys, yAt(col.Mean))
	}
	flush()
}

// legend names the chart's kinds of band by a swatch of each, on the
// title's baseline, ending at right; one that would reach left of left --
// into the title -- is not drawn.
func legend(cv *canvas.Canvas, c chart.Chart, left, right, baseline, px, thin float64, pal Palette) {
	kinds := c.UsedBandKinds()
	if len(kinds) == 0 {
		return
	}
	gap := px * 1.5
	w := -gap
	for _, k := range kinds {
		tw, _, _ := cv.Measure(c.BandKinds[k].Name, px)
		w += px*1.2 + tw + gap
	}
	x := right - w
	if x < left {
		return
	}
	for _, k := range kinds {
		kind := c.BandKinds[k]
		sx, sy, side := x, baseline-px*0.75, px*0.75
		cv.Rect(sx, sy, side, side, bandColour(kind, pal))
		cv.Polyline([]float64{sx, sx + side, sx + side, sx, sx}, []float64{sy, sy, sy + side, sy + side, sy}, thin, pal.Dim)
		_ = cv.Text(kind.Name, x+px*1.2, baseline, 0, 0, px, pal.Dim)
		tw, _, _ := cv.Measure(kind.Name, px)
		x += px*1.2 + tw + gap
	}
}

// bandColour is the colour kind is drawn in: its own, or for a kind given a
// mark for braille and no colour, a faint wash of the chrome -- rather than
// a nil dereference.
func bandColour(kind chart.BandKind, pal Palette) color.Color {
	if kind.Colour == nil {
		return faded(pal.Dim, 0.15)
	}
	return kind.Colour
}

// faded is col at alpha a, to draw over what is already there: col's own
// alpha times a. RGBA() is premultiplied, so a colour that was already
// translucent read as darker than it is; NRGBA is not.
func faded(col color.Color, a float64) color.NRGBA {
	n := color.NRGBAModel.Convert(col).(color.NRGBA)
	n.A = uint8(math.Round(a * float64(n.A)))
	return n
}

// filled is p with each colour it leaves nil taken from Light. Only the
// zero Palette was Light once, and a palette that set its line alone
// filled its background with nil, and panicked.
func (p Palette) filled() Palette {
	for _, c := range []struct {
		to   *color.Color
		from color.Color
	}{
		{&p.Background, Light.Background}, {&p.Foreground, Light.Foreground}, {&p.Dim, Light.Dim}, {&p.Line, Light.Line},
	} {
		if *c.to == nil {
			*c.to = c.from
		}
	}
	return p
}

// Faded is col at alpha a: a band kind's colour from one of a palette's,
// faint enough to draw a line over.
func Faded(col color.Color, a float64) color.Color { return faded(col, a) }

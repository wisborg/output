package picture

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// canvas is what a chart is drawn with: filled shapes, anti-aliased, over
// an RGBA image, and text in Go Mono and any fonts to fall back on.
//
// It is the few primitives a chart needs and nothing more -- a general
// two-dimensional drawing library would be a dependency every program
// drawing a chart then links, for the sake of a rectangle, a disc, a
// stroked line and a label.
type canvas struct {
	img   *image.RGBA
	fonts []*opentype.Font // Go Mono, then the fallbacks
	faces map[int][]font.Face
	buf   sfnt.Buffer
}

func newCanvas(img *image.RGBA, fallbacks []*opentype.Font) (*canvas, error) {
	mono, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, fmt.Errorf("picture: parsing the embedded font: %w", err)
	}
	return &canvas{img: img, fonts: append([]*opentype.Font{mono}, fallbacks...), faces: map[int][]font.Face{}}, nil
}

func (c *canvas) fill(col color.Color) {
	draw.Draw(c.img, c.img.Bounds(), image.NewUniform(col), image.Point{}, draw.Src)
}

// shape is a filled area drawn in one pass: every part of it adds to the
// same coverage, so where two parts overlap -- the segments of a line at a
// join -- the overlap is drawn once, not twice as dark.
//
// Every part is wound the same way, clockwise on the screen; the
// rasteriser's coverage is the magnitude of the winding, and a part wound
// the other way would cut a hole where it overlapped another.
type shape struct {
	pts                    [][][2]float64
	minX, minY, maxX, maxY float64
}

func newShape() *shape {
	return &shape{minX: math.Inf(1), minY: math.Inf(1), maxX: math.Inf(-1), maxY: math.Inf(-1)}
}

func (s *shape) add(poly [][2]float64) {
	for _, p := range poly {
		s.minX, s.minY = math.Min(s.minX, p[0]), math.Min(s.minY, p[1])
		s.maxX, s.maxY = math.Max(s.maxX, p[0]), math.Max(s.maxY, p[1])
	}
	s.pts = append(s.pts, poly)
}

// rect is a rectangle, clockwise.
func (s *shape) rect(x, y, w, h float64) {
	s.add([][2]float64{{x, y}, {x + w, y}, {x + w, y + h}, {x, y + h}})
}

// disc is a disc of radius r at (x, y), clockwise: a polygon of enough
// sides that its edge is a curve at any size a chart draws one.
func (s *shape) disc(x, y, r float64) {
	n := max(12, int(r*4))
	poly := make([][2]float64, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		poly[i] = [2]float64{x + r*math.Cos(a), y + r*math.Sin(a)}
	}
	s.add(poly)
}

// stroke is the line through the points, width wide, with round joins and
// ends: a rectangle along each segment, and a disc at each point.
func (s *shape) stroke(xs, ys []float64, width float64) {
	h := width / 2
	for i := range xs {
		s.disc(xs[i], ys[i], h)
		if i == 0 {
			continue
		}
		dx, dy := xs[i]-xs[i-1], ys[i]-ys[i-1]
		l := math.Hypot(dx, dy)
		if l == 0 {
			continue
		}
		nx, ny := -dy/l*h, dx/l*h
		quad := [][2]float64{
			{xs[i-1] - nx, ys[i-1] - ny}, {xs[i] - nx, ys[i] - ny},
			{xs[i] + nx, ys[i] + ny}, {xs[i-1] + nx, ys[i-1] + ny},
		}
		// Clockwise whichever way the segment runs.
		if area(quad) < 0 {
			quad[1], quad[3] = quad[3], quad[1]
		}
		s.add(quad)
	}
}

// area is a polygon's signed area, positive for clockwise on the screen,
// where y runs down.
func area(poly [][2]float64) float64 {
	a := 0.0
	for i, p := range poly {
		q := poly[(i+1)%len(poly)]
		a += p[0]*q[1] - q[0]*p[1]
	}
	return a / 2
}

// draw fills the shape in col over what is already drawn.
func (c *canvas) draw(s *shape, col color.Color) {
	b := image.Rect(int(math.Floor(s.minX)), int(math.Floor(s.minY)), int(math.Ceil(s.maxX))+1, int(math.Ceil(s.maxY))+1).
		Intersect(c.img.Bounds())
	if b.Empty() {
		return
	}
	var r vector.Rasterizer
	r.Reset(b.Dx(), b.Dy())
	for _, poly := range s.pts {
		for i, p := range poly {
			x, y := float32(p[0]-float64(b.Min.X)), float32(p[1]-float64(b.Min.Y))
			if i == 0 {
				r.MoveTo(x, y)
			} else {
				r.LineTo(x, y)
			}
		}
		r.ClosePath()
	}
	r.DrawOp = draw.Over
	r.Draw(c.img, b, image.NewUniform(col), image.Point{})
}

func (c *canvas) rect(x, y, w, h float64, col color.Color) {
	s := newShape()
	s.rect(x, y, w, h)
	c.draw(s, col)
}

func (c *canvas) disc(x, y, r float64, col color.Color) {
	s := newShape()
	s.disc(x, y, r)
	c.draw(s, col)
}

// polyline strokes the line through the points; fewer than two draw
// nothing.
func (c *canvas) polyline(xs, ys []float64, width float64, col color.Color) {
	if len(xs) != len(ys) || len(xs) < 2 || width <= 0 {
		return
	}
	s := newShape()
	s.stroke(xs, ys, width)
	c.draw(s, col)
}

// facesAt are the fonts' faces at px pixels, rounded to a whole pixel,
// made the first time they are asked for.
func (c *canvas) facesAt(px float64) ([]font.Face, error) {
	key := int(math.Round(math.Max(px, 1)))
	if fs, ok := c.faces[key]; ok {
		return fs, nil
	}
	// At 72 DPI a point is a pixel, so a size asked for in pixels is one.
	opts := &opentype.FaceOptions{Size: float64(key), DPI: 72, Hinting: font.HintingFull}
	var fs []font.Face
	for _, f := range c.fonts {
		face, err := opentype.NewFace(f, opts)
		if err != nil {
			return nil, fmt.Errorf("picture: a %dpx face: %w", key, err)
		}
		fs = append(fs, face)
	}
	c.faces[key] = fs
	return fs, nil
}

// faceFor is the face a letter is drawn in: the first font's that has it,
// or Go Mono's, whose placeholder box says plainly that none did.
func (c *canvas) faceFor(fs []font.Face, r rune) font.Face {
	for i, f := range c.fonts {
		if g, err := f.GlyphIndex(&c.buf, r); err == nil && g != 0 {
			return fs[i]
		}
	}
	return fs[0]
}

// measure is the width text takes at px, and its line's height.
func (c *canvas) measure(text string, px float64) (w, h float64, err error) {
	fs, err := c.facesAt(px)
	if err != nil {
		return 0, 0, err
	}
	var adv fixed.Int26_6
	for _, r := range text {
		a, _ := c.faceFor(fs, r).GlyphAdvance(r)
		adv += a
	}
	return float64(adv) / 64, float64(fs[0].Metrics().Height) / 64, nil
}

// text writes text at px in col, anchored at (x, y): ax and ay place the
// anchor in the text's box, 0 left, 0.5 the middle, 1 right; and for ay,
// 0 is the baseline, 0.5 a line's height below it -- the text centred on
// y -- and 1 a whole line below.
func (c *canvas) text(text string, x, y, ax, ay, px float64, col color.Color) error {
	if text == "" {
		return nil
	}
	w, h, err := c.measure(text, px)
	if err != nil {
		return err
	}
	fs, _ := c.facesAt(px)
	d := font.Drawer{Dst: c.img, Src: image.NewUniform(col),
		Dot: fixed.Point26_6{X: fixed.Int26_6(math.Round((x - ax*w) * 64)), Y: fixed.Int26_6(math.Round((y + ay*h) * 64))}}
	for _, r := range text {
		d.Face = c.faceFor(fs, r)
		d.DrawString(string(r))
	}
	return nil
}

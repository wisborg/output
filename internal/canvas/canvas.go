// Package canvas is the few drawing primitives this module's pictures are
// made of -- filled and stroked shapes, anti-aliased, and text in Go Mono
// with fonts to fall back on -- over golang.org/x/image alone.
//
// It is internal: chart/picture and diagram/picture draw with it, and a
// program that wants a general two-dimensional drawing library should use
// one. Keeping it to what those two need is what keeps it small.
package canvas

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"slices"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
	"golang.org/x/image/vector"
)

// Canvas is what a picture is drawn with: filled shapes, anti-aliased, over
// an RGBA image, and text in Go Mono and any fonts to fall back on.
//
// It is the few primitives a chart or a diagram needs and nothing more -- a
// general two-dimensional drawing library would be a dependency every
// program drawing one then links, for the sake of a rectangle, a disc, a
// stroked line and a label.
type Canvas struct {
	img   *image.RGBA
	fonts []*opentype.Font // Go Mono, then the fallbacks
	faces map[int][]font.Face
	buf   sfnt.Buffer
}

// New is a canvas over img, writing text in Go Mono and, for a letter Go
// Mono lacks, the first of fallbacks that has it.
func New(img *image.RGBA, fallbacks []*opentype.Font) (*Canvas, error) {
	mono, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, fmt.Errorf("canvas: parsing the embedded font: %w", err)
	}
	return &Canvas{img: img, fonts: append([]*opentype.Font{mono}, fallbacks...), faces: map[int][]font.Face{}}, nil
}

// Fill paints the whole image col.
func (c *Canvas) Fill(col color.Color) {
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
func (c *Canvas) draw(s *shape, col color.Color) {
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

// Rect fills the rectangle at (x, y), w by h, in col.
func (c *Canvas) Rect(x, y, w, h float64, col color.Color) {
	s := newShape()
	s.rect(x, y, w, h)
	c.draw(s, col)
}

// Disc fills the disc of radius r at (x, y) in col.
func (c *Canvas) Disc(x, y, r float64, col color.Color) {
	s := newShape()
	s.disc(x, y, r)
	c.draw(s, col)
}

// Polyline strokes the line through the points, width wide, with round
// joins and ends; fewer than two points draw nothing.
func (c *Canvas) Polyline(xs, ys []float64, width float64, col color.Color) {
	if len(xs) != len(ys) || len(xs) < 2 || width <= 0 {
		return
	}
	s := newShape()
	s.stroke(xs, ys, width)
	c.draw(s, col)
}

// facesAt are the fonts' faces at px pixels, rounded to a whole pixel,
// made the first time they are asked for.
func (c *Canvas) facesAt(px float64) ([]font.Face, error) {
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
			return nil, fmt.Errorf("canvas: a %dpx face: %w", key, err)
		}
		fs = append(fs, face)
	}
	c.faces[key] = fs
	return fs, nil
}

// faceFor is the face a letter is drawn in: the first font's that has it,
// or Go Mono's, whose placeholder box says plainly that none did.
func (c *Canvas) faceFor(fs []font.Face, r rune) font.Face {
	for i, f := range c.fonts {
		if g, err := f.GlyphIndex(&c.buf, r); err == nil && g != 0 {
			return fs[i]
		}
	}
	return fs[0]
}

// Measure is the width text takes at px, and its line's height.
func (c *Canvas) Measure(text string, px float64) (w, h float64, err error) {
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

// Centre is the ay that puts a line's letters across the middle of the y
// Text writes them at: their baseline about a third of a line's height
// below it, for the capitals and the x-height of Go Mono.
const Centre = 0.32

// Text writes text at px in col, anchored at (x, y): ax places the anchor
// across the text, 0 its left end, 0.5 its middle, 1 its right; ay puts
// the baseline ay times a line's height below y -- 0 on it, and Centre
// for the letters centred on y. (0.5 is not the middle: a line's height
// includes the room below the baseline, and text anchored at 0.5 sat a
// quarter of a letter low, its labels below the gridlines they named.)
func (c *Canvas) Text(text string, x, y, ax, ay, px float64, col color.Color) error {
	if text == "" {
		return nil
	}
	w, h, err := c.Measure(text, px)
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

// Polygon fills the polygon through pts in col, whichever way it winds.
func (c *Canvas) Polygon(pts [][2]float64, col color.Color) {
	if len(pts) < 3 {
		return
	}
	poly := append([][2]float64(nil), pts...)
	if area(poly) < 0 {
		// Clockwise, as every part of a shape is; see shape.
		slices.Reverse(poly)
	}
	s := newShape()
	s.add(poly)
	c.draw(s, col)
}

// Dashed strokes the line through the points as Polyline does, broken into
// dashes dash long with gaps gap long, measured along the line so a dash
// turns a corner rather than starting afresh at each point. A dash or gap
// of zero or less draws it whole.
func (c *Canvas) Dashed(xs, ys []float64, width, dash, gap float64, col color.Color) {
	if dash <= 0 || gap <= 0 {
		c.Polyline(xs, ys, width, col)
		return
	}
	if len(xs) != len(ys) || len(xs) < 2 || width <= 0 {
		return
	}
	s := newShape()
	on, left := true, dash // drawing a dash or a gap, and how much of it remains
	var px, py []float64
	flush := func() {
		if len(px) >= 2 {
			s.stroke(px, py, width)
		}
		px, py = px[:0], py[:0]
	}
	px, py = append(px, xs[0]), append(py, ys[0])
	for i := 1; i < len(xs); i++ {
		x0, y0 := xs[i-1], ys[i-1]
		dx, dy := xs[i]-x0, ys[i]-y0
		l := math.Hypot(dx, dy)
		done := 0.0
		for l-done > left {
			done += left
			x, y := x0+dx*done/l, y0+dy*done/l
			if on {
				px, py = append(px, x), append(py, y)
				flush()
			} else {
				px, py = append(px[:0], x), append(py[:0], y)
			}
			on = !on
			left = dash
			if !on {
				left = gap
			}
		}
		left -= l - done
		if on {
			px, py = append(px, xs[i]), append(py, ys[i])
		}
	}
	if on {
		flush()
	}
	c.draw(s, col)
}

// Ellipse is the outline of the ellipse at (cx, cy) with radii rx and ry,
// as a polygon of enough sides to be a curve, clockwise on the screen, for
// Polygon to fill and Polyline or Dashed, closed by repeating its first
// point, to stroke.
func Ellipse(cx, cy, rx, ry float64) [][2]float64 {
	n := max(24, int((rx+ry)*2))
	pts := make([][2]float64, n)
	for i := range n {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = [2]float64{cx + rx*math.Cos(a), cy + ry*math.Sin(a)}
	}
	return pts
}

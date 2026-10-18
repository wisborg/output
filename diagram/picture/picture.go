// Package picture draws a laid-out diagram as an image: the same layout the
// diagram package draws in characters, each cell a box of pixels, so the
// picture and the text agree on where everything is.
//
// It draws in Go Mono, embedded, whose letters are each the same width, as
// the layout measures them -- with fonts of the caller's to fall back on for
// letters Go Mono lacks -- over golang.org/x/image.
package picture

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"golang.org/x/image/font/opentype"

	"github.com/wisborg/output/diagram"
	"github.com/wisborg/output/internal/canvas"
)

// DefaultTextPx is the size a diagram's text is drawn at where Options do
// not say.
const DefaultTextPx = 16

// Options are how a picture is drawn. The zero value is the Light palette,
// text DefaultTextPx tall, and Go Mono alone.
type Options struct {
	Palette diagram.Palette
	// TextPx is the size of the labels, in pixels; it sets the size of
	// everything else.
	TextPx float64
	// Fonts are fonts to fall back on for a letter Go Mono lacks.
	Fonts []*opentype.Font
}

// MaxPixels is the most pixels a picture Draw draws may have: 2^26, a
// quarter of a gigabyte of RGBA, 8000 by 8000. A size asked for in a flag,
// or a diagram drawn at a large text size, can come to terabytes: an error
// saying so is better than a program that dies allocating them.
const MaxPixels = 1 << 26

// Draw draws lay. A TextPx that is not a number, or that makes a picture of
// more than MaxPixels, is an error.
func Draw(lay *diagram.Layout, opts Options) (*image.RGBA, error) {
	if lay == nil {
		return nil, errors.New("picture: no layout to draw")
	}
	pal := opts.Palette
	if pal == (diagram.Palette{}) {
		pal = diagram.Light
	}
	px := opts.TextPx
	if math.IsNaN(px) || math.IsInf(px, 0) {
		return nil, fmt.Errorf("picture: a text size of %v pixels", px)
	}
	if px <= 0 {
		px = DefaultTextPx
	}
	s := diagram.ScaleFor(px)
	w, h := s.Size(lay)
	if w < 1 || h < 1 {
		// A text size so small the picture is no pixel wide: a blank
		// one-pixel PNG, written with exit 0, once.
		return nil, fmt.Errorf("picture: %.2g by %.2g pixels, at text %v pixels, is no picture", w, h, px)
	}
	if w*h > MaxPixels {
		return nil, fmt.Errorf("picture: %.0f by %.0f pixels is more than %d million", math.Ceil(w), math.Ceil(h), MaxPixels/1000000)
	}
	img := image.NewRGBA(image.Rect(0, 0, int(math.Ceil(w)), int(math.Ceil(h))))
	cv, err := canvas.New(img, opts.Fonts)
	if err != nil {
		return nil, err
	}
	d := drawer{cv: cv, s: s, pal: pal, px: px, lay: lay, thin: math.Max(1, px/12)}
	cv.Fill(pal.Background)
	d.groups()
	for _, e := range lay.Edges {
		d.edge(e)
	}
	for _, n := range lay.Nodes {
		d.node(n)
	}
	for _, e := range lay.Legend {
		d.entry(e)
	}
	d.labels()
	if lay.Title != "" {
		_ = cv.Text(lay.Title, s.Left(0), s.Y(0), 0, textCentre, px*1.15, pal.Foreground)
	}
	return img, nil
}

// textCentre is the anchor that puts a line of text's letters across the
// middle of the y it is written at: its baseline that far below.
const textCentre = canvas.Centre

type drawer struct {
	cv   *canvas.Canvas
	s    diagram.Scale
	pal  diagram.Palette
	px   float64
	lay  *diagram.Layout
	thin float64
}

func or(c, def color.Color) color.Color {
	if c == nil {
		return def
	}
	return c
}

// stroke strokes the line through the points in line's style.
func (d drawer) stroke(xs, ys []float64, line diagram.Line, col color.Color) {
	w := d.thin
	switch line {
	case diagram.Bold:
		d.cv.Polyline(xs, ys, w*2.5, col)
	case diagram.Dashed:
		d.cv.Dashed(xs, ys, w, d.px*0.5, d.px*0.35, col)
	case diagram.Dotted:
		d.cv.Dashed(xs, ys, w*1.3, w*0.1, d.px*0.3, col)
	default:
		d.cv.Polyline(xs, ys, w, col)
	}
}

// outline strokes the closed outline through pts in line's style.
func (d drawer) outline(pts [][2]float64, line diagram.Line, col color.Color) {
	xs, ys := make([]float64, 0, len(pts)+1), make([]float64, 0, len(pts)+1)
	for _, p := range append(pts, pts[0]) {
		xs, ys = append(xs, p[0]), append(ys, p[1])
	}
	d.stroke(xs, ys, line, col)
}

func rect(x0, y0, x1, y1 float64) [][2]float64 {
	return [][2]float64{{x0, y0}, {x1, y0}, {x1, y1}, {x0, y1}}
}

// rounded is a rectangle with corners of radius r.
func rounded(x0, y0, x1, y1, r float64) [][2]float64 {
	r = math.Min(r, math.Min(x1-x0, y1-y0)/2)
	var pts [][2]float64
	corner := func(cx, cy, from float64) {
		for i := 0; i <= 6; i++ {
			a := from + float64(i)*math.Pi/12
			pts = append(pts, [2]float64{cx + r*math.Cos(a), cy + r*math.Sin(a)})
		}
	}
	corner(x1-r, y0+r, -math.Pi/2)
	corner(x1-r, y1-r, 0)
	corner(x0+r, y1-r, math.Pi/2)
	corner(x0+r, y0+r, math.Pi)
	return pts
}

// groups draws the frames, the lanes' fills and their lines, beneath
// everything else.
func (d drawer) groups() {
	for _, g := range d.lay.Groups {
		col := or(g.Group.Colour, d.pal.Dim)
		x0, y0, x1, y1 := d.s.Box(g.Rect)
		if g.Group.Fill != nil && g.Group.Style != diagram.Hidden {
			d.cv.Polygon(rect(x0, y0, x1, y1), g.Group.Fill)
		}
		if g.Group.Style == diagram.Frame {
			d.outline(rounded(x0, y0, x1, y1, d.px*0.4), diagram.Solid, col)
		}
		for _, div := range g.Dividers {
			d.cv.Polyline([]float64{d.s.X(div[0].X), d.s.X(div[1].X)}, []float64{d.s.Y(div[0].Y), d.s.Y(div[1].Y)}, d.thin, col)
		}
	}
}

// edge draws an edge's line and its arrowheads.
func (d drawer) edge(e diagram.PlacedEdge) {
	if e.Edge.Invisible || len(e.Points) < 2 {
		return
	}
	col := or(e.Edge.Colour, d.pal.Foreground)
	xs, ys := d.s.Route(d.lay, e)
	n := len(xs)
	// The line stops where the arrowhead begins, so it does not show
	// through the head's point.
	head := d.px * 0.6
	if e.Edge.Head {
		xs[n-1], ys[n-1] = pullBack(xs[n-2], ys[n-2], xs[n-1], ys[n-1], head*0.8)
	}
	if e.Edge.Tail {
		xs[0], ys[0] = pullBack(xs[1], ys[1], xs[0], ys[0], head*0.8)
	}
	d.stroke(xs, ys, e.Edge.Line, col)
	ox, oy := d.s.Route(d.lay, e)
	if e.Edge.Head {
		d.arrow(ox[n-2], oy[n-2], ox[n-1], oy[n-1], head, col)
	}
	if e.Edge.Tail {
		d.arrow(ox[1], oy[1], ox[0], oy[0], head, col)
	}
}

// pullBack is (x, y) moved by l towards (fx, fy).
func pullBack(fx, fy, x, y, l float64) (float64, float64) {
	dx, dy := x-fx, y-fy
	n := math.Hypot(dx, dy)
	if n <= l {
		return x, y
	}
	return x - dx/n*l, y - dy/n*l
}

// arrow fills an arrowhead l long with its point at (x, y), on the line
// from (fx, fy).
func (d drawer) arrow(fx, fy, x, y, l float64, col color.Color) {
	dx, dy := x-fx, y-fy
	n := math.Hypot(dx, dy)
	if n == 0 {
		return
	}
	ux, uy := dx/n, dy/n
	bx, by := x-ux*l, y-uy*l
	w := l * 0.45
	d.cv.Polygon([][2]float64{{x, y}, {bx - uy*w, by + ux*w}, {bx + uy*w, by - ux*w}}, col)
}

// node draws a node: its fill, its outline in its shape, and its label.
func (d drawer) node(pn diagram.PlacedNode) {
	n := pn.Node
	if n.Invisible {
		return
	}
	x0, y0, x1, y1 := d.s.Box(pn.Rect)
	fill := or(n.Fill, d.pal.Background)
	line := or(n.Colour, d.pal.Foreground)
	top := pn.Rect.Y + 1
	switch n.Shape {
	case diagram.Box:
		d.cv.Polygon(rect(x0, y0, x1, y1), fill)
		d.outline(rect(x0, y0, x1, y1), n.Line, line)
	case diagram.Rounded:
		pts := rounded(x0, y0, x1, y1, d.px*0.6)
		d.cv.Polygon(pts, fill)
		d.outline(pts, n.Line, line)
	case diagram.Ellipse:
		pts := canvas.Ellipse((x0+x1)/2, (y0+y1)/2, (x1-x0)/2, (y1-y0)/2)
		d.cv.Polygon(pts, fill)
		d.outline(pts, n.Line, line)
	case diagram.Cylinder:
		d.cylinder(x0, y0, x1, y1, fill, line, n.Line)
		top++
	case diagram.Plain:
		top = pn.Rect.Y
	}
	text := n.Text
	if text == nil {
		text = d.pal.TextOn(filledBy(n))
	}
	label := n.Label
	if label == "" {
		label = n.ID
	}
	for i, l := range strings.Split(label, "\n") {
		_ = d.cv.Text(l, (x0+x1)/2, d.s.Y(top+i), 0.5, textCentre, d.px, text)
	}
}

// cylinder draws a drum: its body, the front of its bottom, and its lid.
func (d drawer) cylinder(x0, y0, x1, y1 float64, fill, line color.Color, style diagram.Line) {
	lid := d.s.Lid()
	cx, rx := (x0+x1)/2, (x1-x0)/2
	top := canvas.Ellipse(cx, y0+lid, rx, lid)
	bottom := canvas.Ellipse(cx, y1-lid, rx, lid)
	d.cv.Polygon(rect(x0, y0+lid, x1, y1-lid), fill)
	d.cv.Polygon(bottom, fill)
	d.cv.Polygon(top, fill)
	// The sides and the front half of the bottom, as one line: down the
	// left side, round the bottom's lower half from left to right, and up
	// the right side.
	xs, ys := []float64{x0}, []float64{y0 + lid}
	for i := 0; i <= 24; i++ {
		a := math.Pi - float64(i)*math.Pi/24
		xs, ys = append(xs, cx+rx*math.Cos(a)), append(ys, y1-lid+lid*math.Sin(a))
	}
	xs, ys = append(xs, x1), append(ys, y0+lid)
	d.stroke(xs, ys, style, line)
	d.outline(top, style, line)
}

// labels writes the groups' and the edges' labels, over everything, each
// on a patch of the background so a line passing beneath does not cut it.
func (d drawer) labels() {
	for _, g := range d.lay.Groups {
		label := g.Group.Label
		if label == "" || g.Group.Style == diagram.Hidden {
			continue
		}
		col := or(g.Group.Colour, d.pal.Dim)
		if g.Group.Style == diagram.Frame {
			d.patch(" "+label+" ", d.s.Left(g.LabelAt.X), d.s.Y(g.LabelAt.Y), d.px)
			_ = d.cv.Text(label, d.s.Left(g.LabelAt.X+1), d.s.Y(g.LabelAt.Y), 0, textCentre, d.px, col)
			continue
		}
		_ = d.cv.Text(label, d.s.Left(g.LabelAt.X), d.s.Y(g.LabelAt.Y), 0, textCentre, d.px, col)
	}
	for _, e := range d.lay.Edges {
		if e.Edge.Label == "" || e.Edge.Invisible {
			continue
		}
		for i, l := range strings.Split(e.Edge.Label, "\n") {
			at := e.PictureLabelAt()
			nx, ny := e.PictureLabelNudge()
			x, y := d.s.Left(at.X)+nx*d.s.Cell, d.s.Y(at.Y+i)+ny*d.s.Line
			d.patch(l, x, y, d.px)
			_ = d.cv.Text(l, x, y, 0, textCentre, d.px*0.9, d.pal.Dim)
		}
	}
}

// patch fills the cells text written at at would take with the background.
func (d drawer) patch(text string, x, y, px float64) {
	w, _, _ := d.cv.Measure(text, px)
	d.cv.Rect(x, y-d.s.Line/2+1, w, d.s.Line-2, d.pal.Background)
}

// filledBy is what fills n behind its label: its fill, unless it is plain
// text, which is drawn on the background whatever its fill.
func filledBy(n *diagram.Node) color.Color {
	if n.Shape == diagram.Plain {
		return nil
	}
	return n.Fill
}

// entry draws a legend entry: its sample, a stroke drawn as an edge is or a
// swatch filled and outlined as a node is, across the cells given it, and
// its label after it.
func (d drawer) entry(pe diagram.PlacedEntry) {
	e, r := pe.Entry, pe.Sample
	x0, x1, y := d.s.Left(r.X)+d.px*0.2, d.s.Left(r.X+r.W)-d.px*0.2, d.s.Y(r.Y)
	col := or(e.Colour, d.pal.Foreground)
	if e.Sample == diagram.Stroke {
		d.stroke([]float64{x0, x1}, []float64{y, y}, e.Line, col)
	} else {
		half := d.s.Line * 0.3
		box := rect(x0, y-half, x1, y+half)
		if e.Fill != nil {
			d.cv.Polygon(box, e.Fill)
		}
		d.outline(box, diagram.Solid, col)
	}
	for i, l := range strings.Split(e.Label, "\n") {
		_ = d.cv.Text(l, d.s.Left(pe.LabelAt.X), d.s.Y(pe.LabelAt.Y+i), 0, textCentre, d.px, d.pal.Foreground)
	}
}

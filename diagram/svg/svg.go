// Package svg writes a laid-out diagram as SVG: the same layout, drawn as
// the picture package draws it, as a document a browser shows and scales,
// whose text can be selected and searched.
//
// The text is set in a monospaced font -- Go Mono where the reader has it,
// else the platform's own -- since the layout measures every letter as one
// cell wide, or two for a wide one, as diagram.Width counts them. It
// imports nothing the diagram package does not.
package svg

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"math"
	"strings"

	"github.com/wisborg/output/diagram"
)

// DefaultTextPx is the size a diagram's text is set at where Options do not
// say.
const DefaultTextPx = 16

// Options are how a diagram is written. The zero value is the Light palette
// and text DefaultTextPx tall.
type Options struct {
	Palette diagram.Palette
	TextPx  float64
}

// fonts is the font-family the text is set in.
const fonts = `'Go Mono', Menlo, Consolas, 'DejaVu Sans Mono', 'Liberation Mono', monospace`

// MaxSide is the largest width or height Write writes a document at, in
// its pixels: far past any screen or page, and short of the sizes viewers
// refuse.
const MaxSide = 100000

// Write writes lay to w as an SVG document. A TextPx that is not a number
// is an error: written into the document it made width="NaN", which no
// reader draws.
func Write(w io.Writer, lay *diagram.Layout, opts Options) error {
	if lay == nil {
		return errors.New("svg: no layout to write")
	}
	pal := opts.Palette
	if pal == (diagram.Palette{}) {
		pal = diagram.Light
	}
	px := opts.TextPx
	if math.IsNaN(px) || math.IsInf(px, 0) {
		return fmt.Errorf("svg: a text size of %v pixels", px)
	}
	if px <= 0 {
		px = DefaultTextPx
	}
	s := diagram.ScaleFor(px)
	d := &doc{s: s, pal: pal, px: px, lay: lay, thin: math.Max(1, px/12)}
	width, height := s.Size(lay)
	// A document no viewer draws is a mistake to say, not to write: a
	// text size of 1e300 wrote a width of three hundred digits, and one of
	// 1e-300 a width and a height of 0 -- an invisible document, exit 0.
	if width < 1 || height < 1 || width > MaxSide || height > MaxSide {
		return fmt.Errorf("svg: a document %s by %s, at text %v pixels: draw it between 1 and %d a side", num(width), num(height), px, MaxSide)
	}
	fmt.Fprintf(&d.b, `<svg xmlns="http://www.w3.org/2000/svg" width="%s" height="%s" viewBox="0 0 %s %s" font-family="%s" font-size="%s">`+"\n",
		num(width), num(height), num(width), num(height), xmlText(fonts), num(px))
	fmt.Fprintf(&d.b, `<rect width="100%%" height="100%%" %s/>`+"\n", fill(pal.Background))
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
		d.text(lay.Title, s.Left(0), s.Y(0), "start", px*1.15, pal.Foreground)
	}
	d.b.WriteString("</svg>\n")
	_, err := io.WriteString(w, d.b.String())
	return err
}

type doc struct {
	b    strings.Builder
	s    diagram.Scale
	pal  diagram.Palette
	px   float64
	lay  *diagram.Layout
	thin float64
}

// num is a coordinate, to a hundredth of a pixel and no more.
func num(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "-0" {
		return "0"
	}
	return s
}

// colour is c as SVG takes a colour, and its opacity, 1 where opaque.
func colour(c color.Color) (string, float64) {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("#%02x%02x%02x", n.R, n.G, n.B), float64(n.A) / 255
}

// fill is the attributes filling a shape with c.
func fill(c color.Color) string {
	s, a := colour(c)
	switch {
	case a == 0:
		return `fill="none"`
	case a < 1:
		return fmt.Sprintf(`fill="%s" fill-opacity="%s"`, s, num(a))
	}
	return fmt.Sprintf(`fill="%s"`, s)
}

func or(c, def color.Color) color.Color {
	if c == nil {
		return def
	}
	return c
}

// stroke is the attributes of a line in line's style.
func (d *doc) stroke(line diagram.Line, col color.Color) string {
	w := d.thin
	s, a := colour(col)
	attrs := fmt.Sprintf(`stroke="%s" stroke-linecap="round" stroke-linejoin="round"`, s)
	if a < 1 {
		attrs += fmt.Sprintf(` stroke-opacity="%s"`, num(a))
	}
	switch line {
	case diagram.Bold:
		w *= 2.5
	case diagram.Dashed:
		attrs += fmt.Sprintf(` stroke-dasharray="%s %s"`, num(d.px*0.5), num(d.px*0.35))
	case diagram.Dotted:
		w *= 1.3
		attrs += fmt.Sprintf(` stroke-dasharray="0.1 %s"`, num(d.px*0.3))
	}
	return attrs + fmt.Sprintf(` stroke-width="%s"`, num(w))
}

func pathOf(xs, ys []float64, closed bool) string {
	var b strings.Builder
	for i := range xs {
		if i == 0 {
			b.WriteString("M")
		} else {
			b.WriteString(" L")
		}
		b.WriteString(num(xs[i]) + " " + num(ys[i]))
	}
	if closed {
		b.WriteString(" Z")
	}
	return b.String()
}

func (d *doc) groups() {
	for _, g := range d.lay.Groups {
		col := or(g.Group.Colour, d.pal.Dim)
		x0, y0, x1, y1 := d.s.Box(g.Rect)
		filled := `fill="none"`
		if g.Group.Fill != nil && g.Group.Style != diagram.Hidden {
			filled = fill(g.Group.Fill)
		}
		switch {
		case g.Group.Style == diagram.Frame:
			fmt.Fprintf(&d.b, `<rect x="%s" y="%s" width="%s" height="%s" rx="%s" %s %s/>`+"\n",
				num(x0), num(y0), num(x1-x0), num(y1-y0), num(d.px*0.4), filled, d.stroke(diagram.Solid, col))
		case filled != `fill="none"`:
			fmt.Fprintf(&d.b, `<rect x="%s" y="%s" width="%s" height="%s" %s/>`+"\n", num(x0), num(y0), num(x1-x0), num(y1-y0), filled)
		}
		for _, div := range g.Dividers {
			fmt.Fprintf(&d.b, `<path d="%s" fill="none" %s/>`+"\n",
				pathOf([]float64{d.s.X(div[0].X), d.s.X(div[1].X)}, []float64{d.s.Y(div[0].Y), d.s.Y(div[1].Y)}, false), d.stroke(diagram.Solid, col))
		}
	}
}

func (d *doc) edge(e diagram.PlacedEdge) {
	if e.Edge.Invisible || len(e.Points) < 2 {
		return
	}
	col := or(e.Edge.Colour, d.pal.Foreground)
	xs, ys := d.s.Route(d.lay, e)
	ox, oy := append([]float64(nil), xs...), append([]float64(nil), ys...)
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
	fmt.Fprintf(&d.b, `<path d="%s" fill="none" %s/>`+"\n", pathOf(xs, ys, false), d.stroke(e.Edge.Line, col))
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

// arrow is an arrowhead l long with its point at (x, y), on the line from
// (fx, fy).
func (d *doc) arrow(fx, fy, x, y, l float64, col color.Color) {
	dx, dy := x-fx, y-fy
	n := math.Hypot(dx, dy)
	if n == 0 {
		return
	}
	ux, uy := dx/n, dy/n
	bx, by := x-ux*l, y-uy*l
	w := l * 0.45
	fmt.Fprintf(&d.b, `<path d="%s" %s/>`+"\n",
		pathOf([]float64{x, bx - uy*w, bx + uy*w}, []float64{y, by + ux*w, by - ux*w}, true), fill(col))
}

func (d *doc) node(pn diagram.PlacedNode) {
	n := pn.Node
	if n.Invisible {
		return
	}
	x0, y0, x1, y1 := d.s.Box(pn.Rect)
	filled := fill(or(n.Fill, d.pal.Background))
	stroke := d.stroke(n.Line, or(n.Colour, d.pal.Foreground))
	top := pn.Rect.Y + 1
	switch n.Shape {
	case diagram.Box, diagram.Rounded:
		rx := 0.0
		if n.Shape == diagram.Rounded {
			rx = math.Min(d.px*0.6, math.Min(x1-x0, y1-y0)/2)
		}
		fmt.Fprintf(&d.b, `<rect x="%s" y="%s" width="%s" height="%s" rx="%s" %s %s/>`+"\n",
			num(x0), num(y0), num(x1-x0), num(y1-y0), num(rx), filled, stroke)
	case diagram.Ellipse:
		fmt.Fprintf(&d.b, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s" %s %s/>`+"\n",
			num((x0+x1)/2), num((y0+y1)/2), num((x1-x0)/2), num((y1-y0)/2), filled, stroke)
	case diagram.Cylinder:
		lid := d.s.Lid()
		rx := (x1 - x0) / 2
		// The body with the front of the bottom, then the lid over it.
		fmt.Fprintf(&d.b, `<path d="M%s %s L%s %s A%s %s 0 0 0 %s %s L%s %s" %s %s/>`+"\n",
			num(x0), num(y0+lid), num(x0), num(y1-lid), num(rx), num(lid), num(x1), num(y1-lid), num(x1), num(y0+lid), filled, stroke)
		fmt.Fprintf(&d.b, `<ellipse cx="%s" cy="%s" rx="%s" ry="%s" %s %s/>`+"\n",
			num((x0+x1)/2), num(y0+lid), num(rx), num(lid), filled, stroke)
		top++
	case diagram.Plain:
		top = pn.Rect.Y
	}
	label := n.Label
	if label == "" {
		label = n.ID
	}
	for i, l := range strings.Split(label, "\n") {
		d.text(l, (x0+x1)/2, d.s.Y(top+i), "middle", d.px, or(n.Text, d.pal.TextOn(filledBy(n))))
	}
}

// labels writes the groups' and the edges' labels, over everything, each
// on a patch of the background so a line passing beneath does not cut it.
func (d *doc) labels() {
	for _, g := range d.lay.Groups {
		label := g.Group.Label
		if label == "" || g.Group.Style == diagram.Hidden {
			continue
		}
		col := or(g.Group.Colour, d.pal.Dim)
		if g.Group.Style == diagram.Frame {
			d.patch(diagram.Width(label)+2, d.s.Left(g.LabelAt.X), d.s.Y(g.LabelAt.Y))
			d.text(label, d.s.Left(g.LabelAt.X+1), d.s.Y(g.LabelAt.Y), "start", d.px, col)
			continue
		}
		d.text(label, d.s.Left(g.LabelAt.X), d.s.Y(g.LabelAt.Y), "start", d.px, col)
	}
	for _, e := range d.lay.Edges {
		if e.Edge.Label == "" || e.Edge.Invisible {
			continue
		}
		for i, l := range strings.Split(e.Edge.Label, "\n") {
			at := e.PictureLabelAt()
			nx, ny := e.PictureLabelNudge()
			x, y := d.s.Left(at.X)+nx*d.s.Cell, d.s.Y(at.Y+i)+ny*d.s.Line
			d.patch(diagram.Width(l), x, y)
			d.text(l, x, y, "start", d.px*0.9, d.pal.Dim)
		}
	}
}

// patch fills cells cells from at with the background, beneath a label.
func (d *doc) patch(cells int, x, y float64) {
	fmt.Fprintf(&d.b, `<rect x="%s" y="%s" width="%s" height="%s" %s/>`+"\n",
		num(x), num(y-d.s.Line/2+1), num(float64(cells)*d.s.Cell), num(d.s.Line-2), fill(d.pal.Background))
}

func (d *doc) text(s string, x, y float64, anchor string, px float64, col color.Color) {
	if s == "" {
		return
	}
	size := ""
	if px != d.px {
		size = fmt.Sprintf(` font-size="%s"`, num(px))
	}
	fmt.Fprintf(&d.b, `<text x="%s" y="%s" text-anchor="%s" dominant-baseline="central" %s%s xml:space="preserve">%s</text>`+"\n",
		num(x), num(y), anchor, fill(col), size, xmlText(s))
}

// xmlText is s escaped for XML, as text or an attribute's value, with what
// XML 1.0 cannot hold at all -- a control character other than a tab or a
// line break, a byte that is not UTF-8, U+FFFE and U+FFFF -- written as
// U+FFFD. html.EscapeString escapes the five markup characters and passes
// the rest through, so a label holding a form feed made a file no browser
// would open: not well-formed, which is not a rendering anyone can repair.
func xmlText(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '&':
			b.WriteString("&amp;")
		case r == '"':
			b.WriteString("&#34;")
		case r == '\'':
			b.WriteString("&#39;")
		case r == '\t' || r == '\n' || r == '\r' || r >= 0x20 && r < 0xD800 || r >= 0xE000 && r < 0xFFFE || r >= 0x10000 && r <= 0x10FFFF:
			b.WriteRune(r)
		default:
			b.WriteRune('\uFFFD')
		}
	}
	return b.String()
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
// swatch filled and outlined as a node is, and its label after it.
func (d *doc) entry(pe diagram.PlacedEntry) {
	e, r := pe.Entry, pe.Sample
	x0, x1, y := d.s.Left(r.X)+d.px*0.2, d.s.Left(r.X+r.W)-d.px*0.2, d.s.Y(r.Y)
	col := or(e.Colour, d.pal.Foreground)
	if e.Sample == diagram.Stroke {
		fmt.Fprintf(&d.b, `<path d="%s" fill="none" %s/>`+"\n", pathOf([]float64{x0, x1}, []float64{y, y}, false), d.stroke(e.Line, col))
	} else {
		half := d.s.Line * 0.3
		filled := `fill="none"`
		if e.Fill != nil {
			filled = fill(e.Fill)
		}
		fmt.Fprintf(&d.b, `<rect x="%s" y="%s" width="%s" height="%s" %s %s/>`+"\n",
			num(x0), num(y-half), num(x1-x0), num(2*half), filled, d.stroke(diagram.Solid, col))
	}
	for i, l := range strings.Split(e.Label, "\n") {
		d.text(l, d.s.Left(pe.LabelAt.X), d.s.Y(pe.LabelAt.Y+i), "start", d.px, d.pal.Foreground)
	}
}

package diagram

import "math"

// Scale is a layout's cells in a picture's units: Cell wide, Line tall, and
// Margin round the whole. A picture is drawn through the middles of the
// cells the text drawing would put its characters in, so the two drawings
// of one layout agree -- an outline along its outermost cells, a line along
// the cells it passes through.
//
// The picture renderers share it, so that a PNG and an SVG of one layout
// agree too.
type Scale struct {
	Cell, Line, Margin float64
}

// ScaleFor is the scale a picture with text px tall is drawn at: a cell a
// letter of Go Mono wide -- 1229/2048 of its size, as near as makes no
// difference the width of every common monospaced font's letters -- a line
// and a half tall, and a margin of two letters.
func ScaleFor(px float64) Scale {
	return Scale{Cell: px * 1229 / 2048, Line: px * 1.5, Margin: px * 2}
}

// Size is the picture's size for lay.
func (s Scale) Size(lay *Layout) (w, h float64) {
	return 2*s.Margin + float64(lay.Width)*s.Cell, 2*s.Margin + float64(lay.Height)*s.Line
}

// X is the middle of column x.
func (s Scale) X(x int) float64 { return s.Margin + (float64(x)+0.5)*s.Cell }

// Y is the middle of row y.
func (s Scale) Y(y int) float64 { return s.Margin + (float64(y)+0.5)*s.Line }

// Left is the left edge of column x, where text written from it begins.
func (s Scale) Left(x int) float64 { return s.Margin + float64(x)*s.Cell }

// Box is the outline of r: through the middles of its outermost cells.
func (s Scale) Box(r Rect) (x0, y0, x1, y1 float64) {
	return s.X(r.X), s.Y(r.Y), s.X(r.X + r.W - 1), s.Y(r.Y + r.H - 1)
}

// Lid is the height of a cylinder's lid, the ellipse its top is drawn as,
// from its top to its middle: half the cylinder's first row and half its
// second, where the text drawing draws the lid's line.
func (s Scale) Lid() float64 { return s.Line / 2 }

// Path is the edge's path in the picture's units: through the middles of
// its cells, with each end on its node's outline -- moved in from the box
// around an ellipse or the lid of a cylinder to the curve itself, so a line
// meets the shape rather than stopping short of it.
func (s Scale) Path(lay *Layout, e PlacedEdge) (xs, ys []float64) {
	for _, p := range e.Points {
		xs, ys = append(xs, s.X(p.X)), append(ys, s.Y(p.Y))
	}
	n := len(xs)
	if n < 2 {
		return xs, ys
	}
	if from, ok := lay.Node(e.Edge.From); ok {
		xs[0], ys[0] = s.onOutline(from, xs[0], ys[0], xs[1], ys[1])
	}
	if to, ok := lay.Node(e.Edge.To); ok {
		xs[n-1], ys[n-1] = s.onOutline(to, xs[n-1], ys[n-1], xs[n-2], ys[n-2])
	}
	return xs, ys
}

// onOutline is the end (x, y) of a line arriving from (fx, fy), moved onto
// the outline of n where n is curved there.
func (s Scale) onOutline(n PlacedNode, x, y, fx, fy float64) (float64, float64) {
	x0, y0, x1, y1 := s.Box(n.Rect)
	cx, cy, rx, ry := (x0+x1)/2, (y0+y1)/2, (x1-x0)/2, (y1-y0)/2
	vertical := x == fx
	switch n.Node.Shape {
	case Ellipse:
		if vertical {
			d := ry * math.Sqrt(max(0, 1-sq((x-cx)/rx)))
			if fy < cy {
				return x, cy - d
			}
			return x, cy + d
		}
		d := rx * math.Sqrt(max(0, 1-sq((y-cy)/ry)))
		if fx < cx {
			return cx - d, y
		}
		return cx + d, y
	case Cylinder:
		if !vertical {
			return x, y
		}
		lid := s.Lid()
		d := lid * math.Sqrt(max(0, 1-sq((x-cx)/rx)))
		if fy < cy {
			return x, y0 + lid - d
		}
		return x, y1 - lid + d
	}
	return x, y
}

func sq(x float64) float64 { return x * x }

// Straight is a straight edge's line in the picture's units: from the
// middle of its tail node towards the middle of its head, Shift half rows
// to one side, cut where it leaves the one's outline and meets the other's
// -- an ellipse's curve and a cylinder's lid included.
func (s Scale) Straight(lay *Layout, e PlacedEdge) (xs, ys []float64) {
	from, _ := lay.Node(e.Edge.From)
	to, _ := lay.Node(e.Edge.To)
	ax, ay := s.middle(from.Rect)
	bx, by := s.middle(to.Rect)
	if e.Shift != 0 {
		// To the side of the line from the node of the lesser ID to the
		// other, so that a to b and b to a are set apart, not on one line.
		ux, uy := bx-ax, by-ay
		if e.Edge.To < e.Edge.From {
			ux, uy = -ux, -uy
		}
		if l := math.Hypot(ux, uy); l > 0 {
			d := e.Shift * s.Line / 2
			px, py := -uy/l*d, ux/l*d
			ax, ay, bx, by = ax+px, ay+py, bx+px, by+py
		}
	}
	x0, y0 := s.leave(from, ax, ay, bx, by)
	x1, y1 := s.leave(to, bx, by, ax, ay)
	return []float64{x0, x1}, []float64{y0, y1}
}

func (s Scale) middle(r Rect) (x, y float64) {
	x0, y0, x1, y1 := s.Box(r)
	return (x0 + x1) / 2, (y0 + y1) / 2
}

// leave is where the line from (x, y), inside n, towards (tx, ty) crosses
// n's outline.
func (s Scale) leave(n PlacedNode, x, y, tx, ty float64) (float64, float64) {
	x0, y0, x1, y1 := s.Box(n.Rect)
	dx, dy := tx-x, ty-y
	cx, rx := (x0+x1)/2, (x1-x0)/2
	switch n.Node.Shape {
	case Ellipse:
		if t, ok := exitEllipse(x, y, dx, dy, cx, (y0+y1)/2, rx, (y1-y0)/2); ok {
			return x + t*dx, y + t*dy
		}
	case Cylinder:
		// Through the lid or the base where it leaves by the top or the
		// bottom, as those are drawn as halves of ellipses.
		t := exitBox(x, y, dx, dy, x0, y0, x1, y1)
		ey := y + t*dy
		lid := s.Lid()
		if ey <= y0+1e-9 {
			if u, ok := exitEllipse(x, y, dx, dy, cx, y0+lid, rx, lid); ok {
				t = u
			}
		} else if ey >= y1-1e-9 {
			if u, ok := exitEllipse(x, y, dx, dy, cx, y1-lid, rx, lid); ok {
				t = u
			}
		}
		return x + t*dx, y + t*dy
	}
	t := exitBox(x, y, dx, dy, x0, y0, x1, y1)
	return x + t*dx, y + t*dy
}

// exitBox is how far along (dx, dy) from (x, y), inside the box, the line
// leaves it, as a fraction of (dx, dy).
func exitBox(x, y, dx, dy, x0, y0, x1, y1 float64) float64 {
	t := math.Inf(1)
	if dx > 0 {
		t = min(t, (x1-x)/dx)
	} else if dx < 0 {
		t = min(t, (x0-x)/dx)
	}
	if dy > 0 {
		t = min(t, (y1-y)/dy)
	} else if dy < 0 {
		t = min(t, (y0-y)/dy)
	}
	if math.IsInf(t, 1) {
		return 0
	}
	return max(t, 0)
}

// exitEllipse is how far along (dx, dy) from (x, y) the line last crosses
// the ellipse about (cx, cy) with radii rx and ry, as a fraction of (dx,
// dy); false where it does not cross it ahead.
func exitEllipse(x, y, dx, dy, cx, cy, rx, ry float64) (float64, bool) {
	if rx <= 0 || ry <= 0 {
		return 0, false
	}
	px, py := (x-cx)/rx, (y-cy)/ry
	qx, qy := dx/rx, dy/ry
	a := qx*qx + qy*qy
	b := 2 * (px*qx + py*qy)
	c := px*px + py*py - 1
	disc := b*b - 4*a*c
	if a == 0 || disc < 0 {
		return 0, false
	}
	t := (-b + math.Sqrt(disc)) / (2 * a)
	return t, t >= 0
}

// Route is the line a picture draws an edge along: Straight where the
// edge is drawn straight, Path otherwise.
func (s Scale) Route(lay *Layout, e PlacedEdge) (xs, ys []float64) {
	if e.Straight {
		return s.Straight(lay, e)
	}
	return s.Path(lay, e)
}

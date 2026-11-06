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

// Lid is half the height of a cylinder's lid, the ellipse its top is
// drawn as: from its top, the middle of the cylinder's first row, to its
// middle, half a row down. The whole ellipse runs on to the middle of the
// second row, where the text drawing draws the lid's line.
// The layout asks it of a cylinder's shape in cells (within).
func (s Scale) Lid() float64 { return s.Line / 2 }

// Corner is the radius of a Rounded node's corners, drawn as quarters of
// circles: two fifths of a row, and no more than half the box's shorter
// side. The layout asks it of a rounded box's shape in cells (within).
//
// No more than a cell wide, and at ScaleFor's proportions a hair less:
// a line along rows and columns ends a cell or more from a corner, and
// Path cuts it at the box, so it meets a side's straight part only while
// the arc reaches no further along the side than that. A larger radius
// would end those lines beside the arc, as straight lines were before
// leave cut them at it.
func (s Scale) Corner(r Rect) float64 {
	x0, y0, x1, y1 := s.Box(r)
	return min(s.Line*2/5, (x1-x0)/2, (y1-y0)/2)
}

// inCorner is the middle of the corner whose square, r on a side, (x, y)
// is in, of the box from (x0, y0) to (x1, y1), and false where it is in
// none: by a side, or inside.
func inCorner(x, y, x0, y0, x1, y1, rx, ry float64) (cx, cy float64, ok bool) {
	switch {
	case x < x0+rx:
		cx = x0 + rx
	case x > x1-rx:
		cx = x1 - rx
	default:
		return 0, 0, false
	}
	switch {
	case y < y0+ry:
		cy = y0 + ry
	case y > y1-ry:
		cy = y1 - ry
	default:
		return 0, 0, false
	}
	return cx, cy, true
}

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
	switch n.Node.Outline() {
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
// middle of its tail node towards the middle of its head, Shift rows
// to one side, cut where it leaves the one's outline and meets the
// other's as drawn, as leave finds it -- an ellipse's curve, a cylinder's
// lid and base, by a side too where the line leaves above the lid's
// middle or below the base's, and a rounded box's arcs included.
func (s Scale) Straight(lay *Layout, e PlacedEdge) (xs, ys []float64) {
	from, _ := lay.Node(e.Edge.From)
	to, _ := lay.Node(e.Edge.To)
	ax, ay := s.middle(from.Rect)
	bx, by := s.middle(to.Rect)
	if e.Shift != 0 {
		// To the side of the line from the node of the lesser ID to the
		// other, so that a to b and b to a are set apart, not on one line.
		ux, uy := bx-ax, by-ay
		if backwards(e.Edge) {
			ux, uy = -ux, -uy
		}
		if l := math.Hypot(ux, uy); l > 0 {
			// A row apart, so the text drawing, in which a row is four
			// dots, draws the two in cells of their own.
			d := e.Shift * s.Line
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
	switch n.Node.Outline() {
	case Ellipse:
		if t, ok := exitEllipse(x, y, dx, dy, cx, (y0+y1)/2, rx, (y1-y0)/2); ok {
			return x + t*dx, y + t*dy
		}
	case Rounded:
		// Through a corner's arc where it leaves the box within a corner's
		// square, as the corners are drawn round: cut at the box, a line
		// aimed near a corner was drawn from where the square corner would
		// be, beside the arc.
		t := exitBox(x, y, dx, dy, x0, y0, x1, y1)
		r := s.Corner(n.Rect)
		if ccx, ccy, ok := inCorner(x+t*dx, y+t*dy, x0, y0, x1, y1, r, r); ok {
			if u, ok := exitEllipse(x, y, dx, dy, ccx, ccy, r, r); ok {
				t = u
			}
		}
		return x + t*dx, y + t*dy
	case Cylinder:
		// Through the lid where it leaves the box above the lid's middle,
		// and the base below the base's, as those are drawn as halves of
		// ellipses and the sides only between their middles. Taken through
		// the lid only where it left by the top, a line leaving by a side
		// above the lid's middle, as one between the middles of two
		// cylinders side by side but a row apart does, was drawn from
		// beside the curve.
		t := exitBox(x, y, dx, dy, x0, y0, x1, y1)
		ey := y + t*dy
		lid := s.Lid()
		if ey < y0+lid {
			if u, ok := exitEllipse(x, y, dx, dy, cx, y0+lid, rx, lid); ok {
				t = u
			}
		} else if ey > y1-lid {
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

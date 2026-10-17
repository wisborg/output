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

package diagram

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

// parallelGrid is a grid of nine nodes, three by three, with labels of
// different sizes, and edges of which several run between the same two
// nodes, as the picture sets apart: the case both promises below once
// failed in, and which randomDiagram seldom makes. All boxes, or, shaped,
// nodes of every shape. A grid of boxes takes no number for its shapes,
// so a seed's grid of boxes is the one it made before there were shaped
// grids, among which the promises below once failed; a shaped grid of
// the same seed is another grid, its labels and edges drawn from the
// numbers after its first shape.
func parallelGrid(rnd *rand.Rand, shaped bool) *Diagram {
	labels := []string{"a", "ab", "abc", "wide label", "x\ny", "a\nb\nc"}
	shapes := []Shape{Box, Rounded, Ellipse, Cylinder}
	d := &Diagram{Straight: true, Root: Group{Columns: 3}}
	for i := range 9 {
		n := &Node{ID: fmt.Sprint("n", i), Label: labels[rnd.Intn(len(labels))]}
		if shaped {
			n.Shape = shapes[rnd.Intn(len(shapes))]
		}
		d.Root.Items = append(d.Root.Items, Item{Node: n})
	}
	for range 1 + rnd.Intn(3) {
		from, to := rnd.Intn(9), rnd.Intn(9)
		if from == to {
			continue
		}
		for range 2 + rnd.Intn(6) {
			d.Edges = append(d.Edges, Edge{From: fmt.Sprint("n", from), To: fmt.Sprint("n", to)})
		}
	}
	for range rnd.Intn(6) {
		d.Edges = append(d.Edges, Edge{From: fmt.Sprint("n", rnd.Intn(9)), To: fmt.Sprint("n", rnd.Intn(9))})
	}
	return d
}

// A straight line set apart from the others between its nodes is held to
// what any straight line is: clear of every box but its own two, as the
// picture draws it -- checked between the middles alone, a line drawn up to
// three rows aside crossed a third box -- and no path along rows and
// columns runs along it: kept clear of the line between the middles, a
// path ran along one drawn half a row or more beside it, and the picture
// drew the two as one -- 154 times in 3,000 such grids, where a line
// between the middles was run along 4 times, each where no other way was.
// It begins and ends inside its boxes, off their outlines, as the picture
// draws it from where it leaves a box, worked out from a point inside:
// set apart further, it was drawn from a box's corner or from beside it.
// And the picture draws it from its shape's outline, as drawn: an
// ellipse's curve, a cylinder's sides, lid and base, a rounded box's
// arcs. Held inside the rectangle alone, a line set apart from an
// ellipse's middle began in the rectangle's corner, beside the curve. The
// grids of boxes the first promises were found in, and as many again of
// every shape.
//
// The line is judged as drawn, not as the router records it: between its
// boxes, on the row it runs along, or on both rows where it runs between
// two, as rowsOf says, and the same of a column.
func TestSetApartStraightLines(t *testing.T) {
	shifted, curved := 0, 0
	s := ScaleFor(16)
	for k := range int64(800) {
		// Named in a failure as parallelGrid is called, so that it
		// reproduces: the seed, and shaped or not.
		seed, shaped := k/2, k%2 == 1
		d := parallelGrid(rand.New(rand.NewSource(seed)), shaped)
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		// The cells of the lines along a row, and of those along a column.
		on := [2]map[Point]bool{{}, {}}
		for _, e := range lay.Edges {
			if !e.Straight {
				continue
			}
			a, _ := lay.Node(e.Edge.From)
			b, _ := lay.Node(e.Edge.To)
			x0, y0, x1, y1 := lay.ends(e)
			if lay.lineCrossesBox(a, b, x0, y0, x1, y1) {
				t.Fatalf("seed %d, shaped %v: %s -> %s, set %v aside, crosses a box", seed, shaped, e.Edge.From, e.Edge.To, e.Shift)
			}
			if e.Shift != 0 {
				shifted++
			}
			xs, ys := s.Straight(lay, e)
			for k, n := range []PlacedNode{a, b} {
				x, y := xs[k*(len(xs)-1)], ys[k*(len(ys)-1)]
				if !onDrawnOutline(s, n, x, y) {
					t.Fatalf("seed %d, shaped %v: %s -> %s, set %v aside, is drawn from (%.1f, %.1f), off the outline of %s, %v drawn from %v", seed, shaped, e.Edge.From, e.Edge.To, e.Shift, x, y, n.Node.ID, n.Node.Shape, n.Rect)
				}
				// Drawn from a curve: inside the rectangle, off its sides.
				if bx0, by0, bx1, by1 := s.Box(n.Rect); e.Shift != 0 && x > bx0+1e-6 && x < bx1-1e-6 && y > by0+1e-6 && y < by1-1e-6 {
					curved++
				}
			}
			for _, c := range []struct {
				r    Rect
				x, y float64
			}{{a.Rect, x0, y0}, {b.Rect, x1, y1}} {
				if e.Shift != 0 && (c.x <= float64(c.r.X) || c.x >= float64(c.r.X+c.r.W-1) || c.y <= float64(c.r.Y) || c.y >= float64(c.r.Y+c.r.H-1)) {
					t.Fatalf("seed %d, shaped %v: %s -> %s, set %v aside, ends at (%v, %v), not inside its box %v", seed, shaped, e.Edge.From, e.Edge.To, e.Shift, c.x, c.y, c.r)
				}
			}
			switch {
			case y0 == y1:
				lo, hi := a.Rect, b.Rect
				if hi.X < lo.X {
					lo, hi = hi, lo
				}
				for _, y := range rowsOf(y0) {
					for x := lo.X + lo.W; x < hi.X; x++ {
						on[0][Point{x, y}] = true
					}
				}
			case x0 == x1:
				lo, hi := a.Rect, b.Rect
				if hi.Y < lo.Y {
					lo, hi = hi, lo
				}
				for _, x := range rowsOf(x0) {
					for y := lo.Y + lo.H; y < hi.Y; y++ {
						on[1][Point{x, y}] = true
					}
				}
			}
		}
		for _, e := range lay.Edges {
			// Not a path found only where joining another's line was
			// allowed, at joinCost, as no other way was free: a crowded
			// grid of these has such paths, and they may join any line.
			if e.Straight || e.Edge.Invisible || e.joined || e.plain || len(e.Points) == 0 {
				continue
			}
			prev := e.Points[0]
			for k := 1; k < len(e.Points); k++ {
				for p := range cellsAfter(e.Points[k-1], e.Points[k]) {
					// A step along a row on a line along one, or down a
					// column on one along that: crossing lines side by
					// side, a step from one to the next, is no fault.
					along := on[0]
					if p.X == prev.X {
						along = on[1]
					}
					if along[p] && along[prev] {
						t.Fatalf("seed %d, shaped %v: %s -> %s runs along a straight line at %v", seed, shaped, e.Edge.From, e.Edge.To, p)
					}
					prev = p
				}
			}
		}
	}
	if shifted == 0 || curved == 0 {
		t.Fatalf("%d straight lines set apart, %d ends of them drawn from a curve: the test tests too little", shifted, curved)
	}
}

// onDrawnOutline says (x, y), in the picture's units, is on n's outline
// as the picture draws it: an ellipse's curve; a cylinder's sides between
// its lid's middle and its base's, or the outer half of either; a rounded
// box's sides between its corners and the arcs that round them; a box's
// four sides. Worked out from the picture's own measures, Box, Lid and
// Corner, and not from the layout's test of a point inside a shape, so
// that it judges that test too.
func onDrawnOutline(s Scale, n PlacedNode, x, y float64) bool {
	const eps = 1e-6
	x0, y0, x1, y1 := s.Box(n.Rect)
	cx, rx := (x0+x1)/2, (x1-x0)/2
	near := func(a, b float64) bool { return math.Abs(a-b) < eps*max(1, math.Abs(b)) }
	onEllipse := func(cy, ry float64) bool { return near(sq((x-cx)/rx)+sq((y-cy)/ry), 1) }
	between := func(v, lo, hi float64) bool { return v > lo-eps && v < hi+eps }
	switch n.Node.Outline() {
	case Ellipse:
		return onEllipse((y0+y1)/2, (y1-y0)/2)
	case Cylinder:
		lid := s.Lid()
		side := (near(x, x0) || near(x, x1)) && between(y, y0+lid, y1-lid)
		return side || onEllipse(y0+lid, lid) && y <= y0+lid+eps || onEllipse(y1-lid, lid) && y >= y1-lid-eps
	case Rounded:
		r := s.Corner(n.Rect)
		sides := (near(x, x0) || near(x, x1)) && between(y, y0+r, y1-r) || (near(y, y0) || near(y, y1)) && between(x, x0+r, x1-r)
		for _, c := range [4][2]float64{{x0 + r, y0 + r}, {x1 - r, y0 + r}, {x0 + r, y1 - r}, {x1 - r, y1 - r}} {
			outer := (x-c[0])*(c[0]-cx) >= -eps && (y-c[1])*(c[1]-(y0+y1)/2) >= -eps
			if outer && near(math.Hypot(x-c[0], y-c[1]), r) {
				return true
			}
		}
		return sides
	}
	return (near(x, x0) || near(x, x1)) && between(y, y0, y1) || (near(y, y0) || near(y, y1)) && between(x, x0, x1)
}

// rowsOf is the rows, or the columns, a line along one at v is drawn on:
// every one less than one away from it, the row or the column v is at
// where it is a whole one, and both either side where it is not. A path
// along the column beside a line in a column's middle runs a column from
// it, and is no fault; one along a column three quarters of a column from
// a line set apart, as a line half a row aside is, runs nearer it than
// that, and nearer than a path along a row runs to a line half way
// between two rows, which is one.
func rowsOf(v float64) []int {
	if f := math.Floor(v); v != f {
		return []int{int(f), int(f) + 1}
	}
	return []int{int(v)}
}

// cellsAfter yields the cells from a to b along a row or a column, b
// included and a not, so that a path's corner is met once.
func cellsAfter(a, b Point) func(func(Point) bool) {
	return func(yield func(Point) bool) {
		for p := a; p != b; {
			p = step(p, b)
			if !yield(p) {
				return
			}
		}
	}
}

// within is the shape as the picture draws it: it takes a point inside
// the outline as drawn, by more than a hair, and no point outside it; and
// from any point it takes, the picture's line, whichever way it runs,
// leaves from the outline as drawn. Of every shape and of sizes from two
// cells to a dozen, at points all over each box, as random layouts seldom
// set a line's end in a cylinder's corner -- held to the rectangle there,
// 20,000 random grids made one -- and never yet in one by its base. Both
// ways, as one too strict draws no line wrong but draws fewer lines
// straight, which no other test would see.
func TestWithinIsTheShapeDrawn(t *testing.T) {
	s := ScaleFor(16)
	for _, shape := range []Shape{Box, Rounded, Ellipse, Cylinder, Plain} {
		inside, outside := 0, 0
		// From two cells a side, smaller than any node is laid out, where a
		// rounded box's corners are held to half its side.
		for w := 2; w <= 12; w++ {
			for h := 2; h <= 6; h++ {
				n := PlacedNode{Node: &Node{ID: "n", Shape: shape}, Rect: Rect{X: 4, Y: 3, W: w, H: h}}
				// Counted in tenths, not added up: a tenth added twenty
				// times is a hair short of two, a point a hair inside a
				// side, from which a line runs down the side itself.
				for i := 0; i <= 10*(w-1); i++ {
					for j := 0; j <= 10*(h-1); j++ {
						x, y := float64(n.Rect.X)+float64(i)/10, float64(n.Rect.Y)+float64(j)/10
						// In the picture's units, as Scale.Straight asks.
						px, py := s.Margin+(x+0.5)*s.Cell, s.Margin+(y+0.5)*s.Line
						in := within(n, x, y)
						switch {
						case in && !insideDrawn(s, n, px, py, 0):
							t.Fatalf("%v %dx%d: (%.2f, %.2f) is within it, and outside it as drawn", shape, w, h, x, y)
						case !in && insideDrawn(s, n, px, py, 1e-6):
							t.Fatalf("%v %dx%d: (%.2f, %.2f) is inside it as drawn, and not within it", shape, w, h, x, y)
						case !in:
							if insideDrawn(s, n, px, py, -1e-6) || shape == Box || shape == Plain {
								continue // on the outline
							}
							outside++
							continue
						}
						inside++
						for k := range 12 {
							a := float64(k) * math.Pi / 6
							ex, ey := s.leave(n, px, py, px+1000*math.Cos(a), py+1000*math.Sin(a))
							if !onDrawnOutline(s, n, ex, ey) {
								t.Fatalf("%v %dx%d: a line from (%.2f, %.2f), within it, at %d degrees leaves from (%.1f, %.1f), off its outline", shape, w, h, x, y, k*30, ex, ey)
							}
						}
					}
				}
			}
		}
		// Points of the box's outside its curve, which the curve is for.
		if inside == 0 || (shape == Ellipse || shape == Cylinder || shape == Rounded) && outside == 0 {
			t.Fatalf("%v: %d points within, %d inside the box but outside the curve: the test tests too little", shape, inside, outside)
		}
	}
}

// insideDrawn says (x, y), in the picture's units, is inside n's shape as
// the picture draws it by more than d -- or, d less than nought, inside it
// or no further out than -d: an ellipse's curve, a cylinder's lid and
// base, a rounded box's corners' arcs, and the box. From Box, Lid and
// Corner, as onDrawnOutline is.
func insideDrawn(s Scale, n PlacedNode, x, y, d float64) bool {
	x0, y0, x1, y1 := s.Box(n.Rect)
	box := func(x0, y0, x1, y1 float64) bool {
		return x > x0+d && x < x1-d && y > y0+d && y < y1-d
	}
	cx, cy, rx, ry := (x0+x1)/2, (y0+y1)/2, (x1-x0)/2, (y1-y0)/2
	switch n.Node.Outline() {
	case Ellipse:
		return math.Hypot((x-cx)/rx, (y-cy)/ry) < 1-d/max(rx, ry)
	case Cylinder:
		lid := s.Lid()
		u := min(1, math.Abs(x-cx)/rx)
		c := lid * math.Sqrt(1-u*u)
		return box(x0, y0+lid-c, x1, y1-lid+c)
	case Rounded:
		r := s.Corner(n.Rect)
		for _, c := range [4][2]float64{{x0 + r, y0 + r}, {x1 - r, y0 + r}, {x0 + r, y1 - r}, {x1 - r, y1 - r}} {
			if (x-c[0])*(c[0]-cx) > 0 && (y-c[1])*(c[1]-cy) > 0 {
				return math.Hypot(x-c[0], y-c[1]) < r-d
			}
		}
	}
	return box(x0, y0, x1, y1)
}

// A rounded box's corner is no wider than a cell at any proportions, as
// a line along rows and columns ending a cell from the corner meets the
// side's straight part only so; at ScaleFor's, two fifths of a row is a
// hair less than a cell already, and no other test sees the cap.
func TestCornerNoWiderThanACell(t *testing.T) {
	r := Rect{W: 10, H: 10}
	if c := (Scale{Cell: 6, Line: 24}).Corner(r); c != 6 {
		t.Errorf("a cell 6 wide and a row 24 tall: a corner of %v, want 6", c)
	}
	if s := ScaleFor(16); s.Corner(r) != s.Line*2/5 {
		t.Errorf("at ScaleFor's proportions: a corner of %v, want two fifths of a row, %v", s.Corner(r), s.Line*2/5)
	}
}

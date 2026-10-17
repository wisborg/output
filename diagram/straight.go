package diagram

import (
	"math"
	"strings"
)

// The layout's straight edges are worked out in cell coordinates, a cell's
// middle at its column and row: the coordinates the picture renderers scale,
// so that a line clear of a box here is clear of it in the picture.

// cellAspect is a cell's width over its height in a picture: a letter of
// the monospaced font, over a line and a half of text.
const cellAspect = 1229.0 / 2048 / 1.5

// centre is the middle of r, in cell coordinates.
func centre(r Rect) (x, y float64) {
	return float64(r.X) + float64(r.W-1)/2, float64(r.Y) + float64(r.H-1)/2
}

// straightens says edge e is drawn straight in a picture: its line from
// the middle of one node to the middle of the other is clear of every
// other node. A loop keeps its loop.
func (lay *Layout) straightens(e Edge) bool {
	if e.From == e.To {
		return false
	}
	a, _ := lay.Node(e.From)
	b, _ := lay.Node(e.To)
	return !lay.crossesBox(a, b)
}

// straighten sets the straight edges between the same two nodes apart,
// and places the labels as the picture draws them.
func (lay *Layout) straighten() {
	pairs := map[[2]string][]int{}
	for i := range lay.Edges {
		e := &lay.Edges[i]
		if !e.Straight {
			continue
		}
		key := [2]string{e.Edge.From, e.Edge.To}
		if key[1] < key[0] {
			key[0], key[1] = key[1], key[0]
		}
		pairs[key] = append(pairs[key], i)
	}
	for _, edges := range pairs {
		for k, i := range edges {
			lay.Edges[i].Shift = float64(k) - float64(len(edges)-1)/2
		}
	}
	lay.placeStraightLabels()
}

// crossesBox says the line between the middles of a and b passes through
// a node other than the two.
func (lay *Layout) crossesBox(a, b PlacedNode) bool {
	x0, y0 := centre(a.Rect)
	x1, y1 := centre(b.Rect)
	for _, n := range lay.Nodes {
		if n.Node == a.Node || n.Node == b.Node {
			continue
		}
		r := n.Rect
		if segmentMeetsBox(x0, y0, x1, y1, float64(r.X)-0.5, float64(r.Y)-0.5, float64(r.X+r.W)-0.5, float64(r.Y+r.H)-0.5) {
			return true
		}
	}
	return false
}

// segmentMeetsBox says the segment from (x0, y0) to (x1, y1) touches the
// box from (bx0, by0) to (bx1, by1): Liang and Barsky's clipping, which
// narrows the part of the segment inside each side in turn.
func segmentMeetsBox(x0, y0, x1, y1, bx0, by0, bx1, by1 float64) bool {
	t0, t1 := 0.0, 1.0
	dx, dy := x1-x0, y1-y0
	for _, c := range [4][2]float64{{-dx, x0 - bx0}, {dx, bx1 - x0}, {-dy, y0 - by0}, {dy, by1 - y0}} {
		p, q := c[0], c[1]
		if p == 0 {
			if q < 0 {
				return false
			}
			continue
		}
		t := q / p
		if p < 0 {
			if t > t1 {
				return false
			}
			t0 = max(t0, t)
		} else {
			if t < t0 {
				return false
			}
			t1 = min(t1, t)
		}
	}
	return t0 <= t1
}

// ends is where e's straight line runs, in cell coordinates: between its
// nodes' middles, Shift rows to one side as Scale draws it -- the
// same turn, from the node of the lesser ID to the other, worked in the
// picture's proportions and brought back to cells.
func (lay *Layout) ends(e PlacedEdge) (x0, y0, x1, y1 float64) {
	a, _ := lay.Node(e.Edge.From)
	b, _ := lay.Node(e.Edge.To)
	x0, y0 = centre(a.Rect)
	x1, y1 = centre(b.Rect)
	if e.Shift == 0 {
		return x0, y0, x1, y1
	}
	ux, uy := (x1-x0)*cellAspect, y1-y0
	if e.Edge.To < e.Edge.From {
		ux, uy = -ux, -uy
	}
	l := math.Hypot(ux, uy)
	if l == 0 {
		return x0, y0, x1, y1
	}
	d := e.Shift
	ox, oy := -uy/l*d/cellAspect, ux/l*d
	return x0 + ox, y0 + oy, x1 + ox, y1 + oy
}

// lineCells are the columns and rows a slanting line through (x, y) is
// taken to fill: both columns either side of x, as columns are narrow and
// the line passes between them; the row nearest y, and the next as well
// only where it runs near the line between the two, as rows are tall. A
// label beside the line is written clear of them.
func lineCells(x, y float64) (c0, c1, r0, r1 int) {
	return int(math.Floor(x)), int(math.Ceil(x)), int(math.Round(y - 0.2)), int(math.Round(y + 0.2))
}

// alongStraight calls f at points along e's line, from end to end, four to
// a cell, as the picture draws it.
func (lay *Layout) alongStraight(e PlacedEdge, f func(x, y float64)) {
	x0, y0, x1, y1 := lay.ends(e)
	steps := int(4*max(math.Abs(x1-x0), math.Abs(y1-y0))) + 1
	for k := 0; k <= steps; k++ {
		t := float64(k) / float64(steps)
		f(x0+t*(x1-x0), y0+t*(y1-y0))
	}
}

// straightCells is the cells e's straight line takes: at each point
// along it, those lineCells says it fills.
func (lay *Layout) straightCells(e PlacedEdge) map[Point]bool {
	cells := map[Point]bool{}
	lay.alongStraight(e, func(x, y float64) {
		c0, c1, r0, r1 := lineCells(x, y)
		for cx := c0; cx <= c1; cx++ {
			for cy := r0; cy <= r1; cy++ {
				cells[Point{cx, cy}] = true
			}
		}
	})
	return cells
}

// placeStraightLabels places the labels where a picture with straight
// edges writes them, clear of the boxes, the groups' lines and labels, the
// lines as the picture draws them and the labels placed already: a
// straight edge's beside the middle of its line, any other's as the text
// drawing places it, but against the picture's lines.
//
// A label may go above the first row -- over a line between two boxes of
// the top row, too close together for it to go between them -- and the
// layout then grows at the top to hold it. Not above a title, which is
// first whatever else is.
func (lay *Layout) placeStraightLabels() {
	top := 4
	if lay.Title != "" {
		top = 0
	}
	o := newOccupancy(lay.Width+40, lay.Height+4, top)
	o.take(Point{0, 0}, []string{lay.Title})
	for _, n := range lay.Nodes {
		o.rect(n.Rect, true)
	}
	for _, g := range lay.Groups {
		for _, d := range g.Dividers {
			o.line(d[0], d[1])
		}
		if g.Group.Style == Frame {
			o.rect(g.Rect, false)
		}
		if g.Group.Label != "" && g.Group.Style != Hidden {
			o.take(g.LabelAt, []string{" " + g.Group.Label + " "})
		}
	}
	// What is there but the lines: the title, the boxes, the groups'
	// outlines and labels, and the edges' labels as they are placed. A
	// label on its own line is excused the line's cells, never these.
	fixed := o.clone()
	var drawn []PlacedEdge // the edges the picture draws along their paths
	for _, e := range lay.Edges {
		if !e.Straight {
			for i := 1; i < len(e.Points); i++ {
				o.line(e.Points[i-1], e.Points[i])
			}
			drawn = append(drawn, e)
			continue
		}
		for p := range lay.straightCells(e) {
			o.mark(p.X, p.Y)
		}
	}
	shared := edgeCells(drawn)
	// The cells each edge's line takes, as marked above: a straight one's
	// over the cells either side of it, as the picture draws it, so that
	// a label on its own line is excused exactly the cells it took.
	taken := make([]map[Point]bool, len(lay.Edges))
	// The cells each edge's line, as the picture draws it, passes through:
	// to judge whose line a place is nearest.
	lineAt := make([][]Point, len(lay.Edges))
	for i, e := range lay.Edges {
		if !e.Straight {
			taken[i] = cellsOfPath(e.Points)
			for p := range taken[i] {
				lineAt[i] = append(lineAt[i], p)
			}
			continue
		}
		taken[i] = lay.straightCells(e)
		seen := map[Point]bool{}
		lay.alongStraight(e, func(x, y float64) {
			p := Point{int(math.Round(x)), int(math.Round(y))}
			if !seen[p] && !lay.inNode(p) {
				seen[p] = true
				lineAt[i] = append(lineAt[i], p)
			}
		})
	}
	for i := range lay.Edges {
		e := &lay.Edges[i]
		if e.Edge.Label == "" {
			continue
		}
		lines := strings.Split(e.Edge.Label, "\n")
		w := 0
		for _, l := range lines {
			w = max(w, Width(l))
		}
		if !e.Straight {
			e.pictureAt, e.pictured = lay.placeOn(o, segmentTries(e.Points, shared, w, len(lines)), lines, w, top)
			fixed.take(e.pictureAt, lines)
			continue
		}
		a, _ := lay.Node(e.Edge.From)
		b, _ := lay.Node(e.Edge.To)
		x0, y0, x1, y1 := lay.ends(*e)
		dx, dy := math.Abs(x1-x0), math.Abs(y1-y0)
		steep := dy > dx*cellAspect
		// Steeper than flat, as drawn: beside the line, clear of the cells
		// it crosses in a row. Flatter: over or under it, clear of its rise
		// across the label's width.
		side := int(math.Ceil(dx/max(dy, 1)/2)) + 1
		rise := int(math.Round(dy / max(dx, 1) * float64(w) / 2))
		// The part of the line between the outlines, which is what is seen
		// of it: from where it leaves the one box to where it enters the
		// other.
		ta := exitBox(x0, y0, x1-x0, y1-y0, float64(a.Rect.X)-0.5, float64(a.Rect.Y)-0.5, float64(a.Rect.X+a.Rect.W)-0.5, float64(a.Rect.Y+a.Rect.H)-0.5)
		tb := 1 - exitBox(x1, y1, x0-x1, y0-y1, float64(b.Rect.X)-0.5, float64(b.Rect.Y)-0.5, float64(b.Rect.X+b.Rect.W)-0.5, float64(b.Rect.Y+b.Rect.H)-0.5)
		// A line drawn to one side of another between the same two nodes
		// is labelled on its own side, outside the pair, so each label is
		// by its line and not across the other. The side is Scale's: from
		// the node of the lesser ID to the other, turned a right angle.
		ux, uy := x1-x0, y1-y0
		if e.Edge.To < e.Edge.From {
			ux, uy = -ux, -uy
		}
		// Scale moves the line by (-uy, ux) times Shift: right of a steep
		// line where -uy*Shift is positive, above a flat one where ux*Shift
		// is negative. The first try is right, or above.
		flip := steep && -uy*e.Shift < 0 || !steep && ux*e.Shift > 0
		var tries []Point
		// From the middle of that out along it to its ends, by the arrowhead
		// too, each side in turn: placeNear chooses among them.
		for _, f := range []float64{0.5, 0.4, 0.6, 0.3, 0.7, 0.2, 0.8, 0.9, 0.1, 1, 0} {
			t := ta + f*(tb-ta)
			// From the cells either side of the point, which the line is
			// taken to fill, outwards.
			fx, fy := x0+t*(x1-x0), y0+t*(y1-y0)
			lx, hx, ly, hy := lineCells(fx, fy)
			py, px := int(math.Round(fy)), int(math.Round(fx))
			var first, second Point
			if steep {
				first, second = Point{hx + side, py}, Point{lx - side - w + 1, py}
			} else {
				first, second = Point{px - w/2, ly - len(lines) - rise}, Point{px - w/2, hy + 1 + rise}
			}
			if flip {
				first, second = second, first
			}
			tries = append(tries, first, second)
		}
		// A label longer than the line has room for, between two boxes
		// close together, goes further out -- past the boxes' ends -- not
		// over them.
		near := len(tries)
		for k := 1; k <= 6; k++ {
			for _, t := range tries[:near] {
				switch {
				case steep && t.X > int(math.Round(x0+(x1-x0)/2)):
					tries = append(tries, Point{t.X + k, t.Y})
				case steep:
					tries = append(tries, Point{t.X - k, t.Y})
				case t.Y < int(math.Round(y0+(y1-y0)/2)):
					tries = append(tries, Point{t.X, t.Y - k})
				default:
					tries = append(tries, Point{t.X, t.Y + k})
				}
			}
		}
		// On the line itself first, near its middle, the line broken
		// under the words -- a label in its own line cannot be read as
		// another's -- where nothing but the line is there.
		own := map[Point]bool{}
		for p := range taken[i] {
			if !fixed.has(p) {
				own[p] = true
			}
		}
		// Only where the line is alone: over a cell another line passes
		// too, the words would cut that one.
		for j, cells := range taken {
			if j != i {
				for p := range cells {
					delete(own, p)
				}
			}
		}
		placed := false
		for _, f := range []float64{0.5, 0.4, 0.6, 0.3, 0.7} {
			t := ta + f*(tb-ta)
			fx, fy := x0+t*(x1-x0), y0+t*(y1-y0)
			// The cells nearest the words' place centred on the point, so
			// that the nudge from them is never more than half a cell.
			cx, cy := fx+0.5-float64(w)/2, fy-float64(len(lines)-1)/2
			at := Point{int(math.Round(cx)), int(math.Round(cy))}
			if at.X >= 0 && o.freeBut(at, lines, own) {
				o.take(at, lines)
				e.pictureAt, e.pictured, placed = at, true, true
				// The words' middle on the point exactly, in a picture: a
				// cell's middle is half a cell into it, the words' middle
				// half their width.
				e.nudge = [2]float64{cx - float64(at.X), cy - float64(at.Y)}
				lay.Width = max(lay.Width, at.X+w)
				break
			}
		}
		if !placed {
			e.pictureAt, e.pictured = lay.placeNear(o, tries, lines, w, top, i, lineAt)
		}
		fixed.take(e.pictureAt, lines)
	}
	least := 0
	for _, e := range lay.Edges {
		if e.pictured {
			least = min(least, e.pictureAt.Y)
		}
	}
	lay.shiftDown(-least)
}

// placeNear places edge i's label at the best of tries that is free: the
// nearest its own line, as drawn, and not as near another's, where a
// reader could not tell which line it names; the earlier of two as good,
// as tries run from the middle of the line out. Where none is free, at the
// first, as placeOn does.
func (lay *Layout) placeNear(o *occupancy, tries []Point, lines []string, w, top, i int, lineAt [][]Point) (Point, bool) {
	// How far, as drawn -- a cell narrower than a row is tall -- the
	// label at t is from the nearest of cells.
	dist := func(t Point, cells []Point) float64 {
		d := math.Inf(1)
		for _, p := range cells {
			dx := max(t.X-p.X, 0, p.X-(t.X+w-1))
			dy := max(t.Y-p.Y, 0, p.Y-(t.Y+len(lines)-1))
			d = min(d, math.Hypot(float64(dx)*cellAspect, float64(dy)))
		}
		return d
	}
	best, bestScore := -1, math.Inf(1)
	for k, t := range tries {
		if t.X < 0 || !o.free(t, lines) {
			continue
		}
		own := dist(t, lineAt[i])
		score := own + float64(k)*0.001
		for j, cells := range lineAt {
			if j != i && dist(t, cells) <= own+0.25 {
				score += 3 // as near another line: whose is it?
				break
			}
		}
		if score < bestScore {
			best, bestScore = k, score
		}
	}
	if best < 0 {
		return lay.placeOn(o, tries, lines, w, top)
	}
	return lay.placeOn(o, tries[best:best+1], lines, w, top)
}

// inNode says p is in a visible node's box.
func (lay *Layout) inNode(p Point) bool {
	for _, n := range lay.Nodes {
		r := n.Rect
		if !n.Node.Invisible && p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H {
			return true
		}
	}
	return false
}

// cellsOfPath is the cells a path along rows and columns passes through.
func cellsOfPath(pts []Point) map[Point]bool {
	cells := map[Point]bool{}
	for k := 1; k < len(pts); k++ {
		a, b := pts[k-1], pts[k]
		dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
		for p := a; ; p = (Point{p.X + dx, p.Y + dy}) {
			cells[p] = true
			if p == b {
				break
			}
		}
	}
	return cells
}

// placeOn places a label at the first of tries that is free, or the first
// where none is, takes its cells, and grows the layout to hold it.
func (lay *Layout) placeOn(o *occupancy, tries []Point, lines []string, w, top int) (Point, bool) {
	at := Point{}
	if len(tries) > 0 {
		at = tries[0]
	}
	for _, t := range tries {
		if t.X >= 0 && o.free(t, lines) {
			at = t
			break
		}
	}
	at.X, at.Y = max(at.X, 0), max(at.Y, -top)
	o.take(at, lines)
	lay.Width = max(lay.Width, at.X+w)
	lay.Height = max(lay.Height, at.Y+len(lines))
	return at, true
}

// shiftDown moves everything placed k rows down, making room at the top
// for labels only a picture writes there.
func (lay *Layout) shiftDown(k int) {
	if k > 0 {
		lay.shift(k)
		lay.pictureTop += k
	}
}

// shift moves everything placed k rows down, making room at the top.
func (lay *Layout) shift(k int) {
	if k <= 0 {
		return
	}
	lay.Height += k
	for i := range lay.Nodes {
		lay.Nodes[i].Rect.Y += k
		lay.nodeAt[lay.Nodes[i].Node.ID] = lay.Nodes[i]
	}
	for i := range lay.Groups {
		g := &lay.Groups[i]
		g.Rect.Y += k
		g.LabelAt.Y += k
		for j := range g.Dividers {
			g.Dividers[j][0].Y += k
			g.Dividers[j][1].Y += k
		}
	}
	for i := range lay.Edges {
		e := &lay.Edges[i]
		for j := range e.Points {
			e.Points[j].Y += k
		}
		e.LabelAt.Y += k
		e.pictureAt.Y += k
	}
}

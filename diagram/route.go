package diagram

import (
	"math"
	"sort"
)

// route finds each edge's path between the nodes as placed, along rows and
// columns only, by searching the page's cells for the cheapest way from an
// outline cell of its tail to one of its head.
//
// Lines along rows and columns are what a terminal can draw, and they read
// as a diagram rather than as a scribble; a picture draws the same path so
// that the two drawings agree.
//
// A path never enters a box but at its two ends: a line through a box
// reads as a line from it. Nor does it run along another edge's line, or
// turn where another's line is, or end where another ends -- unless the
// two have their arrowheads at one node, or their bare ends, and not both
// (see meet): each of those joins two edges into one line, which reads as
// a connection. For two into one node, or two out of one, that is the
// truth, and a trunk merging into one arrowhead, or a stem forking, is the
// clearest way to draw it; for any
// other two it is a connection that is not there -- a to b and b to a
// drawn as one line with a head at each end, or one edge's line taken over
// by another's half way. Two between the same nodes the same way are kept
// apart too, or one would hide the other. A path may cross another's line,
// at a right angle, as a ┼ reads as two lines crossing and nothing more;
// it may not run along a lane's line or a frame's edge, only cross it. Of
// the paths left it takes the cheapest: short, with few turns, out of the
// middle of a side, not hugging the boxes it passes, and not running close
// beside a line it has nothing to do with.
//
// Edges are routed shortest first, so the straight lines between
// neighbours, which most diagrams are mostly made of, are drawn straight,
// and a long edge finds its way round them. An edge for which there is no
// such path -- the lines already drawn have closed every way between its
// boxes -- is searched for again, allowed to run along another's line or
// end beside another's end at a cost far above any detour: a line joining
// another is a fault, but one through a box is a worse one, so the search
// never takes a path through a box at all. Only where even that finds
// nothing -- a page too big to search -- is the edge drawn along the plain
// path between its boxes, whatever is in the way.
func (l *layouter) route() {
	// Nothing to route, nothing to search: the router's record of the page
	// is a cell's worth of memory a cell, and a page of boxes alone needs
	// none of it.
	visible := false
	for _, e := range l.d.Edges {
		visible = visible || !e.Invisible
	}
	if !visible {
		for i := range l.d.Edges {
			l.out.Edges = append(l.out.Edges, PlacedEdge{Edge: &l.d.Edges[i]})
		}
		return
	}
	order := make([]int, len(l.d.Edges))
	for i := range order {
		order[i] = i
	}
	dist := func(i int) int {
		e := l.d.Edges[i]
		a, b := l.out.nodeAt[e.From].Rect, l.out.nodeAt[e.To].Rect
		return abs((a.X+a.W/2)-(b.X+b.W/2)) + abs((a.Y+a.H/2)-(b.Y+b.H/2))
	}
	// Where the picture draws some edges straight, the others first: the
	// picture draws their paths and not the straight ones', so they may
	// share a line only with one another. The straight ones' paths, which
	// only the text drawing draws, then find their way among them.
	straight := make([]bool, len(l.d.Edges))
	for i, e := range l.d.Edges {
		straight[i] = l.d.Straight && l.out.straightens(e)
	}
	l.out.keepClear(l.d.Edges, straight)
	sort.SliceStable(order, func(i, j int) bool {
		if straight[order[i]] != straight[order[j]] {
			return !straight[order[i]]
		}
		return dist(order[i]) < dist(order[j])
	})
	paths := make([][]Point, len(l.d.Edges))
	dropped := make([]bool, len(l.d.Edges))
	plain, joined := make([]bool, len(l.d.Edges)), make([]bool, len(l.d.Edges))
	parallel := map[[2]string]int{}
	// The edges the picture draws along their paths keep clear of the
	// straight lines it draws, which they may cross but not follow, turn
	// on or end beside; the straight edges' own paths, for the text
	// drawing alone, are found afterwards on a page without those lines.
	fields := edgeFields(l)
	r := newRouter(l, fields)
	r.avoidStraight(l, straight)
	switched := false
	for _, i := range order {
		if straight[i] && !switched {
			switched = true
			r = newRouter(l, fields)
			for k, p := range paths {
				if p != nil {
					r.draw(p, k)
				}
			}
		}
		e := &l.d.Edges[i]
		if e.Invisible {
			// Drawn nowhere, so routed nowhere: a path for it took cells
			// from the edges that are drawn, and turned a straight line
			// between neighbours into a detour round a line nobody sees.
			continue
		}
		from, to := l.out.nodeAt[e.From], l.out.nodeAt[e.To]
		a, b := from.Rect, to.Rect
		key := nodePair(e)
		nth := parallel[key]
		parallel[key]++
		// A loop is searched for as any edge is, from the node back into
		// it, so that it keeps clear of the lines and frames about it and
		// a second loop on the same node goes round the first.
		//
		// Lines kept apart come before the sides an edge asks for: a path
		// by any side that joins no other line is taken over one by the
		// side asked for that does, since a line read as another's
		// misleads, and one by another side only looks other than asked.
		sided := e.FromSide != 0 || e.ToSide != 0 || r.fields[i] != [2]Rect{}
		var pts []Point
		for _, try := range []struct{ strict, anySide bool }{{true, false}, {true, true}, {false, false}, {false, true}} {
			if try.anySide && !sided {
				continue
			}
			r.anySide = try.anySide
			if pts = r.find(from, to, e.Tail, i, try.strict); pts != nil {
				joined[i] = !try.strict
				break
			}
		}
		r.anySide = false
		if pts == nil {
			pts, plain[i] = r.plain(from, to, i, nth), true
		}
		r.draw(pts, i)
		paths[i] = pts
		// Said of the path drawn, not of the search that found it: one
		// free to end anywhere may still have kept a side, and a plain
		// path may happen to.
		dropped[i] = sided && !(keepsSides(pts, e.FromSide, e.ToSide) && keepsFields(pts, a, b, r.fields[i]))
	}
	for i := range l.d.Edges {
		l.out.Edges = append(l.out.Edges, PlacedEdge{Edge: &l.d.Edges[i], Points: paths[i], Straight: straight[i], SidesDropped: dropped[i], plain: plain[i], joined: joined[i]})
	}
}

// The costs a path is weighed by, in tenths of a step.
const (
	stepCost   = 10 // each cell
	turnCost   = 25 // each turn: fewer corners read more easily
	hugCost    = 4  // each cell beside a box: a line pressed to a box is hard to tell from its edge
	crossCost  = 20 // each crossing of another line
	portCost   = 2  // each cell an end is from the middle of its side
	sideCost   = 40 // an end on a side that does not face the other box: more than a turn, so a flow keeps flowing
	besideCost = 6  // each cell beside another's line, running the same way
	shareCost  = -4 // each cell along a line it may share: a trunk, not two lines side by side
	jointCost  = 80 // an end beside a record's joint, as ┴┬, reads as running into the divider: more than the two turns and the steps that move it a cell along
	// joinCost is each cell where a path the rules do not allow -- along
	// another's line, turning on it, at another's end -- is taken all the
	// same, where nothing else is: more than any detour across a page.
	joinCost = 1000
	// missCost is an end on a side its edge does not ask for, where a
	// path keeping to those it asks for at both ends cannot be found:
	// more than a detour, so one end keeps its side where only the other
	// cannot, and less than a join, which outranks a side.
	missCost = 500
)

// router is the page as the paths found so far have left it.
type router struct {
	edges []Edge
	w, h  int
	box   [][]bool    // inside or on the outline of a node
	lines [][]uint8   // the directions lines already drawn leave each cell in
	whose [][][]int   // the edges whose lines those are, -1 for a group's
	bits  [][][]uint8 // and the directions each one's leaves in
	heads [][][]int   // the edges whose head ends here, or whose cell before it is
	tails [][][]int   // the same for the tails
	near  [][]bool    // beside a box
	slant [][]bool    // a straight line a picture draws crosses here at an angle
	// offJoint is a cell straight out from a record's joint, where a line
	// turning, or one joining it, would run on from the divider: ─┐├. It
	// is nil on a page with no joints, as most are: ask offJointAt.
	offJoint [][]bool
	// search is the arrays find keeps its work in, kept for the next;
	// kept is the ends outline keeps, for the next call.
	search search
	kept   []endChoice
	// anySide has find end a path at any side of its boxes, an end off
	// the sides its edge asks for at missCost.
	anySide bool
	// fields are the cells of the record field each edge's From and To
	// ports name, the zero Rect where it names none.
	fields [][2]Rect
}

// oneWay is the node at the end of e with an arrowhead and the node at
// its bare end, where it has an arrowhead at one end only: at its head,
// or at its tail, dir=back, which runs as much one way as the other.
func oneWay(e Edge) (arrow, bare string, ok bool) {
	switch {
	case e.Head && !e.Tail:
		return e.To, e.From, true
	case e.Tail && !e.Head:
		return e.From, e.To, true
	}
	return "", "", false
}

// meet is the node edges a and b may share a line at, and whether there
// is one: each runs one way, and they have their arrowheads at one node
// and their bare ends at two -- a trunk merging into one arrowhead -- or
// their bare ends at one and their arrowheads at two, a stem forking.
// Which of From and To each end is does not matter: b -> a and
// a -> c [dir=back] merge into one arrowhead at a as b -> a and c -> a do.
// An arrowhead at a node where the other's end is bare is a chain
// through it, a into s and out of s to b, and shares nothing.
//
// One way, because a shared stretch says which way it runs only by the
// arrowheads at its far ends. Out of a node into a fork, with an
// arrowhead at the end of each branch and none at the node, it can only
// have come from the node. An edge with arrowheads at both ends runs
// both ways, and an edge leaving its line at a fork could have come from
// either end of it; an edge with none runs neither, and a fork joining
// three lines reads as joining any two of them.
func meet(a, b Edge) (string, bool) {
	aArrow, aBare, aOk := oneWay(a)
	bArrow, bBare, bOk := oneWay(b)
	switch {
	case !aOk || !bOk:
		return "", false
	case aArrow == bArrow && aBare != bBare:
		return aArrow, true
	case aBare == bBare && aArrow != bArrow:
		return aBare, true
	}
	return "", false
}

// fieldAt is the record field edge i ends beside at node, the zero Rect
// where its port there names none.
func (rt *router) fieldAt(i int, node string) Rect {
	if rt.edges[i].To == node {
		return rt.fields[i][1]
	}
	return rt.fields[i][0]
}

// sharing says edge i's line may run along edge j's: they meet, and are
// drawn alike -- a dashed line and a solid one merged into one would be
// drawn as one or the other.
//
// And end beside the same field of the node they meet at, or both
// anywhere on it: a line into one field of a record and a line into the
// whole of it, merged, end beside the field, and the second reads as
// joining it too. The field, not the port's name, as a port no field has
// is anywhere on the box, as no port is.
func (rt *router) sharing(i, j int) bool {
	_, ok := rt.sharingAt(i, j)
	return ok
}

// sharingAt is sharing, and the node the two lines meet at where they may
// share one.
func (rt *router) sharingAt(i, j int) (string, bool) {
	if j < 0 {
		return "", false
	}
	a, b := rt.edges[i], rt.edges[j]
	// A loop shares with nothing: a loop and an edge out of its node
	// merged into one stem read as one line forking to two nodes.
	if a.From == a.To || b.From == b.To {
		return "", false
	}
	at, ok := meet(a, b)
	return at, ok && rt.fieldAt(i, at) == rt.fieldAt(j, at) && a.Line == b.Line && same(a.Colour, b.Colour)
}

// mayEnd says edge i may pass a cell where the heads and the tails of
// others end, at their To and their From: only where the two may share a
// line and meet at that node -- into the same arrowhead, or out of the
// same bare end as a stem forking. Anything else there would cut off an
// arrowhead, or read as leaving or entering with an edge it is not.
func (rt *router) mayEnd(i int, heads, tails []int) bool {
	for _, j := range heads {
		if j < 0 {
			return false // a straight line's end
		}
		if !rt.meetsAt(i, j, rt.edges[j].To) {
			return false
		}
	}
	for _, j := range tails {
		if !rt.meetsAt(i, j, rt.edges[j].From) {
			return false
		}
	}
	return true
}

// meetsAt says edges i and j may share a line, and meet at node.
func (rt *router) meetsAt(i, j int, node string) bool {
	at, ok := rt.sharingAt(i, j)
	return ok && at == node
}

// foreign and shared are the directions the lines in cell (x, y) leave
// in: of the edges edge i may not share a line with, a group's included,
// and of those it may.
func (rt *router) foreign(i, x, y int) uint8 { f, _ := rt.split(i, x, y); return f }

func (rt *router) split(i, x, y int) (foreign, shared uint8) {
	for k, j := range rt.whose[y][x] {
		if rt.sharing(i, j) {
			shared |= rt.bits[y][x][k]
		} else {
			foreign |= rt.bits[y][x][k]
		}
	}
	return foreign, shared
}

// edgeFields is the cells of the record field each edge's From and To
// ports name, the zero Rect where one names none: worked out once a
// layout, as each router -- two, where some edges are drawn straight --
// asks them all.
func edgeFields(l *layouter) [][2]Rect {
	fields := make([][2]Rect, len(l.d.Edges))
	for i, e := range l.d.Edges {
		fields[i][0], _ = l.out.nodeAt[e.From].field(e.FromPort)
		fields[i][1], _ = l.out.nodeAt[e.To].field(e.ToPort)
	}
	return fields
}

// newRouter is a router for the page l has laid out, with the fields of
// its edges' ports as edgeFields has them.
func newRouter(l *layouter, fields [][2]Rect) *router {
	w, h := 0, 0
	for _, n := range l.out.Nodes {
		w, h = max(w, n.Rect.X+n.Rect.W), max(h, n.Rect.Y+n.Rect.H)
	}
	for _, g := range l.out.Groups {
		w, h = max(w, g.Rect.X+g.Rect.W), max(h, g.Rect.Y+g.Rect.H)
	}
	// Room to go round the boxes at the right and the bottom, as the
	// layout leaves room at the left and the top.
	w, h = w+colGap, h+rowGap
	r := &router{edges: l.d.Edges, w: w, h: h, fields: fields}
	grid := func() [][]bool {
		g := make([][]bool, h)
		for y := range g {
			g[y] = make([]bool, w)
		}
		return g
	}
	r.box, r.near, r.slant = grid(), grid(), grid()
	r.lines, r.whose, r.heads, r.tails = make([][]uint8, h), make([][][]int, h), make([][][]int, h), make([][][]int, h)
	r.bits = make([][][]uint8, h)
	for y := range r.lines {
		r.lines[y], r.whose[y], r.heads[y], r.tails[y] = make([]uint8, w), make([][]int, w), make([][]int, w), make([][]int, w)
		r.bits[y] = make([][]uint8, w)
	}
	// An invisible node keeps its place and draws nothing: a line may go
	// where it is.
	for _, n := range l.out.Nodes {
		if n.Node.Invisible {
			continue
		}
		b := n.Rect
		for y := b.Y - 1; y <= b.Y+b.H; y++ {
			for x := b.X - 1; x <= b.X+b.W; x++ {
				if r.in(x, y) {
					inside := x >= b.X && x < b.X+b.W && y >= b.Y && y < b.Y+b.H
					r.box[y][x] = r.box[y][x] || inside
					r.near[y][x] = r.near[y][x] || !inside
				}
			}
		}
		for _, p := range n.outsideJoints() {
			if r.offJoint == nil {
				r.offJoint = grid()
			}
			if r.in(p.X, p.Y) {
				r.offJoint[p.Y][p.X] = true
			}
		}
	}
	// Nor along the title's row, or the blank one beneath it: a line
	// beside the title reads as underlining it, or as coming from it.
	if l.d.Title != "" {
		for y := 0; y < 2 && y < h; y++ {
			for x := range w {
				r.box[y][x] = true
			}
		}
	}
	// A lane's label is a box to a line: a line through it, or past its
	// end, reads as labelled by it -- "a ──east──▶ b" says the edge is
	// called east. A cell either side keeps the words apart from a line.
	for _, g := range l.out.Groups {
		if g.Group.Style != Lane || g.Group.Label == "" {
			continue
		}
		at := laneLabelAt(g)
		for x := at.X - 1; x <= at.X+Width(g.Group.Label); x++ {
			if r.in(x, at.Y) {
				r.box[at.Y][x] = true
			}
		}
	}
	// The groups' lines are lines like any other: to be crossed, not
	// followed.
	for _, g := range l.out.Groups {
		for _, d := range g.Dividers {
			r.line(d[0], d[1], -1)
		}
		if g.Group.Style == Frame {
			x1, y1 := g.Rect.X+g.Rect.W-1, g.Rect.Y+g.Rect.H-1
			r.line(Point{g.Rect.X, g.Rect.Y}, Point{x1, g.Rect.Y}, -1)
			r.line(Point{x1, g.Rect.Y}, Point{x1, y1}, -1)
			r.line(Point{x1, y1}, Point{g.Rect.X, y1}, -1)
			r.line(Point{g.Rect.X, y1}, Point{g.Rect.X, g.Rect.Y}, -1)
		}
	}
	return r
}

func (r *router) in(x, y int) bool { return x >= 0 && y >= 0 && x < r.w && y < r.h }

// line records edge i's line from a to b, along a row or a column; i is
// -1 for a group's.
func (r *router) line(a, b Point, i int) {
	dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
	if dx != 0 && dy != 0 {
		return
	}
	for p := a; ; p = (Point{p.X + dx, p.Y + dy}) {
		var bits uint8
		if p != a {
			bits |= dir(-dx, -dy)
		}
		if p != b {
			bits |= dir(dx, dy)
		}
		if r.in(p.X, p.Y) {
			r.lines[p.Y][p.X] |= bits
			r.whose[p.Y][p.X] = append(r.whose[p.Y][p.X], i)
			r.bits[p.Y][p.X] = append(r.bits[p.Y][p.X], bits)
		}
		if p == b || dx == 0 && dy == 0 {
			return
		}
	}
}

// avoidStraight records the straight lines a picture draws, in the cells
// each crosses between its boxes: one along a row or a column as a line
// no edge may share, one at an angle as cells a path may cross straight
// over but not turn on; and where each meets its boxes' outlines as ends
// no path may end at.
func (r *router) avoidStraight(l *layouter, straight []bool) {
	// Where the picture draws them, set apart from the others between the
	// same two nodes: recorded between the middles, a path ran along a
	// line drawn half a row or more beside the one kept clear, and the two
	// read as one.
	sh := shifts(l.d.Edges, straight)
	for i, e := range l.d.Edges {
		if !straight[i] {
			continue
		}
		a, b := l.out.nodeAt[e.From].Rect, l.out.nodeAt[e.To].Rect
		x0, y0, x1, y1 := l.out.ends(PlacedEdge{Edge: &l.d.Edges[i], Shift: sh[i]})
		// Along a row, on each row beside it; along a column, on each
		// column.
		switch {
		case y0 == y1:
			for _, y := range beside(y0) {
				r.recordStraight(a, b, x0, y, x1, y)
			}
		case x0 == x1:
			for _, x := range beside(x0) {
				r.recordStraight(a, b, x, y0, x, y1)
			}
		default:
			r.recordStraight(a, b, x0, y0, x1, y1)
		}
	}
}

// beside is the rows a line along a row at v is kept clear on, or the
// columns of one along a column: every one less than one from it, the
// one it runs in where it runs in a middle, and both either side where
// it runs between two, as a path along either reads as running along it.
// Rounded to the nearer, paths ran along the other: half way between two
// rows, half a row from the line, and a quarter of a column off a
// column's middle, where a line set apart half a row runs, as a column
// is narrower than a row is tall, three quarters of a column from it --
// nearer than a path in the column beside a line in a column's middle,
// which is no fault.
//
// Within besideHair of a middle is in it: cellAspect is a hair over two
// fifths, so a line set apart a whole number of rows is a hair short of
// a column's middle, never in it -- three lines between boxes of an even
// width, one above the other, are drawn 2.9996 columns either side of the
// middle -- and a column a hair under one from it, as far from it as the
// column beside a line in its middle, was kept clear for nothing.
func beside(v float64) []float64 {
	if m := math.Round(v); math.Abs(v-m) < besideHair {
		return []float64{m}
	}
	return []float64{math.Floor(v), math.Ceil(v)}
}

// besideHair is how near a middle beside counts a line as in it: a
// hundredth of a column, which cellAspect's hair, four ten-thousandths of
// a column a row of shift, reaches only at some twenty-five rows, and a
// quarter of a column, where a line set apart half a row runs, is far
// from.
const besideHair = 0.01

// recordStraight records the straight line from (x0, y0), in a, to (x1,
// y1), in b, as avoidStraight says.
func (r *router) recordStraight(a, b Rect, x0, y0, x1, y1 float64) {
	steps := int(4*max(abs(int(x1-x0)), abs(int(y1-y0)))) + 4
	var last, onA, onB Point
	seenA, seenB := false, false
	for k := 0; k <= steps; k++ {
		t := float64(k) / float64(steps)
		// Floor, not int, which rounds towards zero: a point half a
		// cell or more left of the page fell on its first column.
		p := Point{int(math.Floor(x0 + t*(x1-x0) + 0.5)), int(math.Floor(y0 + t*(y1-y0) + 0.5))}
		if !r.in(p.X, p.Y) || (k > 0 && p == last) {
			continue
		}
		last = p
		inA := p.X >= a.X && p.X < a.X+a.W && p.Y >= a.Y && p.Y < a.Y+a.H
		inB := p.X >= b.X && p.X < b.X+b.W && p.Y >= b.Y && p.Y < b.Y+b.H
		switch {
		case inA:
			onA, seenA = p, true
		case inB:
			if !seenB {
				onB, seenB = p, true
			}
		case r.box[p.Y][p.X]:
		case y0 == y1:
			r.lines[p.Y][p.X] |= left | right
			r.whose[p.Y][p.X] = append(r.whose[p.Y][p.X], -2)
			r.bits[p.Y][p.X] = append(r.bits[p.Y][p.X], left|right)
		case x0 == x1:
			r.lines[p.Y][p.X] |= up | down
			r.whose[p.Y][p.X] = append(r.whose[p.Y][p.X], -2)
			r.bits[p.Y][p.X] = append(r.bits[p.Y][p.X], up|down)
		default:
			r.slant[p.Y][p.X] = true
		}
	}
	// Where it leaves the one box and enters the other: the last cell
	// in a, the first in b, are on their outlines.
	for k, p := range []Point{onA, onB} {
		if []bool{seenA, seenB}[k] {
			r.heads[p.Y][p.X] = append(r.heads[p.Y][p.X], -2)
		}
	}
}

// draw records edge i's path, so the paths after it keep clear of it.
func (r *router) draw(pts []Point, i int) {
	for k := 1; k < len(pts); k++ {
		r.line(pts[k-1], pts[k], i)
	}
	if len(pts) < 2 {
		return
	}
	// Its ends and the cells before them, where an arrowhead is drawn:
	// a line crossing there would cut the head off.
	n := len(pts)
	for _, p := range []Point{pts[0], step(pts[0], pts[1])} {
		if r.in(p.X, p.Y) {
			r.tails[p.Y][p.X] = append(r.tails[p.Y][p.X], i)
		}
	}
	for _, p := range []Point{pts[n-1], step(pts[n-1], pts[n-2])} {
		if r.in(p.X, p.Y) {
			r.heads[p.Y][p.X] = append(r.heads[p.Y][p.X], i)
		}
	}
}

// offJointAt says (x, y), a cell of the page, is straight out from a
// record's joint, where no line turns.
func (r *router) offJointAt(x, y int) bool { return r.offJoint != nil && r.offJoint[y][x] }

// step is the cell one along from a towards b.
func step(a, b Point) Point { return Point{a.X + sign(b.X-a.X), a.Y + sign(b.Y-a.Y)} }

// The four ways a path goes, as the bits of text.go's cells, and each
// one's step.
var ways = [4]struct {
	bit    uint8
	dx, dy int
}{{up, 0, -1}, {down, 0, 1}, {left, -1, 0}, {right, 1, 0}}

// state is a search's place: a cell, the way it was entered, and whether
// the path may turn there yet -- not at the first cell out of the tail
// where the tail has an arrowhead, which a corner there would replace.
type state struct {
	x, y, way int
	settled   bool
}

// The search keeps what it knows of each state in arrays the router holds
// from one search to the next, indexed by id, rather than in maps made for
// each: on a page of a few hundred boxes the maps' hashing and growth, and
// a heap item allocated for every push, were most of the time a layout
// took. gen marks which entries are this search's, so the arrays need not
// be cleared between searches.
type search struct {
	cost []int32  // the cheapest way to the state found so far
	prev []int32  // the state before it on that way, or -1 at a start
	seen []uint32 // the gen of the search that set cost and prev
	gen  uint32
}

// id is s's index in the search's arrays.
func (rt *router) id(s state) int32 {
	k := ((s.y*rt.w+s.x)*4 + s.way) * 2
	if s.settled {
		k++
	}
	return int32(k)
}

// stateOf is the state with id k.
func (rt *router) stateOf(k int32) state {
	settled := k%2 == 1
	k /= 2
	way := int(k % 4)
	k /= 4
	return state{int(k) % rt.w, int(k) / rt.w, way, settled}
}

// maxSearch is the most states one search visits before it gives up. Every
// state of a page of 400 by 400 cells, where once it was a third as many,
// which with the search kept in maps took seconds an edge; on a page that
// large a search that gave up drew the edge through the boxes. A variable
// only so that a test can make every search give up, and see the paths
// drawn then.
var maxSearch = 400 * 400 * 8

// item is a state waiting to be searched from: the cost to it, that and
// the estimate of the rest, and the order it was found in, so that the
// search is the same every time.
type item struct {
	id, cost, at, n int32
}

// queue is a binary heap of items, cheapest first, written out rather than
// through container/heap, whose interface holds each item as an any.
type queue []item

func (q queue) less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	return q[i].n < q[j].n
}

func (q *queue) push(it item) {
	*q = append(*q, it)
	h := *q
	for i := len(h) - 1; i > 0; {
		parent := (i - 1) / 2
		if !h.less(i, parent) {
			break
		}
		h[i], h[parent] = h[parent], h[i]
		i = parent
	}
}

func (q *queue) pop() item {
	h := *q
	top := h[0]
	last := len(h) - 1
	h[0] = h[last]
	h = h[:last]
	for i := 0; ; {
		l, r, least := 2*i+1, 2*i+2, i
		if l < len(h) && h.less(l, least) {
			least = l
		}
		if r < len(h) && h.less(r, least) {
			least = r
		}
		if least == i {
			break
		}
		h[i], h[least] = h[least], h[i]
		i = least
	}
	*q = h
	return top
}

func opposite(way int) int { return way ^ 1 } // up and down, left and right, are pairs

// wayside is the side of a box a way out of it leaves by.
var wayside = [4]Side{North, South, West, East}

// sideOut is the side of its box a path leaves by, from its first point,
// on the box's outline, to its next, out of it. Every path starts so, the
// plain ones too; read from its last point to the one before, it is the
// side the path enters its other box by.
func sideOut(p, q Point) Side { return wayside[wayOut(p, q)] }

// wayOut is the way out of its box a path leaves by, from p to q, as
// sideOut's side.
func wayOut(p, q Point) int {
	switch {
	case q.Y < p.Y:
		return 0
	case q.Y > p.Y:
		return 1
	case q.X < p.X:
		return 2
	}
	return 3
}

// keepsSides says the path pts leaves its first box by one of from and
// enters its last by one of to, each zero for any.
func keepsSides(pts []Point, from, to Side) bool {
	n := len(pts)
	if n < 2 {
		return from == 0 && to == 0
	}
	return (from == 0 || from&sideOut(pts[0], pts[1]) != 0) && (to == 0 || to&sideOut(pts[n-1], pts[n-2]) != 0)
}

// keepsFields says the path pts leaves its first box, a, beside the field
// fields[0] and enters its last, b, beside fields[1], each the zero Rect
// for anywhere.
func keepsFields(pts []Point, a, b Rect, fields [2]Rect) bool {
	n := len(pts)
	if n < 2 {
		return fields == [2]Rect{}
	}
	return (fields[0] == Rect{} || besideField(a, fields[0], pts[0], wayOut(pts[0], pts[1]))) &&
		(fields[1] == Rect{} || besideField(b, fields[1], pts[n-1], wayOut(pts[n-1], pts[n-2])))
}

// outline is the cells of n's outline edge i may end at, as n's ends say
// -- not its corners, unless it is plain text, nor its joints -- each with
// the way out of the box there: those on the page, on the sides and beside
// the field asked for (under anySide, any, those off them at missCost),
// and not where another edge's line runs or ends, but one it may share
// with (where strict is false, there too, at joinCost); then priced for
// the joints over those kept, as priceJoints says why.
func (rt *router) outline(n PlacedNode, i int, strict bool, sides Side, field Rect) (cells []Point, out, costs []int) {
	es, o := n.endsFrom()
	kept := rt.kept[:0]
	for _, e := range es {
		e.at = Point{e.at.X + o.X, e.at.Y + o.Y}
		if !rt.in(e.at.X, e.at.Y) {
			continue
		}
		c := choice(n.Rect, e, sides, field)
		if !c.asked && !rt.anySide {
			continue
		}
		if !rt.mayEnd(i, rt.heads[c.at.Y][c.at.X], rt.tails[c.at.Y][c.at.X]) || rt.foreign(i, c.at.X, c.at.Y) != 0 {
			if strict {
				continue
			}
			c.cost += joinCost
		}
		kept = append(kept, c)
	}
	priceJoints(kept)
	rt.kept = kept
	if len(kept) == 0 {
		return nil, nil, nil
	}
	cells, out, costs = make([]Point, len(kept)), make([]int, len(kept)), make([]int, len(kept))
	for k, c := range kept {
		cells[k], out[k], costs[k] = c.at, c.way, c.cost
	}
	return cells, out, costs
}

// facing says the side of r a path leaves by the way out faces o: o lies
// beyond it.
func facing(r, o Rect, way int) bool {
	switch way {
	case 0:
		return o.Y+o.H <= r.Y
	case 1:
		return o.Y >= r.Y+r.H
	case 2:
		return o.X+o.W <= r.X
	}
	return o.X >= r.X+r.W
}

// offMiddle is how far p is from the middle of its side of r.
func offMiddle(p Point, r Rect, way int) int {
	if way < 2 {
		return abs(p.X - (r.X + r.W/2))
	}
	return abs(p.Y - (r.Y + r.H/2))
}

// find is the cheapest path from an outline cell of na to one of nb, as
// the router's rules allow, its corners only; nil where there is none. Where
// strict is false, a path the rules forbid -- along another's line, turning
// on it, ending beside another's end -- is allowed at joinCost a cell; one
// through a box never is.
func (rt *router) find(na, nb PlacedNode, tail bool, i int, strict bool) []Point {
	a, b := na.Rect, nb.Rect
	starts, outs, startCosts := rt.outline(na, i, strict, rt.edges[i].FromSide, rt.fields[i][0])
	goals, ins, endCosts := rt.outline(nb, i, strict, rt.edges[i].ToSide, rt.fields[i][1])
	if len(starts) == 0 || len(goals) == 0 {
		return nil
	}
	// A goal is entered the way into the box: the opposite of the way out.
	// A box one row tall is entered from above or below at the same cell.
	type entry struct {
		p   Point
		way int
	}
	goalCost := map[entry]int{}
	least := math.MaxInt
	for k, g := range goals {
		e := entry{g, opposite(ins[k])}
		goalCost[e] = endCosts[k]
		if !facing(b, a, ins[k]) {
			goalCost[e] += sideCost
		}
		least = min(least, goalCost[e])
	}
	// What every goal costs is no choice between them, and the search,
	// which does not foresee it, would look that much further round
	// before reaching one: where each cell of a side is beside a joint,
	// as in a record of fields a row tall, that doubled the time.
	if least > 0 {
		for e := range goalCost {
			goalCost[e] -= least
		}
	}
	estimate := func(x, y int) int {
		dx := max(b.X-x, 0, x-(b.X+b.W-1))
		dy := max(b.Y-y, 0, y-(b.Y+b.H-1))
		// At the cheapest a step can be, along a line it may share, so
		// the estimate is never more than the rest costs and the path
		// found is the cheapest there is.
		return (dx + dy) * (stepCost + shareCost)
	}
	sr := &rt.search
	if size := rt.w * rt.h * 8; len(sr.cost) < size {
		sr.cost, sr.prev, sr.seen = make([]int32, size), make([]int32, size), make([]uint32, size)
	}
	sr.gen++
	started := func(id int32) bool { return sr.prev[id] >= 0 }
	q := &queue{}
	var n int32
	visited := 0
	push := func(s state, cost int, prev int32) {
		id := rt.id(s)
		if sr.seen[id] == sr.gen {
			if int(sr.cost[id]) <= cost {
				return
			}
		} else {
			sr.seen[id] = sr.gen
			visited++
		}
		sr.cost[id], sr.prev[id] = int32(cost), prev
		n++
		q.push(item{id: id, cost: int32(cost), at: int32(cost + estimate(s.x, s.y)), n: n})
	}
	for k, p := range starts {
		cost := offMiddle(p, a, outs[k])*portCost + startCosts[k]
		if !facing(a, b, outs[k]) {
			cost += sideCost
		}
		push(state{p.X, p.Y, outs[k], false}, cost, -1)
	}
	for len(*q) > 0 {
		it := q.pop()
		if it.cost != sr.cost[it.id] {
			continue
		}
		s := rt.stateOf(it.id)
		if _, ok := goalCost[entry{Point{s.x, s.y}, s.way}]; ok && inside(b, s.x, s.y) && started(it.id) {
			return rt.corners(it.id)
		}
		atStart := inside(a, s.x, s.y) && !started(it.id) // only a start is in a box and not yet the goal
		for w, way := range ways {
			if w == opposite(s.way) {
				continue
			}
			turning := w != s.way
			cost := int(it.cost) + stepCost
			if turning {
				// Out of the box straight; not at the first cell where the
				// tail has a head; not straight out from a record's joint,
				// where a stretch would run on from the divider, in any try,
				// as a line ends at no joint; and never on another's line.
				if atStart || !s.settled || rt.offJointAt(s.x, s.y) {
					continue
				}
				if rt.foreign(i, s.x, s.y) != 0 || rt.slant[s.y][s.x] {
					if strict {
						continue
					}
					cost += joinCost
				}
				cost += turnCost
			}
			nx, ny := s.x+way.dx, s.y+way.dy
			if !rt.in(nx, ny) {
				continue
			}
			next := Point{nx, ny}
			// An invisible node is no box to the router, but its own edges
			// end at it all the same.
			if rt.box[ny][nx] || inside(b, nx, ny) {
				// Into a box only at a goal, the way into it, not turning.
				gc, ok := goalCost[entry{next, w}]
				if !ok || turning {
					continue
				}
				push(state{nx, ny, w, true}, cost+gc+offMiddle(next, b, opposite(w))*portCost, it.id)
				continue
			}
			if !rt.mayEnd(i, rt.heads[ny][nx], rt.tails[ny][nx]) {
				if strict {
					continue
				}
				cost += joinCost
			}
			along := uint8(left | right)
			if way.dx == 0 {
				along = up | down
			}
			if foreign, shared := rt.split(i, nx, ny); foreign|shared != 0 {
				// Across the lines it may not share only, at a right
				// angle; along those it may.
				if foreign != 0 {
					switch {
					case foreign == (up|down|left|right)&^along:
						cost += crossCost
					case strict:
						continue
					default:
						cost += joinCost
					}
				}
				if shared&along != 0 {
					cost += shareCost
				}
			}
			if rt.slant[ny][nx] {
				cost += crossCost
			}
			if rt.near[ny][nx] {
				cost += hugCost
			}
			// Beside a line running the same way, that it may not share:
			// two lines a cell apart are hard to follow.
			for _, side := range [2][2]int{{way.dy, way.dx}, {-way.dy, -way.dx}} {
				sx, sy := nx+side[0], ny+side[1]
				if rt.in(sx, sy) && rt.foreign(i, sx, sy)&along != 0 {
					cost += besideCost
				}
			}
			// The first cell out of a tail with a head may not be a
			// corner; every later one may.
			settled := !(atStart && tail)
			push(state{nx, ny, w, settled}, cost, it.id)
		}
		if visited > maxSearch {
			return nil // a page too big to search: the plain path will do
		}
	}
	return nil
}

// inside says (x, y) is one of r's cells.
func inside(r Rect, x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

// corners walks back from the goal, keeping the cells the path turns at.
func (rt *router) corners(id int32) []Point {
	var cells []Point
	ways := []int{}
	for id >= 0 {
		s := rt.stateOf(id)
		cells = append(cells, Point{s.x, s.y})
		ways = append(ways, s.way)
		id = rt.search.prev[id]
	}
	// Reverse, start first.
	for i, j := 0, len(cells)-1; i < j; i, j = i+1, j-1 {
		cells[i], cells[j] = cells[j], cells[i]
		ways[i], ways[j] = ways[j], ways[i]
	}
	pts := []Point{cells[0]}
	for i := 1; i < len(cells)-1; i++ {
		if ways[i+1] != ways[i] {
			pts = append(pts, cells[i])
		}
	}
	return append(pts, cells[len(cells)-1])
}

// span is the cells a and b share along one axis, from lo to hi, leaving
// out each one's corner cells, which an edge must not leave from.
func span(a0, a1, b0, b1 int) (lo, hi int, ok bool) {
	lo, hi = max(a0, b0)+1, min(a1, b1)-1
	return lo, hi, lo <= hi
}

// path is the path from a to b, the nth edge between the two: a later one
// beside the first rather than over it, where they share a straight line.
func path(a, b Rect, nth int) []Point {
	ax1, ay1, bx1, by1 := a.X+a.W-1, a.Y+a.H-1, b.X+b.W-1, b.Y+b.H-1
	acx, acy, bcx, bcy := a.X+a.W/2, a.Y+a.H/2, b.X+b.W/2, b.Y+b.H/2
	// The nth of several between the same two: 0, +2, -2, +4 ... cells off
	// the middle, held within the cells they share.
	shift := func(mid, lo, hi int) int {
		off := (nth + 1) / 2 * 2
		if nth%2 == 0 {
			off = -off
		}
		return min(max(mid+off, lo), hi)
	}
	// One straight above, below or beside the other: a straight line down
	// the middle of what they share, near the middle of both.
	if lo, hi, ok := span(a.X, ax1, b.X, bx1); ok && (b.Y > ay1 || a.Y > by1) {
		x := shift(min(max((acx+bcx)/2, lo), hi), lo, hi)
		if b.Y > ay1 {
			return []Point{{x, ay1}, {x, b.Y}}
		}
		return []Point{{x, a.Y}, {x, by1}}
	}
	if lo, hi, ok := span(a.Y, ay1, b.Y, by1); ok && (b.X > ax1 || a.X > bx1) {
		y := shift(min(max((acy+bcy)/2, lo), hi), lo, hi)
		if b.X > ax1 {
			return []Point{{ax1, y}, {b.X, y}}
		}
		return []Point{{a.X, y}, {bx1, y}}
	}
	// Neither: out across the wider gap between them, along, and in.
	gapY := max(b.Y-ay1, a.Y-by1)
	gapX := max(b.X-ax1, a.X-bx1)
	if gapY >= gapX {
		from, to := Point{acx, ay1}, Point{bcx, b.Y}
		if a.Y > by1 {
			from, to = Point{acx, a.Y}, Point{bcx, by1}
		}
		mid := (from.Y + to.Y) / 2
		return []Point{from, {from.X, mid}, {to.X, mid}, to}
	}
	from, to := Point{ax1, acy}, Point{b.X, bcy}
	if a.X > bx1 {
		from, to = Point{a.X, acy}, Point{bx1, bcy}
	}
	mid := (from.X + to.X) / 2
	return []Point{from, {mid, from.Y}, {mid, to.Y}, to}
}

// plain is edge i's plain path, from na to nb, drawn where every search
// has given up: path's, or loop's from a node back into itself, with each
// end, and the point after it, moved along its side as far as three cells
// to where it costs least. Where the path is that one stretch, its two
// ends move together.
//
// Each end is priced as the search prices it, by outline asked as the
// search's last try asks -- any side, and others' lines and ends allowed
// at joinCost -- so that off the corners and the joints, beside the field
// and on the sides asked for, and clear of a joint where it can be, are
// one rule with the search's; and the cells of the stretches either side
// of each point it moves as a step of the search is, each cell once:
// running along another's line, meeting one where it turns or ends,
// turning on a straight line a picture draws, turning straight out from
// a joint, where the search never turns, or passing the cell before
// another's arrowhead, at joinCost a cell, crossing a line at a right
// angle free. A cell moved costs a little, so that not moving wins a tie;
// not moving is priced as any move is, so a plain path on another's line,
// a cell from a free one, takes the free one. A move that takes the path
// through more of the boxes than it ran through is not made, a line
// through a box being the worse fault; where every move would, or no end
// is offered, the path is drawn as it is -- the one line that may still
// end at a joint.
//
// It chooses, where offJoints once moved an end only off a joint, a cell
// or a few along, after the fact: that took a cell beside the joint as
// readily as one clear of it, kept no field, left a loop round a box three
// or four cells wide at its corner, and drew a plain path along another's
// line, two edges as one, where a cell over was free.
func (rt *router) plain(na, nb PlacedNode, i, nth int) []Point {
	e := rt.edges[i]
	var pts []Point
	if e.From == e.To {
		pts = loop(na.Rect)
	} else {
		pts = path(na.Rect, nb.Rect, nth)
	}
	n := len(pts)
	if n < 2 {
		return pts
	}
	// The ends each node offers this edge, and at what cost.
	type at struct {
		p   Point
		way int
	}
	offered := func(nd PlacedNode, sides Side, field Rect) map[at]int {
		was := rt.anySide
		rt.anySide = true
		cells, out, costs := rt.outline(nd, i, false, sides, field)
		rt.anySide = was
		m := make(map[at]int, len(cells))
		for k, c := range cells {
			m[at{c, out[k]}] = costs[k]
		}
		return m
	}
	from := offered(na, e.FromSide, rt.fields[i][0])
	to := offered(nb, e.ToSide, rt.fields[i][1])
	// walk is what the stretches of q either side of the points ms cost
	// as steps of the search, and how many of their cells are in a box but
	// the path's own two ends. Each cell is priced once, a corner the two
	// stretches meet at among them, as the search prices each cell it
	// steps to once.
	walk := func(q []Point, ms ...int) (cost, boxed int) {
		turns := map[Point]bool{}
		for _, t := range q[1 : n-1] {
			turns[t] = true
		}
		done := map[Point]bool{}
		for _, m := range ms {
			for _, lo := range []int{m - 1, m} {
				if lo < 0 || lo+1 >= n {
					continue
				}
				p, end := q[lo], q[lo+1]
				w := wayOut(p, end)
				along := uint8(left | right)
				if ways[w].dx == 0 {
					along = up | down
				}
				for ; ; p = step(p, end) {
					if !done[p] && p != q[0] && p != q[n-1] && rt.in(p.X, p.Y) {
						done[p] = true
						if rt.box[p.Y][p.X] {
							boxed++
						}
						if !rt.mayEnd(i, rt.heads[p.Y][p.X], rt.tails[p.Y][p.X]) {
							cost += joinCost
						}
						// Turning on another's line, or on a straight one a
						// picture draws, joins it, and turning straight out
						// from a joint runs on from the divider; passing over
						// a line at a right angle does neither.
						f := rt.foreign(i, p.X, p.Y)
						switch {
						case turns[p] && (f != 0 || rt.slant[p.Y][p.X] || rt.offJointAt(p.X, p.Y)):
							cost += joinCost
						case f != 0 && f != (up|down|left|right)&^along:
							cost += joinCost
						}
					}
					if p == end {
						break
					}
				}
			}
		}
		return cost, boxed
	}
	type move struct{ k, next int }
	moves := []move{{0, 1}, {n - 1, n - 2}}
	if n == 2 {
		moves = moves[:1] // one stretch: both ends move together
	}
	for _, mv := range moves {
		var best []Point
		least, inBoxes := math.MaxInt, 0
		for _, d := range []int{0, 1, -1, 2, -2, 3, -3} {
			q := append([]Point(nil), pts...)
			for _, m := range []int{mv.k, mv.next} {
				if pts[mv.k].X == pts[mv.next].X {
					q[m].X += d
				} else {
					q[m].Y += d
				}
			}
			c, ok := 0, true
			if mv.k == 0 || n == 2 {
				cf, okf := from[at{q[0], wayOut(q[0], q[1])}]
				c, ok = c+cf, ok && okf
			}
			if mv.k == n-1 || n == 2 {
				ct, okt := to[at{q[n-1], wayOut(q[n-1], q[n-2])}]
				c, ok = c+ct, ok && okt
			}
			// Not moving comes first, and is walked whether offered or not:
			// a move may take the path through no more of the boxes than
			// it already ran through. The plain path's shape is drawn
			// through whatever is in the way, and an end kept at a joint
			// because the stretch beyond it crossed a box, moved or not,
			// was kept there for nothing.
			cw, boxed := walk(q, mv.k, mv.next)
			if d == 0 {
				inBoxes = boxed
			} else if boxed > inBoxes {
				continue
			}
			if !ok {
				continue
			}
			if c += cw + abs(d)*portCost; c < least {
				best, least = q, c
			}
		}
		if best != nil {
			pts = best
		}
	}
	return pts
}

// loop is the path of an edge from a node to itself: out of its right side,
// round, and into its bottom.
func loop(a Rect) []Point {
	ax1, ay1 := a.X+a.W-1, a.Y+a.H-1
	cy := a.Y + a.H/2
	x := a.X + a.W*3/4
	return []Point{{ax1, cy}, {ax1 + 3, cy}, {ax1 + 3, ay1 + 2}, {x, ay1 + 2}, {x, ay1}}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

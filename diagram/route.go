package diagram

import (
	"container/heap"
	"image/color"
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
// two share their head or their tail, and not both: each of those joins
// two edges into one line, which reads as a connection. For two into one
// node, or two out of one, that is the truth, and a trunk merging into one
// arrowhead, or a stem forking, is the clearest way to draw it; for any
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
	order := make([]int, len(l.d.Edges))
	for i := range order {
		order[i] = i
	}
	dist := func(i int) int {
		e := l.d.Edges[i]
		a, b := l.rect[e.From], l.rect[e.To]
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
	sort.SliceStable(order, func(i, j int) bool {
		if straight[order[i]] != straight[order[j]] {
			return !straight[order[i]]
		}
		return dist(order[i]) < dist(order[j])
	})
	paths := make([][]Point, len(l.d.Edges))
	parallel := map[[2]string]int{}
	// The edges the picture draws along their paths keep clear of the
	// straight lines it draws, which they may cross but not follow, turn
	// on or end beside; the straight edges' own paths, for the text
	// drawing alone, are found afterwards on a page without those lines.
	r := newRouter(l)
	r.avoidStraight(l, straight)
	switched := false
	for _, i := range order {
		if straight[i] && !switched {
			switched = true
			r = newRouter(l)
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
		a, b := l.rect[e.From], l.rect[e.To]
		key := [2]string{e.From, e.To}
		if e.To < e.From {
			key = [2]string{e.To, e.From}
		}
		nth := parallel[key]
		parallel[key]++
		// A loop is searched for as any edge is, from the node back into
		// it, so that it keeps clear of the lines and frames about it and
		// a second loop on the same node goes round the first.
		pts := r.find(a, b, e.Tail, i, true)
		if pts == nil {
			pts = r.find(a, b, e.Tail, i, false)
		}
		if pts == nil {
			if e.From == e.To {
				pts = loop(a)
			} else {
				pts = path(a, b, nth)
			}
		}
		r.draw(pts, i)
		paths[i] = pts
	}
	for i := range l.d.Edges {
		l.out.Edges = append(l.out.Edges, PlacedEdge{Edge: &l.d.Edges[i], Points: paths[i], Straight: straight[i]})
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
	// joinCost is each cell where a path the rules do not allow -- along
	// another's line, turning on it, at another's end -- is taken all the
	// same, where nothing else is: more than any detour across a page.
	joinCost = 1000
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
	// bare are the nodes drawn without an outline -- plain text -- whose
	// every cell a line may end at, its corners too: there is no corner
	// for a line to be mistaken for. A one-letter label is a box a cell
	// wide and a row tall, which has no outline cell but its corners, and
	// every edge into it fell back to the plain path through the boxes.
	bare map[Rect]bool
}

// sharing says edge i's line may run along edge j's: the two share their
// head or their tail, but not both, and are drawn alike -- a dashed line
// and a solid one merged into one would be drawn as one or the other.
func (rt *router) sharing(i, j int) bool {
	if j < 0 {
		return false
	}
	a, b := rt.edges[i], rt.edges[j]
	// A loop shares with nothing: a loop and an edge out of its node
	// merged into one stem read as one line forking to two nodes.
	if a.From == a.To || b.From == b.To {
		return false
	}
	return (a.From == b.From) != (a.To == b.To) && a.Line == b.Line && sameColour(a.Colour, b.Colour)
}

func sameColour(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

// mayEnd says edge i may pass a cell where the heads and the tails of
// others end: by a head, only if it goes into the same node, from another,
// to merge into its arrowhead; by a tail, only if it comes out of the same
// node to another, as a stem forking. Anything else there would cut off an
// arrowhead, or read as leaving or entering with an edge it is not.
func (rt *router) mayEnd(i int, heads, tails []int) bool {
	a := rt.edges[i]
	for _, j := range heads {
		if j < 0 {
			return false // a straight line's end
		}
		if b := rt.edges[j]; a.To != b.To || a.From == b.From {
			return false
		}
	}
	for _, j := range tails {
		if b := rt.edges[j]; a.From != b.From || a.To == b.To {
			return false
		}
	}
	return true
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

// mayShare says edge i may run along, turn on or end at everything in the
// cell owned by those.
func (rt *router) mayShare(i int, owners []int) bool {
	for _, j := range owners {
		if !rt.sharing(i, j) {
			return false
		}
	}
	return true
}

func newRouter(l *layouter) *router {
	w, h := 0, 0
	for _, r := range l.rect {
		w, h = max(w, r.X+r.W), max(h, r.Y+r.H)
	}
	for _, g := range l.out.Groups {
		w, h = max(w, g.Rect.X+g.Rect.W), max(h, g.Rect.Y+g.Rect.H)
	}
	// Room to go round the boxes at the right and the bottom, as the
	// layout leaves room at the left and the top.
	w, h = w+colGap, h+rowGap
	r := &router{edges: l.d.Edges, w: w, h: h, bare: map[Rect]bool{}}
	for _, n := range l.out.Nodes {
		if n.Node.Shape == Plain {
			r.bare[n.Rect] = true
		}
	}
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
	for i, e := range l.d.Edges {
		if !straight[i] {
			continue
		}
		a, b := l.rect[e.From], l.rect[e.To]
		x0, y0 := centre(a)
		x1, y1 := centre(b)
		steps := int(4*max(abs(int(x1-x0)), abs(int(y1-y0)))) + 4
		var last, onA, onB Point
		seenA, seenB := false, false
		for k := 0; k <= steps; k++ {
			t := float64(k) / float64(steps)
			p := Point{int(x0 + t*(x1-x0) + 0.5), int(y0 + t*(y1-y0) + 0.5)}
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

type item struct {
	s        state
	cost, at int // at: cost and the estimate of the rest
	idx      int
}

type queue []*item

func (q queue) Len() int { return len(q) }
func (q queue) Less(i, j int) bool {
	if q[i].at != q[j].at {
		return q[i].at < q[j].at
	}
	return q[i].idx < q[j].idx // in the order found, so the search is the same every time
}
func (q queue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *queue) Push(x any)   { *q = append(*q, x.(*item)) }
func (q *queue) Pop() any     { old := *q; it := old[len(old)-1]; *q = old[:len(old)-1]; return it }
func opposite(way int) int    { return way ^ 1 } // up and down, left and right, are pairs

// outline is r's outline cells edge i may end at -- not its corners, nor
// a cell another edge ends at but one it may share with -- each with the
// way out of the box there. Where strict is false, any but a corner, each
// with what ending there costs.
func (rt *router) outline(r Rect, i int, strict bool) (cells []Point, out, costs []int) {
	add := func(p Point, way int) {
		if !rt.in(p.X, p.Y) {
			return
		}
		cost := 0
		if !rt.mayEnd(i, rt.heads[p.Y][p.X], rt.tails[p.Y][p.X]) || rt.foreign(i, p.X, p.Y) != 0 {
			if strict {
				return
			}
			cost = joinCost
		}
		for k, c := range cells {
			if c == p {
				// A box one row tall has its top and its bottom in the
				// same cells: a line may end there from above or below.
				// The second way is kept beside the first, not over it.
				if out[k] != way {
					cells, out, costs = append(cells, p), append(out, way), append(costs, cost)
				}
				return
			}
		}
		cells, out, costs = append(cells, p), append(out, way), append(costs, cost)
	}
	corner := 1
	if rt.bare[r] {
		corner = 0
	}
	for x := r.X + corner; x < r.X+r.W-corner; x++ {
		add(Point{x, r.Y}, 0)
		add(Point{x, r.Y + r.H - 1}, 1)
	}
	for y := r.Y + corner; y < r.Y+r.H-corner; y++ {
		add(Point{r.X, y}, 2)
		add(Point{r.X + r.W - 1, y}, 3)
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

// find is the cheapest path from an outline cell of a to one of b, as the
// router's rules allow, its corners only; nil where there is none. Where
// strict is false, a path the rules forbid -- along another's line, turning
// on it, ending beside another's end -- is allowed at joinCost a cell; one
// through a box never is.
func (rt *router) find(a, b Rect, tail bool, i int, strict bool) []Point {
	starts, outs, startCosts := rt.outline(a, i, strict)
	goals, ins, endCosts := rt.outline(b, i, strict)
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
	for k, g := range goals {
		e := entry{g, opposite(ins[k])}
		goalCost[e] = endCosts[k]
		if !facing(b, a, ins[k]) {
			goalCost[e] += sideCost
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
	best := map[state]int{}
	from := map[state]state{}
	started := func(s state) bool { _, ok := from[s]; return ok }
	q := &queue{}
	n := 0
	push := func(s state, cost int, prev *state) {
		if c, ok := best[s]; ok && c <= cost {
			return
		}
		best[s] = cost
		if prev != nil {
			from[s] = *prev
		}
		n++
		heap.Push(q, &item{s: s, cost: cost, at: cost + estimate(s.x, s.y), idx: n})
	}
	for k, p := range starts {
		cost := offMiddle(p, a, outs[k])*portCost + startCosts[k]
		if !facing(a, b, outs[k]) {
			cost += sideCost
		}
		push(state{p.X, p.Y, outs[k], false}, cost, nil)
	}
	for q.Len() > 0 {
		it := heap.Pop(q).(*item)
		s := it.s
		if it.cost != best[s] {
			continue
		}
		if _, ok := goalCost[entry{Point{s.x, s.y}, s.way}]; ok && inside(b, s.x, s.y) {
			if _, started := from[s]; started {
				return rt.corners(s, from)
			}
		}
		atStart := inside(a, s.x, s.y) && !started(s) // only a start is in a box and not yet the goal
		for w, way := range ways {
			if w == opposite(s.way) {
				continue
			}
			turning := w != s.way
			cost := it.cost + stepCost
			if turning {
				// Out of the box straight; not at the first cell where the
				// tail has a head; and never on another's line.
				if atStart || !s.settled {
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
				push(state{nx, ny, w, true}, cost+gc+offMiddle(next, b, opposite(w))*portCost, &s)
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
			push(state{nx, ny, w, settled}, cost, &s)
		}
		if len(best) > 400000 {
			return nil // a page too big to search: the plain path will do
		}
	}
	return nil
}

// inside says (x, y) is one of r's cells.
func inside(r Rect, x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

// corners walks back from the goal, keeping the cells the path turns at.
func (rt *router) corners(s state, from map[state]state) []Point {
	var cells []Point
	ways := []int{}
	for {
		cells = append(cells, Point{s.x, s.y})
		ways = append(ways, s.way)
		prev, ok := from[s]
		if !ok {
			break
		}
		s = prev
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

// loop is the path of an edge from a node to itself: out of its right side,
// round, and into its bottom.
func loop(a Rect) []Point {
	ax1, ay1 := a.X+a.W-1, a.Y+a.H-1
	cy := a.Y + a.H/2
	x := a.X + a.W*3/4
	return []Point{{ax1, cy}, {ax1 + 3, cy}, {ax1 + 3, ay1 + 2}, {x, ay1 + 2}, {x, ay1}}
}

func firstLine(s string) string {
	for i, r := range s {
		if r == '\n' {
			return s[:i]
		}
	}
	return s
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

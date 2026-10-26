package diagram

// A line's end on a node: the one answer to where it may be, which every
// way a line is drawn asks -- the router's search, the plain path drawn
// where every search gives up, and a braille stub -- rather than each
// keeping a rule of its own about joints and corners, as they once did,
// four rules that had to be changed together.

// end is an outline cell of a node a line may end at, the way out of the
// box there -- an index into ways -- and whether it lies a cell from a
// joint. A cell of a box one row tall, or one column wide, is two ends,
// one each way.
type end struct {
	at          Point
	way         int
	besideJoint bool
}

// settle works out n's ends and joints, once, as it is placed: the router
// asks them of every node for every edge, up to four searches each, and
// the text drawing again for every straight line. They are kept relative
// to n's Rect, so that moving the node -- the layout trims its margins,
// and makes room for a picture's labels, after the lines are routed --
// moves them with it.
func (n *PlacedNode) settle() {
	o := Point{n.Rect.X, n.Rect.Y}
	n.jointAt, n.offAt = nil, nil
	for _, j := range n.workJoints() {
		off := outward(n.Rect, j)
		n.jointAt = append(n.jointAt, Point{j.X - o.X, j.Y - o.Y})
		n.offAt = append(n.offAt, Point{off.X - o.X, off.Y - o.Y})
	}
	n.endAt = nil
	for _, e := range n.workEnds() {
		e.at = Point{e.at.X - o.X, e.at.Y - o.Y}
		n.endAt = append(n.endAt, e)
	}
	n.settled = true
}

// ends is where on n's outline a line may end, as settle worked it out:
// every outline cell but the corners -- a plain-text node's corners
// excepted, as it has no corner for a line to be mistaken for -- and the
// joints, in the order the search tries them, which decides between two
// paths that cost the same. A node nobody settled, as a test may build
// one, has them worked out on the spot, so that it is never taken for one
// without joints.
func (n PlacedNode) ends() []end {
	kept, o := n.endsFrom()
	if len(kept) == 0 {
		return nil
	}
	es := make([]end, len(kept))
	for k, e := range kept {
		e.at = Point{e.at.X + o.X, e.at.Y + o.Y}
		es[k] = e
	}
	return es
}

// endsFrom is n's ends as kept, each relative to the cell it returns --
// its Rect's corner where settled, the page's own where they had to be
// worked out on the spot -- without the copy ends makes: the router asks
// for them twice a search, and copied, every search took a node's whole
// outline twice over in memory.
func (n PlacedNode) endsFrom() ([]end, Point) {
	if !n.settled {
		return n.workEnds(), Point{}
	}
	return n.endAt, Point{n.Rect.X, n.Rect.Y}
}

// joints is where n's dividers meet its outline: the cells no line ends
// at, and ends beside only where its field has no other cell, as it reads
// as running into the divider. A divider's end inside the box, on another
// divider, is none, and an invisible record, whose dividers are not
// drawn, has none.
func (n PlacedNode) joints() []Point {
	if !n.settled {
		return n.workJoints()
	}
	if len(n.jointAt) == 0 {
		return nil
	}
	js := make([]Point, len(n.jointAt))
	for k, j := range n.jointAt {
		js[k] = Point{j.X + n.Rect.X, j.Y + n.Rect.Y}
	}
	return js
}

// offJoints is the cell straight out from each of n's joints, as kept,
// relative to the cell it returns, as endsFrom's are: no line turns
// there, as one stretch of the turn would run on from the divider, ─┐├.
func (n PlacedNode) offJoints() ([]Point, Point) {
	if !n.settled {
		var offs []Point
		for _, j := range n.workJoints() {
			offs = append(offs, outward(n.Rect, j))
		}
		return offs, Point{}
	}
	return n.offAt, Point{n.Rect.X, n.Rect.Y}
}

// outward is the cell straight out from r at j, a cell of its outline off
// the corners.
func outward(r Rect, j Point) Point {
	switch {
	case j.Y == r.Y:
		j.Y--
	case j.Y == r.Y+r.H-1:
		j.Y++
	case j.X == r.X:
		j.X--
	default:
		j.X++
	}
	return j
}

func (n PlacedNode) workJoints() []Point {
	if n.Node != nil && n.Node.Invisible {
		return nil
	}
	r := n.Rect
	var js []Point
	for _, d := range n.Dividers {
		for _, p := range d {
			if p.X == r.X || p.X == r.X+r.W-1 || p.Y == r.Y || p.Y == r.Y+r.H-1 {
				js = append(js, p)
			}
		}
	}
	return js
}

func (n PlacedNode) workEnds() []end {
	var joint map[Point]bool // nil, and every lookup false, for most nodes
	if js := n.workJoints(); len(js) > 0 {
		joint = make(map[Point]bool, len(js))
		for _, j := range js {
			joint[j] = true
		}
	}
	r := n.Rect
	var es []end
	add := func(p Point, way int) {
		if joint[p] {
			return
		}
		es = append(es, end{p, way, joint[Point{p.X - 1, p.Y}] || joint[Point{p.X + 1, p.Y}] || joint[Point{p.X, p.Y - 1}] || joint[Point{p.X, p.Y + 1}]})
	}
	corner := 1
	if n.Node != nil && n.Node.Outline() == Plain {
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
	return es
}

// endChoice is one of a node's ends as one end of one edge sees it:
// whether it is on the sides and beside the field the edge asks for there,
// and what ending at it costs -- missCost where it is not, before what a
// caller adds of its own.
type endChoice struct {
	end
	asked bool
	cost  int
}

// choice is the end e of the node whose box is r, in the page's cells, as
// an end asking for sides and field, zero for anywhere, sees it. Most
// edges ask for neither, and every end is as asked for, at no cost.
func choice(r Rect, e end, sides Side, field Rect) endChoice {
	if sides == 0 && field == (Rect{}) {
		return endChoice{end: e, asked: true}
	}
	asked := (sides == 0 || sides&wayside[e.way] != 0) && (field == Rect{} || besideField(r, field, e.at, e.way))
	c := endChoice{end: e, asked: asked}
	if !asked {
		c.cost = missCost
	}
	return c
}

// priceJoints charges jointCost to each of cs beside a joint where its
// side, among those of cs as asked for as it is, has one beside none. An
// end beside a joint is allowed, as a field two cells wide has nowhere
// else, but a line takes a cell further along where there is one; and the
// cost picks a cell along a side, never turning a line to another side --
// under rankdir=LR every cell of a field a row tall is beside one, and a
// line between two records went round their tops for it.
//
// It is charged over the ends a caller has kept, never over all of a
// node's: a side whose clear cells another line has taken is a side of
// cells beside joints, and charging them all would turn the line away.
func priceJoints(cs []endChoice) {
	group := func(c endChoice) int { // a side, as asked for or not
		if c.asked {
			return c.way + 4
		}
		return c.way
	}
	var clear [8]bool
	for _, c := range cs {
		if !c.besideJoint {
			clear[group(c)] = true
		}
	}
	for k, c := range cs {
		if c.besideJoint && clear[group(c)] {
			cs[k].cost += jointCost
		}
	}
}

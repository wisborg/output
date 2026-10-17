package diagram

// route draws each edge's path between the nodes as placed, along rows and
// columns only: a straight line where one node is above, below or beside
// the other, two corners where it is not -- out of the tail across the gap
// between them, along to the head, and into it.
//
// Lines along rows and columns are what a terminal can draw, and they read
// as a diagram rather than as a scribble; a picture draws the same path so
// that the two drawings agree.
//
// The path does not go round the boxes between its ends. In a grid, where
// boxes are placed by what the diagram declares, a line between two that are
// not neighbours may pass one: the diagram says where each box goes, and
// the line follows.
func (l *layouter) route() {
	parallel := map[[2]string]int{}
	for i := range l.d.Edges {
		e := &l.d.Edges[i]
		a, b := l.rect[e.From], l.rect[e.To]
		key := [2]string{e.From, e.To}
		if e.To < e.From {
			key = [2]string{e.To, e.From}
		}
		nth := parallel[key]
		parallel[key]++
		var pts []Point
		switch {
		case e.From == e.To:
			pts = loop(a)
		default:
			pts = path(a, b, nth)
			if len(pts) == 2 && l.blocked(pts, e.From, e.To) {
				pts = detour(a, b, l.below(a, b), l.beside(a, b))
			}
		}
		l.out.Edges = append(l.out.Edges, PlacedEdge{Edge: e, Points: pts})
	}
}

// blocked says the path passes through a node other than its ends: a
// straight line along a row of a grid, from the first box of the row to the
// third, through the second. Drawn so, it reads as a line from the second.
func (l *layouter) blocked(pts []Point, from, to string) bool {
	for id, r := range l.rect {
		if id == from || id == to {
			continue
		}
		for i := 1; i < len(pts); i++ {
			a, b := pts[i-1], pts[i]
			x0, x1, y0, y1 := min(a.X, b.X), max(a.X, b.X), min(a.Y, b.Y), max(a.Y, b.Y)
			if x0 <= r.X+r.W-1 && x1 >= r.X && y0 <= r.Y+r.H-1 && y1 >= r.Y {
				return true
			}
		}
	}
	return false
}

// below is the row a detour between a and b runs along: the middle of the
// gap beneath the lowest box the row between them holds.
func (l *layouter) below(a, b Rect) int {
	lo, hi := min(a.X, b.X), max(a.X+a.W, b.X+b.W)
	bottom := max(a.Y+a.H, b.Y+b.H)
	for _, r := range l.rect {
		if r.X < hi && r.X+r.W > lo && r.Y < bottom && r.Y+r.H > min(a.Y, b.Y) {
			bottom = max(bottom, r.Y+r.H)
		}
	}
	// The middle of the gap, unless a lane's line or a frame's edge runs
	// along it there; then the row nearest the boxes.
	if l.rowTaken(bottom+rowGap/2, lo, hi) {
		return bottom
	}
	return bottom + rowGap/2
}

// rowTaken says a lane's line or a frame's edge runs along row y somewhere
// from x0 to x1.
func (l *layouter) rowTaken(y, x0, x1 int) bool {
	for _, g := range l.out.Groups {
		for _, d := range g.Dividers {
			if d[0].Y == y && d[1].Y == y && min(d[0].X, d[1].X) < x1 && max(d[0].X, d[1].X) >= x0 {
				return true
			}
		}
		r := g.Rect
		if g.Group.Style == Frame && (r.Y == y || r.Y+r.H-1 == y) && r.X < x1 && r.X+r.W > x0 {
			return true
		}
	}
	return false
}

// beside is the column a detour between a and b runs down: the middle of
// the gap to the right of the widest box the column between them holds.
func (l *layouter) beside(a, b Rect) int {
	lo, hi := min(a.Y, b.Y), max(a.Y+a.H, b.Y+b.H)
	right := max(a.X+a.W, b.X+b.W)
	for _, r := range l.rect {
		if r.Y < hi && r.Y+r.H > lo && r.X < right && r.X+r.W > min(a.X, b.X) {
			right = max(right, r.X+r.W)
		}
	}
	return right + colGap/2
}

// detour is the path from a to b round the boxes between them: for two in
// one row, out of a's bottom, along the gap beneath the row, and up into
// b's; for two in one column, out of a's right side, down the gap beside
// the column, and into b's. Each leaves and enters a little to the side of
// the middle, towards the other, so that the straight edge from the middle
// to a box beneath is not drawn over.
func detour(a, b Rect, below, beside int) []Point {
	if a.Y+a.H <= b.Y || b.Y+b.H <= a.Y {
		// One above the other: round the side.
		ay, by := a.Y+a.H/2, b.Y+b.H/2
		return []Point{{a.X + a.W - 1, ay}, {beside, ay}, {beside, by}, {b.X + b.W - 1, by}}
	}
	toward := func(r Rect, other Rect) int {
		c := r.X + r.W/2
		if other.X > r.X {
			return min(c+2, r.X+r.W-2)
		}
		return max(c-2, r.X+1)
	}
	ax, bx := toward(a, b), toward(b, a)
	ay1, by1 := a.Y+a.H-1, b.Y+b.H-1
	return []Point{{ax, ay1}, {ax, below}, {bx, below}, {bx, by1}}
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

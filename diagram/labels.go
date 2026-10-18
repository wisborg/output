package diagram

import (
	"sort"
	"strings"
)

// occupancy is which cells of a layout hold something a label must not be
// written over: an outline, a line, another label. Its rows run from -top,
// so a label can be placed above the layout's first row.
type occupancy struct {
	w, h, top int
	taken     [][]bool
}

func newOccupancy(w, h, top int) *occupancy {
	o := &occupancy{w: w, h: h, top: top, taken: make([][]bool, h+top)}
	for y := range o.taken {
		o.taken[y] = make([]bool, w)
	}
	return o
}

// clone is a copy of o, to be marked apart from it.
func (o *occupancy) clone() *occupancy {
	c := &occupancy{w: o.w, h: o.h, top: o.top, taken: make([][]bool, len(o.taken))}
	for y, row := range o.taken {
		c.taken[y] = append([]bool(nil), row...)
	}
	return c
}

// has says p is taken.
func (o *occupancy) has(p Point) bool {
	return p.X >= 0 && p.Y >= -o.top && p.X < o.w && p.Y < o.h && o.taken[p.Y+o.top][p.X]
}

func (o *occupancy) mark(x, y int) {
	if x >= 0 && y >= -o.top && x < o.w && y < o.h {
		o.taken[y+o.top][x] = true
	}
}

func (o *occupancy) line(a, b Point) {
	dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
	for p := a; ; p = (Point{p.X + dx, p.Y + dy}) {
		o.mark(p.X, p.Y)
		if p == b || dx == 0 && dy == 0 {
			return
		}
	}
}

func (o *occupancy) rect(r Rect, inside bool) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			if inside || y == r.Y || y == r.Y+r.H-1 || x == r.X || x == r.X+r.W-1 {
				o.mark(x, y)
			}
		}
	}
}

// labelRoom is how wide the cells labels are placed in are: the layout, and
// past its right edge room for the widest label. A fixed forty once: a
// label wider than that fitted nowhere, and was written over the boxes.
func (lay *Layout) labelRoom() int {
	widest := 0
	for _, e := range lay.Edges {
		for _, l := range strings.Split(e.Edge.Label, "\n") {
			widest = max(widest, Width(l))
		}
	}
	for _, g := range lay.Groups {
		widest = max(widest, Width(g.Group.Label)+2)
	}
	return lay.Width + widest + 2
}

// takeTitle takes the rows of the title and the blank row beneath it, the
// whole width, from o: no label is written over the title, nor in the gap
// that sets it off, where it reads as the title's second line. The router
// keeps lines out of them too.
func (lay *Layout) takeTitle(o *occupancy) {
	if lay.Title == "" {
		return
	}
	for y := range 2 {
		for x := range o.w {
			o.mark(x, y)
		}
	}
}

// laneLabelAt is where a lane's label begins: at the lane's far side, on
// its middle row. The router keeps lines off it, and labels.go writes it
// there.
func laneLabelAt(g PlacedGroup) Point {
	return Point{g.Rect.X + g.Rect.W - Width(g.Group.Label), g.Rect.Y + (g.Rect.H-1)/2}
}

// free says the block of lines from at is clear, and inside the layout.
func (o *occupancy) free(at Point, lines []string) bool {
	for i, l := range lines {
		for k := range Width(l) {
			x, y := at.X+k, at.Y+i
			if x < 0 || y < -o.top || x >= o.w || y >= o.h || o.taken[y+o.top][x] {
				return false
			}
		}
	}
	return true
}

// freeBut is free, but for cells in own, which a label may be written
// over: its own line's.
func (o *occupancy) freeBut(at Point, lines []string, own map[Point]bool) bool {
	for i, l := range lines {
		for k := range Width(l) {
			x, y := at.X+k, at.Y+i
			if x < 0 || y < -o.top || x >= o.w || y >= o.h || o.taken[y+o.top][x] && !own[Point{x, y}] {
				return false
			}
		}
	}
	return true
}

func (o *occupancy) take(at Point, lines []string) {
	for i, l := range lines {
		for k := range Width(l) {
			o.mark(at.X+k, at.Y+i)
		}
	}
}

// placeLabels decides where the groups' and the edges' labels are written,
// once, so that every renderer writes them in the same place.
//
// A frame's label is set in its top edge near its left corner, or further
// along where an edge crosses the edge there: the label must not hide where
// a line goes in or out. A lane's is at its far side, on its middle row.
//
// An edge's label goes beside the stretch labelSegments chooses -- the
// first the edge has to itself, where a branch leaves its decision -- a
// space to the right of a segment down a column or over one along a row.
// Where that place is taken, by an outline, a line or
// another label, it is moved to the nearest free place beside the same
// segment; where none is free, it stays, and is written over what is there
// rather than lost.
//
// A label may go above the first row -- over a line between two boxes of
// the top row, too close together for it to go between them -- and the
// layout then grows at the top to hold it, as placeStraightLabels lets a
// picture's. Not above a title, which is first whatever else is.
func (lay *Layout) placeLabels() {
	top := 4
	if lay.Title != "" {
		top = 0
	}
	// Room for labels past the boxes: a column of them to the right.
	o := newOccupancy(lay.labelRoom(), lay.Height+4, top)
	crossings := map[Point]bool{} // cells where a line runs down a column
	for _, e := range lay.Edges {
		for i := 1; i < len(e.Points); i++ {
			a, b := e.Points[i-1], e.Points[i]
			o.line(a, b)
			if a.X == b.X {
				for y := min(a.Y, b.Y); y <= max(a.Y, b.Y); y++ {
					crossings[Point{a.X, y}] = true
				}
			}
		}
	}
	// boxes is the boxes alone: where no free place is left for a label, it
	// is written over a line rather than over a box, which hides what the
	// box says.
	boxes := newOccupancy(lay.labelRoom(), lay.Height+4, top)
	lay.takeTitle(o)
	lay.takeTitle(boxes)
	for _, n := range lay.Nodes {
		o.rect(n.Rect, true)
		boxes.rect(n.Rect, true)
	}
	for i := range lay.Groups {
		g := &lay.Groups[i]
		for _, d := range g.Dividers {
			o.line(d[0], d[1])
		}
		r, label := g.Rect, g.Group.Label
		switch g.Group.Style {
		case Frame:
			o.rect(r, false)
			n := Width(label) + 2
			g.LabelAt = Point{r.X + 2, r.Y}
		search:
			for x := r.X + 2; x+n <= r.X+r.W-2; x++ {
				for k := range n {
					if crossings[Point{x + k, r.Y}] {
						continue search
					}
				}
				g.LabelAt = Point{x, r.Y}
				break
			}
		case Lane:
			g.LabelAt = laneLabelAt(*g)
		}
		if label != "" && g.Group.Style != Hidden {
			o.take(g.LabelAt, []string{" " + label + " "})
		}
	}
	shared := edgeCells(lay.Edges)
	for i := range lay.Edges {
		e := &lay.Edges[i]
		if e.Edge.Label == "" || len(e.Points) < 2 {
			continue
		}
		lines := strings.Split(e.Edge.Label, "\n")
		width := 0
		for _, l := range lines {
			width = max(width, Width(l))
		}
		tries := segmentTries(e.Points, shared, width, len(lines))
		// A try past the left edge is moved in to it, rather than passed
		// over: the tries to the left of a line at the edge were each
		// refused, and a long label in the bottom row of a grid was
		// written over the boxes though a free row was beside its line.
		for k := range tries {
			tries[k].X = max(tries[k].X, 0)
		}
		e.LabelAt = tries[0]
		placed := false
		for _, t := range tries {
			if o.free(t, lines) {
				e.LabelAt, placed = t, true
				break
			}
		}
		for _, t := range tries {
			if placed {
				break
			}
			if boxes.free(t, lines) {
				e.LabelAt, placed = t, true
			}
		}
		e.LabelAt.X, e.LabelAt.Y = max(e.LabelAt.X, 0), max(e.LabelAt.Y, -top)
		o.take(e.LabelAt, lines)
		lay.Width = max(lay.Width, e.LabelAt.X+width)
		lay.Height = max(lay.Height, e.LabelAt.Y+len(lines))
	}
	least := 0
	for _, e := range lay.Edges {
		if e.Edge.Label != "" && len(e.Points) >= 2 {
			least = min(least, e.LabelAt.Y)
		}
	}
	lay.shift(-least)
}

func length(a, b Point) int { return abs(b.X-a.X) + abs(b.Y-a.Y) }

// edgeCells is how many edges pass through each cell, counting each edge
// once however it turns there.
func edgeCells(edges []PlacedEdge) map[Point]int {
	n := map[Point]int{}
	for _, e := range edges {
		seen := map[Point]bool{}
		for k := 1; k < len(e.Points); k++ {
			a, b := e.Points[k-1], e.Points[k]
			dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
			for p := a; ; p = (Point{p.X + dx, p.Y + dy}) {
				if !seen[p] {
					seen[p] = true
					n[p]++
				}
				if p == b {
					break
				}
			}
		}
	}
	return n
}

// segmentTries are the places, in turn, a label width cells wide and n
// lines tall is tried at beside the path pts: beside each stretch
// labelSegments gives, a space to the right of one down a column, sliding
// along it, then to its left; over one along a row, sliding along it,
// then under it. A slide stays beside the stretch.
func segmentTries(pts []Point, shared map[Point]int, width, n int) []Point {
	var tries []Point
	segs := labelSegments(pts, shared)
	for _, seg := range segs {
		a, b := seg.a, seg.b
		// Sliding along the stretch, but never past its ends: a label
		// three rows beyond a stretch two cells long is beside nothing of
		// its edge's, and as near another's line as its own.
		if seg.down {
			lo, hi := min(a.Y, b.Y), max(a.Y, b.Y)
			mid := (a.Y + b.Y) / 2
			for _, d := range []int{0, 1, -1, 2, -2, 3, -3} {
				if y := mid + d; y >= lo && y <= hi {
					tries = append(tries, Point{a.X + 2, y})
				}
			}
			tries = append(tries, Point{a.X - 1 - width, mid})
		} else {
			lo, hi := min(a.X, b.X), max(a.X, b.X)
			mid := (a.X+b.X)/2 - width/2
			for _, d := range []int{0, 2, -2, 4, -4} {
				if x := mid + d; x <= hi && x+width-1 >= lo {
					tries = append(tries, Point{x, a.Y - n})
				}
			}
			tries = append(tries, Point{mid, a.Y + 1})
		}
	}
	// A label wider than the gap it is beside, between two boxes close
	// together, goes further out -- past the boxes' ends -- rather than
	// over them: a grid's gaps are its boxes', and never widen for a label.
	for k := 1; k <= 6; k++ {
		for _, seg := range segs {
			a, b := seg.a, seg.b
			if seg.down {
				mid := (a.Y + b.Y) / 2
				tries = append(tries, Point{a.X + 2 + k, mid}, Point{a.X - 1 - width - k, mid})
			} else {
				mid := (a.X+b.X)/2 - width/2
				tries = append(tries, Point{mid, a.Y - n - k}, Point{mid, a.Y + 1 + k})
			}
		}
	}
	return tries
}

// stretch is a run of a path's cells from a to b, along a row or down a
// column: which, said by down, since a stretch of one cell has a == b and
// its ends cannot say.
type stretch struct {
	a, b Point
	down bool
}

// labelSegments are the stretches of pts an edge's label is tried beside,
// in turn: first those, even of a single cell, that the edge has to itself,
// from the tail -- the first is where a branch leaves its decision, which
// is where a reader looks for "yes" or "no" -- and never a stem or a trunk
// it shares with other edges, where the labels of all of them would pile
// up and could not be told apart, unless it has nothing else; then its
// segments, the longest first. A later one is tried where every place
// beside the ones before is taken.
func labelSegments(pts []Point, shared map[Point]int) []stretch {
	var own, rest []stretch
	for k := 1; k < len(pts); k++ {
		a, b := pts[k-1], pts[k]
		rest = append(rest, stretch{a, b, a.X == b.X})
		// The runs of cells no other edge passes through: the ends of a
		// segment are where it meets the next, and are left out.
		dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
		start, n := Point{}, 0
		flush := func() {
			if n >= 1 {
				end := Point{start.X + (n-1)*dx, start.Y + (n-1)*dy}
				own = append(own, stretch{start, end, a.X == b.X})
			}
			n = 0
		}
		for p := (Point{a.X + dx, a.Y + dy}); p != b; p = (Point{p.X + dx, p.Y + dy}) {
			if shared[p] > 1 {
				flush()
				continue
			}
			if n == 0 {
				start = p
			}
			n++
		}
		flush()
	}
	sort.SliceStable(rest, func(i, j int) bool {
		return length(rest[i].a, rest[i].b) > length(rest[j].a, rest[j].b)
	})
	return append(own, rest...)
}

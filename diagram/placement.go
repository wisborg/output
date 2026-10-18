package diagram

import "strings"

// The costs a label's place is weighed by. A place is not refused for any
// of them: where nothing better is left, a label is written over a line,
// or even a box, rather than lost.
const (
	overSolidCost = 100000 // each cell over a box, the title or a group's label: it hides what they say
	overLineCost  = 1000   // each cell over a line
	touchCost     = 40     // a box's outline in the cell before or after a row of it: it reads as the box's
	touchRowCost  = 10     // a box's outline in the row above or below it, over its width
	touchLineCost = 30     // a line in the cell before or after a row of it: the text runs into it
	nearOtherCost = 60     // another edge's line nearer it than its own: it reads as the other's; half that as near
	pairCost      = 40     // another label beside it with two spaces or fewer between, or stacked on it: one phrase
	farCost       = 30     // each cell further from its own line than beside it
	rankCost      = 1      // each try before it: the order segmentTries prefers
)

// labelBlock is a label's lines at a place: at, and each line's width, so
// that the row of line i takes the cells from at.X to at.X+widths[i]-1 on
// row at.Y+i.
type labelBlock struct {
	at     Point
	widths []int
	w      int // the widest line's
}

func newBlock(at Point, lines []string) labelBlock {
	b := labelBlock{at: at, widths: make([]int, len(lines))}
	for i, l := range lines {
		b.widths[i] = Width(l)
		b.w = max(b.w, b.widths[i])
	}
	return b
}

func (b labelBlock) rows(f func(x0, x1, y int)) {
	for i, w := range b.widths {
		if w > 0 {
			f(b.at.X, b.at.X+w-1, b.at.Y+i)
		}
	}
}

// pair is what two labels' places cost together: overlapping, more than
// any other cost; side by side with two spaces or fewer between, or one on
// the row above the other over the same columns, pairCost.
func pair(a, b labelBlock) int {
	// Too far apart to touch: more than three columns or a row between.
	if a.at.X > b.at.X+b.w+3 || b.at.X > a.at.X+a.w+3 || a.at.Y > b.at.Y+len(b.widths) || b.at.Y > a.at.Y+len(a.widths) {
		return 0
	}
	cost := 0
	a.rows(func(ax0, ax1, ay int) {
		b.rows(func(bx0, bx1, by int) {
			switch {
			case ay == by && ax0 <= bx1 && bx0 <= ax1:
				cost += overSolidCost
			case ay == by && (bx0-ax1 <= 3 && bx0 > ax1 || ax0-bx1 <= 3 && ax0 > bx1):
				cost += pairCost
			case abs(ay-by) == 1 && ax0 <= bx1+1 && bx0 <= ax1+1:
				cost += pairCost
			}
		})
	})
	return cost
}

// labelPage is what the labels are placed against: the cells of what they
// must not be written over, of the lines, and of each edge's own line.
type labelPage struct {
	solid, lines *occupancy
	// whose is the edges whose lines pass through each cell.
	whose map[Point][]int
}

// placeCost is what writing edge i's label lines at at costs, against the
// page alone: the labels' costs to one another are pair's.
func (pg *labelPage) placeCost(i int, b labelBlock, rank int) int {
	cost := rank * rankCost
	inside := true
	b.rows(func(x0, x1, y int) {
		for x := x0; x <= x1; x++ {
			p := Point{x, y}
			switch {
			case x < 0 || y < -pg.solid.top || x >= pg.solid.w || y >= pg.solid.h:
				inside = false
			case pg.solid.has(p):
				cost += overSolidCost
			case pg.lines.has(p):
				cost += overLineCost
			}
			if pg.solid.has(Point{x, y - 1}) {
				cost += touchRowCost
			}
			if pg.solid.has(Point{x, y + 1}) {
				cost += touchRowCost
			}
		}
		for _, x := range []int{x0 - 1, x1 + 1} {
			switch {
			case pg.solid.has(Point{x, y}):
				cost += touchCost
			case pg.lines.has(Point{x, y}):
				cost += touchLineCost
			}
		}
	})
	if !inside {
		return -1
	}
	// How near its own line is, and whether another's is as near: the
	// nearest of each, by rings of cells about the block. A cell another
	// edge's line passes through too -- a trunk several share -- is not
	// its own but theirs: a label there reads as any of theirs.
	own, other := -1, -1
	for d := 1; d <= 6 && (own < 0 || other < 0); d++ {
		b.ring(d, func(p Point) {
			switch ws := pg.whose[p]; {
			case len(ws) == 1 && ws[0] == i:
				if own < 0 {
					own = d
				}
			case len(ws) > 0:
				if other < 0 {
					other = d
				}
			}
		})
	}
	if own < 0 {
		own = 7
	}
	cost += max(own-2, 0) * farCost
	switch {
	case other >= 0 && other < own:
		cost += nearOtherCost
	case other >= 0 && other == own:
		cost += nearOtherCost / 2 // as near either: less clear than its own nearer, clearer than another's
	}
	return cost
}

// ring calls f with each cell d from the block: d columns before or after
// a row of it, or d rows above or below, as far out across. A cell beside
// a row is its column distance away; a row's text is read across, so the
// space between a label and a line beside it is what says whose it is.
func (b labelBlock) ring(d int, f func(Point)) {
	x0, y0, x1, y1 := b.at.X-d, b.at.Y-d, b.at.X+b.w-1+d, b.at.Y+len(b.widths)-1+d
	for x := x0; x <= x1; x++ {
		f(Point{x, y0})
		f(Point{x, y1})
	}
	for y := y0 + 1; y < y1; y++ {
		f(Point{x0, y})
		f(Point{x1, y})
	}
}

// edgeLabel is one edge's label as the placement sees it: its lines, the
// places it may go, each with what it costs against the page, and the one
// it has.
type edgeLabel struct {
	edge   int
	lines  []string
	tries  []labelBlock
	costs  []int
	chosen int
}

// chooseLabelPlaces puts each label at one of its tries so that what they
// cost all together is as little as a local search finds: each placed in
// turn at its cheapest, given those before it; then each moved, over and
// over, to a cheaper place, until none moves. A move may take a place
// another label has, if that one, moved to its own cheapest place given
// the first's new one, leaves the two costing less than before. Placed
// first-fit, an earlier label took the place a later one needed and left
// it beside a box, or reading as another edge's; and moved one at a time,
// neither of two labels in each other's way would move first.
//
// Every move costs the whole less than it did, so the search ends.
func chooseLabelPlaces(labels []edgeLabel) {
	near := labelNeighbours(labels)
	block := func(k int) labelBlock { return labels[k].tries[labels[k].chosen] }
	// cost is what the labels in set cost where they are: their places,
	// and every pair one of them is in, each pair once.
	cost := func(set []int) int {
		c := 0
		for n, k := range set {
			c += labels[k].costs[labels[k].chosen]
			for _, j := range near[k] {
				if !slicesHasBefore(set, j, n) {
					c += pair(block(k), block(j))
				}
			}
		}
		return c
	}
	valid := func(k, t int) bool { return labels[k].costs[t] >= 0 }
	// best moves label k to its cheapest place given the others.
	best := func(k int) {
		bt, bc := labels[k].chosen, -1
		for t := range labels[k].tries {
			if !valid(k, t) || bc >= 0 && labels[k].costs[t] >= bc {
				continue
			}
			labels[k].chosen = t
			if c := cost([]int{k}); bc < 0 || c < bc {
				bt, bc = t, c
			}
		}
		labels[k].chosen = bt
	}
	// The first pass: each at its cheapest given those before it.
	for k := range labels {
		bt, bc := 0, -1
		for t := range labels[k].tries {
			if !valid(k, t) || bc >= 0 && labels[k].costs[t] >= bc {
				continue
			}
			c := labels[k].costs[t]
			for _, j := range near[k] {
				if j < k {
					c += pair(labels[k].tries[t], block(j))
				}
			}
			if bc < 0 || c < bc {
				bt, bc = t, c
			}
		}
		labels[k].chosen = bt
	}
	for pass := 0; pass < 50; pass++ {
		moved := false
		for k := range labels {
			from := labels[k].chosen
			if !valid(k, from) {
				continue
			}
			now := cost([]int{k})
			for t := range labels[k].tries {
				// Only a place that would cost k less than it pays now:
				// moving others for one no better is no gain.
				if t == from || !valid(k, t) || labels[k].costs[t] >= now {
					continue
				}
				// The labels in the way of k at t.
				at := labels[k].tries[t]
				set := []int{k}
				for _, j := range near[k] {
					if pair(at, block(j)) > 0 {
						set = append(set, j)
					}
				}
				if len(set) > 4 {
					continue
				}
				was := make([]int, len(set))
				for n, j := range set {
					was[n] = labels[j].chosen
				}
				before := cost(set)
				labels[k].chosen = t
				for _, j := range set[1:] {
					best(j)
				}
				if cost(set) < before {
					moved = true
					break
				}
				for n, j := range set {
					labels[j].chosen = was[n]
				}
			}
		}
		if !moved {
			return
		}
	}
}

// labelNeighbours is, for each label, the others some place of which is
// near enough some place of its own for pair to count: the only ones what
// it costs can depend on.
func labelNeighbours(labels []edgeLabel) [][]int {
	type span struct{ x0, y0, x1, y1 int }
	spans := make([]span, len(labels))
	for k, l := range labels {
		sp := span{x0: l.tries[0].at.X, y0: l.tries[0].at.Y, x1: l.tries[0].at.X, y1: l.tries[0].at.Y}
		for _, b := range l.tries {
			sp.x0, sp.y0 = min(sp.x0, b.at.X), min(sp.y0, b.at.Y)
			sp.x1, sp.y1 = max(sp.x1, b.at.X+b.w), max(sp.y1, b.at.Y+len(b.widths))
		}
		spans[k] = sp
	}
	near := make([][]int, len(labels))
	for k, a := range spans {
		for j, b := range spans {
			if j != k && a.x0 <= b.x1+3 && b.x0 <= a.x1+3 && a.y0 <= b.y1 && b.y0 <= a.y1 {
				near[k] = append(near[k], j)
			}
		}
	}
	return near
}

// slicesHasBefore says j is in set before index n: so that cost counts a
// pair of two labels both in set once.
func slicesHasBefore(set []int, j, n int) bool {
	for _, k := range set[:n] {
		if k == j {
			return true
		}
	}
	return false
}

// newLabelPage is the page lay's edge labels are placed on: the boxes, the
// title and the groups' labels as solid, the edges' and the groups' lines
// as lines.
func (lay *Layout) newLabelPage(top int) *labelPage {
	w, h := lay.labelRoom(), lay.Height+lay.labelRows()
	pg := &labelPage{solid: newOccupancy(w, h, top), lines: newOccupancy(w, h, top), whose: map[Point][]int{}}
	lay.takeTitle(pg.solid)
	for _, n := range lay.Nodes {
		pg.solid.rect(n.Rect, true)
	}
	for i, e := range lay.Edges {
		for k := 1; k < len(e.Points); k++ {
			pg.lines.line(e.Points[k-1], e.Points[k])
		}
		for p := range cellsOfPath(e.Points) {
			pg.whose[p] = append(pg.whose[p], i)
		}
	}
	for _, g := range lay.Groups {
		for _, d := range g.Dividers {
			pg.lines.line(d[0], d[1])
		}
		if g.Group.Style == Frame {
			pg.lines.rect(g.Rect, false)
		}
		if g.Group.Label != "" && g.Group.Style != Hidden {
			pg.solid.take(groupLabelAt(g), []string{" " + g.Group.Label + " "})
		}
	}
	return pg
}

// placeEdgeLabels decides where each edge's label is written: see
// placeLabels.
func (lay *Layout) placeEdgeLabels(top int) {
	pg := lay.newLabelPage(top)
	shared := edgeCells(lay.Edges)
	var labels []edgeLabel
	for i, e := range lay.Edges {
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
		l := edgeLabel{edge: i, lines: lines, tries: make([]labelBlock, len(tries)), costs: make([]int, len(tries))}
		for k, t := range tries {
			l.tries[k] = newBlock(t, lines)
			l.costs[k] = pg.placeCost(i, l.tries[k], k)
		}
		labels = append(labels, l)
	}
	chooseLabelPlaces(labels)
	for _, l := range labels {
		e := &lay.Edges[l.edge]
		e.LabelAt = l.tries[l.chosen].at
		e.LabelAt.X, e.LabelAt.Y = max(e.LabelAt.X, 0), max(e.LabelAt.Y, -top)
		width := 0
		for _, line := range l.lines {
			width = max(width, Width(line))
		}
		lay.Width = max(lay.Width, e.LabelAt.X+width)
		lay.Height = max(lay.Height, e.LabelAt.Y+len(l.lines))
	}
}

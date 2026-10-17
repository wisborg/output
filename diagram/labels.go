package diagram

import (
	"strings"
	"unicode/utf8"
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

// free says the block of lines from at is clear, and inside the layout.
func (o *occupancy) free(at Point, lines []string) bool {
	for i, l := range lines {
		for k := range utf8.RuneCountInString(l) {
			x, y := at.X+k, at.Y+i
			if x < 0 || y < -o.top || x >= o.w || y >= o.h || o.taken[y+o.top][x] {
				return false
			}
		}
	}
	return true
}

func (o *occupancy) take(at Point, lines []string) {
	for i, l := range lines {
		for k := range utf8.RuneCountInString(l) {
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
// An edge's label goes beside the segment labelSegment chooses -- the
// first the edge has to itself, where a branch leaves its decision -- a
// space to the right of a segment down a column or over one along a row.
// Where that place is taken, by an outline, a line or
// another label, it is moved to the nearest free place beside the same
// segment; where none is free, it stays, and is written over what is there
// rather than lost.
func (lay *Layout) placeLabels() {
	// Room for labels past the boxes: a column of them to the right.
	o := newOccupancy(lay.Width+40, lay.Height+4, 0)
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
	for _, n := range lay.Nodes {
		o.rect(n.Rect, true)
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
			n := utf8.RuneCountInString(label) + 2
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
			g.LabelAt = Point{r.X + r.W - utf8.RuneCountInString(label), r.Y + (r.H-1)/2}
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
			width = max(width, utf8.RuneCountInString(l))
		}
		pts := e.Points
		seg := labelSegment(pts, shared)
		a, b := pts[seg-1], pts[seg]
		var tries []Point
		if a.X == b.X {
			// Beside a segment down a column: to its right, sliding along
			// it, then to its left.
			mid := (a.Y + b.Y) / 2
			for _, d := range []int{0, 1, -1, 2, -2, 3, -3} {
				tries = append(tries, Point{a.X + 2, mid + d})
			}
			tries = append(tries, Point{a.X - 1 - width, mid})
		} else {
			// Over a segment along a row, sliding along it, then under it.
			mid := (a.X+b.X)/2 - width/2
			for _, d := range []int{0, 2, -2, 4, -4} {
				tries = append(tries, Point{mid + d, a.Y - len(lines)})
			}
			tries = append(tries, Point{mid, a.Y + 1})
		}
		e.LabelAt = tries[0]
		for _, t := range tries {
			if t.X >= 0 && t.Y >= 0 && o.free(t, lines) {
				e.LabelAt = t
				break
			}
		}
		e.LabelAt.X, e.LabelAt.Y = max(e.LabelAt.X, 0), max(e.LabelAt.Y, 0)
		o.take(e.LabelAt, lines)
		lay.Width = max(lay.Width, e.LabelAt.X+width)
		lay.Height = max(lay.Height, e.LabelAt.Y+len(lines))
	}
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

// labelSegment is the segment of pts, by the index of its end, an edge's
// label is written beside: the first from the tail at least two cells long
// that the edge has to itself -- where a branch leaves its decision, which
// is where a reader looks for "yes" or "no", and never a stem or a trunk it
// shares with other edges, where the labels of all of them would pile up
// and could not be told apart. Where it has none to itself, the last, or
// the longest where the last is too short to hold one.
func labelSegment(pts []Point, shared map[Point]int) int {
	for k := 1; k < len(pts); k++ {
		a, b := pts[k-1], pts[k]
		if length(a, b) < 2 {
			continue
		}
		own := true
		dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
		// The cells between its ends: a corner is where it meets another
		// segment, or another edge leaving the same side, and says nothing.
		for p := (Point{a.X + dx, a.Y + dy}); p != b; p = (Point{p.X + dx, p.Y + dy}) {
			if shared[p] > 1 {
				own = false
				break
			}
		}
		if own {
			return k
		}
	}
	seg := len(pts) - 1
	if length(pts[seg-1], pts[seg]) < 2 {
		for k := 1; k < len(pts); k++ {
			if length(pts[k-1], pts[k]) > length(pts[seg-1], pts[seg]) {
				seg = k
			}
		}
	}
	return seg
}

package diagram

import (
	"fmt"
	"strings"
	"testing"
)

// cellsOf is each cell of e's path, with the directions the path leaves
// it in.
func cellsOf(e PlacedEdge) map[Point]uint8 {
	cells := map[Point]uint8{}
	for k := 1; k < len(e.Points); k++ {
		a, b := e.Points[k-1], e.Points[k]
		dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
		for p := a; ; p = (Point{p.X + dx, p.Y + dy}) {
			if p != a {
				cells[p] |= dir(-dx, -dy)
			}
			if p != b {
				cells[p] |= dir(dx, dy)
			}
			if p == b {
				break
			}
		}
	}
	return cells
}

// Two edges that share neither their head nor their tail never meet but
// where one crosses the other at a right angle: no line taken over half
// way, no two drawn as one with a head at each end, no turn where another
// runs, each of which reads as a connection that is not there. A service
// mesh, every box with several edges, is where they used to.
func TestEdgesNeverJoin(t *testing.T) {
	ellipse := func(id string) Item { return Item{Node: &Node{ID: id, Shape: Ellipse}} }
	d := &Diagram{
		Root: Group{Columns: 3, Items: []Item{
			ellipse("api"), {Node: &Node{ID: "cache", Shape: Cylinder}}, {Node: &Node{ID: "db", Shape: Cylinder}},
			ellipse("worker"), node("queue"), ellipse("mail"),
		}},
		Edges: []Edge{
			{From: "api", To: "cache", Head: true}, {From: "cache", To: "api", Head: true},
			{From: "api", To: "db", Head: true}, {From: "api", To: "queue", Head: true},
			{From: "queue", To: "worker", Head: true}, {From: "worker", To: "db", Head: true},
			{From: "worker", To: "mail", Head: true}, {From: "db", To: "cache", Head: true, Line: Dashed},
		},
	}
	neverJoin(t, d)
}

// An edge with arrowheads at both ends, or none, shares its line with no
// other: a stem forking off it, or a trunk merging into it, could have come
// from either end -- out of a, along a's line to b with an arrowhead at
// each end, and off to d, read as out of a or of b -- and a fork of lines
// with no arrowheads reads as joining any two of the three. That holds
// whichever end of the other edge its arrowhead is at, dir=back's tail
// too. Nor does an edge share with one whose bare end is at the node its
// arrowhead is at: that is a chain through the node, not a merge.
func TestOnlyOneWayEdgesShareALine(t *testing.T) {
	grid := Group{Columns: 2, Items: nodes("a", "b", "c", "d")}
	for _, edges := range [][]Edge{
		{{From: "a", To: "b", Head: true, Tail: true}, {From: "a", To: "d", Head: true}},
		{{From: "a", To: "d", Head: true}, {From: "a", To: "b", Head: true, Tail: true}},
		{{From: "a", To: "b"}, {From: "a", To: "d"}},
		{{From: "b", To: "a", Head: true, Tail: true}, {From: "d", To: "a", Head: true}},
		// Arrowheads at both ends, and dir=back: into a, and out of it.
		{{From: "a", To: "b", Head: true, Tail: true}, {From: "a", To: "d", Tail: true}},
		{{From: "a", To: "d", Tail: true}, {From: "a", To: "b", Head: true, Tail: true}},
		{{From: "a", To: "b", Head: true, Tail: true}, {From: "d", To: "a", Tail: true}},
		{{From: "b", To: "a", Head: true, Tail: true}, {From: "a", To: "d", Tail: true}},
		// None, and dir=back.
		{{From: "a", To: "b"}, {From: "a", To: "d", Tail: true}},
		// A chain through a: into it with an arrowhead, out of it bare.
		{{From: "b", To: "a", Head: true}, {From: "d", To: "a", Tail: true}},
		{{From: "b", To: "a", Tail: true}, {From: "d", To: "a", Head: true}},
	} {
		neverJoin(t, &Diagram{Root: grid, Edges: edges})
	}
}

// Edges with one arrowhead each share a line where the arrowheads are at
// one node and the bare ends at two, or the other way round, whichever of
// From and To the arrowhead is at: dir=back runs one way as much as
// forward does, and a class diagram's arrows into one base class, written
// Base -> Derived [dir=back], merge into one arrowhead as Derived -> Base
// do.
func TestOneWayEdgesShareALineWhicheverWayWritten(t *testing.T) {
	grid := Group{Columns: 2, Items: nodes("a", "b", "c", "d")}
	for _, c := range []struct {
		what  string
		edges []Edge
	}{
		{"a stem out of a", []Edge{{From: "a", To: "b", Head: true}, {From: "a", To: "d", Head: true}}},
		{"a stem out of a, dir=back", []Edge{{From: "b", To: "a", Tail: true}, {From: "d", To: "a", Tail: true}}},
		{"a stem out of a, one each way", []Edge{{From: "a", To: "b", Head: true}, {From: "d", To: "a", Tail: true}}},
		{"a trunk into a", []Edge{{From: "b", To: "a", Head: true}, {From: "d", To: "a", Head: true}}},
		{"a trunk into a, dir=back", []Edge{{From: "a", To: "b", Tail: true}, {From: "a", To: "d", Tail: true}}},
		{"a trunk into a, one each way", []Edge{{From: "b", To: "a", Head: true}, {From: "a", To: "d", Tail: true}}},
	} {
		d := &Diagram{Root: grid, Edges: c.edges}
		neverJoin(t, d)
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		one, two := cellsOf(lay.Edges[0]), cellsOf(lay.Edges[1])
		shared := 0
		for p := range one {
			if _, ok := two[p]; ok {
				shared++
			}
		}
		if shared < 2 {
			t.Errorf("%s: the edges share %d cells, not a line:\n%s", c.what, shared, strings.Join(lay.Text(), "\n"))
		}
	}
}

// mayShare says two edges may share a line, as the router has it, written
// out here apart from it: each with an arrowhead at one end only, drawn
// alike, and the arrowheads at one node with the bare ends at two, or the
// bare ends at one with the arrowheads at two.
func mayShare(a, b Edge) bool {
	ends := func(e Edge) (arrow, bare string, ok bool) {
		if e.Head == e.Tail {
			return "", "", false
		}
		if e.Head {
			return e.To, e.From, true
		}
		return e.From, e.To, true
	}
	aArrow, aBare, aOk := ends(a)
	bArrow, bBare, bOk := ends(b)
	loop := a.From == a.To || b.From == b.To
	return !loop && aOk && bOk && a.Line == b.Line && (aArrow == bArrow) != (aBare == bBare)
}

// neverJoin fails the test where two of d's edges that may not share a
// line meet but crossing at a right angle, or where an edge passes through
// a box but its own two.
func neverJoin(t *testing.T, d *Diagram) {
	t.Helper()
	noneThrough(t, d)
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lay.Text(), "\n")
	for i, a := range lay.Edges {
		for _, b := range lay.Edges[i+1:] {
			if mayShare(*a.Edge, *b.Edge) {
				continue // these may share a trunk or a stem
			}
			cb := cellsOf(b)
			for p, bitsA := range cellsOf(a) {
				bitsB, both := cb[p]
				if !both || endsIn(lay, a, p) && endsIn(lay, b, p) {
					// Two ends in one node's cells: a plain node a letter
					// wide has one cell for every line to end at.
					continue
				}
				crossing := bitsA == up|down && bitsB == left|right || bitsA == left|right && bitsB == up|down
				if !crossing {
					t.Errorf("%s to %s and %s to %s meet at %v, not crossing:\n%s", a.Edge.From, a.Edge.To, b.Edge.From, b.Edge.To, p, text)
				}
			}
		}
	}
}

// noneThrough fails the test where an edge of d passes through a box but
// its own two.
func noneThrough(t *testing.T, d *Diagram) {
	t.Helper()
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lay.Text(), "\n")
	for _, e := range lay.Edges {
		for p := range cellsOf(e) {
			for _, n := range lay.Nodes {
				r := n.Rect
				inside := p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
				if inside && n.Node.ID != e.Edge.From && n.Node.ID != e.Edge.To {
					t.Errorf("%s to %s passes through %s at %v:\n%s", e.Edge.From, e.Edge.To, n.Node.ID, p, text)
				}
			}
		}
	}
}

// edges is the edges written "a>b c>d", each with a head.
func edges(s string) []Edge {
	var out []Edge
	for _, f := range strings.Fields(s) {
		ends := strings.Split(f, ">")
		out = append(out, Edge{From: ends[0], To: ends[1], Head: true})
	}
	return out
}

// nodes are boxes with the ids ids.
func nodes(ids ...string) []Item {
	var out []Item
	for _, id := range ids {
		out = append(out, node(id))
	}
	return out
}

// A line goes round the boxes above and to the left as it does below and to
// the right. The router had room only at the right and the bottom, and an
// edge between two boxes of the top row, the bottom crowded, fell back to the
// plain path -- straight through the box between them, where it read as
// that box's. And a to c and c to a, twice each, in a row of three, were
// drawn as one line.
func TestEdgesGoRoundTheTopAndTheLeft(t *testing.T) {
	neverJoin(t, &Diagram{
		Root:  Group{Columns: 3, Items: nodes("n0", "n1", "n2", "n3", "n4", "n5")},
		Edges: edges("n4>n0 n0>n2 n4>n0 n1>n0 n2>n3 n3>n4 n2>n5 n1>n0 n2>n0 n5>n4 n0>n3 n1>n3"),
	})
	neverJoin(t, &Diagram{
		Root:  Group{Columns: 3, Items: nodes("a", "b", "c")},
		Edges: edges("a>c c>a b>a b>c a>b c>b a>c c>a"),
	})
	neverJoin(t, &Diagram{
		Title: "a title",
		Root:  Group{Columns: 3, Items: nodes("a", "b", "c")},
		Edges: edges("a>c c>a b>a b>c a>b c>b a>c c>a"),
	})
}

// A diagram drawn as many do -- a grid of a dozen boxes and twice as many
// edges, a flow of forty and twice as many -- has no two lines joined and
// none through a box. One so crowded that the lines leave no way between
// some boxes may join two lines at last, but never runs one through a box:
// that is the worse lie, since it reads as the box's own line.
func TestEdgesNeverJoinOnRandomDiagrams(t *testing.T) {
	random := func(n, m, columns int, seed uint64) *Diagram {
		d := &Diagram{Root: Group{Columns: columns}}
		for i := range n {
			d.Root.Items = append(d.Root.Items, node(fmt.Sprintf("n%d", i)))
		}
		x := seed
		next := func() int { // xorshift: the same edges every run
			x ^= x << 13
			x ^= x >> 7
			x ^= x << 17
			return int(x % uint64(n))
		}
		for range m {
			if a, b := next(), next(); a != b {
				d.Edges = append(d.Edges, Edge{From: fmt.Sprintf("n%d", a), To: fmt.Sprintf("n%d", b), Head: true})
			}
		}
		return d
	}
	for seed := uint64(1); seed <= 3; seed++ {
		neverJoin(t, random(12, 24, 4, seed))
		neverJoin(t, random(40, 80, 0, seed))
		noneThrough(t, random(50, 98, 8, seed))
	}
}

// A plain node is its label alone: a line ends anywhere on it, its corners
// too, there being no outline to mistake them for. A one-letter label is a
// box a cell wide and a row tall, which had nowhere for a line to end but
// its corners, so every edge into one was drawn through the boxes between.
func TestEdgesReachPlainNodes(t *testing.T) {
	plain := func(id string) Item { return Item{Node: &Node{ID: id, Shape: Plain}} }
	d := &Diagram{
		Root:  Group{Columns: 3, Items: []Item{plain("a"), node("b"), plain("c"), node("d"), node("e"), node("f")}},
		Edges: edges("a>c d>a e>a f>c"),
	}
	neverJoin(t, d)
}

// An invisible edge is drawn nowhere and so routed nowhere: its path took
// cells from the edges that are drawn, and turned a straight line between
// neighbours into a detour. An edge to an invisible node ends at it, round
// the boxes between, rather than running on through them looking for an
// outline that is not there.
func TestInvisibleEdgesAndNodes(t *testing.T) {
	lay, err := (&Diagram{
		Root:  Group{Columns: 3, Items: nodes("a", "b", "c", "d", "e", "f")},
		Edges: []Edge{{From: "d", To: "e", Invisible: true}, {From: "d", To: "e", Head: true, Label: "seen"}},
	}).Layout()
	if err != nil {
		t.Fatal(err)
	}
	if p := lay.Edges[0].Points; p != nil {
		t.Errorf("the invisible edge has a path %v", p)
	}
	if p := lay.Edges[1].Points; len(p) != 2 || p[0].Y != p[1].Y {
		t.Errorf("d to e along %v, want one straight line along a row:\n%s", p, strings.Join(lay.Text(), "\n"))
	}
	gap := Item{Node: &Node{ID: "gap", Invisible: true}}
	noneThrough(t, &Diagram{Root: Group{Columns: 3, Items: []Item{node("a"), node("b"), gap}}, Edges: edges("a>gap")})
}

// A loop goes round outside its node, clear of what is about it: two on
// one node are two loops, and one in a frame is not drawn along the
// frame's edge, where it disappeared.
func TestLoops(t *testing.T) {
	d := &Diagram{
		Root: Group{Items: []Item{
			{Group: &Group{ID: "g", Label: "grp", Items: []Item{node("a")}}},
		}},
		Edges: []Edge{{From: "a", To: "a", Head: true, Label: "retry"}, {From: "a", To: "a", Head: true, Label: "again"}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lay.Text(), "\n")
	one, two := cellsOf(lay.Edges[0]), cellsOf(lay.Edges[1])
	a, _ := lay.Node("a")
	for p := range one {
		if _, both := two[p]; both && !inside(a.Rect, p.X, p.Y) {
			t.Errorf("the two loops meet at %v:\n%s", p, text)
		}
	}
	frame := lay.Groups[0].Rect
	for _, cells := range []map[Point]uint8{one, two} {
		run := 0
		for p, bits := range cells {
			onEdge := p.Y == frame.Y || p.Y == frame.Y+frame.H-1 || p.X == frame.X || p.X == frame.X+frame.W-1
			if onEdge && bits != up|down && bits != left|right {
				t.Errorf("a loop turns on the frame's edge at %v:\n%s", p, text)
			}
			if onEdge {
				run++
			}
		}
		if run > 2 {
			t.Errorf("a loop runs %d cells along the frame's edge:\n%s", run, text)
		}
	}
}

// endsIn says p is a cell of one of e's two nodes.
func endsIn(lay *Layout, e PlacedEdge, p Point) bool {
	for _, id := range []string{e.Edge.From, e.Edge.To} {
		if n, ok := lay.Node(id); ok && inside(n.Rect, p.X, p.Y) {
			return true
		}
	}
	return false
}

// A lane's label is not on a line: "a ──east──▶ b" reads as an edge
// called east. Lines go round it.
func TestLinesKeepOffLaneLabels(t *testing.T) {
	d := &Diagram{
		Root: Group{Columns: 2, Items: []Item{
			{Group: &Group{ID: "1", Style: Lane, Label: "east", Items: []Item{node("a")}}},
			{Group: &Group{ID: "2", Style: Lane, Label: "west", Items: []Item{node("b")}}},
		}},
		Edges: edges("a>b"),
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	cells := cellsOf(lay.Edges[0])
	for _, g := range lay.Groups {
		at := g.LabelAt
		for x := at.X - 1; x <= at.X+Width(g.Group.Label); x++ {
			if _, on := cells[Point{x, at.Y}]; on {
				t.Errorf("a to b runs by %q at %v:\n%s", g.Group.Label, Point{x, at.Y}, strings.Join(lay.Text(), "\n"))
			}
		}
	}
}

// A label too long for any free place beside its line is written over a
// line rather than over a box, and one wider than forty columns past the
// boxes -- once the most room there was -- still finds its place. A long
// label in the bottom row goes above its line, where its tries left of the
// edge were refused, not over the boxes.
func TestLongLabelsKeepOffBoxes(t *testing.T) {
	for _, d := range []*Diagram{
		{Root: Group{Columns: 2, Items: nodes("a", "b")}, Edges: []Edge{{From: "a", To: "b", Head: true, Label: strings.Repeat("long ", 13)}}},
		{Root: Group{Columns: 3, Items: nodes("a", "b", "c", "d", "e", "f", "g", "h", "i")}, Edges: []Edge{{From: "g", To: "h", Head: true, Label: "bottom long label"}}},
	} {
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		e := lay.Edges[0]
		for k := range Width(e.Edge.Label) {
			p := Point{e.LabelAt.X + k, e.LabelAt.Y}
			for _, n := range lay.Nodes {
				if inside(n.Rect, p.X, p.Y) {
					t.Errorf("%q is written over %s at %v:\n%s", e.Edge.Label, n.Node.ID, p, strings.Join(lay.Text(), "\n"))
					break
				}
			}
		}
	}
}

// BenchmarkLayoutFlow is a flow of a hundred boxes and two hundred edges:
// its time is almost all the router's search.
func BenchmarkLayoutFlow(b *testing.B) {
	d := &Diagram{}
	for i := range 100 {
		d.Root.Items = append(d.Root.Items, node(fmt.Sprintf("n%d", i)))
	}
	x := uint64(1)
	for range 200 {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		a, c := int(x%100), int((x>>20)%100)
		if a != c {
			d.Edges = append(d.Edges, Edge{From: fmt.Sprintf("n%d", a), To: fmt.Sprintf("n%d", c), Head: true})
		}
	}
	for b.Loop() {
		if _, err := d.Layout(); err != nil {
			b.Fatal(err)
		}
	}
}

// No edge label is written over the title, nor in the blank row beneath it,
// in the text drawing or a picture's straight edges: a label there reads as
// part of the title. Labels too wide for their gaps once went to row 0,
// over the title's own letters.
func TestLabelsKeepOffTheTitle(t *testing.T) {
	for _, straight := range []bool{false, true} {
		d := &Diagram{
			Title:    "The title of this diagram",
			Straight: straight,
			Root:     Group{Rows: 1, Items: nodes("a", "b", "c", "d")},
			Edges: []Edge{
				{From: "a", To: "b", Head: true, Label: "first long label wider than gap"},
				{From: "b", To: "c", Head: true, Label: "second long label wider than gap"},
				{From: "c", To: "d", Head: true, Label: "third long label wider than gap"},
			},
		}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range lay.Edges {
			for _, at := range []Point{e.LabelAt, e.PictureLabelAt()} {
				if at.Y < 2 {
					t.Errorf("straight %v: %q at row %d, in the title's rows:\n%s", straight, e.Edge.Label, at.Y, strings.Join(lay.Text(), "\n"))
				}
			}
		}
	}
}

// A label is given room for all its lines: one centred on its straight line
// near the bottom grows the page to hold it rather than running off it,
// and one taller than four lines may go above or below the boxes rather
// than over its own line. The text drawing ends with no blank rows a
// picture's labels added, and an edge with no label has no place for one.
func TestLabelRoom(t *testing.T) {
	tall := "1\n2\n3\n4\n5\n6\n7\n8\n9"
	for _, straight := range []bool{true, false} {
		d := &Diagram{Straight: straight, Root: Group{Rows: 1, Items: nodes("a", "b")},
			Edges: []Edge{{From: "a", To: "b", Head: true, Label: tall}, {From: "b", To: "a"}}}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		e := lay.Edges[0]
		for _, at := range []Point{e.LabelAt, e.PictureLabelAt()} {
			if at.Y < 0 || at.Y+9 > lay.Height {
				t.Errorf("straight %v: the label at row %d, 9 rows, on a page of %d", straight, at.Y, lay.Height)
			}
		}
		if !straight {
			cells := cellsOf(e)
			for k := range 9 {
				if _, on := cells[Point{e.LabelAt.X, e.LabelAt.Y + k}]; on {
					t.Errorf("the label's line %d is written over its own line:\n%s", k+1, strings.Join(lay.Text(), "\n"))
				}
			}
		}
		text := lay.Text()
		if len(text) > 0 && strings.TrimSpace(text[len(text)-1]) == "" {
			t.Errorf("straight %v: the text ends with a blank row:\n%q", straight, text)
		}
		if p := lay.Edges[1].LabelAt; p != (Point{}) {
			t.Errorf("straight %v: an edge with no label has LabelAt %v", straight, p)
		}
	}
}

// A lane's label keeps a free cell either side of its words: an edge's
// label set flush against it read as one word with it.
func TestLaneLabelKeepsItsSpace(t *testing.T) {
	lay, err := (&Diagram{Root: Group{Items: []Item{{Group: &Group{ID: "l", Style: Lane, Label: "east", Items: nodes("a")}}}}}).Layout()
	if err != nil {
		t.Fatal(err)
	}
	g := lay.Groups[0]
	if at := groupLabelAt(g); at.X != g.LabelAt.X-1 {
		t.Errorf("the lane's label is reserved from %v, its words from %v", at, g.LabelAt)
	}
}

// Where a search gives up -- a page too large to search -- an edge is
// drawn on the plain path: straight across what two boxes share, the
// second of two between them two cells off the first; out, along and in
// where they share nothing; and a loop out of its node's right side and
// into its bottom. Every search gives up here, so these are the paths.
func TestPlainPathsWhereSearchGivesUp(t *testing.T) {
	defer func(n int) { maxSearch = n }(maxSearch)
	maxSearch = 0
	d := &Diagram{
		// a above b, both wide enough for the second edge between them to
		// run two cells off the first.
		Root: Group{Columns: 2, Items: nodes("a is wide", "c", "b is wide")},
		Edges: []Edge{
			{From: "a is wide", To: "b is wide", Head: true}, {From: "a is wide", To: "b is wide", Head: true},
			{From: "a is wide", To: "c", Head: true}, {From: "b is wide", To: "c", Head: true},
			{From: "c", To: "c", Head: true},
		},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	r := map[string]Rect{}
	for _, n := range lay.Nodes {
		r[n.Node.ID] = n.Rect
	}
	if fmt.Sprint(lay.Edges[0].Points) == fmt.Sprint(lay.Edges[1].Points) {
		t.Errorf("the two edges from a to b are drawn as one: %v", lay.Edges[0].Points)
	}
	for k, want := range [][]Point{
		path(r["a is wide"], r["b is wide"], 0), path(r["a is wide"], r["b is wide"], 1),
		path(r["a is wide"], r["c"], 0), path(r["b is wide"], r["c"], 0),
		loop(r["c"]),
	} {
		if got := lay.Edges[k].Points; fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("edge %d: %v, want the plain path %v", k, got, want)
		}
	}

	// And the plain paths themselves, between boxes a row apart, a column
	// apart and neither.
	a, below, right, far := Rect{0, 0, 5, 3}, Rect{0, 5, 5, 3}, Rect{8, 0, 5, 3}, Rect{8, 6, 5, 3}
	wide, wideBelow := Rect{0, 0, 11, 3}, Rect{0, 5, 11, 3}
	for _, c := range []struct {
		a, b Rect
		nth  int
		want []Point
	}{
		{a, below, 0, []Point{{2, 2}, {2, 5}}},
		{below, a, 0, []Point{{2, 5}, {2, 2}}},
		{wide, wideBelow, 1, []Point{{7, 2}, {7, 5}}}, // two cells off the middle
		{wide, wideBelow, 2, []Point{{3, 2}, {3, 5}}},
		{wide, wideBelow, 3, []Point{{9, 2}, {9, 5}}}, // four, held to the cells inside the corners
		{a, below, 1, []Point{{3, 2}, {3, 5}}},
		{a, right, 0, []Point{{4, 1}, {8, 1}}},
		{right, a, 0, []Point{{8, 1}, {4, 1}}},
		{a, far, 0, []Point{{2, 2}, {2, 4}, {10, 4}, {10, 6}}}, // the gap below is wider than the one beside
		{far, a, 0, []Point{{10, 6}, {10, 4}, {2, 4}, {2, 2}}},
		{a, Rect{20, 3, 5, 3}, 0, []Point{{4, 1}, {12, 1}, {12, 4}, {20, 4}}},
	} {
		if got := path(c.a, c.b, c.nth); fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("path(%v, %v, %d) = %v, want %v", c.a, c.b, c.nth, got, c.want)
		}
	}
	if got, want := loop(a), []Point{{4, 1}, {7, 1}, {7, 4}, {3, 4}, {3, 2}}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("loop = %v, want %v", got, want)
	}
	for _, c := range []struct{ a0, a1, b0, b1, lo, hi int }{{0, 4, 0, 4, 1, 3}, {0, 4, 2, 9, 3, 3}} {
		if lo, hi, ok := span(c.a0, c.a1, c.b0, c.b1); !ok || lo != c.lo || hi != c.hi {
			t.Errorf("span(%d, %d, %d, %d) = %d, %d, %v", c.a0, c.a1, c.b0, c.b1, lo, hi, ok)
		}
	}
	if _, _, ok := span(0, 4, 3, 9); ok {
		t.Errorf("boxes sharing only a corner's cells share a span")
	}
}

// An edge passes another's arrowhead only to merge into it, so only where
// the two may share a line: into the same node, each one way, drawn alike.
// Into the arrowhead of an edge with one at each end, or of a dashed one
// where it is solid, it would cross the line there and cut the head off.
func TestMayPassAnArrowheadOnlyToMerge(t *testing.T) {
	rt := &router{edges: []Edge{
		{From: "x", To: "b", Head: true},
		{From: "a", To: "b", Head: true},
		{From: "a", To: "b", Head: true, Tail: true},
		{From: "c", To: "b", Head: true, Line: Dashed},
		{From: "x", To: "c", Head: true},
	}}
	for j, want := range []bool{1: true, 2: false, 3: false} {
		if j == 0 {
			continue
		}
		if got := rt.mayEnd(0, []int{j}, nil); got != want {
			t.Errorf("x to b past the head of %+v: %v, want %v", rt.edges[j], got, want)
		}
	}
	// And a tail only to fork from it: out of the same node, each one way.
	if !rt.mayEnd(0, nil, []int{4}) {
		t.Errorf("x to b past the tail of x to c: refused, want a stem forking")
	}
	rt.edges[4].Tail = true
	if rt.mayEnd(0, nil, []int{4}) {
		t.Errorf("x to b past the tail of x to c with an arrowhead there: allowed")
	}
	// The arrowhead of an edge written the other way round, dir=back, at
	// its tail: x to b may pass it into b, the two merging, but not the
	// bare tail of one out of b, which is a chain through b.
	rt.edges = append(rt.edges, Edge{From: "b", To: "y", Tail: true}, Edge{From: "b", To: "y", Head: true})
	if !rt.mayEnd(0, nil, []int{5}) {
		t.Errorf("x to b past the arrowhead of b to y, dir=back: refused, want a trunk merging")
	}
	if rt.mayEnd(0, nil, []int{6}) {
		t.Errorf("x to b past the bare tail of b to y: allowed")
	}
}

// on says the path pts leaves r by side at its first point: the point on
// that side's row or column, the next beyond it.
func on(pts []Point, r Rect, side Side) bool {
	p, q := pts[0], pts[1]
	switch side {
	case North:
		return p.Y == r.Y && q.Y < p.Y
	case South:
		return p.Y == r.Y+r.H-1 && q.Y > p.Y
	case West:
		return p.X == r.X && q.X < p.X
	case East:
		return p.X == r.X+r.W-1 && q.X > p.X
	}
	return false
}

// reversed is pts end first.
func reversed(pts []Point) []Point {
	out := make([]Point, len(pts))
	for i, p := range pts {
		out[len(pts)-1-i] = p
	}
	return out
}

// An edge leaves and enters by the sides its FromSide and ToSide ask for,
// each of the four out of one box into each of the four of another, and
// either side of a corner; asking for none leaves the router its choice.
func TestPortsSteerTheLine(t *testing.T) {
	sides := []Side{North, East, South, West}
	for _, from := range sides {
		for _, to := range sides {
			d := &Diagram{
				Root:  Group{Columns: 2, Items: nodes("a", "b", "c", "d")},
				Edges: []Edge{{From: "a", To: "d", Head: true, FromSide: from, ToSide: to}},
			}
			lay, err := d.Layout()
			if err != nil {
				t.Fatal(err)
			}
			e := lay.Edges[0]
			a, _ := lay.Node("a")
			b, _ := lay.Node("d")
			if e.SidesDropped || !on(e.Points, a.Rect, from) || !on(reversed(e.Points), b.Rect, to) {
				t.Errorf("a by %v to d by %v: %v, dropped %v:\n%s", from, to, e.Points, e.SidesDropped, strings.Join(lay.Text(), "\n"))
			}
		}
	}
	d := &Diagram{
		Root:  Group{Columns: 2, Items: nodes("a", "b", "c", "d")},
		Edges: []Edge{{From: "a", To: "d", Head: true, FromSide: South | West, ToSide: North | West}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	e := lay.Edges[0]
	a, _ := lay.Node("a")
	b, _ := lay.Node("d")
	if !(on(e.Points, a.Rect, South) || on(e.Points, a.Rect, West)) || !(on(reversed(e.Points), b.Rect, North) || on(reversed(e.Points), b.Rect, West)) {
		t.Errorf("a by its bottom or left to d by its top or left: %v:\n%s", e.Points, strings.Join(lay.Text(), "\n"))
	}
}

// Where no line can enter by the side asked for without joining another's
// -- d's top taken by three dashed arrowheads, which a solid line may not
// merge into -- the edge enters by another, joins no line, and says it
// was moved; one that never asked is never said to be. It still leaves
// by the side it asks for at its other end, which nothing blocks: one
// side that cannot be kept does not give up the other. And where the
// search gives up altogether, a plain path keeps no side either.
func TestPortsDroppedWhereNoLineFits(t *testing.T) {
	d := &Diagram{
		Root: Group{Columns: 3, Items: nodes("x", "y", "z", "m", "d", "q", "a")},
		Edges: []Edge{
			{From: "x", To: "d", Head: true, Line: Dashed, FromSide: South, ToSide: North},
			{From: "y", To: "d", Head: true, Line: Dashed, FromSide: South, ToSide: North},
			{From: "z", To: "d", Head: true, Line: Dashed, FromSide: South, ToSide: North},
			{From: "a", To: "d", Head: true, FromSide: West, ToSide: North},
			{From: "m", To: "q", Head: true},
		},
	}
	neverJoin(t, d)
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if a, _ := lay.Node("a"); !on(lay.Edges[3].Points, a.Rect, West) {
		t.Errorf("a to d leaves a by other than its left, which nothing blocks:\n%s", strings.Join(lay.Text(), "\n"))
	}
	for i, want := range []bool{false, false, false, true, false} {
		if got := lay.Edges[i].SidesDropped; got != want {
			t.Errorf("edge %d from %s: dropped %v, want %v:\n%s", i, lay.Edges[i].Edge.From, got, want, strings.Join(lay.Text(), "\n"))
		}
	}

	defer func(n int) { maxSearch = n }(maxSearch)
	maxSearch = 0
	d = &Diagram{
		Root:  Group{Columns: 2, Items: nodes("a", "b")},
		Edges: []Edge{{From: "a", To: "b", Head: true, FromSide: North}, {From: "b", To: "a", Head: true}},
	}
	if lay, err = d.Layout(); err != nil {
		t.Fatal(err)
	}
	if !lay.Edges[0].SidesDropped || lay.Edges[1].SidesDropped {
		t.Errorf("plain paths: dropped %v and %v, want true and false", lay.Edges[0].SidesDropped, lay.Edges[1].SidesDropped)
	}
}

// An edge that asks for a side is drawn along its path in a picture with
// straight edges: a straight line runs between the boxes' middles, out of
// whichever side it meets.
func TestPortsAreNeverStraight(t *testing.T) {
	d := &Diagram{
		Straight: true,
		Root:     Group{Columns: 2, Items: nodes("a", "b")},
		Edges:    []Edge{{From: "a", To: "b", Head: true, FromSide: South}, {From: "b", To: "a", Head: true}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if lay.Edges[0].Straight || !lay.Edges[1].Straight {
		t.Errorf("straight: %v and %v, want false and true", lay.Edges[0].Straight, lay.Edges[1].Straight)
	}
}

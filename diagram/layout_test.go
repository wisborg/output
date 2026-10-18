package diagram

import (
	"errors"
	"image/color"
	"math"
	"strings"
	"testing"
)

func node(id string) Item { return Item{Node: &Node{ID: id}} }

func rects(t *testing.T, d *Diagram) map[string]Rect {
	t.Helper()
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Rect{}
	for _, n := range lay.Nodes {
		out[n.Node.ID] = n.Rect
	}
	return out
}

func mid(r Rect) (int, int) { return r.X + r.W/2, r.Y + r.H/2 }

// apart fails the test where two boxes overlap or touch: whatever a layout
// does, every box is clear of every other.
func apart(t *testing.T, r map[string]Rect) {
	t.Helper()
	for a, ra := range r {
		for b, rb := range r {
			if a < b && ra.X <= rb.X+rb.W && rb.X <= ra.X+ra.W && ra.Y <= rb.Y+rb.H && rb.Y <= ra.Y+ra.H {
				t.Errorf("%s at %v and %s at %v overlap", a, ra, b, rb)
			}
		}
	}
}

// A grid fills its cells in the order its items are written, row by row,
// and a pinned item takes its own cell, the rest flowing round it.
func TestGridPlacesInOrder(t *testing.T) {
	d := &Diagram{Root: Group{Columns: 2, Items: []Item{node("a"), node("b"), node("c"), {Node: &Node{ID: "p", Row: 1, Col: 1}}}}}
	r := rects(t, d)
	apart(t, r)
	// p has row 1, column 1; a, b and c flow into the cells left: row 1
	// column 2, then row 2.
	px, py := mid(r["p"])
	ax, ay := mid(r["a"])
	bx, by := mid(r["b"])
	cx, cy := mid(r["c"])
	if !(px < ax && py == ay) {
		t.Errorf("a at (%d,%d) is not right of p at (%d,%d)", ax, ay, px, py)
	}
	if !(by > py && bx == px) || !(cy == by && cx == ax) {
		t.Errorf("b at (%d,%d) and c at (%d,%d) are not the second row", bx, by, cx, cy)
	}
}

// Two items pinned to one cell are refused, naming both.
func TestGridRefusesAClash(t *testing.T) {
	d := &Diagram{Root: Group{Columns: 2, Items: []Item{{Node: &Node{ID: "a", Col: 2}}, {Node: &Node{ID: "b", Row: 1, Col: 2}}}}}
	_, err := d.Layout()
	if err == nil || !strings.Contains(err.Error(), `"a" and "b"`) {
		t.Errorf("%v, want an error naming a and b", err)
	}
}

// The point of a grid: every box is where the diagram puts it, whatever
// the edges. A replica that stops replicating moves no box. Every box is
// where it was relative to the others; the whole drawing may move down or
// right, where a line goes round the boxes above or to the left of them,
// which moves no box out of its place in the grid.
func TestGridIgnoresEdges(t *testing.T) {
	build := func(edges []Edge) *Diagram {
		return &Diagram{
			Root: Group{Rows: 2, Items: []Item{
				{Group: &Group{ID: "east", Style: Lane, Label: "us-east-1", Columns: 3, Items: []Item{node("db1"), node("db2"), node("db3")}}},
				{Group: &Group{ID: "west", Style: Lane, Label: "us-west-2", Columns: 3, Items: []Item{node("dr1")}}},
			}},
			Edges: edges,
		}
	}
	relative := func(r map[string]Rect) map[string]Rect {
		o := r["db1"]
		for id, b := range r {
			r[id] = Rect{b.X - o.X, b.Y - o.Y, b.W, b.H}
		}
		return r
	}
	all := relative(rects(t, build([]Edge{{From: "db1", To: "db2"}, {From: "db1", To: "db3"}, {From: "db1", To: "dr1"}})))
	broken := relative(rects(t, build([]Edge{{From: "db1", To: "db2"}, {From: "db3", To: "dr1"}})))
	none := relative(rects(t, build(nil)))
	for id, r := range all {
		if broken[id] != r || none[id] != r {
			t.Errorf("%s at %v with every link, %v with one broken, %v with none", id, r, broken[id], none[id])
		}
	}
}

// Grids stacked in a grid share their columns: what is in the first column
// of one region is above what is in the first column of the next, though
// the boxes differ in width.
func TestStackedGridsShareColumns(t *testing.T) {
	d := &Diagram{Root: Group{Rows: 2, Items: []Item{
		{Group: &Group{ID: "east", Style: Lane, Label: "us-east-1", Columns: 2, Items: []Item{
			{Node: &Node{ID: "dbA", Label: "dbA primary, wide"}}, node("dbB")}}},
		{Group: &Group{ID: "west", Style: Lane, Label: "us-west-2", Columns: 2, Items: []Item{
			node("dbC"), {Node: &Node{ID: "dbD", Col: 2}}}}},
	}}}
	r := rects(t, d)
	apart(t, r)
	ax, _ := mid(r["dbA"])
	bx, _ := mid(r["dbB"])
	cx, _ := mid(r["dbC"])
	dx, _ := mid(r["dbD"])
	if cx != ax || dx != bx {
		t.Errorf("dbC's middle at %d under dbA's at %d, dbD's at %d under dbB's at %d: the columns are not shared", cx, ax, dx, bx)
	}
}

// Without a grid, an item goes a layer below those its edges come from,
// two branches of a step sit either side of it, and the step they meet at
// sits under the middle of them.
func TestLayersFollowEdges(t *testing.T) {
	d := &Diagram{
		Root: Group{Items: []Item{node("start"), node("check"), node("yes"), node("no"), node("end")}},
		Edges: []Edge{
			{From: "start", To: "check"}, {From: "check", To: "yes"}, {From: "check", To: "no"},
			{From: "yes", To: "end"}, {From: "no", To: "end"},
		},
	}
	r := rects(t, d)
	apart(t, r)
	sx, sy := mid(r["start"])
	cx, cy := mid(r["check"])
	yx, yy := mid(r["yes"])
	nx, ny := mid(r["no"])
	ex, ey := mid(r["end"])
	if !(sy < cy && cy < yy && yy == ny && ny < ey) {
		t.Errorf("layers at rows %d, %d, %d/%d, %d: not one below the other", sy, cy, yy, ny, ey)
	}
	if yx >= nx || abs(cx-(yx+nx)/2) > 1 || abs(ex-cx) > 1 || abs(sx-cx) > 1 {
		t.Errorf("start at %d, check at %d, the branches at %d and %d, end at %d: the flow is not centred on its step", sx, cx, yx, nx, ex)
	}
}

// A cycle still lays out: its first item, in the order written, is at the
// top.
func TestLayersBreakACycle(t *testing.T) {
	d := &Diagram{Root: Group{Items: []Item{node("a"), node("b"), node("c")}},
		Edges: []Edge{{From: "a", To: "b"}, {From: "b", To: "c"}, {From: "c", To: "a"}}}
	r := rects(t, d)
	apart(t, r)
	if !(r["a"].Y < r["b"].Y && r["b"].Y < r["c"].Y) {
		t.Errorf("a at row %d, b %d, c %d: want a, then b, then c", r["a"].Y, r["b"].Y, r["c"].Y)
	}
}

// A line from the first box of a row to the third goes round the second
// rather than through it, where it would read as a line from the second.
func TestEdgeGoesRoundABox(t *testing.T) {
	d := &Diagram{Root: Group{Columns: 3, Items: []Item{node("a"), node("b"), node("c")}},
		Edges: []Edge{{From: "a", To: "c", Head: true}}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	b, _ := lay.Node("b")
	pts := lay.Edges[0].Points
	for i := 1; i < len(pts); i++ {
		p, q := pts[i-1], pts[i]
		if min(p.X, q.X) <= b.Rect.X+b.Rect.W-1 && max(p.X, q.X) >= b.Rect.X &&
			min(p.Y, q.Y) <= b.Rect.Y+b.Rect.H-1 && max(p.Y, q.Y) >= b.Rect.Y {
			t.Fatalf("the edge from a to c, %v, passes through b at %v", pts, b.Rect)
		}
	}
}

// Every segment of every edge runs along a row or a column, from an
// outline cell of its tail to one of its head.
func TestEdgesRunAlongRowsAndColumns(t *testing.T) {
	d := &Diagram{Root: Group{Items: []Item{node("a"), node("b"), node("c"), node("d")}},
		Edges: []Edge{{From: "a", To: "b"}, {From: "a", To: "c"}, {From: "c", To: "d"}, {From: "b", To: "d"}, {From: "d", To: "d"}}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	onOutline := func(p Point, r Rect) bool {
		in := p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
		edge := p.X == r.X || p.X == r.X+r.W-1 || p.Y == r.Y || p.Y == r.Y+r.H-1
		return in && edge
	}
	for _, e := range lay.Edges {
		pts := e.Points
		for i := 1; i < len(pts); i++ {
			if pts[i].X != pts[i-1].X && pts[i].Y != pts[i-1].Y {
				t.Errorf("%s to %s: a diagonal segment %v to %v", e.Edge.From, e.Edge.To, pts[i-1], pts[i])
			}
		}
		from, _ := lay.Node(e.Edge.From)
		to, _ := lay.Node(e.Edge.To)
		if !onOutline(pts[0], from.Rect) || !onOutline(pts[len(pts)-1], to.Rect) {
			t.Errorf("%s to %s: %v does not run from outline to outline", e.Edge.From, e.Edge.To, pts)
		}
	}
}

func TestLayoutRefuses(t *testing.T) {
	for _, c := range []struct {
		d    *Diagram
		want string
	}{
		{&Diagram{Root: Group{Items: []Item{node("a")}}, Edges: []Edge{{From: "a", To: "x"}}}, `"x", which is no node`},
		{&Diagram{Root: Group{Items: []Item{node("a"), {Group: &Group{ID: "g", Items: []Item{node("a")}}}}}}, `"a" is in the diagram twice`},
	} {
		if _, err := c.d.Layout(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v, want an error saying %s", err, c.want)
		}
	}
}

// A label is not written over a line: the label of an edge crossing a
// lane's line, which would be in the middle of its edge, is moved beside
// it, clear of the line.
func TestLabelsKeepOffLines(t *testing.T) {
	d := &Diagram{
		Root: Group{Rows: 2, Items: []Item{
			{Group: &Group{ID: "east", Style: Lane, Label: "east", Columns: 1, Items: []Item{node("a")}}},
			{Group: &Group{ID: "west", Style: Lane, Label: "west", Columns: 1, Items: []Item{node("b")}}},
		}},
		Edges: []Edge{{From: "a", To: "b", Head: true, Label: "async"}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := lay.Text()
	at := lay.Edges[0].LabelAt
	if got := string([]rune(text[at.Y])[at.X : at.X+5]); got != "async" {
		t.Errorf("the label is not at %v, where Layout put it:\n%s", at, strings.Join(text, "\n"))
	}
	for _, g := range lay.Groups {
		for _, div := range g.Dividers {
			if div[0].Y == at.Y {
				t.Errorf("the label is on the lane's line, row %d:\n%s", at.Y, strings.Join(text, "\n"))
			}
		}
	}
}

// A label on a fill stands out from it in either palette: dark text on a
// pale fill even in the dark palette, light text on a deep one even in the
// light; without a fill, the foreground.
func TestTextOn(t *testing.T) {
	pale := color.RGBA{0xF5, 0xD7, 0x6E, 0xFF}
	deep := color.RGBA{0x20, 0x30, 0x80, 0xFF}
	for _, c := range []struct {
		pal  Palette
		fill color.Color
		want color.Color
	}{
		{Light, nil, Light.Foreground},
		{Dark, nil, Dark.Foreground},
		{Light, pale, Light.Foreground},
		{Dark, pale, Dark.Background},
		{Light, deep, Light.Background},
		{Dark, deep, Dark.Foreground},
		// Hardly there, a translucent pale fill is the dark background.
		{Dark, color.NRGBA{0xF5, 0xD7, 0x6E, 0x10}, Dark.Foreground},
	} {
		if got := c.pal.TextOn(c.fill); got != c.want {
			t.Errorf("%v on %v: %v, want %v", c.pal, c.fill, got, c.want)
		}
	}
}

// A legend goes below everything, in a frame of its own holding an entry
// to a row -- two rows for a label of two lines -- and moves nothing: every
// box is where it is without it.
func TestLegend(t *testing.T) {
	build := func(lg *Legend) *Diagram {
		return &Diagram{
			Root:   Group{Columns: 2, Items: []Item{node("a"), node("b")}},
			Edges:  []Edge{{From: "a", To: "b", Head: true, Label: "sync"}},
			Legend: lg,
		}
	}
	lg := &Legend{Label: "Key", Entries: []LegendEntry{
		{Label: "healthy", Fill: color.RGBA{0, 0xFF, 0, 0xFF}},
		{Label: "two\nlines"},
		{Label: "async", Sample: Stroke, Line: Dashed},
	}}
	with, err := build(lg).Layout()
	if err != nil {
		t.Fatal(err)
	}
	without, err := build(nil).Layout()
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range without.Nodes {
		if got, _ := with.Node(n.Node.ID); got.Rect != n.Rect {
			t.Errorf("%s at %v with a legend, %v without", n.Node.ID, got.Rect, n.Rect)
		}
	}
	if len(with.Legend) != 3 || len(with.Groups) != len(without.Groups)+1 {
		t.Fatalf("%d entries and %d groups, want 3 and one more than %d", len(with.Legend), len(with.Groups), len(without.Groups))
	}
	frame := with.Groups[len(with.Groups)-1]
	if frame.Group.Label != "Key" || frame.Group.Style != Frame || frame.Rect.Y < without.Height {
		t.Errorf("the legend's frame %+v, want Key, below row %d", frame, without.Height)
	}
	inside := func(r Rect) bool {
		return r.X > frame.Rect.X && r.Y > frame.Rect.Y && r.X+r.W < frame.Rect.X+frame.Rect.W && r.Y+r.H < frame.Rect.Y+frame.Rect.H
	}
	for i, e := range with.Legend {
		label := Rect{e.LabelAt.X, e.LabelAt.Y, len(e.Entry.Label), 1}
		if !inside(e.Sample) || !inside(label) || e.LabelAt.Y != e.Sample.Y || e.LabelAt.X <= e.Sample.X+e.Sample.W {
			t.Errorf("entry %d: sample %v, label at %v, not side by side in the frame %v", i, e.Sample, e.LabelAt, frame.Rect)
		}
	}
	if rows := []int{with.Legend[1].Sample.Y - with.Legend[0].Sample.Y, with.Legend[2].Sample.Y - with.Legend[1].Sample.Y}; rows[0] != 1 || rows[1] != 2 {
		t.Errorf("entries %v rows apart, want 1 and then 2 for the label of two lines", rows)
	}
	if with.Height != frame.Rect.Y+frame.Rect.H || with.Width < frame.Rect.W {
		t.Errorf("the layout is %d by %d, the legend's frame %v", with.Width, with.Height, frame.Rect)
	}
}

// An empty diagram, an empty cluster, and a legend with nothing else lay
// out rather than fail.
func TestLayoutEmpty(t *testing.T) {
	for name, d := range map[string]*Diagram{
		"empty":         {},
		"empty cluster": {Root: Group{Items: []Item{{Group: &Group{ID: "g", Label: "x"}}, node("b")}}},
		"legend alone":  {Legend: &Legend{Entries: []LegendEntry{{Label: "ok"}}}},
	} {
		if _, err := d.Layout(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Where several branches meet one step along a trunk they share, each
// branch's label is written by its own part of the line, not on the
// trunk, where they would pile up and could not be told apart.
func TestBranchLabelsAtTheirDecisions(t *testing.T) {
	d := &Diagram{
		Root: Group{Items: []Item{node("one"), node("two"), node("three"), node("go"), node("stop")}},
		Edges: []Edge{
			{From: "one", To: "two", Head: true, Label: "yes"},
			{From: "two", To: "three", Head: true, Label: "yes"},
			{From: "three", To: "go", Head: true, Label: "yes"},
			{From: "one", To: "stop", Head: true, Label: "no"},
			{From: "two", To: "stop", Head: true, Label: "no"},
			{From: "three", To: "stop", Head: true, Label: "no"},
		},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lay.Text(), "\n")
	// Each "no" is written next to a cell of its own edge that no other
	// edge passes through -- not on a trunk the three share -- and in a
	// place of its own.
	shared := edgeCells(lay.Edges)
	places := map[Point]bool{}
	for _, e := range lay.Edges {
		if e.Edge.Label != "no" {
			continue
		}
		near := false
		for k := 1; k < len(e.Points) && !near; k++ {
			a, b := e.Points[k-1], e.Points[k]
			dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
			for p := a; !near; p = (Point{p.X + dx, p.Y + dy}) {
				if shared[p] == 1 {
					for x := e.LabelAt.X; x < e.LabelAt.X+2; x++ {
						near = near || abs(p.X-x) <= 2 && abs(p.Y-e.LabelAt.Y) <= 1
					}
				}
				if p == b {
					break
				}
			}
		}
		if !near {
			t.Errorf("%s's no at %v is by no part of its own edge:\n%s", e.Edge.From, e.LabelAt, text)
		}
		if places[e.LabelAt] {
			t.Errorf("two no labels at %v:\n%s", e.LabelAt, text)
		}
		places[e.LabelAt] = true
	}
	// And no branch runs through the steps it skips.
	for _, e := range lay.Edges {
		for _, n := range lay.Nodes {
			if n.Node.ID == e.Edge.From || n.Node.ID == e.Edge.To {
				continue
			}
			r := n.Rect
			for i := 1; i < len(e.Points); i++ {
				p, q := e.Points[i-1], e.Points[i]
				if min(p.X, q.X) <= r.X+r.W-1 && max(p.X, q.X) >= r.X && min(p.Y, q.Y) <= r.Y+r.H-1 && max(p.Y, q.Y) >= r.Y {
					t.Errorf("%s to %s passes through %s:\n%s", e.Edge.From, e.Edge.To, n.Node.ID, text)
				}
			}
		}
	}
}

// With Straight, an edge whose line from box to box is clear of the other
// boxes is drawn straight, at an angle where its ends are not in line; one
// whose line would cross a box, and a loop, keep their paths. Two between
// the same nodes are set either side of the line between them. Without
// Straight, nothing is.
func TestStraightEdges(t *testing.T) {
	build := func(straight bool) *Diagram {
		return &Diagram{
			Root: Group{Columns: 3, Items: []Item{node("a"), node("b"), node("c"), node("d")}},
			Edges: []Edge{
				{From: "a", To: "d", Head: true, Label: "down"}, // d is under a: clear
				{From: "a", To: "c", Head: true},                // along the top row, through b
				{From: "b", To: "d", Head: true},                // the diagonals, both ways
				{From: "d", To: "b", Head: true},
				{From: "c", To: "c"},
			},
			Straight: straight,
		}
	}
	lay, err := build(true).Layout()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]PlacedEdge{}
	for _, e := range lay.Edges {
		got[e.Edge.From+e.Edge.To] = e
	}
	for key, want := range map[string]bool{"ad": true, "ac": false, "bd": true, "db": true, "cc": false} {
		if got[key].Straight != want {
			t.Errorf("%s straight %v, want %v", key, got[key].Straight, want)
		}
	}
	if s1, s2 := got["bd"].Shift, got["db"].Shift; s1 == 0 || s1 != -s2 {
		t.Errorf("b to d shifted %v and d to b %v: want them either side of the line", s1, s2)
	}
	// The label is clear of every box.
	at := got["ad"].PictureLabelAt()
	label := Rect{at.X, at.Y, len("down"), 1}
	for _, n := range lay.Nodes {
		r := n.Rect
		if label.X < r.X+r.W && r.X < label.X+label.W && label.Y < r.Y+r.H && r.Y < label.Y+label.H {
			t.Errorf("the label at %v is over %s at %v", at, n.Node.ID, r)
		}
	}
	plain, err := build(false).Layout()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range plain.Edges {
		if e.Straight {
			t.Errorf("%s to %s straight without Straight", e.Edge.From, e.Edge.To)
		}
	}
}

// A labelled edge between neighbours in a row of a grid, its label wider
// than the gap between them, moves no box: the label goes past the boxes'
// ends instead, clear of every box, in the text and in a picture. The whole
// drawing may grow at the top to hold it; the boxes keep their places
// relative to one another.
func TestLongLabelLeavesGridAlone(t *testing.T) {
	const label = "a long label"
	for _, straight := range []bool{false, true} {
		build := func(label string) *Diagram {
			return &Diagram{
				Root: Group{Rows: 2, Items: []Item{
					{Group: &Group{ID: "top", Columns: 3, Items: []Item{node("a"), node("b"), node("c")}}},
					{Group: &Group{ID: "bottom", Columns: 3, Items: []Item{node("x"), node("y"), node("z")}}},
				}},
				Edges:    []Edge{{From: "b", To: "a", Head: true, Label: label}},
				Straight: straight,
			}
		}
		with, without := rects(t, build(label)), rects(t, build(""))
		dy := with["a"].Y - without["a"].Y
		for id, r := range without {
			r.Y += dy
			if with[id] != r {
				t.Errorf("straight %v: %s at %v with the label, %v without", straight, id, with[id], r)
			}
		}
		lay, err := build(label).Layout()
		if err != nil {
			t.Fatal(err)
		}
		e := lay.Edges[0]
		for _, at := range []Point{e.LabelAt, e.PictureLabelAt()} {
			for id, r := range with {
				if at.Y >= r.Y && at.Y < r.Y+r.H && at.X < r.X+r.W && at.X+len(label) > r.X {
					t.Errorf("straight %v: the label at %v is over %s at %v:\n%s", straight, at, id, r, strings.Join(lay.Text(), "\n"))
				}
			}
		}
	}
}

// A straight edge's label is written on its own line, centred on it in a
// picture, where the line is alone there.
func TestStraightLabelOnItsLine(t *testing.T) {
	// Labels wider than a cell too: the line takes the cells either side
	// of it, and a label on it must be excused all of them.
	for _, label := range []string{"x", "ab", "abc", "label", "longer label"} {
		d := &Diagram{
			Root:     Group{Columns: 2, Items: []Item{node("a"), node("b"), node("c"), node("d")}},
			Edges:    []Edge{{From: "a", To: "d", Head: true, Label: label}},
			Straight: true,
		}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		e := lay.Edges[0]
		x0, y0, x1, y1 := lay.ends(e)
		at := e.PictureLabelAt()
		nx, ny := e.PictureLabelNudge()
		// The label's middle, in cell coordinates -- its first cell's
		// middle is half a cell in, so its own is half its width less
		// half a cell further -- and the line through it.
		mx, my := float64(at.X)+nx+float64(len(label))/2-0.5, float64(at.Y)+ny
		t0 := (my - y0) / (y1 - y0)
		lx := x0 + t0*(x1-x0)
		if d := mx - lx; d < -0.6 || d > 0.6 {
			t.Errorf("%q: the label's middle at (%.2f, %.2f), the line there at x %.2f: not on it", label, mx, my, lx)
		}
	}
}

// A straight edge's label written on its own line is excused only the
// line: where the line crosses a lane's divider, the label is not written
// along the divider.
func TestStraightLabelClearOfDivider(t *testing.T) {
	d := &Diagram{
		Root: Group{Rows: 2, Items: []Item{
			{Group: &Group{ID: "east", Style: Lane, Columns: 3, Items: []Item{node("a"), node("p"), node("q")}}},
			{Group: &Group{ID: "west", Style: Lane, Columns: 3, Items: []Item{node("r"), node("s"), node("b")}}},
		}},
		Edges:    []Edge{{From: "a", To: "b", Head: true, Label: "label"}},
		Straight: true,
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	div := newOccupancy(lay.Width+40, lay.Height+4, 0)
	for _, g := range lay.Groups {
		for _, dv := range g.Dividers {
			div.line(dv[0], dv[1])
		}
	}
	at := lay.Edges[0].PictureLabelAt()
	for k := range len("label") {
		if p := (Point{at.X + k, at.Y}); div.has(p) {
			t.Errorf("the label at %v is over the divider at %v", at, p)
		}
	}
}

// A picture moves a label on its own line by less than half a cell either
// way from the cells it took, whatever the label's width or lines: more,
// and it is drawn over cells nobody checked were free.
func TestStraightLabelNudgeWithinHalfACell(t *testing.T) {
	// Boxes of several widths, so the line crosses its middle row at
	// several fractions of a cell.
	for _, wide := range []string{"d", "dd", "ddd", "dddd", "ddddd"} {
		for _, label := range []string{"x", "ab", "abc", "abcd", "x\ny", "ab\ncd", "abc\nd\ne"} {
			d := &Diagram{
				Root:     Group{Columns: 2, Items: []Item{node("a"), node("b"), node("c"), {Node: &Node{ID: "d", Label: wide}}}},
				Edges:    []Edge{{From: "a", To: "d", Head: true, Label: label}},
				Straight: true,
			}
			lay, err := d.Layout()
			if err != nil {
				t.Fatal(err)
			}
			if nx, ny := lay.Edges[0].PictureLabelNudge(); math.Abs(nx) > 0.5 || math.Abs(ny) > 0.5 {
				t.Errorf("%q to a box %q: nudged (%.2f, %.2f)", label, wide, nx, ny)
			}
		}
	}
}

// A stretch of one cell along a row is a stretch along a row: its label is
// tried over and under it, not to its right and left, where the row's
// line runs.
func TestOneCellStretchAlongRow(t *testing.T) {
	pts := []Point{{0, 5}, {10, 5}}
	shared := map[Point]int{}
	for x := 1; x <= 8; x++ {
		shared[Point{x, 5}] = 2 // all but the cell before the end
	}
	tries := segmentTries(pts, shared, 3, 1)
	for _, at := range tries[:6] {
		if at.Y == 5 {
			t.Errorf("tried at %v, on the line's own row; tries %v", at, tries[:6])
		}
	}
}

// A diagram built in code is held to what DOT is: a grid of more than
// MaxGrid rows or columns, a pin past it, and an item that is neither a node
// nor a group are errors, not gigabytes or a nil dereference.
func TestLayoutLimits(t *testing.T) {
	for _, c := range []struct {
		name string
		d    *Diagram
		want string
	}{
		{"columns", &Diagram{Root: Group{Columns: MaxGrid + 1, Items: []Item{node("a")}}}, "columns"},
		{"a pin", &Diagram{Root: Group{Columns: 2, Items: []Item{{Node: &Node{ID: "a", Row: 3000}}}}}, "pinned to row 3000"},
		{"an empty item", &Diagram{Root: Group{Items: []Item{node("a"), {}}}}, "neither a node nor a group"},
	} {
		if _, err := c.d.Layout(); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an error saying %q", c.name, err, c.want)
		}
	}
}

// A page larger than LayoutOptions.MaxCells is refused before anything is
// routed: one node labelled with lines enough to need gigabytes of the
// router's memory, within a few kilobytes of DOT, by default; and a smaller
// limit refuses a smaller page.
func TestLayoutMaxCells(t *testing.T) {
	big := &Diagram{Root: Group{Items: []Item{{Node: &Node{ID: "a", Label: strings.Repeat(strings.Repeat("x", 1200)+"\n", 1200)}}}}}
	if _, err := big.Layout(); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a 1200 by 1200 label: %v, want ErrTooLarge", err)
	}
	small := &Diagram{Root: Group{Items: []Item{node("a"), node("b")}}, Edges: []Edge{{From: "a", To: "b"}}}
	if _, err := small.LayoutWith(LayoutOptions{MaxCells: 50}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("MaxCells 50: %v, want ErrTooLarge", err)
	}
	if _, err := small.Layout(); err != nil {
		t.Error(err)
	}
}

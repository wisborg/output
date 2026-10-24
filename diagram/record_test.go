package diagram

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// struct3 is Graphviz's own example of a record, "hello\nworld |{ b |{c|<here> d|e}| f}| g | h":
// fields in a row, one of them a column, one of whose fields is a row.
func struct3() *Node {
	return &Node{ID: "struct3", Record: []Field{
		{Label: "hello\nworld"},
		{Fields: []Field{{Label: "b"}, {Fields: []Field{{Label: "c"}, {Port: "here", Label: "d"}, {Label: "e"}}}, {Label: "f"}}},
		{Label: "g"},
		{Label: "h"},
	}}
}

// recordsHold checks every record of lay: its fields inside its outline,
// none overlapping another, and with the dividers covering every cell of
// the inside; each divider ending on the outline or on another divider;
// each field as wide and tall as its label.
func recordsHold(t *testing.T, lay *Layout) {
	t.Helper()
	for _, n := range lay.Nodes {
		if len(n.Node.Record) == 0 {
			continue
		}
		r := n.Rect
		inside := map[Point]string{}
		take := func(p Point, what string) {
			if p.X <= r.X || p.Y <= r.Y || p.X >= r.X+r.W-1 || p.Y >= r.Y+r.H-1 {
				t.Errorf("%s: %s at %v, off the inside of %v", n.Node.ID, what, p, r)
			}
			if was, ok := inside[p]; ok && !strings.HasPrefix(was, "divider") || ok && !strings.HasPrefix(what, "divider") {
				t.Errorf("%s: %s and %s both at %v", n.Node.ID, was, what, p)
			}
			inside[p] = what
		}
		for _, f := range n.Fields {
			for y := f.Rect.Y; y < f.Rect.Y+f.Rect.H; y++ {
				for x := f.Rect.X; x < f.Rect.X+f.Rect.W; x++ {
					take(Point{x, y}, "field "+f.Field.Label)
				}
			}
			lines := strings.Split(f.Field.Label, "\n")
			w := 0
			for _, l := range lines {
				w = max(w, Width(l))
			}
			if f.Rect.W < w+2 || f.Rect.H < len(lines) {
				t.Errorf("%s: field %q in %v, too small for it", n.Node.ID, f.Field.Label, f.Rect)
			}
		}
		onOutline := func(p Point) bool {
			return (p.X == r.X || p.X == r.X+r.W-1) && p.Y >= r.Y && p.Y < r.Y+r.H || (p.Y == r.Y || p.Y == r.Y+r.H-1) && p.X >= r.X && p.X < r.X+r.W
		}
		for k, d := range n.Dividers {
			for p := d[0]; ; {
				if p != d[0] && p != d[1] {
					take(p, fmt.Sprintf("divider %d", k))
				}
				if p == d[1] {
					break
				}
				p = Point{p.X + sign(d[1].X-d[0].X), p.Y + sign(d[1].Y-d[0].Y)}
			}
		}
		for k, d := range n.Dividers {
			for _, end := range d {
				if !onOutline(end) && !strings.HasPrefix(inside[end], "divider") {
					t.Errorf("%s: divider %d ends at %v, on neither the outline nor another divider", n.Node.ID, k, end)
				}
			}
		}
		for y := r.Y + 1; y < r.Y+r.H-1; y++ {
			for x := r.X + 1; x < r.X+r.W-1; x++ {
				if _, ok := inside[Point{x, y}]; !ok {
					t.Errorf("%s: %v inside it is neither a field nor a divider", n.Node.ID, Point{x, y})
				}
			}
		}
	}
}

// A record's fields fill its inside, a row of them where the layers run
// down and a column where they run across, as Graphviz lays them; a field
// divided further runs the other way; the text drawing joins each divider
// to the outline and writes each field's words.
func TestRecordFields(t *testing.T) {
	for _, dir := range []Direction{Down, Right, Up, Left} {
		d := &Diagram{Direction: dir, Title: "records", Straight: dir == Up, Root: Group{Items: []Item{
			{Node: struct3()},
			{Node: &Node{ID: "two", Shape: Rounded, Record: []Field{{Port: "a", Label: "one"}, {Port: "b", Label: "two"}}}},
			// A column of two beside a field of five lines: the column is
			// given three rows more than its own, which its fields share.
			{Node: &Node{ID: "tall", Record: []Field{{Fields: []Field{{Label: "x"}, {Label: "y"}}}, {Label: "1\n2\n3\n4\n5"}}}},
		}}, Edges: []Edge{{From: "two", FromPort: "b", To: "struct3", ToPort: "here", Head: true, Label: "a label"}}}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		recordsHold(t, lay)
		n, _ := lay.Node("struct3")
		var hello, g PlacedField
		for _, f := range n.Fields {
			switch f.Field.Label {
			case "hello\nworld":
				hello = f
			case "g":
				g = f
			}
		}
		if across := dir.across(); across != (g.Rect.Y > hello.Rect.Y) || !across && g.Rect.X <= hello.Rect.X {
			t.Errorf("%v: hello at %v and g at %v: want them side by side in a row where the layers run down, one above the other where they run across", dir, hello.Rect, g.Rect)
		}
		text := strings.Join(lay.Text(), "\n")
		for _, want := range []string{"hello", "world", " d ", "one", "two"} {
			if !strings.Contains(text, want) {
				t.Errorf("%v: no %q in:\n%s", dir, want, text)
			}
		}
		// Dividers meet the outline in tees across and down, and each
		// other in tees too: no line stops short of the one it meets.
		if !strings.ContainsAny(text, "┬┴") || !strings.ContainsAny(text, "├┤") {
			t.Errorf("%v: dividers not joined to the outline and to each other:\n%s", dir, text)
		}
	}
}

// An edge with a port ends on the outline beside its field, on a side the
// field reaches, even where another field is nearer the box it goes to; an
// edge without one never ends where a divider meets the outline, which
// reads as running into the divider -- in the middle of a record of two
// fields alike, where a line otherwise ends.
func TestRecordPorts(t *testing.T) {
	d := &Diagram{Root: Group{Items: []Item{
		{Node: &Node{ID: "s", Record: []Field{{Port: "l", Label: "left"}, {Port: "m", Label: "middle"}, {Port: "r", Label: "right"}}}},
		node("x"), node("y"), node("z"),
		{Node: &Node{ID: "pair", Record: []Field{{Label: "same"}, {Label: "same"}}}},
	}}}
	// Crossed: the leftmost field to the rightmost box, and the other way.
	for i, p := range []string{"l", "m", "r"} {
		d.Edges = append(d.Edges, Edge{From: "s", FromPort: p, To: []string{"z", "y", "x"}[i], Head: true})
	}
	d.Edges = append(d.Edges, Edge{From: "x", To: "s", Head: true}, Edge{From: "z", To: "s", Head: true}, Edge{From: "y", To: "pair", Head: true})
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	s, _ := lay.Node("s")
	joints := map[Point]bool{}
	for _, n := range lay.Nodes {
		for _, dv := range n.Dividers {
			joints[dv[0]], joints[dv[1]] = true, true
		}
	}
	for _, e := range lay.Edges {
		for _, end := range []Point{e.Points[0], e.Points[len(e.Points)-1]} {
			if joints[end] {
				t.Errorf("%s to %s ends at %v, where a divider meets the outline:\n%s", e.Edge.From, e.Edge.To, end, strings.Join(lay.Text(), "\n"))
			}
		}
		end := e.Points[0]
		if e.Edge.FromPort == "" {
			continue
		}
		f, _ := s.field(e.Edge.FromPort)
		if e.SidesDropped || end.X < f.X || end.X >= f.X+f.W {
			t.Errorf("s:%s ends at %v (dropped %v), not beside its field at %v:\n%s", e.Edge.FromPort, end, e.SidesDropped, f, strings.Join(lay.Text(), "\n"))
		}
	}
}

// Two edges at one node share a line only where they end beside the same
// field of it, or both anywhere on it: a line into a field and one into
// the whole record, merged, end beside the field, and the second reads as
// joining it. A port no field has is anywhere on the box, as no port is,
// and does not keep two lines apart -- a mistyped port, or one of a label
// this package does not read.
func TestRecordPortsShareOnlyTheirOwn(t *testing.T) {
	r := &PlacedNode{Node: &Node{ID: "r", Record: []Field{{Port: "p", Label: "p"}, {Port: "q", Label: "q"}}}, Rect: Rect{10, 10, 9, 3}}
	placeFields(r, r.Node.Record, Rect{11, 11, 7, 1}, true)
	boxes := map[string]Rect{"r": r.Rect, "x": {0, 0, 3, 3}, "y": {20, 0, 3, 3}, "b": {0, 20, 3, 3}}
	for _, c := range []struct {
		a, b  Edge
		share bool
	}{
		{Edge{From: "x", To: "r", ToPort: "p", Head: true}, Edge{From: "y", To: "r", Head: true}, false},
		{Edge{From: "x", To: "r", ToPort: "p", Head: true}, Edge{From: "y", To: "r", ToPort: "q", Head: true}, false},
		{Edge{From: "x", To: "r", ToPort: "p", Head: true}, Edge{From: "y", To: "r", ToPort: "p", Head: true}, true},
		{Edge{From: "r", FromPort: "p", To: "x", Head: true}, Edge{From: "r", FromPort: "p", To: "y", Head: true}, true},
		{Edge{From: "x", To: "r", ToPort: "nofield", Head: true}, Edge{From: "y", To: "r", Head: true}, true},
		{Edge{From: "b", To: "x", Head: true}, Edge{From: "b", FromPort: "p1", To: "y", Head: true}, true},
	} {
		l := &layouter{d: &Diagram{Edges: []Edge{c.a, c.b}}, out: &Layout{nodeAt: map[string]PlacedNode{}}, rect: boxes}
		for id, rc := range boxes {
			pn := PlacedNode{Node: &Node{ID: id}, Rect: rc}
			if id == "r" {
				pn = *r
			}
			l.out.Nodes = append(l.out.Nodes, pn)
			l.out.nodeAt[id] = pn
		}
		if got := newRouter(l).sharing(0, 1); got != c.share {
			t.Errorf("%s:%s -> %s:%s and %s:%s -> %s:%s: share %v, want %v", c.a.From, c.a.FromPort, c.a.To, c.a.ToPort,
				c.b.From, c.b.FromPort, c.b.To, c.b.ToPort, got, c.share)
		}
	}
}

// A record is drawn in a box, or a rounded one, whatever other Shape it is
// given: its fields fill a box's inside, and a cylinder's lid or an
// ellipse's curve would cut through them.
func TestRecordDrawnInABox(t *testing.T) {
	for _, c := range []struct {
		shape   Shape
		rounded bool
	}{{Box, false}, {Rounded, true}, {Ellipse, false}, {Cylinder, false}, {Plain, false}} {
		n := &Node{ID: "r", Shape: c.shape, Record: []Field{{Label: "a"}, {Label: "b"}}}
		lay, err := (&Diagram{Root: Group{Items: []Item{{Node: n}}}}).Layout()
		if err != nil {
			t.Fatal(err)
		}
		recordsHold(t, lay)
		text := lay.Text()
		if got := strings.ContainsRune(text[0], '╭'); got != c.rounded || len(text) != 3 || !strings.ContainsRune(text[0], '┬') {
			t.Errorf("shape %v: drawn\n%s", c.shape, strings.Join(text, "\n"))
		}
	}
}

// Lines is where every drawing writes a field's words: centred top to
// bottom, the odd row below.
func TestFieldLines(t *testing.T) {
	f := PlacedField{Field: &Field{Label: "a\nb"}, Rect: Rect{0, 10, 3, 5}}
	if lines, top := f.Lines(); len(lines) != 2 || top != 11 {
		t.Errorf("two lines in five rows from 10: %q from row %d, want from 11", lines, top)
	}
}

// Field finds a port among the fields not divided further, however deep:
// a port set on a divided field is not one, as Graphviz has none, and the
// layout ends an edge naming it anywhere on the box, as it does an edge
// naming no field.
func TestRecordFieldLookup(t *testing.T) {
	n := &Node{ID: "r", Record: []Field{
		{Port: "group", Fields: []Field{{Label: "x"}, {Fields: []Field{{Port: "deep", Label: "y"}}}}},
		{Port: "top", Label: "z"},
	}}
	for port, want := range map[string]string{"deep": "y", "top": "z", "group": "(none)", "": "(none)", "none": "(none)"} {
		got := "(none)"
		if f := n.Field(port); f != nil {
			got = f.Label
		}
		if got != want {
			t.Errorf("Field(%q): %q, want %q", port, got, want)
		}
	}
	d := &Diagram{Root: Group{Items: []Item{{Node: n}, node("a")}}, Edges: []Edge{{From: "a", To: "r", ToPort: "deep", Head: true}}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	pn, _ := lay.Node("r")
	if f, ok := pn.field("deep"); !ok || f.W == 0 {
		t.Errorf("the placed field of r:deep: %v %v", f, ok)
	}
	if _, ok := pn.field("group"); ok {
		t.Error("r:group, a divided field's port, placed as a field")
	}
}

// A straight line never leaves a record where a divider meets its outline,
// as a path along rows and columns never ends there: in a record of two
// fields alike it would leave from the middle of a side, right under the
// divider, and read as the divider running on. One leaving a record
// clear of its dividers stays straight; and in braille, no line's stub
// lies beside a divider's end.
func TestStraightLinesMissRecordJoints(t *testing.T) {
	d := &Diagram{Straight: true, Root: Group{Items: []Item{
		{Node: &Node{ID: "even", Record: []Field{{Label: "ab"}, {Label: "cd"}}}},
		{Node: &Node{ID: "odd", Record: []Field{{Label: "a"}, {Label: "a much wider field"}}}},
		node("y"), node("z"),
	}}, Edges: []Edge{{From: "even", To: "y", Head: true}, {From: "odd", To: "z", Head: true}}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range lay.Edges {
		if want := e.Edge.From == "odd"; e.Straight != want {
			t.Errorf("%s to %s: straight %v, want %v:\n%s", e.Edge.From, e.Edge.To, e.Straight, want, strings.Join(lay.Text(), "\n"))
		}
		if e.Straight {
			continue
		}
		for _, end := range []Point{e.Points[0], e.Points[len(e.Points)-1]} {
			n, _ := lay.Node(e.Edge.From)
			for _, dv := range n.Dividers {
				if end == dv[0] || end == dv[1] {
					t.Errorf("%s to %s ends at a divider's end, %v", e.Edge.From, e.Edge.To, end)
				}
			}
		}
	}
	noEndBesideJoints(t, lay, lay.Text(TextOptions{Braille: BrailleThin}))
}

// noEndBesideJoints checks text, a drawing of lay, has nothing in the cell
// outside each record's outline across from where a divider meets it: no
// arrowhead -- a braille line's stub carries one -- and no line running
// straight on from it, which would each read as the divider running on. A
// braille line's dots passing at a slant read as nothing of the kind.
func noEndBesideJoints(t *testing.T, lay *Layout, text []string) {
	t.Helper()
	at := func(p Point) rune {
		if p.Y < 0 || p.Y >= len(text) || p.X < 0 || p.X >= len([]rune(text[p.Y])) {
			return ' '
		}
		return []rune(text[p.Y])[p.X]
	}
	for _, n := range lay.Nodes {
		r := n.Rect
		for _, dv := range n.Dividers {
			for _, j := range dv {
				// The cell across from it, and the line that would run on
				// from the divider there.
				out, on := Point{j.X, r.Y - 1}, "│┼┬┴"
				switch {
				case j.Y == r.Y+r.H-1:
					out = Point{j.X, j.Y + 1}
				case j.X == r.X:
					out, on = Point{j.X - 1, j.Y}, "─┼├┤"
				case j.X == r.X+r.W-1:
					out, on = Point{j.X + 1, j.Y}, "─┼├┤"
				}
				if c := at(out); strings.ContainsRune("▲▼◀▶"+on, c) {
					t.Errorf("%q beside %s's divider at %v:\n%s", c, n.Node.ID, j, strings.Join(text, "\n"))
					return
				}
			}
		}
	}
}

// On diagrams of records and boxes made at random from fixed seeds, the
// straight lines among them -- set apart where several join two nodes, and
// drawn in braille with a stub into each box -- end beside no divider: the
// rule holds of lines shifted, and of stubs that slide along a side for
// want of room, not only of a lone line between two middles.
func TestStraightLinesMissRecordJointsAtRandom(t *testing.T) {
	for seed := int64(0); seed < 2000; seed++ {
		rnd := rand.New(rand.NewSource(seed))
		// Down, the joints on a record's top and bottom; across, its fields
		// stacked, on its left and right.
		d := &Diagram{Straight: true, Direction: []Direction{Down, Right}[seed%2]}
		n := 3 + rnd.Intn(4)
		for i := range n {
			id := fmt.Sprint("n", i)
			if rnd.Intn(2) == 0 {
				d.Root.Items = append(d.Root.Items, Item{Node: &Node{ID: id, Record: []Field{
					{Label: strings.Repeat("a", 1+rnd.Intn(4))}, {Label: strings.Repeat("b", 1+rnd.Intn(4))},
				}}})
			} else {
				d.Root.Items = append(d.Root.Items, node(id))
			}
		}
		for range n + rnd.Intn(4) {
			if a, b := rnd.Intn(n), rnd.Intn(n); a != b {
				d.Edges = append(d.Edges, Edge{From: fmt.Sprint("n", a), To: fmt.Sprint("n", b), Head: true, Tail: rnd.Intn(2) == 0})
			}
		}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range [][]string{lay.Text(), lay.Text(TextOptions{Braille: BrailleThin})} {
			noEndBesideJoints(t, lay, text)
		}
		if t.Failed() {
			t.Fatalf("seed %d", seed)
		}
	}
}

// A record's joints are where its dividers meet its outline, on whichever
// sides they do, and not where a field divided further has its dividers
// meet the one beside it, inside the box.
func TestRecordJoints(t *testing.T) {
	r := &PlacedNode{Node: &Node{ID: "r", Record: []Field{{Label: "a"}, {Fields: []Field{{Label: "b"}, {Label: "c"}}}}}, Rect: Rect{10, 10, 9, 5}}
	placeFields(r, r.Node.Record, Rect{11, 11, 7, 3}, true)
	// a | b over c: the divider down the middle meets the top and the
	// bottom, the one between b and c the right and the first divider.
	want := []Point{{14, 10}, {14, 14}, {18, 12}}
	if got := r.joints(); !reflect.DeepEqual(got, want) {
		t.Errorf("joints %v, want %v; dividers %v", got, want, r.Dividers)
	}
}

// The plain path drawn where every search fails ends where the search
// could have, not where a divider meets the outline: between a record of
// two fields alike and a box below it, the plain path runs down the middle,
// right under the divider. Moved a cell along, it stays straight and on
// both boxes' sides; and so does a loop's end.
func TestPlainPathMissesJoints(t *testing.T) {
	r := &PlacedNode{Node: &Node{ID: "r", Record: []Field{{Label: "ab"}, {Label: "cd"}}}, Rect: Rect{10, 10, 11, 3}}
	placeFields(r, r.Node.Record, Rect{11, 11, 9, 1}, true)
	below, narrow := Rect{10, 20, 11, 3}, Rect{13, 20, 4, 3}
	l := &layouter{d: &Diagram{}, out: &Layout{Nodes: []PlacedNode{*r}, nodeAt: map[string]PlacedNode{"r": *r}}, rect: map[string]Rect{"r": r.Rect}}
	rt := newRouter(l)
	plain := path(r.Rect, below, 0)
	if !rt.joints[plain[0]] {
		t.Fatalf("the plain path starts at %v, no joint: the test no longer tests anything", plain[0])
	}
	for _, c := range []struct {
		pts  []Point
		a, b Rect
	}{
		{plain, r.Rect, below}, {path(below, r.Rect, 0), below, r.Rect},
		// A box whose corner is a cell right of the joint: the end moves left.
		{path(r.Rect, narrow, 0), r.Rect, narrow},
	} {
		got := rt.offJoints(c.pts, c.a, c.b)
		first, last := got[0], got[len(got)-1]
		between := func(p Point, box Rect) bool { return p.X > box.X && p.X < box.X+box.W-1 }
		if rt.joints[first] || rt.joints[last] || first.X != last.X || !between(first, c.a) || !between(last, c.b) {
			t.Errorf("%v moved off the joints: %v", c.pts, got)
		}
	}
	if got := rt.offJoints(plain, r.Rect, below); abs(got[0].X-plain[0].X) != 1 {
		t.Errorf("moved to %v, want a cell from %v", got[0], plain[0])
	}
}

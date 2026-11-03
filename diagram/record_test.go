package diagram

import (
	"fmt"
	"math/rand"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// placeFields measures fields and places them in r, as placeNode does a
// record's.
func placeFields(pn *PlacedNode, fields []Field, r Rect, row bool) {
	ms, _ := measure(fields, row)
	place(pn, fields, ms, r, row)
}

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
		l := &layouter{d: &Diagram{Edges: []Edge{c.a, c.b}}, out: &Layout{nodeAt: map[string]PlacedNode{}}}
		for id, rc := range boxes {
			pn := PlacedNode{Node: &Node{ID: id}, Rect: rc}
			if id == "r" {
				pn = *r
			}
			l.out.Nodes = append(l.out.Nodes, pn)
			l.out.nodeAt[id] = pn
		}
		if got := newRouter(l, edgeFields(l)).sharing(0, 1); got != c.share {
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
	noEndBesideJoints(t, lay, TextOptions{Braille: BrailleThin})
}

// noEndBesideJoints checks lay drawn as text with o has nothing in the cell
// outside each record's outline across from where a divider meets it: no
// arrowhead -- a braille line's stub carries one -- and no line running
// straight on from it, which would each read as the divider running on. A
// braille line's dots passing at a slant read as nothing of the kind.
func noEndBesideJoints(t *testing.T, lay *Layout, o TextOptions) {
	t.Helper()
	text := lay.Text(o)
	at := func(p Point) rune {
		if p.Y < 0 || p.Y >= len(text) || p.X < 0 || p.X >= len([]rune(text[p.Y])) {
			return ' '
		}
		return []rune(text[p.Y])[p.X]
	}
	// A plain path, drawn where every search gave up, is drawn through
	// whatever is in the way, and may turn across from a joint: the cells
	// it takes are no line the search drew. A straight edge drawn in
	// braille draws no cell of its path.
	plain := map[Point]bool{}
	for _, e := range lay.Edges {
		drawn := e.plain && !(e.Straight && o.Braille != BrailleOff)
		for k := 1; drawn && k < len(e.Points); k++ {
			for p := e.Points[k-1]; ; p = step(p, e.Points[k]) {
				plain[p] = true
				if p == e.Points[k] {
					break
				}
			}
		}
	}
	for _, n := range lay.Nodes {
		// An invisible record's dividers are not drawn, so there is none
		// for a line to run on from.
		if n.Node.Invisible {
			continue
		}
		r := n.Rect
		for _, dv := range n.Dividers {
			for _, j := range dv {
				// A divider's end inside the box, on another divider, is no
				// joint: nothing outside is across from it.
				if j.X != r.X && j.X != r.X+r.W-1 && j.Y != r.Y && j.Y != r.Y+r.H-1 {
					continue
				}
				// The cell across from it, and the lines that would run on
				// from the divider there: every one with an arm its way --
				// an end, a turn, a junction -- but a line running past it
				// along the side.
				out, on := Point{j.X, r.Y - 1}, "│╎┌┐└┘├┤┬┴┼"
				switch {
				case j.Y == r.Y+r.H-1:
					out = Point{j.X, j.Y + 1}
				case j.X == r.X:
					out, on = Point{j.X - 1, j.Y}, "─╌┌┐└┘├┤┬┴┼"
				case j.X == r.X+r.W-1:
					out, on = Point{j.X + 1, j.Y}, "─╌┌┐└┘├┤┬┴┼"
				}
				if c := at(out); !plain[out] && strings.ContainsRune("▲▼◀▶"+on, c) {
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
		for _, o := range []TextOptions{{}, {Braille: BrailleThin}} {
			noEndBesideJoints(t, lay, o)
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
	// Invisible, nothing is drawn there for a line to run into.
	r.Node.Invisible = true
	if got := r.joints(); len(got) != 0 {
		t.Errorf("an invisible record's joints: %v, want none", got)
	}
}

// The plain path drawn where every search fails ends where the search
// could have: not where a divider meets the outline, nor beside it where
// its side has a cell clear of one, nor at a corner -- between a record of
// two fields alike and a box below it, the plain path runs down the
// middle, right under the divider. Moved along, it stays straight and on
// both boxes' sides; moved, it keeps to a field its edge asks for; with a
// box either side of the line, every move would run through one, the
// worse fault, and it stays.
func TestPlainPathMissesJoints(t *testing.T) {
	rec := &PlacedNode{Node: &Node{ID: "r", Record: []Field{{Port: "a", Label: "ab"}, {Port: "c", Label: "cd"}}}, Rect: Rect{10, 10, 11, 3}}
	placeFields(rec, rec.Node.Record, Rect{11, 11, 9, 1}, true)
	nodes := map[string]PlacedNode{
		"r":      *rec,
		"below":  {Node: &Node{ID: "below"}, Rect: Rect{10, 20, 11, 3}},
		"narrow": {Node: &Node{ID: "narrow"}, Rect: Rect{13, 20, 4, 3}},
	}
	router := func(edges []Edge, extra ...PlacedNode) *router {
		l := &layouter{d: &Diagram{Edges: edges}, out: &Layout{nodeAt: map[string]PlacedNode{}}}
		for id, n := range nodes {
			l.out.Nodes, l.out.nodeAt[id] = append(l.out.Nodes, n), n
		}
		l.out.Nodes = append(l.out.Nodes, extra...)
		return newRouter(l, edgeFields(l))
	}
	joint := map[Point]bool{}
	for _, j := range rec.joints() {
		joint[j] = true
	}
	if ideal := path(rec.Rect, nodes["below"].Rect, 0); !joint[ideal[0]] {
		t.Fatalf("the plain path starts at %v, no joint: the test no longer tests anything", ideal[0])
	}
	besideJ := func(p Point) bool {
		return joint[Point{p.X - 1, p.Y}] || joint[Point{p.X + 1, p.Y}] || joint[Point{p.X, p.Y - 1}] || joint[Point{p.X, p.Y + 1}]
	}
	between := func(p Point, box Rect) bool { return p.X > box.X && p.X < box.X+box.W-1 }
	// Into a box four cells wide, a straight line has two columns, one
	// under the joint and one beside it: beside it is the one place left.
	for _, c := range []struct {
		e      Edge
		beside bool
	}{{Edge{From: "r", To: "below"}, false}, {Edge{From: "below", To: "r"}, false}, {Edge{From: "r", To: "narrow"}, true}} {
		rt := router([]Edge{c.e})
		got := rt.plain(nodes[c.e.From], nodes[c.e.To], 0, 0)
		first, last := got[0], got[len(got)-1]
		if joint[first] || joint[last] || !c.beside && (besideJ(first) || besideJ(last)) || first.X != last.X || !between(first, nodes[c.e.From].Rect) || !between(last, nodes[c.e.To].Rect) {
			t.Errorf("%s to %s: %v, at or beside a joint, bent, or off a side", c.e.From, c.e.To, got)
		}
	}
	// Asked for the field a, left of the divider, or c, right of it, the
	// end moves into it.
	for _, port := range []string{"a", "c"} {
		rt := router([]Edge{{From: "r", FromPort: port, To: "below"}})
		f, _ := rec.field(port)
		if got := rt.plain(nodes["r"], nodes["below"], 0, 0); got[0].X < f.X || got[0].X >= f.X+f.W || besideJ(got[0]) || !keepsFields(got, rec.Rect, nodes["below"].Rect, rt.fields[0]) {
			t.Errorf("from r:%s, %v, not beside the field at %v", port, got, f)
		}
	}
	// With a box either side of the line, every move would run through
	// one: the path stays at the joint.
	rt := router([]Edge{{From: "r", To: "below"}}, PlacedNode{Node: &Node{ID: "left"}, Rect: Rect{11, 15, 4, 2}}, PlacedNode{Node: &Node{ID: "right"}, Rect: Rect{16, 15, 4, 2}})
	if got, ideal := rt.plain(nodes["r"], nodes["below"], 0, 0), path(rec.Rect, nodes["below"].Rect, 0); !reflect.DeepEqual(got, ideal) {
		t.Errorf("moved through a box beside the line: %v, want %v", got, ideal)
	}
	// Where the cells two along from the joint each have another edge's
	// line down them, an end there would join it: the end stays beside the
	// joint, which reads as running into the divider, rather than draw two
	// edges as one line, which reads as a connection that is not there.
	edges := []Edge{{From: "r", To: "below", Head: true}, {From: "below", To: "r", Head: true}, {From: "below", To: "r", Head: true}}
	rt = router(edges)
	rt.draw([]Point{{13, 20}, {13, 12}}, 1)
	rt.draw([]Point{{17, 20}, {17, 12}}, 2)
	got := rt.plain(nodes["r"], nodes["below"], 0, 0)
	if x := got[0].X; x == 13 || x == 17 || joint[got[0]] {
		t.Errorf("with lines down 13 and 17, the plain path from r is %v: on another's line, or at the joint", got)
	}
	// And where those lines run only through the gap, ending at neither
	// box, a stretch moved onto one would still run along it.
	rt = router(edges)
	rt.draw([]Point{{13, 14}, {13, 18}}, 1)
	rt.draw([]Point{{17, 14}, {17, 18}}, 2)
	if got := rt.plain(nodes["r"], nodes["below"], 0, 0); got[0].X == 13 || got[0].X == 17 || joint[got[0]] {
		t.Errorf("with lines through the gap at 13 and 17, the plain path from r is %v: along another's line, or at the joint", got)
	}
	// Not moving is priced as a move is: a plain path between two boxes,
	// down the column another edge's line takes the other way, was drawn
	// along it -- a to b and b to a as one line with an arrowhead at each
	// end -- where a cell over was free.
	nodes["top"] = PlacedNode{Node: &Node{ID: "top"}, Rect: Rect{40, 2, 11, 3}}
	nodes["bottom"] = PlacedNode{Node: &Node{ID: "bottom"}, Rect: Rect{40, 12, 11, 3}}
	rt = router([]Edge{{From: "top", To: "bottom", Head: true}, {From: "bottom", To: "top", Head: true}})
	other := path(nodes["bottom"].Rect, nodes["top"].Rect, 0)
	rt.draw(other, 1)
	if got := rt.plain(nodes["top"], nodes["bottom"], 0, 0); got[0].X == other[0].X {
		t.Errorf("the plain path %v runs along the other edge's line %v", got, other)
	}
	// And where the other line runs only through the gap, ending at
	// neither box and with no arrowhead there, so that only the line in
	// the cells between says so.
	rt = router([]Edge{{From: "top", To: "bottom", Head: true}, {From: "bottom", To: "top", Head: true}})
	gap := []Point{{other[0].X, 10}, {other[0].X, 6}}
	rt.line(gap[0], gap[1], 1)
	if got := rt.plain(nodes["top"], nodes["bottom"], 0, 0); got[0].X == gap[0].X {
		t.Errorf("the plain path %v runs along the other edge's line %v in the gap", got, gap)
	}
	// A stretch moved is priced cell by cell as the search's steps are: it
	// does not pass the cell before another's arrowhead, cutting it off,
	// where a move as cheap the other way passes none.
	edges = append(edges, Edge{From: "x", To: "y", Head: true})
	rt = router(edges)
	rt.heads[16][17] = []int{3}
	if got := rt.plain(nodes["r"], nodes["below"], 0, 0); got[0].X == 17 || joint[got[0]] {
		t.Errorf("with an arrowhead's cell at 17,16, the plain path from r is %v: through it, or at the joint", got)
	}
	// A box in the gap that the plain path crosses whether it moves or not
	// does not keep the end at the joint: a move is refused only for
	// taking the path through more of the boxes than it crossed already.
	rt = router([]Edge{{From: "r", To: "below"}}, PlacedNode{Node: &Node{ID: "across"}, Rect: Rect{11, 15, 9, 2}})
	if got := rt.plain(nodes["r"], nodes["below"], 0, 0); joint[got[0]] {
		t.Errorf("with a box across the gap, the plain path stays at the joint: %v", got)
	}
	// A plain path that turns: out of tall's right side at (50, 6), along
	// to its corner at (60, 6), down to (60, 18) and on into low. Moving
	// its tail moves that corner up or down the column.
	nodes["tall"] = PlacedNode{Node: &Node{ID: "tall"}, Rect: Rect{40, 2, 11, 9}}
	nodes["low"] = PlacedNode{Node: &Node{ID: "low"}, Rect: Rect{70, 14, 11, 9}}
	turn := Point{60, 6}
	if ideal := path(nodes["tall"].Rect, nodes["low"].Rect, 0); len(ideal) != 4 || ideal[1] != turn {
		t.Fatalf("the plain path from tall to low is %v, turning at no %v: the test no longer tests anything", ideal, turn)
	}
	edges = []Edge{{From: "tall", To: "low", Head: true}, {From: "x", To: "y", Head: true}}
	// It does not turn on a straight line a picture draws, as the search
	// does not, but may cross one.
	rt = router(edges)
	rt.slant[turn.Y][turn.X] = true
	if got := rt.plain(nodes["tall"], nodes["low"], 0, 0); got[1] == turn {
		t.Errorf("the plain path %v turns on a straight line at %v", got, turn)
	}
	// Asking outline as the search's last try does, it leaves the router
	// as it found it, mid-try or not.
	for _, any := range []bool{false, true} {
		rt.anySide = any
		if rt.plain(nodes["tall"], nodes["low"], 0, 0); rt.anySide != any {
			t.Errorf("anySide %v before the plain path, %v after", any, rt.anySide)
		}
	}
	// A corner is one cell and is priced once, though both its stretches
	// pass it. On the cell before another's arrowhead it costs one join,
	// the same as each move down, which runs past the cell before an
	// arrowhead of its own, and each move up, whose column runs on past
	// the corner's; not moving is the cheapest. Charged twice, the corner came
	// to two joins and the end moved for nothing.
	rt = router(edges)
	rt.heads[turn.Y][turn.X] = []int{1}
	for d := 1; d <= 3; d++ {
		rt.heads[turn.Y+d][55] = []int{1}
	}
	if got := rt.plain(nodes["tall"], nodes["low"], 0, 0); got[1] != turn {
		t.Errorf("with the corner before an arrowhead and every move past one, the plain path %v turns elsewhere than %v", got, turn)
	}
	// A loop round a box three or four cells wide ended at its corner.
	for _, w := range []int{3, 4} {
		nodes["thin"] = PlacedNode{Node: &Node{ID: "thin"}, Rect: Rect{30, 10, w, 3}}
		rt := router([]Edge{{From: "thin", To: "thin"}})
		got := rt.plain(nodes["thin"], nodes["thin"], 0, 0)
		r := nodes["thin"].Rect
		for _, p := range []Point{got[0], got[len(got)-1]} {
			if (p.X == r.X || p.X == r.X+r.W-1) && (p.Y == r.Y || p.Y == r.Y+r.H-1) {
				t.Errorf("a loop round a box %d wide ends at its corner: %v", w, got)
			}
		}
	}
	// A plain path from src to dst turns at (15, 13), straight out from
	// r's bottom joint, and on down from it in line with the divider: it
	// moves its corner off, as the search never turns there.
	nodes = map[string]PlacedNode{
		"r":   *rec,
		"src": {Node: &Node{ID: "src"}, Rect: Rect{0, 11, 5, 5}},
		"dst": {Node: &Node{ID: "dst"}, Rect: Rect{26, 20, 5, 5}},
	}
	out := Point{15, 13}
	if ideal := path(nodes["src"].Rect, nodes["dst"].Rect, 0); len(ideal) != 4 || ideal[1] != out || !joint[Point{out.X, out.Y - 1}] {
		t.Fatalf("the plain path from src to dst is %v, turning at no %v under a joint: the test no longer tests anything", ideal, out)
	}
	rt = router([]Edge{{From: "src", To: "dst", Head: true}})
	got = rt.plain(nodes["src"], nodes["dst"], 0, 0)
	for _, p := range got[1 : len(got)-1] {
		if slices.Contains(rec.outsideJoints(), p) {
			t.Errorf("the plain path %v turns straight out from one of r's joints, at %v", got, p)
		}
	}
	for k, end := range []Point{got[0], got[len(got)-1]} {
		r := nodes[[]string{"src", "dst"}[k]].Rect
		onOutline := inside(r, end.X, end.Y) && (end.X == r.X || end.X == r.X+r.W-1 || end.Y == r.Y || end.Y == r.Y+r.H-1)
		if !onOutline || joint[end] {
			t.Errorf("the plain path %v ends at %v, off its box's outline %v or at a joint", got, end, r)
		}
	}
}

// A line into a field ends a cell or more from where the field's dividers
// meet the outline, where the field has such a cell: one beside a joint,
// drawn ┴┬, reads as running into the divider. A field two cells wide has
// none, nor the side of a field a row tall, and a line still ends at it.
func TestRecordEndsAwayFromJoints(t *testing.T) {
	for _, c := range []struct {
		name string
		dir  Direction
		to   []Field
		port string
	}{
		{"a field of a column of fields", Down, []Field{{Fields: []Field{{Label: "a"}, {Label: "b"}}}, {Fields: []Field{{Port: "c", Label: "c"}, {Label: "d"}}}}, "c"},
		{"a field of a row of fields, across", Right, []Field{{Fields: []Field{{Port: "x", Label: "o\nn\ne"}, {Label: "two"}}}, {Label: "three"}}, "x"},
		{"a field of one letter", Down, []Field{{Label: "left"}, {Port: "m", Label: "m"}, {Label: "right"}}, "m"},
	} {
		d := &Diagram{Direction: c.dir, Root: Group{Items: []Item{node("a"), {Node: &Node{ID: "r", Record: c.to}}}}}
		d.Edges = []Edge{{From: "a", To: "r", ToPort: c.port, Head: true}}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		r, _ := lay.Node("r")
		e := lay.Edges[0]
		end := e.Points[len(e.Points)-1]
		for _, j := range r.joints() {
			if abs(end.X-j.X)+abs(end.Y-j.Y) <= 1 {
				t.Errorf("%s: the line ends at %v, beside the joint %v:\n%s", c.name, end, j, strings.Join(lay.Text(), "\n"))
			}
		}
		if e.SidesDropped {
			t.Errorf("%s: the port is not kept:\n%s", c.name, strings.Join(lay.Text(), "\n"))
		}
	}
	// An empty field is two cells wide, both beside a joint: the line ends
	// there all the same, by the field it is for.
	d := &Diagram{Root: Group{Items: []Item{node("a"), {Node: &Node{ID: "r", Record: []Field{{Label: "left"}, {Port: "e"}, {Label: "right"}}}}}}}
	d.Edges = []Edge{{From: "a", To: "r", ToPort: "e", Head: true}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if lay.Edges[0].SidesDropped {
		t.Errorf("the line to an empty field does not keep to it:\n%s", strings.Join(lay.Text(), "\n"))
	}
}

// The cost of ending beside a joint picks a cell along a side, and never
// turns a line to another side: across, every cell on the side of a field
// a row tall is beside one, and lines between records charged for each
// went round their tops rather than straight across.
func TestJointCostKeepsSidesFacing(t *testing.T) {
	rec := func(id string, labels ...string) Item {
		var fs []Field
		for _, l := range labels {
			fs = append(fs, Field{Label: l})
		}
		return Item{Node: &Node{ID: id, Record: fs}}
	}
	d := &Diagram{Direction: Right, Root: Group{Items: []Item{rec("a", "one", "two", "three"), rec("b", "x", "y", "z"), rec("c", "p", "q")}}}
	d.Edges = []Edge{{From: "a", To: "b", Head: true}, {From: "b", To: "c", Head: true}, {From: "a", To: "c", Head: true}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range lay.Edges {
		from, _ := lay.Node(e.Edge.From)
		to, _ := lay.Node(e.Edge.To)
		first, last := e.Points[0], e.Points[len(e.Points)-1]
		if first.X != from.Rect.X+from.Rect.W-1 || last.X != to.Rect.X {
			t.Errorf("%s to %s leaves by %v and enters by %v, not the sides facing:\n%s", e.Edge.From, e.Edge.To, first, last, strings.Join(lay.Text(), "\n"))
		}
	}
}

// No straight line, set apart as the picture draws it, crosses a box or
// leaves a record at a joint; and no edge is drawn along rows and columns
// that could be drawn straight with every straight line between its nodes
// so clear: an
// edge taken out to keep one line clear may be clear itself once others
// are taken out, set apart as they then are, and was once left out for
// good, a line drawn round where a straight one would do. Each edge left
// out is tried back, and some line between its nodes must then meet a
// joint.
func TestStraightLinesTakenOutComeBack(t *testing.T) {
	// 300 random diagrams, as the other random tests lay out, and the
	// three of the first 5,000 that left an edge out for good before.
	seeds := []int64{3427, 4665, 4840}
	for seed := range int64(300) {
		seeds = append(seeds, seed)
	}
	for _, seed := range seeds {
		d := randomDiagram(rand.New(rand.NewSource(seed)))
		d.Straight = true
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		joints := lay.jointsByNode()
		index := map[*Edge]int{}
		for i := range d.Edges {
			index[&d.Edges[i]] = i
		}
		straight := make([]bool, len(d.Edges))
		for _, e := range lay.Edges {
			i, ok := index[e.Edge]
			if !ok {
				t.Fatalf("seed %d: a placed edge is none of the diagram's", seed)
			}
			straight[i] = e.Straight
			// Set apart as the picture draws it, not as the check
			// works it out, so that the two differing fails here.
			if e.Straight && !lay.straightClear(joints, e.Edge, e.Shift) {
				t.Fatalf("seed %d: %s -> %s is straight and crosses a box or meets a joint", seed, e.Edge.From, e.Edge.To)
			}
		}
		for i, e := range d.Edges {
			if straight[i] || !lay.straightens(e) {
				continue
			}
			tried := slices.Clone(straight)
			tried[i] = true
			sh, clear := shifts(d.Edges, tried), true
			for j := range d.Edges {
				if tried[j] && !lay.straightClear(joints, &d.Edges[j], sh[j]) {
					clear = false
					break
				}
			}
			if clear {
				t.Fatalf("seed %d: %s -> %s is drawn round, though straight it and every line between its nodes would be clear", seed, e.From, e.To)
			}
		}
	}
}

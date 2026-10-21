package diagram

import (
	"fmt"
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

// Two edges into one node share a line only where they name the same port
// of it: a line into a field and one into the whole record, merged, end
// beside the field, and the second reads as joining it.
func TestRecordPortsShareOnlyTheirOwn(t *testing.T) {
	a := Edge{From: "x", To: "r", ToPort: "p", Head: true}
	if _, ok := meet(a, Edge{From: "y", To: "r", Head: true}); ok {
		t.Error("an edge into r:p and one into r share a line")
	}
	if _, ok := meet(a, Edge{From: "y", To: "r", ToPort: "q", Head: true}); ok {
		t.Error("edges into r:p and r:q share a line")
	}
	if at, ok := meet(a, Edge{From: "y", To: "r", ToPort: "p", Head: true}); !ok || at != "r" {
		t.Error("two edges into r:p do not share a line")
	}
	if _, ok := meet(Edge{From: "r", FromPort: "p", To: "x", Head: true}, Edge{From: "r", FromPort: "p", To: "y", Head: true}); !ok {
		t.Error("two edges out of r:p do not fork from one stem")
	}
}

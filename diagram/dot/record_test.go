package dot

import (
	"reflect"
	"testing"

	"github.com/wisborg/output/diagram"
)

// A record's label is read as Graphviz reads it: fields divided by "|",
// a field divided the other way between braces, a port before a field's
// words, and a backslash before a character of the grammar writing it.
func TestRecordLabels(t *testing.T) {
	F := func(port, label string) diagram.Field { return diagram.Field{Port: port, Label: label} }
	G := func(fs ...diagram.Field) diagram.Field { return diagram.Field{Fields: fs} }
	for _, c := range []struct {
		label string
		want  []diagram.Field
	}{
		{"<f0> left|<f1> mid\\ dle|<f2> right", []diagram.Field{F("f0", "left"), F("f1", "mid dle"), F("f2", "right")}},
		{"hello\\nworld |{ b |{c|<here> d|e}| f}| g | h", []diagram.Field{
			F("", "hello\nworld"), G(F("", "b"), G(F("", "c"), F("here", "d"), F("", "e")), F("", "f")), F("", "g"), F("", "h"),
		}},
		{`a\|b|\{c\}|\<d\>`, []diagram.Field{F("", "a|b"), F("", "{c}"), F("", "<d>")}},
		{"<p>|", []diagram.Field{F("p", ""), F("", "")}},
		{`\N`, []diagram.Field{F("", "n")}},
		{"{a|b}", []diagram.Field{G(F("", "a"), F("", "b"))}},
	} {
		got, ok := record(c.label, "n", "g")
		if !ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: %+v (%v), want %+v", c.label, got, ok, c.want)
		}
	}
	// What is not a record label is none, and its node a box with its
	// label as written, where Graphviz refuses to draw it.
	for _, bad := range []string{"{a|b", "a}|b", "<p a", "a{b}", "a{b", "a>b", "<p><q> x"} {
		if got, ok := record(bad, "n", "g"); ok {
			t.Errorf("%q read as a record: %+v", bad, got)
		}
	}
}

// shape=record and Mrecord make a record node, its fields from its label
// -- or its name, where it has none -- and node:field names a field to an
// edge, with or without a compass point after it, as tailport and headport
// do. A label that is no record leaves a box with the label as written.
func TestRecordNodesAndPorts(t *testing.T) {
	d := parse(t, `digraph {
		node [shape=record]
		a [label="<x> one|<y> two"]
		b [shape=Mrecord]
		c [label="{broken"]
		a:y -> b
		b -> a:x:n
		a -> b [tailport="x:s"]
		a -> b:s
	}`)
	nodes := map[string]*diagram.Node{}
	for _, it := range d.Root.Items {
		nodes[it.Node.ID] = it.Node
	}
	if got := nodes["a"].Record; len(got) != 2 || got[1].Port != "y" || got[1].Label != "two" {
		t.Errorf("a: %+v", got)
	}
	if b := nodes["b"]; b.Shape != diagram.Rounded || len(b.Record) != 1 || b.Record[0].Label != "b" {
		t.Errorf("b: %+v", b)
	}
	if c := nodes["c"]; c.Record != nil || c.Label != "{broken" {
		t.Errorf("c: record %+v, label %q", c.Record, c.Label)
	}
	for i, want := range []struct {
		from, to string
		side     diagram.Side
	}{{"y", "", 0}, {"", "x", diagram.North}, {"x", "", 0}} {
		e := d.Edges[i]
		if e.FromPort != want.from || e.ToPort != want.to {
			t.Errorf("edge %d: ports %q and %q, want %q and %q", i, e.FromPort, e.ToPort, want.from, want.to)
		}
		if e.ToSide != want.side && i == 1 {
			t.Errorf("edge %d: to side %v, want %v", i, e.ToSide, want.side)
		}
	}
	if d.Edges[2].FromSide != diagram.South {
		t.Errorf("tailport x:s: side %v, want South", d.Edges[2].FromSide)
	}
	// A compass point alone is a side, not a field named "s".
	if e := d.Edges[3]; e.ToPort != "" || e.ToSide != diagram.South {
		t.Errorf("b:s: port %q and side %v, want no port and South", e.ToPort, e.ToSide)
	}
}

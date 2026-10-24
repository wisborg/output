package dot

import (
	"reflect"
	"strings"
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
		// An escaped space is kept at either end, as Graphviz keeps it to
		// pad a field; the others there are not.
		{`<p> \ \ pad\  | q `, []diagram.Field{F("p", "  pad "), F("", "q")}},
		{`x|\ y\ `, []diagram.Field{F("", "x"), F("", " y ")}},
		// A run of spaces between words is one, as Graphviz writes it, a
		// tab or a line break among them; escaped ones are each kept.
		{"name    type|a \t\n b|c\\ \\  d", []diagram.Field{F("", "name type"), F("", "a b"), F("", "c   d")}},
		{"\t a \n|b", []diagram.Field{F("", "a"), F("", "b")}},
		// So are tabs and line breaks about a brace, as a record written
		// over several lines has them.
		{"x|\n\t{y|z}\r\n|r", []diagram.Field{F("", "x"), G(F("", "y"), F("", "z")), F("", "r")}},
		{"{p|q}\t|r", []diagram.Field{G(F("", "p"), F("", "q")), F("", "r")}},
		// A backslash at the end is itself, and the label still a record.
		{`a|b\`, []diagram.Field{F("", "a"), F("", `b\`)}},
		// As deep as subgraphs may nest is still a record.
		{strings.Repeat("{", maxDepth) + "x" + strings.Repeat("}", maxDepth), []diagram.Field{nested(maxDepth, F("", "x"))}},
	} {
		got, ok := record(c.label, "n", "g")
		if !ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%q: %+v (%v), want %+v", c.label, got, ok, c.want)
		}
	}
	// What is not a record label is none, and its node a box with its
	// label as written, where Graphviz refuses to draw it.
	deep := strings.Repeat("{", maxDepth+1) + "x" + strings.Repeat("}", maxDepth+1)
	for _, bad := range []string{"{a|b", "a}|b", "<p a", "a{b}", "a{b", "a>b", "<p><q> x", "<> <p> x", "<p> <> x", "x <y> z", deep} {
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
	// A compass point alone, b:s, is a side, as b has no field named s.
	for i, want := range []struct {
		from, to         string
		fromSide, toSide diagram.Side
	}{{"y", "", 0, 0}, {"", "x", 0, diagram.North}, {"x", "", diagram.South, 0}, {"", "", 0, diagram.South}} {
		e := d.Edges[i]
		if e.FromPort != want.from || e.ToPort != want.to || e.FromSide != want.fromSide || e.ToSide != want.toSide {
			t.Errorf("edge %d, %s to %s: ports %q %q, sides %v %v; want %q %q, %v %v", i, e.From, e.To,
				e.FromPort, e.ToPort, e.FromSide, e.ToSide, want.from, want.to, want.fromSide, want.toSide)
		}
	}
}

// A port of one compass word names the record's field of that name where
// it has one, as Graphviz reads it, whichever is written first, the node
// or the edge; where the record has none it is the side.
func TestRecordFieldNamedLikeACompassPoint(t *testing.T) {
	d := parse(t, `digraph {
		c -> a:n
		a [shape=record, label="<n> north|<s> south"]
		b -> a:s
		a -> c [tailport=e]
		c -> b:n
	}`)
	for i, want := range []struct {
		from, to         string
		fromSide, toSide diagram.Side
	}{{"", "n", 0, 0}, {"", "s", 0, 0}, {"", "", diagram.East, 0}, {"", "", 0, diagram.North}} {
		e := d.Edges[i]
		if e.FromPort != want.from || e.ToPort != want.to || e.FromSide != want.fromSide || e.ToSide != want.toSide {
			t.Errorf("edge %d, %s to %s: ports %q %q, sides %v %v; want %q %q, %v %v", i, e.From, e.To,
				e.FromPort, e.ToPort, e.FromSide, e.ToSide, want.from, want.to, want.fromSide, want.toSide)
		}
	}
}

// nested is f inside depth fields, each of one field.
func nested(depth int, f diagram.Field) diagram.Field {
	for range depth {
		f = diagram.Field{Fields: []diagram.Field{f}}
	}
	return f
}

// An HTML label on a record is the node's label, as Graphviz draws it, not
// its fields: its "|" is a character of its words, and a "<" written in it
// as &lt; no port. A label given over an HTML default reads as fields.
func TestRecordHTMLLabel(t *testing.T) {
	d := parse(t, `digraph {
		node [shape=record]
		a [label=<<b>left</b>|right>]
		b [label=<x &lt;y&gt; z>]
		subgraph { node [label=<p|q>]; c [label="r|s"]; e }
	}`)
	nodes := map[string]*diagram.Node{}
	for _, it := range d.Root.Items {
		if it.Node != nil {
			nodes[it.Node.ID] = it.Node
		}
	}
	for id, want := range map[string]string{"a": "left|right", "b": "x <y> z", "e": "p|q"} {
		if n := nodes[id]; n == nil || n.Record != nil || n.Label != want {
			t.Errorf("%s: %+v, want a box labelled %q", id, n, want)
		}
	}
	if c := nodes["c"]; c == nil || len(c.Record) != 2 {
		t.Errorf("c, labelled with a quoted string over an HTML default: %+v, want two fields", c)
	}
}

// A record's name is put into its label once its fields are read, as
// Graphviz does, so a "|" in the name divides nothing: Graphviz draws "x|y"
// as one field and warns that no port y is there.
func TestRecordNameIsNoMarkup(t *testing.T) {
	d := parse(t, `digraph { node [shape=record]; "x|y"; "a|b" [label="<p> \N|z"] }`)
	want := map[string][]diagram.Field{
		"x|y": {{Label: "x|y"}},
		"a|b": {{Port: "p", Label: "a|b"}, {Label: "z"}},
	}
	for _, it := range d.Root.Items {
		if n := it.Node; n != nil && !reflect.DeepEqual(n.Record, want[n.ID]) {
			t.Errorf("%s: fields %+v, want %+v", n.ID, n.Record, want[n.ID])
		}
	}
}

// A record in the legend is named by its fields' words: its Label is their
// markup, which no drawing shows, port names and dividers among it.
func TestRecordInLegend(t *testing.T) {
	d := parse(t, `digraph {
		subgraph cluster_k { style=legend; k [shape=record, label="<p> key|{value|\{x\}}"]; n [shape=record] }
		a
	}`)
	if d.Legend == nil || len(d.Legend.Entries) != 2 {
		t.Fatalf("legend %+v, want two entries", d.Legend)
	}
	for i, want := range []string{"key value {x}", "n"} {
		if got := d.Legend.Entries[i].Label; got != want {
			t.Errorf("entry %d: %q, want %q", i, got, want)
		}
	}
}

// A record label that is none is said to Options.Warn, with the line that
// set it, as Graphviz refuses it: drawn as written, it reads as a mistake
// in the drawing rather than in the file, and its ports quietly name
// nothing. An HTML label is the node's, and a record read is no mistake.
func TestRecordLabelNotOneIsSaid(t *testing.T) {
	var said []*Error
	_, err := ParseWith(`digraph {
		node [shape=record]
		a [label="<k> key|{broken"]
		b [label="<k> key|{fine}"]; c [label=<x|{y>]
		d
		e
		[label="x <y> z"]
	}`, Options{Warn: func(e *Error) { said = append(said, e) }})
	if err != nil {
		t.Fatal(err)
	}
	if len(said) != 2 || said[0].Line != 3 || !strings.Contains(said[0].Msg, `node "a"`) || said[1].Line != 7 || !strings.Contains(said[1].Msg, `node "e"`) {
		t.Errorf("said %v, want node a's label at line 3 and e's at 7", said)
	}
}

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
		a:zz -> b
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
	// A compass point alone, b:s, is a side, as b has no field named s;
	// any other word is a port, a:zz, whether or not a field has it --
	// it is diagram's to say none does.
	for i, want := range []struct {
		from, to         string
		fromSide, toSide diagram.Side
	}{{"y", "", 0, 0}, {"", "x", 0, diagram.North}, {"x", "", diagram.South, 0}, {"", "", 0, diagram.South}, {"zz", "", 0, 0}} {
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

// A port whose last part is no compass point -- f:sth for f:s, or a field
// named with a colon, split at it -- names no side, and is said to
// Options.Warn, with the line it was written on, where an edge takes it,
// as Graphviz warns of it: written on a node, as an edge's tailport or
// headport, on the statement's line or another, or as an edge default.
// The compass point is all after the first colon, as Graphviz has it, so
// f:g:s's is g:s. Not of a default no edge takes, nor of a node
// statement's; a port written on a node is said once for its node and
// line, naming it, and an attribute once a line and port, however many
// edges take it. A port of one word is a field's name, an empty compass
// point is none written, and a compass point, c, _ and ne among them, is
// none of these. The warnings come in the order of their lines, a
// record's label among them.
func TestPortCompassPointNoneIsSaid(t *testing.T) {
	var said []*Error
	d, err := ParseWith(`strict digraph { node [shape=record, label="<f> f|<g> g"]
		a [shape=record, label="<f> x|<g> y"]
		b [shape=record, label="{broken"]
		a:f:sth -> b
		a -> c [tailport="g:1",
			headport="f:nee"]
		edge [headport="f:never"]
		c:f -> b:sth -> c:f:_ -> a:f:c [tailport="f:", headport=":s"]
		edge [tailport="f:left"]
		{x y} -> a:f:up
		x:f:q [color=red]
		a -> c
		z -> a:g:ne
		w [shape=record, label="<a"]
		a:f:x -> v:f:x; d -> a [headport="f:g:s"]
		"":f:zz -> u [headport="f:zz"]
	}`, Options{Warn: func(e *Error) { said = append(said, e) }})
	if err != nil {
		t.Fatal(err)
	}
	// Each warning is held whole: what a port's fault is, and what that
	// does to its line.
	const none = " is no compass point (n, ne, e, se, s, sw, w, nw, c or _) and names no side"
	const broken = ": it is drawn as written, and no port names a field of it"
	want := []struct {
		line int
		msg  string
	}{
		{3, `node "b": the label "{broken" is no record's` + broken},
		{4, `the port "f:sth" of "a": "sth"` + none},
		// a -> c restated keeps the ports its first statement wrote.
		{5, `the tailport "g:1": on "a", "1"` + none},
		{6, `the headport "f:nee": on "c", "nee"` + none},
		// An empty field names nothing, as in Graphviz, and its compass
		// point is ignored.
		{8, `the headport ":s": on "c" and "a", an empty field names nothing, and "s" is ignored: the line ends anywhere on their boxes`},
		// Taken by x, y and z, and said once.
		{9, `the tailport "f:left": on "x", "y", "z" and 1 more, "left"` + none},
		// Taken by x and y, of one node, and said once.
		{10, `the port "f:up" of "a": "up"` + none},
		// Its node read before the edges' ports, said after them.
		{14, `node "w": the label "<a" is no record's` + broken},
		// Two nodes' ports, each said.
		{15, `the port "f:x" of "a": "x"` + none},
		{15, `the port "f:x" of "v": "x"` + none},
		{15, `the headport "f:g:s": on "a", "g:s"` + none},
		// A node named "" is a node, and its port no attribute's.
		{16, `the port "f:zz" of "": "zz"` + none},
		{16, `the headport "f:zz": on "u", "zz"` + none},
	}
	if len(said) != len(want) {
		t.Fatalf("said %v, want %d warnings", said, len(want))
	}
	for i, w := range want {
		if said[i].Line != w.line || said[i].Msg != w.msg {
			t.Errorf("warning %d: %v, want line %d: %s", i, said[i], w.line, w.msg)
		}
	}
	// Read as the field, and no side.
	if e := d.Edges[0]; e.FromPort != "f" || e.FromSide != 0 {
		t.Errorf("a:f:sth: port %q, side %v; want f and no side", e.FromPort, e.FromSide)
	}
	if e := d.Edges[len(d.Edges)-1]; e.ToPort != "f" || e.ToSide != 0 {
		t.Errorf("headport f:g:s: port %q, side %v; want f and no side", e.ToPort, e.ToSide)
	}
}

// A port naming a field of a node that is no record -- b:sth meant as b:s
// on a box -- or a field its record has none of names nothing, and is said
// to Options.Warn with its line, as Graphviz warns of it, with a bad
// compass point after it in the same warning, not a second. Not of a node
// with an HTML label, whose ports Graphviz reads and this does not, nor of
// a record whose label is none, said already, nor of a compass point
// alone, b:n, which is a side, nor of an invisible edge, which no line
// draws. A first part that is no field but a compass point, b:s:w, is the
// side and the rest is ignored, as Graphviz reads it, and that is said,
// as Graphviz does not. A tailport, headport or edge default is said once a line, every
// fault of it over every edge that takes it in one warning, naming the
// nodes it names nothing on -- three, and how many more -- tail or head.
func TestPortOfNoRecordIsSaid(t *testing.T) {
	var said []*Error
	_, err := ParseWith(`digraph {
		b; h [label=<<b>h</b>>]; r [shape=record, label="{x"]
		a -> b:sth
		a -> b:f:s
		a -> b:f:sth
		a -> h:p; a -> r:p; a -> b:n
		edge [headport=p]
		c -> d; e -> f; g -> d
		R [shape=record, label="<x> x"]; a -> R:nope; a -> b:qq [style=invis]
		a -> {R b} [headport="x:sth"]
		edge [tailport=q, headport=""]; {m1 m2 m3 m4 m5} -> R
		S [shape=record, label="<x> x"]; a -> {R S} [tailport="", headport=zz]
		edge [tailport=""]; a -> b:s:w; a -> R:n:sth; Q [shape=record, label="{q"]; a -> Q:nope:s
		a -> Q:s:w; a -> h:s:w; a -> h:p:sth
		edge [tailport=":s"]; k1 -> k2; k3 -> k4
		k5:"" -> k6; k7 -> k8 [tailport=qq, headport=qq]
	}`, Options{Warn: func(e *Error) { said = append(said, e) }})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		`line 2: node "r": the label "{x" is no record's: it is drawn as written, and no port names a field of it`,
		`line 3: the port "sth" of "b": "b" is no record, whose fields a port names: the line ends anywhere on its box`,
		`line 4: the port "f:s" of "b": "b" is no record, whose fields a port names: the line ends anywhere on its box`,
		// Its compass point after a field that is not there is ignored, as
		// Graphviz ignores it, and so not said.
		`line 5: the port "f:sth" of "b": "b" is no record, whose fields a port names: the line ends anywhere on its box`,
		`line 7: the headport "p": "d" and "f" are no records, whose fields a port names: the line ends anywhere on their boxes`,
		`line 9: the port "nope" of "R": "R" has no field "nope": the line ends anywhere on its box`,
		// R's field is there, and its compass point is none; b's is not.
		`line 10: the headport "x:sth": "b" is no record, whose fields a port names: the line ends anywhere on its box; and on "R", "sth" is no compass point (n, ne, e, se, s, sw, w, nw, c or _) and names no side`,
		`line 11: the tailport "q": "m1", "m2", "m3" and 2 more are no records, whose fields a port names: the line ends anywhere on their boxes`,
		`line 12: the headport "zz": "R" and "S" have no field "zz": the line ends anywhere on their boxes`,
		// A first part that is no field but a compass point is the side,
		// and the rest ignored, as Graphviz reads it; said, as it is not.
		// A broken record's port is said in its label's warning, and an
		// HTML label's not at all, its ports not being read: line 14 says
		// nothing.
		`line 13: node "Q": the label "{q" is no record's: it is drawn as written, and no port names a field of it`,
		`line 13: the port "s:w" of "b": "b" has no field "s", so "s" is the side and "w" is ignored`,
		`line 13: the port "n:sth" of "R": "R" has no field "n", so "n" is the side and "sth" is ignored`,
		// An empty field in a default is said once, naming each node.
		`line 15: the tailport ":s": on "k1" and "k3", an empty field names nothing, and "s" is ignored: the line ends anywhere on their boxes`,
		// k5:"" clears the default, as in Graphviz, and is said nothing
		// of; a tailport and a headport alike are said apart.
		`line 16: the tailport "qq": "k7" is no record, whose fields a port names: the line ends anywhere on its box`,
		`line 16: the headport "qq": "k8" is no record, whose fields a port names: the line ends anywhere on its box`,
	}
	if len(said) != len(want) {
		t.Fatalf("said %v, want %d warnings", said, len(want))
	}
	for i, w := range want {
		if got := said[i].Error(); got != w {
			t.Errorf("warning %d: %q, want %q", i, got, w)
		}
	}
}

// A port of a third part, a:f:x:n, is an error, as in DOT's grammar and
// Graphviz.
func TestPortOfThreePartsIsAnError(t *testing.T) {
	_, err := Parse("digraph {\n c -> a:f:x:n\n}")
	if err == nil || !strings.Contains(err.Error(), "line 2: a port is a field and a compass point at most, :\"f\":\"x\"; found a third part") {
		t.Errorf("err %v, want the third part refused at line 2", err)
	}
	if _, err := Parse("digraph {\n c -> a:f:x; d -> a [headport=\"f:x:n\"]\n}"); err != nil {
		t.Errorf("err %v: two parts, and an attribute of three, read", err)
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
	// Of a file with an error after, nothing is drawn, and nothing said
	// to be drawn as written.
	said = nil
	if _, err := ParseWith("digraph {\n a [shape=record, label=\"{x\"]\n b [color=\"nocolour\"]\n}", Options{Warn: func(e *Error) { said = append(said, e) }}); err == nil {
		t.Fatal("a bad colour read")
	}
	if len(said) != 0 {
		t.Errorf("said %v of a file that is an error", said)
	}
}

package dot

import (
	"errors"
	"image/color"
	"reflect"
	"strings"
	"testing"

	"github.com/wisborg/output/diagram"
)

func parse(t *testing.T, src string) *diagram.Diagram {
	t.Helper()
	d, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// items is a group's items as names: a node's ID, a group's ID with what it
// holds in brackets.
func items(g *diagram.Group) []string {
	var out []string
	for _, it := range g.Items {
		if it.Node != nil {
			out = append(out, it.Node.ID)
		} else {
			out = append(out, it.Group.ID+"["+strings.Join(items(it.Group), " ")+"]")
		}
	}
	return out
}

func edges(d *diagram.Diagram) []string {
	var out []string
	for _, e := range d.Edges {
		out = append(out, e.From+">"+e.To)
	}
	return out
}

// A cluster is a group, holding the nodes first mentioned in it -- and a
// node mentioned outside every cluster before it is mentioned in one, at
// the end of an edge, moves into it. Other subgraphs group nothing.
func TestClusters(t *testing.T) {
	d := parse(t, `digraph {
		a -> b
		subgraph cluster_one { b; c; subgraph cluster_inner { d } }
		subgraph plain { e }
		c -> a
	}`)
	if got, want := items(&d.Root), []string{"a", "cluster_one[b c cluster_inner[d]]", "e"}; !reflect.DeepEqual(got, want) {
		t.Errorf("groups %v, want %v", got, want)
	}
}

// An edge statement joins each end to the next, every node of a subgraph
// end to every node of the next, and a strict graph keeps one edge of each
// pair.
func TestEdges(t *testing.T) {
	d := parse(t, `digraph { a -> b -> c; a -> {d e}; {f g} -> h }`)
	want := []string{"a>b", "b>c", "a>d", "a>e", "f>h", "g>h"}
	if got := edges(d); !reflect.DeepEqual(got, want) {
		t.Errorf("edges %v, want %v", got, want)
	}
	d = parse(t, `strict graph { a -- b; b -- a; a -- b [label=x] }`)
	if got := edges(d); !reflect.DeepEqual(got, []string{"a>b"}) {
		t.Errorf("strict: edges %v, want one", got)
	}
	if d.Edges[0].Head {
		t.Error("an undirected graph's edge has an arrowhead")
	}
}

// Defaults apply to what is made after them, a subgraph inheriting and
// changing them for itself alone; a statement's own attributes win.
func TestDefaults(t *testing.T) {
	d := parse(t, `digraph {
		node [shape=cylinder]
		a
		subgraph cluster_x { node [shape=ellipse]; b; c [shape=box] }
		d
		edge [style=dashed]
		a -> d
	}`)
	shapes := map[string]diagram.Shape{}
	var walk func(*diagram.Group)
	walk = func(g *diagram.Group) {
		for _, it := range g.Items {
			if it.Node != nil {
				shapes[it.Node.ID] = it.Node.Shape
			} else {
				walk(it.Group)
			}
		}
	}
	walk(&d.Root)
	want := map[string]diagram.Shape{"a": diagram.Cylinder, "b": diagram.Ellipse, "c": diagram.Box, "d": diagram.Cylinder}
	if !reflect.DeepEqual(shapes, want) {
		t.Errorf("shapes %v, want %v", shapes, want)
	}
	if d.Edges[0].Line != diagram.Dashed {
		t.Error("the edge default style=dashed not applied")
	}
}

// This package's own attributes: a grid on the graph and on a cluster, a
// node pinned to a cell, a cluster drawn as a lane.
func TestGridAttributes(t *testing.T) {
	d := parse(t, `digraph {
		rows = 2
		subgraph cluster_east { label="us-east-1"; style=lane; columns=3; a; b [row=1, col=3] }
	}`)
	g := d.Root.Items[0].Group
	if d.Root.Rows != 2 || g.Columns != 3 || g.Style != diagram.Lane || g.Label != "us-east-1" {
		t.Errorf("root rows %d; cluster columns %d, style %v, label %q", d.Root.Rows, g.Columns, g.Style, g.Label)
	}
	if b := g.Items[1].Node; b.Row != 1 || b.Col != 3 {
		t.Errorf("b pinned to row %d, column %d", b.Row, b.Col)
	}
}

// Labels: \n ends a line, \N is the node's name, quoted strings join with
// +, an HTML label is its words; the graph's label is the title.
func TestLabels(t *testing.T) {
	d := parse(t, `digraph g {
		label = "Title"
		a [label="first\nsecond"]
		b [label="\N" + " node"]
		c [label=<<b>bold</b><br/>and &amp; more>]
		a -> b [label="x\ly\l"]
	}`)
	got := []string{d.Title}
	for _, it := range d.Root.Items {
		got = append(got, it.Node.Label)
	}
	got = append(got, d.Edges[0].Label)
	want := []string{"Title", "first\nsecond", "b node", "bold\nand & more", "x\ny"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("labels %q, want %q", got, want)
	}
}

// Graphviz's colours and fills: a fillcolor fills only with style=filled,
// filled alone is light grey, a colour list is its first colour; dir and
// arrowhead set the arrowheads.
func TestStyleAndColour(t *testing.T) {
	d := parse(t, `digraph {
		a [fillcolor=red]
		b [style=filled, fillcolor="#00ff00"]
		c [style="filled,dashed"]
		d [color="blue:red", style=rounded]
		a -> b [dir=both]
		b -> c [arrowhead=none]
		c -> d [dir=back, color="#123"]
	}`)
	n := func(i int) *diagram.Node { return d.Root.Items[i].Node }
	if n(0).Fill != nil {
		t.Error("a fillcolor without style=filled filled the node")
	}
	if n(1).Fill != (color.RGBA{0, 0xFF, 0, 0xFF}) {
		t.Errorf("b's fill %v", n(1).Fill)
	}
	if n(2).Fill != (color.RGBA{0xD3, 0xD3, 0xD3, 0xFF}) || n(2).Line != diagram.Dashed {
		t.Errorf("c's fill %v, line %v", n(2).Fill, n(2).Line)
	}
	if n(3).Colour != (color.RGBA{0, 0, 0xFF, 0xFF}) || n(3).Shape != diagram.Rounded {
		t.Errorf("d's colour %v, shape %v", n(3).Colour, n(3).Shape)
	}
	heads := [][2]bool{}
	for _, e := range d.Edges {
		heads = append(heads, [2]bool{e.Head, e.Tail})
	}
	if want := [][2]bool{{true, true}, {false, false}, {false, true}}; !reflect.DeepEqual(heads, want) {
		t.Errorf("head and tail %v, want %v", heads, want)
	}
	if d.Edges[2].Colour != (color.RGBA{0x11, 0x22, 0x33, 0xFF}) {
		t.Errorf("#123 read as %v", d.Edges[2].Colour)
	}
}

// Comments, ports, keywords in any case, and a quoted keyword as a name.
func TestSyntax(t *testing.T) {
	d := parse(t, `/* a comment */
# a preprocessor line
STRICT DiGraph {
	// a line comment
	"node" -> b:p:n
	b -> c:e;;
	x = y
}`)
	if got := edges(d); !reflect.DeepEqual(got, []string{"node>b", "b>c"}) {
		t.Errorf("edges %v", got)
	}
}

// Mistakes are refused with the line they are on.
func TestErrors(t *testing.T) {
	for _, c := range []struct {
		src  string
		line int
		want string
	}{
		{"digraph {\n a -- b\n}", 2, "a digraph's edges are ->"},
		{"graph {\n a -> b\n}", 2, "a graph's edges are --"},
		{"digraph {\n a [label=\"open\n}", 2, "never closed"},
		{"digraph {\n a [color=chartreuse4x]\n}", 2, `no colour "chartreuse4x"`},
		{"digraph {\n subgraph cluster_a {\n columns=0 }\n}", 2, "columns"},
		{"digraph {\n a ->\n}", 3, "expected a node"},
		{"flowchart {}", 1, "begins with graph or digraph"},
		{"digraph {\n a\n", 3, "a } is missing"},
	} {
		_, err := Parse(c.src)
		var pe *Error
		if !errors.As(err, &pe) || pe.Line != c.line || !strings.Contains(pe.Msg, c.want) {
			t.Errorf("%q: %v, want line %d saying %q", c.src, err, c.line, c.want)
		}
	}
}

// A cluster with style=legend is the legend, not a part of the diagram:
// each node in it an entry, a swatch filled as the node would be, or a
// stroke for a plain node, in its colour and style.
func TestLegend(t *testing.T) {
	d := parse(t, `digraph {
		node [style=filled]
		a -> b
		subgraph cluster_key {
			label = "Key"; style = legend
			ok [label = "healthy", fillcolor = green]
			async [label = "async", shape = plain, style = dashed, color = red]
		}
	}`)
	if got := items(&d.Root); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("the diagram holds %v, want a and b alone", got)
	}
	lg := d.Legend
	if lg == nil || lg.Label != "Key" || len(lg.Entries) != 2 {
		t.Fatalf("legend %+v, want Key with two entries", lg)
	}
	ok, async := lg.Entries[0], lg.Entries[1]
	if ok.Label != "healthy" || ok.Sample != diagram.Swatch || ok.Fill != (color.RGBA{0, 0xFF, 0, 0xFF}) {
		t.Errorf("first entry %+v, want healthy, a green swatch", ok)
	}
	if async.Label != "async" || async.Sample != diagram.Stroke || async.Line != diagram.Dashed || async.Colour != (color.RGBA{0xFF, 0, 0, 0xFF}) || async.Fill != nil {
		t.Errorf("second entry %+v, want async, a dashed red stroke", async)
	}
	if _, err := d.Layout(); err != nil {
		t.Error(err)
	}
}

func TestLegendErrors(t *testing.T) {
	for _, c := range []struct {
		src  string
		line int
		want string
	}{
		{"digraph {\n subgraph cluster_k { style=legend; k }\n a -> k\n}", 2, `"k" is an entry of it`},
		{"digraph {\n subgraph cluster_k { style=legend; k }\n subgraph cluster_l { style=legend; l }\n}", 3, "one legend"},
		{"digraph {\n subgraph cluster_k { style=legend; subgraph cluster_x { x } }\n}", 2, "not the cluster"},
	} {
		_, err := Parse(c.src)
		var pe *Error
		if !errors.As(err, &pe) || pe.Line != c.line || !strings.Contains(pe.Msg, c.want) {
			t.Errorf("%q: %v, want line %d saying %q", c.src, err, c.line, c.want)
		}
	}
}

// splines asks for straight edges for every value Graphviz draws other
// than along rows and columns; ortho, and no splines at all, do not.
func TestSplines(t *testing.T) {
	for v, want := range map[string]bool{"": false, "ortho": false, "line": true, "polyline": true, "false": true, "true": true, "curved": true} {
		src := "digraph { a -> b }"
		if v != "" {
			src = "digraph { splines = " + v + "; a -> b }"
		}
		if got := parse(t, src).Straight; got != want {
			t.Errorf("splines=%q: straight %v, want %v", v, got, want)
		}
	}
}

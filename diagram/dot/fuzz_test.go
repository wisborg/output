package dot

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wisborg/output/diagram"
	"github.com/wisborg/output/diagram/internal/layoutcheck"
	"github.com/wisborg/output/diagram/svg"
	"github.com/wisborg/output/internal/fuzzcorpus"
)

// FuzzParse reads DOT as a stranger may write it and draws whatever reads:
// laid out with its edges as the file has them and straight, then as text,
// as braille, in colour and as SVG. A DOT file is often someone else's, so
// none of it may panic, and what is drawn must keep what the package
// promises of a label: the text drawing holds no control character and is
// UTF-8, the coloured one no control character but its own escapes, and
// the SVG is well-formed XML. The layout keeps what it promises of its
// lines, as layoutcheck has them: along rows and columns, through no box,
// ending on its boxes' outlines, off their corners and off where a
// record's divider meets one, and saying truly where a port or a side
// was not kept.
//
// The limits are lower than a command's, so that an input is milliseconds
// and the fuzzer reaches far: the defaults are a caller's to raise, and an
// input past them is refused as any is. The PNG is left out, as the SVG
// draws the same layout and a picture is a thousand times slower.
//
// go test runs the seeds and testdata/fuzz alone; make fuzz searches.
func FuzzParse(f *testing.F) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.dot"))
	if err != nil {
		f.Fatal(err)
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(src))
	}
	for _, s := range []string{
		"digraph { a -> b -> c; a -> c [dir=back, label=\"x\"] }",
		"strict graph { a:n -- b:s; b -- a [tailport=e] }",
		"digraph { rankdir=LR; subgraph cluster_x { label=\"c\"; columns=2; a; b; c } d -> a:w }",
		"digraph { splines=line; a -> b; b -> c; a -> c; c -> a [label=\"back\"] }",
		"digraph { a [shape=cylinder, fillcolor=\"#ffd76e\", style=filled]; a -> a }",
		"digraph { \"日本語\" -> \"👨‍👩‍👧\" [label=\"\x1b]0;t\x07\"]; c [label=\"a\tb\u202ec\"] }",
		"digraph { a [label=\"not UTF-8: \xff\"] }",
		"digraph { node [shape=record]; s [label=\"<a> x|{<b> y|{c|<d> e}}|\\{z\\}\"]; t [shape=Mrecord, label=\"<p> p|q\"]; s:d -> t:p; s:a:n -> t; t -> s:b }",
		"digraph { rankdir=LR; node [shape=record]; r [label=\"{<k> key|value}|<x> x\"]; r:k -> r:x; q [label=\"{broken\"] }",
		"digraph { node [shape=record]; a [label=<<b>l</b>|r>]; b [label=\"<n> n|x <y> z|<s> \\ s\\ \"]; c -> b:n; b:s -> a:p; b:\"a:b\" -> c }",
		"digraph { a [label=\"<b>&amp; \\\"q\\\" \x01\"] -> b [label=\"]]> <!--\"] }",
		// Hostile text inside a record's fields, which reach the text
		// drawing by their own path: an escape, a bidi override, a C1 CSI,
		// a zero-width space and SVG's markup.
		"digraph { node [shape=record]; a [label=\"<p> \x1b]0;t\x07|{\u202ex|\u009b2J\u200b}|<q> ]]\\> \\<!--\"]; b -> a:p; a:q -> b }",
		"digraph { legend [label=\"k\"]; {a b} -> {c d} [style=dashed] }",
	} {
		f.Add(s)
	}
	f.Fuzz(checkDOT)
}

// TestFuzzCorpus runs checkDOT on every input a search kept, in
// testdata/corpus: make fuzz-replay.
func TestFuzzCorpus(t *testing.T) {
	for name, src := range fuzzcorpus.Strings(t, filepath.Join("testdata", "corpus", "FuzzParse")) {
		t.Run(name, func(t *testing.T) { checkDOT(t, src) })
	}
}

// checkDOT is FuzzParse's check of one input.
func checkDOT(t *testing.T, src string) {
	if len(src) > 4096 {
		return
	}
	d, err := ParseWith(src, Options{MaxEdges: 40})
	if err != nil {
		return
	}
	for _, straight := range []bool{d.Straight, true} {
		d.Straight = straight
		lay, err := d.LayoutWith(diagram.LayoutOptions{MaxCells: 40000})
		if err != nil {
			return
		}
		// Within MaxCells a search cannot run out of states, so no edge is
		// drawn along the plain path for want of them. One drawn plain for
		// want of any path the rules allow is said here as broken, through
		// a box or turning straight out from a joint, as an edge drawn
		// plain in diagram's own random layouts fails them: a rule that
		// leaves an edge no path is one to look at again, and an input
		// that shows it is worth keeping.
		if bad := layoutcheck.Check(checkView(lay), layoutcheck.Options{}); len(bad) > 0 {
			t.Fatalf("the layout breaks its promises (straight %v):\n%s\n%s", straight, strings.Join(bad, "\n"), strings.Join(lay.Text(), "\n"))
		}
		for _, o := range []diagram.TextOptions{{}, {Braille: diagram.BrailleThin}, {Braille: diagram.BrailleThick}} {
			for _, l := range lay.Text(o) {
				if !utf8.ValidString(l) || strings.ContainsFunc(l, control) {
					t.Fatalf("a text line not UTF-8 or holding a control character: %q", l)
				}
			}
		}
		for _, l := range lay.ColourText(diagram.Dark) {
			if !utf8.ValidString(l) || strings.ContainsFunc(l, func(r rune) bool { return r != '\x1b' && control(r) }) {
				t.Fatalf("a coloured line not UTF-8 or holding a control character: %q", l)
			}
		}
		var buf bytes.Buffer
		if err := svg.Write(&buf, lay, svg.Options{}); err != nil {
			continue // a size no viewer draws is refused, not written
		}
		dec := xml.NewDecoder(&buf)
		for {
			if _, err := dec.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Fatalf("an SVG that is not well-formed: %v", err)
			}
		}
	}
}

// control is a C0 or C1 control character, or DEL.
func control(r rune) bool { return r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0 }

// checkView is lay as layoutcheck reads it. A port's field is found as
// the layout finds it, by the field Node.Field names.
func checkView(lay *diagram.Layout) layoutcheck.Layout {
	var v layoutcheck.Layout
	pt := func(p diagram.Point) layoutcheck.Point { return layoutcheck.Point{X: p.X, Y: p.Y} }
	rc := func(r diagram.Rect) layoutcheck.Rect { return layoutcheck.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H} }
	for _, n := range lay.Nodes {
		b := layoutcheck.Box{ID: n.Node.ID, Rect: rc(n.Rect), Plain: n.Node.Outline() == diagram.Plain, Invisible: n.Node.Invisible}
		for _, d := range n.Dividers {
			b.Dividers = append(b.Dividers, [2]layoutcheck.Point{pt(d[0]), pt(d[1])})
		}
		v.Boxes = append(v.Boxes, b)
	}
	field := func(id, port string) layoutcheck.Rect {
		n, ok := lay.Node(id)
		if !ok {
			return layoutcheck.Rect{}
		}
		want := n.Node.Field(port)
		for _, f := range n.Fields {
			if want != nil && f.Field == want {
				return rc(f.Rect)
			}
		}
		return layoutcheck.Rect{}
	}
	for _, e := range lay.Edges {
		ce := layoutcheck.Edge{From: e.Edge.From, To: e.Edge.To, FromSide: layoutcheck.Side(e.Edge.FromSide), ToSide: layoutcheck.Side(e.Edge.ToSide),
			FromField: field(e.Edge.From, e.Edge.FromPort), ToField: field(e.Edge.To, e.Edge.ToPort), SidesDropped: e.SidesDropped}
		for _, p := range e.Points {
			ce.Points = append(ce.Points, pt(p))
		}
		v.Edges = append(v.Edges, ce)
	}
	return v
}

package diagram

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wisborg/output/diagram/internal/layoutcheck"
	"github.com/wisborg/output/internal/fuzzcorpus"
)

// checkView is lay as layoutcheck reads it.
func checkView(lay *Layout) layoutcheck.Layout {
	var v layoutcheck.Layout
	pt := func(p Point) layoutcheck.Point { return layoutcheck.Point{X: p.X, Y: p.Y} }
	rc := func(r Rect) layoutcheck.Rect { return layoutcheck.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H} }
	for _, n := range lay.Nodes {
		b := layoutcheck.Box{ID: n.Node.ID, Rect: rc(n.Rect), Plain: n.Node.Outline() == Plain, Invisible: n.Node.Invisible}
		for _, d := range n.Dividers {
			b.Dividers = append(b.Dividers, [2]layoutcheck.Point{pt(d[0]), pt(d[1])})
		}
		v.Boxes = append(v.Boxes, b)
	}
	for _, e := range lay.Edges {
		ce := layoutcheck.Edge{From: e.Edge.From, To: e.Edge.To, FromSide: layoutcheck.Side(e.Edge.FromSide), ToSide: layoutcheck.Side(e.Edge.ToSide), SidesDropped: e.SidesDropped}
		for _, p := range e.Points {
			ce.Points = append(ce.Points, pt(p))
		}
		if f, ok := lay.nodeAt[e.Edge.From].field(e.Edge.FromPort); ok {
			ce.FromField = rc(f)
		}
		if f, ok := lay.nodeAt[e.Edge.To].field(e.Edge.ToPort); ok {
			ce.ToField = rc(f)
		}
		v.Edges = append(v.Edges, ce)
	}
	return v
}

// checkLayout fails t for every promise lay breaks, as layoutcheck has
// them, with the drawing to see it by.
func checkLayout(t *testing.T, lay *Layout, o layoutcheck.Options) {
	t.Helper()
	bad := layoutcheck.Check(checkView(lay), o)
	for _, b := range bad {
		t.Error(b)
	}
	if len(bad) > 0 {
		t.Logf("\n%s", strings.Join(lay.Text(), "\n"))
	}
}

// randomDiagram is a diagram made at random from rnd, of the kinds a
// line's ends are hardest on: records of one to four fields, some divided
// further, some empty, some of several lines, half with ports; boxes,
// plain text and the odd invisible node; edges between them by ports, by
// sides, both and neither, loops among them, several between one pair;
// laid out down, across, up or to the left, in layers or a grid, straight
// or not.
func randomDiagram(rnd *rand.Rand) *Diagram {
	d := &Diagram{Direction: Direction(rnd.Intn(4)), Straight: rnd.Intn(3) == 0}
	if rnd.Intn(4) == 0 {
		d.Root.Columns = 2 + rnd.Intn(3)
	}
	labels := []string{"", "a", "ab", "abcd", "x\ny", "wider label"}
	n := 3 + rnd.Intn(5)
	ports := make([][]string, n)
	for i := range n {
		id := fmt.Sprint("n", i)
		node := &Node{ID: id}
		switch rnd.Intn(6) {
		case 0, 1, 2:
			var leaf func(depth int) Field
			leaf = func(depth int) Field {
				if depth < 2 && rnd.Intn(4) == 0 {
					return Field{Fields: []Field{leaf(depth + 1), leaf(depth + 1)}}
				}
				f := Field{Label: labels[rnd.Intn(len(labels))]}
				if rnd.Intn(2) == 0 {
					f.Port = fmt.Sprint("p", len(ports[i]))
					ports[i] = append(ports[i], f.Port)
				}
				return f
			}
			for range 1 + rnd.Intn(4) {
				node.Record = append(node.Record, leaf(0))
			}
			if rnd.Intn(4) == 0 {
				node.Shape = Rounded
			}
		case 3:
			node.Shape = Plain
			node.Label = []string{"a", "ab", "plain text"}[rnd.Intn(3)]
		}
		node.Invisible = rnd.Intn(20) == 0
		d.Root.Items = append(d.Root.Items, Item{Node: node})
	}
	port := func(i int) string {
		switch {
		case len(ports[i]) > 0 && rnd.Intn(2) == 0:
			return ports[i][rnd.Intn(len(ports[i]))]
		case rnd.Intn(10) == 0:
			return "nosuch"
		}
		return ""
	}
	side := func() Side {
		if rnd.Intn(4) == 0 {
			return Side(1 + rnd.Intn(15))
		}
		return 0
	}
	for range n + rnd.Intn(5) {
		a, b := rnd.Intn(n), rnd.Intn(n)
		if a == b && rnd.Intn(3) != 0 {
			continue
		}
		d.Edges = append(d.Edges, Edge{
			From: fmt.Sprint("n", a), To: fmt.Sprint("n", b),
			FromPort: port(a), ToPort: port(b), FromSide: side(), ToSide: side(),
			Head: true, Tail: rnd.Intn(3) == 0, Invisible: rnd.Intn(20) == 0,
		})
	}
	return d
}

// On diagrams made at random from fixed seeds, of records with ports and
// sides asked for in every direction, the layout keeps every promise
// layoutcheck holds; is the same laid out twice; in a grid, places its
// boxes where it would without the edges; and in braille, ends no line
// beside where a divider meets an outline. The promises, not one right
// answer: a change to the router may move a line and keep them all.
func TestLayoutKeepsItsPromisesAtRandom(t *testing.T) {
	seeds := int64(1500)
	if testing.Short() {
		seeds = 200
	}
	for seed := range seeds {
		d := randomDiagram(rand.New(rand.NewSource(seed)))
		lay, err := d.Layout()
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		checkLayout(t, lay, layoutcheck.Options{})
		recordsHold(t, lay)
		again, err := d.Layout()
		if err != nil {
			t.Fatalf("seed %d, again: %v", seed, err)
		}
		if a, b := lay.Text(), again.Text(); !reflect.DeepEqual(a, b) {
			t.Errorf("laid out twice, drawn differently:\n%s\n\n%s", strings.Join(a, "\n"), strings.Join(b, "\n"))
		}
		if d.Root.Columns > 0 {
			bare := *d
			bare.Edges = nil
			alone, err := bare.Layout()
			if err != nil {
				t.Fatalf("seed %d, without edges: %v", seed, err)
			}
			if !sameRelative(lay, alone) {
				t.Errorf("a grid's boxes moved by its edges:\n%s\n\nwithout them:\n%s", strings.Join(lay.Text(), "\n"), strings.Join(alone.Text(), "\n"))
			}
		}
		noEndBesideJoints(t, lay, lay.Text(TextOptions{Braille: BrailleThin}))
		if t.Failed() {
			t.Fatalf("seed %d", seed)
		}
	}
}

// sameRelative says a and b place every node alike, but for the whole
// drawing moved: each node where it is relative to the first.
func sameRelative(a, b *Layout) bool {
	if len(a.Nodes) != len(b.Nodes) || len(a.Nodes) == 0 {
		return len(a.Nodes) == len(b.Nodes)
	}
	oa, ob := a.Nodes[0].Rect, b.Nodes[0].Rect
	for i := range a.Nodes {
		ra, rb := a.Nodes[i].Rect, b.Nodes[i].Rect
		if ra.X-oa.X != rb.X-ob.X || ra.Y-oa.Y != rb.Y-ob.Y || ra.W != rb.W || ra.H != rb.H {
			return false
		}
	}
	return true
}

// Where every search gives up, each edge is drawn along the plain path
// between its boxes: through whatever is in the way, but off where a
// divider meets an outline wherever a move along the side clear of the
// boxes takes it off.
func TestPlainPathsKeepTheirPromisesAtRandom(t *testing.T) {
	defer func(n int) { maxSearch = n }(maxSearch)
	maxSearch = 0
	seeds := int64(500)
	if testing.Short() {
		seeds = 100
	}
	for seed := range seeds {
		d := randomDiagram(rand.New(rand.NewSource(seed)))
		lay, err := d.Layout()
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		checkLayout(t, lay, layoutcheck.Options{Plain: true})
		if t.Failed() {
			t.Fatalf("seed %d", seed)
		}
	}
}

// byteSource is a rand.Source drawing from b, eight bytes a number, and
// zeros once b runs out: a fuzzer's input as a diagram's choices, so that
// a byte changed is a field, a port or an edge changed, where DOT text
// changed is most often DOT no longer.
type byteSource struct {
	b []byte
	i int
}

func (s *byteSource) Int63() int64 {
	var v uint64
	for range 8 {
		var c byte
		if s.i < len(s.b) {
			c, s.i = s.b[s.i], s.i+1
		}
		v = v<<8 | uint64(c)
	}
	return int64(v >> 1)
}

func (s *byteSource) Seed(int64) {}

// FuzzLayout lays out a diagram of records, ports and sides made from its
// input, as randomDiagram makes one, and checks what
// TestLayoutKeepsItsPromisesAtRandom does: DOT text rarely mutates into a
// record with a port, and the choices here reach one at nearly every
// input. An odd first byte has every search give up, so the plain paths
// are drawn and checked too.
//
// go test runs the seeds and testdata/fuzz alone; make fuzz searches.
func FuzzLayout(f *testing.F) {
	for seed := range int64(8) {
		b := make([]byte, 160)
		rand.New(rand.NewSource(seed)).Read(b)
		f.Add(string(b))
	}
	f.Add("")
	f.Add("\x01")
	f.Fuzz(checkRandomLayout)
}

// TestFuzzCorpus runs checkRandomLayout on every input a search kept, in
// testdata/corpus: make fuzz-replay.
func TestFuzzCorpus(t *testing.T) {
	for name, src := range fuzzcorpus.Strings(t, filepath.Join("testdata", "corpus", "FuzzLayout")) {
		t.Run(name, func(t *testing.T) { checkRandomLayout(t, src) })
	}
}

// checkRandomLayout is FuzzLayout's check of one input.
func checkRandomLayout(t *testing.T, s string) {
	if len(s) > 2048 {
		return
	}
	plain := len(s) > 0 && s[0]&1 == 1
	if plain {
		defer func(n int) { maxSearch = n }(maxSearch)
		maxSearch = 0
	}
	if len(s) > 0 {
		s = s[1:]
	}
	d := randomDiagram(rand.New(&byteSource{b: []byte(s)}))
	lay, err := d.LayoutWith(LayoutOptions{MaxCells: 40000})
	if err != nil {
		return
	}
	checkLayout(t, lay, layoutcheck.Options{Plain: plain})
	recordsHold(t, lay)
	again, err := d.LayoutWith(LayoutOptions{MaxCells: 40000})
	if err != nil || !reflect.DeepEqual(lay.Text(), again.Text()) {
		t.Errorf("laid out twice, drawn differently, or refused the second time: %v", err)
	}
	// Drawn plain, a line may end at a joint where every move off it
	// would run through a box, and its arrowhead then sits across from
	// it: layoutcheck holds that exception, and the drawing's check,
	// which knows none, is for paths the search found.
	if !plain {
		noEndBesideJoints(t, lay, lay.Text(TextOptions{Braille: BrailleThin}))
	}
}

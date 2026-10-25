package diagram_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wisborg/output/diagram"
	"github.com/wisborg/output/diagram/dot"
)

// The layout benchmarks: what a change to the layering, the router or the
// labels' placement costs, measured on diagrams kept the same from one
// change to the next. make bench runs them; compare two runs with
// benchstat. Their time is almost all the router's search, which grows
// faster than the square of the edges -- the reason for dot's
// DefaultMaxEdges of 300 -- so the flows run up to it.

// flow is a flow of nodes boxes and edges edges between boxes picked by a
// fixed sequence, the same on every run; every edge labelled, if labels.
func flow(nodes, edges int, labels bool) *diagram.Diagram {
	d := &diagram.Diagram{}
	for i := range nodes {
		d.Root.Items = append(d.Root.Items, diagram.Item{Node: &diagram.Node{ID: fmt.Sprintf("n%d", i)}})
	}
	x := uint64(1)
	for len(d.Edges) < edges {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		a, c := int(x%uint64(nodes)), int((x>>20)%uint64(nodes))
		if a == c {
			continue
		}
		e := diagram.Edge{From: fmt.Sprintf("n%d", a), To: fmt.Sprintf("n%d", c), Head: true}
		if labels {
			e.Label = fmt.Sprintf("e%d", len(d.Edges))
		}
		d.Edges = append(d.Edges, e)
	}
	return d
}

// grid is a grid of side by side boxes, each joined to the one right of it
// and the one below: replicas in a grid, where every line runs between
// boxes a gap apart.
func grid(side int) *diagram.Diagram {
	d := &diagram.Diagram{Root: diagram.Group{Columns: side}}
	id := func(r, c int) string { return fmt.Sprintf("r%dc%d", r, c) }
	for r := range side {
		for c := range side {
			d.Root.Items = append(d.Root.Items, diagram.Item{Node: &diagram.Node{ID: id(r, c)}})
			if c+1 < side {
				d.Edges = append(d.Edges, diagram.Edge{From: id(r, c), To: id(r, c+1), Head: true})
			}
			if r+1 < side {
				d.Edges = append(d.Edges, diagram.Edge{From: id(r, c), To: id(r+1, c), Head: true})
			}
		}
	}
	return d
}

// records is a flow of nodes records of two to four fields, each field
// with a port, and edges edges between them picked by a fixed sequence,
// every other one by a port at each end; laid out in dir, straight or
// not. Where lines end on a record -- off its joints, beside its fields --
// is worked out by the router, the plain paths and the braille stubs, and
// none of the other benchmarks has a record.
func records(nodes, edges int, dir diagram.Direction, straight bool) *diagram.Diagram {
	d := &diagram.Diagram{Direction: dir, Straight: straight}
	labels := []string{"id", "name", "created at", "x"}
	fields := make([]int, nodes)
	for i := range nodes {
		n := &diagram.Node{ID: fmt.Sprintf("r%d", i)}
		fields[i] = 2 + i%3
		for k := range fields[i] {
			n.Record = append(n.Record, diagram.Field{Port: fmt.Sprintf("p%d", k), Label: labels[(i+k)%len(labels)]})
		}
		d.Root.Items = append(d.Root.Items, diagram.Item{Node: n})
	}
	x := uint64(1)
	for len(d.Edges) < edges {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		a, c := int(x%uint64(nodes)), int((x>>20)%uint64(nodes))
		if a == c {
			continue
		}
		e := diagram.Edge{From: fmt.Sprintf("r%d", a), To: fmt.Sprintf("r%d", c), Head: true}
		if len(d.Edges)%2 == 0 {
			e.FromPort = fmt.Sprintf("p%d", int(x>>40)%fields[a])
			e.ToPort = fmt.Sprintf("p%d", int(x>>48)%fields[c])
		}
		d.Edges = append(d.Edges, e)
	}
	return d
}

func layout(b *testing.B, d *diagram.Diagram) {
	b.Helper()
	for b.Loop() {
		if _, err := d.Layout(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLayoutFlow is flows of half as many boxes as edges, up to dot's
// default limit: how the router's time grows with the edges.
func BenchmarkLayoutFlow(b *testing.B) {
	for _, edges := range []int{50, 100, 200, dot.DefaultMaxEdges} {
		b.Run(fmt.Sprintf("edges=%d", edges), func(b *testing.B) { layout(b, flow(edges/2, edges, false)) })
	}
}

// BenchmarkLayoutLabels is a flow of 100 edges each labelled: the labels'
// places are chosen together by a local search, which this measures over
// the routing beneath it.
func BenchmarkLayoutLabels(b *testing.B) {
	layout(b, flow(50, 100, true))
}

// BenchmarkLayoutGrid is grids of 4 by 4 and 8 by 8 boxes, each joined to
// its neighbours: the router's search where every gap holds a line.
func BenchmarkLayoutGrid(b *testing.B) {
	for _, side := range []int{4, 8} {
		b.Run(fmt.Sprintf("side=%d", side), func(b *testing.B) { layout(b, grid(side)) })
	}
}

// BenchmarkLayoutRecords is flows of records, half their edges by ports,
// down and across, straight and not, and drawn as text in braille: the
// cost of choosing where lines end on records, which the plain boxes of
// the other benchmarks never ask.
func BenchmarkLayoutRecords(b *testing.B) {
	for _, edges := range []int{50, 100} {
		for _, dir := range []diagram.Direction{diagram.Down, diagram.Right} {
			for _, straight := range []bool{false, true} {
				name := fmt.Sprintf("edges=%d/dir=%v/straight=%v", edges, map[diagram.Direction]string{diagram.Down: "down", diagram.Right: "right"}[dir], straight)
				d := records(edges/2, edges, dir, straight)
				b.Run(name, func(b *testing.B) {
					for b.Loop() {
						lay, err := d.Layout()
						if err != nil {
							b.Fatal(err)
						}
						lay.Text(diagram.TextOptions{Braille: diagram.BrailleThin})
					}
				})
			}
		}
	}
}

// BenchmarkLayoutExamples is each of examples/*.dot, read once and laid out
// as the file asks: the diagrams a reader is shown first.
func BenchmarkLayoutExamples(b *testing.B) {
	paths, err := filepath.Glob(filepath.Join("..", "examples", "*.dot"))
	if err != nil || len(paths) == 0 {
		b.Fatalf("examples: %v", err)
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			b.Fatal(err)
		}
		d, err := dot.Parse(string(src))
		if err != nil {
			b.Fatalf("%s: %v", p, err)
		}
		b.Run(strings.TrimSuffix(filepath.Base(p), ".dot"), func(b *testing.B) { layout(b, d) })
	}
}

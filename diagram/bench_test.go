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

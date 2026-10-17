package picture

import (
	"image/color"
	"testing"

	"github.com/wisborg/output/diagram"
)

func layout(t *testing.T, d *diagram.Diagram) *diagram.Layout {
	t.Helper()
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	return lay
}

// The picture is the layout's size at the text size asked for, a filled
// node is filled in its colour inside its outline, and the background is
// the palette's.
func TestDrawFillsAndSizes(t *testing.T) {
	red := color.RGBA{0xFF, 0, 0, 0xFF}
	d := &diagram.Diagram{Root: diagram.Group{Columns: 2, Items: []diagram.Item{
		{Node: &diagram.Node{ID: "a", Label: "a wide label", Fill: red}},
		{Node: &diagram.Node{ID: "b"}},
	}}, Edges: []diagram.Edge{{From: "a", To: "b", Head: true}}}
	lay := layout(t, d)
	img, err := Draw(lay, Options{Palette: diagram.Dark, TextPx: 20})
	if err != nil {
		t.Fatal(err)
	}
	s := diagram.ScaleFor(20)
	w, h := s.Size(lay)
	if b := img.Bounds(); b.Dx() < int(w) || b.Dx() > int(w)+1 || b.Dy() < int(h) || b.Dy() > int(h)+1 {
		t.Errorf("%v, want %.0f by %.0f", b, w, h)
	}
	a, _ := lay.Node("a")
	// Inside a's outline, left of its label: the fill.
	x0, y0, _, y1 := s.Box(a.Rect)
	if got := img.RGBAAt(int(x0+s.Cell), int((y0+y1)/2)); got != red {
		t.Errorf("inside a %v, want its fill %v", got, red)
	}
	if got := img.RGBAAt(1, 1); got != diagram.Dark.Background {
		t.Errorf("the corner %v, want the background %v", got, diagram.Dark.Background)
	}
}

// Ink is laid where the edge runs: the middle of its path, between the two
// boxes, is the foreground's colour, not the background's.
func TestDrawDrawsEdges(t *testing.T) {
	d := &diagram.Diagram{Root: diagram.Group{Columns: 2, Items: []diagram.Item{
		{Node: &diagram.Node{ID: "a"}}, {Node: &diagram.Node{ID: "b"}},
	}}, Edges: []diagram.Edge{{From: "a", To: "b"}}}
	lay := layout(t, d)
	img, err := Draw(lay, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := diagram.ScaleFor(DefaultTextPx)
	xs, ys := s.Path(lay, lay.Edges[0])
	mx, my := (xs[0]+xs[len(xs)-1])/2, (ys[0]+ys[len(ys)-1])/2
	if got := img.RGBAAt(int(mx), int(my)); got == diagram.Light.Background {
		t.Errorf("no ink at (%.0f, %.0f), the middle of the edge", mx, my)
	}
}

package canvas

import (
	"image"
	"image/color"
	"testing"
)

func ink(img *image.RGBA) int {
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i+3] != 0 {
			n++
		}
	}
	return n
}

func newCanvas(t *testing.T) (*Canvas, *image.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	c, err := New(img, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c, img
}

// A polygon fills the same whichever way it winds.
func TestPolygonEitherWinding(t *testing.T) {
	c1, img1 := newCanvas(t)
	c2, img2 := newCanvas(t)
	pts := [][2]float64{{10, 10}, {90, 10}, {50, 90}}
	c1.Polygon(pts, color.Black)
	c2.Polygon([][2]float64{pts[2], pts[1], pts[0]}, color.Black)
	if a, b := ink(img1), ink(img2); a == 0 || a != b {
		t.Errorf("%d pixels one way round, %d the other", a, b)
	}
}

// A dashed line is the line with gaps in it: some of its ink, not all, and
// not none.
func TestDashed(t *testing.T) {
	c1, whole := newCanvas(t)
	c2, dashed := newCanvas(t)
	xs, ys := []float64{10, 90, 90}, []float64{10, 10, 90}
	c1.Polyline(xs, ys, 2, color.Black)
	c2.Dashed(xs, ys, 2, 6, 6, color.Black)
	w, d := ink(whole), ink(dashed)
	if d == 0 || d >= w*3/4 || d <= w/4 {
		t.Errorf("dashed %d pixels of the whole line's %d: want about half", d, w)
	}
}

package terminal

import "testing"

// A picture is drawn at twice the pixels its characters cover, as the
// terminal says them or ten by twenty where it does not, and never wider
// than MaxPictureWidth: the commands that draw for a terminal size their
// pictures by this alone.
func TestPictureScale(t *testing.T) {
	for _, c := range []struct {
		name          string
		s             Size
		cols          int
		perCol, perRo float64
	}{
		{"pixels said", Size{Cols: 100, Rows: 40, Width: 800, Height: 640}, 100, 16, 32},
		{"no pixels said", Size{Cols: 100, Rows: 40}, 100, 20, 40},
		{"nothing said", Size{}, 100, 20, 40},
		// Pixels but no rows to divide them by is no size in pixels.
		{"pixels and no rows", Size{Cols: 100, Width: 800, Height: 640}, 100, 20, 40},
		// 400 columns of 8 pixels, twice over, is 6400: halved to 3200.
		{"held to the widest", Size{Cols: 400, Rows: 40, Width: 3200, Height: 640}, 400, 8, 16},
	} {
		perCol, perRow := c.s.PictureScale(c.cols)
		if perCol != c.perCol || perRow != c.perRo {
			t.Errorf("%s: %v a column, %v a row; want %v and %v", c.name, perCol, perRow, c.perCol, c.perRo)
		}
	}
	// However many columns, the picture is no wider than the widest, and
	// a cell keeps its shape.
	s := Size{Cols: 200, Rows: 50, Width: 1800, Height: 1100}
	for cols := 1; cols <= 5000; cols++ {
		perCol, perRow := s.PictureScale(cols)
		if w := perCol * float64(cols); w > MaxPictureWidth+1e-9 {
			t.Fatalf("%d columns: %v pixels wide, past %v", cols, w, MaxPictureWidth)
		}
		if got, want := perRow/perCol, (1100.0/50)/(1800.0/200); got-want > 1e-9 || want-got > 1e-9 {
			t.Fatalf("%d columns: a cell %v times as tall as wide, want %v", cols, got, want)
		}
	}
}

// A picture's columns are held to the window's where the terminal says
// them, and kept as asked where it does not.
func TestPictureCols(t *testing.T) {
	for _, c := range []struct {
		s          Size
		cols, want int
	}{
		{Size{Cols: 120}, 80, 80},
		{Size{Cols: 120}, 999999999, 120},
		{Size{}, 999, 999},
	} {
		if got := c.s.PictureCols(c.cols); got != c.want {
			t.Errorf("%v columns in %+v: %d, want %d", c.cols, c.s, got, c.want)
		}
	}
}

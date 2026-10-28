package terminal

import (
	"math"
	"testing"
)

// A picture is drawn at twice the pixels its characters cover, as the
// terminal says them or ten by twenty where it does not, and never wider
// than MaxPictureWidth: the commands that draw for a terminal size their
// pictures by this alone.
func TestPictureScale(t *testing.T) {
	for _, c := range []struct {
		name           string
		s              Size
		cols           int
		perCol, perRow float64
	}{
		{"pixels said", Size{Cols: 100, Rows: 40, Width: 800, Height: 640}, 100, 16, 32},
		{"no pixels said", Size{Cols: 100, Rows: 40}, 100, 20, 40},
		{"nothing said", Size{}, 100, 20, 40},
		// Pixels but no rows to divide them by is no size in pixels.
		{"pixels and no rows", Size{Cols: 100, Width: 800, Height: 640}, 100, 20, 40},
		// 400 columns of 8 pixels, twice over, is 6400: halved to 3200.
		{"held to the widest", Size{Cols: 400, Rows: 40, Width: 3200, Height: 640}, 400, 8, 16},
	} {
		gotCol, gotRow := c.s.PictureScale(c.cols)
		if gotCol != c.perCol || gotRow != c.perRow {
			t.Errorf("%s: %v a column, %v a row; want %v and %v", c.name, gotCol, gotRow, c.perCol, c.perRow)
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
		{Size{Cols: 120}, 0, 1},
		{Size{}, -5, 1},
	} {
		if got := c.s.PictureCols(c.cols); got != c.want {
			t.Errorf("%v columns in %+v: %d, want %d", c.cols, c.s, got, c.want)
		}
	}
}

// A picture's size is the pixels its characters come to, rounded to the
// nearest: held to the cap, it is the cap exactly, where cutting the
// fraction off drew 3199 for 3200 at 161 columns of ten-pixel characters.
// It is never wider, and a count below one is one.
func TestPictureSize(t *testing.T) {
	for _, s := range []Size{{}, {Cols: 200, Rows: 50, Width: 1800, Height: 1100}, {Cols: 317, Rows: 71, Width: 2537, Height: 1999}} {
		for cols := 1; cols <= 5000; cols++ {
			w, _ := s.PictureSize(cols, 10)
			perCol, _ := s.PictureScale(cols)
			if exact := perCol * float64(cols); w > MaxPictureWidth || float64(w) < exact-0.5 || float64(w) > exact+0.5 {
				t.Fatalf("%+v, %d columns: %d pixels wide, for %v", s, cols, w, exact)
			}
		}
	}
	// Characters ten pixels wide, drawn at twenty, reach the cap at 160
	// columns: from there on the picture is the cap's width, not a pixel
	// short of it.
	for cols := 160; cols <= 5000; cols++ {
		if w, _ := (Size{}).PictureSize(cols, 10); w != MaxPictureWidth {
			t.Fatalf("%d columns held to the cap: %d pixels wide, want %d", cols, w, MaxPictureWidth)
		}
	}
	// 39.75 pixels a row, scaled to the cap, ten times over.
	if w, h := (Size{}).PictureSize(161, 10); w != 3200 || h != 398 {
		t.Errorf("161 columns by 10 rows: %d by %d pixels, want 3200 by 398", w, h)
	}
	// A height past what an int holds is held below it, the same on
	// every platform, rather than wrapping round to a negative one.
	for _, rows := range []int{1 << 40, math.MaxInt} {
		if _, h := (Size{}).PictureSize(80, rows); h != math.MaxInt32 {
			t.Errorf("%d rows: %d pixels high, want %d", rows, h, math.MaxInt32)
		}
	}
	for _, n := range []int{0, -1, -1000} {
		if w, h := (Size{}).PictureSize(n, n); w != 20 || h != 40 {
			t.Errorf("%d columns and rows: %d by %d pixels, want one character's 20 by 40", n, w, h)
		}
		if perCol, perRow := (Size{}).PictureScale(n); perCol != 20 || perRow != 40 {
			t.Errorf("%d columns: %v by %v a character, want 20 by 40", n, perCol, perRow)
		}
	}
}

package terminal

import "math"

// Size is a terminal's size: in characters, and in pixels where it says.
// A picture drawn to fill so many characters is drawn at the pixels they
// cover.
type Size struct {
	Cols, Rows    int
	Width, Height int
}

// MaxPictureWidth is the widest picture PictureSize gives for a terminal,
// in pixels: one the size of a screen of two pixels to a point is seconds
// to draw and megabytes to send, for a picture the terminal shows no
// sharper. It holds the width, which the terminal's columns fix; the
// height is the rows a caller asks for, as many as it asks, scaled with
// the width but not held: holding it would cut a tall chart of many
// strips short.
const MaxPictureWidth = 3200

// maxPictureHeight bounds PictureSize's height below what an int holds on
// every platform: a --height with a digit too many put past it came out
// negative on one and the largest int on another. Any height near it is
// refused as too large by the drawing, where the caller says so.
const maxPictureHeight = math.MaxInt32

// PictureSize is the size in pixels to draw a picture cols characters
// wide and rows high at, for the terminal: PictureScale's pixels a column
// and a row, rounded to the nearest pixel and never wider than
// MaxPictureWidth, nor taller than an int holds on every platform. A
// count below one is taken as one.
func (s Size) PictureSize(cols, rows int) (w, h int) {
	perCol, perRow := s.PictureScale(cols)
	w = min(int(math.Round(perCol*float64(max(cols, 1)))), MaxPictureWidth)
	h = int(min(math.Round(perRow*float64(max(rows, 1))), maxPictureHeight))
	return w, h
}

// PictureScale is how many pixels a picture cols characters wide is drawn
// at for each of its columns and each of its rows: twice what a character
// covers, so that a screen of two pixels to a point draws it sharp, and
// less where that would make it wider than MaxPictureWidth. A terminal
// that does not say its size in pixels -- or in characters, to divide
// them by -- is taken to have characters ten pixels by twenty. A count
// below one is taken as one. PictureSize is the size those come to; this
// is for a caller that needs a column's width itself, as one fitting a
// drawing's own width to the columns does.
func (s Size) PictureScale(cols int) (perCol, perRow float64) {
	cellW, cellH := 10.0, 20.0
	if s.Width > 0 && s.Height > 0 && s.Cols > 0 && s.Rows > 0 {
		cellW, cellH = float64(s.Width)/float64(s.Cols), float64(s.Height)/float64(s.Rows)
	}
	scale := min(1, MaxPictureWidth/(2*cellW*float64(max(cols, 1))))
	return 2 * cellW * scale, 2 * cellH * scale
}

// PictureCols is cols held to the window's columns, where s says them.
// A picture's width in columns goes into its escape sequence, and one
// wider than the window is cut off or wrapped by the terminal: a width
// typed with a digit too many was a request for that many columns. Where
// the terminal says nothing -- a picture sent into a pipe -- cols is the
// caller's own word, and is kept. A count below one is taken as one, as
// PictureSize and PictureScale take it: it goes into the escape sequence.
func (s Size) PictureCols(cols int) int {
	cols = max(cols, 1)
	if s.Cols > 0 {
		return min(cols, s.Cols)
	}
	return cols
}

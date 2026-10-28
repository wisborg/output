package terminal

// Size is a terminal's size: in characters, and in pixels where it says.
// A picture drawn to fill so many characters is drawn at the pixels they
// cover.
type Size struct {
	Cols, Rows    int
	Width, Height int
}

// MaxPictureWidth is the widest picture PictureScale draws for a terminal,
// in pixels: one the size of a screen of two pixels to a point is seconds
// to draw and megabytes to send, for a picture the terminal shows no
// sharper.
const MaxPictureWidth = 3200.0

// PictureScale is how many pixels a picture cols characters wide is drawn
// at for each of its columns and each of its rows: twice what a character
// covers, so that a screen of two pixels to a point draws it sharp, and
// less where that would make it wider than MaxPictureWidth. A terminal
// that does not say its size in pixels -- or in characters, to divide
// them by -- is taken to have characters ten pixels by twenty.
func (s Size) PictureScale(cols int) (perCol, perRow float64) {
	cellW, cellH := 10.0, 20.0
	if s.Width > 0 && s.Height > 0 && s.Cols > 0 && s.Rows > 0 {
		cellW, cellH = float64(s.Width)/float64(s.Cols), float64(s.Height)/float64(s.Rows)
	}
	scale := min(1, MaxPictureWidth/(2*cellW*float64(cols)))
	return 2 * cellW * scale, 2 * cellH * scale
}

// PictureCols is cols held to the window's columns, where s says them.
// A picture's width in columns goes into its escape sequence, and one
// wider than the window is cut off or wrapped by the terminal: a width
// typed with a digit too many was a request for that many columns. Where
// the terminal says nothing -- a picture sent into a pipe -- cols is the
// caller's own word, and is kept.
func (s Size) PictureCols(cols int) int {
	if s.Cols > 0 {
		return min(cols, s.Cols)
	}
	return cols
}

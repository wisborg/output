package terminal

// Size is a terminal's size: in characters, and in pixels where it says.
// A picture drawn to fill so many characters is drawn at the pixels they
// cover.
type Size struct {
	Cols, Rows    int
	Width, Height int
}

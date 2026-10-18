package chart

// Column is what one column of a drawing shows: the readings whose x falls
// in it, or that none does.
//
// A column holds the mean, which is what the line passes through, and the
// extremes, which a renderer may draw as a band behind it: at a few hundred
// columns for an hour's samples, a column stands for many readings, and the
// mean alone would hide how far they spread.
type Column struct {
	// Ok says a reading falls in the column. A column without one draws
	// nothing.
	Ok bool
	// Mean, Min and Max are the column's readings', meaningful only where
	// Ok.
	Mean, Min, Max float64
	// Joined says the line runs into this column from the previous column
	// with a reading: the two hold readings of one segment. A column whose
	// readings begin a segment is not joined, whatever lies before it.
	Joined bool
}

// Bin is n columns spanning x0 to x1, each holding the readings of segments
// whose x falls in it. A reading outside the span is left out, and so is
// one whose x or reading is not a finite number, as Segments leaves it
// out. n of zero or less, or an empty span, is no columns.
//
// x0 is the value at the first column and x1 at the last, so x0 above x1
// bins the axis from its top down: an inverted scale's, from Scale.Ends.
// Columns are joined whichever way a segment's readings run across them.
//
// Columns between two of one segment's readings, holding none of their own,
// are left empty and the later column is joined: a sparse recording drawn
// wide is still one line. Only a break between segments leaves the line
// broken.
func Bin(segments [][]Point, x0, x1 float64, n int) []Column {
	if n <= 0 || x1 == x0 || !finite(x1-x0) {
		return nil
	}
	lo, hi := min(x0, x1), max(x0, x1)
	cols := make([]Column, n)
	sums := make([]float64, n)
	counts := make([]int, n)
	for _, seg := range segments {
		prev := -1
		for _, p := range seg {
			if !(p.X >= lo && p.X <= hi) || !finite(p.Y) {
				// Leaving the span breaks the line, as a missing reading
				// does: what is joined to what outside it is not drawn.
				prev = -1
				continue
			}
			c := min(int((p.X-x0)/(x1-x0)*float64(n)), n-1)
			if counts[c] == 0 {
				cols[c] = Column{Ok: true, Min: p.Y, Max: p.Y}
			}
			// Joined marks the right-hand of two columns a line runs
			// between: the later, where the axis runs left to right; the
			// earlier, where it runs right to left and a segment's readings
			// come leftwards.
			switch {
			case prev >= 0 && x1 > x0 && prev < c:
				cols[c].Joined = true
			case prev >= 0 && x1 < x0 && prev > c:
				cols[prev].Joined = true
			}
			cols[c].Min, cols[c].Max = min(cols[c].Min, p.Y), max(cols[c].Max, p.Y)
			sums[c] += p.Y
			counts[c]++
			prev = c
		}
	}
	for c := range cols {
		if counts[c] > 0 {
			cols[c].Mean = sums[c] / float64(counts[c])
		}
	}
	return cols
}

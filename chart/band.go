package chart

import (
	"image/color"
	"math"
	"strings"
)

// Band is a stretch of a chart's x axis it marks, from From to To in the
// axis's own units: the intervals of a workout, its climbs, the hours a
// service was degraded.
//
// A band is the same stretch in every strip -- it is a fact about what the
// chart is of, not about one series -- so it is the chart's, not a strip's.
type Band struct {
	From, To float64
	// Kind is the band's kind: its index in the chart's BandKinds.
	Kind int
}

// Span is the band's ends in order: a band written To before From is the
// same stretch, and every renderer draws it as one, from Span.
func (b Band) Span() (from, to float64) { return min(b.From, b.To), max(b.From, b.To) }

// BandKind is a kind of band: how a legend names it, and how it is marked.
type BandKind struct {
	// Name is the kind in a legend: "intervals".
	Name string
	// Mark is its mark in a braille chart's row of bands: '━', say, or a
	// slope '╱' for climbs. A control or format character, the zero
	// rune among them, is drawn as U+FFFD, as a title's is.
	Mark rune
	// Colour is its colour in a picture, drawn over the background and
	// under the grid and the line: faint, alpha and all, so a marked
	// stretch reads as easily as any other.
	Colour color.Color
}

// UsedBandKinds are the indices of the kinds among the chart's bands, in
// the order of BandKinds: the rows of a braille chart's bands, and the
// entries of a legend. A kind no band has is no row and no entry, and a
// band whose kind is not in BandKinds is not drawn.
func (c Chart) UsedBandKinds() []int {
	var kinds []int
	for k := range c.BandKinds {
		for _, b := range c.Bands {
			if b.Kind == k {
				kinds = append(kinds, k)
				break
			}
		}
	}
	return kinds
}

// BandRows is how many of a braille chart's lines bands of kinds kinds
// take: a row each and a legend beneath them, or none for none. A caller
// laying out a drawing of a given height asks it before it has a chart.
func BandRows(kinds int) int {
	if kinds == 0 {
		return 0
	}
	return kinds + 1
}

// brailleBands is the chart's bands in text, beneath its x labels: a row
// for each kind, cols characters across the plot past a gutter gutter
// wide, and a legend naming each kind by its mark. A character is marked
// where a band covers its middle, so that two bands a character or more
// apart -- two intervals and the recovery between -- stay apart; a band
// too short to cover any middle marks the character it falls in, so that
// it is never lost.
func (c Chart) brailleBands(gutter, cols int) []string {
	kinds := c.UsedBandKinds()
	if len(kinds) == 0 {
		return nil
	}
	pad := strings.Repeat(" ", gutter+1)
	span := c.X.Max - c.X.Min
	var out, legend []string
	for _, k := range kinds {
		kind := c.BandKinds[k]
		mark := bandMark(kind.Mark)
		row := []rune(strings.Repeat(" ", cols))
		for _, b := range c.Bands {
			if b.Kind != k || span <= 0 {
				continue
			}
			// The characters whose middles the band covers, or the one
			// its middle falls in where it is too short to cover any.
			// Placed by the scale's own fractions, so an inverted axis's
			// bands are mirrored as its line is.
			lo, hi := b.Span()
			f0, cut0 := c.X.Frac(lo)
			f1, cut1 := c.X.Frac(hi)
			if cut0 && cut1 && f0 == f1 {
				// Wholly before the axis or past it, or an end that is no
				// number: drawn nowhere, as the picture draws it. Its
				// middle held to the first character, one before the axis
				// was marked there.
				continue
			}
			f0, f1 = min(f0, f1), max(f0, f1)
			from := int(math.Ceil(f0*float64(cols) - 0.5))
			to := int(math.Floor(f1*float64(cols) - 0.5))
			if from > to {
				// Held to the last, as Bin holds a reading at the axis's
				// end: a band at the end fell a character past it, and
				// was lost.
				from = min(int((f0+f1)/2*float64(cols)), cols-1)
				to = from
			}
			for x := max(from, 0); x <= min(to, cols-1); x++ {
				row[x] = mark
			}
		}
		out = append(out, pad+strings.TrimRight(string(row), " "))
		legend = append(legend, string(mark)+" "+kind.Name)
	}
	return append(out, cut(pad+strings.Join(legend, "   "), gutter+1+cols))
}

// bandMark is m as a band's row and its legend draw it: a control or
// format character as U+FFFD, as in a title, so that an escape is not a
// command to the terminal. Its width is left as it is: in a CJK locale
// the axis's '─' is two columns a character too, and '━' beneath it
// stays under its ticks, where a mark of one column would not.
func bandMark(m rune) rune {
	if hidden(m) {
		return '\uFFFD'
	}
	return m
}

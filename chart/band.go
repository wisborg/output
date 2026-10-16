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

// BandKind is a kind of band: how a legend names it, and how it is marked.
type BandKind struct {
	// Name is the kind in a legend: "intervals".
	Name string
	// Mark is its mark in a braille chart's row of bands: '━', say, or a
	// slope '╱' for climbs.
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
		row := []rune(strings.Repeat(" ", cols))
		for _, b := range c.Bands {
			if b.Kind != k || span <= 0 {
				continue
			}
			// The characters whose middles the band covers, or the one
			// its middle falls in where it is too short to cover any.
			from := int(math.Ceil((b.From-c.X.Min)/span*float64(cols) - 0.5))
			to := int(math.Floor((b.To-c.X.Min)/span*float64(cols) - 0.5))
			if from > to {
				from = int(((b.From+b.To)/2 - c.X.Min) / span * float64(cols))
				to = from
			}
			for x := max(from, 0); x <= min(to, cols-1); x++ {
				row[x] = kind.Mark
			}
		}
		out = append(out, pad+strings.TrimRight(string(row), " "))
		legend = append(legend, string(kind.Mark)+" "+kind.Name)
	}
	return append(out, pad+strings.Join(legend, "   "))
}

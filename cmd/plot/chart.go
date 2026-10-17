package main

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/wisborg/output/chart"
)

// series is what is plotted: the x column and the y columns, read from a
// table.
type series struct {
	xName  string // "" where x is the row
	xKind  chart.Kind
	yNames []string
	// rows are the table's rows that have an x, in x's order: the x and
	// each y column's reading, or none.
	rows []seriesRow
	// noX is how many rows had no x and were left out.
	noX int
}

type seriesRow struct {
	x  float64
	ys []float64
	ok []bool
}

// readSeries reads the columns ys against x from t: x an index into t's
// columns, or -1 for the row's own number. A row without an x has no
// place on the axis and is left out; one without a y is a gap in that y's
// line.
//
// The rows are put in x's order, stably, since a file of prices is as
// often written newest first as oldest, and a line drawn in the file's
// order would run backwards along the axis.
func readSeries(t *table, x int, xcol column, ys []int, ycols []column) series {
	s := series{xKind: chart.Number}
	switch {
	case x >= 0 && xcol.kind == dateColumn:
		s.xName, s.xKind = t.names[x], chart.Date
	case x >= 0:
		s.xName = t.names[x]
	}
	for _, y := range ys {
		s.yNames = append(s.yNames, t.names[y])
	}
	for i, rec := range t.rows {
		r := seriesRow{x: float64(i + 1), ys: make([]float64, len(ys)), ok: make([]bool, len(ys))}
		if x >= 0 {
			v, ok := xcol.value(rec[x])
			if !ok {
				s.noX++
				continue
			}
			r.x = v
		}
		for k, y := range ys {
			r.ys[k], r.ok[k] = ycols[k].value(rec[y])
		}
		s.rows = append(s.rows, r)
	}
	sort.SliceStable(s.rows, func(i, j int) bool { return s.rows[i].x < s.rows[j].x })
	return s
}

// chartOptions are the choices a chart is built with besides its data.
type chartOptions struct {
	title  string
	smooth int  // rows to average each reading over; 0 or 1 is none
	robust bool // scale y from the middle 98% of readings
}

// buildChart is s's chart, for a drawing of resolution columns -- dots
// across, or pixels -- with at most xTicks steps between the x axis's ticks
// and yTicks between each strip's.
//
// Each reading's time, to the chart, is its row: the line joins one row's
// reading to the next, whatever lies between them on the x axis -- a file
// of trading days has no weekends, and its line should not break at every
// one. It breaks where a row has no reading, as far as the drawing can
// show: a missing reading or two among a hundred thousand rows is less
// than a column of the drawing, and breaking the line there would only
// shred it.
func buildChart(s series, opts chartOptions, resolution, xTicks, yTicks int) (chart.Chart, error) {
	if len(s.rows) == 0 {
		return chart.Chart{}, errors.New("no rows with a value to plot against")
	}
	lo, hi := s.rows[0].x, s.rows[len(s.rows)-1].x
	c := chart.Chart{X: chart.ExactScale(lo, hi, xTicks, s.xKind)}
	maxGap := time.Duration(max(1, 2*float64(len(s.rows))/float64(max(resolution, 1))) * float64(time.Second))
	for k, name := range s.yNames {
		points := make([]chart.Point, len(s.rows))
		for i, r := range s.rows {
			points[i] = chart.Point{X: r.x, T: float64(i), Y: r.ys[k], Ok: r.ok[k]}
		}
		segs := chart.Segments(points, maxGap)
		if opts.smooth > 1 {
			segs = chart.Smooth(segs, time.Duration(opts.smooth)*time.Second)
		}
		ylo, yhi, ok := yRange(segs, opts.robust)
		if !ok {
			return chart.Chart{}, fmt.Errorf("%q has no numbers to plot", name)
		}
		c.Strips = append(c.Strips, chart.Strip{Title: name, Y: chart.NewScale(ylo, yhi, yTicks, chart.Number), Segments: segs})
	}
	c.Title = opts.title
	if c.Title == "" {
		c.Title = defaultTitle(s, opts.smooth)
	}
	return c, nil
}

// yRange is the span of segs's readings: all of them, or with --robust the
// middle 98%, as chart.RobustRange leaves them.
//
// All of them by default, because the data is anyone's: a spike in a power
// meter's trace is a dropout to hide, while a day of rain twice the year's
// next wettest is the one the chart is drawn to show.
func yRange(segs [][]chart.Point, robust bool) (lo, hi float64, ok bool) {
	if robust {
		return chart.RobustRange(segs)
	}
	lo, hi = math.Inf(1), math.Inf(-1)
	for _, seg := range segs {
		for _, p := range seg {
			lo, hi = min(lo, p.Y), max(hi, p.Y)
		}
	}
	return lo, hi, !math.IsInf(lo, 1)
}

// defaultTitle names what s plots against what: "close against date",
// "open, close and volume against row".
func defaultTitle(s series, smooth int) string {
	names := strings.Join(s.yNames, ", ")
	if n := len(s.yNames); n > 1 {
		names = strings.Join(s.yNames[:n-1], ", ") + " and " + s.yNames[n-1]
	}
	x := s.xName
	if x == "" {
		x = "row"
	}
	title := names + " against " + x
	if smooth > 1 {
		title += fmt.Sprintf(", averaged over %d rows", smooth)
	}
	return title
}

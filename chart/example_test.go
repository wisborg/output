package chart_test

import (
	"fmt"
	"math"
	"time"

	"github.com/wisborg/output/chart"
)

// A chart of a reading taken once a minute for an hour, with a quarter of
// an hour missing, drawn in braille with two stretches marked beneath it.
func ExampleChart_Braille() {
	var points []chart.Point
	for m := 0; m <= 60; m++ {
		t := float64(m * 60)
		points = append(points, chart.Point{X: t, T: t, Y: 50 + 40*math.Sin(t/600), Ok: m < 20 || m >= 35})
	}
	segments := chart.Segments(points, 2*time.Minute)
	lo, hi, _ := chart.RobustRange(segments)
	c := chart.Chart{
		Title:     "load (%) against time",
		X:         chart.ExactScale(0, 3600, 4, chart.Clock),
		Strips:    []chart.Strip{{Title: "load (%)", Y: chart.NewScale(lo, hi, 4, chart.Number), Segments: segments}},
		BandKinds: []chart.BandKind{{Name: "maintenance", Mark: '━'}},
		Bands:     []chart.Band{{From: 600, To: 1200, Kind: 0}},
	}
	for _, line := range c.Braille(44, 12) {
		fmt.Println(line)
	}
	// Output:
	// load (%) against time
	// 100┤⠀⠀⠀⠀⠀⠀⠀⠀⣀⣀⣀⣀⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
	//    │⠀⠀⠀⠀⣀⡤⠞⠉⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
	//    │⠀⣀⡴⠋⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
	//  50┤⠞⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
	//    │⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠒⢦⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⡴
	//    │⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠳⠤⣄⡀⠀⠀⠀⠀⣀⡤⠴⠋⠁⠀
	//   0┤⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠉⠉⠉⠉⠁⠀⠀⠀⠀⠀
	//    └┬────────┬─────────┬─────────┬─────────┬
	//     0:00   0:15      0:30      0:45     1:00
	//            ━━━━━━
	//     ━ maintenance
}

// ExampleNewScale_log is a log scale for readings from 3 ms to 40 s, as a
// latency's: ticked at 1, 2 and 5 times each power of ten, and labelled
// briefly. Draw a reading at or below zero as a gap -- Point.Ok false --
// since a log scale has no place for it.
func ExampleNewScale_log() {
	s := chart.NewScale(0.003, 40, 16, chart.Log)
	for _, v := range s.Ticks() {
		fmt.Print(s.Label(v), " ")
	}
	fmt.Println()
	// Output:
	// 0.002 0.005 0.01 0.02 0.05 0.1 0.2 0.5 1 2 5 10 20 50
}

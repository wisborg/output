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

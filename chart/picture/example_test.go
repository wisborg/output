package picture_test

import (
	"bytes"
	"fmt"
	"image/png"
	"log"
	"time"

	"github.com/wisborg/output/chart"
	"github.com/wisborg/output/chart/picture"
)

// A chart drawn as a PNG, 1600 by 600, in the dark palette, with a band
// in a faint wash of the line's colour.
func ExampleDraw() {
	var points []chart.Point
	for s := 0; s <= 3600; s += 5 {
		t := float64(s)
		points = append(points, chart.Point{X: t, T: t, Y: float64(200 + s%600/4), Ok: true})
	}
	segments := chart.Segments(points, 10*time.Second)
	lo, hi, _ := chart.RobustRange(segments)
	c := chart.Chart{
		Title:     "power (W) against time",
		X:         chart.ExactScale(0, 3600, 8, chart.Clock),
		Strips:    []chart.Strip{{Title: "power (W)", Y: chart.NewScale(lo, hi, 5, chart.Number), Segments: segments}},
		BandKinds: []chart.BandKind{{Name: "intervals", Mark: '━', Colour: picture.Faded(picture.Dark.Line, 0.14)}},
		Bands:     []chart.Band{{From: 600, To: 900, Kind: 0}},
	}
	img, err := picture.Draw(c, 1600, 600, picture.Options{Palette: picture.Dark})
	if err != nil {
		log.Fatal(err)
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		log.Fatal(err)
	}
	fmt.Println(img.Bounds().Dx(), "by", img.Bounds().Dy(), "pixels")
	// Output:
	// 1600 by 600 pixels
}

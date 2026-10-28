package terminal_test

import (
	"bytes"
	"fmt"
	"image/png"
	"os"

	"github.com/wisborg/output/chart"
	"github.com/wisborg/output/chart/picture"
	"github.com/wisborg/output/chart/terminal"
)

// show draws c in the terminal: as a picture where the terminal shows one,
// the width of the window, at the pixels PictureScale says, and in braille
// everywhere else.
func show(c chart.Chart) error {
	size, tty := terminal.SizeOf(os.Stdout)
	how, note, err := terminal.Choose(terminal.Auto, os.Getenv, tty)
	if err != nil {
		return err
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, note)
	}
	cols, rows := max(size.Cols, 80), 24
	if how == terminal.Braille {
		for _, l := range c.Braille(cols, rows) {
			fmt.Println(l)
		}
		return nil
	}
	pal := picture.Dark
	if terminal.LightBackground(os.Getenv) {
		pal = picture.Light
	}
	cols = size.PictureCols(cols)
	perCol, perRow := size.PictureScale(cols)
	img, err := picture.Draw(c, int(perCol*float64(cols)), int(perRow*float64(rows)), picture.Options{Palette: pal})
	if err != nil {
		return err
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		return err
	}
	return terminal.Write(os.Stdout, how, b.Bytes(), cols)
}

// How a chart is shown: a picture in a terminal known to show one -- iTerm2
// here, by the variable it sets -- and braille into a pipe, whatever the
// terminal; show above draws it either way.
func Example() {
	_ = show
	iterm := func(k string) string {
		if k == "LC_TERMINAL" {
			return "iTerm2"
		}
		return ""
	}
	how, _, _ := terminal.Choose(terminal.Auto, iterm, true)
	fmt.Println("in iTerm2:", how)
	how, _, _ = terminal.Choose(terminal.Auto, iterm, false)
	fmt.Println("into a pipe:", how)
	// Output:
	// in iTerm2: iterm
	// into a pipe: braille
}

// A picture for a terminal is drawn at twice the pixels its characters
// cover, so that it is sharp on a screen of two pixels to a point, and
// no wider than MaxPictureWidth; its columns are held to the window's.
func ExampleSize_PictureScale() {
	// A window of 120 columns by 40 rows, 960 pixels by 800: characters
	// 8 by 20.
	size := terminal.Size{Cols: 120, Rows: 40, Width: 960, Height: 800}
	cols := size.PictureCols(200)
	perCol, perRow := size.PictureScale(cols)
	fmt.Printf("%d columns, %.0f by %.0f pixels a character, %.0f wide\n", cols, perCol, perRow, perCol*float64(cols))
	// A terminal that says nothing: characters 10 by 20, and 300 columns
	// of them held to MaxPictureWidth.
	perCol, perRow = terminal.Size{}.PictureScale(300)
	fmt.Printf("%.2f by %.2f pixels a character, %.0f wide\n", perCol, perRow, perCol*300)
	// Output:
	// 120 columns, 16 by 40 pixels a character, 1920 wide
	// 10.67 by 21.33 pixels a character, 3200 wide
}

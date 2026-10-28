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

// show draws c in the terminal the width of the window, 80 characters
// where it does not say: as a picture where the terminal shows one, at the
// pixels PictureSize says, and in braille everywhere else.
func show(c chart.Chart) error {
	size, tty := terminal.SizeOf(os.Stdout)
	how, note, err := terminal.Choose(terminal.Auto, os.Getenv, tty)
	if err != nil {
		return err
	}
	if note != "" {
		fmt.Fprintln(os.Stderr, note)
	}
	cols, rows := size.Cols, 24
	if cols <= 0 {
		cols = 80
	}
	if err := c.Check(); err != nil {
		return err
	}
	if how == terminal.Braille {
		lines := c.Braille(cols, rows)
		if lines == nil {
			return fmt.Errorf("a window %d characters wide is too narrow for the chart", cols)
		}
		for _, l := range lines {
			fmt.Println(l)
		}
		return nil
	}
	pal := picture.Dark
	if terminal.LightBackground(os.Getenv) {
		pal = picture.Light
	}
	w, h := size.PictureSize(cols, rows)
	img, err := picture.Draw(c, w, h, picture.Options{Palette: pal})
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
func ExampleSize_PictureSize() {
	// A window of 120 columns by 40 rows, 960 pixels by 800: characters
	// 8 by 20. Asked for 200 columns and 30 rows.
	size := terminal.Size{Cols: 120, Rows: 40, Width: 960, Height: 800}
	cols := size.PictureCols(200)
	w, h := size.PictureSize(cols, 30)
	fmt.Printf("%d columns, %d by %d pixels\n", cols, w, h)
	// A terminal that says nothing: characters 10 by 20, and 300 columns
	// of them held to MaxPictureWidth.
	w, h = terminal.Size{}.PictureSize(300, 30)
	fmt.Printf("300 columns, %d by %d pixels\n", w, h)
	// Output:
	// 120 columns, 1920 by 1200 pixels
	// 300 columns, 3200 by 640 pixels
}

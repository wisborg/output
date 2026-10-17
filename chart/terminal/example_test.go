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
// the width of the window, and in braille everywhere else.
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
	// The pixels the characters cover, where the terminal says; ten by
	// twenty a character where it does not.
	w, h := cols*10, rows*20
	if size.Width > 0 && size.Cols > 0 {
		w, h = size.Width*cols/size.Cols, size.Height*rows/max(size.Rows, 1)
	}
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

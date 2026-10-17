// Command diagram draws a diagram written in Graphviz's DOT language -- a
// flow chart, a database cluster and its replicas, a service and what it
// calls -- in the terminal, or as a PNG or SVG.
//
//	diagram cluster.dot
//	diagram -out cluster.svg cluster.dot
//	some-program --graph | diagram
//
// DOT is read as Graphviz reads it, with a few attributes of this
// command's own, which Graphviz ignores: columns=N or rows=N lay a graph
// or a cluster out in a grid, in the order it is written, rather than in
// layers from its edges -- so a box stays where it is when a link changes --
// row= and col= pin a node or a cluster to a cell, and style=lane draws a
// cluster as a band across the diagram, as a region or a data centre is.
//
// It draws with github.com/wisborg/output/diagram: in iTerm2, WezTerm,
// kitty and Ghostty a picture; in any other terminal, or into a pipe, in
// box-drawing characters.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/font/opentype"

	"github.com/wisborg/output/chart/terminal"
	"github.com/wisborg/output/diagram"
	"github.com/wisborg/output/diagram/dot"
	"github.com/wisborg/output/diagram/picture"
	"github.com/wisborg/output/diagram/svg"
)

func main() {
	err := run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv)
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "diagram:", err)
		os.Exit(1)
	}
}

const usage = `diagram draws a diagram written in Graphviz's DOT language, in the
terminal or as a PNG or an SVG.

usage: diagram [flags] [FILE]

FILE is read as DOT; without FILE, or with -, standard input is. Besides
Graphviz's own attributes, diagram reads:

  columns=N, rows=N   on the graph or a cluster: lay it out in a grid of N
                      columns, filled row by row in the order written, or N
                      rows, filled column by column -- places that do not
                      move when an edge changes
  row=R, col=C        on a node or a cluster: pin it to that cell
  style=lane          on a cluster: a band across the diagram, set off by a
                      line, its label at its side

A cluster is a subgraph whose name begins with "cluster".

flags:
`

type options struct {
	out      string
	terminal string
	theme    string
	textPx   float64
	fonts    []string
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) error {
	var o options
	fs := flag.NewFlagSet("diagram", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	fs.StringVar(&o.out, "out", "", "write the diagram to this file rather than the terminal: .png, .svg, or .txt for the text drawing")
	fs.StringVar(&o.terminal, "terminal", "auto", "how the diagram is shown in the terminal: auto (a picture where the terminal shows one, text otherwise), iterm, kitty or text")
	fs.StringVar(&o.theme, "theme", "auto", "the picture's palette: auto (light for a file; the terminal's own in a terminal), light or dark")
	fs.Float64Var(&o.textPx, "text-size", picture.DefaultTextPx, "the size of a PNG's or an SVG's text, in pixels; everything else is in proportion")
	fs.Func("font", "a TrueType or OpenType font for the letters the built-in font lacks, in a PNG; repeat for several", func(s string) error {
		o.fonts = append(o.fonts, s)
		return nil
	})
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 1 {
		return fmt.Errorf("one file at a time: %s", strings.Join(fs.Args(), " "))
	}
	if o.textPx <= 0 {
		return fmt.Errorf("-text-size %v: a size in pixels, above 0", o.textPx)
	}
	// The flags are checked whichever way the diagram is drawn: a wrong
	// one is a mistake to be told of, not one that waits for a terminal
	// that shows pictures to surface.
	if _, err := palette(o.theme, false, getenv); err != nil {
		return err
	}
	switch o.terminal {
	case "auto", "iterm", "kitty", "text":
	default:
		return fmt.Errorf("-terminal %q: use auto, iterm, kitty or text", o.terminal)
	}
	in, name := stdin, "standard input"
	if fs.NArg() == 1 && fs.Arg(0) != "-" {
		f, err := os.Open(fs.Arg(0))
		if err != nil {
			return err
		}
		defer f.Close()
		in, name = f, fs.Arg(0)
	}
	src, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	d, err := dot.Parse(string(src))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	lay, err := d.Layout()
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if o.out != "" {
		if err := writeFile(o, lay, getenv); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "diagram %s\n", o.out)
		return nil
	}
	return writeTerminal(o, lay, stdout, stderr, getenv)
}

// palette is -theme's palette: for auto, light for a file, read on a page,
// and in a terminal the terminal's own, as COLORFGBG says it -- dark where
// it does not say.
func palette(theme string, inTerminal bool, getenv func(string) string) (diagram.Palette, error) {
	switch theme {
	case "light":
		return diagram.Light, nil
	case "dark":
		return diagram.Dark, nil
	case "auto":
		if inTerminal && !terminal.LightBackground(getenv) {
			return diagram.Dark, nil
		}
		return diagram.Light, nil
	}
	return diagram.Palette{}, fmt.Errorf("-theme %q: use auto, light or dark", theme)
}

// writeFile writes the diagram to -out, in the format its name ends in.
func writeFile(o options, lay *diagram.Layout, getenv func(string) string) error {
	ext := strings.ToLower(filepath.Ext(o.out))
	if ext != ".png" && ext != ".svg" && ext != ".txt" {
		return fmt.Errorf("-out %q: name it .png, .svg or .txt", o.out)
	}
	pal, err := palette(o.theme, false, getenv)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	switch ext {
	case ".png":
		fonts, err := loadFonts(o.fonts)
		if err != nil {
			return err
		}
		img, err := picture.Draw(lay, picture.Options{Palette: pal, TextPx: o.textPx, Fonts: fonts})
		if err != nil {
			return err
		}
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
	case ".svg":
		if err := svg.Write(&buf, lay, svg.Options{Palette: pal, TextPx: o.textPx}); err != nil {
			return err
		}
	case ".txt":
		for _, l := range lay.Text() {
			buf.WriteString(l + "\n")
		}
	}
	return os.WriteFile(o.out, buf.Bytes(), 0o644)
}

// writeTerminal shows the diagram on out: a picture where terminal.Choose
// says the terminal shows one, and the text drawing otherwise.
func writeTerminal(o options, lay *diagram.Layout, out, errOut io.Writer, getenv func(string) string) error {
	mode := terminal.Protocol(o.terminal)
	if o.terminal == "text" {
		mode = terminal.Braille
	}
	f, isFile := out.(*os.File)
	ts, tty := terminal.Size{}, false
	if isFile {
		ts, tty = terminal.SizeOf(f)
	}
	how, note, err := terminal.Choose(mode, getenv, tty)
	if err != nil {
		return err
	}
	if note != "" {
		fmt.Fprintln(errOut, strings.Replace(note, "drawn in braille", "drawn as text", 1)+"; -terminal iterm or kitty draws one anyway")
	}
	if how == terminal.Braille {
		for _, l := range lay.Text() {
			fmt.Fprintln(out, l)
		}
		return nil
	}
	pal, err := palette(o.theme, true, getenv)
	if err != nil {
		return err
	}
	fonts, err := loadFonts(o.fonts)
	if err != nil {
		return err
	}
	// Drawn at twice the pixels its characters cover, so a screen of two
	// pixels to a point draws it sharp, the text at most twice its usual
	// size and no wider than the window: a small diagram is not blown up
	// to fill it. A terminal that does not say its size in pixels is taken
	// to have characters ten pixels wide.
	cellW := 10.0
	if ts.Width > 0 && ts.Cols > 0 {
		cellW = float64(ts.Width) / float64(ts.Cols)
	}
	cols := ts.Cols
	if cols <= 0 {
		cols = 100
	}
	px := 2.0 * picture.DefaultTextPx
	if w, _ := diagram.ScaleFor(px).Size(lay); w > 2*cellW*float64(cols) {
		px *= 2 * cellW * float64(cols) / w
	}
	img, err := picture.Draw(lay, picture.Options{Palette: pal, TextPx: px, Fonts: fonts})
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	shown := min(cols, int(math.Ceil(float64(img.Bounds().Dx())/(2*cellW))))
	return terminal.Write(out, how, buf.Bytes(), shown)
}

// loadFonts reads the fonts -font names.
func loadFonts(paths []string) ([]*opentype.Font, error) {
	var fonts []*opentype.Font
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("-font: %w", err)
		}
		f, err := opentype.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("-font %s: %w", p, err)
		}
		fonts = append(fonts, f)
	}
	return fonts, nil
}

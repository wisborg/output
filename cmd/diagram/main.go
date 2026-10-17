// Command diagram draws a diagram written in Graphviz's DOT language -- a
// flow chart, a database cluster and its replicas, a service and what it
// calls -- in the terminal, or as a PNG or SVG.
//
//	diagram cluster.dot
//	diagram --out cluster.svg cluster.dot
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
	"strconv"
	"strings"

	"golang.org/x/image/font/opentype"

	"github.com/wisborg/output/chart/terminal"
	"github.com/wisborg/output/diagram"
	"github.com/wisborg/output/diagram/dot"
	"github.com/wisborg/output/diagram/picture"
	"github.com/wisborg/output/diagram/svg"
	"github.com/wisborg/output/internal/cli"
)

func main() {
	cli.Exit("diagram", run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
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
  style=legend        on a cluster: the diagram's legend, drawn below it --
                      each node an entry, a swatch of its fill, or for
                      shape=plain a line in its style, beside its label

A cluster is a subgraph whose name begins with "cluster".

Flags:
`

type options struct {
	out      string
	terminal string
	theme    string
	color    string
	edges    string
	braille  string
	text     diagram.TextOptions
	width    int
	height   int
	textPx   float64
	fonts    []string
}

// The size of a PNG or an SVG where neither --width, --height nor
// --text-size says: as wide as plot's charts, so a diagram and a chart
// side by side on a page are drawn alike, and as tall as its proportions
// make it.
const imageWidth = 1600

// terminalMaxWidth is the widest picture drawn for the terminal, in
// pixels, as for plot: wider is seconds to draw and megabytes to send for
// a picture the terminal shows no sharper.
const terminalMaxWidth = 3200.0

func run(args []string, stdin io.Reader, stdout, stderr io.Writer, getenv func(string) string) error {
	var o options
	fs := flag.NewFlagSet("diagram", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		cli.PrintDefaults(fs)
	}
	fs.StringVar(&o.out, "out", "", "write the diagram to this file rather than the terminal: .png, .svg, or .txt for the text drawing")
	fs.StringVar(&o.terminal, "terminal", "auto", "how the diagram is shown in the terminal: auto (a picture where the terminal shows one, text otherwise), iterm, kitty or text")
	fs.StringVar(&o.color, "color", "auto", "colour in the text drawing: auto (in a terminal, unless NO_COLOR is set; not into a pipe or a .txt file), always or never")
	fs.StringVar(&o.edges, "edges", "auto", "how a PNG or an SVG draws edges: auto (as the file's splines attribute says, along rows and columns where it says nothing), straight (a straight line between the boxes, at whatever angle, where it is clear of the others) or orthogonal (along rows and columns); the text drawing draws them along rows and columns too, unless --braille says otherwise")
	fs.StringVar(&o.braille, "braille", "off", "how the text drawing draws the edges --edges straight, or the file's splines, draws straight: off (along rows and columns), thin or thick (at their angle, in braille dots one or two deep, as the picture draws them)")
	fs.StringVar(&o.theme, "theme", "auto", "the picture's palette: auto (light for a file; the terminal's own in a terminal), light or dark")
	fs.IntVar(&o.width, "width", 0, "the picture's width: in characters for a picture in the terminal (default: the terminal's, or 100); in pixels in a PNG or an SVG (default: "+strconv.Itoa(imageWidth)+"); its height follows from its proportions. The text drawing is as wide as the diagram, which no flag narrows")
	fs.IntVar(&o.height, "height", 0, "the most the diagram's height may be, narrower where it would be taller: in lines in the terminal (default: the terminal's, less one for the prompt); in pixels in a PNG or an SVG (default: none)")
	fs.Float64Var(&o.textPx, "text-size", 0, "size a PNG or an SVG by its text instead, in pixels, everything else in proportion -- so several diagrams share a text size; not with --width or --height")
	fs.Func("font", "a TrueType or OpenType font for the letters the built-in font lacks, in a PNG; repeat for several", func(s string) error {
		o.fonts = append(o.fonts, s)
		return nil
	})
	files, err := cli.Parse(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		// Asked for, so on standard output, where a pager or grep finds
		// it.
		fs.SetOutput(stdout)
		fs.Usage()
		return err
	}
	if err != nil {
		return err
	}
	if len(files) > 1 {
		return fmt.Errorf("one file at a time: %s", strings.Join(files, " "))
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for _, f := range []struct {
		name string
		v    float64
	}{{"width", float64(o.width)}, {"height", float64(o.height)}, {"text-size", o.textPx}} {
		if set[f.name] && !(f.v > 0 && f.v <= math.MaxFloat64) {
			return cli.Usagef("--%s %v: a size above 0", f.name, f.v)
		}
	}
	if set["text-size"] && (set["width"] || set["height"]) {
		return cli.Usagef("--text-size sizes the diagram by its text, --width and --height by its outline: give one or the other")
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
		return cli.Usagef("--terminal %q: use auto, iterm, kitty or text", o.terminal)
	}
	switch o.color {
	case "auto", "always", "never":
	default:
		return cli.Usagef("--color %q: use auto, always or never", o.color)
	}
	switch o.edges {
	case "auto", "straight", "orthogonal":
	default:
		return cli.Usagef("--edges %q: use auto, straight or orthogonal", o.edges)
	}
	textOpts := diagram.TextOptions{}
	switch o.braille {
	case "off":
	case "thin":
		textOpts.Braille = diagram.BrailleThin
	case "thick":
		textOpts.Braille = diagram.BrailleThick
	default:
		return cli.Usagef("--braille %q: use off, thin or thick", o.braille)
	}
	o.text = textOpts
	in, name := stdin, "standard input"
	if len(files) == 1 && files[0] != "-" {
		f, err := os.Open(files[0])
		if err != nil {
			return err
		}
		defer f.Close()
		in, name = f, files[0]
	}
	src, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	d, err := dot.Parse(string(src))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	switch o.edges {
	case "straight":
		d.Straight = true
	case "orthogonal":
		d.Straight = false
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

// palette is --theme's palette: for auto, light for a file, read on a page,
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
	return diagram.Palette{}, cli.Usagef("--theme %q: use auto, light or dark", theme)
}

// writeFile writes the diagram to --out, in the format its name ends in.
func writeFile(o options, lay *diagram.Layout, getenv func(string) string) error {
	ext := strings.ToLower(filepath.Ext(o.out))
	if ext != ".png" && ext != ".svg" && ext != ".txt" {
		return cli.Usagef("--out %q: name it .png, .svg or .txt", o.out)
	}
	pal, err := palette(o.theme, false, getenv)
	if err != nil {
		return err
	}
	px := o.textPx
	if px == 0 {
		w := float64(o.width)
		if w == 0 {
			w = imageWidth
		}
		px = textPxFor(lay, w, float64(o.height))
	}
	var buf bytes.Buffer
	switch ext {
	case ".png":
		fonts, err := loadFonts(o.fonts)
		if err != nil {
			return err
		}
		img, err := picture.Draw(lay, picture.Options{Palette: pal, TextPx: px, Fonts: fonts})
		if err != nil {
			return err
		}
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
	case ".svg":
		if err := svg.Write(&buf, lay, svg.Options{Palette: pal, TextPx: px}); err != nil {
			return err
		}
	case ".txt":
		// A file is read by more than terminals: in colour only when asked.
		lines := lay.Text(o.text)
		if o.color == "always" {
			lines = lay.ColourText(pal, o.text)
		}
		for _, l := range lines {
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
		fmt.Fprintln(errOut, strings.Replace(note, "drawn in braille", "drawn as text", 1)+"; --terminal iterm or kitty draws one anyway")
	}
	pal, err := palette(o.theme, true, getenv)
	if err != nil {
		return err
	}
	if how == terminal.Braille {
		if o.width > 0 && lay.Width > o.width {
			// The text drawing is the layout's cells, one a character:
			// it cannot be drawn narrower, and saying nothing left a
			// --width that had no effect looking as though it had.
			fmt.Fprintf(errOut, "the text drawing is %d characters wide, wider than --width %d: --width sizes a picture, and the text cannot be drawn narrower\n", lay.Width, o.width)
		}
		lines := lay.Text(o.text)
		if colour(o.color, tty, getenv) {
			lines = lay.ColourText(pal, o.text)
		}
		for _, l := range lines {
			fmt.Fprintln(out, l)
		}
		return nil
	}
	fonts, err := loadFonts(o.fonts)
	if err != nil {
		return err
	}
	// As wide as the window, or --width characters, and no taller than its
	// rows less one for the prompt, or --height lines: drawn at twice the
	// pixels those characters cover, so a screen of two pixels to a point
	// draws it sharp, held to terminalMaxWidth. A terminal that does not
	// say its size in pixels is taken to have characters ten by twenty.
	cellW, cellH := 10.0, 20.0
	if ts.Width > 0 && ts.Height > 0 && ts.Cols > 0 && ts.Rows > 0 {
		cellW, cellH = float64(ts.Width)/float64(ts.Cols), float64(ts.Height)/float64(ts.Rows)
	}
	cols, rows := ts.Cols, ts.Rows-1
	if cols <= 0 {
		cols = 100
	}
	if o.width > 0 {
		cols = o.width
	}
	if o.height > 0 {
		rows = o.height
	}
	scale := min(1, terminalMaxWidth/(2*cellW*float64(cols)))
	perCol := 2 * cellW * scale
	maxH := 0.0
	if rows > 0 {
		maxH = 2 * cellH * scale * float64(rows)
	}
	px := textPxFor(lay, perCol*float64(cols), maxH)
	img, err := picture.Draw(lay, picture.Options{Palette: pal, TextPx: px, Fonts: fonts})
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	shown := min(cols, int(math.Ceil(float64(img.Bounds().Dx())/perCol)))
	return terminal.Write(out, how, buf.Bytes(), shown)
}

// colour is whether the text drawing is in colour: as --color says, and
// for auto, in a terminal that has not been asked for none -- NO_COLOR set
// to anything, as no-color.org has it -- and is not a dumb one.
func colour(mode string, tty bool, getenv func(string) string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	}
	return tty && getenv("NO_COLOR") == "" && getenv("TERM") != "dumb"
}

// textPxFor is the text size that draws lay width pixels wide, or less
// where that would be taller than height; a height of 0 does not hold it.
// Everything in a picture is in proportion to its text, so the size is in
// proportion to the width.
func textPxFor(lay *diagram.Layout, width, height float64) float64 {
	w, h := diagram.ScaleFor(1).Size(lay)
	px := width / w
	if height > 0 {
		px = min(px, height/h)
	}
	// A hair under, so the picture rounds up to the width and not past it.
	return px * (1 - 1e-9)
}

// loadFonts reads the fonts --font names.
func loadFonts(paths []string) ([]*opentype.Font, error) {
	var fonts []*opentype.Font
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("--font: %w", err)
		}
		f, err := opentype.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("--font %s: %w", p, err)
		}
		fonts = append(fonts, f)
	}
	return fonts, nil
}

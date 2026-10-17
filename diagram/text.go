package diagram

import (
	"fmt"
	"image/color"
	"strings"
	"unicode/utf8"
)

// The directions a line leaves a cell in; a cell's box-drawing character is
// the one joining them.
const (
	up = 1 << iota
	down
	left
	right
)

// joins is the box-drawing character for each set of directions.
var joins = [16]rune{
	0: ' ', up: '│', down: '│', up | down: '│',
	left: '─', right: '─', left | right: '─',
	down | right: '┌', down | left: '┐', up | right: '└', up | left: '┘',
	up | down | right: '├', up | down | left: '┤', down | left | right: '┬', up | left | right: '┴',
	up | down | left | right: '┼',
}

// textCanvas is a page of characters a diagram is drawn on: lines kept as
// the directions they leave each cell in, so that two that meet are drawn
// meeting -- a line crossing a lane's line is a ┼, an edge leaving a box a ┬
// in its edge -- and what is written over them, with the colour of each
// cell's ink and of what is behind it.
type textCanvas struct {
	w, h   int
	lines  [][]uint8
	dashed [][]bool        // a cell whose lines are all dashed
	text   [][]rune        // written over the lines; 0 where nothing is
	fg, bg [][]color.Color // nil for the terminal's own
}

func newTextCanvas(w, h int) *textCanvas {
	c := &textCanvas{w: w, h: h}
	c.lines, c.dashed, c.text = make([][]uint8, h), make([][]bool, h), make([][]rune, h)
	c.fg, c.bg = make([][]color.Color, h), make([][]color.Color, h)
	for y := range h {
		c.lines[y], c.dashed[y], c.text[y] = make([]uint8, w), make([]bool, w), make([]rune, w)
		c.fg[y], c.bg[y] = make([]color.Color, w), make([]color.Color, w)
	}
	return c
}

func (c *textCanvas) in(x, y int) bool { return x >= 0 && y >= 0 && x < c.w && y < c.h }

// line draws a line from a to b, along a row or a column, in col; dashed
// marks the cells it alone crosses as dashed.
func (c *textCanvas) line(a, b Point, dashed bool, col color.Color) {
	dx, dy := sign(b.X-a.X), sign(b.Y-a.Y)
	if dx != 0 && dy != 0 {
		return
	}
	for p := a; ; p = (Point{p.X + dx, p.Y + dy}) {
		var bits uint8
		if p != a {
			bits |= dir(-dx, -dy)
		}
		if p != b {
			bits |= dir(dx, dy)
		}
		if c.in(p.X, p.Y) {
			fresh := c.lines[p.Y][p.X] == 0
			c.lines[p.Y][p.X] |= bits
			c.dashed[p.Y][p.X] = dashed && (fresh || c.dashed[p.Y][p.X])
			c.fg[p.Y][p.X] = col
		}
		if p == b {
			return
		}
	}
}

func dir(dx, dy int) uint8 {
	switch {
	case dx > 0:
		return right
	case dx < 0:
		return left
	case dy > 0:
		return down
	case dy < 0:
		return up
	}
	return 0
}

func sign(x int) int {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

// rect draws r's outline in col.
func (c *textCanvas) rect(r Rect, dashed bool, col color.Color) {
	x1, y1 := r.X+r.W-1, r.Y+r.H-1
	c.line(Point{r.X, r.Y}, Point{x1, r.Y}, dashed, col)
	c.line(Point{x1, r.Y}, Point{x1, y1}, dashed, col)
	c.line(Point{x1, y1}, Point{r.X, y1}, dashed, col)
	c.line(Point{r.X, y1}, Point{r.X, r.Y}, dashed, col)
}

// clear clears r's lines, as a box covers what passes behind it, and sets
// what is behind its cells to bg.
func (c *textCanvas) clear(r Rect, bg color.Color) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			if c.in(x, y) {
				c.lines[y][x], c.text[y][x], c.fg[y][x], c.bg[y][x] = 0, 0, nil, bg
			}
		}
	}
}

// fill sets what is behind r's cells to bg, leaving what is in them.
func (c *textCanvas) fill(r Rect, bg color.Color) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			if c.in(x, y) {
				c.bg[y][x] = bg
			}
		}
	}
}

// write writes s from (x, y) in col, over the lines.
func (c *textCanvas) write(x, y int, s string, col color.Color) {
	for _, r := range s {
		c.set(x, y, r, col)
		x++
	}
}

func (c *textCanvas) set(x, y int, r rune, col color.Color) {
	if c.in(x, y) {
		c.text[y][x], c.fg[y][x] = r, col
	}
}

// rows is the page, each line without the blank cells it ends in; with
// colour, in the terminal's escapes for 24-bit colour, each line ending
// with its colours reset.
func (c *textCanvas) rows(colour bool) []string {
	out := make([]string, c.h)
	for y := range c.h {
		end := c.w
		for end > 0 && c.text[y][end-1] == 0 && c.lines[y][end-1] == 0 && (!colour || c.bg[y][end-1] == nil) {
			end--
		}
		var b strings.Builder
		var fg, bg color.Color
		for x := range end {
			r := c.text[y][x]
			if r == 0 {
				r = joins[c.lines[y][x]]
				if c.dashed[y][x] {
					switch r {
					case '─':
						r = '╌'
					case '│':
						r = '╎'
					}
				}
			}
			if colour {
				f := c.fg[y][x]
				if r == ' ' {
					f = fg // a blank's ink does not show: no need to change it
				}
				if !same(f, fg) {
					b.WriteString(sgr(38, f))
					fg = f
				}
				if !same(c.bg[y][x], bg) {
					b.WriteString(sgr(48, c.bg[y][x]))
					bg = c.bg[y][x]
				}
			}
			b.WriteRune(r)
		}
		if colour && (fg != nil || bg != nil) {
			b.WriteString("\x1b[0m")
		}
		out[y] = b.String()
		if !colour {
			out[y] = strings.TrimRight(out[y], " ")
		}
	}
	return out
}

// sgr is the escape that sets the ink (layer 38) or what is behind it (48)
// to col, or back to the terminal's own where col is nil.
func sgr(layer int, col color.Color) string {
	if col == nil {
		return fmt.Sprintf("\x1b[%dm", layer+1)
	}
	n := color.NRGBAModel.Convert(col).(color.NRGBA)
	return fmt.Sprintf("\x1b[%d;2;%d;%d;%dm", layer, n.R, n.G, n.B)
}

func same(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	r1, g1, b1, a1 := a.RGBA()
	r2, g2, b2, a2 := b.RGBA()
	return r1 == r2 && g1 == g2 && b1 == b2 && a1 == a2
}

// Text is the diagram drawn in characters: each node a box of box-drawing
// lines around its label -- rounded at the corners for a rounded box, an
// ellipse or a cylinder, with a cylinder's lid a line beneath its top --
// each frame a box with its label set in its top edge, each lane set off by
// a line with its label at its side, and each edge a line from box to box
// with an arrowhead at the box it points to. Colours are not drawn: see
// ColourText.
func (lay *Layout) Text() []string { return lay.text(Light).rows(false) }

// ColourText is Text in colour, for a terminal: in the escapes for 24-bit
// colour that terminals read, each line ending with its colours reset. A
// node's fill is behind its inside, its outline and an edge are in their
// colours, frames, lanes and edges' labels in pal's Dim, and a label on a
// fill in the ink pal.TextOn gives it. What the diagram does not colour is
// left in the terminal's own colours, on its own background, so it reads
// the same in a light terminal and a dark one; pal is the one the terminal
// is closer to.
func (lay *Layout) ColourText(pal Palette) []string { return lay.text(pal).rows(true) }

// text draws lay on a page, with pal's colours where it uses them and nil
// for the terminal's own.
func (lay *Layout) text(pal Palette) *textCanvas {
	c := newTextCanvas(lay.Width, lay.Height)
	if lay.Title != "" {
		c.write(0, 0, lay.Title, nil)
	}
	for _, g := range lay.Groups {
		if g.Group.Fill != nil && g.Group.Style != Hidden {
			c.fill(g.Rect, over(g.Group.Fill, pal.Background))
		}
		col := g.Group.Colour
		if col == nil {
			col = pal.Dim
		}
		for _, d := range g.Dividers {
			c.line(d[0], d[1], false, col)
		}
		if g.Group.Style == Frame {
			c.rect(g.Rect, false, col)
		}
	}
	for _, e := range lay.Edges {
		if e.Edge.Invisible {
			continue
		}
		for i := 1; i < len(e.Points); i++ {
			c.line(e.Points[i-1], e.Points[i], e.Edge.Line == Dashed || e.Edge.Line == Dotted, e.Edge.Colour)
		}
	}
	for _, n := range lay.Nodes {
		if n.Node.Invisible {
			continue
		}
		drawTextNode(c, n, pal)
	}
	// The labels and arrowheads last, over everything.
	for _, g := range lay.Groups {
		label := g.Group.Label
		col := g.Group.Colour
		if col == nil {
			col = pal.Dim
		}
		switch {
		case label == "":
		case g.Group.Style == Frame:
			c.write(g.LabelAt.X, g.LabelAt.Y, " "+label+" ", col)
		case g.Group.Style == Lane:
			c.write(g.LabelAt.X, g.LabelAt.Y, label, col)
		}
	}
	for _, e := range lay.Edges {
		if e.Edge.Invisible || len(e.Points) < 2 {
			continue
		}
		n := len(e.Points)
		if e.Edge.Head {
			arrow(c, e.Points[n-2], e.Points[n-1], e.Edge.Colour)
		}
		if e.Edge.Tail {
			arrow(c, e.Points[1], e.Points[0], e.Edge.Colour)
		}
		for i, line := range strings.Split(e.Edge.Label, "\n") {
			if e.Edge.Label != "" {
				c.write(e.LabelAt.X, e.LabelAt.Y+i, line, pal.Dim)
			}
		}
	}
	return c
}

// over is col as it shows over bg: a translucent fill mixed with what is
// behind it, as a terminal's cell can only be one colour. With no bg, it
// is col as it is.
func over(col, bg color.Color) color.Color {
	_, _, _, a := col.RGBA()
	if bg == nil || a == 0xffff {
		return col
	}
	r, g, b, _ := col.RGBA()
	br, bgG, bb, _ := bg.RGBA()
	mix := func(c, under uint32) uint8 { return uint8((c + under*(0xffff-a)/0xffff) >> 8) }
	return color.RGBA{mix(r, br), mix(g, bgG), mix(b, bb), 0xff}
}

// arrow draws an arrowhead in the cell before to, on the line from from,
// pointing at to, in col: the cell outside the box the line ends on. The
// box's edge there is left plain, as the arrow points into it, where the
// edge of the box a line leaves shows a tee.
func arrow(c *textCanvas, from, to Point, col color.Color) {
	dx, dy := sign(to.X-from.X), sign(to.Y-from.Y)
	at := Point{to.X - dx, to.Y - dy}
	if c.in(to.X, to.Y) {
		c.lines[to.Y][to.X] &^= dir(-dx, -dy)
	}
	switch {
	case dy > 0:
		c.set(at.X, at.Y, '▼', col)
	case dy < 0:
		c.set(at.X, at.Y, '▲', col)
	case dx > 0:
		c.set(at.X, at.Y, '▶', col)
	case dx < 0:
		c.set(at.X, at.Y, '◀', col)
	}
}

// drawTextNode draws n's outline, over anything passing behind it, its
// fill behind its inside, and its label in the middle.
func drawTextNode(c *textCanvas, n PlacedNode, pal Palette) {
	r, node := n.Rect, n.Node
	// The lines of edges that end on the outline are kept, so the outline
	// shows where each leaves; the inside is cleared.
	edgeBits := make(map[Point]uint8)
	for x := r.X; x < r.X+r.W; x++ {
		for _, y := range []int{r.Y, r.Y + r.H - 1} {
			if c.in(x, y) {
				edgeBits[Point{x, y}] = c.lines[y][x]
			}
		}
	}
	for y := r.Y; y < r.Y+r.H; y++ {
		for _, x := range []int{r.X, r.X + r.W - 1} {
			if c.in(x, y) {
				edgeBits[Point{x, y}] = c.lines[y][x]
			}
		}
	}
	// Behind the box, its own background, as in a picture, so a group's
	// fill does not show through a node without one.
	c.clear(r, nil)
	lines := strings.Split(node.label(), "\n")
	top := r.Y + 1
	ink := node.Text
	if node.Shape != Plain {
		if node.Fill != nil {
			fill := over(node.Fill, pal.Background)
			c.fill(Rect{r.X + 1, r.Y + 1, r.W - 2, r.H - 2}, fill)
			if ink == nil {
				ink = pal.TextOn(fill)
			}
		}
		c.rect(r, node.Line == Dashed || node.Line == Dotted, node.Colour)
		// Keep each edge's way out of the box: the directions pointing
		// out of it.
		for p, bits := range edgeBits {
			out := uint8(0)
			switch {
			case p.Y == r.Y:
				out = bits & up
			case p.Y == r.Y+r.H-1:
				out = bits & down
			}
			switch {
			case p.X == r.X:
				out |= bits & left
			case p.X == r.X+r.W-1:
				out |= bits & right
			}
			c.lines[p.Y][p.X] |= out
		}
		if node.Shape == Rounded || node.Shape == Ellipse || node.Shape == Cylinder {
			x1, y1 := r.X+r.W-1, r.Y+r.H-1
			c.set(r.X, r.Y, '╭', node.Colour)
			c.set(x1, r.Y, '╮', node.Colour)
			c.set(r.X, y1, '╰', node.Colour)
			c.set(x1, y1, '╯', node.Colour)
		}
		if node.Shape == Cylinder {
			c.line(Point{r.X, r.Y + 1}, Point{r.X + r.W - 1, r.Y + 1}, false, node.Colour)
			top++
		}
	} else {
		top = r.Y
	}
	for i, line := range lines {
		c.write(r.X+(r.W-utf8.RuneCountInString(line))/2, top+i, line, ink)
	}
}

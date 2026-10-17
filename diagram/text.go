package diagram

import (
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
// in its edge -- and what is written over them.
type textCanvas struct {
	w, h   int
	lines  [][]uint8
	dashed [][]bool // a cell whose lines are all dashed
	text   [][]rune // written over the lines; 0 where nothing is
}

func newTextCanvas(w, h int) *textCanvas {
	c := &textCanvas{w: w, h: h}
	c.lines, c.dashed, c.text = make([][]uint8, h), make([][]bool, h), make([][]rune, h)
	for y := range h {
		c.lines[y], c.dashed[y], c.text[y] = make([]uint8, w), make([]bool, w), make([]rune, w)
	}
	return c
}

func (c *textCanvas) in(x, y int) bool { return x >= 0 && y >= 0 && x < c.w && y < c.h }

// line draws a line from a to b, along a row or a column; dashed marks the
// cells it alone crosses as dashed.
func (c *textCanvas) line(a, b Point, dashed bool) {
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

// rect draws r's outline.
func (c *textCanvas) rect(r Rect, dashed bool) {
	x1, y1 := r.X+r.W-1, r.Y+r.H-1
	c.line(Point{r.X, r.Y}, Point{x1, r.Y}, dashed)
	c.line(Point{x1, r.Y}, Point{x1, y1}, dashed)
	c.line(Point{x1, y1}, Point{r.X, y1}, dashed)
	c.line(Point{r.X, y1}, Point{r.X, r.Y}, dashed)
}

// clear clears r's lines, as a box covers what passes behind it.
func (c *textCanvas) clear(r Rect) {
	for y := r.Y; y < r.Y+r.H; y++ {
		for x := r.X; x < r.X+r.W; x++ {
			if c.in(x, y) {
				c.lines[y][x], c.text[y][x] = 0, 0
			}
		}
	}
}

// write writes s from (x, y), over the lines; where blank only, it leaves
// cells holding a line or text alone, as an edge's label must not cut its
// edge.
func (c *textCanvas) write(x, y int, s string, blankOnly bool) {
	for _, r := range s {
		if c.in(x, y) && (!blankOnly || c.lines[y][x] == 0 && c.text[y][x] == 0) {
			c.text[y][x] = r
		}
		x++
	}
}

func (c *textCanvas) set(x, y int, r rune) {
	if c.in(x, y) {
		c.text[y][x] = r
	}
}

// String is the page, each line without the spaces it ends in.
func (c *textCanvas) rows() []string {
	out := make([]string, c.h)
	for y := range c.h {
		var b strings.Builder
		for x := range c.w {
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
			b.WriteRune(r)
		}
		out[y] = strings.TrimRight(b.String(), " ")
	}
	return out
}

// Text is the diagram drawn in characters: each node a box of box-drawing
// lines around its label -- rounded at the corners for a rounded box, an
// ellipse or a cylinder, with a cylinder's lid a line beneath its top --
// each frame a box with its label set in its top edge, each lane set off by
// a line with its label at its side, and each edge a line from box to box
// with an arrowhead at the box it points to. Colours are not drawn.
func (lay *Layout) Text() []string {
	c := newTextCanvas(lay.Width, lay.Height)
	if lay.Title != "" {
		c.write(0, 0, lay.Title, false)
	}
	for _, g := range lay.Groups {
		for _, d := range g.Dividers {
			c.line(d[0], d[1], false)
		}
		if g.Group.Style == Frame {
			c.rect(g.Rect, false)
		}
	}
	for _, e := range lay.Edges {
		if e.Edge.Invisible {
			continue
		}
		for i := 1; i < len(e.Points); i++ {
			c.line(e.Points[i-1], e.Points[i], e.Edge.Line == Dashed || e.Edge.Line == Dotted)
		}
	}
	for _, n := range lay.Nodes {
		if n.Node.Invisible {
			continue
		}
		drawTextNode(c, n)
	}
	// The labels and arrowheads last, over everything.
	for _, g := range lay.Groups {
		label := g.Group.Label
		switch {
		case label == "":
		case g.Group.Style == Frame:
			c.write(g.LabelAt.X, g.LabelAt.Y, " "+label+" ", false)
		case g.Group.Style == Lane:
			c.write(g.LabelAt.X, g.LabelAt.Y, label, false)
		}
	}
	for _, e := range lay.Edges {
		if e.Edge.Invisible || len(e.Points) < 2 {
			continue
		}
		n := len(e.Points)
		if e.Edge.Head {
			arrow(c, e.Points[n-2], e.Points[n-1])
		}
		if e.Edge.Tail {
			arrow(c, e.Points[1], e.Points[0])
		}
		for i, line := range strings.Split(e.Edge.Label, "\n") {
			if e.Edge.Label != "" {
				c.write(e.LabelAt.X, e.LabelAt.Y+i, line, false)
			}
		}
	}
	return c.rows()
}

// arrow draws an arrowhead in the cell before to, on the line from from,
// pointing at to: the cell outside the box the line ends on. The box's edge
// there is left plain, as the arrow points into it, where the edge of the
// box a line leaves shows a tee.
func arrow(c *textCanvas, from, to Point) {
	dx, dy := sign(to.X-from.X), sign(to.Y-from.Y)
	at := Point{to.X - dx, to.Y - dy}
	if c.in(to.X, to.Y) {
		c.lines[to.Y][to.X] &^= dir(-dx, -dy)
	}
	switch {
	case dy > 0:
		c.set(at.X, at.Y, '▼')
	case dy < 0:
		c.set(at.X, at.Y, '▲')
	case dx > 0:
		c.set(at.X, at.Y, '▶')
	case dx < 0:
		c.set(at.X, at.Y, '◀')
	}
}

// drawTextNode draws n's outline, over anything passing behind it, and its
// label in the middle.
func drawTextNode(c *textCanvas, n PlacedNode) {
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
	c.clear(r)
	lines := strings.Split(node.label(), "\n")
	top := r.Y + 1
	if node.Shape != Plain {
		c.rect(r, node.Line == Dashed || node.Line == Dotted)
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
			c.set(r.X, r.Y, '╭')
			c.set(x1, r.Y, '╮')
			c.set(r.X, y1, '╰')
			c.set(x1, y1, '╯')
		}
		if node.Shape == Cylinder {
			c.line(Point{r.X, r.Y + 1}, Point{r.X + r.W - 1, r.Y + 1}, false)
			top++
		}
	} else {
		top = r.Y
	}
	for i, line := range lines {
		c.write(r.X+(r.W-utf8.RuneCountInString(line))/2, top+i, line, false)
	}
}

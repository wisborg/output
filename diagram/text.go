package diagram

import (
	"fmt"
	"image/color"
	"math"
	"sort"
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
	dots   [][]uint8       // a straight edge's braille dots, where no line or text is
	merged [][]bool        // a cell where a straight edge crosses a line, both drawn in dots
	thick  bool            // braille two dots deep
	fg, bg [][]color.Color // nil for the terminal's own
}

func newTextCanvas(w, h int) *textCanvas {
	c := &textCanvas{w: w, h: h}
	c.lines, c.dashed, c.text = make([][]uint8, h), make([][]bool, h), make([][]rune, h)
	c.fg, c.bg, c.dots, c.merged = make([][]color.Color, h), make([][]color.Color, h), make([][]uint8, h), make([][]bool, h)
	for y := range h {
		c.dots[y], c.merged[y] = make([]uint8, w), make([]bool, w)
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
		for end > 0 && c.text[y][end-1] == 0 && c.lines[y][end-1] == 0 && c.dots[y][end-1] == 0 && (!colour || c.bg[y][end-1] == nil) {
			end--
		}
		var b strings.Builder
		var fg, bg color.Color
		for x := range end {
			r := c.text[y][x]
			if r == 0 && (c.lines[y][x] == 0 || c.merged[y][x]) && c.dots[y][x] != 0 {
				r = rune(0x2800) + rune(c.dots[y][x])
			}
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
// ColourText. An edge a picture draws straight is drawn along its path,
// along rows and columns, unless o asks for braille.
func (lay *Layout) Text(o ...TextOptions) []string {
	return lay.trimTop(lay.text(Light, opts(o)), opts(o), false)
}

// trimTop is c's rows, without those at the top the layout grew for labels
// only a picture, or a drawing in braille, writes there, where they are
// empty: the text drawing along rows and columns writes its labels by its
// own lines, and would start with blank lines.
func (lay *Layout) trimTop(c *textCanvas, o TextOptions, colour bool) []string {
	rows := c.rows(colour)
	if o.Braille != BrailleOff {
		return rows
	}
	k := 0
	for k < lay.pictureTop && k < c.h {
		blank := true
		for x := range c.w {
			blank = blank && c.text[k][x] == 0 && c.lines[k][x] == 0 && c.dots[k][x] == 0 && c.bg[k][x] == nil
		}
		if !blank {
			break
		}
		k++
	}
	return rows[k:]
}

// TextOptions are how Text and ColourText draw what they may draw more
// than one way.
type TextOptions struct {
	// Braille draws the edges a picture draws straight as the picture
	// does, at whatever angle, in braille dots, rather than along rows and
	// columns.
	Braille Braille
}

// Braille is whether, and how heavily, the text drawing draws straight
// edges in braille dots.
type Braille int

const (
	// BrailleOff draws them along rows and columns, as every other edge.
	BrailleOff Braille = iota
	// BrailleThin draws them a dot deep: the lighter line, closer to the
	// box-drawing lines beside it.
	BrailleThin
	// BrailleThick draws them two dots deep: a stroke, firmer where a
	// terminal's font draws its dots small.
	BrailleThick
)

func opts(o []TextOptions) TextOptions {
	if len(o) == 0 {
		return TextOptions{}
	}
	return o[0]
}

// ColourText is Text in colour, for a terminal: in the escapes for 24-bit
// colour that terminals read, each line ending with its colours reset. A
// node's fill is behind its inside, its outline and an edge are in their
// colours, frames, lanes and edges' labels in pal's Dim, and a label on a
// fill in the ink pal.TextOn gives it. What the diagram does not colour is
// left in the terminal's own colours, on its own background, so it reads
// the same in a light terminal and a dark one; pal is the one the terminal
// is closer to.
func (lay *Layout) ColourText(pal Palette, o ...TextOptions) []string {
	return lay.trimTop(lay.text(pal, opts(o)), opts(o), true)
}

// text draws lay on a page, with pal's colours where it uses them and nil
// for the terminal's own.
func (lay *Layout) text(pal Palette, o TextOptions) *textCanvas {
	c := newTextCanvas(lay.Width, lay.Height)
	c.thick = o.Braille == BrailleThick
	// Whether e is drawn straight here, in braille.
	straight := func(e PlacedEdge) bool { return e.Straight && o.Braille != BrailleOff }
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
		if e.Edge.Invisible || straight(e) {
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
	for _, e := range lay.Legend {
		drawTextEntry(c, e, pal)
	}
	// Straight edges in braille, in the cells the lines, the boxes and
	// their words leave free.
	var heads []func()
	for _, e := range lay.Edges {
		if straight(e) && !e.Edge.Invisible {
			heads = append(heads, drawTextStraight(c, lay, e))
		}
	}
	// Their arrowheads once every line is down, to keep them off all.
	for _, h := range heads {
		h()
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
		if !straight(e) && e.Edge.Head {
			arrow(c, e.Points[n-2], e.Points[n-1], e.Edge.Colour)
		}
		if !straight(e) && e.Edge.Tail {
			arrow(c, e.Points[1], e.Points[0], e.Edge.Colour)
		}
		// With braille, the lines are the picture's, and so are the
		// places of the labels, which are placed against them.
		at := e.LabelAt
		if o.Braille != BrailleOff {
			at = e.PictureLabelAt()
		}
		for i, line := range strings.Split(e.Edge.Label, "\n") {
			if e.Edge.Label != "" {
				c.write(at.X, at.Y+i, line, pal.Dim)
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

// drawTextEntry draws a legend entry: a stroke as a line, dashed where it
// is dashed or dotted, or a swatch as a block in its fill -- an empty box
// where it has none -- and its label after it.
func drawTextEntry(c *textCanvas, pe PlacedEntry, pal Palette) {
	e, r := pe.Entry, pe.Sample
	switch {
	case e.Sample == Stroke:
		c.line(Point{r.X, r.Y}, Point{r.X + r.W - 1, r.Y}, e.Line == Dashed || e.Line == Dotted, e.Colour)
	case e.Fill != nil:
		c.write(r.X, r.Y, strings.Repeat("█", r.W), over(e.Fill, pal.Background))
	default:
		c.write(r.X, r.Y, "["+strings.Repeat(" ", r.W-2)+"]", e.Colour)
	}
	for i, line := range strings.Split(e.Label, "\n") {
		c.write(pe.LabelAt.X, pe.LabelAt.Y+i, line, nil)
	}
}

// drawTextStraight draws a straight edge as the pictures draw it: a line
// at whatever angle from box to box, in braille dots -- two across and four
// down a cell, as plot draws a chart -- one dot for each step along its
// longer way so they are evenly spaced, a dot deep or, thick, two. A cell
// a box or a word holds keeps it, the edge passing under there. So does a
// line it crosses at a shallow angle; one it crosses steeply is drawn in
// dots with it, as a bar through the cell, so neither breaks. A dashed or
// a dotted edge leaves gaps between its dots.
//
// A line along one row or one column of cells is drawn as the edges along
// rows and columns are, which reads more firmly than a row of dots.
//
// At its head, and its tail where that has an arrowhead, it meets its box
// by a stub of one cell straight into the side it reaches, carrying the
// arrowhead the edges along rows and columns end in: an arrow at an angle
// in one cell is small, and lines up with the dots before it only by
// chance. The stub takes the free cell along that side nearest where the
// line arrives, and the line is aimed at it, so the two meet; where the
// side has none free, the arrow is drawn at an angle after all. The
// pictures draw the line straight to the box; the text bends at the end.
// It all happens in the func it returns, once every line is down, so the
// stubs keep clear of all of them.
func drawTextStraight(c *textCanvas, lay *Layout, e PlacedEdge) func() {
	x0, y0, x1, y1 := lay.ends(e)
	from, _ := lay.Node(e.Edge.From)
	to, _ := lay.Node(e.Edge.To)
	inBox := func(x, y int) bool {
		for _, n := range lay.Nodes {
			r := n.Rect
			if !n.Node.Invisible && x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
				return true
			}
		}
		return false
	}
	// In dots: a cell's middle is between its two columns of dots and its
	// second and third rows.
	dotsOf := func(x0, y0, x1, y1 float64) ([]Point, map[Point]uint8) {
		return brailleLine(c, 2*x0+1, 4*y0+2, 2*x1+1, 4*y1+2, e.Edge.Line, inBox)
	}
	cells, _ := dotsOf(x0, y0, x1, y1)
	if len(cells) == 0 {
		return func() {}
	}
	first, last := cells[0], cells[len(cells)-1]
	if first.X == last.X || first.Y == last.Y {
		c.line(first, last, e.Edge.Line == Dashed || e.Edge.Line == Dotted, e.Edge.Colour)
		return func() { straightHeads(c, e, cells, x1-x0, y1-y0) }
	}
	return func() {
		// A cell is free for a stub where no line, word or box is.
		free := func(p Point) bool {
			return c.in(p.X, p.Y) && c.lines[p.Y][p.X] == 0 && c.text[p.Y][p.X] == 0 && !inBox(p.X, p.Y)
		}
		headAt, headArrow, headOK := stub(to.Rect, last, x1-x0, y1-y0, free)
		if headOK {
			c.text[headAt.Y][headAt.X] = headArrow // for the tail to keep clear of
		}
		// A tail has a stub only where it has an arrowhead to carry: a stub
		// alone would add a line beside the others and say nothing.
		tailAt, tailArrow, tailOK := Point{}, rune(0), false
		if e.Edge.Tail {
			tailAt, tailArrow, tailOK = stub(from.Rect, first, x0-x1, y0-y1, free)
		}
		if headOK {
			c.text[headAt.Y][headAt.X] = 0
		}
		// The line runs from stub to stub, where a box's side had a cell
		// free for them, so it meets each: from the middle of the one
		// cell to the middle of the other.
		sx0, sy0, sx1, sy1 := x0, y0, x1, y1
		if tailOK {
			sx0, sy0 = float64(tailAt.X), float64(tailAt.Y)
		}
		if headOK {
			sx1, sy1 = float64(headAt.X), float64(headAt.Y)
		}
		cells, dots := dotsOf(sx0, sy0, sx1, sy1)
		// Where it crosses a line along a row or a column, both are drawn
		// in dots, the line as a bar through the cell where it runs, so
		// neither breaks -- where it crosses steeply.
		// A shallower one runs along the other line's cells for longer, and the dots
		// of the two would be a smudge in which neither could be followed:
		// there the line keeps its cells and this one passes under. A
		// corner or a junction keeps its character, which says more.
		bar := func(p Point) uint8 {
			switch c.lines[p.Y][p.X] {
			case left | right:
				return brailleDot(0, 1) | brailleDot(1, 1)
			case up | down:
				return brailleDot(0, 0) | brailleDot(0, 1) | brailleDot(0, 2) | brailleDot(0, 3)
			}
			return 0
		}
		// Steep enough, as drawn, against a line along a row, or one down
		// a column: more than about 40 degrees to it, where it crosses a
		// row in three cells or so -- a row is tall and a cell narrow, and
		// at 30 degrees it runs along five or six.
		ax, ay := math.Abs(sx1-sx0)*cellAspect, math.Abs(sy1-sy0)
		steepToRow, steepToCol := ay > ax*0.84, ax > ay*0.84
		merge := map[Point]bool{}
		for _, p := range cells {
			switch c.lines[p.Y][p.X] {
			case left | right:
				merge[p] = steepToRow
			case up | down:
				merge[p] = steepToCol
			}
		}
		for _, p := range cells {
			if p == headAt && headOK || p == tailAt && tailOK {
				continue
			}
			if c.text[p.Y][p.X] != 0 {
				continue // under a word or an arrowhead
			}
			if c.lines[p.Y][p.X] != 0 {
				if !merge[p] {
					continue // under the line
				}
				c.merged[p.Y][p.X] = true
				c.dots[p.Y][p.X] |= bar(p)
			}
			c.dots[p.Y][p.X] |= dots[p]
			c.fg[p.Y][p.X] = e.Edge.Colour
		}
		if len(cells) > 0 {
			first, last = cells[0], cells[len(cells)-1]
		}
		ends := []struct {
			at     Point
			arrow  rune
			ok     bool
			head   bool
			end    Point
			dx, dy float64
		}{
			{headAt, headArrow, headOK, e.Edge.Head, last, sx1 - sx0, sy1 - sy0},
			{tailAt, tailArrow, tailOK, e.Edge.Tail, first, sx0 - sx1, sy0 - sy1},
		}
		for _, s := range ends {
			switch {
			case !s.ok && s.head:
				// No free cell by the side: the arrow at an angle, where
				// the line ends.
				c.set(s.end.X, s.end.Y, pointing(s.dx*cellAspect, s.dy), e.Edge.Colour)
			case !s.ok:
			case s.head:
				c.set(s.at.X, s.at.Y, s.arrow, e.Edge.Colour)
			case s.arrow == '▲' || s.arrow == '▼':
				c.set(s.at.X, s.at.Y, '│', e.Edge.Colour)
			default:
				c.set(s.at.X, s.at.Y, '─', e.Edge.Colour)
			}
		}
	}
}

// brailleLine is the line from dot (u0, v0) to dot (u1, v1) in braille: the
// cells it crosses that no box holds, in order, and each one's dots -- one
// for each step along its longer way, so they are evenly spaced, and two
// deep where the canvas is thick, so it reads as a stroke; with gaps for a
// dashed or a dotted line.
func brailleLine(c *textCanvas, u0, v0, u1, v1 float64, line Line, inBox func(x, y int) bool) ([]Point, map[Point]uint8) {
	n := int(math.Ceil(math.Max(math.Abs(u1-u0), math.Abs(v1-v0))))
	flat := math.Abs(u1-u0) >= math.Abs(v1-v0)
	var cells []Point
	dots := map[Point]uint8{}
	put := func(u, v int) {
		q := Point{u / 2, v / 4}
		if u < 0 || v < 0 || !c.in(q.X, q.Y) || inBox(q.X, q.Y) {
			return
		}
		if _, ok := dots[q]; !ok {
			cells = append(cells, q)
		}
		dots[q] |= brailleDot(u%2, v%4)
	}
	for k := 0; k <= n; k++ {
		switch line {
		case Dashed:
			if k%5 >= 3 {
				continue
			}
		case Dotted:
			if k%3 >= 1 {
				continue
			}
		}
		t := float64(k) / float64(max(n, 1))
		iu, iv := int(math.Floor(u0+t*(u1-u0))), int(math.Floor(v0+t*(v1-v0)))
		put(iu, iv)
		if c.thick {
			if flat {
				put(iu, iv+1)
			} else {
				put(iu+1, iv)
			}
		}
	}
	return cells, dots
}

// straightHeads draws the arrowheads of a straight edge along one row or
// one column of cells, from cells' first to its last, pointing (dx, dy)
// in cells.
func straightHeads(c *textCanvas, e PlacedEdge, cells []Point, dx, dy float64) {
	first, last := cells[0], cells[len(cells)-1]
	if e.Edge.Head {
		c.set(last.X, last.Y, pointing(dx, dy), e.Edge.Colour)
	}
	if e.Edge.Tail {
		c.set(first.X, first.Y, pointing(-dx, -dy), e.Edge.Colour)
	}
}

// stub is the cell a straight line, reaching box r at cell near and
// heading (dx, dy) in cells, ends in by r's side, and the arrowhead into
// r there: by the side near is beyond, or where it is beyond two, a
// corner, the one the line runs more towards as drawn; the free cell
// along that side, off its corners, nearest near. False where the side
// has none free.
func stub(r Rect, near Point, dx, dy float64, free func(Point) bool) (Point, rune, bool) {
	left, right := near.X < r.X, near.X > r.X+r.W-1
	above, below := near.Y < r.Y, near.Y > r.Y+r.H-1
	across := math.Abs(dx)*cellAspect >= math.Abs(dy)
	var cells []Point
	var arrow rune
	down := func(x int) {
		for y := r.Y + 1; y <= r.Y+r.H-2; y++ {
			cells = append(cells, Point{x, y})
		}
	}
	along := func(y int) {
		for x := r.X + 1; x <= r.X+r.W-2; x++ {
			cells = append(cells, Point{x, y})
		}
	}
	switch {
	case left && (across || !above && !below):
		down(r.X - 1)
		arrow = '▶'
	case right && (across || !above && !below):
		down(r.X + r.W)
		arrow = '◀'
	case above:
		along(r.Y - 1)
		arrow = '▼'
	default:
		along(r.Y + r.H)
		arrow = '▲'
	}
	sort.SliceStable(cells, func(i, j int) bool {
		return abs(cells[i].X-near.X)+abs(cells[i].Y-near.Y) < abs(cells[j].X-near.X)+abs(cells[j].Y-near.Y)
	})
	for _, p := range cells {
		if free(p) {
			return p, arrow, true
		}
	}
	return Point{}, arrow, false
}

// brailleDot is the bit of a braille character for the dot in column col
// (0 or 1) and row row (0 to 3): the first three rows of each column are
// the low bits, the fourth row came later and has the two high ones.
func brailleDot(col, row int) uint8 {
	if row == 3 {
		return 0x40 << col
	}
	return 1 << (row + 3*col)
}

// pointing is the arrow nearest the way (dx, dy) points, y down: one of
// the four triangles the edges along rows and columns end in, or a
// diagonal arrow between them.
func pointing(dx, dy float64) rune {
	sector := int(math.Round(math.Atan2(dy, dx)/(math.Pi/4))+8) % 8
	return []rune{'▶', '↘', '▼', '↙', '◀', '↖', '▲', '↗'}[sector]
}

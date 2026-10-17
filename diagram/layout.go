package diagram

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// The spaces a layout leaves, in cells: between the columns of a grid or the
// items of a layer, between its rows or layers -- an odd number, so a lane's
// line runs down the middle of the gap -- and inside a frame.
const (
	colGap       = 6
	rowGap       = 3
	frameSide    = 3 // the edge, and two spaces
	frameTop     = 2 // the edge, and a blank line
	laneLabelGap = 4
)

// Rect is a rectangle of cells: X and Y its top-left cell, W and H how many
// cells across and down. A node's outline is drawn in its outermost cells.
type Rect struct{ X, Y, W, H int }

// Point is a cell.
type Point struct{ X, Y int }

// Layout is a diagram laid out: where each node, group and edge is drawn,
// in cells. A renderer only turns it into marks.
type Layout struct {
	// Width and Height are the cells the diagram takes, its title
	// included.
	Width, Height int
	Title         string
	Nodes         []PlacedNode
	Groups        []PlacedGroup
	Edges         []PlacedEdge
	// Legend is the legend's entries, its frame being the last of Groups.
	Legend []PlacedEntry
	// pictureTop is the rows the layout grew at the top for labels only a
	// picture with straight edges writes there.
	pictureTop int
	nodeAt     map[string]PlacedNode
}

// PlacedEntry is a legend entry and where it is drawn: its sample, one row
// tall, and its label after it.
type PlacedEntry struct {
	Entry   *LegendEntry
	Sample  Rect
	LabelAt Point
}

// Node is where the node id is placed.
func (lay *Layout) Node(id string) (PlacedNode, bool) {
	n, ok := lay.nodeAt[id]
	return n, ok
}

// PlacedNode is a node and where it is drawn.
type PlacedNode struct {
	Node *Node
	Rect Rect
}

// PlacedGroup is a group and where it is drawn: a frame's outline, or a
// lane's band.
type PlacedGroup struct {
	Group *Group
	Rect  Rect
	// LabelAt is where a frame's label begins, in its top edge, clear of
	// where edges cross it; or where a lane's ends, on its middle row.
	LabelAt Point
	// Dividers are the lines setting a lane off from the lanes before it:
	// each from its first cell to its last, along a row or a column.
	Dividers [][2]Point
}

// PlacedEdge is an edge and the path it is drawn along: from a cell of its
// tail node's outline, through the corners it turns at, to a cell of its
// head node's, every segment along a row or a column.
type PlacedEdge struct {
	Edge   *Edge
	Points []Point
	// LabelAt is where the label begins, on the row it is written on.
	LabelAt Point
	// Straight says a picture draws the edge as one straight line between
	// its nodes rather than along Points: the diagram asks for straight
	// edges, and the line is clear of every other box. Shift is how far
	// to one side the line is drawn, in rows, so that several between
	// the same two nodes are drawn apart. The text drawing draws Points,
	// unless it is asked for braille.
	Straight bool
	Shift    float64
	// pictureAt, where pictured, is where a picture writes the label: the
	// picture of a diagram with straight edges draws other lines than the
	// text drawing does, and its labels are placed against those.
	pictureAt Point
	pictured  bool
	// nudge is how far, in cells across and rows down, a picture moves
	// the label from pictureAt: a label on its own straight line is
	// centred on it exactly, which whole cells cannot do.
	nudge [2]float64
}

// PictureLabelNudge is how far a picture moves the label from
// PictureLabelAt, in cells across and rows down: a fraction of one, for a
// label written on its own straight line, centred on it.
func (e PlacedEdge) PictureLabelNudge() (dx, dy float64) { return e.nudge[0], e.nudge[1] }

// PictureLabelAt is where a picture writes the edge's label: as the text
// drawing does, unless the diagram has straight edges, when every label is
// placed against the lines the picture draws.
func (e PlacedEdge) PictureLabelAt() Point {
	if e.pictured {
		return e.pictureAt
	}
	return e.LabelAt
}

// size is the cells a node or a group takes.
type size struct{ w, h int }

// nodeSize is the cells n takes: its label's lines, inside its outline.
func nodeSize(n *Node) size {
	lines := strings.Split(n.label(), "\n")
	w := 0
	for _, l := range lines {
		w = max(w, Width(l))
	}
	h := len(lines)
	switch n.Shape {
	case Plain:
		return size{max(w, 1), h}
	case Ellipse:
		return size{w + 6, h + 2}
	case Cylinder:
		return size{w + 4, h + 3}
	}
	return size{w + 4, h + 2}
}

// plan is how a group lays out what it holds, before it is placed.
type plan struct {
	g     *Group
	sizes []size  // each item's size
	kids  []*plan // each group item's plan, nil for a node
	grid  bool
	// A grid's: each item's cell, and the widths of its columns and the
	// heights of its rows.
	cells      []Point
	colW, rowH []int
	// A layered group's: its items by layer, in order along each; the items
	// each item's edges join it to; and where each item begins along its
	// layer.
	layers     [][]int
	neighbours [][]int
	along      []int
	dir        Direction
	w, h       int // the content's size, the group's own edge and label not included
}

// outer is the size of a group whose content is p: the content and the
// group's edge and label.
func (p *plan) outer() size {
	switch p.g.Style {
	case Frame:
		return size{max(p.w+2*frameSide, Width(p.g.Label)+2*frameSide), p.h + 2*frameTop}
	case Lane:
		if p.g.Label == "" {
			return size{p.w, p.h}
		}
		return size{p.w + laneLabelGap + Width(p.g.Label), p.h}
	}
	return size{p.w, p.h}
}

// layouter is the state of one Layout.
type layouter struct {
	d *Diagram
	// path is each node's groups, outermost first, the root included.
	path map[string][]*Group
	out  *Layout
	rect map[string]Rect
}

// Layout lays the diagram out. An edge naming a node the diagram does not
// hold, a node held twice, and two items pinned to one cell are errors.
func (d *Diagram) Layout() (*Layout, error) {
	l := &layouter{d: d, path: map[string][]*Group{}, out: &Layout{Title: d.Title, nodeAt: map[string]PlacedNode{}}, rect: map[string]Rect{}}
	if err := l.index(&d.Root, nil); err != nil {
		return nil, err
	}
	for _, e := range d.Edges {
		for _, id := range []string{e.From, e.To} {
			if _, ok := l.path[id]; !ok {
				return nil, fmt.Errorf("an edge names %q, which is no node of the diagram", id)
			}
		}
	}
	p, err := l.plan(&d.Root)
	if err != nil {
		return nil, err
	}
	top := 0
	if d.Title != "" {
		top = 2
	}
	// The boxes are placed a margin in from the left and the top, as the
	// router has one at the right and the bottom, so that a line can go
	// round them on every side. Without it an edge between two boxes of the
	// top row, with a third between them, had no way round but through the
	// third. What no line takes of the margin is given back after routing,
	// so a drawing that does not need it does not change.
	l.place(p, colGap, top+rowGap, p.w, p.h)
	l.route()
	l.trim(top, colGap+p.w, top+rowGap+p.h)
	l.out.placeLabels()
	if d.Straight {
		l.out.straighten()
	}
	l.out.placeLegend(d.Legend)
	return l.out, nil
}

// trim moves everything placed to the left and up, as far as the leftmost
// and the topmost of the boxes, the groups and the lines allow, and sizes
// the layout to what is placed: the margin route had round the boxes, less
// what its lines took. right and bottom are where the boxes' plan ended.
func (l *layouter) trim(top, right, bottom int) {
	minX, minY := right, bottom
	reach := func(x0, y0, x1, y1 int) {
		minX, minY = min(minX, x0), min(minY, y0)
		right, bottom = max(right, x1), max(bottom, y1)
	}
	for _, n := range l.out.Nodes {
		reach(n.Rect.X, n.Rect.Y, n.Rect.X+n.Rect.W, n.Rect.Y+n.Rect.H)
	}
	for _, g := range l.out.Groups {
		reach(g.Rect.X, g.Rect.Y, g.Rect.X+g.Rect.W, g.Rect.Y+g.Rect.H)
	}
	for _, e := range l.out.Edges {
		for _, pt := range e.Points {
			reach(pt.X, pt.Y, pt.X+1, pt.Y+1) // a loop may reach past the boxes
		}
	}
	minY = max(minY-top, 0)
	l.out.move(-minX, -minY)
	for id, r := range l.rect {
		l.rect[id] = Rect{r.X - minX, r.Y - minY, r.W, r.H}
	}
	l.out.Width = max(right-minX, Width(l.d.Title))
	l.out.Height = max(bottom-minY, top)
}

// move moves everything placed dx cells across and dy rows down.
func (lay *Layout) move(dx, dy int) {
	if dx == 0 && dy == 0 {
		return
	}
	at := func(p *Point) { p.X, p.Y = p.X+dx, p.Y+dy }
	for i := range lay.Nodes {
		r := &lay.Nodes[i].Rect
		r.X, r.Y = r.X+dx, r.Y+dy
		lay.nodeAt[lay.Nodes[i].Node.ID] = lay.Nodes[i]
	}
	for i := range lay.Groups {
		g := &lay.Groups[i]
		g.Rect.X, g.Rect.Y = g.Rect.X+dx, g.Rect.Y+dy
		at(&g.LabelAt)
		for j := range g.Dividers {
			at(&g.Dividers[j][0])
			at(&g.Dividers[j][1])
		}
	}
	for i := range lay.Edges {
		e := &lay.Edges[i]
		for j := range e.Points {
			at(&e.Points[j])
		}
		at(&e.LabelAt)
		at(&e.pictureAt)
	}
}

// A legend entry's sample is legendSample cells wide, its label
// legendGap cells after it.
const legendSample, legendGap = 4, 2

// placeLegend places lg in a frame below everything else, at the left, its
// entries one under another; an entry's label of several lines takes as
// many rows.
func (lay *Layout) placeLegend(lg *Legend) {
	if lg == nil || len(lg.Entries) == 0 {
		return
	}
	top := 0
	if lay.Height > 0 {
		top = lay.Height + 1
	}
	w, h := 0, 0
	for _, e := range lg.Entries {
		lines := strings.Split(e.Label, "\n")
		for _, l := range lines {
			w = max(w, legendSample+legendGap+Width(l))
		}
		h += len(lines)
	}
	frame := Rect{0, top, max(w, Width(lg.Label)) + 2*frameSide, h + 2*frameTop}
	y := top + frameTop
	for i := range lg.Entries {
		e := &lg.Entries[i]
		x := frameSide
		lay.Legend = append(lay.Legend, PlacedEntry{Entry: e, Sample: Rect{x, y, legendSample, 1}, LabelAt: Point{x + legendSample + legendGap, y}})
		y += strings.Count(e.Label, "\n") + 1
	}
	lay.Groups = append(lay.Groups, PlacedGroup{Group: &Group{ID: "legend", Label: lg.Label, Style: Frame}, Rect: frame, LabelAt: Point{frame.X + 2, frame.Y}})
	lay.Width, lay.Height = max(lay.Width, frame.X+frame.W), frame.Y+frame.H
}

// index records each node's groups.
func (l *layouter) index(g *Group, above []*Group) error {
	path := append(append([]*Group(nil), above...), g)
	for _, it := range g.Items {
		switch {
		case it.Node != nil:
			if _, ok := l.path[it.Node.ID]; ok {
				return fmt.Errorf("the node %q is in the diagram twice", it.Node.ID)
			}
			l.path[it.Node.ID] = path
		case it.Group != nil:
			if err := l.index(it.Group, path); err != nil {
				return err
			}
		default:
			// Every step after this one takes an item for one or the
			// other; Item{} was a nil dereference half way through.
			return fmt.Errorf("%s holds an item that is neither a node nor a group", groupName(g))
		}
	}
	return nil
}

// plan plans g and, first, every group it holds.
func (l *layouter) plan(g *Group) (*plan, error) {
	p := &plan{g: g, sizes: make([]size, len(g.Items)), kids: make([]*plan, len(g.Items)), dir: l.d.Direction}
	if g.Rows > MaxGrid || g.Columns > MaxGrid || g.Rows < 0 || g.Columns < 0 {
		return nil, fmt.Errorf("%s: %d rows and %d columns, where a grid has from 1 to %d", groupName(g), g.Rows, g.Columns, MaxGrid)
	}
	for i, it := range g.Items {
		if r, c := pinOf(it); r > MaxGrid || c > MaxGrid || r < 0 || c < 0 {
			return nil, fmt.Errorf("%s is pinned to row %d, column %d, where a grid has from 1 to %d", itemName(it), r, c, MaxGrid)
		}
		if it.Group != nil {
			kid, err := l.plan(it.Group)
			if err != nil {
				return nil, err
			}
			p.kids[i] = kid
			p.sizes[i] = kid.outer()
		} else {
			p.sizes[i] = nodeSize(it.Node)
		}
	}
	if g.Rows > 0 || g.Columns > 0 {
		p.grid = true
		if err := p.assignCells(); err != nil {
			return nil, err
		}
		p.shareColumns()
		p.measureGrid()
	} else {
		p.layers, p.neighbours = l.layer(g, p.dir)
		p.measureLayers()
	}
	return p, nil
}

// pinOf is the row and the column an item is pinned to, as written.
func pinOf(it Item) (row, col int) {
	if it.Node != nil {
		return it.Node.Row, it.Node.Col
	}
	return it.Group.Row, it.Group.Col
}

// pin is an item's pinned cell, counting from 0, and whether it has one: a
// row without a column is the first column, and a column without a row the
// first row.
func pin(it Item) (Point, bool) {
	r, c := pinOf(it)
	if r <= 0 && c <= 0 {
		return Point{}, false
	}
	return Point{max(c, 1) - 1, max(r, 1) - 1}, true
}

// assignCells puts each of the grid's items in a cell: a pinned item in its
// own, and the others in the free cells in the order they are written --
// row by row, Columns to a row, or column by column, Rows to a column. A
// grid grows to hold what it is given.
func (p *plan) assignCells() error {
	g := p.g
	p.cells = make([]Point, len(g.Items))
	taken := map[Point]int{}
	for i, it := range g.Items {
		if c, ok := pin(it); ok {
			if j, clash := taken[c]; clash {
				return fmt.Errorf("%s and %s are both pinned to row %d, column %d of %s",
					itemName(g.Items[j]), itemName(it), c.Y+1, c.X+1, groupName(g))
			}
			taken[c] = i
			p.cells[i] = c
		}
	}
	next := 0
	cellOf := func(n int) Point {
		if g.Columns > 0 {
			return Point{n % g.Columns, n / g.Columns}
		}
		return Point{n / g.Rows, n % g.Rows}
	}
	for i, it := range g.Items {
		if _, ok := pin(it); ok {
			continue
		}
		for {
			c := cellOf(next)
			next++
			if _, used := taken[c]; !used {
				taken[c] = i
				p.cells[i] = c
				break
			}
		}
	}
	return nil
}

func itemName(it Item) string {
	if it.Node != nil {
		return fmt.Sprintf("%q", it.Node.ID)
	}
	return groupName(it.Group)
}

func groupName(g *Group) string {
	if g.ID == "" {
		return "the diagram"
	}
	return fmt.Sprintf("%q", g.ID)
}

// shareColumns gives the grids stacked in each of the grid's columns the
// same column widths, the widest of each, so that what is in the same
// column of each lines up; and the grids side by side in each of its rows
// the same row heights.
func (p *plan) shareColumns() {
	byCol, byRow := map[int][]*plan{}, map[int][]*plan{}
	for i, kid := range p.kids {
		if kid != nil && kid.grid {
			byCol[p.cells[i].X] = append(byCol[p.cells[i].X], kid)
			byRow[p.cells[i].Y] = append(byRow[p.cells[i].Y], kid)
		}
	}
	share := func(kids []*plan, widths func(*plan) []int) {
		if len(kids) < 2 {
			return
		}
		var most []int
		for _, k := range kids {
			for j, w := range widths(k) {
				if j == len(most) {
					most = append(most, 0)
				}
				most[j] = max(most[j], w)
			}
		}
		for _, k := range kids {
			ws := widths(k)
			for j := range ws {
				ws[j] = most[j]
			}
		}
	}
	for _, kids := range byCol {
		share(kids, func(k *plan) []int { return k.colW })
	}
	for _, kids := range byRow {
		share(kids, func(k *plan) []int { return k.rowH })
	}
	for i, kid := range p.kids {
		if kid != nil && kid.grid {
			kid.w, kid.h = sum(kid.colW, colGap), sum(kid.rowH, rowGap)
			p.sizes[i] = kid.outer()
		}
	}
}

// measureGrid sizes the grid's columns and rows to the items in them.
func (p *plan) measureGrid() {
	cols, rows := 0, 0
	for _, c := range p.cells {
		cols, rows = max(cols, c.X+1), max(rows, c.Y+1)
	}
	// A shared width set by the grid around this one is kept: it is at
	// least this grid's own.
	for len(p.colW) < cols {
		p.colW = append(p.colW, 0)
	}
	for len(p.rowH) < rows {
		p.rowH = append(p.rowH, 0)
	}
	for i, c := range p.cells {
		p.colW[c.X] = max(p.colW[c.X], p.sizes[i].w)
		p.rowH[c.Y] = max(p.rowH[c.Y], p.sizes[i].h)
	}
	p.w, p.h = sum(p.colW, colGap), sum(p.rowH, rowGap)
}

// sum is the total of xs with gap between each two.
func sum(xs []int, gap int) int {
	t := 0
	for i, x := range xs {
		if i > 0 {
			t += gap
		}
		t += x
	}
	return t
}

// across is whether layers run across the page -- each to the right or
// left of the last -- rather than down it.
func (d Direction) across() bool { return d == Right || d == Left }

// gaps are a layered group's gaps: between the items along a layer, and
// between one layer and the next.
func (p *plan) gaps() (along, between int) {
	if p.dir.across() {
		return rowGap, colGap
	}
	return colGap, rowGap
}

// oriented is item i's size along its layer and across it: its width and
// height for layers running down the page, its height and width for layers
// running across.
func (p *plan) oriented(i int) size {
	s := p.sizes[i]
	if p.dir.across() {
		return size{s.h, s.w}
	}
	return s
}

// measureLayers places a layered group's items along their layers, and
// sizes the group: its layers one after another, each as thick as the
// thickest item in it.
//
// Each item goes under the items its edges join it to in the layers before
// it -- in the middle of them, where there are several -- then, from the
// last layer back, over those in the layers after it, and then under those
// before it once more: a step of a flow sits over the branches it splits
// into, a branch under its step, and the step the branches meet at under
// the middle of them. The items of a layer stay in the order layer chose, a
// gap apart; see set for where two want the same place.
func (p *plan) measureLayers() {
	ga, gb := p.gaps()
	n := len(p.sizes)
	p.along = make([]int, n)
	if n == 0 {
		// An empty group -- or an empty diagram, a legend alone, say --
		// takes no room.
		return
	}
	layerOf := make([]int, n)
	for li, layer := range p.layers {
		for _, i := range layer {
			layerOf[i] = li
		}
	}
	centre := func(i int) float64 { return float64(p.along[i]) + float64(p.oriented(i).w)/2 }
	// want is where item i's middle would be: the mean of the middles of
	// the items its edges join it to in the layers before -- or after --
	// it, or ok false where it has none.
	want := func(i int, before bool) (float64, bool) {
		t, c := 0.0, 0
		for _, j := range p.neighbours[i] {
			if before && layerOf[j] < layerOf[i] || !before && layerOf[j] > layerOf[i] {
				t += centre(j)
				c++
			}
		}
		if c == 0 {
			return 0, false
		}
		return t / float64(c), true
	}
	// set places a layer's items where they want to be, as near as the
	// gaps between them allow: items that would overlap are packed side by
	// side, as a block, and the block is placed where its items want it on
	// average -- so two branches wanting to be under the same step share
	// it, one either side, rather than the second giving way to the first.
	type block struct {
		items        []int
		offs         []int   // each item's start, from the block's
		sum          float64 // of each item's wanted start less its offset
		width, count int
	}
	start := func(b *block) float64 { return b.sum / float64(b.count) }
	set := func(layer []int, before, keep bool) {
		var blocks []*block
		for _, i := range layer {
			w := p.oriented(i).w
			var d float64
			if c, ok := want(i, before); ok {
				d = c - float64(w)/2
			} else if keep {
				d = float64(p.along[i])
			} else if len(blocks) > 0 {
				last := blocks[len(blocks)-1]
				d = start(last) + float64(last.width+ga)
			}
			blocks = append(blocks, &block{items: []int{i}, offs: []int{0}, sum: d, width: w, count: 1})
			for len(blocks) > 1 {
				a, b := blocks[len(blocks)-2], blocks[len(blocks)-1]
				if start(a)+float64(a.width+ga) <= start(b) {
					break
				}
				shift := a.width + ga
				for k, it := range b.items {
					a.items = append(a.items, it)
					a.offs = append(a.offs, b.offs[k]+shift)
				}
				a.sum += b.sum - float64(b.count*shift)
				a.count += b.count
				a.width += ga + b.width
				blocks = blocks[:len(blocks)-1]
			}
		}
		for _, b := range blocks {
			at := int(math.Round(start(b)))
			for k, it := range b.items {
				p.along[it] = at + b.offs[k]
			}
		}
	}
	for _, layer := range p.layers {
		set(layer, true, false)
	}
	for li := len(p.layers) - 2; li >= 0; li-- {
		set(p.layers[li], false, true)
	}
	// And down once more, so an item under several -- the end a flow's
	// branches meet at -- is under the middle of them where they now are.
	for _, layer := range p.layers[1:] {
		set(layer, true, true)
	}
	// From the first cell.
	least := 0
	for i := range n {
		if i == 0 || p.along[i] < least {
			least = p.along[i]
		}
	}
	length, depth := 0, 0
	for i := range n {
		p.along[i] -= least
		length = max(length, p.along[i]+p.oriented(i).w)
	}
	for li, layer := range p.layers {
		thick := 0
		for _, i := range layer {
			thick = max(thick, p.oriented(i).h)
		}
		if li > 0 {
			depth += gb
		}
		depth += thick
	}
	if p.dir.across() {
		p.w, p.h = depth, length
	} else {
		p.w, p.h = length, depth
	}
}

// layer puts g's items in layers: each below -- or after, in d -- the items
// it has edges from, as far as the edges allow, and the items of each
// layer in the order that crosses fewest edges.
//
// An edge counts at the level of the group: an edge into a node inside one
// of g's groups is an edge into that group. Edges that close a cycle are
// read backwards for the layering, as the first of them in the order
// written that a walk of the items meets, so a cycle still has a first
// item. Items with no edges keep the order they are written in.
func (l *layouter) layer(g *Group, d Direction) (layers, neighbours [][]int) {
	n := len(g.Items)
	if n == 0 {
		return nil, nil
	}
	itemOf := func(id string) int {
		path := l.path[id]
		for i, grp := range path {
			if grp == g {
				if i+1 < len(path) {
					// Inside one of g's groups: that group.
					for j, it := range g.Items {
						if it.Group == path[i+1] {
							return j
						}
					}
				}
				for j, it := range g.Items {
					if it.Node != nil && it.Node.ID == id {
						return j
					}
				}
			}
		}
		return -1
	}
	// Items made one by SameLayer.
	class := make([]int, n)
	for i := range class {
		class[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for class[i] != i {
			i = class[i]
		}
		return i
	}
	for _, same := range l.d.SameLayer {
		first := -1
		for _, id := range same {
			if i := itemOf(id); i >= 0 {
				if first < 0 {
					first = i
				} else if a, b := find(first), find(i); a != b {
					class[max(a, b)] = min(a, b)
				}
			}
		}
	}
	succ := make([][]int, n)
	neighbours = make([][]int, n)
	for _, e := range l.d.Edges {
		a, b := itemOf(e.From), itemOf(e.To)
		if a < 0 || b < 0 || a == b {
			continue
		}
		neighbours[a] = append(neighbours[a], b)
		neighbours[b] = append(neighbours[b], a)
		if find(a) == find(b) {
			continue
		}
		succ[find(a)] = append(succ[find(a)], find(b))
	}
	// Drop the edges that close a cycle: a walk in item order, an edge
	// to an item still on the walk's path is one.
	state := make([]int, n) // 0 unvisited, 1 on the path, 2 done
	keep := make([][]int, n)
	var walk func(int)
	walk = func(i int) {
		state[i] = 1
		for _, j := range succ[i] {
			if state[j] == 1 {
				continue
			}
			keep[i] = append(keep[i], j)
			if state[j] == 0 {
				walk(j)
			}
		}
		state[i] = 2
	}
	for i := range n {
		if find(i) == i && state[i] == 0 {
			walk(i)
		}
	}
	// Longest path from the items nothing points to.
	rank := make([]int, n)
	for range n {
		changed := false
		for i := range n {
			for _, j := range keep[i] {
				if rank[j] < rank[i]+1 {
					rank[j], changed = rank[i]+1, true
				}
			}
		}
		if !changed {
			break
		}
	}
	top := 0
	for i := range n {
		rank[i] = rank[find(i)]
		top = max(top, rank[i])
	}
	layers = make([][]int, top+1)
	for i := range n {
		layers[rank[i]] = append(layers[rank[i]], i)
	}
	// Fewer crossings: each item to the mean place of the items its edges
	// come from in the layer before, then of those they go to in the layer
	// after, a few times over, an item with neither keeping its place.
	pos := make([]float64, n)
	setPos := func() {
		for _, layer := range layers {
			for k, i := range layer {
				pos[i] = float64(k)
			}
		}
	}
	setPos()
	pred := make([][]int, n)
	for i := range n {
		for _, j := range succ[i] {
			pred[j] = append(pred[j], i)
		}
	}
	for i := range n {
		// Members of a SameLayer class share the edges of its root.
		if r := find(i); r != i {
			pred[i], succ[i] = pred[r], succ[r]
		}
	}
	sweep := func(from [][]int, order []int) {
		for _, li := range order {
			layer := layers[li]
			bary := make(map[int]float64, len(layer))
			for _, i := range layer {
				t, c := 0.0, 0
				for _, j := range from[i] {
					if rank[j] != rank[i] {
						t += pos[j]
						c++
					}
				}
				if c > 0 {
					bary[i] = t / float64(c)
				} else {
					bary[i] = pos[i]
				}
			}
			sort.SliceStable(layer, func(a, b int) bool { return bary[layer[a]] < bary[layer[b]] })
			for k, i := range layer {
				pos[i] = float64(k)
			}
		}
	}
	down, up := make([]int, 0, len(layers)), make([]int, 0, len(layers))
	for li := range layers {
		down = append(down, li)
		up = append(up, len(layers)-1-li)
	}
	for range 4 {
		sweep(pred, down[1:])
		sweep(succ, up[1:])
	}
	if d == Up || d == Left {
		for i, j := 0, len(layers)-1; i < j; i, j = i+1, j-1 {
			layers[i], layers[j] = layers[j], layers[i]
		}
	}
	return layers, neighbours
}

// place puts p's group at (x, y), w by h cells -- at least its size, and
// more where its cell in the group around it is larger -- and what it holds
// inside it.
func (l *layouter) place(p *plan, x, y, w, h int) {
	g := p.g
	inner := Rect{x, y, w, h}
	if g != &l.d.Root {
		l.out.Groups = append(l.out.Groups, PlacedGroup{Group: g, Rect: Rect{x, y, w, h}})
		switch g.Style {
		case Frame:
			inner = Rect{x + frameSide, y + frameTop, w - 2*frameSide, h - 2*frameTop}
		case Lane:
			if g.Label != "" {
				inner.W -= laneLabelGap + Width(g.Label)
			}
		}
	}
	if p.grid {
		l.placeGrid(p, inner)
	} else {
		l.placeLayers(p, inner)
	}
}

// placeGrid places a grid's items from inner's top left: a group filling
// its cell, so that stacked lanes and frames are as wide as each other, and
// a node in the middle of its cell.
func (l *layouter) placeGrid(p *plan, inner Rect) {
	colX, rowY := make([]int, len(p.colW)), make([]int, len(p.rowH))
	for i := range colX {
		if i > 0 {
			colX[i] = colX[i-1] + p.colW[i-1] + colGap
		}
	}
	for i := range rowY {
		if i > 0 {
			rowY[i] = rowY[i-1] + p.rowH[i-1] + rowGap
		}
	}
	for i, it := range p.g.Items {
		c := p.cells[i]
		cx, cy := inner.X+colX[c.X], inner.Y+rowY[c.Y]
		cw, ch := p.colW[c.X], p.rowH[c.Y]
		if it.Group != nil {
			at := len(l.out.Groups)
			l.place(p.kids[i], cx, cy, cw, ch)
			if it.Group.Style == Lane {
				pg := &l.out.Groups[at]
				if c.Y > 0 {
					y := cy - (rowGap+1)/2
					pg.Dividers = append(pg.Dividers, [2]Point{{inner.X, y}, {inner.X + p.w - 1, y}})
				}
				if c.X > 0 {
					x := cx - (colGap+1)/2
					pg.Dividers = append(pg.Dividers, [2]Point{{x, inner.Y}, {x, inner.Y + p.h - 1}})
				}
			}
			continue
		}
		s := p.sizes[i]
		l.placeNode(it.Node, cx+(cw-s.w)/2, cy+(ch-s.h)/2, s)
	}
}

// placeLayers places a layered group's items, as measureLayers put them
// along their layers, in the middle of inner; each in the middle of its
// layer's thickness.
func (l *layouter) placeLayers(p *plan, inner Rect) {
	_, gb := p.gaps()
	ox, oy := inner.X+(inner.W-p.w)/2, inner.Y
	at := 0 // where the layer begins, along the layers' direction
	for li, layer := range p.layers {
		thick := 0
		for _, it := range layer {
			thick = max(thick, p.oriented(it).h)
		}
		for _, it := range layer {
			s, o := p.sizes[it], p.oriented(it)
			x, y := ox+p.along[it], oy+at+(thick-o.h)/2
			if p.dir.across() {
				x, y = ox+at+(thick-o.h)/2, oy+p.along[it]
			}
			item := p.g.Items[it]
			if item.Group == nil {
				l.placeNode(item.Node, x, y, s)
				continue
			}
			gi := len(l.out.Groups)
			l.place(p.kids[it], x, y, s.w, s.h)
			if item.Group.Style == Lane && li > 0 {
				pg := &l.out.Groups[gi]
				if p.dir.across() {
					dx := ox + at - (gb+1)/2
					pg.Dividers = append(pg.Dividers, [2]Point{{dx, oy}, {dx, oy + p.h - 1}})
				} else {
					dy := oy + at - (gb+1)/2
					pg.Dividers = append(pg.Dividers, [2]Point{{ox, dy}, {ox + p.w - 1, dy}})
				}
			}
		}
		at += thick + gb
	}
}

func (l *layouter) placeNode(n *Node, x, y int, s size) {
	r := Rect{x, y, s.w, s.h}
	pn := PlacedNode{Node: n, Rect: r}
	l.out.Nodes = append(l.out.Nodes, pn)
	l.out.nodeAt[n.ID] = pn
	l.rect[n.ID] = r
}

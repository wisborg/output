// Package diagram lays out and draws diagrams of boxes and the lines
// between them -- a flow of steps, a database cluster and its replicas, a
// service and what it calls -- in the terminal as text, or, through the
// picture and svg packages, as an image.
//
// A diagram is nodes, the edges between them, and groups of nodes: a
// region, a cluster, a stage of a flow. A group places what it holds in one
// of two ways, chosen per group:
//
//   - In a grid, where the diagram says so (Rows or Columns): its items in
//     the order they are written, row by row or column by column, or pinned
//     to a cell. A grid's places come from what the diagram declares and
//     never from its edges, so a diagram drawn of a system whose state
//     changed -- a replica that stopped replicating -- draws every box where
//     it was, and only the line between them changes. A reader comparing two
//     drawings of one cluster compares the lines, not the layout.
//   - In layers, otherwise: each item a layer below the items it has edges
//     from, as a flow chart is drawn, in Direction.
//
// Grids stacked in a grid share their columns, as a table's rows do: the
// first column of one region is the first column of the region beneath it,
// so what is in the same place in each lines up.
//
// Everything is measured in character cells, so that the layout is one
// thing whether it is drawn in a terminal's characters or as a picture: a
// picture draws each cell as a fixed box of pixels, in a monospaced font,
// whose letters are a cell wide as the terminal's are.
//
// The package imports only the standard library. Reading a diagram from
// Graphviz's DOT language is the dot package's.
package diagram

import (
	"image/color"
	"math"
)

// Direction is the way a layered group's layers run.
type Direction int

const (
	// Down puts each layer below the one its edges come from, as DOT's
	// rankdir=TB.
	Down Direction = iota
	// Right puts each to the right, as rankdir=LR.
	Right
	// Up puts each above, as rankdir=BT.
	Up
	// Left puts each to the left, as rankdir=RL.
	Left
)

// Shape is the outline a node is drawn in.
type Shape int

const (
	// Box is a rectangle.
	Box Shape = iota
	// Rounded is a rectangle with rounded corners.
	Rounded
	// Ellipse is an ellipse around the label.
	Ellipse
	// Cylinder is a drum, as a database is drawn.
	Cylinder
	// Plain is the label alone, with no outline.
	Plain
)

// Line is how a node's outline or an edge is stroked.
type Line int

const (
	Solid Line = iota
	Dashed
	Dotted
	Bold
)

// GroupStyle is how a group is drawn around what it holds.
type GroupStyle int

const (
	// Frame is a box around the group, its label set in the top edge.
	Frame GroupStyle = iota
	// Lane is a band across the diagram, set off from the lanes beside it
	// by a line, its label at its far side: a region, an availability zone,
	// a data centre.
	Lane
	// Hidden draws nothing: the group only places what it holds. The
	// diagram's own top group is one.
	Hidden
)

// Node is a box of the diagram.
type Node struct {
	// ID names the node to the edges.
	ID string
	// Label is what the node says, a line to each "\n"; empty is the ID.
	Label string
	Shape Shape
	// Line is the outline's stroke.
	Line Line
	// Colour is the outline's colour, Fill the inside's and Text the
	// label's; nil is the palette's.
	Colour, Fill, Text color.Color
	// Row and Col pin the node to a cell of its group's grid, counting
	// from 1; zero leaves it to the order the group's items are written in.
	Row, Col int
	// Invisible keeps the node's place and draws nothing, as DOT's
	// style=invis: how a gap in a grid is kept.
	Invisible bool
}

// Edge is a line from one node to another.
type Edge struct {
	From, To string
	Label    string
	// Head and Tail say the edge has an arrowhead at To and at From.
	Head, Tail bool
	Line       Line
	// Colour is the line's colour; nil is the palette's.
	Colour color.Color
	// Invisible draws nothing; in a layered group the edge still orders
	// the layers.
	Invisible bool
}

// Group is a group of nodes, and of groups.
type Group struct {
	ID    string
	Label string
	Style GroupStyle
	// Rows and Columns make the group a grid: Columns of them, filled row
	// by row in the order the items are written, or Rows, filled column by
	// column. Both zero lays the group out in layers.
	Rows, Columns int
	// Row and Col pin the group to a cell of the group around it, as a
	// node's do.
	Row, Col int
	// Colour is the frame's or the lane line's colour, Fill the inside's;
	// nil is the palette's, and no fill.
	Colour, Fill color.Color
	// Items are what the group holds, in the order written.
	Items []Item
}

// Item is one thing a group holds: a node or a group.
type Item struct {
	Node  *Node
	Group *Group
}

// Diagram is a whole diagram.
type Diagram struct {
	// Title is written over the diagram.
	Title string
	// Direction is the way layered groups' layers run.
	Direction Direction
	// Root holds everything; its style is not drawn.
	Root  Group
	Edges []Edge
	// SameLayer are sets of nodes, by ID, a layered group puts in one
	// layer: DOT's rank=same.
	SameLayer [][]string
}

// label is the node's label, its ID where it has none.
func (n *Node) label() string {
	if n.Label == "" {
		return n.ID
	}
	return n.Label
}

// Palette is the colours a diagram is drawn in, where a node, an edge or a
// group does not give its own.
type Palette struct {
	// Background fills the picture, and every node without a fill.
	Background color.Color
	// Foreground is the nodes' outlines and labels, and the edges.
	Foreground color.Color
	// Dim is the groups' frames, lines and labels, and the edges' labels.
	Dim color.Color
}

// TextOn is the colour of a label on fill: whichever of the palette's
// foreground and background stands out from it more, by WCAG's contrast
// ratio. Without a fill it is the foreground. So a pale fill keeps dark
// text in the dark palette too, where light text on it would be white on
// pale yellow; a translucent fill is taken over the background.
func (p Palette) TextOn(fill color.Color) color.Color {
	if fill == nil {
		return p.Foreground
	}
	r, g, b, a := fill.RGBA()
	over := func(c uint32, bg float64) float64 { // c premultiplied, 0..0xffff
		return float64(c)/0xffff + bg*(1-float64(a)/0xffff)
	}
	br, bgG, bb, _ := p.Background.RGBA()
	l := relative(over(r, float64(br)/0xffff), over(g, float64(bgG)/0xffff), over(b, float64(bb)/0xffff))
	fg, bg := luminance(p.Foreground), luminance(p.Background)
	if contrast(l, fg) >= contrast(l, bg) {
		return p.Foreground
	}
	return p.Background
}

func luminance(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	return relative(float64(r)/0xffff, float64(g)/0xffff, float64(b)/0xffff)
}

// relative is WCAG's relative luminance of an sRGB colour, each channel
// from 0 to 1.
func relative(r, g, b float64) float64 {
	lin := func(c float64) float64 {
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrast(a, b float64) float64 { return (max(a, b) + 0.05) / (min(a, b) + 0.05) }

// Light is dark ink on paper.
var Light = Palette{
	Background: color.RGBA{0xF5, 0xF4, 0xF1, 0xFF},
	Foreground: color.RGBA{0x16, 0x16, 0x1A, 0xFF},
	Dim:        color.RGBA{0x7A, 0x7A, 0x84, 0xFF},
}

// Dark is light ink on near-black.
var Dark = Palette{
	Background: color.RGBA{0x0E, 0x0E, 0x10, 0xFF},
	Foreground: color.RGBA{0xE8, 0xE8, 0xEC, 0xFF},
	Dim:        color.RGBA{0x8A, 0x8A, 0x96, 0xFF},
}

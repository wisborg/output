// Package dot reads a diagram in Graphviz's DOT language
// (https://graphviz.org/doc/info/lang.html), so a diagram can be written by
// hand in a language many people know, or taken from a program that already
// writes DOT.
//
// The whole grammar is read: graphs and digraphs, strict or not; node, edge
// and attribute statements; default attributes, which a subgraph inherits
// and may change for itself; subgraphs, named and anonymous, as an edge's
// ends; ports, whose compass points -- a:s, or tailport and headport --
// are the sides an edge leaves and enters its boxes by, diagram.Edge's
// FromSide and ToSide; quoted strings joined with +, HTML strings,
// comments.
//
// A subgraph whose name begins with "cluster" is a group, as Graphviz
// draws one; any other subgraph only scopes defaults, and with rank=same
// keeps its nodes in one layer.
//
// A few attributes are this package's own, which Graphviz ignores, so a
// file using them is still a file Graphviz draws:
//
//   - columns=N or rows=N, on the graph or a cluster, lay what it holds out
//     in a grid -- N columns filled row by row, or N rows filled column by
//     column -- in the order it is written, rather than in layers from its
//     edges. A grid's places do not depend on its edges, so a diagram of a
//     system keeps every box where it was when a link between two changes.
//   - row=R and col=C, on a node or a cluster, pin it to that cell of the
//     grid it is in, counting from 1.
//   - style=lane, on a cluster, draws it as a band across the diagram set
//     off by a line, its label at the far side, rather than as a box.
//   - style=legend, on a cluster, makes it the diagram's legend rather than
//     a part of it: a frame below the diagram, its label the legend's
//     title, each node in it an entry -- a swatch filled and outlined as
//     the node would be, or, for a node of shape plain or none, a short
//     line in its color and style, as an edge is drawn -- and the node's
//     label what it stands for. Graphviz draws the cluster as an ordinary
//     one.
//
// Of Graphviz's own attributes, those that say what a diagram means are
// read -- label, shape, style, color, fillcolor, fontcolor, dir, arrowhead,
// arrowtail, rankdir, rank -- and those about how Graphviz draws, which
// mean nothing to this layout, are ignored. One of those is read too:
// splines, on the graph, asks for the edges of a picture drawn straight,
// at whatever angle, for line, polyline, false, true, spline or curved --
// curves are drawn as straight lines, the nearest this draws -- and along
// rows and columns, as by default, for ortho.
package dot

import (
	"errors"
	"fmt"
	"image/color"
	"maps"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/wisborg/output/diagram"
)

// Parse reads a diagram from DOT source, held to the default Options. Only
// the first graph of a file is read.
func Parse(src string) (*diagram.Diagram, error) { return ParseWith(src, Options{}) }

// Options are the limits a file is read within. A DOT file is often
// someone else's, and a few bytes of one can ask for more work than any
// diagram is worth: these bound it, at limits no diagram anyone reads comes
// near, and a caller with a genuine diagram past them raises them.
type Options struct {
	// MaxEdges is the most edges the file may make -- and, in a strict
	// graph, the most it may state, since each statement of an edge it
	// already has is work too. Zero is DefaultMaxEdges.
	//
	// It is the layout that the count bounds. The router searches the page
	// for every edge, round the lines already drawn, and the time that takes
	// grows faster than the square of the edges: a random flow of 300 lays
	// out in a couple of seconds, of 500 in a quarter of a minute, of 1,000
	// in a minute and a half, and 10,000 -- "{a0 ... a99} -> {b0 ...
	// b99}", under a kilobyte -- did not finish in seven minutes.
	MaxEdges int
}

// DefaultMaxEdges is Options.MaxEdges where it is not set: a diagram of 300
// edges lays out in a few seconds at worst, and is already more lines than
// a page or a terminal shows legibly. Set low on purpose: a caller with a
// genuine diagram past it raises it, and one too high leaves a file able
// to ask for minutes.
const DefaultMaxEdges = 300

// ErrTooManyEdges is what a file that makes more than Options.MaxEdges is
// refused with, wrapped in an *Error naming the line.
var ErrTooManyEdges = errors.New("too many edges")

// ParseWith reads a diagram from DOT source, held to o. Source that is not
// UTF-8 is refused, with the line of its first byte that is not.
func ParseWith(src string, o Options) (*diagram.Diagram, error) {
	// Refused rather than read: a byte that is not UTF-8 went on into the
	// names and labels and out to the terminal raw -- 0x9b, the 8-bit CSI
	// some terminals still read as one -- and a reader made lenient now
	// could not be made strict again once files relied on it.
	if !utf8.ValidString(src) {
		at := 0
		for i, r := range src {
			if r == utf8.RuneError {
				if _, size := utf8.DecodeRuneInString(src[i:]); size == 1 {
					at = i
					break
				}
			}
		}
		return nil, &Error{Line: 1 + strings.Count(src[:at], "\n"), Msg: "the file is not UTF-8"}
	}
	if o.MaxEdges <= 0 {
		o.MaxEdges = DefaultMaxEdges
	}
	p := &parser{lex: lexer{src: src, line: 1}, nodes: map[string]*nodeInfo{}, clusters: map[string]*clusterInfo{}, maxEdges: o.MaxEdges}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if err := p.graph(); err != nil {
		return nil, err
	}
	// Only the first graph is read, but what follows it must be another,
	// or nothing: a stray brace, or a statement pasted after the graph's
	// end, is a mistake to be told of rather than dropped unread.
	if p.tok.kind != tEOF && !p.keyword("strict") && !p.keyword("graph") && !p.keyword("digraph") {
		return nil, p.errorf("unexpected %s after the graph's closing }", p.describe())
	}
	return p.d, nil
}

// nodeInfo is a node as read: the group it is in, and its attributes --
// the defaults where it was made, then each statement's over them -- which
// are read into the node once the file is, as what one means can depend on
// another set later: a fillcolor fills only with style=filled.
type nodeInfo struct {
	n     *diagram.Node
	group *diagram.Group
	attrs map[string]string
	lines map[string]int // the line that set each attribute
	line  int            // the line the node was made on
}

// scope is a graph's or a subgraph's: the defaults its statements start
// from, and what it is.
type scope struct {
	nodeAttrs, edgeAttrs map[string]string
	graphAttrs           map[string]string
	// The line that set each, for an error about it to name.
	nodeLines, edgeLines, graphLines map[string]int
	group                            *diagram.Group // the group its nodes are in: the innermost cluster
	nodes                            []string       // the nodes it holds, in order and once each: an edge's end, rank=same
	holds                            map[string]bool
}

// add records id as one of s's nodes, once however often it is mentioned:
// a subgraph's nodes are a set, as Graphviz reads them, so "{a a} -> b" is
// one edge, and "{a a a ...}" no multiplier of the edges it ends.
func (s *scope) add(id string) {
	if s.holds == nil {
		s.holds = map[string]bool{}
	}
	if !s.holds[id] {
		s.holds[id] = true
		s.nodes = append(s.nodes, id)
	}
}

func (s *scope) child() *scope {
	return &scope{
		nodeAttrs: clone(s.nodeAttrs), edgeAttrs: clone(s.edgeAttrs), graphAttrs: map[string]string{},
		nodeLines: clone(s.nodeLines), edgeLines: clone(s.edgeLines), graphLines: map[string]int{},
		group: s.group,
	}
}

func clone[V any](m map[string]V) map[string]V {
	c := make(map[string]V, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// merge sets each of from's attributes in to, and the line that set it.
func merge(to map[string]string, toLines map[string]int, from map[string]string, fromLines map[string]int) {
	for k, v := range from {
		to[k] = v
		toLines[k] = fromLines[k]
	}
}

type parser struct {
	lex      lexer
	tok      token
	d        *diagram.Diagram
	directed bool
	strict   bool
	nodes    map[string]*nodeInfo
	order    []string          // the nodes in the order made
	seen     map[[2]string]int // a strict graph's edges, each its index in d.Edges
	// edgeAttrs are each edge's attributes, by its index in d.Edges: a
	// strict graph's second statement of an edge sets them over the first's.
	edgeAttrs []map[string]string
	name      string // the graph's, for \G
	// clusters are the clusters read so far, by name, with the group each
	// is in and its attributes: a cluster written twice is one cluster.
	clusters map[string]*clusterInfo
	// legend is the cluster with style=legend, and the line it begins on.
	legend     *diagram.Group
	legendLine int
	depth      int // how many subgraphs the one being read is inside
	maxEdges   int // Options.MaxEdges
	stated     int // the edges stated, a strict graph's repeats included
}

// clusterInfo is a cluster as read so far.
type clusterInfo struct {
	g      *diagram.Group
	parent *diagram.Group
	attrs  map[string]string
	lines  map[string]int
	line   int
}

// maxDepth is the deepest subgraphs may nest. Nested millions deep they
// overflowed the parser's stack, which is fatal in Go and cannot be
// recovered from. No diagram anyone reads comes near it; the edges, which
// one does, are Options.MaxEdges.
const maxDepth = 100

func (p *parser) advance() error {
	t, err := p.lex.next()
	p.tok = t
	return err
}

func (p *parser) errorf(format string, args ...any) error {
	return &Error{Line: p.tok.line, Msg: fmt.Sprintf(format, args...)}
}

// keyword says the token is the keyword kw, which DOT reads in any case.
func (p *parser) keyword(kw string) bool {
	return p.tok.kind == tID && !p.tok.quoted && strings.EqualFold(p.tok.text, kw)
}

// reserved says the token is one of DOT's keywords, unquoted, which names
// no node: "a -> node" is a mistake to Graphviz, and was a node called
// node here. Quoted, "node" is a name like any other.
func (p *parser) reserved() bool {
	for _, kw := range []string{"node", "edge", "graph", "digraph", "subgraph", "strict"} {
		if p.keyword(kw) {
			return true
		}
	}
	return false
}

func (p *parser) punct(s string) bool { return p.tok.kind == tPunct && p.tok.text == s }

func (p *parser) expect(s string) error {
	if !p.punct(s) {
		return p.errorf("expected %q, found %s", s, p.describe())
	}
	return p.advance()
}

func (p *parser) describe() string {
	switch p.tok.kind {
	case tEOF:
		return "the end of the file"
	case tID:
		return strconv.Quote(p.tok.text)
	}
	return fmt.Sprintf("%q", p.tok.text)
}

// graph reads [strict] (graph|digraph) [ID] { stmt_list }.
func (p *parser) graph() error {
	if p.keyword("strict") {
		p.strict = true
		p.seen = map[[2]string]int{}
		if err := p.advance(); err != nil {
			return err
		}
	}
	switch {
	case p.keyword("digraph"):
		p.directed = true
	case p.keyword("graph"):
	default:
		return p.errorf("a DOT file begins with graph or digraph, not %s", p.describe())
	}
	if err := p.advance(); err != nil {
		return err
	}
	if p.tok.kind == tID {
		p.name = p.tok.text
		if err := p.advance(); err != nil {
			return err
		}
	}
	p.d = &diagram.Diagram{}
	p.d.Root.Style = diagram.Hidden
	root := &scope{
		nodeAttrs: map[string]string{}, edgeAttrs: map[string]string{}, graphAttrs: map[string]string{},
		nodeLines: map[string]int{}, edgeLines: map[string]int{}, graphLines: map[string]int{},
		group: &p.d.Root,
	}
	if err := p.expect("{"); err != nil {
		return err
	}
	if err := p.stmts(root); err != nil {
		return err
	}
	if err := p.expect("}"); err != nil {
		return err
	}
	for _, id := range p.order {
		if err := p.applyNode(p.nodes[id]); err != nil {
			return err
		}
	}
	if err := p.makeLegend(); err != nil {
		return err
	}
	return p.applyGraph(root.graphAttrs, root.graphLines)
}

// makeLegend turns the legend's cluster into the diagram's Legend: each
// node an entry, in the order written.
func (p *parser) makeLegend() error {
	g := p.legend
	if g == nil {
		return nil
	}
	wrap := func(format string, args ...any) error {
		return &Error{Line: p.legendLine, Msg: fmt.Sprintf("legend %q: ", g.ID) + fmt.Sprintf(format, args...)}
	}
	lg := &diagram.Legend{Label: g.Label}
	in := map[string]bool{}
	for _, it := range g.Items {
		if it.Group != nil {
			return wrap("a legend holds entries, not the cluster %q", it.Group.ID)
		}
		n := it.Node
		in[n.ID] = true
		// An entry without a label is named by its node's name, as a node
		// is: one left blank had a swatch and nothing beside it.
		e := diagram.LegendEntry{Label: n.Label, Fill: n.Fill, Colour: n.Colour, Line: n.Line}
		if e.Label == "" {
			e.Label = n.ID
		}
		if n.Shape == diagram.Plain {
			e.Sample, e.Fill = diagram.Stroke, nil
		}
		lg.Entries = append(lg.Entries, e)
	}
	for _, e := range p.d.Edges {
		for _, id := range []string{e.From, e.To} {
			if in[id] {
				return wrap("%q is an entry of it, not a node an edge can reach", id)
			}
		}
	}
	p.d.Legend = lg
	return nil
}

// stmts reads statements up to the closing brace.
func (p *parser) stmts(s *scope) error {
	for !p.punct("}") {
		if p.tok.kind == tEOF {
			return p.errorf("the file ends inside a graph: a } is missing")
		}
		if p.punct(";") {
			// An empty statement, as Graphviz allows: a;;b.
			if err := p.advance(); err != nil {
				return err
			}
			continue
		}
		if err := p.stmt(s); err != nil {
			return err
		}
	}
	return nil
}

// stmt reads one statement.
func (p *parser) stmt(s *scope) error {
	switch {
	case p.keyword("graph") || p.keyword("node") || p.keyword("edge"):
		kind := strings.ToLower(p.tok.text)
		if err := p.advance(); err != nil {
			return err
		}
		attrs, lines, err := p.attrLists()
		if err != nil {
			return err
		}
		switch kind {
		case "graph":
			merge(s.graphAttrs, s.graphLines, attrs, lines)
		case "node":
			merge(s.nodeAttrs, s.nodeLines, attrs, lines)
		default:
			merge(s.edgeAttrs, s.edgeLines, attrs, lines)
		}
		return nil
	case p.keyword("subgraph") || p.punct("{"):
		ends, err := p.subgraph(s)
		if err != nil {
			return err
		}
		return p.edgeRest(s, ends, "")
	case p.tok.kind != tID:
		return p.errorf("expected a statement, found %s", p.describe())
	case p.reserved():
		return p.errorf("expected a statement, found the keyword %s; quote it to name a node so", p.tok.text)
	}
	id := p.tok.text
	if err := p.advance(); err != nil {
		return err
	}
	if p.punct("=") {
		if err := p.advance(); err != nil {
			return err
		}
		if p.tok.kind != tID {
			return p.errorf("expected a value for %s, found %s", id, p.describe())
		}
		s.graphAttrs[id], s.graphLines[id] = p.tok.text, p.tok.line
		return p.advance()
	}
	port, err := p.port()
	if err != nil {
		return err
	}
	if p.tok.kind == tEdge {
		return p.edgeRest(s, []string{p.node(s, id)}, port)
	}
	attrs, lines, err := p.attrLists()
	if err != nil {
		return err
	}
	info := p.nodes[p.node(s, id)]
	merge(info.attrs, info.lines, attrs, lines)
	return nil
}

// port reads a node's [: ID [: ID]], and is the port as a tailport or
// headport attribute would give it -- "s", "field:s" -- or "" where none
// is written.
func (p *parser) port() (string, error) {
	var parts []string
	for p.punct(":") {
		if err := p.advance(); err != nil {
			return "", err
		}
		if p.tok.kind != tID {
			return "", p.errorf("expected a port after :, found %s", p.describe())
		}
		parts = append(parts, p.tok.text)
		if err := p.advance(); err != nil {
			return "", err
		}
	}
	return strings.Join(parts, ":"), nil
}

// portSide is the sides a port names by its compass point, its last part
// -- node:s, node:field:s, or a tailport or headport of "s" or "field:s"
// -- and any side where it has none: node:field names a field of a record,
// a shape not drawn here.
func portSide(parts []string) diagram.Side {
	if len(parts) == 0 {
		return 0
	}
	side, _ := compass(parts[len(parts)-1])
	return side
}

// compass is the sides a DOT compass point names, and whether word is
// one: n, e, s and w the top, the right, the bottom and the left; ne, se,
// sw and nw, which Graphviz takes for the corner, the two sides either
// side of it, since a line here never ends at a box's corner, where it
// reads as part of the outline; c and _ any side.
func compass(word string) (diagram.Side, bool) {
	switch word {
	case "n":
		return diagram.North, true
	case "e":
		return diagram.East, true
	case "s":
		return diagram.South, true
	case "w":
		return diagram.West, true
	case "ne":
		return diagram.North | diagram.East, true
	case "se":
		return diagram.South | diagram.East, true
	case "sw":
		return diagram.South | diagram.West, true
	case "nw":
		return diagram.North | diagram.West, true
	case "c", "_":
		return 0, true
	}
	return 0, false
}

// edgeRest reads the rest of an edge statement whose first end is from --
// the node, or the nodes of a subgraph -- if the statement is one: each
// further end, and the attributes. Every node of one end is joined to
// every node of the next.
func (p *parser) edgeRest(s *scope, from []string, fromPort string) error {
	if p.tok.kind != tEdge {
		return nil
	}
	ends := [][]string{from}
	// ports is each end's port, written on the node -- a:s -- and so of
	// every edge that ends there: a -> b:n -> c leaves b by its top as it
	// enters it. A subgraph's end has none.
	ports := []string{fromPort}
	for p.tok.kind == tEdge {
		if p.directed != (p.tok.text == "->") {
			if p.directed {
				return p.errorf("a digraph's edges are ->, not --")
			}
			return p.errorf("a graph's edges are --, not ->; a digraph has ->")
		}
		if err := p.advance(); err != nil {
			return err
		}
		switch {
		case p.keyword("subgraph") || p.punct("{"):
			nodes, err := p.subgraph(s)
			if err != nil {
				return err
			}
			ends, ports = append(ends, nodes), append(ports, "")
		case p.reserved():
			return p.errorf("expected a node after the edge, found the keyword %s; quote it to name a node so", p.tok.text)
		case p.tok.kind == tID:
			id := p.tok.text
			if err := p.advance(); err != nil {
				return err
			}
			port, err := p.port()
			if err != nil {
				return err
			}
			ends, ports = append(ends, []string{p.node(s, id)}), append(ports, port)
		default:
			return p.errorf("expected a node after the edge, found %s", p.describe())
		}
	}
	attrs, lines, err := p.attrLists()
	if err != nil {
		return err
	}
	mergedLines := clone(s.edgeLines)
	maps.Copy(mergedLines, lines)
	for i := 1; i < len(ends); i++ {
		for _, a := range ends[i-1] {
			for _, b := range ends[i] {
				if p.stated++; p.stated > p.maxEdges {
					return &Error{Line: p.tok.line, Msg: fmt.Sprintf("more than %d edges", p.maxEdges), Err: ErrTooManyEdges}
				}
				if p.strict {
					key := [2]string{a, b}
					if !p.directed && b < a {
						key = [2]string{b, a}
					}
					if k, ok := p.seen[key]; ok {
						// One edge, as Graphviz has it, with the second
						// statement's own attributes set over the first's
						// rather than dropped -- its own alone: defaults
						// apply where an edge is made, so an "edge [...]"
						// default in force at the second statement is no
						// attribute of the edge it restates.
						// An undirected edge restated the other way
						// round, b -- a for a -- b, has its ports
						// written the other way round too.
						tail, head := ports[i-1], ports[i]
						if p.d.Edges[k].From != a {
							tail, head = head, tail
						}
						both := withPorts(p.edgeAttrs[k], tail, head, attrs)
						e := diagram.Edge{From: p.d.Edges[k].From, To: p.d.Edges[k].To, Head: p.directed}
						if err := p.applyEdge(&e, both, mergedLines); err != nil {
							return err
						}
						p.d.Edges[k], p.edgeAttrs[k] = e, both
						continue
					}
					p.seen[key] = len(p.d.Edges)
				}
				merged := withPorts(s.edgeAttrs, ports[i-1], ports[i], attrs)
				e := diagram.Edge{From: a, To: b, Head: p.directed}
				if err := p.applyEdge(&e, merged, mergedLines); err != nil {
					return err
				}
				p.d.Edges = append(p.d.Edges, e)
				p.edgeAttrs = append(p.edgeAttrs, merged)
			}
		}
	}
	return nil
}

// withPorts is an edge's attributes, as Graphviz has them: before -- the
// defaults in force, or what a strict graph's earlier statement of the
// edge gave it -- then the ports written on its nodes, tail and head, as
// its tailport and headport, then the statement's own attributes, own.
// So a:n -> b [tailport = s] leaves a by its bottom, while under
// edge [tailport = s] it leaves by its top; and a port written in one
// statement of a strict graph's edge stays where the next writes none.
func withPorts(before map[string]string, tail, head string, own map[string]string) map[string]string {
	m := clone(before)
	if tail != "" {
		m["tailport"] = tail
	}
	if head != "" {
		m["headport"] = head
	}
	maps.Copy(m, own)
	return m
}

// subgraph reads [subgraph [ID]] { stmt_list }, and is the nodes it holds.
func (p *parser) subgraph(s *scope) ([]string, error) {
	name := ""
	if p.keyword("subgraph") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if p.tok.kind == tID {
			name = p.tok.text
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
	}
	line := p.tok.line
	if p.depth >= maxDepth {
		return nil, p.errorf("subgraphs nested more than %d deep", maxDepth)
	}
	p.depth++
	defer func() { p.depth-- }()
	if err := p.expect("{"); err != nil {
		return nil, err
	}
	sub := s.child()
	var g *diagram.Group
	var cl *clusterInfo
	if strings.HasPrefix(name, "cluster") {
		// A cluster written twice is one cluster, as Graphviz draws it:
		// its second statement adds to the first's nodes and attributes.
		if cl = p.clusters[name]; cl == nil {
			cl = &clusterInfo{g: &diagram.Group{ID: name}, parent: s.group, attrs: map[string]string{}, lines: map[string]int{}, line: line}
			p.clusters[name] = cl
			s.group.Items = append(s.group.Items, diagram.Item{Group: cl.g})
		}
		g = cl.g
		sub.group = g
	}
	if err := p.stmts(sub); err != nil {
		return nil, err
	}
	if err := p.expect("}"); err != nil {
		return nil, err
	}
	if g != nil {
		merge(cl.attrs, cl.lines, sub.graphAttrs, sub.graphLines)
		if err := p.applyGroup(g, cl.attrs, cl.lines, line); err != nil {
			return nil, err
		}
		if styleSet(cl.attrs["style"])["legend"] && p.legend != g {
			if p.legend != nil {
				return nil, &Error{Line: line, Msg: fmt.Sprintf("cluster %q: a diagram has one legend, and %q is already it", g.ID, p.legend.ID)}
			}
			// Not a part of the diagram: out of the group it was in.
			for i, it := range cl.parent.Items {
				if it.Group == g {
					cl.parent.Items = append(cl.parent.Items[:i], cl.parent.Items[i+1:]...)
					break
				}
			}
			p.legend, p.legendLine = g, line
		} else if !styleSet(cl.attrs["style"])["legend"] && p.legend == g {
			// Reopened with a style that is not legend, which as the last
			// value written wins: a part of the diagram again, back in
			// the group it was taken out of.
			cl.parent.Items = append(cl.parent.Items, diagram.Item{Group: g})
			p.legend = nil
		}
	}
	if strings.EqualFold(sub.graphAttrs["rank"], "same") {
		p.d.SameLayer = append(p.d.SameLayer, sub.nodes)
	}
	// The subgraph's nodes are the enclosing graph's too.
	for _, id := range sub.nodes {
		s.add(id)
	}
	return sub.nodes, nil
}

// attrLists reads any number of [ a_list ]: the attributes, and the line
// each is on.
func (p *parser) attrLists() (map[string]string, map[string]int, error) {
	attrs, lines := map[string]string{}, map[string]int{}
	for p.punct("[") {
		if err := p.advance(); err != nil {
			return nil, nil, err
		}
		for !p.punct("]") {
			if p.tok.kind != tID {
				return nil, nil, p.errorf("expected an attribute, found %s", p.describe())
			}
			k, line := p.tok.text, p.tok.line
			if err := p.advance(); err != nil {
				return nil, nil, err
			}
			v := "true"
			if p.punct("=") {
				if err := p.advance(); err != nil {
					return nil, nil, err
				}
				if p.tok.kind != tID {
					return nil, nil, p.errorf("expected a value for %s, found %s", k, p.describe())
				}
				v = p.tok.text
				if err := p.advance(); err != nil {
					return nil, nil, err
				}
			}
			attrs[k], lines[k] = v, line
			if p.punct(",") || p.punct(";") {
				if err := p.advance(); err != nil {
					return nil, nil, err
				}
			}
		}
		if err := p.advance(); err != nil {
			return nil, nil, err
		}
	}
	return attrs, lines, nil
}

// node is the node id, made with s's defaults where this is its first
// mention, and recorded as one of s's nodes.
//
// A node is in the group of the innermost cluster it is first mentioned in:
// one mentioned outside every cluster -- at the end of an edge written
// before the clusters, say -- and then inside one moves into it, as
// Graphviz draws it there; and so does one mentioned in a cluster and then
// in a cluster inside that. A node is in one group only; mentioned in a
// second cluster that is not inside the first, it stays in the first.
func (p *parser) node(s *scope, id string) string {
	info, ok := p.nodes[id]
	if !ok {
		n := &diagram.Node{ID: id}
		// Defaults apply where a node is made, as in Graphviz: a node
		// statement later in the file does not change one made before it.
		info = &nodeInfo{n: n, group: s.group, attrs: clone(s.nodeAttrs), lines: clone(s.nodeLines), line: p.tok.line}
		p.nodes[id] = info
		p.order = append(p.order, id)
		s.group.Items = append(s.group.Items, diagram.Item{Node: n})
	} else if s.group != info.group && p.inside(s.group, info.group) {
		removeNode(info.group, info.n)
		s.group.Items = append(s.group.Items, diagram.Item{Node: info.n})
		info.group = s.group
	}
	s.add(id)
	return id
}

// inside says g is a cluster within outer, at any depth.
func (p *parser) inside(g, outer *diagram.Group) bool {
	for g != nil && g != &p.d.Root {
		cl := p.clusters[g.ID]
		if cl == nil {
			return false
		}
		if cl.parent == outer {
			return true
		}
		g = cl.parent
	}
	return outer == &p.d.Root && g == &p.d.Root
}

func removeNode(g *diagram.Group, n *diagram.Node) {
	for i, it := range g.Items {
		if it.Node == n {
			g.Items = append(g.Items[:i], g.Items[i+1:]...)
			return
		}
	}
}

// applyGraph reads the top graph's attributes.
func (p *parser) applyGraph(attrs map[string]string, lines map[string]int) error {
	d := p.d
	d.Title = label(attrs["label"], "", p.name, "")
	switch strings.ToLower(attrs["splines"]) {
	case "line", "polyline", "false", "true", "spline", "curved":
		d.Straight = true
	}
	switch strings.ToUpper(attrs["rankdir"]) {
	case "LR":
		d.Direction = diagram.Right
	case "BT":
		d.Direction = diagram.Up
	case "RL":
		d.Direction = diagram.Left
	}
	var err error
	if d.Root.Rows, err = count(attrs, "rows"); err != nil {
		return &Error{Line: lines["rows"], Msg: err.Error()}
	}
	if d.Root.Columns, err = count(attrs, "columns"); err != nil {
		return &Error{Line: lines["columns"], Msg: err.Error()}
	}
	return nil
}

// applyGroup reads a cluster's attributes; line is where it is written, for
// an error about an attribute lines does not place.
func (p *parser) applyGroup(g *diagram.Group, attrs map[string]string, lines map[string]int, line int) error {
	g.Label = label(attrs["label"], g.ID, p.name, "")
	styles := styleSet(attrs["style"])
	switch {
	case styles["lane"]:
		g.Style = diagram.Lane
	case styles["invis"]:
		g.Style = diagram.Hidden
	}
	var err error
	wrap := func(err error) error {
		return &Error{Line: lineOf(err, lines, line), Msg: fmt.Sprintf("cluster %q: %v", g.ID, err)}
	}
	if g.Colour, err = colourAttr(attrs, "pencolor", "color"); err != nil {
		return wrap(err)
	}
	if g.Fill, err = colourAttr(attrs, "fillcolor", "bgcolor"); err != nil {
		return wrap(err)
	}
	if g.Fill == nil && styles["filled"] {
		g.Fill = g.Colour
	}
	for _, f := range []struct {
		name string
		to   *int
	}{{"rows", &g.Rows}, {"columns", &g.Columns}, {"row", &g.Row}, {"col", &g.Col}} {
		if *f.to, err = count(attrs, f.name); err != nil {
			return wrap(err)
		}
	}
	return nil
}

// applyNode reads a node's attributes into it. An error names the line
// that set the attribute at fault: the node's statement, or the node
// statement whose defaults it was made with.
func (p *parser) applyNode(info *nodeInfo) error {
	n, attrs := info.n, info.attrs
	wrap := func(err error) error {
		return &Error{Line: lineOf(err, info.lines, info.line), Msg: fmt.Sprintf("node %q: %v", n.ID, err)}
	}
	n.Label = label(attrs["label"], n.ID, p.name, "")
	if _, ok := attrs["label"]; ok && n.Label == "" {
		// label="" is a node with no words, where a node with no label
		// is named by its name; a Node's empty Label is the second, so
		// the first is a blank. So is a label of line breaks alone,
		// "\l", which reads as nothing once they are taken off its end.
		n.Label = " "
	}
	n.Shape = shape(attrs["shape"])
	st := styleSet(attrs["style"])
	n.Line, n.Invisible = line(st), st["invis"]
	if st["rounded"] && n.Shape == diagram.Box {
		n.Shape = diagram.Rounded
	}
	var err error
	if n.Colour, err = colourAttr(attrs, "color"); err != nil {
		return wrap(err)
	}
	if n.Text, err = colourAttr(attrs, "fontcolor"); err != nil {
		return wrap(err)
	}
	// Filled only with style=filled, as Graphviz fills: in fillcolor, or
	// else color, or else Graphviz's light grey.
	fill, err := colourAttr(attrs, "fillcolor", "color")
	if err != nil {
		return wrap(err)
	}
	if st["filled"] {
		n.Fill = fill
		if fill == nil {
			n.Fill = color.RGBA{0xD3, 0xD3, 0xD3, 0xFF}
		}
	}
	if n.Row, err = count(attrs, "row"); err != nil {
		return wrap(err)
	}
	if n.Col, err = count(attrs, "col"); err != nil {
		return wrap(err)
	}
	return nil
}

// applyEdge reads an edge's attributes into e; lines are where each was
// set.
func (p *parser) applyEdge(e *diagram.Edge, attrs map[string]string, lines map[string]int) error {
	e.Label = label(attrs["label"], "", p.name, "")
	st := styleSet(attrs["style"])
	e.Line, e.Invisible = line(st), st["invis"]
	switch strings.ToLower(attrs["dir"]) {
	case "forward":
		e.Head, e.Tail = true, false
	case "back":
		e.Head, e.Tail = false, true
	case "both":
		e.Head, e.Tail = true, true
	case "none":
		e.Head, e.Tail = false, false
	}
	if strings.EqualFold(attrs["arrowhead"], "none") {
		e.Head = false
	}
	if strings.EqualFold(attrs["arrowtail"], "none") {
		e.Tail = false
	}
	e.FromSide = portSide(strings.Split(attrs["tailport"], ":"))
	e.ToSide = portSide(strings.Split(attrs["headport"], ":"))
	c, err := colourAttr(attrs, "color")
	if err != nil {
		return &Error{Line: lineOf(err, lines, p.tok.line), Msg: fmt.Sprintf("edge %s to %s: %v", e.From, e.To, err)}
	}
	e.Colour = c
	return nil
}

// label is a label attribute's text, with DOT's escapes: \n, \l and \r each
// end a line -- Graphviz centres, left-aligns or right-aligns it; here every
// line is centred -- \N is the node's name and \G the graph's. An absent
// label is def.
func label(v, name, graph, def string) string {
	if v == "" {
		return def
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] != '\\' || i+1 == len(v) {
			b.WriteByte(v[i])
			continue
		}
		i++
		switch v[i] {
		case 'n', 'l', 'r':
			if i+1 < len(v) || v[i] == 'n' {
				b.WriteByte('\n')
			}
		case 'N':
			b.WriteString(name)
		case 'G':
			b.WriteString(graph)
		case '\\':
			b.WriteByte('\\')
		default:
			b.WriteByte('\\')
			b.WriteByte(v[i])
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// shape is a DOT shape's nearest of the shapes drawn here.
func shape(v string) diagram.Shape {
	switch strings.ToLower(v) {
	case "ellipse", "oval", "circle", "doublecircle", "point", "egg":
		return diagram.Ellipse
	case "cylinder":
		return diagram.Cylinder
	case "plaintext", "plain", "none", "underline":
		return diagram.Plain
	case "mrecord":
		return diagram.Rounded
	}
	return diagram.Box
}

// styleSet is a style attribute's styles, "filled,dashed" as a set.
func styleSet(v string) map[string]bool {
	set := map[string]bool{}
	for _, s := range strings.Split(v, ",") {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			set[s] = true
		}
	}
	return set
}

func line(st map[string]bool) diagram.Line {
	switch {
	case st["dashed"]:
		return diagram.Dashed
	case st["dotted"]:
		return diagram.Dotted
	case st["bold"]:
		return diagram.Bold
	}
	return diagram.Solid
}

// attrError is an attribute's value read wrongly, naming the attribute,
// so the error can say the line that set it.
type attrError struct {
	name string
	msg  string
}

func (e *attrError) Error() string { return e.msg }

// lineOf is the line that set the attribute err is about, or def.
func lineOf(err error, lines map[string]int, def int) int {
	var ae *attrError
	if errors.As(err, &ae) {
		if l, ok := lines[ae.name]; ok && l > 0 {
			return l
		}
	}
	return def
}

// count is a whole-number attribute from 1 to diagram.MaxGrid, or 0 where
// absent.
func count(attrs map[string]string, name string) (int, error) {
	v, ok := attrs[name]
	if !ok {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 || n > diagram.MaxGrid {
		return 0, &attrError{name, fmt.Sprintf("%s=%q: a whole number from 1 to %d", name, v, diagram.MaxGrid)}
	}
	return n, nil
}

// colourAttr is the colour of the first of names the attributes give.
func colourAttr(attrs map[string]string, names ...string) (color.Color, error) {
	for _, n := range names {
		if v, ok := attrs[n]; ok && v != "" {
			c, err := parseColour(v)
			if err != nil {
				return nil, &attrError{n, fmt.Sprintf("%s=%q: %v", n, v, err)}
			}
			return c, nil
		}
	}
	return nil, nil
}

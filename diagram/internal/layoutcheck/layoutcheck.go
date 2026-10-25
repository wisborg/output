// Package layoutcheck checks the promises a diagram's layout keeps about
// where its lines run and end, for tests alone.
//
// It holds the rules any correct layout keeps, not one right answer: a
// change to the router may move a line's end a cell along, take another
// path or draw an edge straight that was not, and the layout is no worse
// for it; a line through a box, or ending where a record's divider meets
// its outline, is wrong whatever drew it. So it is written apart from the
// code it checks, and works out what it needs -- where the joints are,
// which side a line leaves by, whether it is beside its field -- from the
// layout's boxes and paths alone: a check asking the router's own answer
// passes whenever the router and the answer are wrong together.
//
// It sees a layout as plain data, Layout, not as a diagram.Layout: the
// diagram package's own tests use it, and a package they import may not
// import diagram. Each test package turns a diagram.Layout into one.
package layoutcheck

import "fmt"

// Rect is a rectangle of cells, as diagram's: X and Y its top-left cell,
// W and H how many across and down.
type Rect struct{ X, Y, W, H int }

// Point is a cell.
type Point struct{ X, Y int }

// Side is a side of a box, as diagram's: North, East, South and West
// or'd together, zero for any.
type Side uint8

const (
	North Side = 1 << iota
	East
	South
	West
)

// Layout is what Check reads of a laid-out diagram.
type Layout struct {
	Boxes []Box
	Edges []Edge
}

// Box is a node as placed.
type Box struct {
	ID   string
	Rect Rect
	// Plain is a node drawn without an outline, whose corners a line may
	// end at: there is no corner for it to be mistaken for.
	Plain bool
	// Invisible keeps its place and draws nothing: a line may run where
	// it is, and it has no joints.
	Invisible bool
	// Dividers are a record's lines between its fields, each from one
	// end to the other.
	Dividers [][2]Point
}

// Edge is an edge as placed.
type Edge struct {
	From, To string
	// Points is the path, from a cell of From's outline to one of To's,
	// along rows and columns. Empty for an edge drawn nowhere.
	Points []Point
	// FromSide and ToSide are the sides asked for, FromField and ToField
	// the fields of a record asked for, as placed: the zero Rect where
	// none is, or the port names no field.
	FromSide, ToSide   Side
	FromField, ToField Rect
	SidesDropped       bool
}

// Options say what is known of how the layout was drawn.
type Options struct {
	// Plain says every path was drawn along the plain path between its
	// boxes, the search having given up: whatever is in the way, so a line
	// may pass through a box, and may end where a divider meets an outline
	// where every move off it would take the line through one.
	Plain bool
}

// Check is every promise lay breaks, each said in a line; none where it
// keeps them all.
func Check(lay Layout, o Options) []string {
	var bad []string
	say := func(e Edge, f string, a ...any) {
		bad = append(bad, fmt.Sprintf("%s -> %s: ", e.From, e.To)+fmt.Sprintf(f, a...))
	}
	box := map[string]Box{}
	joint := map[Point]bool{}
	for _, b := range lay.Boxes {
		box[b.ID] = b
		for _, j := range joints(b) {
			joint[j] = true
		}
	}
	// inBox is the visible box p lies in, if any.
	inBox := func(p Point) (Box, bool) {
		for _, b := range lay.Boxes {
			if !b.Invisible && in(b.Rect, p) {
				return b, true
			}
		}
		return Box{}, false
	}
	for _, e := range lay.Edges {
		n := len(e.Points)
		if n == 0 {
			continue
		}
		if n < 2 {
			say(e, "a path of one point, %v", e.Points)
			continue
		}
		from, to := box[e.From], box[e.To]
		for k := 1; k < n; k++ {
			if p, q := e.Points[k-1], e.Points[k]; p.X != q.X && p.Y != q.Y {
				say(e, "%v to %v runs along neither a row nor a column", p, q)
			}
		}
		for _, end := range []struct {
			at, next Point
			b        Box
			what     string
		}{{e.Points[0], e.Points[1], from, "starts"}, {e.Points[n-1], e.Points[n-2], to, "ends"}} {
			r := end.b.Rect
			if !onOutline(r, end.at) {
				say(e, "%s at %v, off %s's outline %v", end.what, end.at, end.b.ID, r)
				continue
			}
			if !end.b.Plain && corner(r, end.at) {
				say(e, "%s at %v, a corner of %s", end.what, end.at, end.b.ID)
			}
			if in(r, end.next) {
				say(e, "%s at %v and runs on into %s, to %v", end.what, end.at, end.b.ID, end.next)
			}
			if joint[end.at] && !(o.Plain && boxedIn(e.Points, end.what == "starts", lay.Boxes, box, e, joint)) {
				say(e, "%s at %v, where a divider of %s meets its outline", end.what, end.at, end.b.ID)
			}
		}
		if !o.Plain {
			cells(e.Points, func(p Point, end bool) {
				if b, ok := inBox(p); ok && !(end && (b.ID == e.From || b.ID == e.To)) {
					say(e, "runs through %s at %v", b.ID, p)
				}
			})
		}
		sided := e.FromSide != 0 || e.ToSide != 0 || e.FromField != (Rect{}) || e.ToField != (Rect{})
		keeps := keepsSide(e.FromSide, e.Points[0], e.Points[1]) && keepsSide(e.ToSide, e.Points[n-1], e.Points[n-2]) &&
			keepsField(from.Rect, e.FromField, e.Points[0], e.Points[1]) && keepsField(to.Rect, e.ToField, e.Points[n-1], e.Points[n-2])
		if want := sided && !keeps; e.SidesDropped != want {
			say(e, "SidesDropped %v, but the path %s what it asks for: from %v by side %v beside %v, to %v by side %v beside %v",
				e.SidesDropped, map[bool]string{true: "keeps", false: "does not keep"}[keeps],
				e.Points[0], e.FromSide, e.FromField, e.Points[n-1], e.ToSide, e.ToField)
		}
	}
	return bad
}

// Joints is where b's dividers meet its outline -- none for an invisible
// box, whose dividers are not drawn -- worked out from its dividers alone.
func Joints(b Box) []Point { return joints(b) }

func joints(b Box) []Point {
	if b.Invisible {
		return nil
	}
	var js []Point
	for _, d := range b.Dividers {
		for _, p := range d {
			if onOutline(b.Rect, p) {
				js = append(js, p)
			}
		}
	}
	return js
}

func in(r Rect, p Point) bool {
	return p.X >= r.X && p.X < r.X+r.W && p.Y >= r.Y && p.Y < r.Y+r.H
}

func onOutline(r Rect, p Point) bool {
	return in(r, p) && (p.X == r.X || p.X == r.X+r.W-1 || p.Y == r.Y || p.Y == r.Y+r.H-1)
}

func corner(r Rect, p Point) bool {
	return (p.X == r.X || p.X == r.X+r.W-1) && (p.Y == r.Y || p.Y == r.Y+r.H-1)
}

// cells calls f with every cell of the path pts, saying whether it is one
// of the path's two ends. A stretch along neither a row nor a column,
// which Check says of, is passed over: walked a cell across and down at a
// time, one not as many across as down never reaches its end.
func cells(pts []Point, f func(p Point, end bool)) {
	n := len(pts)
	for k := 1; k < n; k++ {
		p, q := pts[k-1], pts[k]
		if p.X != q.X && p.Y != q.Y {
			continue
		}
		dx, dy := sign(q.X-p.X), sign(q.Y-p.Y)
		for {
			if k == 1 || p != pts[k-1] {
				f(p, p == pts[0] || p == pts[n-1])
			}
			if p == q {
				break
			}
			p.X, p.Y = p.X+dx, p.Y+dy
		}
	}
}

// way is the way out of its box a path leaves by, from its end at to the
// point after it: 0 up, 1 down, 2 left, 3 right.
func way(at, next Point) int {
	switch {
	case next.Y < at.Y:
		return 0
	case next.Y > at.Y:
		return 1
	case next.X < at.X:
		return 2
	}
	return 3
}

var waySide = [4]Side{North, South, West, East}

func keepsSide(s Side, at, next Point) bool {
	return s == 0 || s&waySide[way(at, next)] != 0
}

// keepsField says a path leaving r at at, towards next, is beside the
// field f, the zero Rect for none: across from it, on a side of r the
// field reaches -- or on any side where it reaches none, a field between
// others on every side.
func keepsField(r, f Rect, at, next Point) bool {
	if f == (Rect{}) {
		return true
	}
	reaches := [4]bool{f.Y == r.Y+1, f.Y+f.H == r.Y+r.H-1, f.X == r.X+1, f.X+f.W == r.X+r.W-1}
	if reaches == [4]bool{} {
		reaches = [4]bool{true, true, true, true}
	}
	w := way(at, next)
	if !reaches[w] {
		return false
	}
	if w < 2 {
		return at.X >= f.X && at.X < f.X+f.W
	}
	return at.Y >= f.Y && at.Y < f.Y+f.H
}

// boxedIn says the end of the plain path pts at its start, or its end,
// has nowhere off a joint to go: no move of it a cell to three along its
// side, the point after it moved with it, puts it on its box's outline off
// the corners and the joints -- the other end too, where the path is that
// one stretch -- with the stretches either side of each moved point
// passing through no box but at the path's own two ends. The plain path
// then stays where it is, a line through a box being the worse fault.
func boxedIn(pts []Point, start bool, boxes []Box, owner map[string]Box, e Edge, joint map[Point]bool) bool {
	n := len(pts)
	k, next := n-1, n-2
	if start {
		k, next = 0, 1
	}
	fine := func(q []Point, m int) bool {
		r := owner[e.From].Rect
		if m == n-1 {
			r = owner[e.To].Rect
		}
		return onOutline(r, q[m]) && !corner(r, q[m]) && !joint[q[m]]
	}
	clear := func(q []Point, m int) bool {
		for _, o := range []int{m - 1, m + 1} {
			if o < 0 || o >= n {
				continue
			}
			lo, hi := min(m, o), max(m, o)
			through := false
			cells(q[lo:hi+1], func(p Point, _ bool) {
				if p == q[0] || p == q[n-1] {
					return
				}
				for _, b := range boxes {
					through = through || !b.Invisible && in(b.Rect, p)
				}
			})
			if through {
				return false
			}
		}
		return true
	}
	for _, d := range []int{1, -1, 2, -2, 3, -3} {
		q := append([]Point(nil), pts...)
		for _, m := range []int{k, next} {
			if pts[k].X == pts[next].X {
				q[m].X += d
			} else {
				q[m].Y += d
			}
		}
		if fine(q, k) && (n > 2 || fine(q, next)) && clear(q, k) && clear(q, next) {
			return false
		}
	}
	return true
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}

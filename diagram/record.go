package diagram

import "strings"

// A record is laid out in cells as any node is: its outline, and inside it
// its fields, each a space either side of its widest line, divided by a
// line of one cell. A row of fields is as tall as its tallest, and a
// column as wide as its widest; what a field is given past its own size it
// shares with the fields beside it, so their dividers stay where the
// eye expects them, spread evenly, rather than one field taking the rest.

// fieldsSize is the cells fields take inside a record's outline, laid in a
// row, or a column where row is false.
func fieldsSize(fields []Field, row bool) size {
	_, s := measure(fields, row)
	return s
}

// measured is the cells a field takes, and those of the fields it is
// divided into: worked out once, from the fields not divided up, so a
// field is measured once however deep it lies, rather than again for each
// field it is part of.
type measured struct {
	size   size
	fields []measured
}

// measure measures fields, laid in a row or a column, and is each one's
// cells and theirs all together, a divider of a cell between each two.
func measure(fields []Field, row bool) ([]measured, size) {
	ms := make([]measured, len(fields))
	var s size
	for i, f := range fields {
		if len(f.Fields) > 0 {
			ms[i].fields, ms[i].size = measure(f.Fields, !row)
		} else {
			lines := strings.Split(f.Label, "\n")
			w := 0
			for _, l := range lines {
				w = max(w, Width(l))
			}
			ms[i].size = size{w + 2, len(lines)}
		}
		fs := ms[i].size
		if row {
			s.w += fs.w
			s.h = max(s.h, fs.h)
		} else {
			s.w = max(s.w, fs.w)
			s.h += fs.h
		}
		if i > 0 {
			if row {
				s.w++
			} else {
				s.h++
			}
		}
	}
	return ms, size{max(s.w, 1), max(s.h, 1)}
}

// placeFields places fields in r, the inside of a record's outline or of a
// field divided further, in a row or a column, adding each field and each
// divider to pn. A divider runs a cell past r at either end, onto the
// outline or the divider beside r, so a drawing joins it to them.
func placeFields(pn *PlacedNode, fields []Field, r Rect, row bool) {
	ms, _ := measure(fields, row)
	place(pn, fields, ms, r, row)
}

// place is placeFields, with each field's cells measured already.
func place(pn *PlacedNode, fields []Field, ms []measured, r Rect, row bool) {
	n := len(fields)
	sizes := make([]int, n)
	total := n - 1 // the dividers
	for i := range fields {
		if row {
			sizes[i] = ms[i].size.w
		} else {
			sizes[i] = ms[i].size.h
		}
		total += sizes[i]
	}
	room := r.W
	if !row {
		room = r.H
	}
	// The room past their own sizes, shared out, the last fields taking
	// a cell more where it does not divide.
	extra := room - total
	at := r.X
	if !row {
		at = r.Y
	}
	for i, f := range fields {
		k := sizes[i] + extra/n
		if i >= n-extra%n {
			k++
		}
		fr := Rect{at, r.Y, k, r.H}
		if !row {
			fr = Rect{r.X, at, r.W, k}
		}
		if len(f.Fields) > 0 {
			place(pn, f.Fields, ms[i].fields, fr, !row)
		} else {
			pn.Fields = append(pn.Fields, PlacedField{Field: &fields[i], Rect: fr})
		}
		at += k
		if i < n-1 {
			if row {
				pn.Dividers = append(pn.Dividers, [2]Point{{at, r.Y - 1}, {at, r.Y + r.H}})
			} else {
				pn.Dividers = append(pn.Dividers, [2]Point{{r.X - 1, at}, {r.X + r.W, at}})
			}
			at++
		}
	}
}

// Lines is f's label, a line to each "\n", and the row the first is
// written in, the lines centred in f top to bottom: the one place a
// drawing finds where a field's words go, as a node's are found in the
// layout, so the text drawing and the pictures put them in the same rows.
func (f PlacedField) Lines() (lines []string, top int) {
	lines = strings.Split(f.Field.Label, "\n")
	return lines, f.Rect.Y + (f.Rect.H-len(lines))/2
}

// field is the cells of n's field named port, and whether it has one.
func (n PlacedNode) field(port string) (Rect, bool) {
	if n.Node == nil {
		return Rect{}, false
	}
	want := n.Node.Field(port)
	for _, f := range n.Fields {
		if want != nil && f.Field == want {
			return f.Rect, true
		}
	}
	return Rect{}, false
}

// besideField says the outline cell p of r, left the way way, is beside
// the field f: across from it on a side of r it reaches, or, where it
// reaches none -- a field between others on every side -- on any side.
func besideField(r, f Rect, p Point, way int) bool {
	reaches := [4]bool{f.Y == r.Y+1, f.Y+f.H == r.Y+r.H-1, f.X == r.X+1, f.X+f.W == r.X+r.W-1}
	if !reaches[0] && !reaches[1] && !reaches[2] && !reaches[3] {
		reaches = [4]bool{true, true, true, true}
	}
	if !reaches[way] {
		return false
	}
	if way < 2 {
		return p.X >= f.X && p.X < f.X+f.W
	}
	return p.Y >= f.Y && p.Y < f.Y+f.H
}

// move moves n, its fields and its dividers dx cells across and dy rows
// down. The fields and dividers are moved where they are, and so for every
// copy of n, which share them: each node is moved once.
func (n *PlacedNode) move(dx, dy int) {
	n.Rect.X, n.Rect.Y = n.Rect.X+dx, n.Rect.Y+dy
	for i := range n.Fields {
		n.Fields[i].Rect.X += dx
		n.Fields[i].Rect.Y += dy
	}
	for i := range n.Dividers {
		for j := range n.Dividers[i] {
			n.Dividers[i][j].X += dx
			n.Dividers[i][j].Y += dy
		}
	}
}

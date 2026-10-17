package diagram

import (
	"image/color"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// The text drawing of two regions, a link within the first and a link from
// it to the second: the line crossing the lanes' line meets it in a cross,
// the edges leave their boxes through a tee in the outline and arrive at an
// arrowhead, and each lane's label is on its own row.
func TestTextDrawsLanes(t *testing.T) {
	d := &Diagram{
		Root: Group{Rows: 2, Items: []Item{
			{Group: &Group{ID: "east", Style: Lane, Label: "us-east-1", Columns: 2, Items: []Item{node("dbA"), node("dbB")}}},
			{Group: &Group{ID: "west", Style: Lane, Label: "us-west-2", Columns: 2, Items: []Item{node("dbC")}}},
		}},
		Edges: []Edge{{From: "dbA", To: "dbB", Head: true}, {From: "dbA", To: "dbC", Head: true}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	lines := lay.Text()
	text := strings.Join(lines, "\n")
	for _, want := range []string{"│ dbA ├─────▶│ dbB │", "┼", "▼", "us-east-1", "us-west-2", "└──┬──┘"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}
	for i, l := range lines {
		if n := utf8.RuneCountInString(l); n > lay.Width {
			t.Errorf("line %d is %d characters, wider than the layout's %d", i, n, lay.Width)
		}
	}
	if len(lines) != lay.Height {
		t.Errorf("%d lines, want the layout's %d", len(lines), lay.Height)
	}
}

// Dashed lines are drawn dashed, an invisible node keeps its cell and draws
// nothing, and a frame's label is set in its top edge.
func TestTextStyles(t *testing.T) {
	d := &Diagram{
		Title: "t",
		Root: Group{Items: []Item{{Group: &Group{ID: "f", Label: "frame", Columns: 3, Items: []Item{
			node("a"), {Node: &Node{ID: "gap", Invisible: true}}, {Node: &Node{ID: "b", Shape: Rounded}},
		}}}}},
		Edges: []Edge{{From: "a", To: "b", Line: Dashed}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lay.Text(), "\n")
	for _, want := range []string{"t\n", "┌─ frame ─", "╭───╮", "╌"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "gap") {
		t.Errorf("the invisible node is drawn:\n%s", text)
	}
}

var escape = regexp.MustCompile("\x1b\\[[0-9;]*m")

// In colour, the drawing is the same characters as without, a fill is
// behind the inside of its box and the label on it is in the ink that
// stands out from it, and every line ends with the terminal's own colours
// back.
func TestColourText(t *testing.T) {
	green := color.RGBA{0x8F, 0xD1, 0x8F, 0xFF}
	d := &Diagram{
		Root: Group{Rows: 2, Items: []Item{
			{Group: &Group{ID: "east", Style: Lane, Label: "east", Columns: 2, Items: []Item{
				{Node: &Node{ID: "a", Fill: green}}, node("b")}}},
			{Group: &Group{ID: "west", Style: Lane, Label: "west", Columns: 2, Items: []Item{node("c")}}},
		}},
		Edges: []Edge{{From: "a", To: "b", Head: true, Label: "sync"}, {From: "a", To: "c", Head: true, Colour: color.RGBA{0xFF, 0, 0, 0xFF}}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	plain, coloured := lay.Text(), lay.ColourText(Dark)
	if len(plain) != len(coloured) {
		t.Fatalf("%d lines plain, %d in colour", len(plain), len(coloured))
	}
	for i := range plain {
		if got := strings.TrimRight(escape.ReplaceAllString(coloured[i], ""), " "); got != plain[i] {
			t.Errorf("line %d is %q in colour, %q plain", i, got, plain[i])
		}
		fg, bg := false, false
		for _, e := range escape.FindAllString(coloured[i], -1) {
			switch {
			case e == "\x1b[0m":
				fg, bg = false, false
			case e == "\x1b[39m":
				fg = false
			case e == "\x1b[49m":
				bg = false
			case strings.HasPrefix(e, "\x1b[38;"):
				fg = true
			case strings.HasPrefix(e, "\x1b[48;"):
				bg = true
			}
		}
		if fg || bg {
			t.Errorf("line %d leaves its colour set: %q", i, coloured[i])
		}
	}
	a, _ := lay.Node("a")
	label := coloured[a.Rect.Y+1]
	fill, ink := "\x1b[48;2;143;209;143m", "\x1b[38;2;14;14;16m" // the green; Dark's background
	if i, j := strings.Index(label, fill), strings.Index(label, ink+"a"); i < 0 || j < i {
		t.Errorf("a's label line %q: want the green behind it, then a in dark ink", label)
	}
	all := strings.Join(coloured, "\n")
	for _, want := range []string{"\x1b[38;2;255;0;0m", "\x1b[38;2;138;138;150msync", "\x1b[38;2;138;138;150meast"} {
		if !strings.Contains(all, want) {
			t.Errorf("no %q: the red edge, the dim edge label and lane label", want)
		}
	}
}

// Nothing coloured, nothing to escape: a diagram without colours, groups or
// labels on its edges is the same in colour as plain.
func TestColourTextWithoutColours(t *testing.T) {
	d := &Diagram{Root: Group{Items: []Item{node("a"), node("b")}}, Edges: []Edge{{From: "a", To: "b", Head: true}}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if plain, coloured := strings.Join(lay.Text(), "\n"), strings.Join(lay.ColourText(Light), "\n"); plain != coloured {
		t.Errorf("in colour:\n%q\nplain:\n%q", coloured, plain)
	}
}

// A legend's entries are drawn as a block for a fill, a line -- dashed for
// a dashed one -- for a stroke, each with its label.
func TestTextLegend(t *testing.T) {
	d := &Diagram{Root: Group{Items: []Item{node("a")}}, Legend: &Legend{Label: "Key", Entries: []LegendEntry{
		{Label: "ok", Fill: color.RGBA{0, 0xFF, 0, 0xFF}},
		{Label: "sync", Sample: Stroke},
		{Label: "async", Sample: Stroke, Line: Dashed},
	}}}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lay.Text(), "\n")
	for _, want := range []string{"┌─ Key ─", "│  ████  ok", "│  ────  sync", "│  ╌╌╌╌  async"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}
	if coloured := strings.Join(lay.ColourText(Light), "\n"); !strings.Contains(coloured, "\x1b[38;2;0;255;0m████") {
		t.Errorf("the swatch is not green in colour:\n%q", coloured)
	}
}

// brailleDots counts the dots of the braille characters in lines.
func brailleDots(lines []string) (cells, dots int) {
	for _, l := range lines {
		for _, r := range l {
			if r > 0x2800 && r <= 0x28FF {
				cells++
				for b := r - 0x2800; b != 0; b &= b - 1 {
					dots++
				}
			}
		}
	}
	return cells, dots
}

// A straight edge is drawn along rows and columns unless braille is asked
// for; then at its angle in braille dots -- more of them thick than thin --
// ending in an arrowhead of the kind every edge ends in, on a stub against
// its head's box.
func TestTextBraille(t *testing.T) {
	d := &Diagram{
		Root:     Group{Columns: 2, Items: []Item{node("a"), node("b"), node("c"), node("d")}},
		Edges:    []Edge{{From: "a", To: "d", Head: true}},
		Straight: true,
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	if cells, _ := brailleDots(lay.Text()); cells != 0 {
		t.Errorf("braille without asking:\n%s", strings.Join(lay.Text(), "\n"))
	}
	thin := lay.Text(TextOptions{Braille: BrailleThin})
	thick := lay.Text(TextOptions{Braille: BrailleThick})
	_, thinDots := brailleDots(thin)
	_, thickDots := brailleDots(thick)
	if thinDots == 0 || thickDots <= thinDots {
		t.Errorf("%d dots thin, %d thick: want some, and more thick:\n%s\n\n%s", thinDots, thickDots, strings.Join(thin, "\n"), strings.Join(thick, "\n"))
	}
	// The arrowhead: ▼ just above d, or ▶ just left of it.
	dn, _ := lay.Node("d")
	r := dn.Rect
	found := false
	for y, l := range thin {
		for x, ch := range []rune(l) {
			above := ch == '▼' && y == r.Y-1 && x > r.X && x < r.X+r.W-1
			beside := ch == '▶' && x == r.X-1 && y > r.Y && y < r.Y+r.H-1
			found = found || above || beside
		}
	}
	if !found {
		t.Errorf("no arrowhead on a stub against d at %v:\n%s", r, strings.Join(thin, "\n"))
	}
}

func TestBrailleDot(t *testing.T) {
	// ⠁⠂⠄ down the left, ⠈⠐⠠ down the right, then ⡀ and ⢀ beneath.
	for _, c := range []struct {
		col, row int
		want     rune
	}{{0, 0, '⠁'}, {0, 1, '⠂'}, {0, 2, '⠄'}, {1, 0, '⠈'}, {1, 1, '⠐'}, {1, 2, '⠠'}, {0, 3, '⡀'}, {1, 3, '⢀'}} {
		if got := rune(0x2800) + rune(brailleDot(c.col, c.row)); got != c.want {
			t.Errorf("dot %d,%d is %c, want %c", c.col, c.row, got, c.want)
		}
	}
}

// A label's control characters are not written to the terminal: an escape
// in a label someone else wrote would be a command to it, and a tab or a
// carriage return breaks the box it is in. Each is one cell of U+FFFD, so
// the box still closes where the layout put it.
func TestTextWritesNoControlCharacters(t *testing.T) {
	d := &Diagram{
		Title: "t\x1b]0;title\a",
		Root:  Group{Columns: 2, Items: []Item{node("a\x1b[2Jb"), node("c\td\re")}},
		Edges: []Edge{{From: "a\x1b[2Jb", To: "c\td\re", Head: true, Label: "x\x9by"}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	for _, colour := range []bool{false, true} {
		lines := lay.Text()
		if colour {
			lines = lay.ColourText(Dark)
		}
		text := strings.Join(lines, "\n")
		plain := regexp.MustCompile("\x1b\\[[0-9;]*m").ReplaceAllString(text, "")
		for _, r := range plain {
			if r != '\n' && (r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0) {
				t.Errorf("colour %v: control character %U in:\n%q", colour, r, text)
			}
		}
		for _, want := range []string{"│ a�[2Jb ├", "│ c�d�e │", "x�y"} {
			if !strings.Contains(plain, want) {
				t.Errorf("colour %v: no %q in:\n%s", colour, want, plain)
			}
		}
	}
}

// A label is measured in a terminal's columns: a CJK character two, a
// combining accent none, a flag two. Counted in runes, a CJK label
// overflowed its box and pushed the boxes after it on its line out of true;
// a combining accent made its box a column too wide.
func TestTextMeasuresLabelsInColumns(t *testing.T) {
	for in, want := range map[string]int{"abc": 3, "日本": 4, "e\u0301t\u00e9": 3, "🇩🇰": 2, "a\x1bb": 3, "±°": 2} {
		if got := Width(in); got != want {
			t.Errorf("Width(%q) = %d, want %d", in, got, want)
		}
	}
	d := &Diagram{
		Root:  Group{Columns: 3, Items: []Item{node("日本"), node("e\u0301te\u0301"), node("🇩🇰 漢字")}},
		Edges: []Edge{{From: "日本", To: "e\u0301te\u0301", Head: true, Label: "東京"}},
	}
	lay, err := d.Layout()
	if err != nil {
		t.Fatal(err)
	}
	lines := lay.Text()
	text := strings.Join(lines, "\n")
	for _, want := range []string{"│ 日本 ├", "│ e\u0301te\u0301 │", "│ 🇩🇰 漢字 │", "東京"} {
		if !strings.Contains(text, want) {
			t.Errorf("no %q in:\n%s", want, text)
		}
	}
	// Every box's corners and sides line up: each column holding a corner
	// in one row holds the box's side or other corner in the rows of its
	// height. Checked on the display columns, not the runes.
	cols := func(l string) map[int]rune {
		m := map[int]rune{}
		x := 0
		for _, r := range l {
			m[x] = r
			x += Width(string(r))
		}
		return m
	}
	for i := range lines {
		row := cols(lines[i])
		for x, r := range row {
			if r != '┌' && r != '┐' {
				continue
			}
			for j := i + 1; j < len(lines); j++ {
				below := cols(lines[j])[x]
				if below == '└' || below == '┘' {
					break
				}
				if below != '│' && below != '├' && below != '┤' && below != '┼' {
					t.Errorf("the corner at row %d, column %d has %q beneath it in row %d:\n%s", i, x, below, j, text)
					break
				}
			}
		}
	}
	for i, l := range lines {
		if w := Width(l); w > lay.Width {
			t.Errorf("line %d is %d columns, wider than the layout's %d", i, w, lay.Width)
		}
	}
}

// A straight edge drawn in braille that runs along a row or down a column is
// drawn as a line, with its heads pointing the way it runs: into its head's
// box, and out of the tail's back into it. No test drew one, so a head
// pointing the wrong way, or one not drawn, went unseen.
func TestBrailleStraightHeads(t *testing.T) {
	for _, c := range []struct {
		name       string
		root       Group
		head, tail rune
	}{
		{"across", Group{Columns: 2, Items: []Item{node("a"), node("b")}}, '▶', '◀'},
		{"down", Group{Columns: 1, Items: []Item{node("a"), node("b")}}, '▼', '▲'},
	} {
		d := &Diagram{Straight: true, Root: c.root, Edges: []Edge{{From: "a", To: "b", Head: true, Tail: true}}}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(lay.Text(TextOptions{Braille: BrailleThin}), "\n")
		h, tl := strings.IndexRune(text, c.head), strings.IndexRune(text, c.tail)
		a, b := strings.Index(text, " a "), strings.Index(text, " b ")
		if h < 0 || tl < 0 {
			t.Errorf("%s: no %c and %c in:\n%s", c.name, c.head, c.tail, text)
			continue
		}
		if !(tl < h && a < tl && h < b) {
			t.Errorf("%s: a at %d, %c at %d, %c at %d, b at %d:\n%s", c.name, a, c.tail, tl, c.head, h, b, text)
		}
	}
}

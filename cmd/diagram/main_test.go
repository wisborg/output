package main

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/wisborg/output/diagram"
	"github.com/wisborg/output/diagram/dot"
)

func noEnv(string) string { return "" }

const regions = `digraph {
	rows = 2
	subgraph cluster_east { label = "us-east-1"; style = lane; columns = 2; dbA; dbB }
	subgraph cluster_west { label = "us-west-2"; style = lane; columns = 2; dbC }
	dbA -> dbB
	dbA -> dbC
}`

func draw(t *testing.T, src string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = run(args, strings.NewReader(src), &out, &errOut, noEnv)
	return out.String(), errOut.String(), err
}

// Not on a terminal, the diagram is drawn as text.
func TestDiagramDrawsText(t *testing.T) {
	out, _, err := draw(t, regions)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"│ dbA ├─────▶│ dbB │", "┼", "us-west-2"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
}

// --out writes the format its name ends in.
func TestDiagramWritesFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"d.png", "d.svg", "d.txt"} {
		path := filepath.Join(dir, name)
		_, stderr, err := draw(t, regions, "--out", path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr, path) {
			t.Errorf("stderr %q does not name %s", stderr, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		switch filepath.Ext(name) {
		case ".png":
			if _, err := png.Decode(bytes.NewReader(data)); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		case ".svg":
			if !bytes.HasPrefix(data, []byte("<svg ")) {
				t.Errorf("%s begins %q", name, data[:min(len(data), 20)])
			}
		case ".txt":
			if !bytes.Contains(data, []byte("dbC")) {
				t.Errorf("%s:\n%s", name, data)
			}
		}
	}
}

func TestDiagramRefuses(t *testing.T) {
	for _, c := range []struct {
		src  string
		args []string
		want string
	}{
		{regions, []string{"--out", "d.jpg"}, "name it .png, .svg or .txt"},
		{regions, []string{"--terminal", "braille"}, "use auto, iterm, kitty or text"},
		{regions, []string{"--theme", "pink"}, "--theme"},
		{regions, []string{"--width", "-1"}, "--width -1"},
		{regions, []string{"--color", "sometimes"}, "use auto, always or never"},
		{regions, []string{"--edges", "curvy"}, "use auto, straight or orthogonal"},
		{regions, []string{"--braille", "bold"}, "use off, thin or thick"},
		{regions, []string{"--text-size", "20", "--width", "800"}, "one or the other"},
		{"digraph {\n a -- b\n}", nil, "standard input: line 2"},
		{"digraph { subgraph cluster_a { columns=1; a [col=1]; b [col=1, row=1] } }", nil, "both pinned"},
		// Sizes that are not numbers, or that make a picture of terabytes:
		// NaN passed a test of <= 0 and wrote width="NaN" into an SVG.
		{regions, []string{"--text-size", "NaN", "--out", "d.svg"}, "--text-size NaN"},
		{regions, []string{"--text-size", "+Inf", "--out", "d.svg"}, "--text-size +Inf"},
		{regions, []string{"--text-size", "1e6", "--out", "d.png"}, "million"},
		{regions, []string{"--width", "200000", "--out", "d.png"}, "million"},
	} {
		_, _, err := draw(t, c.src, c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want an error saying %q", c.args, err, c.want)
		}
	}
}

// A flag after the file is a flag, as it is before it.
func TestDiagramFlagsAfterTheFile(t *testing.T) {
	dir := t.TempDir()
	src, out := filepath.Join(dir, "d.dot"), filepath.Join(dir, "d.txt")
	if err := os.WriteFile(src, []byte(regions), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := draw(t, "", src, "--out", out); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(out); err != nil || !bytes.Contains(data, []byte("dbC")) {
		t.Errorf("--out after the file wrote %q, %v", data, err)
	}
}

// A PNG or an SVG is as wide as plot's charts unless --width says, in its
// own proportions, narrower where --height would be passed; --text-size
// sizes it by its text instead.
func TestDiagramSize(t *testing.T) {
	dir := t.TempDir()
	size := func(args ...string) (int, int) {
		t.Helper()
		path := filepath.Join(dir, "d.png")
		if _, _, err := draw(t, regions, append([]string{"--out", path}, args...)...); err != nil {
			t.Fatal(err)
		}
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		cfg, err := png.DecodeConfig(f)
		if err != nil {
			t.Fatal(err)
		}
		return cfg.Width, cfg.Height
	}
	w, h := size()
	if w != imageWidth || h < 100 {
		t.Errorf("by default %d by %d, want %d wide", w, h, imageWidth)
	}
	if w2, h2 := size("--width", "800"); w2 != 800 || abs(h2*2-h) > 2 {
		t.Errorf("--width 800: %d by %d, want 800 by about %d", w2, h2, h/2)
	}
	if w3, h3 := size("--width", "800", "--height", "200"); h3 != 200 || w3 >= 800 {
		t.Errorf("--width 800 --height 200: %d by %d, want 200 tall and narrower than 800", w3, h3)
	}
	w4, _ := size("--text-size", "16")
	w5, _ := size("--text-size", "32")
	if abs(w5-2*w4) > 2 || w4 == imageWidth {
		t.Errorf("--text-size 16 is %d wide and 32 is %d: want twice as wide", w4, w5)
	}

	svgPath := filepath.Join(dir, "d.svg")
	if _, _, err := draw(t, regions, "--out", svgPath, "--width", "800"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(svgPath); err != nil || !bytes.Contains(data, []byte(`width="800"`)) {
		t.Errorf("--width 800 SVG begins %q, %v", data[:min(len(data), 80)], err)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// In the terminal the picture is --width characters wide, at two pixels to
// a character's ten where the terminal does not say its size, and is shown
// that many characters wide.
func TestDiagramTerminalWidth(t *testing.T) {
	out, _, err := draw(t, regions, "--terminal", "iterm", "--width", "40")
	if err != nil {
		t.Fatal(err)
	}
	_, data, ok := strings.Cut(strings.TrimSuffix(out, "\a\n"), "preserveAspectRatio=1:")
	if !ok || !strings.Contains(out, ";width=40;") {
		t.Fatalf("not an iTerm2 picture 40 characters wide: %.80q", out)
	}
	img, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(img))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 800 {
		t.Errorf("%d pixels wide, want 800", cfg.Width)
	}
}

// --color always draws the text in colour, to the terminal or a .txt file;
// by default neither a pipe nor a file gets colour, and NO_COLOR or a dumb
// terminal turns auto's off.
func TestDiagramColour(t *testing.T) {
	const filled = `digraph { a [style=filled, fillcolor=green]; a -> b }`
	out, _, err := draw(t, filled, "--terminal", "text")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("colour into a pipe by default:\n%q", out)
	}
	out, _, err = draw(t, filled, "--terminal", "text", "--color", "always")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "\x1b[48;2;0;255;0m") {
		t.Errorf("--color always has no green behind a:\n%q", out)
	}
	dir := t.TempDir()
	for _, c := range []struct {
		args []string
		want bool
	}{{nil, false}, {[]string{"--color", "always"}, true}} {
		path := filepath.Join(dir, "d.txt")
		if _, _, err := draw(t, filled, append([]string{"--out", path}, c.args...)...); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if got := bytes.Contains(data, []byte("\x1b[")); got != c.want {
			t.Errorf("%v: colour in the .txt file %v, want %v", c.args, got, c.want)
		}
	}
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	for _, c := range []struct {
		mode string
		tty  bool
		env  func(string) string
		want bool
	}{
		{"auto", true, noEnv, true},
		{"auto", false, noEnv, false},
		{"auto", true, env("NO_COLOR", "1"), false},
		{"auto", true, env("TERM", "dumb"), false},
		{"always", false, env("NO_COLOR", "1"), true},
		{"never", true, noEnv, false},
	} {
		if got := colour(c.mode, c.tty, c.env); got != c.want {
			t.Errorf("%s, tty %v: colour %v, want %v", c.mode, c.tty, got, c.want)
		}
	}
}

// Every example in the repository draws, as text, a PNG and an SVG: an
// example that no longer reads is worse than none.
func TestExamples(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.dot"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no examples/*.dot")
	}
	dir := t.TempDir()
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := dot.Parse(string(src))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		words := labelsOf(d)
		for _, ext := range []string{".txt", ".png", ".svg"} {
			out := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), ".dot")+ext)
			if _, _, err := draw(t, "", path, "--out", out); err != nil {
				t.Errorf("%s as %s: %v", path, ext, err)
				continue
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			// Drawn, not merely written: every label in the text and the
			// SVG, and a picture with more than its background in it.
			switch ext {
			case ".txt":
				for _, w := range words {
					if !strings.Contains(string(data), w) {
						t.Errorf("%s as text has no %q", path, w)
					}
				}
			case ".svg":
				var text strings.Builder
				dec := xml.NewDecoder(bytes.NewReader(data))
				for {
					tok, err := dec.Token()
					if err == io.EOF {
						break
					}
					if err != nil {
						t.Fatalf("%s as SVG: %v", path, err)
					}
					if cd, ok := tok.(xml.CharData); ok {
						text.Write(cd)
						text.WriteByte('\n')
					}
				}
				for _, w := range words {
					if !strings.Contains(text.String(), w) {
						t.Errorf("%s as SVG has no %q", path, w)
					}
				}
			case ".png":
				img, err := png.Decode(bytes.NewReader(data))
				if err != nil {
					t.Fatalf("%s as PNG: %v", path, err)
				}
				b := img.Bounds()
				bg, inked := img.At(0, 0), 0
				for y := b.Min.Y; y < b.Max.Y; y += 4 {
					for x := b.Min.X; x < b.Max.X; x += 4 {
						if img.At(x, y) != bg {
							inked++
						}
					}
				}
				if inked < b.Dx()*b.Dy()/16/100 {
					t.Errorf("%s as PNG is %d by %d and nearly blank: %d of its samples inked", path, b.Dx(), b.Dy(), inked)
				}
			}
		}
	}
}

// labelsOf is every line of every label d draws: its nodes', its edges',
// its clusters', its legend's.
func labelsOf(d *diagram.Diagram) []string {
	var out []string
	add := func(s string) {
		for _, l := range strings.Split(s, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
	}
	var walk func(g *diagram.Group)
	walk = func(g *diagram.Group) {
		add(g.Label)
		for _, it := range g.Items {
			if it.Node != nil {
				if it.Node.Invisible {
					continue
				}
				if it.Node.Label != "" {
					add(it.Node.Label)
				} else {
					add(it.Node.ID)
				}
			} else {
				walk(it.Group)
			}
		}
	}
	walk(&d.Root)
	for _, e := range d.Edges {
		if !e.Invisible {
			add(e.Label)
		}
	}
	if d.Legend != nil {
		add(d.Legend.Label)
		for _, e := range d.Legend.Entries {
			add(e.Label)
		}
	}
	add(d.Title)
	return out
}

// --edges straight or orthogonal says what the file's splines would have;
// auto takes the file's word. Only a picture draws straight; the text is
// the same either way.
func TestDiagramEdges(t *testing.T) {
	const src = `digraph { splines = line; columns = 2; a; b; c; d; a -> d }`
	svg := func(args ...string) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "d.svg")
		if _, _, err := draw(t, src, append([]string{"--out", path}, args...)...); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	// A path of one segment, from the corner of a towards d, is diagonal:
	// both its coordinates differ from one end to the other.
	diagonal := func(doc string) bool {
		for _, m := range regexp.MustCompile(`<path d="M([\d.]+) ([\d.]+) L([\d.]+) ([\d.]+)"`).FindAllStringSubmatch(doc, -1) {
			if m[1] != m[3] && m[2] != m[4] {
				return true
			}
		}
		return false
	}
	if !diagonal(svg()) || !diagonal(svg("--edges", "straight")) {
		t.Error("splines=line, or --edges straight, drew no diagonal")
	}
	if diagonal(svg("--edges", "orthogonal")) {
		t.Error("--edges orthogonal drew a diagonal")
	}
	a, _, _ := draw(t, src, "--terminal", "text", "--edges", "straight")
	b, _, _ := draw(t, src, "--terminal", "text", "--edges", "orthogonal")
	if strings.TrimLeft(a, "\n") != strings.TrimLeft(b, "\n") {
		t.Errorf("the text differs:\n%s\n---\n%s", a, b)
	}
}

// --braille draws the straight edges of the text drawing in braille; off,
// the default, draws them along rows and columns.
func TestDiagramBraille(t *testing.T) {
	const src = `digraph { splines = line; columns = 2; a; b; c; d; a -> d }`
	hasBraille := func(s string) bool {
		for _, r := range s {
			if r > 0x2800 && r <= 0x28FF {
				return true
			}
		}
		return false
	}
	for _, c := range []struct {
		args []string
		want bool
	}{{nil, false}, {[]string{"--braille", "off"}, false}, {[]string{"--braille", "thin"}, true}, {[]string{"--braille", "thick"}, true}, {[]string{"--braille", "thin", "--edges", "orthogonal"}, false}} {
		out, _, err := draw(t, src, append([]string{"--terminal", "text"}, c.args...)...)
		if err != nil {
			t.Fatal(err)
		}
		if got := hasBraille(out); got != c.want {
			t.Errorf("%v: braille %v, want %v:\n%s", c.args, got, c.want, out)
		}
	}
}

// In every example, drawn every way, no edge's label is written over a box:
// in a picture, where it is moved by its nudge, too.
func TestExampleLabelsClearOfBoxes(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.dot"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no examples found")
	}
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, straight := range []bool{false, true} {
			d, err := dot.Parse(string(src))
			if err != nil {
				t.Fatal(err)
			}
			d.Straight = straight
			lay, err := d.Layout()
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range lay.Edges {
				if e.Edge.Label == "" {
					continue
				}
				w := 0
				for _, l := range strings.Split(e.Edge.Label, "\n") {
					w = max(w, len([]rune(l)))
				}
				h := strings.Count(e.Edge.Label, "\n") + 1
				nx, ny := e.PictureLabelNudge()
				for k, at := range []diagram.Point{e.LabelAt, e.PictureLabelAt()} {
					// In cells, the text's at whole ones, the picture's
					// moved by its nudge.
					x, y := float64(at.X), float64(at.Y)
					if k == 1 {
						x, y = x+nx, y+ny
					}
					for _, n := range lay.Nodes {
						r := n.Rect
						if x < float64(r.X+r.W) && float64(r.X) < x+float64(w) && y < float64(r.Y+r.H) && float64(r.Y) < y+float64(h) {
							t.Errorf("%s, straight %v: %s to %s's label at (%.1f, %.1f) is over %s at %v", filepath.Base(path), straight, e.Edge.From, e.Edge.To, x, y, n.Node.ID, r)
						}
					}
				}
			}
		}
	}
}

// In every example's text drawing, each edge's label stands apart: no
// box's outline in the cell before or after it, where "get╭" read as part
// of the box; no other label with two spaces or fewer between them, or on
// the row above over the same columns, where "enqueue invalidate" read as
// one phrase; no line in the cell before or after it, where "enqueue▼" ran
// into its arrowhead; and within two cells of its own line, which
// "invalidate", moved off the phrase, was once six rows from.
// cellsOf is the cells e's line passes through.
func cellsOf(e diagram.PlacedEdge) map[diagram.Point]bool {
	cells := map[diagram.Point]bool{}
	for k := 1; k < len(e.Points); k++ {
		a, z := e.Points[k-1], e.Points[k]
		for x := min(a.X, z.X); x <= max(a.X, z.X); x++ {
			for y := min(a.Y, z.Y); y <= max(a.Y, z.Y); y++ {
				cells[diagram.Point{X: x, Y: y}] = true
			}
		}
	}
	return cells
}

func TestExampleLabelsStandApart(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.dot"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("examples: %v", err)
	}
	type block struct {
		edge   int
		x0, y0 int
		widths []int
	}
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		d, err := dot.Parse(string(src))
		if err != nil {
			t.Fatal(err)
		}
		lay, err := d.Layout()
		if err != nil {
			t.Fatal(err)
		}
		text := strings.Join(lay.Text(), "\n")
		name := filepath.Base(path)
		box := func(x, y int) bool {
			for _, n := range lay.Nodes {
				r := n.Rect
				if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
					return true
				}
			}
			return false
		}
		// line says an edge's line passes through (x, y).
		line := func(x, y int) bool {
			for _, e := range lay.Edges {
				for k := 1; k < len(e.Points); k++ {
					a, z := e.Points[k-1], e.Points[k]
					if x >= min(a.X, z.X) && x <= max(a.X, z.X) && y >= min(a.Y, z.Y) && y <= max(a.Y, z.Y) {
						return true
					}
				}
			}
			return false
		}
		// alone is the cells of each edge's line no other edge's passes
		// through.
		count := map[diagram.Point]int{}
		for _, e := range lay.Edges {
			for p := range cellsOf(e) {
				count[p]++
			}
		}
		alone := make([]map[diagram.Point]bool, len(lay.Edges))
		for i, e := range lay.Edges {
			alone[i] = map[diagram.Point]bool{}
			for p := range cellsOf(e) {
				if count[p] == 1 {
					alone[i][p] = true
				}
			}
		}
		var blocks []block
		for i, e := range lay.Edges {
			if e.Edge.Label == "" {
				continue
			}
			b := block{edge: i, x0: e.LabelAt.X, y0: e.LabelAt.Y}
			for _, l := range strings.Split(e.Edge.Label, "\n") {
				b.widths = append(b.widths, diagram.Width(l))
			}
			blocks = append(blocks, b)
		}
		for _, b := range blocks {
			label := lay.Edges[b.edge].Edge.Label
			// Within two cells of its own line, measured from its nearest
			// letter: further, and it labels nothing. Its line alone, where
			// any is: a label by a trunk it shares is as near the others.
			mine := alone[b.edge]
			if len(mine) == 0 {
				mine = map[diagram.Point]bool{}
				for p := range cellsOf(lay.Edges[b.edge]) {
					mine[p] = true
				}
			}
			near := false
			for p := range mine {
				for r, w := range b.widths {
					if abs(p.Y-(b.y0+r)) <= 2 && p.X >= b.x0-2 && p.X <= b.x0+w-1+2 {
						near = true
					}
				}
			}
			if !near {
				t.Errorf("%s: %q is more than two cells from its line:\n%s", name, label, text)
			}
			for k, w := range b.widths {
				y := b.y0 + k
				if box(b.x0-1, y) || box(b.x0+w, y) {
					t.Errorf("%s: %q touches a box:\n%s", name, label, text)
				}
				if line(b.x0-1, y) || line(b.x0+w, y) {
					t.Errorf("%s: %q runs into a line:\n%s", name, label, text)
				}
			}
			for _, o := range blocks {
				if o.edge <= b.edge {
					continue
				}
				for k, w := range b.widths {
					for m, ow := range o.widths {
						y, oy := b.y0+k, o.y0+m
						beside := y == oy && (o.x0-(b.x0+w-1) <= 3 && o.x0 > b.x0+w-1 || b.x0-(o.x0+ow-1) <= 3 && b.x0 > o.x0+ow-1)
						stacked := abs(y-oy) == 1 && b.x0 <= o.x0+ow && o.x0 <= b.x0+w
						if beside || stacked {
							t.Errorf("%s: %q and %q read as one:\n%s", name, label, lay.Edges[o.edge].Edge.Label, text)
						}
					}
				}
			}
		}
	}
}

// Help asked for is on standard output; --width given for the text
// drawing, which it cannot narrow, says so rather than having no effect
// and no word.
func TestDiagramHelpAndTextWidth(t *testing.T) {
	out, errOut, err := draw(t, "", "--help")
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(out, "usage: diagram") || errOut != "" {
		t.Errorf("--help: %v, stdout %d bytes, stderr %q", err, len(out), errOut)
	}
	_, errOut, err = draw(t, regions, "--terminal", "text", "--width", "20")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "wider than --width 20") {
		t.Errorf("stderr %q says nothing of --width", errOut)
	}
}

// The limits on a file's edges and a layout's page are guards, raised by a
// flag for a genuine diagram past them, and the refusal names the flag.
func TestDiagramLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString("digraph{")
	for i := range 30 {
		fmt.Fprintf(&b, " n%d -> m%d;", i, i)
	}
	b.WriteString("}")
	_, _, err := draw(t, b.String(), "--terminal", "text", "--max-edges", "20")
	if err == nil || !strings.Contains(err.Error(), "--max-edges raises the limit") {
		t.Errorf("30 edges with --max-edges 20: %v", err)
	}
	if _, _, err := draw(t, b.String(), "--terminal", "text", "--max-edges", "30"); err != nil {
		t.Errorf("30 edges with --max-edges 30: %v", err)
	}
	_, _, err = draw(t, `digraph{ a [label="`+strings.Repeat("x", 300)+`"] }`, "--terminal", "text", "--max-cells", "1000")
	if err == nil || !strings.Contains(err.Error(), "--max-cells raises the limit") || strings.Contains(err.Error(), "--high-memory") {
		t.Errorf("a 300-letter label with --max-cells 1000: %v", err)
	}
	_, _, err = draw(t, `digraph{ a [label="`+strings.Repeat("x", 300)+`"] }`, "--terminal", "text", "--max-cells", "1000", "--high-memory")
	if err == nil || !strings.Contains(err.Error(), "--max-cells raises the limit") {
		t.Errorf("--max-cells 1000 outranks --high-memory: %v", err)
	}
}

// Past the default limits a refusal says what it would take and that
// --high-memory lifts the limit; with it, a limit only Go's allocation
// sets is refused with no flag named, since none would help.
func TestDiagramHighMemory(t *testing.T) {
	big := `digraph{ a [label="` + strings.Repeat(strings.Repeat("x", 1200)+`\n`, 1200) + `"] }`
	_, _, err := draw(t, big, "--terminal", "text")
	if err == nil || !strings.Contains(err.Error(), "--high-memory, or --max-cells, raises the limit") || !strings.Contains(err.Error(), "would take about") {
		t.Errorf("a page of 1.4 million cells: %v", err)
	}
	// With --high-memory a page is held only to MostCells, past which
	// no flag raises anything: one of 3e8 cells is refused for that.
	huge := `digraph{ wide [label="` + strings.Repeat("x", 30000) + `"]; tall [label="` + strings.Repeat(`x\n`, 10000) + `"] }`
	_, _, err = draw(t, huge, "--terminal", "text", "--high-memory")
	if err == nil || !strings.Contains(err.Error(), strconv.Itoa(diagram.MostCells)) || strings.Contains(err.Error(), "raises the limit") {
		t.Errorf("a page of 3e8 cells with --high-memory: %v", err)
	}
	png := filepath.Join(t.TempDir(), "d.png")
	_, _, err = draw(t, "digraph{ a -> b }", "--out", png, "--text-size", "5000")
	if err == nil || !strings.Contains(err.Error(), "--high-memory draws it") || !strings.Contains(err.Error(), "would take about") {
		t.Errorf("a PNG past the default pixels: %v", err)
	}
	_, _, err = draw(t, "digraph{ a -> b }", "--out", png, "--text-size", "1e6", "--high-memory")
	// Refused by the ceiling Go's allocation sets, not by the default.
	if err == nil || strings.Contains(err.Error(), "--high-memory draws it") || strings.Contains(err.Error(), "the 67 million") {
		t.Errorf("a PNG past what Go can allocate, with --high-memory: %v", err)
	}
}

// A straight edge drawn in braille whose box has no free cell beside it for
// a stub ends in an arrowhead on its own line, never on another's: in the
// services example worker to db's head took the last cell of the dashed db
// to cache line, which then stopped a cell short of db.
func TestBrailleHeadKeepsOffOtherLines(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "examples", "services.dot"))
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := draw(t, string(src), "--terminal", "text", "--braille", "thin")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "cache │◀") && !strings.Contains(l, "◀╌╌╌╌╌╌╌") {
			t.Errorf("the dashed db to cache line is cut short: %q", l)
		}
		if strings.Contains(l, "╌↗") || strings.Contains(l, "╌↘") {
			t.Errorf("an arrowhead on the dashed line: %q", l)
		}
	}
}

// --width and --height size a picture; given for the text drawing -- into
// a .txt file as into a terminal -- they say they cannot shorten it,
// named as every message of the command is. --width 0 is the default, as
// in plot.
func TestDiagramTextSizeNotes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.txt")
	_, errOut, err := draw(t, regions, "--out", path, "--width", "10", "--height", "2")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"diagram: the text drawing is", "--width 10", "--height 2"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q does not say %q", errOut, want)
		}
	}
	if _, _, err := draw(t, regions, "--terminal", "text", "--width", "0"); err != nil {
		t.Errorf("--width 0: %v", err)
	}
}

// A port that cannot be kept is said so, naming the edge: one that had no
// effect and no word looked as though it had not been read. One kept is
// not mentioned. The names are quoted, an escape in one with them: the
// file's text written as it is would retitle the terminal's window.
func TestDiagramSaysWhichPortsWereNotKept(t *testing.T) {
	const blocked = "digraph {\n" +
		"columns = 3\n" +
		"x; y; z; m; d; q; \"a\x1b]0;t\x07\"\n" +
		"x:s -> d:n [style = dashed]\n" +
		"y:s -> d:n [style = dashed]\n" +
		"z:s -> d:n [style = dashed]\n" +
		"\"a\x1b]0;t\x07\" -> d:n\n" +
		"}"
	_, errOut, err := draw(t, blocked)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(errOut, "drawn by other sides than its ports ask for") != 1 || !strings.Contains(errOut, `the edge "a\x1b]0;t\a" to "d" `) {
		t.Errorf("stderr %q, want one note, for a to d", errOut)
	}
	if strings.ContainsAny(errOut, "\x1b\x07") {
		t.Errorf("stderr %q holds the name's escape as it is", errOut)
	}
}

package main

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
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
		{regions, []string{"--width", "0"}, "--width 0"},
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
		for _, ext := range []string{".txt", ".png", ".svg"} {
			out := filepath.Join(dir, strings.TrimSuffix(filepath.Base(path), ".dot")+ext)
			if _, _, err := draw(t, "", path, "--out", out); err != nil {
				t.Errorf("%s as %s: %v", path, ext, err)
			}
		}
	}
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

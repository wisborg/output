package dot

import (
	"bytes"
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/wisborg/output/diagram"
	"github.com/wisborg/output/diagram/svg"
)

// FuzzParse reads DOT as a stranger may write it and draws whatever reads:
// laid out with its edges as the file has them and straight, then as text,
// as braille, in colour and as SVG. A DOT file is often someone else's, so
// none of it may panic, and what is drawn must keep what the package
// promises of a label: the text drawing holds no control character and is
// UTF-8, the coloured one no control character but its own escapes, and
// the SVG is well-formed XML.
//
// The limits are lower than a command's, so that an input is milliseconds
// and the fuzzer reaches far: the defaults are a caller's to raise, and an
// input past them is refused as any is. The PNG is left out, as the SVG
// draws the same layout and a picture is a thousand times slower.
//
// go test runs the seeds and testdata/fuzz alone; make fuzz searches.
func FuzzParse(f *testing.F) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.dot"))
	if err != nil {
		f.Fatal(err)
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(src))
	}
	for _, s := range []string{
		"digraph { a -> b -> c; a -> c [dir=back, label=\"x\"] }",
		"strict graph { a:n -- b:s; b -- a [tailport=e] }",
		"digraph { rankdir=LR; subgraph cluster_x { label=\"c\"; columns=2; a; b; c } d -> a:w }",
		"digraph { splines=line; a -> b; b -> c; a -> c; c -> a [label=\"back\"] }",
		"digraph { a [shape=cylinder, fillcolor=\"#ffd76e\", style=filled]; a -> a }",
		"digraph { \"日本語\" -> \"👨‍👩‍👧\" [label=\"\x1b]0;t\x07\"]; c [label=\"a\tb\u202ec\"] }",
		"digraph { a [label=\"not UTF-8: \xff\"] }",
		"digraph { a [label=\"<b>&amp; \\\"q\\\" \x01\"] -> b [label=\"]]> <!--\"] }",
		"digraph { legend [label=\"k\"]; {a b} -> {c d} [style=dashed] }",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 4096 {
			return
		}
		d, err := ParseWith(src, Options{MaxEdges: 40})
		if err != nil {
			return
		}
		for _, straight := range []bool{d.Straight, true} {
			d.Straight = straight
			lay, err := d.LayoutWith(diagram.LayoutOptions{MaxCells: 40000})
			if err != nil {
				return
			}
			for _, o := range []diagram.TextOptions{{}, {Braille: diagram.BrailleThin}, {Braille: diagram.BrailleThick}} {
				for _, l := range lay.Text(o) {
					if !utf8.ValidString(l) || strings.ContainsFunc(l, control) {
						t.Fatalf("a text line not UTF-8 or holding a control character: %q", l)
					}
				}
			}
			for _, l := range lay.ColourText(diagram.Dark) {
				if !utf8.ValidString(l) || strings.ContainsFunc(l, func(r rune) bool { return r != '\x1b' && control(r) }) {
					t.Fatalf("a coloured line not UTF-8 or holding a control character: %q", l)
				}
			}
			var buf bytes.Buffer
			if err := svg.Write(&buf, lay, svg.Options{}); err != nil {
				continue // a size no viewer draws is refused, not written
			}
			dec := xml.NewDecoder(&buf)
			for {
				if _, err := dec.Token(); err == io.EOF {
					break
				} else if err != nil {
					t.Fatalf("an SVG that is not well-formed: %v", err)
				}
			}
		}
	})
}

// control is a C0 or C1 control character, or DEL.
func control(r rune) bool { return r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0 }

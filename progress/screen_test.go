package progress

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
)

// screen replays the escape sequences a Display writes and reports what a
// terminal would actually be showing.
//
// Asserting on the raw byte stream is the obvious thing and it tests the
// wrong property. What matters is not that a particular cursor-up was
// emitted, it is that the live region ends up REPLACED rather than repeated,
// and that the log lines above it survive -- which is a statement about the
// screen, not about the bytes. A golden byte string would also have to be
// rewritten for every cosmetic change, and would still pass if the sequences
// were internally inconsistent.
//
// It understands only what this package emits: CSI nA (cursor up), CSI J
// (clear to end of screen), CSI K (clear to end of line), carriage return and
// newline. Anything else is a bug in the display, so it is reported rather
// than skipped.
type screen struct {
	lines []string
	row   int
	col   int
	junk  []string // escape sequences this replay did not expect
}

var csi = regexp.MustCompile(`^\x1b\[([0-9]*)([A-Za-z])`)

func replay(s string) *screen {
	sc := &screen{}
	for i := 0; i < len(s); {
		if s[i] == '\x1b' {
			m := csi.FindStringSubmatch(s[i:])
			if m == nil {
				sc.junk = append(sc.junk, strconv.Quote(s[i:min(i+8, len(s))]))
				i++
				continue
			}
			n, _ := strconv.Atoi(m[1])
			if m[1] == "" {
				n = 1
			}
			switch m[2] {
			case "A":
				sc.row -= n
				if sc.row < 0 {
					sc.row = 0
				}
				sc.col = 0
			case "J":
				if sc.row < len(sc.lines) {
					sc.lines = sc.lines[:sc.row]
				}
			case "K":
				sc.truncateLine()
			default:
				sc.junk = append(sc.junk, strconv.Quote(m[0]))
			}
			i += len(m[0])
			continue
		}
		switch s[i] {
		case '\n':
			sc.row++
			sc.col = 0
		case '\r':
			sc.col = 0
		default:
			r, size := decodeRune(s[i:])
			sc.put(r)
			i += size
			continue
		}
		i++
	}
	return sc
}

func decodeRune(s string) (rune, int) {
	for i, r := range s {
		_ = i
		return r, len(string(r))
	}
	return 0, 1
}

func (sc *screen) grow() {
	for len(sc.lines) <= sc.row {
		sc.lines = append(sc.lines, "")
	}
}

func (sc *screen) put(r rune) {
	sc.grow()
	line := []rune(sc.lines[sc.row])
	for len(line) < sc.col {
		line = append(line, ' ')
	}
	if sc.col < len(line) {
		line[sc.col] = r
	} else {
		line = append(line, r)
	}
	sc.lines[sc.row] = string(line)
	sc.col++
}

func (sc *screen) truncateLine() {
	sc.grow()
	line := []rune(sc.lines[sc.row])
	if sc.col < len(line) {
		sc.lines[sc.row] = string(line[:sc.col])
	}
}

// text is what the terminal shows, with trailing blank lines removed.
func (sc *screen) text() []string {
	out := append([]string(nil), sc.lines...)
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	for i := range out {
		out[i] = strings.TrimRight(out[i], " ")
	}
	return out
}

// widest is the display width of the longest line on screen, which is what
// the "never exceed the terminal" invariant is asserted against.
func (sc *screen) widest() (int, string) {
	w, worst := 0, ""
	for _, l := range sc.text() {
		if n := runewidth.StringWidth(l); n > w {
			w, worst = n, l
		}
	}
	return w, worst
}

// columnOf is the display COLUMN a rune sits at, which is not its byte
// offset: the block glyphs are three bytes each and a space is one, so
// comparing byte indexes across two bars says they are misaligned when they
// are not. Returns -1 when the rune is absent.
func columnOf(line string, want rune) int {
	col := 0
	for _, r := range line {
		if r == want {
			return col
		}
		col += runewidth.RuneWidth(r)
	}
	return -1
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

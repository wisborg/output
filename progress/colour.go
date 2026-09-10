package progress

import (
	"os"
	"strconv"
)

// RGB is a 24-bit colour.
type RGB struct{ R, G, B uint8 }

// ColourMode selects how the filled part of a bar is coloured.
type ColourMode int

const (
	// Monochrome draws the fill in whatever colour the terminal is already
	// using. It is the zero value, so a caller that says nothing about
	// colour gets none -- a program's output should not acquire escape
	// sequences because a library thought they would look nice.
	Monochrome ColourMode = iota
	// Solid draws the whole fill in one colour.
	Solid
	// Gradient blends across the trough, from one colour at the left to
	// another at the right.
	Gradient
)

// Palette is how a Display colours the filled part of its bars.
//
// The zero value is monochrome, and monochrome is the same code path as the
// others with the colouring step doing nothing -- there is no separate
// uncoloured renderer to drift away from the coloured one.
//
// Only the FILL is coloured. The trough's brackets, the label and the
// numbers are left alone: they are read, not glanced at, and a terminal's
// own foreground colour is what its user chose for reading.
type Palette struct {
	Mode ColourMode
	// From is Solid's colour and Gradient's left-hand end.
	From RGB
	// To is Gradient's right-hand end. Ignored by the other modes.
	To RGB
}

// SolidPalette returns a palette drawing the fill in one colour.
func SolidPalette(c RGB) Palette { return Palette{Mode: Solid, From: c} }

// GradientPalette returns a palette blending the fill from one colour to
// another across the trough.
func GradientPalette(from, to RGB) Palette {
	return Palette{Mode: Gradient, From: from, To: to}
}

// DefaultGradient is a blue-to-green ramp.
//
// Both ends are mid-tone on purpose. A gradient running into a very light or
// very dark colour disappears at one end against a terminal whose background
// happens to be the same, and this package cannot know which that is: a
// terminal reports its size, never its colours. These two are legible on
// white and on black.
func DefaultGradient() Palette {
	return GradientPalette(RGB{0x3B, 0x82, 0xF6}, RGB{0x22, 0xC5, 0x5E})
}

// at returns the colour of cell i of width, and whether there is one to
// apply at all.
//
// The gradient spans the whole TROUGH rather than the filled part of it, so
// a given cell keeps its colour as the bar grows past it. Blending across
// only the fill would recolour every cell on every redraw, turning a bar
// filling up into a bar that also shimmers.
func (p Palette) at(i, width int) (RGB, bool) {
	switch p.Mode {
	case Solid:
		return p.From, true
	case Gradient:
		if width <= 1 {
			return p.From, true
		}
		t := float64(i) / float64(width-1)
		return RGB{
			R: lerp8(p.From.R, p.To.R, t),
			G: lerp8(p.From.G, p.To.G, t),
			B: lerp8(p.From.B, p.To.B, t),
		}, true
	default:
		return RGB{}, false
	}
}

func lerp8(a, b uint8, t float64) uint8 {
	return uint8(float64(a) + (float64(b)-float64(a))*t + 0.5)
}

// depth is how much colour the terminal has been found to accept.
type depth int

const (
	depthNone depth = iota
	depth256
	depthTrueColour
)

// detectDepth decides how much colour to use, from the environment.
//
// NO_COLOR is honoured first and unconditionally: it is a request from the
// person running the program, and the convention (no-color.org) is that any
// non-empty value means no colour at all. TERM=dumb says the same thing in
// older words.
//
// Truecolor is used only when COLORTERM claims it, because the failure is
// silent and ugly: a terminal that does not understand a 24-bit SGR shows
// its digits as text across the bar. The 256-colour form is understood
// essentially everywhere that understands colour at all, so it is the
// default rather than the fallback.
func detectDepth(getenv func(string) string) depth {
	if getenv("NO_COLOR") != "" {
		return depthNone
	}
	switch getenv("TERM") {
	case "dumb":
		return depthNone
	}
	switch getenv("COLORTERM") {
	case "truecolor", "24bit":
		return depthTrueColour
	}
	return depth256
}

// sgr appends the escape sequence selecting c as the foreground colour.
func (d depth) sgr(dst []byte, c RGB) []byte {
	switch d {
	case depthTrueColour:
		dst = append(dst, "\x1b[38;2;"...)
		dst = strconv.AppendUint(dst, uint64(c.R), 10)
		dst = append(dst, ';')
		dst = strconv.AppendUint(dst, uint64(c.G), 10)
		dst = append(dst, ';')
		dst = strconv.AppendUint(dst, uint64(c.B), 10)
		return append(dst, 'm')
	case depth256:
		dst = append(dst, "\x1b[38;5;"...)
		dst = strconv.AppendUint(dst, uint64(to256(c)), 10)
		return append(dst, 'm')
	default:
		return dst
	}
}

// colourReset returns the foreground to the terminal's default.
//
// 39 rather than 0: a full reset would also clear bold, italics and the
// background, which belong to whatever the caller's own output was doing
// around this line and are none of this package's business.
const colourReset = "\x1b[39m"

// cube holds the six values the 256-colour palette's RGB cube is built from.
var cube = [6]uint8{0, 95, 135, 175, 215, 255}

// to256 maps a 24-bit colour onto the xterm 256-colour palette, choosing
// between its 6x6x6 cube and its 24-step grey ramp by whichever lands closer.
func to256(c RGB) int {
	ri, gi, bi := nearestCube(c.R), nearestCube(c.G), nearestCube(c.B)
	cubeErr := sqDiff(c.R, cube[ri]) + sqDiff(c.G, cube[gi]) + sqDiff(c.B, cube[bi])

	// The grey ramp is 232..255, from 8 to 238 in steps of 10.
	grey := (int(c.R) + int(c.G) + int(c.B)) / 3
	gi2 := (grey - 8 + 5) / 10
	if gi2 < 0 {
		gi2 = 0
	}
	if gi2 > 23 {
		gi2 = 23
	}
	greyLevel := uint8(8 + gi2*10)
	greyErr := sqDiff(c.R, greyLevel) + sqDiff(c.G, greyLevel) + sqDiff(c.B, greyLevel)

	if greyErr < cubeErr {
		return 232 + gi2
	}
	return 16 + 36*ri + 6*gi + bi
}

func nearestCube(v uint8) int {
	best, bestErr := 0, sqDiff(v, cube[0])
	for i := 1; i < len(cube); i++ {
		if e := sqDiff(v, cube[i]); e < bestErr {
			best, bestErr = i, e
		}
	}
	return best
}

func sqDiff(a, b uint8) int {
	d := int(a) - int(b)
	return d * d
}

// osGetenv is os.Getenv, named so a test can substitute one.
func osGetenv(k string) string { return os.Getenv(k) }

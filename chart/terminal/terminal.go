// Package terminal shows a picture in a terminal that can show one --
// iTerm2's inline images, which WezTerm reads too, and kitty's graphics
// protocol, which Ghostty reads too -- and says which terminal that is,
// how big it is in characters and pixels, and whether its background is
// light.
//
// A chart is drawn in braille where none of that holds: Choose says when.
package terminal

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Protocol is how a picture is shown in a terminal, or that it is drawn in
// braille instead.
type Protocol string

const (
	// Auto is Choose's to decide.
	Auto Protocol = "auto"
	// ITerm is iTerm2's inline image, which WezTerm reads too.
	ITerm Protocol = "iterm"
	// Kitty is kitty's graphics protocol, which Ghostty reads too.
	Kitty Protocol = "kitty"
	// Braille is no picture: the chart drawn in braille dots, which any
	// terminal shows.
	Braille Protocol = "braille"
)

// Protocols are the protocols Choose takes, in the order a program offers
// them.
var Protocols = []Protocol{Auto, ITerm, Kitty, Braille}

// Choose is how a picture is shown for mode, given the environment and
// whether the output is a terminal at all, and a note for the reader where
// the choice would surprise them: in tmux, a terminal that shows pictures
// is drawn braille, and its user may not know why. Elsewhere braille is
// what the terminal can show, and saying so every time would be noise.
//
// Auto shows a picture only where it will be seen as one: on a terminal
// rather than into a pipe or a file, where the picture would arrive as a
// page of base64; in a terminal known to show pictures, by the variables
// it sets; and not inside tmux or screen, which pass a picture's escape
// sequence through only when configured to, and otherwise drop it or show
// it as text. Everywhere else it is braille.
//
// The terminals are known by what they set and an ssh session forwards:
// iTerm2's LC_TERMINAL travels with the locale, where TERM_PROGRAM does
// not. A mode other than Auto is obeyed whatever the output is: it is how
// a user says outright what detection missed.
func Choose(mode Protocol, getenv func(string) string, tty bool) (how Protocol, note string, err error) {
	switch mode {
	case ITerm, Kitty, Braille:
		return mode, "", nil
	case Auto:
	default:
		return "", "", fmt.Errorf("%q: use auto, iterm, kitty or braille", string(mode))
	}
	if !tty {
		return Braille, "", nil
	}
	term, program := getenv("TERM"), getenv("TERM_PROGRAM")
	if getenv("TMUX") != "" || getenv("STY") != "" || strings.HasPrefix(term, "screen") || strings.HasPrefix(term, "tmux") {
		return Braille, "drawn in braille: tmux and screen pass a picture through only when set to", nil
	}
	switch {
	case getenv("KITTY_WINDOW_ID") != "" || term == "xterm-kitty", program == "ghostty" || term == "xterm-ghostty":
		return Kitty, "", nil
	case program == "iTerm.app" || getenv("LC_TERMINAL") == "iTerm2", program == "WezTerm":
		return ITerm, "", nil
	}
	return Braille, "", nil
}

// Write writes png to w in protocol how, ITerm or Kitty, cols characters
// wide, its height as its shape makes it.
func Write(w io.Writer, how Protocol, png []byte, cols int) error {
	switch how {
	case ITerm:
		return WriteITerm(w, png, cols)
	case Kitty:
		return WriteKitty(w, png, cols)
	}
	return fmt.Errorf("terminal: %q shows no picture", string(how))
}

// WriteITerm writes png to w as iTerm2's inline image, cols characters
// wide: the whole file, base64, in one escape sequence.
func WriteITerm(w io.Writer, png []byte, cols int) error {
	_, err := fmt.Fprintf(w, "\x1b]1337;File=inline=1;size=%d;width=%d;preserveAspectRatio=1:%s\a\n",
		len(png), cols, base64.StdEncoding.EncodeToString(png))
	return err
}

// KittyChunk is the most base64 kitty's graphics protocol takes in one
// escape sequence.
const KittyChunk = 4096

// WriteKitty writes png to w in kitty's graphics protocol, cols characters
// wide: the base64 in chunks of at most KittyChunk, each but the last
// saying more follows. q=2 asks the terminal not to answer: an answer
// arrives on the terminal's input, where the shell would read it as
// typing.
func WriteKitty(w io.Writer, png []byte, cols int) error {
	data := base64.StdEncoding.EncodeToString(png)
	var b bytes.Buffer
	for first := true; first || len(data) > 0; first = false {
		n := min(len(data), KittyChunk)
		chunk, more := data[:n], 0
		data = data[n:]
		if len(data) > 0 {
			more = 1
		}
		if first {
			fmt.Fprintf(&b, "\x1b_Ga=T,f=100,q=2,c=%d,m=%d;%s\x1b\\", cols, more, chunk)
		} else {
			fmt.Fprintf(&b, "\x1b_Gm=%d;%s\x1b\\", more, chunk)
		}
	}
	b.WriteString("\n")
	_, err := w.Write(b.Bytes())
	return err
}

// LightBackground says the terminal's background is light, by COLORFGBG --
// "foreground;background" as colour numbers -- which iTerm2, Konsole, rxvt
// and others set and an ssh session can carry: 7 and 15 are white, and
// every number from 9 up but those is a bright colour, read as light too;
// 0 to 6 and 8 are dark. A terminal that does not say is taken to be
// dark, as iTerm2, kitty, WezTerm and Ghostty -- the terminals that show
// pictures -- are by default.
func LightBackground(getenv func(string) string) bool {
	fields := strings.Split(getenv("COLORFGBG"), ";")
	bg, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil {
		return false
	}
	return bg == 7 || bg >= 9 && bg <= 15
}

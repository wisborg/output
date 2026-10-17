package diagram

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"
)

// cells is how a label is measured in a terminal's columns: a CJK
// character two, a combining accent nothing, a flag of two regional
// indicators two, as runewidth has them.
//
// With East Asian Ambiguous characters one column, whatever the locale.
// runewidth's default reads LANG and counts them two in a CJK one, and a
// layout whose boxes moved with the environment it was drawn in could not be
// compared with the last drawing of the same system -- which is what a grid
// is for -- nor tested. The box-drawing lines are ambiguous too, so a
// terminal that draws them two columns wide draws no drawing of this kind
// well, measured either way.
var cells = &runewidth.Condition{StrictEmojiNeutral: true}

// Width is how many cells s takes in a layout: its width in a terminal's
// columns, with each control character a cell, as the text drawing writes
// it as U+FFFD. The pictures are drawn on the same cells, and measure a
// label by it to patch the background beneath one.
//
// Not the count of its runes, which is what the layout once measured: a
// CJK label then overflowed its box, and pushed every box after it on its
// line out of true.
func Width(s string) int { return cells.StringWidth(printable(s)) }

// printable is s with each control character written as U+FFFD. A label
// is text someone else may have written, and an escape in it written to a
// terminal is a command to the terminal -- to clear it, retitle the window,
// or write the clipboard -- where a tab or a carriage return breaks the box
// it is in.
func printable(s string) string {
	if !strings.ContainsFunc(s, unicode.IsControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return '�'
		}
		return r
	}, s)
}

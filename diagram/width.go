package diagram

import (
	"strings"
	"unicode"
	"unicode/utf8"

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
// columns, with each control character, format character and byte that is
// not UTF-8 a cell, as the text drawing writes each as U+FFFD. The pictures are drawn on the same cells, and measure a
// label by it to patch the background beneath one.
//
// Not the count of its runes, which is what the layout once measured: a
// CJK label then overflowed its box, and pushed every box after it on its
// line out of true.
func Width(s string) int { return cells.StringWidth(printable(s)) }

// printable is s with each character hidden has, and each byte that is
// not UTF-8, written as U+FFFD. A label is text someone else may have
// written, and an escape in it written to a terminal is a command to the
// terminal -- to clear it, retitle the window, or write the clipboard --
// where a tab or a carriage return breaks the box it is in.
func printable(s string) string {
	if utf8.ValidString(s) && !strings.ContainsFunc(s, hidden) {
		return s
	}
	return strings.Map(func(r rune) rune { // a byte that is not UTF-8 comes as U+FFFD
		if hidden(r) {
			return '\uFFFD'
		}
		return r
	}, s)
}

// hidden is whether r is written as U+FFFD: a control character, a line or
// paragraph separator, or a format character -- a bidi override, a
// zero-width space, a byte-order mark. A format character draws nothing,
// and a terminal's reader cannot see it: U+202E turns the rest of a line
// round, so "exe.txt" in a name reads as something else, and a zero-width
// space makes two names that look the same differ. The joiners U+200C and
// U+200D are kept: they are part of the text they are in -- an emoji
// family, a Persian word -- and a width measures it with them.
func hidden(r rune) bool {
	switch {
	case unicode.IsControl(r), r == '\u2028', r == '\u2029':
		return true
	case r == '\u200C', r == '\u200D':
		return false
	}
	return unicode.Is(unicode.Cf, r)
}

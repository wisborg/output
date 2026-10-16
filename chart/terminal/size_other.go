//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd

package terminal

import "os"

// SizeOf is not asked of the terminal here: no terminal, so a caller falls
// back to COLUMNS and LINES and its own defaults, and draws in braille.
func SizeOf(*os.File) (Size, bool) { return Size{}, false }

//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !windows

package progress

// terminalSize reports no terminal on a platform this package has no way to
// ask.
//
// Reporting "not a terminal" rather than guessing a width is what makes the
// fallback safe: Display then renders plain periodic lines with no escape
// sequences at all, which is correct output everywhere, merely less pretty
// than it could be. A build that guessed would emit cursor movements at
// something that may not understand them.
func terminalSize(uintptr) (cols, rows int, ok bool) { return 0, 0, false }

func enableVirtualTerminal(uintptr) bool { return false }

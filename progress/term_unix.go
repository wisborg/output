//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package progress

import (
	"syscall"
	"unsafe"
)

// winsize mirrors the kernel's struct winsize, which TIOCGWINSZ fills in.
// Only the first two fields are ever read; the pixel dimensions are part of
// the layout and are meaningless on every terminal emulator in practice.
type winsize struct {
	rows, cols     uint16
	xpixel, ypixel uint16
}

// terminalSize reports the size of the terminal on fd.
//
// ONE ioctl answers both of this package's questions, which is why there is
// no separate isTerminal alongside it. "Is this a terminal" and "how wide is
// it" have the same answer here: a pipe, a file and /dev/null all fail
// TIOCGWINSZ with ENOTTY, and anything that can report a window size is
// something a live display belongs on.
//
// That is deliberately not the test the standard library makes easy.
// os.File.Stat with ModeCharDevice is the usual shortcut and it is wrong in a
// way that shows up in ordinary use: /dev/null is a character device, so
// `program 2>/dev/null` is reported as a terminal and the program then writes
// escape sequences into the void -- harmless there, but the same reasoning
// admits any other character device.
//
// A terminal that reports zero columns is still a terminal; resolving that to
// a usable width is the caller's business (see Options.Columns), not this
// function's. It reports what the kernel said.
func terminalSize(fd uintptr) (cols, rows int, ok bool) {
	var ws winsize
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		fd,
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(&ws)),
	)
	if errno != 0 {
		return 0, 0, false
	}
	return int(ws.cols), int(ws.rows), true
}

// enableVirtualTerminal is a no-op outside Windows: a unix terminal that
// answers TIOCGWINSZ interprets ANSI sequences already. It exists so the
// platform-independent code has one shape to call.
func enableVirtualTerminal(uintptr) bool { return true }

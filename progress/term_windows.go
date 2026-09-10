//go:build windows

package progress

import (
	"syscall"
	"unsafe"
)

// This file is written against the documented Win32 console API but is NOT
// exercised by this project's own testing, which runs on unix. It is
// deliberately shaped so that every uncertainty fails CLOSED: a call that
// does not succeed reports "not a terminal", and Display then renders plain
// periodic lines with no escape sequences. The worst outcome of a mistake
// here is therefore output that is less pretty, never a corrupted console.

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

type coord struct{ x, y int16 }

type smallRect struct{ left, top, right, bottom int16 }

type consoleScreenBufferInfo struct {
	size              coord
	cursorPosition    coord
	attributes        uint16
	window            smallRect
	maximumWindowSize coord
}

// enableVirtualTerminalProcessing is the console mode bit that makes a
// Windows console interpret ANSI escape sequences. It is on by default in
// Windows Terminal and in conhost on current Windows 10 and 11, but this
// package sets it explicitly rather than assuming: the whole live display is
// escape sequences, and a console without this bit prints them literally.
const enableVirtualTerminalProcessing = 0x0004

// terminalSize reports the size of the console WINDOW on fd -- not the size
// of its screen buffer.
//
// The distinction is the one real trap in this API. A Windows console's
// buffer is routinely far taller than the window showing it, and often wider,
// so a display sized from buffer.size would draw lines wider than the visible
// window and wrap every one of them. The visible window is window.right -
// window.left + 1.
func terminalSize(fd uintptr) (cols, rows int, ok bool) {
	var info consoleScreenBufferInfo
	r, _, _ := procGetConsoleScreenBufferInfo.Call(fd, uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0, 0, false
	}
	cols = int(info.window.right) - int(info.window.left) + 1
	rows = int(info.window.bottom) - int(info.window.top) + 1
	if cols <= 0 || rows <= 0 {
		return 0, 0, false
	}
	return cols, rows, true
}

// enableVirtualTerminal turns on ANSI interpretation for fd, reporting
// whether the console will now understand the sequences Display writes.
//
// Failure is not an error to report: it means this console cannot render a
// live display, which is a fact about the environment rather than a fault.
// The caller degrades to plain lines.
func enableVirtualTerminal(fd uintptr) bool {
	h := syscall.Handle(fd)
	var mode uint32
	if err := syscall.GetConsoleMode(h, &mode); err != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	// SetConsoleMode is not in the standard syscall package's Windows
	// surface, so it is reached the same way GetConsoleScreenBufferInfo is.
	proc := kernel32.NewProc("SetConsoleMode")
	r, _, _ := proc.Call(fd, uintptr(mode|enableVirtualTerminalProcessing))
	return r != 0
}

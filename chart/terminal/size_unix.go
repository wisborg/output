//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package terminal

import (
	"os"
	"syscall"
	"unsafe"
)

// SizeOf is the size of the terminal f writes to, and whether f is one:
// asked of the terminal itself, so a window resized since the shell
// started is measured as it is now, which COLUMNS would not. Its size in
// pixels is zero where the terminal does not say.
func SizeOf(f *os.File) (Size, bool) {
	var ws struct{ Row, Col, X, Y uint16 }
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws))); errno != 0 {
		return Size{}, false
	}
	return Size{Cols: int(ws.Col), Rows: int(ws.Row), Width: int(ws.X), Height: int(ws.Y)}, ws.Col > 0 && ws.Row > 0
}

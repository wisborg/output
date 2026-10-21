//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package terminal

import (
	"errors"
	"os"
	"testing"
	"time"
)

// SizeOf leaves the file as it found it: asked through Fd, it put a pipe
// into blocking mode, and a deadline set on it afterwards never fired --
// a read past it waited for the writer instead.
func TestSizeOfLeavesDeadlinesWorking(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if _, ok := SizeOf(r); ok {
		t.Error("a pipe reported as a terminal")
	}
	if err := r.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := r.Read(make([]byte, 1))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Errorf("a read past the deadline: %v, want the deadline exceeded", err)
		}
		w.Close()
	case <-time.After(2 * time.Second):
		w.Close() // ends the read, with EOF
		<-done
		t.Error("a read past the deadline waited: SizeOf left the pipe blocking")
	}
}

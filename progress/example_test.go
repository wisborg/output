package progress_test

import (
	"fmt"
	"os"
	"time"

	"github.com/wisborg/output/progress"
)

// A Display writes to a terminal when it has one and falls back to plain
// periodic lines when it does not, so a program can use it without asking
// where its output is going.
func ExampleNew() {
	d := progress.New(os.Stderr, progress.Options{})
	defer d.Stop()

	bar := d.Bar(progress.BarSpec{Label: "rendering", Total: 10800, Unit: "frames"})
	for i := 0; i < 10800; i++ {
		// ... render frame i ...
		bar.Set(int64(i + 1))
	}
	// Output:
}

// steadyClock advances a fixed amount on every reading, so an example's
// output does not depend on how fast the machine running it happens to be.
// A real program leaves Options.Now unset and gets time.Now.
func steadyClock() func() time.Time {
	t := time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)
	return func() time.Time {
		t = t.Add(time.Second)
		return t
	}
}

// A Display is an io.Writer, which is the whole mechanism for putting log
// output and live bars on one terminal: writes scroll up the screen and the
// bars stay at the bottom. Any logger over an io.Writer composes this way,
// which is why this package contains no logger of its own.
func ExampleDisplay_Write() {
	// Plain mode and a fixed clock so the example's output is stable; a real
	// program passes the zero Options and lets the display detect.
	d := progress.New(os.Stdout, progress.Options{
		Mode:     progress.Plain,
		Interval: time.Millisecond,
		Now:      steadyClock(),
	})
	defer d.Stop()

	bar := d.Bar(progress.BarSpec{Label: "rendering", Total: 4, Unit: "frames"})
	bar.Set(1)
	fmt.Fprintln(d, "warn  clip-0043 has no GPS fixes")
	bar.Set(2)

	// Output:
	// rendering 25% 1/4 frames 0.5/s ~6s left
	// warn  clip-0043 has no GPS fixes
	// rendering 50% 2/4 frames 0.5/s ~4s left
}

// A job whose size is not known reports what it has -- a count and a rate --
// and no percentage, no estimate and no empty trough. A bar that never fills
// reads as one that is stuck rather than as a measurement nobody has.
func ExampleBarSpec_unknownTotal() {
	d := progress.New(os.Stdout, progress.Options{
		Mode:     progress.Plain,
		Interval: time.Millisecond,
		Now:      steadyClock(),
	})
	defer d.Stop()

	bar := d.Bar(progress.BarSpec{Label: "encoding", Unit: "frames"})
	bar.Set(500)

	// Output:
	// encoding 500 frames 250/s
}

// Package progress renders live progress on a terminal: one or more bars
// pinned to the bottom of the screen, with ordinary output scrolling above
// them.
//
// It is the third shape this module produces. The root package writes one
// finished result and table writes a grid; both answer "what did the program
// find". This answers "what is the program doing", which is a different
// question with different rules -- it is written while the work runs, it is
// overwritten in place, and it must leave nothing behind when the work ends.
//
// # Logs and bars together
//
// A Display is an io.Writer, and that is the whole mechanism for combining it
// with a logger. A write erases the live region, emits the line, and redraws
// the bars beneath it, so log output scrolls up the screen normally while the
// bars stay at the bottom:
//
//	d := progress.New(os.Stderr, progress.Options{})
//	defer d.Stop()
//	log := slog.New(slog.NewTextHandler(d, nil))   // or any logger over an io.Writer
//	bar := d.Bar(progress.BarSpec{Label: "rendering", Total: frames, Unit: "frames"})
//
// Nothing here knows what a log line is. Any logger that writes to an
// io.Writer composes with this, which is why this package deliberately does
// not contain one.
//
// # Not a terminal
//
// When the writer is not a terminal -- redirected to a file, piped, or on a
// platform this package cannot ask -- a Display emits plain periodic lines
// with no escape sequences at all and never overwrites anything. That is the
// same output a redirected run has always produced, and it is why a program
// can use this unconditionally rather than branching on where its output is
// going.
//
// Those lines are snapshots, written once an interval, so the last one can
// fall short of where the job ended; Bar.Done writes one closing line at the
// final state for a bar that has written any. A job that finishes within an
// interval writes nothing at all, as before.
//
// # The one rule
//
// A live line is NEVER allowed to exceed the terminal's width. Everything
// else here follows from that. A line that wraps occupies two rows, the
// cursor arithmetic that erases the region counts one, and from then on the
// display eats the line above it on every redraw -- shredding the log it was
// supposed to sit beneath. So lines are measured in terminal columns and
// truncated, and a bar too narrow to hold its fields drops fields rather than
// wrapping.
package progress

import (
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-runewidth"
)

// DefaultInterval is how often a terminal Display redraws.
//
// Twelve times a second: fast enough that a bar looks continuous rather than
// stepped, slow enough that a frame loop calling Set at 300 Hz spends its
// time rendering frames rather than escape sequences. It bounds redraws, not
// Set calls, which stay cheap enough to make per-frame.
const DefaultInterval = 80 * time.Millisecond

// DefaultPlainInterval is how often a non-terminal Display writes a line.
//
// Two seconds, which is a different question answered by a different number:
// these lines accumulate in a log file rather than replacing each other, so
// the cost of one is permanent. It is also slow enough that the remaining-time
// estimate has settled between lines and does not visibly jitter.
const DefaultPlainInterval = 2 * time.Second

// FallbackColumns is the width assumed when the writer is a terminal that
// reports no usable width of its own -- a pty opened without a size being
// set, most commonly. Eighty is the conventional answer and is narrow enough
// to be safe on anything wider.
const FallbackColumns = 80

// Mode selects how a Display renders, overriding what it would detect.
type Mode int

const (
	// Auto renders live when the writer is a terminal that can report its
	// width, and plain otherwise. This is the zero value and the right
	// choice for a program that does not know where its output is going.
	Auto Mode = iota
	// Live forces the terminal rendering, escape sequences and all. For
	// tests, and for a caller writing to something it knows is a terminal
	// through an intermediary this package cannot see through.
	Live
	// Plain forces periodic lines with no escape sequences. For a caller
	// that wants progress in a log regardless of where it is running.
	Plain
)

// Options configures a Display. The zero value is the intended configuration
// for an ordinary program; every field exists for a caller that needs to
// override a detected value or a test that needs to pin one.
type Options struct {
	// Interval bounds how often the display is redrawn. Zero means
	// DefaultInterval in Live mode and DefaultPlainInterval in Plain.
	Interval time.Duration

	// Columns forces the terminal width. Zero means detect it, falling back
	// to FallbackColumns.
	//
	// Detection happens on every redraw rather than once, which is how a
	// terminal resize is handled: there is no signal handler here, because a
	// library that installs one takes a process-wide resource its caller may
	// already be using. An ioctl per redraw is far cheaper than the write it
	// accompanies.
	Columns int

	// Mode overrides live-versus-plain detection.
	Mode Mode

	// ASCII draws bars from + and - instead of the eighth-block characters
	// U+2588..U+258F. The blocks give a bar eight times the resolution of
	// its width in cells, which is the difference between a bar that moves
	// smoothly and one that jumps; they need a font that has them, which is
	// nearly universal but not guaranteed.
	ASCII bool

	// Palette colours the filled part of every bar. The zero value is
	// monochrome; see Palette, SolidPalette, GradientPalette and
	// DefaultGradient.
	//
	// Asking for colour is not the same as getting it. Colour is emitted
	// only on a live display whose terminal will take it -- never in plain
	// mode, and never when NO_COLOR or TERM=dumb says otherwise.
	Palette Palette

	// Now stands in for time.Now so a test can drive throttling and rates
	// without sleeping.
	Now func() time.Time

	// Width measures a string in terminal columns. Nil means
	// runewidth.StringWidth.
	//
	// This exists for the same reason table.Table.Width does: bytes, runes
	// and display columns are three different numbers, the failure is silent,
	// and a caller may have a cheaper measure it trusts for its own data.
	Width func(string) int
}

// Display owns the live region at the bottom of a writer and the bars in it.
//
// A nil *Display is usable and does nothing: Bar returns a nil *Bar, Write
// discards, and Stop is a no-op. That is what lets a program with a --quiet
// flag decide once, at construction, instead of guarding every call site.
type Display struct {
	w     io.Writer
	file  *os.File // nil unless w is one; the only way to ask about the terminal
	live  bool
	ascii bool

	palette  Palette
	depth    depth
	interval time.Duration
	now      func() time.Time
	measure  func(string) int
	columns  int // forced; 0 means detect

	// nextDue is the earliest nanosecond at which a redraw may happen, read
	// and written atomically so Bar.Set can decline to redraw without taking
	// the mutex. Set is called once per unit of work -- per video frame, in
	// both of this package's intended consumers -- so the path that does not
	// redraw must not contend on a lock.
	nextDue atomic.Int64

	mu      sync.Mutex
	bars    []*Bar
	drawn   int // live lines currently on screen
	stopped bool
}

// New returns a Display writing to w.
//
// Whether it renders live is decided here, once, and never revisited: a
// program's output does not become a terminal part-way through a run, and a
// display that changed shape mid-render would leave half its escape sequences
// in a file.
func New(w io.Writer, opts Options) *Display {
	d := &Display{
		w:       w,
		ascii:   opts.ASCII,
		palette: opts.Palette,
		now:     opts.Now,
		measure: opts.Width,
		columns: opts.Columns,
	}
	if d.now == nil {
		d.now = time.Now
	}
	if d.measure == nil {
		d.measure = runewidth.StringWidth
	}
	if f, ok := w.(*os.File); ok {
		d.file = f
	}

	switch opts.Mode {
	case Live:
		d.live = true
	case Plain:
		d.live = false
	default:
		d.live = d.detectTerminal()
	}

	// Resolved once, here, for the same reason live-versus-plain is: the
	// environment does not change part-way through a run, and a bar that
	// started colouring and stopped would leave the terminal's foreground
	// wherever the last sequence put it. Plain mode never colours at all --
	// those lines go into log files.
	if d.live && d.palette.Mode != Monochrome {
		d.depth = detectDepth(osGetenv)
	}

	d.interval = opts.Interval
	if d.interval <= 0 {
		if d.live {
			d.interval = DefaultInterval
		} else {
			d.interval = DefaultPlainInterval
		}
	}
	// When live, the first redraw is due immediately: the region should
	// appear the moment a long job starts, which is exactly when a user is
	// looking for it. When plain, the first line waits out an interval,
	// because these lines accumulate in a log rather than replacing each
	// other -- and a "0%" line recording that a job began carries nothing a
	// reader wants, least of all above the error from a job that failed in
	// its first second.
	if d.live {
		d.nextDue.Store(d.now().UnixNano())
	} else {
		d.nextDue.Store(d.now().Add(d.interval).UnixNano())
	}
	return d
}

// detectTerminal reports whether w can carry a live display: a real terminal
// that will interpret the escape sequences it is about to be sent.
//
// A forced Columns does not make a pipe into a terminal. The two are separate
// questions and conflating them is how a redirected run ends up with escape
// sequences in its log: a caller who knows the width still has to say Live to
// get live rendering.
func (d *Display) detectTerminal() bool {
	if d.file == nil {
		return false
	}
	if _, _, ok := terminalSize(d.file.Fd()); !ok {
		return false
	}
	return enableVirtualTerminal(d.file.Fd())
}

// Live reports whether this Display renders in place. Callers use it to
// decide what else to print, not to decide whether to report progress.
func (d *Display) Live() bool { return d != nil && d.live }

// Columns is the display's current width in terminal columns.
func (d *Display) Columns() int {
	if d == nil {
		return FallbackColumns
	}
	if d.columns > 0 {
		return d.columns
	}
	if d.file != nil {
		if cols, _, ok := terminalSize(d.file.Fd()); ok && cols > 0 {
			return cols
		}
	}
	return FallbackColumns
}

// Bar adds a live line and returns it. Bars render in the order they were
// added.
func (d *Display) Bar(spec BarSpec) *Bar {
	if d == nil {
		return nil
	}
	b := newBar(d, spec)
	d.mu.Lock()
	d.bars = append(d.bars, b)
	d.mu.Unlock()
	// Drawn at once when live, so the region appears the moment the job
	// starts. Never when plain: creating a bar is not progress, and the same
	// call would otherwise put a "0%" line in the log before anything had
	// happened. A job that fails in its first second should leave its error
	// in the log, not a stale zero above it, and one that finishes inside a
	// single interval should say nothing at all.
	if d.live {
		d.redraw(true)
	}
	return b
}

// Write emits p above the live region, which is what makes a logger and a set
// of bars coexist.
//
// A trailing newline is added when p lacks one. That is not tidiness: the
// live region is positioned by counting the lines written before it, so a
// fragment left without a newline would put the first bar on the end of it
// and the erase arithmetic would then be one row out for the rest of the run.
// Loggers terminate their lines; this covers everything else.
func (d *Display) Write(p []byte) (int, error) {
	if d == nil {
		return len(p), nil
	}
	if len(p) == 0 {
		return 0, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	// Nothing to step around when the display is stopped or was never live:
	// a plain display has no region on screen, so a write is just a write,
	// and re-emitting the bars after it would double every line in the log.
	if d.stopped || !d.live {
		return d.w.Write(p)
	}

	d.erase()
	n, err := d.w.Write(p)
	if err == nil && p[len(p)-1] != '\n' {
		if _, err2 := io.WriteString(d.w, "\n"); err2 != nil {
			err = err2
		}
	}
	d.draw()
	return n, err
}

// Stop erases the live region and releases the display. Every later Write
// passes straight through, so a program's final summary lands on a clean
// line.
//
// It must be deferred. A Display that is never stopped leaves its bars as the
// last thing on the terminal, and whatever the program prints next overwrites
// them half-way. Stop is idempotent.
func (d *Display) Stop() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	d.erase()
	d.bars = nil
	d.stopped = true
}

// redraw renders the live region, subject to the throttle unless force.
//
// The throttle is checked without the mutex, because this is reached from
// Bar.Set on every unit of work: at a few hundred calls a second, per-call
// lock contention across several bars would be a real cost paid by the work
// itself rather than by the reporting.
func (d *Display) redraw(force bool) {
	now := d.now()
	if !force {
		if now.UnixNano() < d.nextDue.Load() {
			return
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	d.nextDue.Store(now.Add(d.interval).UnixNano())
	d.draw()
}

// draw renders every bar, replacing whatever was on screen. The caller holds
// the mutex.
//
// It erases first, unconditionally. That is the difference between a display
// and a log: a redraw must REPLACE the region, and a draw that only appended
// would walk a fresh copy of every bar down the screen several times a second
// -- which looks almost right in a terminal, because the old copies scroll
// away, and is unmistakably wrong the moment the output is captured.
func (d *Display) draw() {
	d.erase()
	if len(d.bars) == 0 {
		return
	}
	now := d.now()
	if d.live {
		labels := make([]string, len(d.bars))
		fields := make([][numSlots]string, len(d.bars))
		for i, b := range d.bars {
			labels[i], fields[i] = b.slots(now)
		}
		l := d.resolveLayout(labels)

		var buf []byte
		for i, b := range d.bars {
			buf = append(buf, b.composeLine(labels[i], fields[i], l, d.measure)...)
			// Erase to end of line before the newline, so a line shorter
			// than the one it replaces does not leave the old tail behind.
			buf = append(buf, "\x1b[K\n"...)
		}
		_, _ = d.w.Write(buf)
		d.drawn = len(d.bars)
		return
	}
	// Plain: one line per bar, appended rather than replaced. erase above is
	// a no-op here (it does nothing unless live), so there is no cursor
	// state to keep and nothing to undo.
	for _, b := range d.bars {
		_, _ = io.WriteString(d.w, b.renderPlain(now)+"\n")
		b.plainDone = b.done.Load()
	}
}

// resolveLayout decides the geometry every bar on this redraw shares: one
// label column and one trough width.
//
// Laying the bars out independently is the obvious implementation and it
// looks wrong. Each line then sizes its own trough from its own text, so two
// bars whose labels or counts differ in length -- which is to say, any two
// bars -- close their troughs at different columns, and the whole region
// jitters sideways as the numbers grow. Sharing the geometry makes the set
// read as one thing, which is what it is.
//
// The trough gets whatever is left over, and when that is too little the
// LAST FIELD IS DROPPED FROM EVERY BAR rather than from the one that
// overflowed. Dropping per-bar would misalign them again, and inconsistently:
// a reader would see an ETA on one line and not the next for no reason
// visible on screen.
func (d *Display) resolveLayout(labels []string) layout {
	cols := d.Columns()
	l := layout{cols: cols, ascii: d.ascii}

	for _, label := range labels {
		if w := d.measure(label); w > l.labelWidth {
			l.labelWidth = w
		}
	}

	l.slots = numSlots
	head := l.labelWidth
	if head > 0 {
		head += 2 // the gap after the label column
	}
	for {
		textWidth := 0
		for _, b := range d.bars {
			if w := b.reservedWidth(l.slots, d.measure); w > textWidth {
				textWidth = w
			}
		}
		// Two brackets around the trough, and two spaces after it.
		room := cols - head - textWidth - 4
		if room >= minBarWidth {
			l.barWidth = room
			return l
		}
		if l.slots <= 1 {
			// No trough at all: the text keeps the room, and composeLine
			// truncates whatever still does not fit.
			l.barWidth = 0
			return l
		}
		l.slots--
	}
}

// erase removes the live region, leaving the cursor where it began. The
// caller holds the mutex.
//
// Cursor-up by the number of lines drawn, then clear from there to the end of
// the screen. This is why nothing may ever wrap: the count is of lines
// written, and a wrapped line occupies two rows while counting as one, so the
// cursor would come to rest one row too low and the next erase would consume
// the log line above the region.
func (d *Display) erase() {
	if !d.live || d.drawn == 0 {
		return
	}
	buf := make([]byte, 0, 16)
	buf = append(buf, '\r')
	buf = appendUint(append(buf, "\x1b["...), d.drawn)
	buf = append(buf, 'A')
	buf = append(buf, "\x1b[J"...)
	_, _ = d.w.Write(buf)
	d.drawn = 0
}

// appendUint appends a small non-negative integer without going through fmt,
// which is worth doing here only because erase and render are on the redraw
// path several times per second.
func appendUint(dst []byte, n int) []byte {
	if n == 0 {
		return append(dst, '0')
	}
	var tmp [20]byte
	i := len(tmp)
	for n > 0 {
		i--
		tmp[i] = byte('0' + n%10)
		n /= 10
	}
	return append(dst, tmp[i:]...)
}

// widthFunc is the resolved column measure, so a test can reach the same one
// the display uses rather than assuming which it defaulted to.
func (d *Display) widthFunc() func(string) int {
	if d == nil || d.measure == nil {
		return runewidth.StringWidth
	}
	return d.measure
}

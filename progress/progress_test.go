package progress

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// clock is an injected time source, so throttling and rates are tested by
// stating what time it is rather than by sleeping.
type clock struct{ t time.Time }

func newClock() *clock {
	return &clock{t: time.Date(2026, 3, 14, 9, 0, 0, 0, time.UTC)}
}
func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

// liveDisplay builds a Display rendering into a buffer as though it were a
// terminal of the given width.
func liveDisplay(cols int, c *clock) (*Display, *bytes.Buffer) {
	var buf bytes.Buffer
	d := New(&buf, Options{Mode: Live, Columns: cols, Now: c.now, Interval: time.Second})
	return d, &buf
}

// TestDisplay_RedrawReplacesTheRegionRatherThanRepeatingIt is the central
// mechanic, and the bug it caught was invisible in a terminal.
//
// A redraw that only appended would walk a fresh copy of every bar down the
// screen several times a second. On a real terminal the old copies scroll
// away and it looks very nearly right; captured to a file, or run beside log
// output, it is a mess. Asserting on the replayed SCREEN rather than on the
// byte stream is what makes the difference visible.
func TestDisplay_RedrawReplacesTheRegionRatherThanRepeatingIt(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "rendering", Total: 100, Unit: "frames"})

	for i := 1; i <= 5; i++ {
		c.add(time.Second)
		bar.Set(int64(i * 10))
	}

	sc := replay(buf.String())
	if len(sc.junk) > 0 {
		t.Errorf("display emitted escape sequences this test does not model: %v", sc.junk)
	}
	lines := sc.text()
	if len(lines) != 1 {
		t.Fatalf("screen shows %d lines, want exactly 1 bar:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[0], "50%") {
		t.Errorf("the surviving line is not the latest state: %q", lines[0])
	}
}

// TestDisplay_LogsScrollAboveTheBars is the requirement the whole design
// exists for: a program that logs while it works must be able to do both.
func TestDisplay_LogsScrollAboveTheBars(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "rendering", Total: 100, Unit: "frames"})

	c.add(time.Second)
	bar.Set(20)
	fmt.Fprintln(d, "warn  clip-0043 has no GPS fixes")
	c.add(time.Second)
	bar.Set(40)
	fmt.Fprintln(d, "info  merged 5 files")
	c.add(time.Second)
	bar.Set(60)

	lines := replay(buf.String()).text()
	if len(lines) != 3 {
		t.Fatalf("screen shows %d lines, want 2 log lines and 1 bar:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if lines[0] != "warn  clip-0043 has no GPS fixes" || lines[1] != "info  merged 5 files" {
		t.Errorf("log lines were not preserved verbatim above the bar:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[2], "60%") {
		t.Errorf("the bar is not the last line, or is stale: %q", lines[2])
	}
}

// TestDisplay_WriteWithoutATrailingNewlineDoesNotShiftTheRegion pins why
// Write appends one.
//
// The live region is positioned by counting lines. A fragment written without
// a newline would leave the first bar on the end of it, and every erase
// afterwards would then be one row short -- eating the log line above the
// region on every redraw for the rest of the run.
func TestDisplay_WriteWithoutATrailingNewlineDoesNotShiftTheRegion(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "rendering", Total: 100})

	fmt.Fprint(d, "a fragment with no newline")
	c.add(time.Second)
	bar.Set(50)
	fmt.Fprintln(d, "a proper line")
	c.add(time.Second)
	bar.Set(60)

	lines := replay(buf.String()).text()
	if len(lines) != 3 {
		t.Fatalf("screen shows %d lines, want 2 log lines and 1 bar:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if lines[0] != "a fragment with no newline" || lines[1] != "a proper line" {
		t.Errorf("log lines are not intact and in order:\n%s", strings.Join(lines, "\n"))
	}
}

// TestDisplay_NeverExceedsTheTerminalWidth is the invariant everything else
// depends on, asserted across widths and shapes rather than at one size.
//
// A line that wraps occupies two rows while the erase arithmetic counts one,
// and from then on the display eats the line above it on every redraw. This
// caught a real off-by-two: the trough and the line composer each wrote the
// gap after the bar, so every line ran two columns over.
func TestDisplay_NeverExceedsTheTerminalWidth(t *testing.T) {
	specs := []BarSpec{
		{Label: "rendering", Total: 108000, Unit: "frames"},
		{Label: "encoding", Total: 0, Unit: "frames"},
		{Label: "a considerably longer label than the others", Total: 3, Unit: "clips"},
		{Label: "", Total: 100},
		{Label: "描画中", Total: 500, Unit: "コマ"}, // wide characters: columns != runes != bytes
	}
	for _, cols := range []int{8, 12, 20, 30, 40, 60, 80, 100, 160, 200} {
		for _, ascii := range []bool{false, true} {
			c := newClock()
			var buf bytes.Buffer
			d := New(&buf, Options{Mode: Live, Columns: cols, Now: c.now, Interval: time.Second, ASCII: ascii})
			bars := make([]*Bar, len(specs))
			for i, s := range specs {
				bars[i] = d.Bar(s)
			}
			for step := 1; step <= 4; step++ {
				c.add(time.Second)
				for i, b := range bars {
					b.Set(int64(step * (i + 1) * 7))
				}
			}
			d.Stop()

			if w, worst := replay(buf.String()).widest(); w > cols {
				t.Errorf("cols=%d ascii=%v: a line is %d columns wide:\n%q", cols, ascii, w, worst)
			}
		}
	}
}

// TestDisplay_BarsShareOneTroughColumn pins the layout decision that makes a
// multi-bar display read as one thing. Laid out independently, each line
// sizes its trough from its own text and the region jitters sideways as the
// numbers grow.
func TestDisplay_BarsShareOneTroughColumn(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(90, c)
	a := d.Bar(BarSpec{Label: "rendering", Total: 108000, Unit: "frames"})
	b := d.Bar(BarSpec{Label: "enc", Total: 900, Unit: "f"})

	c.add(time.Second)
	a.Set(5000)
	b.Set(100)

	lines := replay(buf.String()).text()
	if len(lines) != 2 {
		t.Fatalf("want 2 bars, got %d:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if x, y := columnOf(lines[0], barClose), columnOf(lines[1], barClose); x != y || x < 0 {
		t.Errorf("troughs close at columns %d and %d, want the same column:\n%s", x, y, strings.Join(lines, "\n"))
	}
}

// TestDisplay_StopLeavesNothingBehind pins the end state. A display that did
// not clean up would leave its bars as the last thing on the terminal, and
// the program's own summary would overwrite them half-way.
func TestDisplay_StopLeavesNothingBehind(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "rendering", Total: 100})
	c.add(time.Second)
	bar.Set(50)
	fmt.Fprintln(d, "a log line")
	c.add(time.Second)
	bar.Set(60)

	d.Stop()
	fmt.Fprintln(d, "the summary")

	lines := replay(buf.String()).text()
	want := []string{"a log line", "the summary"}
	if len(lines) != len(want) {
		t.Fatalf("screen shows %d lines, want %d:\n%s", len(lines), len(want), strings.Join(lines, "\n"))
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	d.Stop() // idempotent
}

// TestDisplay_BarDoneRemovesItsLine covers a worker finishing while others
// keep going -- the shape videofx's parallel jobs produce.
func TestDisplay_BarDoneRemovesItsLine(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	a := d.Bar(BarSpec{Label: "clip-1", Total: 100})
	b := d.Bar(BarSpec{Label: "clip-2", Total: 100})

	c.add(time.Second)
	a.Set(50)
	b.Set(10)
	a.Done()
	c.add(time.Second)
	b.Set(20)

	lines := replay(buf.String()).text()
	if len(lines) != 1 {
		t.Fatalf("screen shows %d lines, want just the unfinished bar:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[0], "clip-2") {
		t.Errorf("the wrong bar survived: %q", lines[0])
	}
}

// TestDisplay_PlainModeEmitsNoEscapeSequences is what makes `program 2>log`
// safe. A redirected run must never collect cursor movements.
func TestDisplay_PlainModeEmitsNoEscapeSequences(t *testing.T) {
	c := newClock()
	var buf bytes.Buffer
	d := New(&buf, Options{Mode: Plain, Now: c.now, Interval: time.Second})
	bar := d.Bar(BarSpec{Label: "rendering", Total: 100, Unit: "frames"})
	for i := 1; i <= 3; i++ {
		c.add(time.Second)
		bar.Set(int64(i * 10))
	}
	fmt.Fprintln(d, "a log line")
	d.Stop()

	out := buf.String()
	if strings.ContainsRune(out, '\x1b') {
		t.Errorf("plain mode emitted an escape sequence:\n%q", out)
	}
	if strings.ContainsRune(out, '\r') {
		t.Errorf("plain mode emitted a carriage return:\n%q", out)
	}
	if n := strings.Count(out, "\n"); n < 4 {
		t.Errorf("plain mode wrote %d lines, want one per update plus the log line:\n%q", n, out)
	}
	if !strings.Contains(out, "a log line") {
		t.Errorf("the log line was lost:\n%q", out)
	}
}

// TestDisplay_ThrottleHoldsBackRedraws pins that Set is safe to call per unit
// of work. A frame loop calls it hundreds of times a second and must not pay
// for a redraw each time.
func TestDisplay_ThrottleHoldsBackRedraws(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "rendering", Total: 1000})

	// Well inside one interval: hundreds of updates, none of them due.
	before := buf.Len()
	for i := 0; i < 500; i++ {
		c.add(time.Millisecond)
		bar.Set(int64(i))
	}
	if grew := buf.Len() - before; grew != 0 {
		t.Errorf("500 updates inside a 1s interval wrote %d bytes, want none", grew)
	}

	// Crossing it emits exactly once, however many updates cross it.
	before = buf.Len()
	c.add(time.Second)
	for i := 0; i < 100; i++ {
		bar.Set(int64(500 + i))
	}
	if buf.Len() == before {
		t.Fatal("crossing the interval wrote nothing; the throttle never lets anything through")
	}
	if n := replay(buf.String()[before:]).text(); len(n) != 1 {
		t.Errorf("crossing the interval produced %d lines, want 1 redraw for the whole burst", len(n))
	}
}

// TestDisplay_SetDoesNotAllocateWhenItDoesNotRedraw protects the hot path.
// Set is called once per frame in both intended consumers; an allocation
// there is paid by the work rather than by the reporting.
func TestDisplay_SetDoesNotAllocateWhenItDoesNotRedraw(t *testing.T) {
	c := newClock()
	d, _ := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "rendering", Total: 1000})
	c.add(time.Second)
	bar.Set(1) // let the first redraw happen, so the rest are throttled

	if n := testing.AllocsPerRun(200, func() { bar.Set(2) }); n != 0 {
		t.Errorf("Set allocated %v objects per call on the non-redrawing path, want 0", n)
	}
}

// TestDisplay_NilIsUsable covers the --quiet path both consumers want: decide
// once, at construction, instead of guarding every call site.
func TestDisplay_NilIsUsable(t *testing.T) {
	var d *Display
	bar := d.Bar(BarSpec{Label: "rendering", Total: 10})
	bar.Set(1)
	bar.Add(1)
	bar.SetTotal(5)
	bar.Done()
	if n, err := d.Write([]byte("ignored")); n != len("ignored") || err != nil {
		t.Errorf("Write on a nil Display = (%d, %v), want (%d, nil)", n, err, len("ignored"))
	}
	if d.Live() {
		t.Error("a nil Display reports Live")
	}
	if d.Columns() != FallbackColumns {
		t.Errorf("Columns on a nil Display = %d, want %d", d.Columns(), FallbackColumns)
	}
	d.Stop()
}

// TestNew_DetectsPlainForSomethingThatIsNotATerminal is the check that keeps
// escape sequences out of redirected output, and it is deliberately made
// against a real file rather than a bytes.Buffer: a buffer is not an *os.File
// at all, so it would pass even if the detection were broken.
func TestNew_DetectsPlainForSomethingThatIsNotATerminal(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if d := New(f, Options{}); d.Live() {
		t.Error("a regular file was detected as a terminal")
	}
	// os.DevNull is the case a Stat/ModeCharDevice check gets wrong: it IS a
	// character device, so that test reports a terminal and the program
	// writes escape sequences into it.
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("cannot open %s: %v", os.DevNull, err)
	}
	defer null.Close()
	if d := New(null, Options{}); d.Live() {
		t.Errorf("%s was detected as a terminal", os.DevNull)
	}
}

// TestDisplay_DoesNotTruncateWhatWouldHaveFitted guards the layout
// arithmetic, which the width invariant alone cannot.
//
// composeLine truncates anything still too long, which is the right backstop
// and is also why a mistake in resolveLayout's accounting does not show up as
// an over-wide line: it shows up as a line quietly missing its right-hand
// fields on a terminal with room to spare. That is the failure this asserts
// against. It caught a real one, where the trough and the line composer each
// wrote the gap after the bar and every line lost two columns' worth of
// information it had room for.
func TestDisplay_DoesNotTruncateWhatWouldHaveFitted(t *testing.T) {
	specs := []BarSpec{
		{Label: "rendering", Total: 108000, Unit: "frames"},
		{Label: "enc", Total: 900, Unit: "f"},
		{Label: "描画中", Total: 500, Unit: "コマ"},
	}
	for _, cols := range []int{100, 120, 160, 200} {
		c := newClock()
		var buf bytes.Buffer
		d := New(&buf, Options{Mode: Live, Columns: cols, Now: c.now, Interval: time.Second})
		bars := make([]*Bar, len(specs))
		for i, s := range specs {
			bars[i] = d.Bar(s)
		}
		c.add(30 * time.Second)
		for i, b := range bars {
			b.Set(int64((i + 1) * 137))
		}

		for _, line := range replay(buf.String()).text() {
			if strings.HasSuffix(line, "…") {
				t.Errorf("cols=%d: a line was truncated with room to spare (%d columns used of %d):\n%q",
					cols, len([]rune(line)), cols, line)
			}
		}
	}
}

// troughColumn is where the bar currently on screen closes its trough, in
// display columns, or -1 when there is no trough.
//
// It is sampled after each update rather than parsed out of the whole stream,
// because a redraw REPLACES the region: replaying the finished stream shows
// only the last state, which is exactly the one frame that cannot reveal the
// trough moving.
func troughColumn(t *testing.T, out string) int {
	t.Helper()
	lines := replay(out).text()
	if len(lines) == 0 {
		return -1
	}
	return columnOf(lines[len(lines)-1], barClose)
}

// TestDisplay_TroughWidthIsStaticThroughoutTheRun is the reason the field
// widths are reserved up front rather than measured each redraw.
//
// Sized from the current values, the trough is re-fitted around whatever the
// numbers happen to be at that instant: it shrinks a column every time the
// counts gain a digit, and lurches when the rate and the estimate appear a
// second in -- which is the most visible of the lot, because by then the eye
// has settled on a bar that is not moving sideways any more.
//
// The run below deliberately spans all three: the first redraw before any
// rate has been measured, the moment the estimate becomes available, and the
// counts growing from one digit to six.
func TestDisplay_TroughWidthIsStaticThroughoutTheRun(t *testing.T) {
	c := newClock()
	var buf bytes.Buffer
	d := New(&buf, Options{Mode: Live, Columns: 100, Now: c.now, Interval: 100 * time.Millisecond})
	bar := d.Bar(BarSpec{Label: "rendering", Total: 108000, Unit: "frames"})

	var cols []int
	sample := func() { cols = append(cols, troughColumn(t, buf.String())) }
	sample() // the first draw, before any rate or estimate exists

	for done := int64(1); done <= 108000; done *= 3 {
		c.add(300 * time.Millisecond)
		bar.Set(done)
		sample()
	}
	c.add(300 * time.Millisecond)
	bar.Set(108000)
	sample()

	if len(cols) < 6 {
		t.Fatalf("only %d samples; this test needs the run to cover the rate and estimate appearing", len(cols))
	}
	for i, got := range cols {
		if got < 0 {
			t.Fatalf("sample %d has no trough at all; widths seen: %v", i, cols)
		}
		if got != cols[0] {
			t.Errorf("sample %d closes its trough at column %d, want %d (fixed for the whole run); widths seen: %v",
				i, got, cols[0], cols)
			break
		}
	}
}

// TestDisplay_TroughNeverWidensAfterNarrowing covers the cases a reservation
// cannot predict: a rate larger than the room kept for it, and a total that
// only arrives once the job is under way.
//
// Neither can be estimated in advance, so the trough does have to give up
// room when they happen. What it must never do is take that room back, which
// would leave it oscillating for the rest of the run -- so the widths only
// ever grow and the trough only ever narrows.
func TestDisplay_TroughNeverWidensAfterNarrowing(t *testing.T) {
	c := newClock()
	var buf bytes.Buffer
	d := New(&buf, Options{Mode: Live, Columns: 100, Now: c.now, Interval: 100 * time.Millisecond})
	bar := d.Bar(BarSpec{Label: "encoding", Unit: "frames"}) // no total yet

	var cols []int
	step := func(f func()) {
		c.add(time.Second)
		f()
		cols = append(cols, troughColumn(t, buf.String()))
	}
	step(func() { bar.Set(50) })
	step(func() { bar.Set(4_000_000) })      // a rate far past what was reserved
	step(func() { bar.SetTotal(9_000_000) }) // a total, bringing a percentage and an estimate
	step(func() { bar.Set(5_000_000) })
	step(func() { bar.Set(6_000_000) })

	// Samples before a total arrives have no trough at all (an unsized job
	// draws none, see Bar.hasBar), so they are dropped rather than compared:
	// a trough appearing where there was none is the job's shape changing,
	// not the width oscillating.
	var seen []int
	for _, c := range cols {
		if c >= 0 {
			seen = append(seen, c)
		}
	}
	if len(seen) < 2 {
		t.Fatalf("only %d samples had a trough; widths seen: %v", len(seen), cols)
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] > seen[i-1] {
			t.Errorf("the trough grew back from column %d to %d; widths seen: %v", seen[i-1], seen[i], cols)
			break
		}
	}
}

// sgrPattern matches the colour sequences a Display emits.
var sgrPattern = regexp.MustCompile(`\x1b\[(38;2;\d+;\d+;\d+|38;5;\d+|39)m`)

// coloursIn returns every distinct colour sequence in the output, in the
// order they first appear, ignoring the resets between them.
func coloursIn(out string) []string {
	var seen []string
	index := map[string]bool{}
	for _, m := range sgrPattern.FindAllString(out, -1) {
		if m == colourReset || index[m] {
			continue
		}
		index[m] = true
		seen = append(seen, m)
	}
	return seen
}

// TestDisplay_PaletteModes covers the three shapes a caller can ask for, and
// that the default is none of them.
//
// Monochrome is the zero value deliberately: a program's output should not
// acquire escape sequences because a library thought they would look nice.
func TestDisplay_PaletteModes(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("NO_COLOR", "")

	cases := []struct {
		name    string
		palette Palette
		want    int // distinct colours expected
	}{
		{"the zero value is monochrome", Palette{}, 0},
		{"solid uses exactly one colour", SolidPalette(RGB{0x11, 0x22, 0x33}), 1},
		{"a gradient uses many", DefaultGradient(), 2}, // at least
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clk := newClock()
			var buf bytes.Buffer
			d := New(&buf, Options{Mode: Live, Columns: 100, Now: clk.now, Interval: time.Second, Palette: c.palette})
			bar := d.Bar(BarSpec{Label: "rendering", Total: 100, Unit: "frames"})
			clk.add(time.Second)
			bar.Set(80)

			got := coloursIn(buf.String())
			switch {
			case c.want == 0 && len(got) != 0:
				t.Errorf("monochrome emitted colour: %v", got)
			case c.want == 1 && len(got) != 1:
				t.Errorf("solid emitted %d distinct colours, want exactly 1: %v", len(got), got)
			case c.want == 2 && len(got) < 2:
				t.Errorf("a gradient emitted %d distinct colours, want several: %v", len(got), got)
			}
			if c.want > 0 && !strings.Contains(buf.String(), colourReset) {
				t.Error("colour was left set: no reset was emitted")
			}
		})
	}
}

// TestDisplay_ColourNeverChangesTheWidth is the invariant colour was deferred
// for when this package was first written.
//
// A line's width is now accumulated as it is built rather than measured
// afterwards, precisely because the assembled string holds escape sequences:
// measuring it would count their digits as columns, judge the bar too wide,
// and truncate it -- cutting an escape sequence in half. The replayed screen
// ignores colour, so this compares what a terminal would actually show.
func TestDisplay_ColourNeverChangesTheWidth(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("NO_COLOR", "")

	specs := []BarSpec{
		{Label: "rendering", Total: 108000, Unit: "frames"},
		{Label: "描画中", Total: 500, Unit: "コマ"},
		{Label: "encoding", Total: 0, Unit: "frames"},
	}
	for _, palette := range []Palette{{}, SolidPalette(RGB{0, 200, 0}), DefaultGradient()} {
		for _, cols := range []int{12, 30, 60, 100, 160} {
			clk := newClock()
			var buf bytes.Buffer
			d := New(&buf, Options{Mode: Live, Columns: cols, Now: clk.now, Interval: time.Second, Palette: palette})
			bars := make([]*Bar, len(specs))
			for i, sp := range specs {
				bars[i] = d.Bar(sp)
			}
			for step := 1; step <= 4; step++ {
				clk.add(time.Second)
				for i, b := range bars {
					b.Set(int64(step * (i + 1) * 997))
				}
			}
			d.Stop()

			sc := replay(buf.String())
			if len(sc.junk) > 0 {
				t.Errorf("mode=%v cols=%d: unmodelled escape sequences %v", palette.Mode, cols, sc.junk)
			}
			if w, worst := sc.widest(); w > cols {
				t.Errorf("mode=%v cols=%d: a line is %d columns wide: %q", palette.Mode, cols, w, worst)
			}
		}
	}
}

// TestDisplay_ColourIsRefusedWhereItDoesNotBelong covers every case where a
// caller asks for colour and must not get it. Each is a place escape
// sequences would end up somewhere they cannot be seen and cannot be removed
// -- a log file, a pipe, or the terminal of someone who asked for none.
func TestDisplay_ColourIsRefusedWhereItDoesNotBelong(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		mode Mode
	}{
		{"NO_COLOR is set", map[string]string{"NO_COLOR": "1"}, Live},
		{"TERM is dumb", map[string]string{"TERM": "dumb"}, Live},
		{"the display is not live", nil, Plain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("COLORTERM", "truecolor")
			for k, v := range c.env {
				t.Setenv(k, v)
			}

			clk := newClock()
			var buf bytes.Buffer
			d := New(&buf, Options{Mode: c.mode, Columns: 100, Now: clk.now, Interval: time.Second, Palette: DefaultGradient()})
			bar := d.Bar(BarSpec{Label: "rendering", Total: 100, Unit: "frames"})
			clk.add(2 * time.Second)
			bar.Set(50)
			d.Stop()

			if got := coloursIn(buf.String()); len(got) != 0 {
				t.Errorf("colour was emitted anyway: %v", got)
			}
		})
	}
}

// TestDisplay_FallsBackTo256Colours pins that a 24-bit sequence is only sent
// where COLORTERM claims it will be understood. The failure is silent and
// ugly: a terminal that does not know the form prints its digits across the
// bar.
func TestDisplay_FallsBackTo256Colours(t *testing.T) {
	for _, c := range []struct{ colorterm, want string }{
		{"truecolor", "38;2;"},
		{"24bit", "38;2;"},
		{"", "38;5;"},
		{"something else", "38;5;"},
	} {
		t.Run("COLORTERM="+c.colorterm, func(t *testing.T) {
			t.Setenv("NO_COLOR", "")
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("COLORTERM", c.colorterm)

			clk := newClock()
			var buf bytes.Buffer
			d := New(&buf, Options{Mode: Live, Columns: 100, Now: clk.now, Interval: time.Second, Palette: DefaultGradient()})
			bar := d.Bar(BarSpec{Label: "rendering", Total: 100})
			clk.add(time.Second)
			bar.Set(50)

			if !strings.Contains(buf.String(), c.want) {
				t.Errorf("want %q sequences; colours seen: %v", c.want, coloursIn(buf.String()))
			}
		})
	}
}

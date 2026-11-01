package progress

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestBar_UnknownTotalReportsWhatItKnowsAndNothingMore is the absence rule
// applied to a progress bar.
//
// A job whose size is not known has no percentage and no estimate, and must
// not be given an empty trough either: a full-width bar that never fills
// reads as a bar that is stuck, not as a measurement nobody has. What it does
// have -- a count and a rate, both moving -- is the honest report.
func TestBar_UnknownTotalReportsWhatItKnowsAndNothingMore(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "encoding", Total: 0, Unit: "frames"})
	c.add(time.Second)
	bar.Set(500)

	line := replay(buf.String()).text()[0]
	for _, unwanted := range []string{"%", "left", string(barOpen), string(barClose)} {
		if strings.Contains(line, unwanted) {
			t.Errorf("an unsized job's line contains %q, which it cannot know: %q", unwanted, line)
		}
	}
	if !strings.Contains(line, "500 frames") {
		t.Errorf("the count is missing: %q", line)
	}
	if !strings.Contains(line, "/s") {
		t.Errorf("the rate is missing: %q", line)
	}
}

// TestBar_SetTotalLater covers a caller that learns the size after starting,
// which is the shape of anything that has to probe its input first.
func TestBar_SetTotalLater(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "encoding", Unit: "frames"})
	c.add(time.Second)
	bar.Set(50)
	if line := replay(buf.String()).text()[0]; strings.Contains(line, "%") {
		t.Fatalf("a percentage before any total was known: %q", line)
	}

	c.add(time.Second)
	bar.SetTotal(200)
	if line := replay(buf.String()).text()[0]; !strings.Contains(line, "25%") {
		t.Errorf("no percentage after the total became known: %q", line)
	}
}

// TestBar_RateHoldsSteadyBetweenMeasurements pins the reason the rate window
// is longer than the redraw interval.
//
// Redrawing twelve times a second is what makes a bar look continuous, but a
// rate measured over 80ms is dominated by whatever the scheduler was doing
// in those 80ms and swings by an order of magnitude between redraws. The
// previous figure is held instead, so the number moves when the work's speed
// moves and not before.
func TestBar_RateHoldsSteadyBetweenMeasurements(t *testing.T) {
	c := newClock()
	var buf bytes.Buffer
	// The redraw interval is deliberately far SHORTER than the rate window:
	// that gap is the thing under test, and a display whose interval
	// outlasts its rate window could not exhibit the bug either way.
	d := New(&buf, Options{Mode: Live, Columns: 80, Now: c.now, Interval: minRateWindow / 10})
	bar := d.Bar(BarSpec{Label: "rendering", Total: 100000, Unit: "frames"})

	// One full window at a steady 1000/s establishes the rate.
	c.add(minRateWindow)
	bar.Set(int64(minRateWindow.Seconds() * 1000))
	first := rateOf(t, replay(buf.String()).text()[0])

	// A redraw inside the next window must not restate the rate from a
	// sliver of time.
	buf.Reset()
	c.add(minRateWindow / 5) // past the redraw interval, inside the rate window
	bar.Set(int64(minRateWindow.Seconds()*1000) + 1)
	second := rateOf(t, replay(buf.String()).text()[0])

	if first != second {
		t.Errorf("rate moved from %q to %q on a redraw inside one rate window", first, second)
	}
}

func rateOf(t *testing.T, line string) string {
	t.Helper()
	for _, f := range strings.Fields(line) {
		if strings.HasSuffix(f, "/s") {
			return f
		}
	}
	t.Fatalf("no rate in %q", line)
	return ""
}

// TestBar_NoEstimateUntilThereIsSomethingToEstimateFrom keeps the display
// from presenting noise as information. An estimate extrapolated from two
// frames is a number nobody should act on.
func TestBar_NoEstimateUntilThereIsSomethingToEstimateFrom(t *testing.T) {
	c := newClock()
	// Redrawing every millisecond, so the Set at 100ms redraws: at the
	// display's second it was held back, and the line asserted on below
	// was the one drawn at the start, which says nothing either way.
	var out bytes.Buffer
	d := New(&out, Options{Mode: Live, Columns: 80, Now: c.now, Interval: time.Millisecond})
	buf := &out
	bar := d.Bar(BarSpec{Label: "rendering", Total: 100000, Unit: "frames"})

	c.add(100 * time.Millisecond)
	bar.Set(3)
	if line := replay(buf.String()).text()[0]; strings.Contains(line, "left") {
		t.Errorf("an estimate from 100ms of work: %q", line)
	}

	buf.Reset()
	c.add(30 * time.Second)
	bar.Set(30000)
	if line := replay(buf.String()).text()[0]; !strings.Contains(line, "left") {
		t.Errorf("no estimate after 30s and a third of the work: %q", line)
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{45 * time.Second, "45s"},
		{90 * time.Second, "1m30s"},
		{59*time.Minute + 59*time.Second, "59m59s"},
		{90 * time.Minute, "1h30m"},
		{25 * time.Hour, "25h00m"},
		{-time.Second, "0s"},
		// Rounded to the unit before the form is chosen: 59.6 s is not
		// "59s", truncated, and 59 min 59.6 s is not "60m00s".
		{59*time.Second + 600*time.Millisecond, "1m00s"},
		{59*time.Minute + 59*time.Second + 600*time.Millisecond, "1h00m"},
	}
	for _, c := range cases {
		if got := formatDuration(c.d); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.d, got, c.want)
		}
	}
}

func TestPercentClamps(t *testing.T) {
	cases := []struct {
		done, total int64
		want        int
	}{
		{0, 100, 0}, {50, 100, 50}, {100, 100, 100},
		{150, 100, 100}, // a caller overshooting must not report 150%
		{-5, 100, 0},
		{1, 0, 0}, // no total: percent is meaningless, never a division
		// done*100 overflowed an int64 here, and read as 0%.
		{1 << 62, 1<<62 + 1, 99},
	}
	for _, c := range cases {
		if got := percent(c.done, c.total); got != c.want {
			t.Errorf("percent(%d, %d) = %d, want %d", c.done, c.total, got, c.want)
		}
	}
}

// TestTruncate_MeasuresColumnsNotBytesOrRunes is the wide-character case.
// Bytes, runes and display columns are three different numbers and the
// failure is silent -- a label that looks fine until somebody's data is not
// ASCII.
func TestTruncate_MeasuresColumnsNotBytesOrRunes(t *testing.T) {
	measure := (&Display{}).widthFunc()
	const wide = "描画描画描画" // six runes, twelve columns, eighteen bytes

	got := truncate(wide, 7, measure)
	if w := measure(got); w > 7 {
		t.Errorf("truncate(%q, 7) = %q, which is %d columns", wide, got, w)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncate did not mark the cut: %q", got)
	}
	if unchanged := truncate(wide, 12, measure); unchanged != wide {
		t.Errorf("truncate cut a string that already fitted: %q", unchanged)
	}
}

// A label is cut between characters, as a table's cell and a chart's
// title are: walked rune by rune, a family of emoji joined by zero-width
// joiners was cut part way and left a joiner dangling before the ellipsis,
// and a flag of two regional indicators became half of one.
func TestTruncate_KeepsCharactersWhole(t *testing.T) {
	measure := (&Display{}).widthFunc()
	for _, c := range []struct{ s, unit string }{
		{strings.Repeat("👨‍👩‍👧", 6), "👨‍👩‍👧"},
		{strings.Repeat("🇩🇰", 6), "🇩🇰"},
	} {
		for cols := 2; cols <= 9; cols++ {
			got := truncate(c.s, cols, measure)
			if w := measure(got); w > cols {
				t.Errorf("truncate(%q, %d) = %q, %d columns", c.s, cols, got, w)
			}
			if rest := strings.ReplaceAll(strings.TrimSuffix(got, "…"), c.unit, ""); rest != "" {
				t.Errorf("truncate(%q, %d) = %q: part of a character, %q, left", c.s, cols, got, rest)
			}
		}
	}
}

// TestBar_ASCIIAvoidsTheBlockGlyphs covers the option for a font without
// U+2588..U+258F.
func TestBar_ASCIIAvoidsTheBlockGlyphs(t *testing.T) {
	c := newClock()
	var buf bytes.Buffer
	d := New(&buf, Options{Mode: Live, Columns: 80, Now: c.now, Interval: time.Second, ASCII: true})
	bar := d.Bar(BarSpec{Label: "rendering", Total: 100})
	c.add(time.Second)
	bar.Set(50)

	line := replay(buf.String()).text()[0]
	for _, r := range line {
		if r >= '▀' && r <= '▟' {
			t.Errorf("ASCII mode drew a block glyph %q in %q", r, line)
		}
	}
	if !strings.Contains(line, "=") || !strings.Contains(line, "[") {
		t.Errorf("ASCII mode drew no bar at all: %q", line)
	}
}

// A bar counting bytes writes them as bytes are read: both counts in the
// total's prefix, so the field keeps its shape as the done figure grows
// through the prefixes on its way to the total, and the rate in its own.
// Counted as units, a 945 MiB download read "312475648/990904320 B" and
// "3412k/s".
func TestBar_BytesAreWrittenInBinaryPrefixes(t *testing.T) {
	for _, c := range []struct {
		done, total int64
		want        string
	}{
		{311_951_360, 990_904_320, "297.5/945.0 MiB"},
		{0, 990_904_320, "0.0/945.0 MiB"},
		{512, 2048, "0.5/2.0 KiB"},
		{100, 900, "100/900 B"},
		{3 << 30, 0, "3.0 GiB"},
	} {
		if got := formatBytes(c.done, c.total); got != c.want {
			t.Errorf("formatBytes(%d, %d) = %q, want %q", c.done, c.total, got, c.want)
		}
	}
	for _, c := range []struct {
		rate float64
		want string
	}{
		{3.4 * 1024 * 1024, "3.4 MiB/s"},
		{512, "512 B/s"},
		{0, ""},
	} {
		if got := formatByteRate(c.rate); got != c.want {
			t.Errorf("formatByteRate(%g) = %q, want %q", c.rate, got, c.want)
		}
	}

	clk := newClock()
	d, buf := liveDisplay(100, clk)
	bar := d.Bar(BarSpec{Label: "map data", Total: 990_904_320, Unit: "B", Bytes: true})
	clk.add(time.Second)
	bar.Set(311_951_360)
	line := replay(buf.String()).text()[0]
	if !strings.Contains(line, "297.5/945.0 MiB") || !strings.Contains(line, "MiB/s") {
		t.Errorf("a byte bar's line: %q", line)
	}
	if strings.Contains(line, " B") && !strings.Contains(line, "MiB") {
		t.Errorf("the unit was printed beside the prefix: %q", line)
	}
}

// TestFormatRateAndBytesKeepTheirWidth: a figure just under a bound rounded
// up into the next one's width -- "10000k/s", eight columns where seven are
// reserved, and "1024.0/1024.0 KiB" where the next prefix reads 1.0.
func TestFormatRateAndBytesKeepTheirWidth(t *testing.T) {
	for v, want := range map[float64]string{9.96: "10/s", 9_999.6: "10k/s", 9_999_600: "10M/s", 9_999_000: "9999k/s"} {
		if got := formatRate(v); got != want {
			t.Errorf("formatRate(%v) = %q, want %q", v, got, want)
		}
	}
	for v := 1.0; v < 1e9; v *= 1.37 {
		if n := len(formatRate(v)); n > 7 {
			t.Errorf("formatRate(%v) = %q, %d columns", v, formatRate(v), n)
		}
	}
	if got := formatBytes(1048575, 1048575); got != "1.0/1.0 MiB" {
		t.Errorf("formatBytes(1048575) = %q, want 1.0/1.0 MiB", got)
	}
}

// TestBar_StalledBarShowsNoRate: a job that has stopped must not go on
// showing the speed it last ran at. It once held its last rate for ever,
// 300/s an hour after the last frame, which reads as a job running.
func TestBar_StalledBarShowsNoRate(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(100, c)
	stuck := d.Bar(BarSpec{Label: "stuck", Total: 1000})
	moving := d.Bar(BarSpec{Label: "moving", Total: 1000000})
	c.add(time.Second)
	stuck.Set(300)
	for i := range 60 {
		c.add(time.Second)
		moving.Set(int64(i * 100))
	}
	lines := replay(buf.String()).text()
	if len(lines) != 2 {
		t.Fatalf("%d lines: %q", len(lines), lines)
	}
	if strings.Contains(lines[0], "/s") {
		t.Errorf("a bar idle for a minute still shows a rate: %q", lines[0])
	}
	if !strings.Contains(lines[1], "/s") {
		t.Errorf("the moving bar lost its rate: %q", lines[1])
	}
}

// TestBar_UnsizedBarKeepsItsCount: where no bar has a total there is no
// trough to make room for, and an unsized bar's first field is its count.
// The room once held for a trough it never draws pushed its count to the
// far edge, and in a narrow terminal took the count away altogether.
func TestBar_UnsizedBarKeepsItsCount(t *testing.T) {
	for _, cols := range []int{20, 25, 30, 80} {
		c := newClock()
		d, buf := liveDisplay(cols, c)
		bar := d.Bar(BarSpec{Label: "rendering", Unit: "frames"})
		c.add(time.Second)
		bar.Set(42)
		line := replay(buf.String()).text()[0]
		if !strings.Contains(line, "42") {
			t.Errorf("cols=%d: no count in %q", cols, line)
		}
		if strings.Contains(line, "      ") {
			t.Errorf("cols=%d: a gap held for a trough in %q", cols, line)
		}
	}
}

// TestPlain_FragmentIsEndedBeforeAProgressLine: a plain display writes what
// it is given as it is, but its next progress line begins a line of its own
// rather than running on from a fragment. And a bar done twice says its
// closing line once.
func TestPlain_FragmentIsEndedBeforeAProgressLine(t *testing.T) {
	c := newClock()
	var buf bytes.Buffer
	d := New(&buf, Options{Mode: Plain, Now: c.now, Interval: time.Second})
	bar := d.Bar(BarSpec{Label: "x", Total: 10})
	c.add(time.Second)
	bar.Set(3)
	_, _ = d.Write([]byte("partial"))
	c.add(time.Second)
	bar.Set(5)
	if !strings.Contains(buf.String(), "partial\nx 50%") {
		t.Errorf("the progress line ran on from the fragment: %q", buf.String())
	}
	bar.Set(10)
	bar.Done()
	bar.Done()
	if n := strings.Count(buf.String(), "x 100%"); n != 1 {
		t.Errorf("the closing line written %d times: %q", n, buf.String())
	}
}

// TestDisplay_BarAfterStopDrawsNothing: a stopped display draws nothing
// again, whatever is added to it.
func TestDisplay_BarAfterStopDrawsNothing(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(80, c)
	d.Stop()
	before := buf.Len()
	bar := d.Bar(BarSpec{Label: "late", Total: 10})
	c.add(time.Second)
	bar.Set(5)
	bar.Done()
	if buf.Len() != before {
		t.Errorf("a stopped display wrote %q", buf.String()[before:])
	}
}

// stallTimer stands in for the display's stall timer: it records each
// timer the display sets, when it is due and what it does, so a test can
// move the clock to it and fire it.
type stallTimer struct {
	due  time.Duration
	fire func()
	set  int
}

func stallDisplay(t *testing.T, c *clock) (*Display, *bytes.Buffer, *stallTimer) {
	t.Helper()
	d, buf := liveDisplay(80, c)
	st := &stallTimer{}
	d.after = func(dur time.Duration, f func()) *time.Timer {
		st.due, st.fire = dur, f
		st.set++
		return time.NewTimer(time.Hour)
	}
	t.Cleanup(d.Stop)
	return d, buf, st
}

// stall moves the clock to the timer set last and fires it, and fails if
// that redraw set another: were it set again, its deadline long past, each
// redraw of a bar stalled would set another a millisecond on, and the
// display would redraw for ever. It returns the line drawn.
func (st *stallTimer) stall(t *testing.T, c *clock, buf *bytes.Buffer) string {
	t.Helper()
	if st.fire == nil {
		t.Fatal("no redraw was set for a bar showing a rate or an estimate")
	}
	c.add(st.due)
	fire := st.fire
	st.fire = nil
	fire()
	if st.fire != nil {
		t.Error("the stall's redraw set the timer again")
	}
	return replay(buf.String()).text()[0]
}

// A display whose only bar has stopped is redrawn when the bar has gone
// stallAfter without moving, and loses its rate and its estimate: nothing
// else redraws it, so a hung job's last speed once stood on the screen for
// ever. The redraw is the display's own timer, here a test's.
func TestBar_StalledOnlyBarIsRedrawn(t *testing.T) {
	c := newClock()
	d, buf, st := stallDisplay(t, c)
	bar := d.Bar(BarSpec{Label: "x", Total: 100000})
	c.add(time.Second)
	bar.Set(300)
	if line := replay(buf.String()).text()[0]; !strings.Contains(line, "/s") || !strings.Contains(line, "left") {
		t.Fatalf("a moving bar shows no rate or estimate: %q", line)
	}
	if line := st.stall(t, c, buf); strings.Contains(line, "/s") || strings.Contains(line, "left") {
		t.Errorf("stalled for %v, the only bar still reads %q", st.due, line)
	}
}

// A bar moving again after a stall shows its estimate, the run's average,
// and no rate until a window has run; a bar that then hangs is redrawn
// without the estimate all the same. The timer was once set only for a bar
// showing a rate, so the estimate stood on the screen for ever.
func TestBar_StalledEstimateIsRedrawnWithoutARate(t *testing.T) {
	c := newClock()
	d, buf, st := stallDisplay(t, c)
	bar := d.Bar(BarSpec{Label: "x", Total: 100000})
	c.add(time.Second)
	bar.Set(300)
	st.stall(t, c, buf)
	c.add(time.Second)
	bar.Set(400)
	if line := replay(buf.String()).text()[0]; strings.Contains(line, "/s") || !strings.Contains(line, "left") {
		t.Fatalf("moving again after a stall: %q, want an estimate and no rate", line)
	}
	if line := st.stall(t, c, buf); strings.Contains(line, "left") {
		t.Errorf("stalled for %v, the only bar still reads %q", st.due, line)
	}
}

// A bar of no known total shows a rate and no estimate, and is redrawn
// without the rate once it stalls, as a bar with both is.
func TestBar_StalledUnsizedBarIsRedrawn(t *testing.T) {
	c := newClock()
	d, buf, st := stallDisplay(t, c)
	bar := d.Bar(BarSpec{Label: "x"})
	c.add(time.Second)
	bar.Set(300)
	if line := replay(buf.String()).text()[0]; !strings.Contains(line, "/s") || strings.Contains(line, "left") {
		t.Fatalf("an unsized bar moving: %q, want a rate and no estimate", line)
	}
	if line := st.stall(t, c, buf); strings.Contains(line, "/s") {
		t.Errorf("stalled for %v, the only bar still reads %q", st.due, line)
	}
}

// A bar that moves between redraws and then hangs loses its rate
// stallAfter after it last moved, not after the stall timer's redraw first
// sees the move: judged at a redraw, the move was taken as happening then,
// and the rate stood up to twice stallAfter.
func TestBar_StallCountsFromTheLastSet(t *testing.T) {
	c := newClock()
	d, buf, st := stallDisplay(t, c)
	bar := d.Bar(BarSpec{Label: "x", Total: 100000})
	c.add(time.Second)
	bar.Set(300)
	c.add(500 * time.Millisecond)
	bar.Set(400) // inside the interval: not drawn
	// The timer was set at the first Set, half a second ago, so it fires
	// early, finds the bar moving and is set again for the rest.
	c.add(st.due - 500*time.Millisecond)
	fire := st.fire
	st.fire = nil
	fire()
	if line := replay(buf.String()).text()[0]; !strings.Contains(line, "/s") {
		t.Fatalf("%v after it last moved, the bar reads %q, want a rate still", stallAfter-500*time.Millisecond, line)
	}
	if st.due > time.Second {
		t.Errorf("set again for %v, want the half second left of stallAfter since the last Set", st.due)
	}
	if line := st.stall(t, c, buf); strings.Contains(line, "/s") {
		t.Errorf("stalled, the bar reads %q", line)
	}
}

// A bar moving again after a stall shows no rate until a window has run
// from where it started again, then the rate since, the step that ended
// the stall left out: measured from the stall, the first was the work of
// an instant over the whole stall, 7/s for a job running at a hundred, and
// with the step counted, 200/s.
func TestBar_RateAfterAStallIsMeasuredFromTheResume(t *testing.T) {
	c := newClock()
	d, buf, st := stallDisplay(t, c)
	bar := d.Bar(BarSpec{Label: "x"})
	c.add(time.Second)
	bar.Set(300)
	st.stall(t, c, buf)
	c.add(8 * time.Second)
	bar.Set(400)
	if line := replay(buf.String()).text()[0]; strings.Contains(line, "/s") {
		t.Errorf("moving again after a stall: %q, want no rate yet", line)
	}
	c.add(time.Second)
	bar.Set(500)
	if want, line := formatRate(100), replay(buf.String()).text()[0]; !strings.Contains(line, want) {
		t.Errorf("a second after moving again: %q, want %q", line, want)
	}
	// And trailing from there, not averaged from the resume: taken again
	// at every redraw, the resume made the rate an average since it.
	c.add(time.Second)
	bar.Set(800)
	if want, line := formatRate(300), replay(buf.String()).text()[0]; !strings.Contains(line, want) {
		t.Errorf("a second at 300 after one at 100: %q, want %q", line, want)
	}
}

// Moving again after a stall shows no rate even where no redraw came
// during it -- a timer firing late, say -- rather than the speed before the
// stall, which the redraw that moving again makes held.
func TestBar_NoRateHeldOverAStallNoRedrawSaw(t *testing.T) {
	c := newClock()
	d, buf, _ := stallDisplay(t, c)
	bar := d.Bar(BarSpec{Label: "x"})
	c.add(time.Second)
	bar.Set(300)
	c.add(10 * time.Second) // the timer never fires
	bar.Set(400)
	if line := replay(buf.String()).text()[0]; strings.Contains(line, "/s") {
		t.Errorf("moving again after a stall no redraw saw: %q, want no rate yet", line)
	}
}

// Workers whose Sets arrive out of order, the count going back a little
// several times a second, keep the rate on the screen: noted as resets,
// each restarted the rate window, and the rate never showed.
func TestBar_OutOfOrderSetsKeepTheRate(t *testing.T) {
	c := newClock()
	d, buf, _ := stallDisplay(t, c)
	bar := d.Bar(BarSpec{Label: "x"})
	for n := int64(10); n <= 300; n += 10 {
		c.add(100 * time.Millisecond)
		bar.Set(n + 5)
		bar.Set(n)
	}
	if line := replay(buf.String()).text()[0]; !strings.Contains(line, "/s") {
		t.Errorf("three seconds of Sets a little out of order: %q, want a rate", line)
	}
}

// A bar moving again after a stall at the same reading of the clock as a
// redraw -- another bar's -- is taken as moving again all the same, and its
// rate measured from the count it moved to: taken only where later than
// the window's start, it was missed, and the step that ended the stall was
// counted.
func TestBar_ResumeAtTheReadingOfARedraw(t *testing.T) {
	c := newClock()
	d, buf, st := stallDisplay(t, c)
	a := d.Bar(BarSpec{Label: "a"})
	b := d.Bar(BarSpec{Label: "b"})
	c.add(time.Second)
	a.Set(300)
	st.stall(t, c, buf) // a forced redraw: the next is due a second on
	c.add(2 * time.Second)
	b.Set(1) // a redraw, closing a's window at this reading
	a.Set(400)
	c.add(time.Second)
	a.Set(500)
	if want, line := formatRate(100), replay(buf.String()).text()[0]; !strings.Contains(line, want) {
		t.Errorf("100 done in the second since moving again: %q, want %q", line, want)
	}
}

// A Set after Stop -- a worker still running as the program shuts down --
// returns without the display's mutex, as one between redraws does: the
// deadline Stop left was passed, and each took the lock to find the
// display stopped.
func TestBar_SetAfterStopTakesNoLock(t *testing.T) {
	c := newClock()
	d, _ := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "x", Total: 10})
	d.Stop()
	c.add(time.Hour)
	d.mu.Lock()
	defer d.mu.Unlock()
	done := make(chan struct{})
	go func() {
		bar.Set(5)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a Set after Stop waited for the display's mutex")
	}
}

// When the bar last moved only ever moves forward: of two Sets racing,
// the one that read the clock first may note its move last, and a worker
// paused for seconds between the two set it seconds back, so that a bar
// moving read as stalled and its next Set as moving again after a stall.
func TestBar_MovedNeverGoesBack(t *testing.T) {
	c := newClock()
	d, _ := liveDisplay(80, c)
	bar := d.Bar(BarSpec{Label: "x"})
	early, late := c.now().Add(time.Second), c.now().Add(7*time.Second)
	bar.moveFrom(late, 1, 3)
	bar.moveFrom(early, 0, 2) // read the clock first, noted last
	if got := bar.movedAt(); !got.Equal(late) {
		t.Errorf("moved at %v after a move at %v noted late, want %v", got.Sub(c.now()), early.Sub(c.now()), late.Sub(c.now()))
	}
	if at := bar.resumedAt.Load(); at != int64(7*time.Second) {
		t.Errorf("resumed at %v, want the one move a stall after the start, at 7s", time.Duration(at))
	}
}

// While the bars move, the stall timer is set once a stallAfter, not again
// at every redraw; and the last bar's Done stops it.
func TestBar_StallTimerIsKeptWhileMoving(t *testing.T) {
	c := newClock()
	d, _, st := stallDisplay(t, c)
	bar := d.Bar(BarSpec{Label: "x", Total: 100000})
	for i := range 40 {
		c.add(100 * time.Millisecond)
		bar.Set(int64(i+1) * 10)
	}
	if st.set != 1 {
		t.Errorf("the timer was set %d times in %v of moving, want once", st.set, 40*100*time.Millisecond)
	}
	bar.Done()
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stall != nil {
		t.Error("the last bar is done and the stall timer is still set")
	}
}

// On a plain display a bar finishing writes its own closing line and no
// other bar's: the others keep to their interval. Done once wrote a line
// for every bar still running, a "0%" in the first second among them.
func TestPlain_DoneWritesOnlyItsOwnLine(t *testing.T) {
	c := newClock()
	var buf bytes.Buffer
	d := New(&buf, Options{Mode: Plain, Now: c.now, Interval: time.Second})
	a := d.Bar(BarSpec{Label: "a", Total: 10})
	d.Bar(BarSpec{Label: "b", Total: 10})
	c.add(100 * time.Millisecond)
	a.Set(10)
	a.Done()
	if strings.Contains(buf.String(), "b ") {
		t.Errorf("a finishing wrote b's line: %q", buf.String())
	}
}

// The smaller formatting edges: whole bytes round into kibibytes as tenths
// do; a rate too slow for a tenth is "<0.1/s", not a "0.0/s" that reads as
// stopped; an estimate past a year is none; and an ellipsis wider than the
// line is left out.
func TestFormattingEdges(t *testing.T) {
	if got := formatByteRate(1023.6); got != "1.0 KiB/s" {
		t.Errorf("formatByteRate(1023.6) = %q", got)
	}
	if got := formatRate(1.0 / 30); got != "<0.1/s" {
		t.Errorf("formatRate(1/30) = %q", got)
	}
	b := &Bar{start: time.Unix(0, 0)}
	b.moved.Store(int64(10 * time.Second))
	if got := b.eta(time.Unix(10, 0), 1, 1<<62); got != "" {
		t.Errorf("an estimate of centuries: %q", got)
	}
	wide := func(s string) int { return 2 * len([]rune(s)) }
	if got := truncate("abc", 1, wide); got != "" {
		t.Errorf("a two-column ellipsis in one column: %q", got)
	}
}

// With no room for a trough the fields keep all the room: a bar with room
// for its count beside its percentage shows both.
func TestLayout_FieldsRefitWithoutATrough(t *testing.T) {
	c := newClock()
	d, buf := liveDisplay(17, c)
	bar := d.Bar(BarSpec{Label: "x", Total: 100})
	c.add(time.Second)
	bar.Set(50)
	if line := replay(buf.String()).text()[0]; !strings.Contains(line, "50/100") {
		t.Errorf("at 17 columns: %q, want the count too", line)
	}
}

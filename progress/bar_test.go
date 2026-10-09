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
	d, buf := liveDisplay(80, c)
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

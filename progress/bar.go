package progress

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// BarSpec describes one bar. Only Label is really needed; a Total of zero or
// less means the size of the job is not known, which is a real and common
// case rather than an error.
type BarSpec struct {
	// Label names the work, e.g. "rendering". It is shown first and is the
	// last thing dropped when the terminal is narrow.
	Label string

	// Total is how many units the job will take. Zero or less means unknown:
	// the bar then reports what has been done and how fast, and shows no
	// percentage and no estimate rather than inventing either.
	Total int64

	// Unit names what is being counted, e.g. "frames". Empty prints the
	// counts bare.
	Unit string
}

// Bar is one line of a Display.
//
// A nil *Bar is usable and does nothing, so a caller holding one from a
// disabled Display needs no nil check on its hot path.
type Bar struct {
	d     *Display
	label string
	unit  string

	done  atomic.Int64
	total atomic.Int64

	start time.Time

	// rate is the trailing-window measurement: every rate shown covers only
	// the span since the previous redraw, not the whole run. A cumulative
	// average takes minutes to reflect a slowdown, which is exactly when
	// somebody is watching the number.
	//
	// Guarded by the Display's mutex, since only render touches them.
	lastTime time.Time
	lastDone int64
	lastRate float64
}

func newBar(d *Display, spec BarSpec) *Bar {
	b := &Bar{d: d, label: spec.Label, unit: spec.Unit, start: d.now()}
	b.total.Store(spec.Total)
	b.lastTime = b.start
	return b
}

// Set records that done units are complete and redraws if the display is due.
//
// This is called once per unit of work -- per frame, in a render loop -- so
// the path that does not redraw is an atomic store, a clock read and a
// comparison. No allocation, no formatting, no lock.
func (b *Bar) Set(done int64) {
	if b == nil {
		return
	}
	b.done.Store(done)
	b.d.redraw(false)
}

// Add records that n more units are complete.
func (b *Bar) Add(n int64) {
	if b == nil {
		return
	}
	b.done.Add(n)
	b.d.redraw(false)
}

// SetTotal revises how many units the job will take, for a caller that learns
// the size after starting. A total of zero or less returns the bar to
// reporting an unknown size.
func (b *Bar) SetTotal(total int64) {
	if b == nil {
		return
	}
	b.total.Store(total)
	b.d.redraw(false)
}

// Done removes this bar from the display.
//
// The line is removed rather than frozen at 100%, because a finished bar is
// no longer live and the live region is for what is still running. A caller
// that wants a permanent record of the finished job writes one -- to the
// Display, so it lands above the bars that are still going.
func (b *Bar) Done() {
	if b == nil {
		return
	}
	d := b.d
	d.mu.Lock()
	for i, other := range d.bars {
		if other == b {
			d.bars = append(d.bars[:i], d.bars[i+1:]...)
			break
		}
	}
	// Erase before releasing the lock: the region just lost a line, and
	// leaving the old one on screen until the next redraw would show a
	// finished bar sitting under a live one.
	d.erase()
	d.mu.Unlock()
	d.redraw(true)
}

// glyphs for the bar's filled portion, at eighth-of-a-cell resolution.
// Index 0 is empty and index 8 is a full block, so a fill of n eighths is
// barGlyphs[n].
var barGlyphs = [...]rune{' ', '▏', '▎', '▍', '▌', '▋', '▊', '▉', '█'}

const (
	// minBarWidth is the narrowest bar worth drawing. Below this the glyphs
	// say less than the percentage they sit beside, so the bar is dropped
	// and the text keeps the room.
	minBarWidth = 6
	// barOpen and barClose bracket the trough so its extent is visible when
	// it is nearly empty.
	barOpen  = '▕'
	barClose = '▏'
)

// fields returns this bar's label and its right-hand text fields, most- to
// least- important. It advances the rate window as a side effect, so it is
// called exactly once per redraw. The caller holds the Display's mutex.
// pad widens the percentage to a fixed three digits, which keeps a live bar's
// right-hand fields from shuffling sideways as it crosses 10% and 100%. A
// plain line is not aligned against anything and takes the number bare.
func (b *Bar) fields(now time.Time, pad bool) (label string, fields []string) {
	done, total := b.done.Load(), b.total.Load()
	b.observe(now, done)

	if total > 0 {
		if pad {
			fields = append(fields, fmt.Sprintf("%3d%%", percent(done, total)))
		} else {
			fields = append(fields, fmt.Sprintf("%d%%", percent(done, total)))
		}
	}
	fields = append(fields, b.counts(done, total))
	if r := formatRate(b.lastRate); r != "" {
		fields = append(fields, r)
	}
	if eta := b.eta(now, done, total); eta != "" {
		fields = append(fields, eta)
	}
	return b.label, fields
}

// hasBar reports whether this bar has a trough to draw at all.
//
// A job whose size is unknown does not get an empty one. A full-width trough
// that never fills is a progress indicator indicating nothing -- it looks
// like a bar that is stuck rather than like a measurement nobody has. The
// count and the rate are the honest report of an unsized job, and they are
// moving, which is the same thing a viewer reads a bar for.
func (b *Bar) hasBar() bool { return b != nil && b.total.Load() > 0 }

// layout is the shared geometry one redraw gives every bar on the display.
type layout struct {
	labelWidth int // every label padded to this, so the troughs line up
	barWidth   int // 0 when there is no room for a trough at all
	fields     int // how many of each bar's fields survived the width
	cols       int
	ascii      bool
}

// composeLine renders one bar against the shared layout.
func (b *Bar) composeLine(label string, fields []string, l layout, measure func(string) int) string {
	var sb strings.Builder
	if l.labelWidth > 0 {
		sb.WriteString(label)
		sb.WriteString(strings.Repeat(" ", l.labelWidth-measure(label)))
		sb.WriteString("  ")
	}
	if l.barWidth > 0 {
		// A bar with no trough still takes the room, so a sized bar beside
		// an unsized one does not slide sideways as the pair redraw.
		if b.hasBar() {
			sb.WriteString(b.glyphBar(b.done.Load(), b.total.Load(), l.barWidth, l.ascii))
		} else {
			sb.WriteString(strings.Repeat(" ", l.barWidth+2))
		}
		sb.WriteString("  ")
	}
	if n := l.fields; n < len(fields) {
		fields = fields[:n]
	}
	sb.WriteString(strings.Join(fields, "  "))

	line := sb.String()
	if measure(line) > l.cols {
		return truncate(line, l.cols, measure)
	}
	return line
}

// renderPlain builds one line for a non-terminal display: the same
// information with no bar and no escape sequences.
//
// It is deliberately close to what both of this package's intended consumers
// already wrote to their logs, so adopting this does not silently change the
// shape of anybody's log file.
func (b *Bar) renderPlain(now time.Time) string {
	label, fields := b.fields(now, false)
	if label != "" {
		fields = append([]string{label}, fields...)
	}
	return strings.Join(fields, " ")
}

// minRateWindow is the shortest span a displayed rate may be measured over.
//
// It is deliberately much longer than the redraw interval, and that gap is
// the point. Redrawing twelve times a second is what makes a bar look
// continuous, but a rate measured over 80ms is dominated by whatever the
// scheduler was doing in those 80ms: it swings by an order of magnitude
// between redraws and reads as broken. Measuring over half a second and
// holding the previous figure in between gives a number that moves when the
// work's speed actually moves.
const minRateWindow = 500 * time.Millisecond

// observe advances the trailing rate window. The caller holds the Display's
// mutex, which is what makes the unsynchronised fields safe.
//
// A redraw inside the current window leaves the previous rate in place rather
// than recomputing it over a sliver of time -- see minRateWindow.
func (b *Bar) observe(now time.Time, done int64) {
	elapsed := now.Sub(b.lastTime)
	if elapsed < minRateWindow {
		return
	}
	if done > b.lastDone {
		b.lastRate = float64(done-b.lastDone) / elapsed.Seconds()
	} else if done < b.lastDone {
		// Went backwards: a caller reset the count. Start the window again
		// rather than reporting a negative rate.
		b.lastRate = 0
	}
	b.lastTime, b.lastDone = now, done
}

func (b *Bar) counts(done, total int64) string {
	unit := b.unit
	if unit != "" {
		unit = " " + unit
	}
	if total > 0 {
		return fmt.Sprintf("%d/%d%s", done, total, unit)
	}
	return fmt.Sprintf("%d%s", done, unit)
}

// eta estimates the time remaining, from the CUMULATIVE average rather than
// from the trailing rate the line displays beside it.
//
// The two answer different questions and the best estimator differs. The rate
// answers "how fast is it going now", where a trailing window is the honest
// measure. The estimate answers "when will this finish", where a cumulative
// average is steadier: a trailing window makes the estimate lurch by minutes
// on every hiccup, and an estimate that will not sit still is one nobody can
// use.
//
// It says nothing until a second has passed and something has been done. An
// estimate extrapolated from two frames is noise presented as information.
func (b *Bar) eta(now time.Time, done, total int64) string {
	if total <= 0 || done <= 0 || done >= total {
		return ""
	}
	elapsed := now.Sub(b.start)
	if elapsed < time.Second {
		return ""
	}
	left := time.Duration(float64(elapsed) / float64(done) * float64(total-done))
	if left < time.Second {
		// "~0s left" is noise: at this point the thing is finishing, and a
		// figure that rounds away to nothing tells a reader less than the
		// percentage already beside it.
		return ""
	}
	return "~" + formatDuration(left) + " left"
}

// glyphBar draws the trough and its fill at eighth-of-a-cell resolution,
// occupying exactly width+2 terminal columns: the two brackets and the width
// between them.
//
// A total that is not known gets a full-width trough with no fill: it is a
// place-holder for a measurement nobody has, and animating something through
// it would suggest a position that is not being reported.
func (b *Bar) glyphBar(done, total int64, width int, ascii bool) string {
	var sb strings.Builder
	if ascii {
		sb.WriteByte('[')
	} else {
		sb.WriteRune(barOpen)
	}
	if total > 0 {
		frac := float64(done) / float64(total)
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
		if ascii {
			full := int(frac * float64(width))
			sb.WriteString(strings.Repeat("=", full))
			sb.WriteString(strings.Repeat(" ", width-full))
		} else {
			eighths := int(frac * float64(width) * 8)
			full := eighths / 8
			sb.WriteString(strings.Repeat(string(barGlyphs[8]), full))
			rest := width - full
			if rest > 0 {
				if part := eighths % 8; part > 0 {
					sb.WriteRune(barGlyphs[part])
					rest--
				}
				sb.WriteString(strings.Repeat(" ", rest))
			}
		}
	} else {
		sb.WriteString(strings.Repeat(" ", width))
	}
	if ascii {
		sb.WriteByte(']')
	} else {
		sb.WriteRune(barClose)
	}
	// No trailing separator: composeLine owns the gap after the trough, and
	// resolveLayout's width arithmetic counts it exactly once. Adding it here
	// too put every line two columns over the terminal's width, which showed
	// up as the right-hand field being truncated on a line that had room for
	// it.
	return sb.String()
}

func percent(done, total int64) int {
	if total <= 0 {
		return 0
	}
	p := done * 100 / total
	switch {
	case p < 0:
		return 0
	case p > 100:
		return 100
	}
	return int(p)
}

// formatRate renders a rate per second, or "" when there is not yet one worth
// showing. Precision falls away as the number grows, because the third
// significant digit of a rate is never the point.
func formatRate(perSecond float64) string {
	switch {
	case perSecond <= 0:
		return ""
	case perSecond < 10:
		return fmt.Sprintf("%.1f/s", perSecond)
	default:
		return fmt.Sprintf("%.0f/s", perSecond)
	}
}

// formatDuration renders a duration compactly and without false precision:
// seconds below a minute, minutes and seconds below an hour, hours and
// minutes above it.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		d = d.Round(time.Second)
		return fmt.Sprintf("%dm%02ds", int(d/time.Minute), int((d%time.Minute)/time.Second))
	default:
		d = d.Round(time.Minute)
		return fmt.Sprintf("%dh%02dm", int(d/time.Hour), int((d%time.Hour)/time.Minute))
	}
}

// truncate cuts s to at most cols terminal columns, marking the cut with a
// horizontal ellipsis so a reader can tell a shortened line from a complete
// one.
//
// It walks runes and accumulates measured width rather than slicing bytes:
// slicing splits multi-byte runes, and counting runes gets wide characters
// wrong. Both failures are invisible until somebody's label is not ASCII.
func truncate(s string, cols int, measure func(string) int) string {
	if cols <= 0 {
		return ""
	}
	if measure(s) <= cols {
		return s
	}
	const ellipsis = "…"
	budget := cols - measure(ellipsis)
	if budget <= 0 {
		return ellipsis
	}
	var sb strings.Builder
	used := 0
	for _, r := range s {
		w := measure(string(r))
		if used+w > budget {
			break
		}
		sb.WriteRune(r)
		used += w
	}
	return sb.String() + ellipsis
}

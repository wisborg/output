package progress

import (
	"fmt"
	"io"
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

	// Bytes says the counts are bytes, written as bytes are read: in
	// KiB, MiB or GiB, with the rate in the same, rather than as a count of
	// units. Unit is not printed. A download of 945 MiB counted as
	// 990904320 B is a number nobody reads at a glance, and its rate --
	// "3412k/s" -- does not say what it is a rate of.
	Bytes bool
}

// The right-hand fields, in display order. They are SLOTS rather than a list
// that grows and shrinks, because the width of each one is reserved up front
// and held for the whole run -- see reserveSlots.
const (
	slotPercent = iota
	slotCounts
	slotRate
	slotETA
	numSlots
)

// Bar is one line of a Display.
//
// A nil *Bar is usable and does nothing, so a caller holding one from a
// disabled Display needs no nil check on its hot path.
type Bar struct {
	d     *Display
	label string
	unit  string
	bytes bool

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

	// reserve is the column width held for each slot, whether or not that
	// slot has anything to show yet. It is what keeps the trough from
	// resizing: without it the bar is re-fitted around whatever the fields
	// happen to measure this instant, so it shrinks a little every time the
	// counts gain a digit, and lurches when the rate and the estimate first
	// appear a second into the run. A width may only ever GROW (see widen),
	// so the trough can never oscillate.
	//
	// Guarded by the Display's mutex, like the rate window above it.
	reserve [numSlots]int

	// plainDone is how far the bar had got when a plain display last wrote
	// a line for it, and -1 until one has. See Done.
	plainDone int64
}

func newBar(d *Display, spec BarSpec) *Bar {
	b := &Bar{d: d, label: spec.Label, unit: spec.Unit, bytes: spec.Bytes, start: d.now(), plainDone: -1}
	b.total.Store(spec.Total)
	b.lastTime = b.start
	b.reserveSlots()
	return b
}

// reserveSlots estimates, before any work has been reported, how wide each
// field will ever get. It only ever widens a slot, so it is safe to re-run
// when the job's shape changes under SetTotal.
//
// Two of the four can be known exactly. A percentage is always three digits
// and a sign, and the counts are widest when the job is finished -- at which
// point "done" has exactly as many digits as the total, which is known now.
//
// The other two cannot be, and their estimates matter MORE rather than less,
// because they are the fields that are not there yet: the rate needs a
// measurement window and the estimate needs a second of work, so both appear
// part-way through a run that has already settled. Reserving their room from
// the start is what stops the trough jumping when they arrive. The figures
// are derived from the formatters rather than written as constants, so they
// cannot drift away from what those actually produce, and widen covers
// whatever exceeds them.
func (b *Bar) reserveSlots() {
	measure := b.d.widthFunc()
	total := b.total.Load()

	if total > 0 {
		b.widen(slotPercent, measure("100%"))
		b.widen(slotCounts, measure(b.counts(total, total)))
		// A duration long enough to need every part it can print. An
		// estimate wider than this is possible and simply widens the slot
		// once; one narrower is the ordinary case and costs a few columns
		// of trough.
		b.widen(slotETA, measure(formatETA(59*time.Minute+59*time.Second)))
	} else {
		// No total means no percentage and no estimate, ever -- neither can
		// be computed without one -- so they take no room at all. If a total
		// arrives later (SetTotal), the slots open then.
		b.widen(slotCounts, measure(b.counts(0, 0)))
	}
	// The widest a rate prints below ten million a second -- see formatRate,
	// which abbreviates precisely so this has an answer; for bytes, the
	// widest any prefix prints.
	if b.bytes {
		b.widen(slotRate, measure(formatByteRate(1023.9*1024*1024)))
	} else {
		b.widen(slotRate, measure(formatRate(9_999_000)))
	}
}

// widen grows a slot to fit a value that outran its reservation, and never
// shrinks one. Growing is a one-off jump; shrinking would let the trough
// oscillate for the rest of the run, which is the thing this whole mechanism
// exists to prevent.
func (b *Bar) widen(slot int, w int) {
	if w > b.reserve[slot] {
		b.reserve[slot] = w
	}
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
	// A total arriving late brings a percentage and an estimate with it, and
	// widens the counts to their finished size. Reserving that room now costs
	// the trough one adjustment here rather than one when each field first
	// appears -- and the reservations only grow, so the trough cannot go back
	// and forth afterwards. The lock is what reserve is guarded by.
	b.d.mu.Lock()
	b.reserveSlots()
	b.d.mu.Unlock()
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
	// A plain display's lines stay in the log, so the last one a bar wrote
	// is what the log says it reached -- and that is a periodic snapshot,
	// taken up to an interval before the end. Without this, a bar that
	// finished leaves its log reading 99%. One closing line, then, but only
	// for a bar that has written a line before and has moved since: a job
	// that finished inside one interval still says nothing, which is the
	// rule for plain lines, and one whose last line was already its last
	// state does not say it twice.
	if !d.live && !d.stopped && b.plainDone >= 0 && b.done.Load() != b.plainDone {
		_, _ = io.WriteString(d.w, b.renderPlain(d.now())+"\n")
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
// slots fills this bar's four fields for the current instant, leaving a slot
// empty when it has nothing to say yet, and updates the reservations for
// anything that outgrew them.
//
// An empty slot still occupies its reserved width when the line is composed,
// which is the whole point: the rate and the estimate arrive a second into a
// run, and a layout that made room for them only once they existed would move
// everything to their left at that moment.
func (b *Bar) slots(now time.Time) (label string, out [numSlots]string) {
	done, total := b.done.Load(), b.total.Load()
	b.observe(now, done)
	measure := b.d.widthFunc()

	if total > 0 {
		// Three digits wide always, so the fields to its right do not shuffle
		// as it crosses 10% and 100%.
		out[slotPercent] = fmt.Sprintf("%3d%%", percent(done, total))
	}
	out[slotCounts] = b.counts(done, total)
	if b.bytes {
		out[slotRate] = formatByteRate(b.lastRate)
	} else {
		out[slotRate] = formatRate(b.lastRate)
	}
	out[slotETA] = b.eta(now, done, total)

	for i, v := range out {
		if v != "" {
			b.widen(i, measure(v))
		}
	}
	return b.label, out
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
	slots      int // how many field slots survived the width, from the left
	cols       int
	ascii      bool
}

// reservedWidth is the room this bar's fields occupy when the first n slots
// are shown: each at its reserved width, joined by two spaces, and slots this
// bar will never fill skipped entirely.
//
// The layout is sized from THIS rather than from what the fields currently
// measure, which is the whole mechanism. Sizing from the current values
// re-fits the trough around whatever the numbers happen to be this instant,
// so it shrinks a little as the counts gain a digit and jumps when the rate
// and the estimate appear.
func (b *Bar) reservedWidth(n int, measure func(string) int) int {
	total, shown := 0, 0
	for i := 0; i < numSlots && i < n; i++ {
		if b.reserve[i] == 0 {
			continue
		}
		total += b.reserve[i]
		shown++
	}
	if shown > 1 {
		total += 2 * (shown - 1)
	}
	return total
}

// composeLine renders one bar against the shared layout.
// composeFields lays the slots out at their reserved widths, joined by two
// spaces, skipping any slot this bar will never fill and any the layout has
// dropped for want of room.
//
// A value is padded on the LEFT, so the numbers line up at their right-hand
// edge as they grow -- the same reason a table right-aligns a numeric column.
func (b *Bar) composeFields(fields [numSlots]string, l layout, measure func(string) int) string {
	var parts []string
	for i := 0; i < numSlots && i < l.slots; i++ {
		w := b.reserve[i]
		if w == 0 {
			continue
		}
		v := fields[i]
		parts = append(parts, strings.Repeat(" ", w-measure(v))+v)
	}
	return strings.TrimRight(strings.Join(parts, "  "), " ")
}

// The composed line's width is ACCUMULATED as it is built rather than
// measured afterwards, and that is not an optimisation. Once the trough can
// carry colour, the assembled string holds escape sequences, and measuring it
// would count their digits as columns -- so the bar would be judged too wide,
// truncated, and cut off mid-sequence.
//
// The width of the coloured part is known without measuring it: a trough is
// always barWidth+2 columns whatever it is painted in, because the colouring
// step adds sequences and never characters.
func (b *Bar) composeLine(label string, fields [numSlots]string, l layout, measure func(string) int) string {
	var sb strings.Builder
	width := 0
	if l.labelWidth > 0 {
		sb.WriteString(label)
		sb.WriteString(strings.Repeat(" ", l.labelWidth-measure(label)))
		sb.WriteString("  ")
		width += l.labelWidth + 2
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
		width += l.barWidth + 4
	}
	text := b.composeFields(fields, l, measure)
	sb.WriteString(text)
	width += measure(text)

	line := sb.String()
	if width > l.cols {
		// Reachable only when there is no trough: resolveLayout sizes one to
		// leave the line exactly cols wide, and gives up on it entirely when
		// that cannot be done. No trough means no colour, so truncate never
		// has to cut an escape sequence in half -- which it has no way to do
		// safely.
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
	label, slots := b.slots(now)

	// Neither padded nor reserved. These lines accumulate in a log rather
	// than replacing each other, so nothing is aligned against anything and
	// a column of blanks held open for a field that is not there yet would
	// be noise in a file somebody greps.
	parts := make([]string, 0, numSlots+1)
	if label != "" {
		parts = append(parts, label)
	}
	for i, v := range slots {
		if v == "" {
			continue
		}
		if i == slotPercent {
			// The live line pads this to three digits to stop the fields
			// shuffling; a log line has no such problem and takes it bare.
			v = strings.TrimLeft(v, " ")
		}
		parts = append(parts, v)
	}
	return strings.Join(parts, " ")
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

// formatETA renders a remaining time. It is a function rather than an inline
// format so reserveSlots can measure exactly what eta will later produce.
func formatETA(left time.Duration) string {
	return "~" + formatDuration(left) + " left"
}

func (b *Bar) counts(done, total int64) string {
	if b.bytes {
		return formatBytes(done, total)
	}
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
	return formatETA(left)
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
		var filled []rune
		if ascii {
			full := int(frac * float64(width))
			filled = make([]rune, 0, full)
			for i := 0; i < full; i++ {
				filled = append(filled, '=')
			}
		} else {
			eighths := int(frac * float64(width) * 8)
			full := eighths / 8
			filled = make([]rune, 0, full+1)
			for i := 0; i < full; i++ {
				filled = append(filled, barGlyphs[8])
			}
			if part := eighths % 8; part > 0 && full < width {
				filled = append(filled, barGlyphs[part])
			}
		}
		sb.WriteString(b.paint(filled, width))
		sb.WriteString(strings.Repeat(" ", width-len(filled)))
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

// paint writes the filled cells with the palette applied, emitting a colour
// sequence only where the colour actually changes.
//
// Cell-by-cell sequences would be correct and wasteful: a fifty-cell
// gradient redrawn twelve times a second is a great many escape sequences,
// and adjacent cells of a gentle ramp usually round to the same colour --
// always, under the 256-colour palette. Coalescing them is what keeps the
// output a few hundred bytes a redraw rather than a few thousand.
//
// It returns the cells unchanged in monochrome, which is what makes
// monochrome the same code path as the other modes rather than a second
// renderer alongside them.
func (b *Bar) paint(filled []rune, width int) string {
	d := b.d
	if len(filled) == 0 {
		return ""
	}
	if d == nil || d.depth == depthNone || d.palette.Mode == Monochrome {
		return string(filled)
	}

	out := make([]byte, 0, len(filled)*4+32)
	var last RGB
	var started bool
	for i, r := range filled {
		c, ok := d.palette.at(i, width)
		if !ok {
			out = append(out, string(r)...)
			continue
		}
		if !started || c != last {
			out = d.depth.sgr(out, c)
			last, started = c, true
		}
		out = append(out, string(r)...)
	}
	if started {
		out = append(out, colourReset...)
	}
	return string(out)
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
// showing.
//
// Precision falls away as the number grows, because the third significant
// digit of a rate is never the point, and past ten thousand the number is
// abbreviated. That is not only for readability: an unabbreviated rate has no
// bound on its WIDTH, so the field would keep growing past whatever room was
// reserved for it and take a column off the trough each time -- the jitter
// this package reserves widths to avoid. Abbreviating bounds it at seven
// columns for any rate below ten million a second.
func formatRate(perSecond float64) string {
	switch {
	case perSecond <= 0:
		return ""
	case perSecond < 10:
		return fmt.Sprintf("%.1f/s", perSecond)
	case perSecond < 10_000:
		return fmt.Sprintf("%.0f/s", perSecond)
	case perSecond < 10_000_000:
		return fmt.Sprintf("%.0fk/s", perSecond/1000)
	default:
		return fmt.Sprintf("%.0fM/s", perSecond/1_000_000)
	}
}

// byteUnits are the binary prefixes bytes are written in, smallest first.
var byteUnits = []string{"B", "KiB", "MiB", "GiB", "TiB"}

// bytePrefix is the largest prefix n is at least one of, and its size.
func bytePrefix(n float64) (name string, size float64) {
	name, size = byteUnits[0], 1
	for _, u := range byteUnits[1:] {
		if n < size*1024 {
			break
		}
		name, size = u, size*1024
	}
	return name, size
}

// formatBytes renders a byte count, and with a total, as "done/total" both
// in the total's prefix: "297.5/944.1 MiB". One prefix for both, chosen from
// the total, so the field does not change its shape as done grows past a
// prefix boundary on its way there. Plain bytes are whole; anything larger
// has one decimal.
func formatBytes(done, total int64) string {
	ref := float64(total)
	if total <= 0 {
		ref = float64(done)
	}
	name, size := bytePrefix(ref)
	f := func(n int64) string {
		if size == 1 {
			return fmt.Sprintf("%d", n)
		}
		return fmt.Sprintf("%.1f", float64(n)/size)
	}
	if total > 0 {
		return f(done) + "/" + f(total) + " " + name
	}
	return f(done) + " " + name
}

// formatByteRate renders a rate of bytes a second in its own prefix,
// "3.4 MiB/s", or "" when there is not yet one worth showing.
func formatByteRate(perSecond float64) string {
	if perSecond <= 0 {
		return ""
	}
	name, size := bytePrefix(perSecond)
	if size == 1 {
		return fmt.Sprintf("%.0f B/s", perSecond)
	}
	return fmt.Sprintf("%.1f %s/s", perSecond/size, name)
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

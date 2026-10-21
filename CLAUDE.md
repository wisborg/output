# output

Go packages for producing program output in more than one shape. The root
package `output` writes one result as text, CSV, JSON or YAML, chosen when
the program runs; `table` is the tabular half of that — data rendered as
aligned text or written as CSV; `progress` is the live half — bars pinned to
the bottom of a terminal while ordinary output scrolls above them; `chart`
draws series of readings, in braille or — through `chart/picture` — as an
image, which `chart/terminal` shows in a terminal that can show one.

## Building and testing

Pure Go, no cgo, no external tools. `make` lists the targets; each is a plain
`go` invocation, as in osmbase, rather than a delegation to a script, since
there is nothing here a script would add:

```
make gates                 # gofmt + vet + test + deps, one status line each
make test PKG=./chart/ RUN=Date
make race                  # the tests as CI runs them
make deps                  # each package compiles in only what it is meant to
make build                 # -> bin/plot, bin/diagram
make run ARGS="--list data.csv"     # CMD=diagram for the other
make clean                 # remove bin/, empty .scratch/
```

`make deps` is how the layering under Layout below is kept rather than
merely described, and `make gates` and CI both run it. Each group of
packages is checked against an allowlist of the modules it may compile in,
so any new dependency fails until the target names it, and the packages the
layering keeps apart are checked by import too. It once checked each group
against a few modules it must not have, and let anything else through:
x/text into `table`, or `table` into `progress`, passed. It fails if
`chart/terminal` compiles in
anything outside the standard library, `chart` or the diagram packages anything but
go-runewidth and uax29, `table` or `progress` YAML or x/image,
or the root package x/image. A new dependency in any package is a visible
edit to that target.

`go test ./...` runs the `Example` functions too, so the documentation is
verified rather than merely present. An example whose `// Output:` block drifts
from reality is a test failure.

## Layout

- `output.go` — the package doc, `Document`, `Document.Write` and the three
  error sentinels. This is the only file that imports `table`.
- `format.go` — `Format`, its names and aliases, `ParseFormat`, `Formats`,
  and the `flag.Value` and `encoding.TextMarshaler` methods. No dependency on
  `table` or on either encoder.
- `json.go` — `JSONStyle` and `WriteJSON`, over `encoding/json`.
- `yaml.go` — `YAMLStyle` and `WriteYAML`, over `go.yaml.in/yaml/v3`. The
  only file that imports the YAML library, and no type from it appears in
  this package's exported API, so replacing the library is confined here.
- `table/table.go` — `Table`, `Column`, `Align`, and building operations.
- `table/render.go` — text rendering: `Style`, `Render`, `String`, and the
  unexported `layout` that resolves a table for one style.
- `table/csv.go` — `CSVStyle` and `WriteCSV`, over the same rows.
- `progress/progress.go` — the package doc, `Display`, `Options`, `Mode`, and
  the redraw/erase/layout machinery.
- `progress/bar.go` — `Bar`, `BarSpec`, and the rendering of one line: the
  trough, the counts, the rate and the estimate.
- `progress/term_unix.go`, `term_windows.go`, `term_other.go` — build-tagged
  terminal detection and size.

- `chart/series.go` — the package doc, `Point`, `Segments`, `Smooth`.
- `chart/bin.go` — `Column` and `Bin`: what each column of a drawing shows.
- `chart/scale.go` — `RobustRange`, `Scale`, `NewScale`, `ExactScale`, ticks
  and labels.
- `chart/braille.go` — `Chart`, `Strip`, and the braille rendering.
- `chart/date.go` — the `Date` kind's steps, calendar ticks and labels.
- `chart/band.go` — `Band`, `BandKind`, and their braille rows.
- `chart/picture/picture.go` — `Draw`, `Palette`, `Options`, `TextPx`.
- `internal/canvas/` — the few drawing primitives a picture needs, over
  `golang.org/x/image/vector` and Go Mono: shared by `chart/picture` and
  `diagram/picture`, and internal so that it is not an API anyone else
  depends on.
- `examples/` — DOT files a reader can copy, each drawn as text, PNG and
  SVG by `cmd/diagram`'s `TestExamples`, so one that stops reading fails the
  build rather than the reader.
- `internal/cli/` — what the commands share in reading their command line:
  `Parse` takes flags before or after the file and returns a mistake as a
  `UsageError` without printing, `Exit` reports it and exits 2 (0 for
  help, which the command prints on standard output, 1 for anything
  else), and `PrintDefaults` lists the flags as `--out string`, as fitdash
  does. The standard `flag` package reads
  `-out` and `--out` alike; pflag would list them the same but read `-out`
  as `-o -u -t`, breaking command lines already written.
- `chart/terminal/terminal.go` — `Choose`, `Write`, the iTerm2 and kitty
  writers, `LightBackground`; `size_*.go`, build-tagged, `SizeOf`.
- `cmd/plot/` — the `plot` command: `read.go` reads CSV into a table of text,
  `columns.go` decides what each column holds and reads its values,
  `chart.go` turns the columns into a `chart.Chart`, `main.go` is the flags
  and the drawing. A new input format is a new reader of the same table.
- `diagram/diagram.go` — the package doc, the model (`Diagram`, `Group`,
  `Node`, `Edge`), `Palette`; `layout.go` — `Layout`, grids and layers;
  `route.go` — the search for edges' paths along rows and columns; `straight.go` — which edges a
  picture draws straight, and their labels; `labels.go` — where labels go;
  `placement.go` — the costs and the local search that choose orthogonal
  edges' label places together;
  `scale.go` — cells to a picture's units; `text.go` — the text drawing;
  `width.go` — `Width`, the one measure of a label in cells.
- `diagram/dot/` — the DOT lexer, parser and colours.
- `diagram/picture/`, `diagram/svg/` — the PNG and the SVG of a layout.
- `cmd/diagram/` — the `diagram` command.

`progress` imports neither the root package nor `table`, the same way `table`
imports neither of the others. `chart/terminal` imports only the standard
library. `chart` imports go-runewidth and uax29 to cut a title, a strip's
name and a band kind's to the width in columns, between grapheme clusters,
as `table` cuts a cell: cut by runes, a CJK title came out twice the width.
`plot` linked both already, through `table`, so the measure became the one
`table` and `progress` use rather than a second, standard-library one that
could disagree with it. `diagram`, `diagram/dot` and `diagram/svg` import
go-runewidth and the grapheme clusters beneath it, and nothing else: a label
is measured in a terminal's columns, as a table's cell is, because counting
its runes overflowed a CJK label's box and pushed the boxes after it out of
true. That was a decision, against the standard library alone, which the
diagram packages had kept until then. Only `chart/picture`,
`diagram/picture` and `internal/canvas` import `golang.org/x/image`, so a
program drawing charts or diagrams in text, or a diagram as SVG, does not
link it.

`table` does not import the root package, and nothing imports the YAML
library twice: a program that only builds tables does not link it.

## Design decisions that must not be quietly undone

These are the reasons the package is shaped the way it is. Changing any of
them is a real decision, not a cleanup.

**Cells are stored exactly as passed and formatted only at render time.** It
would be simpler to format on `Append` and keep running column widths. Do not:
a value formatted on the way in cannot be recovered, and two things depend on
it — a column can be re-aligned or re-formatted after its rows exist, and the
same rows can feed a renderer that is not text. CSV already relies on this.

**Widths are measured in terminal columns via `Table.Width`, never `len` and
never `utf8.RuneCountInString`.** Bytes, runes and display columns are three
different numbers. The failure is silent: a table that looks correct until
someone's data is not ASCII. `Table.Width` is a field so a caller can choose a
cheaper measure; the package must never measure with anything else, or padding
and column sizing disagree and only non-ASCII data shows it.

**A table with no rows renders as no text at all, but CSV still writes its
header.** The asymmetry is deliberate and documented at both sites: a header
over blank space misleads a human, while a program reading CSV usually needs
the header line and a zero-byte file breaks parsers that require one.

**`Align` and `MaxWidth` do not apply to CSV; `Format` does.** Padding and
truncation are concessions to a fixed-width display. Truncating a value on its
way into a file someone will compute with loses data silently.

**The object and the table are built separately by the caller, and neither is
derived from the other.** It is tempting to generate the JSON from the table
(or the table from a struct's tags) and delete half the caller's code. Do not:
forcing the richer shape through the poorer one makes both worse — JSON full
of pre-formatted strings, or a table with a column per struct field. This is
why `table` has no row accessor: there is nothing for a converter to be built
on, and that absence is deliberate.

**`nil` is not `empty`.** A nil `Data` or `Table` means "the program did not
supply this", and asking for a format that needs it is an error wrapping
`ErrNoData` or `ErrNoTable` with nothing written. A zero-row `Table` is a
supplied answer that happens to be empty and keeps `table`'s own rules — text
writes nothing, CSV writes its header. Collapsing the two would make a
forgotten field indistinguishable from a genuine empty result.

**`WriteJSON(w, nil, ...)` writes `null`, where a `Document` with a nil `Data`
is an error.** The asymmetry is deliberate and commented at both sites: in
`WriteJSON` the nil is the argument the caller passed, and `null` is JSON's
word for it; in a `Document` it is a field nobody filled in. A typed nil
pointer is a value in both, since only a nil interface is absent.

**There is no `default:` arm falling through to text.** An unknown format is
an error at parse time and at write time. A program that prints a table when
its caller asked for JSON has produced unparseable output and no message
saying why.

**JSON does not escape HTML, inverting `encoding/json`'s default,** and
marshal errors are wrapped with `%w` and never flattened with `%v` —
`*json.UnsupportedTypeError`, `*json.UnsupportedValueError` and
`*json.MarshalerError` all carry things a caller can act on.

**YAML's indent is forced to two spaces and `Encoder.Close` is always
called.** Dropping the `SetIndent` yields four-space YAML that is still valid
and still parses identically, which is why the test asserts literal
indentation rather than a round trip. `Close` is the encoder's documented way
to finish a stream; v3.0.5 happens to flush at the end of each document, so
removing the call changes nothing *today* — that is precisely why it must
stay, because the flushing point is an internal detail and the failure if it
moves is a truncated document rather than an error.

**`WriteYAML` buffers the whole document before writing a byte of it.** The
encoder streams, so a value it rejects part-way through had already put tens
of kilobytes of truncated YAML on the writer — measured at 270 KB for a big
map followed by a `chan int` field, cut off mid-value with no trailing
newline, where the JSON equivalent wrote nothing. Weakening the documented
"empty or ends in exactly one newline" invariant to describe that exception
was considered and rejected: the invariant is the useful thing, so the format
was made to keep it. The cost is holding the rendered document in memory.
(The one exception the invariant does state is YAML's own: a last value that
is a string ending in newlines is written as a block scalar, `|+`, which keeps
them, since anything else would not read back as the same string. That is
the value's newline, not a fragment.)
A second, welcome effect: the single write is now this package's own, so a
writer error is wrapped with `%w` and `errors.Is` reaches it — the YAML
library formats write errors into text of its own and would have lost it.

**An out-of-range `YAMLStyle.Indent` is an error, not a clamp.** The emitter
honours 2 through 9 and RESETS anything else to 2 — not to the nearer bound,
to 2 — so `Indent: 10` silently produced output identical to never setting
it. `Indent: 0` still means the default of 2, because that is the zero value
of the field and the caller saying nothing; any other value outside [2, 9] is
refused with an error naming the value and the range, before anything is
written. Clamping 10 to 9 is a quieter version of the same surprise, and the
ceiling is the library's, so it is documented on the field.

**`WriteYAML`'s recover shim is narrow on purpose, and cycles cannot be
fixed.** The library panics with a `string` beginning `"cannot marshal type: "`
on a channel or a function, and over the caller's own value in a few more
ways: with an `error` for struct tags it cannot follow (two fields with one
key, an `,inline` on a field that cannot be inlined, an unknown flag), and
with a string for an inlined map holding a key a field has. Those panics,
recognised by their text (`yamlCallerPanics`), become errors; anything else
is re-panicked because it is an encoder bug. The shim once converted the
first alone, on the belief that every other panic was the encoder's fault,
and `--format yaml` crashed on a struct with a duplicated tag that JSON wrote
without complaint. Two more were found after: a struct embedding an
unexported type, and a nil pointer held in an interface, both of which JSON
writes and the encoder panics on from reflect or the runtime. They are matched
by their exact text too, and say what to change; a reflect or runtime panic of
any other text is still re-panicked. The shim wraps the encode alone, not the write, so a panic
from the caller's `io.Writer` is not caught by it.
`TestWriteYAML_UpstreamStillPanics` and
`TestWriteYAML_UpstreamStillPanicsOnStructTags` are tripwires on those panic
values — a dependency bump that changes them must fail there, naming the
dependency, rather than turning every unsupported type back into a crash. A **cyclic** value is
encoded again at every level for ever: it spins, emitting an ever-deeper
nesting of the same data and consuming CPU and memory without bound until the
buffer cannot grow or the stack overflows, and a stack overflow is fatal and
unrecoverable in Go. Buffering did not change that; it moved the unbounded
growth from the writer into memory. A reflective pre-walk to detect cycles was
considered and rejected, since it would cost that walk on every write. It is
documented in `WriteYAML` and in the README.

**`progress` renders live only on a terminal, and never wraps.** These are one
decision. A live line that exceeds the terminal's width occupies two rows,
while the cursor arithmetic that erases the region counts one — so from the
next redraw onward the display eats the log line above it, permanently. Every
line is therefore measured in terminal columns and truncated, and a bar with
too little room drops fields from the right rather than wrapping. A line
also stops one column short of the terminal's width: what writing the last
column does differs between terminals (xterm's deferred wrap lets `\x1b[K`
erase it; the Windows console wraps at once), and a line that never reaches
it is safe on all of them. Where the trough's glyphs are not a column each by
the display's measure -- they are East Asian Ambiguous, two columns in a CJK
locale -- the trough is drawn in ASCII. A width test measures the screen
before `Stop`, which erases it: two of them once measured after, and passed
on an empty screen whatever the lines' widths. `Display`
degrades to plain periodic lines with no escape sequences whenever the writer
is not a terminal, which is what makes `program 2>log` safe and what lets a
caller use it without branching on where its output is going.

**A redraw REPLACES the region; it does not append.** `draw` erases first,
unconditionally. Appending looks very nearly right on a terminal, because the
old copies scroll away — and is unmistakably wrong the moment output is
captured. This is asserted by replaying the escape sequences onto a model
screen (`progress/screen_test.go`) rather than by matching bytes: the property
is about what a terminal ends up showing, and a golden byte string would both
need rewriting for every cosmetic change and still pass if the sequences were
internally inconsistent.

**`Display` is an `io.Writer`, and `progress` contains no logger.** That
writer is the entire mechanism for combining log output with live bars, and
any logger built over an `io.Writer` composes with it. Adding a logger here —
or importing one — would couple the two for no gain; the seam already works.

**The bars on one display share a label column and a trough width.** Laid out
independently each line sizes its trough from its own text, so two bars whose
counts differ in length close at different columns and the region jitters
sideways as the numbers grow. When the width is tight the last field is
dropped from *every* bar, not from the one that overflowed, for the same
reason.

**Terminal detection is one `TIOCGWINSZ` ioctl, not `Stat` and
`ModeCharDevice`.** "Is this a terminal" and "how wide is it" have the same
answer, and the usual `ModeCharDevice` shortcut is wrong in ordinary use:
`/dev/null` is a character device, so it reports a terminal and the program
writes escape sequences into it. The platform files use only `syscall`; adding
`golang.org/x/term` for this was considered and rejected to keep the
dependency list where it is. The Windows file is written against the
documented API but is **not exercised by this project's testing**, so every
uncertainty there fails closed: anything that does not succeed reports "not a
terminal" and the display falls back to plain lines.

**Each field's width is reserved before the run starts, and only ever grows.**
Sizing the trough from what the fields currently measure re-fits it around
whatever the numbers happen to be at that instant: it loses a column every
time the counts gain a digit, and lurches when the rate and the estimate
appear a second in — the most visible of the lot, because by then the eye has
settled on a bar that had stopped moving sideways. Two of the four widths are
known exactly up front (a percentage is always four columns; the counts are
widest when done equals the total). The other two cannot be, and their
estimates matter *more* rather than less, because those are the fields that
are not there yet. `widen` covers whatever outruns an estimate, and never
shrinks a slot, so the trough can narrow once but never oscillate. The cost is
a few columns held for a field that a short run never shows, which is the
price of the width being fixed.

This is also why `formatRate` abbreviates past ten thousand. An unabbreviated
rate has no bound on its width, so the field would keep outgrowing its
reservation and take another column off the trough each time — the very
jitter the reservations exist to prevent.

**Colour applies to the fill only, and never enters the width arithmetic.**
This is why colour was not in the package's first version: every cell has to
be measurable in terminal columns, and escape sequences are not. The line's
width is now *accumulated as it is built* rather than measured afterwards — a
trough is `barWidth+2` columns whatever it is painted in, because colouring
adds sequences and never characters. Measuring the assembled string instead
counts the digits of an SGR as columns, decides the line is too wide, and
truncates it mid-sequence; that is mutation-tested. Truncation and colour can
never meet in any case: `resolveLayout` gives up the trough entirely when it
cannot fit, and no trough means no colour.

Monochrome is the same code path as the coloured modes with `paint` returning
its cells unchanged, rather than a second renderer that could drift from them.
The gradient spans the whole trough, not the filled part, so a cell keeps its
colour as the bar grows past it — blending across only the fill would recolour
every cell on every redraw, so a bar filling up would also shimmer. Sequences
are emitted only where the colour changes, which under the 256-colour palette
coalesces most of them away.

Colour is refused wherever it would end up somewhere it cannot be seen or
removed: never in plain mode (those lines go to log files), never when
`NO_COLOR` is set or `TERM=dumb`, and 24-bit sequences only when `COLORTERM`
claims them — a terminal that does not know that form prints its digits across
the bar, so 256-colour is the default and truecolor the opt-in, not the other
way round.

**The rate window is much longer than the redraw interval.** Redrawing twelve
times a second is what makes a bar look continuous, but a rate measured over
80ms is dominated by whatever the scheduler was doing and swings by an order
of magnitude between redraws. The previous figure is held until half a second
has passed -- but not for ever: a bar that has not moved for five seconds
shows no rate and no estimate, since a hung job showing the speed it last ran
at reads as a job running. A timer redraws the display to say so: a display
redraws on its bars' calls, and a hung job's only bar makes none. Relatedly, the displayed rate is a *trailing* measurement while the
estimate is derived from the *cumulative* average: the two answer different
questions, and an estimate that lurches by minutes on every hiccup is one
nobody can use.

**An unknown total gets no percentage, no estimate and no empty trough.** A
full-width bar that never fills reads as a bar that is stuck rather than as a
measurement nobody has. The count and the rate are the honest report, and they
move, which is what a viewer reads a bar for.

**`Bar.Set` must not allocate when it does not redraw.** It is called once per
unit of work — per video frame, in the intended consumers — so the
non-redrawing path is an atomic store, a clock read and a comparison, with the
throttle checked outside the mutex.

**A chart's truthfulness is decided in `chart`, never in a renderer.** Where
a line breaks (`Segments`), what a column shows (`Bin`), the scale
(`RobustRange`, `NewScale`) and the ticks are computed once, and braille and
the picture only turn them into marks. A renderer that made its own choice —
joining across a gap because a picture has the pixels to — would make the
two drawings of one chart disagree, which is worse than either being coarse.

**A missing reading is never drawn as zero, and a column without one is left
blank.** A line dropped to the bottom of the axis where nobody measured reads
as a measurement of zero. `Point.Ok` is the flag; nothing defaults it.

**`chart/picture` draws with its own primitives rather than a 2D library.**
A chart needs a rectangle, a disc, a stroked line with round joins and a
label; a general drawing library would be a dependency for that, and for every
program that draws a chart. Every part of a shape is wound clockwise so the
rasteriser draws overlaps once — a line's segments overlap at every join, and
a part wound the other way would cut a hole there.

**`chart/terminal` shows a picture only where it will be seen as one.** On a
terminal, in one known to show pictures, and not inside tmux or screen;
everywhere else braille. A picture's escape sequence into a pipe or a file is
a page of base64.

**A `Date` scale reads its seconds in UTC, and `plot` places a date by its
wall clock as written.** A file's `09:30-05:00` is plotted at 09:30, not at
14:30 UTC nor at whatever the reader's zone makes it: a chart should read as
the file does to the people who wrote it. The cost is a step of an hour where
a file crosses a change of clocks, which is what its own timestamps say too.

**A `diagram` grid's places never depend on its edges.** It is the reason
grids exist: a diagram drawn of a system's state is compared with the last
drawing of it, and a box that moved because a link broke is a difference
the reader has to discount. Layers -- placed by the edges -- are for flows,
whose shape is their edges. Anything that lets an edge move a box in a grid
-- a router that nudges boxes apart to fit a line, say -- breaks the one
promise `TestGridIgnoresEdges` holds.

**A `diagram` is laid out in character cells, once, and every renderer
draws that layout.** The text drawing and the pictures agree on where
everything is because none of them decides it: labels are placed in
`labels.go`, not by a renderer finding a free spot on its own canvas. The
cost is that a picture's text must be monospaced, at `ScaleFor`'s width.

**`diagram` edges run along rows and columns, and go round a box rather
than through it.** A straight line through a box reads as a line from it;
in a grid of replicas, the primary's line to the third replica drawn
through the second says the second replicates to the third.

**A label is text someone else may have written, and is written as text.**
A DOT file, a bar's label built from a file name, a CSV's column names: none
of it is the program's own. In the text drawing, in `progress`, in a table
and in a braille chart's titles, a control character, a format character
(a bidi override, a zero-width space; the two joiners excepted) and a byte
that is not UTF-8 are each written as one U+FFFD -- an escape reaching the
terminal is a command to it (clear the screen, retitle the window, write
the clipboard), a line break or a tab breaks the box or the bar it is in,
and a bidi override turns the rest of the line round. The four packages
each keep their own `printable` and `hidden`, since none may import
another; change one and change all four. In SVG, which
a browser runs scripts in as it would a page, a label goes only into a
`<text>` element's content, through `xmlText`, which escapes the markup
characters and replaces what XML cannot hold; no attribute is ever made
from a DOT string, and the writer emits no `href`, `url()`, `style` or event
attribute. `TestWriteHoldsHostileLabelsAsText` holds that, so carrying DOT's
`URL` through, say, is a decision and not an accident.

**A label belongs to one line, visibly.** A label that sits as near
another line as its own names neither. So: a label too wide for the gap
between two neighbours in a grid goes past the boxes' ends -- above the
top row, growing the drawing there, if need be -- rather than over a box,
and never by widening the gap, which would let an edge move a box; an
orthogonal edge's label
goes by a stretch of its own path no other edge shares, from the tail,
even a single cell -- the one before its arrowhead -- before any shared
trunk; and a straight edge's label is written on its own line, centred on
it in a picture by a fractional nudge, where only that line is there, and
otherwise at the free place nearest its line and not as near another's.
The orthogonal labels' places are chosen all together, in `placement.go`,
not first-fit one at a time: each place has a cost -- over a box or a
line, touching a box or running into a line, another edge's line as near
as its own, far from its own -- and so does each pair of labels close
enough to read as one phrase, and a local search moves labels, one or a
few at a time, until no move costs less. First-fit let the first label
take the place a later one needed, and every local rule added on top of it
made some other label worse; `TestExampleLabelsStandApart` holds the
examples to what the costs are for.

**Two edges never meet unless they share an end.** `route.go` searches the
page's cells for each path, shortest edges first, and a path may cross
another's line only at a right angle -- never run along it, turn on it, or
end where it ends -- unless each runs one way, an arrowhead at one end
and none at the other, the two are drawn alike, and they have their
arrowheads at one node and their bare ends at two, or their bare ends at
one and their arrowheads at two: a trunk merging into one arrowhead, or a
stem forking, which is the truth and the clearest drawing of it. One way,
because the arrowheads at a shared stretch's far ends are all that say
which way it runs: an edge forking off a line with an arrowhead at each
end could have come from either end, and a fork of lines with none joins
any two of the three. Which of From and To an arrowhead is at does not
matter -- `Base -> Derived [dir=back]` runs one way as `Derived -> Base`
does, and a class diagram's arrows merge into the base class's arrowhead
either way -- but an arrowhead at a node where the other edge's end is
bare is a chain through it, and shares nothing. `meet` is the rule;
`TestOnlyOneWayEdgesShareALine` and
`TestOneWayEdgesShareALineWhicheverWayWritten` hold it. Anything else
joining two lines reads as a connection that is not there: a to b and b to
a drawn as one double-headed line, or one edge's line taken over half way
by another's. `TestEdgesNeverJoin` holds it on a service mesh, where the
earlier router, which routed each edge alone from fixed shapes, broke it
dozens of times. A path also leaves and enters by the sides facing the
other box where it can, so a flow keeps flowing. Do not "simplify" this
back to shapes worked out per edge.

**A port steers a line, never a box.** `Edge.FromSide` and `ToSide` --
DOT's compass points, `a:s` or `tailport`/`headport` -- limit which outline
cells `outline` offers the search, and nothing else: the grid and the
layers place the boxes as before. Keeping lines apart outranks a port: a
path by any side that joins no line is taken before one by the side asked
for that does, and `PlacedEdge.SidesDropped` and `diagram`'s note say so,
since a port with no visible effect and no word reads as one ignored. One
end that cannot keep its side does not give up the other's: the search
free to end anywhere charges `missCost` for an end off the sides asked
for. Where `a:s` and a `tailport` meet, the reader takes them in
Graphviz's order -- the statement's own attribute, then the node's port,
then a default -- which `withPorts` keeps by holding the node's port as
the edge's `tailport` attribute, as Graphviz does.

The page the router searches has a margin on every side, not only the right
and the bottom: without room above, an edge between two boxes of the top row
had no way round the box between them. What the lines do not take of it is
trimmed off, so the whole drawing may sit a row lower or a column further
right when a line goes round its top or its left -- every box keeps its
place relative to the others, which is what `TestGridIgnoresEdges` holds.
Where the lines already drawn leave an edge no path the rules allow, it is
searched for again with joining another line allowed at a cost far above any
detour, and running through a box still forbidden: a joined line is a fault,
a line through a box the worse one, since it reads as that box's own. Only
where even that search finds nothing is the plain path drawn. A loop is
searched for as any edge is, and shares no line with another; an invisible
edge is not routed at all.

**Straight edges are the pictures' alone, and only where clear.** With
`splines` set in the DOT, or `--edges straight`, a PNG or an SVG draws an
edge as one straight line between its boxes, at whatever angle -- but only
where that line is clear of every other box; one that would cross a box
keeps its path along rows and columns, for the reason above. The text
drawing draws them along rows and columns unless `TextOptions.Braille`
(`--braille thin|thick`) asks for braille: a diagonal of `╱` and `╲` is a
ragged staircase, but in braille dots, two across and four down a cell, it
is a line at its angle. Off is the default, as dots are fainter than
box-drawing lines in many fonts. In braille a straight edge ends in a
one-cell stub into its box with the usual arrowhead, the dots aimed at the
stub, so the text bends where the picture does not; it passes under a
line it crosses shallowly and is drawn in dots with one it crosses steeply,
as two lines' dots running along each other for cells are a smudge.
Straight edges'
labels are placed by `placeStraightLabels`, against the lines as a picture
draws them, and may sit above the first row, the layout growing at the top
to hold them.

**`diagram/dot` reads DOT as Graphviz does, and adds attributes rather than
syntax.** `columns`, `rows`, `row`, `col` and `style=lane` are attributes
Graphviz ignores, so a file written for `diagram` still draws in Graphviz,
and a file from any program that writes DOT draws here.

**`plot` joins a column's readings row to row, not by their distance on the
axis.** Its rows have no rhythm to measure a gap by — trading days skip
weekends, a log skips quiet hours — so a line breaks only where a row has no
reading, and only where the drawing can show it: a missing cell among a
hundred thousand rows is less than a column wide, and breaking there would
shred the line.

**`plot`'s y axis spans every value; `--robust` is opt-in.** `RobustRange`
suits a sensor whose spikes are dropouts; in someone's spreadsheet the
outlier is as likely to be the point — the wettest day of the year.

**A column is a column of numbers when most of its values are, not all.** A
typed file holds the odd "n.d."; refusing the column for one would refuse the
file. The cells left out are counted and reported, so a gap is never
unexplained. Dates in numbers alone that read the same day-first and
month-first are refused until `--dates` says which, rather than guessed.

## Testing conventions

The characteristic bug here is output that *looks* right, so a test asserting
"no error" is worth nothing.

- Prefer asserting the **invariant** over a golden string where the invariant
  is the point. `TestWidth_WideCharactersKeepColumnsAligned` asserts every
  rendered line occupies the same number of terminal columns; a golden string
  would encode one right answer, this encodes why it is right.
- Mutation-test anything load-bearing before trusting it: break the production
  code deliberately, confirm the test fails, restore. The width handling, the
  CSV flush-error path and the sign of every deliberate asymmetry were all
  checked this way, as were `progress`'s erase-before-draw, its
  trailing-newline guard and its layout arithmetic.
- In `progress`, note that `composeLine` truncates as a backstop, so an error
  in the layout arithmetic does NOT show up as an over-wide line — it shows up
  as a line quietly missing its right-hand fields on a terminal with room to
  spare. `TestDisplay_DoesNotTruncateWhatWouldHaveFitted` is the assertion
  that catches that class; the width invariant alone cannot.
- Examples are tests. Add one for anything a user would reach for first.

## Git

Create the topic branch *before* the first edit, and do not move `main` until
the user gives an explicit go-ahead:

```
git checkout -b <topic>     # before editing anything
git commit ...              # as the work lands
```

Then stop and report which branch the work is on. Once the user says go ahead,
`main` wants a linear history with no merge commits:

```
git checkout main && git merge --ff-only <topic> && git branch -d <topic>
```

Do not push; that is the user's step. Commits are SSH-signed (`git log
--format="%h %G? %s"` should show `G`). Messages are long and explain *why*: an
imperative subject line, then several paragraphs on what was wrong, what was
considered and rejected, and what a reader might otherwise mistakenly "fix".
Do not reference other commits by hash.

## Scratch space

Anything a session produces — captured output, throwaway probe programs,
benchmark logs — goes in the gitignored `.scratch/` at the repo root, never in
a per-session temp directory. A Go program dropped there needs its own
`go.mod`, or it becomes part of this module and breaks `go build ./...`.

## This repository is intended to be public

Apache-2.0. `mattn/go-runewidth` is MIT, as is its own dependency
`clipperhouse/uax29`. `go.yaml.in/yaml/v3` is **dual-licensed, MIT and
Apache-2.0** — the eight files ported from libyaml (`apic.go`, `emitterc.go`,
`parserc.go`, `readerc.go`, `scannerc.go`, `writerc.go`, `yamlh.go`,
`yamlprivateh.go`) are MIT and the rest is Apache-2.0, and it ships a NOTICE
file — so it must be described as both, never as one. All are compatible.
Keep personal data and machine-specific paths out of commits, comments and
test fixtures.

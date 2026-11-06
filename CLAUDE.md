# output

Go packages for producing program output in more than one shape. The root
package `output` writes one result as text, CSV, JSON, YAML or a Markdown
table, chosen when the program runs; `table` is the tabular half of that —
data rendered as aligned text, or written as CSV or Markdown; `progress` is the live half — bars pinned to
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
make bench COUNT=10        # time the layout on fixed diagrams; compare with benchstat
make fuzz FUZZTIME=5m      # search for DOT, CSV and cells that break a writer
make fuzz-replay           # every input a search kept, once: seconds
make fuzz-save             # keep what searches found, in testdata/corpus/
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
- `table/markdown.go` — `MarkdownStyle` and `WriteMarkdown`: a
  GitHub-flavoured table, over the same rows, its cells escaped.
- `progress/progress.go` — the package doc, `Display`, `Options`, `Mode`, and
  the redraw/erase/layout machinery.
- `progress/bar.go` — `Bar`, `BarSpec`, and the rendering of one line: the
  trough, the counts, the rate and the estimate.
- `progress/term_unix.go`, `term_windows.go`, `term_other.go` — build-tagged
  terminal detection and size.

- `chart/series.go` — the package doc, `Point`, `Segments`, `Smooth`.
- `chart/bin.go` — `Column` and `Bin`: what each column of a drawing shows.
- `chart/scale.go` — `RobustRange`, `Scale`, `NewScale`, `ExactScale`, ticks
  and labels, a `Log` scale's among them.
- `chart/braille.go` — `Chart`, `Strip`, and the braille rendering.
- `chart/date.go` — the `Date` kind's steps, calendar ticks and labels.
- `chart/band.go` — `Band`, `BandKind`, and their braille rows.
- `chart/picture/picture.go` — `Draw`, `Palette`, `Options`, `TextPx`.
- `internal/canvas/` — the few drawing primitives a picture needs, over
  `golang.org/x/image/vector` and Go Mono: shared by `chart/picture` and
  `diagram/picture`, and internal so that it is not an API anyone else
  depends on.
- `examples/` — DOT files a reader can copy, each drawn as text, PNG and
  SVG by `cmd/diagram`'s `TestExamples`, and CSV and JSON Lines files,
  each drawn in braille and as a PNG by `cmd/plot`'s, so one that stops
  reading fails the build rather than the reader.
- `internal/cli/` — what the commands share in reading their command line:
  `Parse` takes flags before or after the file and returns a mistake as a
  `UsageError` without printing, `Exit` reports it and exits 2 (0 for
  help, which the command prints on standard output, 1 for anything
  else), and `PrintDefaults` lists the flags as `--out string`, as fitdash
  does. The standard `flag` package reads
  `-out` and `--out` alike; pflag would list them the same but read `-out`
  as `-o -u -t`, breaking command lines already written.
- `diagram/internal/layoutcheck/` — `Check`, for tests alone: the
  promises any correct layout keeps of its lines -- along rows and
  columns, through no box, ending on the outline, off the corners and the
  joints, turning nowhere straight out from a joint, `SidesDropped` said
  truly, a record two cells or more from any other box -- worked out from
  the boxes and paths
  alone, never by asking the router, so that it still judges when the
  router and its own answer are wrong together. It takes plain data, not
  a `diagram.Layout`, as `diagram`'s own tests import it; each test
  package converts (`checkView`). Where a test makes every search give up,
  each edge is held to what a plain path keeps by `Edge.Plain`, from the
  layout's own unexported mark, which only `diagram`'s tests can read;
  anywhere else an edge drawn plain fails (`checkLayout`), so that the
  mark, the router's own answer, never excuses a path the router should
  have found. `FuzzParse`, which cannot read it, fails on one the same
  way, through the path's own faults. A rule it holds is a gate; a line moved
  a cell, or a path changed, is not wrong for being different.
- `internal/fuzzcorpus/` — `Strings`, for tests alone: reads a fuzz
  target's kept inputs from `testdata/corpus/`, skipping unless
  `FUZZ_CORPUS` is set, which `make fuzz-replay` does.
- `internal/memory/` — `About`, the words a refusal gives the memory a
  drawing would take, "about 1.3 GB": shared by the packages that refuse a
  drawing as too large and the commands.
- `chart/terminal/terminal.go` — `Choose`, `Write`, the iTerm2 and kitty
  writers, `LightBackground`; `size.go`, `Size` and the one rule both commands
  size a picture for the terminal by -- `PictureSize`, twice the pixels
  the characters cover, rounded to the nearest and held to
  `MaxPictureWidth`; `PictureScale`, the same a column and a row, for
  diagram, which fits its drawing's own width to the columns; and
  `PictureCols`, the columns held to the window's, which both commands
  say where `--width` asked for more; `size_*.go`, build-tagged,
  `SizeOf`.
- `cmd/plot/` — the `plot` command: `read.go` reads CSV into a table of
  text, `jsonl.go` JSON Lines into the same table, `columns.go` decides
  what each column holds and reads its values, `chart.go` turns the
  columns into a `chart.Chart`, `main.go` is the flags and the drawing. A
  new input format is a new reader of the same table.
- `diagram/diagram.go` — the package doc, the model (`Diagram`, `Group`,
  `Node`, `Edge`), `Palette`; `layout.go` — `Layout`, grids and layers;
  `route.go` — the search for edges' paths along rows and columns; `straight.go` — which edges a
  picture draws straight, and their labels; `labels.go` — where labels go;
  `placement.go` — the costs and the local search that choose orthogonal
  edges' label places together;
  `scale.go` — cells to a picture's units; `text.go` — the text drawing;
  `width.go` — `Width`, the one measure of a label in cells; `ends.go` --
  where on a node a line may end, worked out once as it is placed
  (`settle`), relative to its box, and priced for joints (`priceJoints`),
  the one answer every way a line is drawn asks; `record.go` —
  a record's fields sized and placed, and which outline cells are beside a
  field.
- `diagram/dot/` — the DOT lexer, parser and colours; `record.go` reads a
  record's label into its fields.
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

**A Markdown table is the text table's rules for a page, and its cells are
text.** `WriteMarkdown` follows `Render` where a reader is the point --
`Align` applies, as the row of dashes and as padding so the source lines up;
an empty table writes nothing -- and departs where a renderer differs:
`MaxWidth` does not apply, as a renderer wraps a long cell rather than running
it off an edge, and a line break is `<br>`, as a newline would end the table.
`Format` applies, as it does to CSV, being the caller saying how a value is
written; separators are skipped, as a Markdown table has no rule between rows.
A cell is someone else's data as much in Markdown as on a terminal, and there
its punctuation is markup: a name with stars in it would be set in italics,
`[x](url)` would be a link, a pipe would end the cell and shift every cell
after it. So each of `` \ ` * _ [ ] < > | ~ & $ `` is escaped -- an underscore
inside a word excepted, which CommonMark never reads as emphasis, so
snake_case stays readable in the source -- and other punctuation is left as it
is, so numbers, dates and paths read in the source as they render.
`MarkdownStyle.Markup` is the opt-in for a caller's own Markdown, escaping
only a pipe, and not one escaped already, where a second backslash would let
it end the cell. Escaping every ASCII punctuation character, which CommonMark
allows, was considered and rejected: `1\.5` and `2024\-01\-02` in the source,
for nothing a renderer does differently. What escaping cannot stop -- GitHub
linking a bare URL, mentioning `@name`, linking `#12` -- is documented, not
worked round. `TestWriteMarkdown_CellsNeverBreakTheTable` holds the shape:
every line has a cell-ending pipe a column and one more, and every line is as
wide as the first.

**`nil` is not `empty`.** A nil `Data` or `Table` means "the program did not
supply this", and asking for a format that needs it is an error wrapping
`ErrNoData` or `ErrNoTable` with nothing written. A zero-row `Table` is a
supplied answer that happens to be empty and keeps `table`'s own rules — text
and Markdown write nothing, CSV writes its header. Collapsing the two would make a
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
redraws on its bars' calls, and a hung job's only bar makes none. A plain
display is watched the same way, and writes one line without them, as its
last line in a log is read as the job's state. The timer draws only where a
bar has reached its deadline; one firing early, the bars having moved since,
sets itself again, so a plain log gets no lines between its own. A bar
moving again after a stall starts the window again from there. A count gone
back does not, nor the estimate's average: a count back by one -- a retried
chunk, two workers' Sets out of order -- is a hiccup, not a reset of the
run, and restarting at each kept the rate off the screen. Relatedly, the displayed rate is a *trailing* measurement while the
estimate is derived from the *cumulative* average: the two answer different
questions, and an estimate that lurches by minutes on every hiccup is one
nobody can use.

**An unknown total gets no percentage, no estimate and no empty trough.** A
full-width bar that never fills reads as a bar that is stuck rather than as a
measurement nobody has. The count and the rate are the honest report, and they
move, which is what a viewer reads a bar for.

**`Bar.Set` must not allocate when it does not redraw.** It is called once per
unit of work — per video frame, in the intended consumers — so the
non-redrawing path is a clock read, an atomic swap, a load or two and a
comparison, with the throttle checked outside the mutex, and a store where
the last noted move is a hundredth of a second old. They note when the count
moved, and moved again after a stall, as it happens: judged at a
redraw instead, a move between redraws was seen late, and a hung job kept its
rate up to twice the stall's five seconds.

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

**A record is fields in cells, and a field's port is a part of its
outline.** A record is laid out as any node is -- in cells, once, every
renderer drawing that -- its fields side by side where the layers run down
and stacked under `rankdir=LR`, as Graphviz lays them, a field divided
further running the other way, and the room a field is given past its own
size shared among those beside it, so the dividers stay evenly spread. A
divider runs a cell past its field at each end, onto the outline or the
divider it meets, so the text drawing joins them as `┬` and `├` with no
special case. `FromPort` and `ToPort` narrow the outline cells `outline`
offers to those across from the field on the sides it reaches, by the same
mechanism and the same `missCost` fallback as a side, and `SidesDropped`
covers both. A field with fields on every side reaches none, and may end on
any side across from it: a line here never runs through a box to a field's
middle, as Graphviz's does. No line ends where a divider meets the outline,
as none ends at a corner, since it reads as running into the divider -- in a
record of two equal fields that is the middle of a side, where a line most
wants to end. Beside one, drawn `┴┬`, it reads the same way, but a field
two cells wide, or the side of one a row tall, has nowhere else: the
search charges `jointCost` there, more than the two turns and the steps
that move an end a cell along, and does not forbid it. It is charged only
where the same side has a cell beside no joint, among those as asked for:
it chooses a cell along a side and never a side, for charged on every such
cell it sent lines between records under `rankdir=LR` round their tops,
off the sides facing (`TestJointCostKeepsSidesFacing`). Nor does a line
turn in the cell straight out from a joint, or join another there: one
stretch of the turn runs in line with the divider, `─┐├`, or a junction
`─┤├`, and reads as the divider running on out of the box, where the trunk
of two lines into a record met beside one. The search never turns there,
in any of its tries, as it never ends a line at a joint: a turn a cell
further out costs a step or two, and in the random diagrams no edge lost
its search to it. Where no path but one turning there exists, the edge
is drawn plain, through whatever is in the way -- the worse fault, risked
as it is for an end at a joint, so that the rule stays one the checks
hold without exception. Forbidding it in the strict tries only, and
charging `joinCost` for it in the last, was considered and left for when
a layout shows the risk taken. A line running past along the side, `│├`, is clear of
it. `layoutcheck` holds it of every path the search found, and the
drawing's check of a record looks for any line with an arm the divider's
way there, a corner among them -- it once looked for a junction alone,
and the random diagrams' test passed with turns there in its drawings. That holds of every
line, not only the search's: a straight
one leaving a record by a joint -- the middle, or a cell from it once lines
between the same two nodes are set apart -- is drawn along rows and columns
instead (`keepClear`, before routing, as the other lines are routed
round the straight ones); a braille stub (`stub`) chooses among the node's
ends on the side the line reaches, each a cell out, priced as the search
prices them, so it never sits across from a joint and sits beside one only
where its side has no clear cell free -- it looks at its own node's ends
alone, and another's joints are never beside it because a box with joints
stands two cells or more from any other, which `layoutcheck` holds -- and,
a plain-text node's corners
being among its ends, a line into a label of a letter or two ends in a stub
and an arrowhead as into any box, where it once had no cell off the corners
and ended in a small arrow at an angle, hard to read; and
the plain path drawn where every search fails (`plain`) keeps its shape and
slides each end up to three cells along its side to where it costs least,
by the search's own rules: each end priced by `outline` as the search's
last try asks it -- any side, others' lines and ends at `joinCost` -- and
the stretches either side of each point moved priced cell by cell as the
search's steps are, each cell once, the corner the two share among them:
a line run along, met where it turns or ends, a straight line a picture
draws turned on, a turn straight out from a joint, or the cell before
another's arrowhead at `joinCost`, a right-angle crossing free. Not moving is priced the same way, so a plain path along another's
line takes a free cell beside it; and a move is refused only for taking
the path through more of the boxes than it crossed already, the plain
shape being drawn through whatever is in the way. An end beside a joint
stays rather than join; an end at one moves even so, as the search never
ends a line there; where every move would cross more boxes, it stays, the
one line that may end at a joint -- and `layoutcheck` allows it there
alone, counting the boxes as `plain` does, so that an end kept at a joint
for a box crossed moved or not fails it. It once moved an end only off a joint, a
cell along, and so took a cell beside the joint as readily as one clear of
it, kept no field, and left a loop round a box three or four cells wide at
its corner. The versions of `plain` before this one priced ends by a rule
of their own and slid an end onto another edge's line, drawing two edges
as one -- which the layout rules do not see, a join being allowed on a
fallback path, and the comparison of drawings before and after did -- and
refused every move where the plain shape crossed a box anyway, keeping an
end at a joint for nothing. `TestStraightLinesMissRecordJointsAtRandom` found the set-apart
and sliding cases that a lone line between two middles did not show; it runs
down and across, so the joints on a record's left and right are held as well
as those on its top and bottom. `PlacedNode.joints` is the one answer to
where the joints are, which the router, the straight lines and the stubs all
ask, and a node's ends (`ends.go`) the one answer to where a line may end on
it -- its outline but the corners and the joints, each marked if beside a
joint -- worked out once as the node is placed, relative to its box so that
trimming the page moves them with it. The search's `outline` walks them in
place, as kept, rather than a copy: copied twice a search, every layout took
a node's outline twice over in memory. `TestEndsMatchOutline` holds the
search to the outline it had before, kept as `outlineBefore`, cell for
cell, cost for cost and in the same order, which decides between paths
that cost the same.
`Node.Field` is the one lookup of a port, which the layout, the DOT reader
and `diagram`'s note of an unknown port all use: three lookups had come to
disagree on a divided field's port, which Graphviz does not have and none
now reads. And two edges share a line at a node only where they end beside
the same field of it (`sharing`): a line into a field and one into the whole
record, merged, end beside the field and the second reads as joining it. The
field, not the port's name: a port no field has is anywhere on the box, and
keeps no line apart.
`TestRecordFields`, `TestRecordPorts` and `TestRecordPortsShareOnlyTheirOwn`
hold these, on Graphviz's own example among others. A label whose braces
nest more than `maxDepth` deep -- the depth subgraphs may nest to -- is no
record's: reading and laying out a record recurse once a brace, in time
growing with the square of the depth, and a 40 KB label of braces took
seconds -- a few megabytes would overflow the stack, which Go cannot
recover from. Where a subgraph so nested is an error, though, such a label
is no error: it is drawn as written and warned of, as below. A record label that is no record's is drawn as written and
said to `Options.Warn`, which `diagram` writes to standard error with its
line: Graphviz refuses one, and drawn without a word its ports quietly
named nothing. It is said once the file has read and the drawing is laid
out, as it says what is drawn: said as it was read, it told of a label
drawn as written in a file an error later stopped from drawing at all. A
port the reader cannot read as written is said to `Options.Warn` too, as
Graphviz warns of it, all its faults in one warning: one naming a field
its record has none of, one naming a field of a node that is no record,
`b:sth` on a box, and one whose compass point is none, `a:f:sth` for
`a:f:s` or a field named with a colon and split at its first, as
Graphviz splits it. The reader says them all, and `diagram` has no note
of ports of its own: only the reader holds the port's text once it is
read as a side, and knows which node has an HTML label, whose ports
Graphviz reads and this does not, and which are left unsaid; a note in
`diagram` said an unknown field without its line, and split one bad port
over two messages. Nor is a port of an invisible edge said, as no line
draws it. A port is read as Graphviz's `record_port` and `poly_port`
read it, in `portOf` alone: the field first, and the rest the compass
point where the node has it; where it has not, a first part that is a
compass point is the side, `b:s:w` the bottom with `w` ignored, which
is said, as Graphviz does not say it; and one that is none names no
side, `b:nope:s` ending anywhere on `b`, its compass point ignored; an
empty field, `b:"":s`, names nothing, its compass point ignored and
said. A box, and a record whose label is none, have no fields; a node
with an HTML label is taken to have any but a compass word, which
Graphviz falls back to where no port of the label has the name. What
in a port names nothing is `portOf`'s answer too, so that reading a port
and saying what is wrong with it cannot disagree, as they once did. It is said where an edge takes it -- not of an `edge [...]`
default no edge takes, as Graphviz says nothing of one -- at the line
the port was written on, the node's, the attribute's or the default's,
which `withPorts` takes with the port; and once a place it is written,
every fault of it over every edge that takes it in one warning, where
Graphviz says it once an edge: a port written on a node once for its
node and line, a tailport, headport or default once a line, port and
end, saying which and naming the nodes it names nothing on, three and how many more, so a bad
`edge [tailport = …]` over a thousand edges is one warning. A port
written on a node of three parts, `a:f:x:n`, is an error, as DOT's
grammar has it. Every error and warning quotes the file's text, as a
label is written as text: `FuzzParse` holds both to it.

**Straight edges are the pictures' alone, and only where clear.** With
`splines` set in the DOT, or `--edges straight`, a PNG or an SVG draws an
edge as one straight line between its boxes, at whatever angle -- but only
where that line is clear of every other box; one that would cross a box
keeps its path along rows and columns, for the reason above. Lines
between the same two nodes are set apart, and each is judged where it is
drawn, set apart: it must begin and end inside its own boxes, off their
outlines, as the picture draws it from where it leaves a box, and the
router keeps paths off it there -- on both rows, or both columns, where
it runs half way between two (`straightClear`, `avoidStraight`). Inside
the shape as drawn, not the rectangle: an ellipse's curve, below a
cylinder's lid and above its base, inside a rounded box's corners' arcs,
which `within` works out in cells from the box and from the lid's height
and the corners' radius the picture draws them by, `Scale.Lid` and
`Scale.Corner`, asked of `ScaleFor`'s; held to the rectangle, a line set apart from an ellipse's
middle began in its corner and was drawn from beside the curve. And a
line leaves a cylinder through its lid wherever it leaves the box above
the lid's middle, by a side as well as by the top, and a rounded box
through an arc wherever it leaves within a corner's square
(`Scale.leave`): `TestWithinIsTheShapeDrawn` holds the two to each other
and to the shape drawn, both ways, as a `within` too strict draws no
line wrong but fewer lines straight. The text
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

**A log scale is a `Kind`, for Y alone, and never draws a reading it has
no place for.** `Kind` already decides a scale's ticks and labels, and those
and `Frac` are all a log scale changes, so it is `chart.Log` rather than a
field beside `Kind`, and both renderers draw it unchanged. It is for a
strip's Y: `Bin` divides the x axis evenly, and `Check` refuses a Log X
rather than bin it wrongly. A reading at or below zero has no place on one,
and `Frac` holds it at the bottom as it holds any reading below a scale --
which would draw it as the least reading, where it is none -- so `plot
--log` makes each one a gap and says how many, as it says of a cell that is
not a number; a library caller does the same with `Point.Ok`. A column's dot
is still its arithmetic mean, drawn by its logarithm: a geometric mean would
draw a different number from the one a column's readings average to, and
the two agree where the column's readings are close, which at a chart's
resolution they mostly are. The ends are rounded out to whole powers of ten,
not to whole steps: every ten powers, readings from 1 to 1e30 were drawn from
1e-10 to 1e40. Its arithmetic is in powers of ten throughout, as readings of
1e-300 and 1e300 overflow a ratio, and it reaches no lower than 1e-300, below
which a float64 holds too few digits for 1, 2 and 5 times a power of ten to
differ.

**A drawing too large is refused by default, and `--high-memory` draws it.**
A size is usually a flag, and a digit too many once took 1.5 GB for a braille
chart before a line was drawn. So each drawing whose memory grows with a size
someone typed has a default limit -- a braille chart's million characters
(`plot`'s alone: `Chart.Braille` sets none), a picture's
`DefaultMaxPixels`, a page's `DefaultMaxCells` -- and its refusal says what
the drawing would take, from a measured cost a unit (`BrailleBytesPerChar`,
`BytesPerPixel`, `BytesPerCell`, `TextBytesPerCell`), and that
`--high-memory` lifts it. A diagram's page is held to its limit as the
router's, its boxes and their margin, and again as its labels, its title
and its legend make it: first at least as wide as the widest edge label
and as tall as the tallest, before any is placed -- the cells they are
placed among are sized by both at once, so a line of two thousand
letters and two thousand lines of one, 6 KB, asked for four million --
and then as laid out. That page is never routed, so its refusal quotes
`TextBytesPerCell`, what every cell of it costs to draw. Counting the
labels into the router's page beforehand was tried and taken out: it was
an estimate kept in step with the labels' placement by hand, and it
already disagreed with it three ways. Whether
the machine has the memory is the user's to know, not the program's: there
is no cap on what `--high-memory` may take, short of what Go will allocate
or the router can number, which are refused with no flag named, since none
would help. One flag in both commands rather than one per limit, so a user
learns one word; it lifts limits on memory only -- `--max-edges` bounds
time, and an SVG's `MaxSide` what viewers open. An explicit `--max-cells`
outranks it. Re-measure the costs a unit when the drawing code changes much,
since a refusal quoting them is only as true as they are.

**`plot` reads JSON Lines into the table of text a CSV is read into.**
A key is a column, in the order keys are first met, a nested one named by
its path (`cpu.user`, `load[0]`), and every value becomes the text a CSV
cell would hold -- a number as written, `null` an empty cell -- so
`columns.go` decides what a column holds the same way for both, and a
string of digits in a log reads as the number it is. Typing columns from
JSON's own types was considered and rejected: logs quote numbers as often
as not, and a second classifier would disagree with the first. A line that
is not one object is left out and said, as a cell that is not a number is
a gap and said: a log cut mid-line by rotation is ordinary, and refusing
the file for it would refuse every such log. A column given two values in
one line, and keys past `maxJSONColumns` and an array's elements past
`maxJSONArray`, are said too: each element is a column, and a long array
took every column, losing the keys after it. A key written twice in one
object keeps its last value, in its first's place, as Python does, and
the whole of the first is gone (`walker.replace`): matched by path alone,
`{"a":{"x":1},"a":{"y":2}}` kept `a.x` beside `a.y` without a word. `NaN` and `Infinity`, which
encoding/json refuses and Python writes, are quoted before decoding
(`allowNonFinite`) where a value stands, never a key, so they read as the
words a CSV's cell would be, and lose no line. A JSON number has a point,
but a string may hold `"1,5"`. **The decimal mark of JSON Lines' strings
is decided from the columns plotted alone, not the file** -- the one place
plot departs from deciding it for the file, as a CSV's is. A CSV's cells
were written by one program in one place; a log's strings are fields of
their own, ids, tags, versions, and decided for the file, a `"3,4"` among
the ids made `"1.234"` in the column plotted 1234 without a word, while a
`"2.5"` in one field and a `"1,2"` in another refused a log plotting
neither. Each column's evidence is kept as it is read (`table.seen`, a
`marks` a column, its sightings holding the strings as written), each
column's strings rewritten with a point once its mark is a comma
(`table.rewrite`, by `readMark`; rewritten as each cell was read, a column
was matched three or four times over), and once `--x` and `--y` are settled,
`settleMarks` decides it for those columns together. A point in one and a
comma in another, or both in one, are refused, naming each, rather than
guessed between -- the user's decision, as `"1.234"` read wrongly is a
chart a thousand times out; where they show neither, a point. Until then,
and for `--list`, each column is read by its own strings -- one showing
both marks the way most of them are, so that a column mostly `"1,5"`
reads as numbers and is refused when plotted, not taken for text and left
out without a word; and `--y` naming one half each way, text however
read, says it is written both ways -- so the default
x is chosen for being in order read by its own, and where the mark the
columns plotted settle on puts it out of order, "1,234" and "2,000" read as
1.234 and 2, the chart is refused, asking for `--x` or `--decimal`: choosing
x and settling the mark wait on each other, and that is the user's to
break, not a guess's. A sighting holds its column's index, named as a
message is written, so the name it says is the one `nameColumns` gave.
Where `--decimal` says the mark, no evidence is kept and `--list` says
nothing of it; nor is it kept of a string with neither `.` nor `,`, which
reads alike either way, nor of one beginning other than a number may, a
digit, a sign or a point, "Request completed.". `--list` says a column's
mark, written both ways or guessed, only where it holds numbers: a tag
among words is no number to plot. A column read
with another mark than its own strings showed is not typed again: its
strings showed none, so each is a number either way or neither, and only
its value changes. `--decimal` applies to every string column, and a
number is never rewritten, as `commaToPoint` would make `1.250` 1250;
whether a cell is a string is its last value's to say, where a key is
written twice. `--input auto`
takes a file for JSON Lines where any of its first 20 lines is a whole
object holding a value -- `{}` gives no column, and a CSV of a column of
them was read as JSON Lines of nothing -- and with no tab before or after
it, not where its first byte is a brace: a log read from its middle
begins with the end of a line, and was read as CSV of fragments. Nor where
half are: a log with a traceback of three lines after each object is a
quarter objects, and was read as CSV too. No CSV holds a line that is a
whole object -- a cell holding one is quoted -- but a TSV quotes nothing,
and an object beside an empty cell is one once trimmed: so a line with a
tab at either end is not counted. The lines that are none are left out
and said. JSON that is not JSON Lines -- one array, an array a
line, objects over several lines -- is refused, naming `jq -c` where it
helps, and looked for before the lines are counted, as an array of objects a
line has a line that is a whole object; objects over several lines only
where the first is, not merely where the first three values are objects,
which a log of three whole lines and then cut ones decoded as. The CSV-only flags, `--delimiter` and `--header`,
are refused for JSON Lines at any value but `auto`, never ignored, as a
`--header no` that changed nothing reads as one obeyed -- before the input
is read, where `--input jsonl` says the format; nor do they make auto take
a file for CSV, as an alias holding one would read every log so. A JSON
Lines row is sparse, `table.sparse`, holding only the cells its line has
keys for, and keys stop at `maxJSONColumns`: padded to every key, a log
keyed by user grew with the square of its lines, 640 MB for 4,000, and
held only to its last key's column it still took 2.9 GB for 5 MB, a line
naming one user of a thousand holding a cell for each before it. Rows are
read through `rowCount`, `cell` and `eachColumn`, which answer the same
of a CSV's dense rows, and `eachColumn` types every column in one pass
over the cells there are. A sparse cell costs a log of the same keys a
line about a tenth more memory than a dense one would. Nesting is read by
recursion, so a line deeper than `maxJSONDepth` is left out, and said to
be nested too deep rather than to be no object, which it is.

Four limits of the JSON Lines reader are known and kept; reviews have
raised each more than once, so here is why. **It is two to three times
slower than CSV, and takes two to three times the memory**: 300,000 lines
of a log in 2 s and 330 MB, against 0.9 s and 145 MB as CSV. Each line has a
`json.Decoder` of its own, read a token at a time (`walkLine`).
encoding/json's Decoder cannot be reset onto a new line, `Unmarshal` into a
map loses the keys' order, which is the columns' order, and one decoder
over the whole file cannot leave out a broken line and read on. Doing
better means a tokenizer of plot's own, judged not worth its code for
seconds at half a million lines; it is the change to make if that
judgement changes. **`table.dropped` holds every key past
`maxJSONColumns`, without a limit.** It is what lets the note count the
keys left out exactly and `--y` say a key was left out rather than that
there is none. The file is in memory whole already and the names are no
larger than it, so the set at most doubles what reading takes; a bounded
one would miscount, or answer "no such column" for a key in the file, or
need the file read twice. **The default x is chosen before the decimal
mark is settled**, and a chart is refused where settling it puts that x
out of order, rather than the mark settled first from the columns the
default would plot: with `--y` given and `--x` not, column 0 would count
toward the mark before it is known to be plotted, and a contradiction in a
column no one plots would refuse the file. The refusal asks for one flag,
and only where the mark x is read with changed. **`--input auto` decodes
the first 20 lines twice**, once to choose and once to read: 20 lines
against a file already in memory, and a cheaper test for a whole object
would lose a line too deep counting as one.

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
- The layout's time is measured in `diagram/bench_test.go`, on diagrams
  that stay the same from one change to the next: flows of 50 to 300
  edges, a flow of labelled edges, grids, and the examples. A change to the
  layering, the router or the labels' placement is measured before and
  after, ten runs each (`make bench COUNT=10`), and compared with
  benchstat. When first written, a flow of 50 edges took 20 ms, of 100
  edges 130 ms, 200 edges a second and 300 edges three: about seven times
  the time for twice the edges, which is why dot's default limit is 300.
- Input a stranger writes is fuzzed. `FuzzParse` (in `diagram/dot`) reads
  DOT and draws it as text, braille, colour and SVG, and holds the layout
  to `layoutcheck`'s promises; `FuzzLayout` (in `diagram`) lays out
  diagrams of records, ports and sides made from its bytes, as
  `randomDiagram` makes them -- DOT text rarely mutates into a record
  with a port -- an odd first byte having every search give up so the
  plain paths are checked too; `FuzzPlot` (in
  `cmd/plot`) charts a CSV or JSON Lines and lists it, and lists it read
  as JSON Lines, and `FuzzWriteMarkdown` (in
  `table`) writes a cell as Markdown; each checks no panic and the promises
  a label carries -- no control character reaching a terminal, UTF-8
  throughout, well-formed SVG, a chart within its width, a Markdown row a
  cell a column -- not merely "no error". `go test` replays the seeds and `testdata/fuzz/`, which holds
  only inputs that once broke a target, committed with the fix: under a
  second, so the everyday run stays quick. What a search keeps -- every
  input that reached new code, hundreds -- is committed apart, in
  `testdata/corpus/`, which `go test` does not read: `make fuzz` starts
  from it, `make fuzz-save` adds to it, and `make fuzz-replay` runs it all
  through each target's check, as `TestFuzzCorpus`, in seconds -- for
  after a change likelier than most to break an edge case. `go test -fuzz
  -fuzztime 1x` is not a replay: it stops before reaching what it holds,
  and passed an input planted there to fail. A seed for each hostile case is what makes the replay worth having:
  one holding `\x1b` as text in DOT, which has no such escape, was no
  escape at all, and one with a byte that is not UTF-8 was refused whole,
  before any label was drawn. The text drawing keeps escapes out twice --
  `printable`, and `set` cell by cell -- and drops a control character for
  taking no cell, so breaking `hidden` alone does not show in it.

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

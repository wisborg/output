# output

Go packages for producing program output in more than one shape: aligned
text, CSV, JSON or YAML, chosen when the program runs — and, while the program
is still working, live progress on the terminal.

`table` builds tabular data and renders it as aligned text.

```go
import "github.com/wisborg/output/table"

t := table.New(
    table.Column{Header: "clip"},
    table.Column{Header: "offset", Align: table.Right, Format: "%+.2fs"},
    table.Column{Header: "score", Align: table.Right, Format: "%.3f"},
)
t.MustAppend("corner_1", 2.70, 0.821)
t.MustAppend("corner_3", 2.75, 0.366)

fmt.Print(t)
```

```
clip       offset   score
-------------------------
corner_1   +2.70s   0.821
corner_3   +2.75s   0.366
```

`Style{Frame: true}` draws it in the style of the `mysql` client;
`Style{Multiline: true}` gives each line of a multi-line cell its own row;
`Style{Spacing: n}` sets the gap between columns.

The same table writes CSV, over the same rows:

```go
t.WriteCSV(os.Stdout, table.CSVStyle{})
```

```
clip,offset,score
corner_1,+2.70s,0.821
corner_3,+2.75s,0.366
```

`Align` and `MaxWidth` do not apply there — padding and truncation are
concessions to a fixed-width display, and silently shortening a value on its
way into a file someone will compute with is how you lose it. `Format` does
apply, because that is you saying how the value should be written. Separators
are skipped, and an empty table still writes its header, where the text
renderer writes nothing: a program reading CSV usually needs that line.

## Choosing the format when the program runs

The root package writes one result in whichever format was asked for. The
premise is that the program builds **two** representations of that result and
lets the format choose between them:

```go
import "github.com/wisborg/output"

doc := output.Document{
    Data:  results, // the object: for JSON and YAML
    Table: summary, // the simplified table: for text and CSV
}
if err := doc.Write(os.Stdout, format); err != nil {
    return err
}
```

They are built independently on purpose. Deriving one from the other forces
the richer shape through the poorer one and makes both worse: JSON grows
stringly-typed cells and pre-formatted numbers, while the table grows columns
nobody wanted to read on a terminal. Writing both is a few lines in the
program that has the data, and each comes out shaped for its reader — the
object can carry the checksum and the detector settings, the table can show
three columns and stop.

A `Document` only needs the representation the chosen format uses: JSON of a
document with no `Table` is fine, CSV of one with no `Data` is fine. Asking
for a format whose representation is missing is an error naming the format —
`ErrNoData` or `ErrNoTable` — and nothing is written. Nothing is written on
any other failure either: whatever the format, the output is either empty or
a whole document ending in exactly one newline. A table with **no rows**
is not a missing table: it is a supplied answer that happens to be empty, and
it keeps `table`'s own rules, so text writes nothing and CSV writes its
header.

`Format` is a `flag.Value`, so the flag is one line and an unknown name is
refused where the user typed it rather than quietly falling back to text:

```go
format := output.Text // the default; the zero Format is Text
flag.Var(&format, "format", "output format: text, csv, json or yaml")
```

`text`, `csv`, `json` and `yaml`, case-insensitively, plus `table` for text
and `yml` for YAML. `Format(99)`, from a cast or a decoded config file, is an
error too: there is no `default:` arm falling through to text, because a
program that prints a table when its caller asked for JSON has produced
output that cannot be parsed and no message saying why.

### JSON and YAML

`WriteJSON` and `WriteYAML` are the same encoders without the `Document`, for
a program that has only the object. Both end their output with exactly one
newline, as text and CSV do.

JSON is indented two spaces by default, and **does not escape HTML**, which
inverts `encoding/json`'s default. That default is right when the JSON is
embedded in a web page and wrong here: a URL in a CLI's output should read
`https://x/?a&b`, not `https://x/?a\u0026b`, which stays wrong when the
reader copies it out of their terminal. `JSONStyle{EscapeHTML: true}` puts it
back. Marshalling errors are wrapped, not flattened, so
`errors.As(err, new(*json.UnsupportedTypeError))` still reaches the type that
could not be encoded.

YAML is indented two spaces, against the library's default of four, and gets
no `---` marker for a single document. `YAMLStyle{Indent: n}` takes 2 to 9,
which is the emitter's own range: it *resets* anything outside that to 2
rather than clamping to the nearer bound, so `Indent: 10` would produce
output identical to never having set it. That is refused with an error naming
the value and the range, rather than being clamped to 9 — a quieter version
of the same surprise.

`WriteYAML` writes all of a document or none of it. The encoder streams, so a
value it rejects part-way through would otherwise leave tens of kilobytes of
truncated YAML on the writer; the document is rendered into a buffer first
and copied out only once it is known to be good. That keeps the invariant
above true for every format instead of true for three of them.

Two things about YAML are worth knowing before you offer it as a format:

- The library **panics** on a type it cannot marshal — a channel or a
  function — where `encoding/json` returns an error. `WriteYAML` recovers
  that one panic and returns an error naming the type. The recovery is
  narrow: any other panic is re-raised, because anything else is a bug in the
  encoder and swallowing it would hide it.
- A **cyclic value still takes the process down**, and cannot be fixed here.
  The encoder follows pointers with no visited set, so a value pointing back
  at itself is encoded again at every level, for ever. It does not fail
  quickly: it spins, emitting an ever-deeper nesting of the same data and
  consuming CPU and memory without bound, until the buffer cannot grow or the
  recursion overflows the stack — and a stack overflow in Go is fatal, not a
  panic, not recoverable. `encoding/json` detects cycles and reports them as
  errors, so the same `Document` that writes as JSON can take the program
  down as YAML. Break cycles before handing data to any encoder.

Both encoders sort mapping keys their own way — `encoding/json` alphabetically
for maps and in declaration order for structs, the YAML library likewise —
and this package adds no knob for that.

## Three decisions worth knowing about

**Cells are stored as you pass them, and formatted only when rendered.** An
`int` stays an `int` until there is a table to draw. That is what allows a
column's alignment, format verb or width limit to be changed after its rows
are already in place — the thing you most often want, because you learn what
a column holds by looking at it:

```go
t.Columns[1].Align = table.Right
```

It is also what leaves room for renderers other than text. CSV shares this row
model exactly. JSON and YAML deliberately do not: they are far more flexible
than a grid of cells, so forcing them through a row-and-column model would
make both worse. They read a separate data source instead — see below — and
are never derived from a table.

**Column widths are measured in terminal columns, not bytes or runes.** These
are three different numbers. `len("café")` is 5 or 6 depending on whether the
`é` is precomposed; `utf8.RuneCountInString("日本語")` is 3 where the terminal
uses 6; an emoji with a skin-tone modifier is two runes and one glyph. Getting
this wrong produces a table that looks correct until someone's data is not
ASCII. The measure is `Table.Width`, defaulting to
[`runewidth.StringWidth`](https://github.com/mattn/go-runewidth), and it is a
field you can replace:

```go
t.Width = utf8.RuneCountInString // fine for ASCII and precomposed Latin
```

**An empty table renders as nothing** — not a lone header over blank space,
which reads as data that failed to load. Only the caller knows what "no rows"
means for them, so only the caller should say it.

## Status

Text and CSV in `table`; text, CSV, JSON and YAML through `output.Document`.

JSON and YAML are still not *table* renderers, and that is the same decision
as before rather than a reversal of it: nothing converts a table into an
object, there is no row accessor for a converter to use, and asking a
`Document` for JSON reads `Data` and never looks at `Table`. What has been
added is the other data source that argument always implied, and the
dispatch that picks between the two.

The API is not yet frozen.

## Dependencies

Two, both compatible with this project's Apache-2.0 licence:

- [`mattn/go-runewidth`](https://github.com/mattn/go-runewidth) (MIT), for
  display-width measurement, which in turn uses
  [`clipperhouse/uax29`](https://github.com/clipperhouse/uax29) (MIT).
- [`go.yaml.in/yaml/v3`](https://github.com/yaml/go-yaml) (**MIT and
  Apache-2.0** — the eight files ported from libyaml are MIT, the rest is
  Apache-2.0, and there is a NOTICE file), for YAML. This is the YAML
  organisation's maintained fork of `gopkg.in/yaml.v3`, which was archived in
  April 2025. It has no dependencies of its own.

`table` does not import the YAML library, so a program that only builds
tables does not link it.

## Licence

Apache License 2.0 — see [LICENSE](LICENSE).

## progress

`progress` puts one or more bars at the bottom of a terminal and lets ordinary
output scroll above them, so a program can log and show progress at once.

```go
import "github.com/wisborg/output/progress"

d := progress.New(os.Stderr, progress.Options{})
defer d.Stop()

bar := d.Bar(progress.BarSpec{Label: "rendering", Total: 10800, Unit: "frames"})
for i := 0; i < 10800; i++ {
    render(i)
    bar.Set(int64(i + 1))
}
```

```
info  merged 5 files into one activity
warn  clip-0043 has no GPS fixes
rendering  ▕███████████████████████▍                 ▏   56%  6100/10800 frames  7375/s  ~1m02s left
encoding   ▕████████▏                                ▏   19%  2100/10800 frames  3688/s  ~2m21s left
```

**A `Display` is an `io.Writer`.** That is the whole mechanism for the two
living together: a write erases the bars, emits the line, and redraws beneath
it. Any logger over an `io.Writer` composes with it, which is why this package
contains no logger of its own.

```go
log := slog.New(slog.NewTextHandler(d, nil))
```

**It degrades on its own.** When the writer is not a terminal — redirected,
piped, or a platform it cannot ask — it emits plain periodic lines with no
escape sequences at all, so `program 2>log` collects a readable log rather
than a file full of cursor movements. A caller does not branch on where its
output is going.

```
rendering 44% 4800/10800 frames 7482/s
encoding 22% 2300/10800 frames 3585/s
```

**The fill can be coloured.** A `Palette` chooses between no colour (the zero
value), one colour, or a gradient across the trough.

```go
d := progress.New(os.Stderr, progress.Options{Palette: progress.DefaultGradient()})

// or
progress.SolidPalette(progress.RGB{R: 0x22, G: 0xC5, B: 0x5E})
progress.GradientPalette(from, to)
```

Only the fill is coloured — the brackets, the label and the numbers are left in
the terminal's own foreground, which is what its user chose for reading. Colour
is emitted only on a live display whose terminal will take it: never in plain
mode, never when `NO_COLOR` is set or `TERM=dumb`, and 24-bit sequences only
where `COLORTERM` claims them, falling back to the 256-colour palette
everywhere else. So it is safe to ask for unconditionally.

**The trough is a fixed width.** Each field's room is reserved before the run
starts — including for the rate and the estimate, which only appear once there
is enough work to measure them — so the bar does not shrink as the counts gain
a digit or lurch when those fields arrive. A width that turns out to be too
small grows once and never shrinks back, so the trough can narrow but never
oscillate.

**Bars never wrap.** A live line that exceeded the terminal's width would
occupy two rows while the erase arithmetic counted one, and the display would
then eat the log above it on every redraw. Lines are measured in terminal
columns and a bar with too little room drops fields from the right instead.

A job whose `Total` is unknown reports its count and rate and shows no
percentage, no estimate and no empty trough: a bar that never fills reads as
one that is stuck rather than as a measurement nobody has.

`Bar.Set` is meant to be called once per unit of work — it does not allocate
when it is not redrawing — and a nil `*Display` is usable and does nothing, so
a `--quiet` flag is one decision at construction rather than a condition at
every call site.

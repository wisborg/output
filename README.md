# output

Go packages for producing program output in more than one shape.

`table` is the first: it builds tabular data and renders it as aligned text.

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
model exactly and is the obvious next one. JSON and YAML deliberately do not:
they are far more flexible than a grid of cells, so forcing them through a
row-and-column model would make both worse. They belong to a separate data
source, converted from a table only when someone actually wants that.

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

Text tables and CSV. JSON and YAML are deliberately not here — see above.
The API is not yet frozen.

## Dependencies

One: [`mattn/go-runewidth`](https://github.com/mattn/go-runewidth) (MIT), for
display-width measurement, which in turn uses
[`clipperhouse/uax29`](https://github.com/clipperhouse/uax29) (MIT). Both are
compatible with this project's Apache-2.0 licence.

## Licence

Apache License 2.0 — see [LICENSE](LICENSE).

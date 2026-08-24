# output

Go packages for producing program output in more than one shape. The root
package `output` writes one result as text, CSV, JSON or YAML, chosen when
the program runs; `table` is the tabular half of that — data rendered as
aligned text or written as CSV.

## Building and testing

Pure Go, no cgo, no external tools. The plain commands work — there is no
Makefile or wrapper script here, unlike some of the other repositories on this
machine, because there is nothing to wrap:

```
gofmt -l .        # must print nothing
go vet ./...
go test ./...
```

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
on a channel or a function; that one panic becomes an error, and anything else
is re-panicked because it is an encoder bug. The shim wraps the encode alone,
not the write, so a panic from the caller's `io.Writer` is not caught by it.
`TestWriteYAML_UpstreamStillPanics` is a tripwire on that panic value — a
dependency bump that changes it must fail there, naming the dependency, rather
than turning every unsupported type back into a crash. A **cyclic** value is
encoded again at every level for ever: it spins, emitting an ever-deeper
nesting of the same data and consuming CPU and memory without bound until the
buffer cannot grow or the stack overflows, and a stack overflow is fatal and
unrecoverable in Go. Buffering did not change that; it moved the unbounded
growth from the writer into memory. A reflective pre-walk to detect cycles was
considered and rejected, since it would cost that walk on every write. It is
documented in `WriteYAML` and in the README.

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
  checked this way.
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

# output

Go packages for producing program output in more than one shape. `table` is
the first: tabular data rendered as aligned text or written as CSV.

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

- `table/table.go` — `Table`, `Column`, `Align`, and building operations.
- `table/render.go` — text rendering: `Style`, `Render`, `String`, and the
  unexported `layout` that resolves a table for one style.
- `table/csv.go` — `CSVStyle` and `WriteCSV`, over the same rows.

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

Apache-2.0. Its one dependency, `mattn/go-runewidth`, is MIT, as is that
library's own dependency `clipperhouse/uax29` — both compatible. Keep personal
data and machine-specific paths out of commits, comments and test fixtures.

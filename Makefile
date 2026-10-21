# output -- the standard way to work with this code.
#
# Go packages, and two commands built on them, `plot` and `diagram`. Every
# target is a plain `go` invocation rather than a delegation to a script, as
# in osmbase: the jobs here are short enough that a script would be a second
# spelling of them and nothing else.
#
# `make` on its own lists the targets.

# The commands are built into bin/, not the root as osmbase's is: a binary
# called diagram at the root would be the diagram package's directory, and
# go build writes into a directory of the name it is given.
BINS    := bin/plot bin/diagram
GOFILES := $(shell find . -name '*.go' -not -path './.scratch/*')

# CMD is the command `make run` runs, and ARGS its flags, e.g.
#   make run ARGS="--y close prices.csv"
#   make run CMD=diagram ARGS="cluster.dot"
CMD  ?= plot
ARGS ?=

# PKG and RUN narrow `make test`, e.g.
#   make test PKG=./chart/ RUN=Date
PKG ?=
RUN ?=

.DEFAULT_GOAL := help

## help: list these targets
help:
	@echo "output targets:"
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk -F': ' '{printf "  %-12s %s\n", $$1, $$2}'

## build: compile bin/plot and bin/diagram
build: $(BINS)

$(BINS): bin/%: $(GOFILES) go.mod go.sum
	go build -o $@ ./cmd/$*

## install: put plot and diagram on your PATH, in GOBIN
install:
	go install ./cmd/...

## run: build and run a command, e.g. make run CMD=diagram ARGS="x.dot"
run: build
	./bin/$(CMD) $(ARGS)

# gofmt is asked of the packages' directories rather than of `.`, so a probe
# left in .scratch/ is not taken for the module's code.
## gates: gofmt, vet, test and deps -- what must pass before a commit
gates:
	@printf 'gofmt: '
	@out=$$(gofmt -l $$(go list -f '{{.Dir}}' ./...)); \
	  if [ -n "$$out" ]; then echo "FAILED"; echo "$$out"; exit 1; else echo "clean"; fi
	@printf 'vet:   '
	@if go vet ./... >/dev/null 2>&1; then echo "clean"; else echo "FAILED"; go vet ./...; exit 1; fi
	@printf 'test:  '
	@if go test ./... >/dev/null 2>&1; then echo "all packages ok"; else echo "FAILED"; go test ./...; exit 1; fi
	@printf 'deps:  '
	@if $(MAKE) -s deps >/dev/null 2>&1; then echo "layered"; else echo "FAILED"; $(MAKE) -s deps; exit 1; fi

## test: run the tests, or narrow with PKG= and RUN=
test:
	go test $(if $(PKG),$(PKG),./...) $(if $(RUN),-run $(RUN),) -count=1

# CI runs the tests this way; see .github/workflows/ci.yml.
## race: the tests under the race detector, as CI runs them
race:
	go test -race ./... -count=1

## cover: test coverage per package
cover:
	go test ./... -cover

## tidy: go mod tidy, then show what the module requires
tidy:
	go mod tidy
	@echo "--- requires ---"
	@go list -m all | tail -n +2

# The module's packages are layered so that a program links only what it
# uses, and that is a promise to the programs depending on them -- whose
# NOTICE files are kept by hand -- not an accident of what each happens to
# import today. CLAUDE.md gives the reasons; this checks them, against what
# each package actually compiles in:
#
#   chart/terminal,
#   internal/memory         the standard library only
#   chart                   go-runewidth and uax29, to cut titles to columns
#   diagram, diagram/dot,
#   diagram/svg             go-runewidth and uax29, to measure labels
#   internal/canvas         golang.org/x/image, and x/text beneath it
#   chart/picture,
#   diagram/picture         x/image, and their model's go-runewidth
#   table, progress         go-runewidth and uax29, and never YAML or x/image
#   the root package        adds YAML, and never x/image
#
# Each group is checked against an ALLOWLIST of the modules it may compile
# in, by module path, so a new dependency anywhere fails here until this
# target names it -- a list of modules each group must not have let any
# other through, x/text into table, say. And every library package is
# checked against an allowlist of the module's own packages it may
# compile in: progress, table and chart none but their own; diagram
# internal/memory, which words a refusal's estimate of the memory it would
# take; the root package table; the picture packages their model, the
# canvas and internal/memory; dot and svg the diagram. Checked by module alone, chart importing
# internal/cli, or diagram importing chart, passed: those are the same
# module.
## deps: check each package compiles in only the modules it is meant to
deps:
	@fail=0; \
	  rw='github.com/mattn/go-runewidth|github.com/clipperhouse/uax29/v2'; \
	  img='golang.org/x/image|golang.org/x/text'; \
	  check() { \
	    bad=$$(go list -deps -f '{{with .Module}}{{.Path}}{{end}}' $$1 | sort -u | grep -v '^github.com/wisborg/output$$' | grep -vxE "$$2" || true); \
	    if [ -n "$$bad" ]; then echo "deps: FAILED -- $$1 compiles in:"; echo "$$bad" | sed 's/^/  /'; fail=1; fi; \
	  }; \
	  own() { \
	    bad=$$(go list -deps $$1 | grep '^github.com/wisborg/output' | grep -vxE "$$2" || true); \
	    if [ -n "$$bad" ]; then echo "deps: FAILED -- $$1 imports:"; echo "$$bad" | sed 's/^/  /'; fail=1; fi; \
	  }; \
	  check "./chart/terminal ./internal/memory ./internal/fuzzcorpus" ''; \
	  check "./chart ./diagram ./diagram/dot ./diagram/svg" "$$rw"; \
	  check "./internal/canvas" "$$img"; \
	  check "./chart/picture ./diagram/picture" "$$rw|$$img"; \
	  check "./table ./progress" "$$rw"; \
	  check "." "$$rw|go.yaml.in/yaml/v3"; \
	  check "./cmd/plot ./cmd/diagram" "$$rw|$$img"; \
	  m='github.com/wisborg/output'; \
	  own "." "$$m|$$m/table"; \
	  own "./table" "$$m/table"; \
	  own "./progress" "$$m/progress"; \
	  own "./chart" "$$m/chart"; \
	  own "./chart/terminal" "$$m/chart/terminal"; \
	  own "./chart/picture" "$$m/chart/picture|$$m/chart|$$m/internal/canvas|$$m/internal/memory"; \
	  own "./internal/canvas" "$$m/internal/canvas"; \
	  own "./internal/cli" "$$m/internal/cli"; \
	  own "./internal/memory" "$$m/internal/memory"; \
	  own "./internal/fuzzcorpus" "$$m/internal/fuzzcorpus"; \
	  own "./diagram" "$$m/diagram|$$m/internal/memory"; \
	  own "./diagram/dot" "$$m/diagram/dot|$$m/diagram|$$m/internal/memory"; \
	  own "./diagram/svg" "$$m/diagram/svg|$$m/diagram|$$m/internal/memory"; \
	  own "./diagram/picture" "$$m/diagram/picture|$$m/diagram|$$m/internal/canvas|$$m/internal/memory"; \
	  [ $$fail -eq 0 ] && echo "deps: chart/terminal is the standard library alone, chart and diagram add only go-runewidth, x/image only in the picture packages, YAML only in the root package"; \
	  exit $$fail

# FUZZTIME is how long `make fuzz` searches each target, e.g.
#   make fuzz FUZZTIME=10m
FUZZTIME ?= 1m

# The fuzz targets read what a stranger may write -- DOT, and a CSV for
# plot -- and draw it every way a command does. `go test` replays only
# their seeds and testdata/fuzz/, which hold the inputs that once broke
# something: a second at most, so the everyday run stays quick.
#
# A search keeps the inputs that reach code no earlier one did, in Go's
# build cache, which no other machine has. testdata/corpus/ is where they
# are kept for good, outside what go test reads: `make fuzz` starts from
# it, so a search continues from the last, and `make fuzz-save` adds what a
# search found, to be committed when it is worth keeping. `make
# fuzz-replay` runs the whole corpus once through the targets' checks, in
# seconds -- for after a change likelier than most to break an edge case. An input that breaks a target
# is written to testdata/fuzz/ and committed with its fix, replayed for
# good. go test fuzzes one target of one package at a time, hence a loop.
FUZZ      := diagram/dot:FuzzParse cmd/plot:FuzzPlot
FUZZCACHE  = $(shell go env GOCACHE)/fuzz/github.com/wisborg/output

## fuzz: search for DOT and CSV that break a command, FUZZTIME each (1m)
fuzz: fuzz-load
	@for t in $(FUZZ); do pkg=$${t%%:*}; fn=$${t##*:}; \
	  go test -run '^$$' -fuzz "^$$fn\$$" -fuzztime $(FUZZTIME) ./$$pkg || exit 1; done

# fuzz-replay runs each package's TestFuzzCorpus, which go test skips
# unless FUZZ_CORPUS is set. `go test -fuzz -fuzztime 1x` looked like the
# way, and replayed nothing: it stops before reaching what the cache holds,
# and an input planted there to fail went through.
## fuzz-replay: run every input in testdata/corpus once, in seconds
fuzz-replay:
	FUZZ_CORPUS=1 go test -count=1 -run '^TestFuzzCorpus$$' $(foreach t,$(FUZZ),./$(firstword $(subst :, ,$(t))))

## fuzz-save: add what searches found to testdata/corpus, to commit
fuzz-save:
	@for t in $(FUZZ); do pkg=$${t%%:*}; fn=$${t##*:}; \
	  mkdir -p $$pkg/testdata/corpus/$$fn; \
	  before=$$(ls $$pkg/testdata/corpus/$$fn | wc -l); \
	  [ -d "$(FUZZCACHE)/$$pkg/$$fn" ] && cp -n "$(FUZZCACHE)/$$pkg/$$fn"/* $$pkg/testdata/corpus/$$fn/ 2>/dev/null; \
	  echo "$$fn: $$(( $$(ls $$pkg/testdata/corpus/$$fn | wc -l) - before )) new, in $$pkg/testdata/corpus/$$fn"; done

# fuzz-load puts testdata/corpus where go test -fuzz looks for what it has
# kept: a file is named by its content's hash, so one already there is the
# same input, and is left.
fuzz-load:
	@for t in $(FUZZ); do pkg=$${t%%:*}; fn=$${t##*:}; \
	  if [ -d $$pkg/testdata/corpus/$$fn ]; then \
	    mkdir -p "$(FUZZCACHE)/$$pkg/$$fn"; \
	    cp -n $$pkg/testdata/corpus/$$fn/* "$(FUZZCACHE)/$$pkg/$$fn/" 2>/dev/null || true; \
	  fi; done

## clean: remove bin/, and empty .scratch/
clean:
	rm -rf bin
	@find .scratch -mindepth 1 -maxdepth 1 ! -name env -exec rm -rf {} + 2>/dev/null || true
	@echo "cleaned"

.PHONY: help build install run gates test race cover tidy deps fuzz fuzz-replay fuzz-save fuzz-load clean

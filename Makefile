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
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk -F': ' '{printf "  %-8s %s\n", $$1, $$2}'

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
## gates: gofmt, vet and test -- what must pass before a commit
gates:
	@printf 'gofmt: '
	@out=$$(gofmt -l $$(go list -f '{{.Dir}}' ./...)); \
	  if [ -n "$$out" ]; then echo "FAILED"; echo "$$out"; exit 1; else echo "clean"; fi
	@printf 'vet:   '
	@if go vet ./... >/dev/null 2>&1; then echo "clean"; else echo "FAILED"; go vet ./...; exit 1; fi
	@printf 'test:  '
	@if go test ./... >/dev/null 2>&1; then echo "all packages ok"; else echo "FAILED"; go test ./...; exit 1; fi

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
#   chart, chart/terminal   the standard library only
#   diagram, diagram/dot,
#   diagram/svg             go-runewidth and uax29, to measure labels
#   chart/picture,
#   internal/canvas         add golang.org/x/image, and x/text beneath it
#   diagram/picture         x/image, and diagram's go-runewidth
#   table, progress         go-runewidth and uax29, and never YAML or x/image
#   the root package        adds YAML, and never x/image
#
# A package is outside the standard library when its first path element has
# a dot in it: go.yaml.in as much as github.com.
## deps: check each package compiles in only the modules it is meant to
deps:
	@fail=0; \
	  check() { \
	    pkgs=$$1; banned=$$2; \
	    bad=$$(go list -deps $$pkgs | grep -E '^[^/]*\.[^/]*/' | grep -v '^github.com/wisborg/output' | grep -E "$$banned" || true); \
	    if [ -n "$$bad" ]; then echo "deps: FAILED -- $$pkgs compiles in:"; echo "$$bad" | sed 's/^/  /'; fail=1; fi; \
	  }; \
	  check "./chart ./chart/terminal" '.'; \
	  check "./diagram ./diagram/dot ./diagram/svg" '^(go.yaml.in|golang.org/x)/'; \
	  check "./chart/picture ./internal/canvas" '^(github.com/mattn|github.com/clipperhouse|go.yaml.in)/'; \
	  check "./diagram/picture" '^go.yaml.in/'; \
	  check "./table ./progress" '^(go.yaml.in|golang.org/x/image)/'; \
	  check "." '^golang.org/x/image/'; \
	  [ $$fail -eq 0 ] && echo "deps: chart is the standard library alone, diagram adds only go-runewidth, x/image only in the picture packages, YAML only in the root package"; \
	  exit $$fail

## clean: remove bin/, and empty .scratch/
clean:
	rm -rf bin
	@find .scratch -mindepth 1 -maxdepth 1 ! -name env -exec rm -rf {} + 2>/dev/null || true
	@echo "cleaned"

.PHONY: help build install run gates test race cover tidy deps clean

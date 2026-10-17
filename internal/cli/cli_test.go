package cli

import (
	"errors"
	"flag"
	"io"
	"slices"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, c := range []struct {
		args []string
		out  string
		rest []string
	}{
		{[]string{"a.dot"}, "", []string{"a.dot"}},
		{[]string{"-out", "x.png", "a.dot"}, "x.png", []string{"a.dot"}},
		{[]string{"a.dot", "-out", "x.png"}, "x.png", []string{"a.dot"}},
		{[]string{"a.dot", "-out", "x.png", "b.dot"}, "x.png", []string{"a.dot", "b.dot"}},
		{[]string{"-", "-out=x.png"}, "x.png", []string{"-"}},
		{[]string{"-out", "x.png", "--", "-a.dot", "-out"}, "x.png", []string{"-a.dot", "-out"}},
		{[]string{"a.dot", "--", "-b.dot"}, "", []string{"a.dot", "-b.dot"}},
		{[]string{"--"}, "", nil},
		{nil, "", nil},
		// A "--" that is a flag's value is the value, and the flags after
		// the file are still flags.
		{[]string{"-out", "--", "a.dot", "-out", "y.png"}, "y.png", []string{"a.dot"}},
		{[]string{"-out", "--", "a.dot"}, "--", []string{"a.dot"}},
		{[]string{"-v", "--", "-a.dot"}, "", []string{"-a.dot"}},
	} {
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		out := fs.String("out", "", "")
		fs.Bool("v", false, "")
		rest, err := Parse(fs, c.args)
		if err != nil {
			t.Errorf("%q: %v", c.args, err)
			continue
		}
		if *out != c.out || !slices.Equal(rest, c.rest) {
			t.Errorf("%q: -out %q and %q, want %q and %q", c.args, *out, rest, c.out, c.rest)
		}
	}
}

// Help asked for after the file is help, not a second file.
func TestParseHelpAfterTheFile(t *testing.T) {
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if _, err := Parse(fs, []string{"a.dot", "--help"}); err != flag.ErrHelp {
		t.Errorf("%v, want flag.ErrHelp", err)
	}
}

// The help spells each flag with two dashes, as the commands read it, with
// its kind and any default, the descriptions aligned.
func TestPrintDefaults(t *testing.T) {
	var buf strings.Builder
	fs := flag.NewFlagSet("diagram", flag.ContinueOnError)
	fs.SetOutput(&buf)
	fs.String("out", "", "write it to this `file`")
	fs.String("theme", "auto", "the palette")
	fs.Float64("text-size", 16, "the size")
	fs.Bool("list", false, "list")
	fs.Func("font", "a font", func(string) error { return nil })
	PrintDefaults(fs)
	want := `  -h, --help              help for diagram
      --font string       a font
      --list              list
      --out file          write it to this file
      --text-size float   the size (default 16)
      --theme string      the palette (default "auto")
`
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

// A mistake is one line naming the flag as it is spelled in the help, and
// nothing is written: the flag package wrote the whole help beside it, and
// named --bogus as -bogus.
func TestParseMistakes(t *testing.T) {
	for args, want := range map[string]string{
		"--bogus":   "flag provided but not defined: --bogus",
		"--out":     "flag needs an argument: --out",
		"--n=x":     `invalid value "x" for flag --n`,
		"-bogus a":  "flag provided but not defined: --bogus",
		"a --bogus": "flag provided but not defined: --bogus",
	} {
		var b strings.Builder
		fs := flag.NewFlagSet("t", flag.ContinueOnError)
		fs.SetOutput(&b)
		fs.String("out", "", "")
		fs.Int("n", 0, "")
		_, err := Parse(fs, strings.Fields(args))
		var ue *UsageError
		if !errors.As(err, &ue) || !strings.HasPrefix(err.Error(), want) {
			t.Errorf("%q: %v, want a UsageError saying %q", args, err, want)
		}
		if b.Len() != 0 {
			t.Errorf("%q: wrote %q", args, b.String())
		}
	}
}

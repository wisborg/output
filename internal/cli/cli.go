// Package cli holds what the commands share in reading their command line.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// UsageError is a mistake in the command line: a flag the command does not
// have, one without its value, a value it cannot read. Run reports one in a
// line and exits 2, as a command does for a mistake in how it was called.
type UsageError struct{ msg string }

func (e *UsageError) Error() string { return e.msg }

// Usagef is a UsageError saying what format and args say: a value of a
// flag the command reads itself and refuses.
func Usagef(format string, args ...any) error {
	return &UsageError{fmt.Sprintf(format, args...)}
}

// dashes spells a flag in the flag package's messages as the command's
// help does, "--width" for "-width": the message once named a flag
// spelled as nobody had typed it.
var dashes = regexp.MustCompile(`(defined:|argument:|for flag) -([^-\s])`)

// Parse parses args with fs, taking flags wherever they are, before the
// file or after it, and returns the arguments that are not flags, in order.
//
// The flag package alone stops at the first argument that is not a flag,
// so "plot data.csv -y price" would read "-y" and "price" as two more
// files. Everything after "--" is an argument, whatever it looks like, so
// a file whose name begins with "-" can still be given -- but a "--" that
// is a flag's value, --title --, is the value.
//
// It writes nothing. Asked for help it returns flag.ErrHelp, for the
// command to print its help where it is asked for, on standard output; a
// mistake is a *UsageError, without the full help the flag package prints
// beside one, which buried the mistake under fifty lines.
func Parse(fs *flag.FlagSet, args []string) ([]string, error) {
	out, usage := fs.Output(), fs.Usage
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}
	defer func() { fs.SetOutput(out); fs.Usage = usage }()
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, &UsageError{dashes.ReplaceAllString(err.Error(), "$1 --$2")}
		}
		left := fs.Args()
		if len(left) == 0 {
			return rest, nil
		}
		// fs.Parse stopped either at "--", which it consumed, or at an
		// argument that is not a flag. The argument before left is a
		// "--" it stopped at only if it is not the value of the flag
		// before it.
		used := args[:len(args)-len(left)]
		if n := len(used); n > 0 && used[n-1] == "--" && !(n > 1 && takesValue(fs, used[n-2])) {
			return append(rest, left...), nil
		}
		rest = append(rest, left[0])
		args = left[1:]
	}
}

// takesValue says arg is a flag of fs's that takes the argument after it as
// its value: not a boolean, and not written with its value, --x=1.
func takesValue(fs *flag.FlagSet, arg string) bool {
	name := strings.TrimLeft(arg, "-")
	if name == arg || name == "" || strings.Contains(name, "=") {
		return false
	}
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !b.IsBoolFlag()
}

// Exit ends a command run returned err from: 0 for help asked for, 2 for a
// mistake in the command line, with a pointer to the help, 1 for anything
// else, each reported on standard error after the command's name.
func Exit(name string, err error) {
	var usage *UsageError
	switch {
	case err == nil || errors.Is(err, flag.ErrHelp):
		os.Exit(0)
	case errors.As(err, &usage):
		fmt.Fprintf(os.Stderr, "%s: %v\nRun %s --help for its flags.\n", name, err, name)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
	os.Exit(1)
}

// PrintDefaults writes fs's flags as fitdash and other GNU-style commands
// list theirs: "--out string" and its description on one line, the
// descriptions aligned, -h and --help first among them.
//
// The flag package's own listing spells each flag with one dash, though
// it reads two as well; the flags are the same, only their spelling in
// the help is. pflag would list them so but would read -out as -o -u -t,
// breaking every command line written with one dash.
func PrintDefaults(fs *flag.FlagSet) {
	type line struct{ name, usage string }
	lines := []line{{"-h, --help", "help for " + fs.Name()}}
	fs.VisitAll(func(f *flag.Flag) {
		kind, usage := flag.UnquoteUsage(f)
		switch kind {
		case "value":
			kind = "string" // a flag.Func, repeatable: a string each time
		case "float64":
			kind = "float"
		}
		name := "    --" + f.Name
		if kind != "" {
			name += " " + kind
		}
		if !zero(f.DefValue) {
			if kind == "string" {
				usage += fmt.Sprintf(" (default %q)", f.DefValue)
			} else {
				usage += fmt.Sprintf(" (default %s)", f.DefValue)
			}
		}
		lines = append(lines, line{name, usage})
	})
	width := 0
	for _, l := range lines {
		width = max(width, len(l.name))
	}
	for _, l := range lines {
		fmt.Fprintf(fs.Output(), "  %-*s   %s\n", width, l.name, l.usage)
	}
}

func zero(v string) bool { return v == "" || v == "0" || v == "false" || v == "[]" }

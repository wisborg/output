// Package cli holds what the commands share in reading their command line.
package cli

import (
	"flag"
	"fmt"
)

// Parse parses args with fs, taking flags wherever they are, before the
// file or after it, and returns the arguments that are not flags, in order.
//
// The flag package alone stops at the first argument that is not a flag,
// so "plot data.csv -y price" would read "-y" and "price" as two more
// files. Everything after "--" is an argument, whatever it looks like, so
// a file whose name begins with "-" can still be given.
func Parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		left := fs.Args()
		if len(left) == 0 {
			return rest, nil
		}
		// fs.Parse stopped either at "--", which it consumed, or at an
		// argument that is not a flag.
		if len(args) > len(left) && args[len(args)-len(left)-1] == "--" {
			return append(rest, left...), nil
		}
		rest = append(rest, left[0])
		args = left[1:]
	}
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

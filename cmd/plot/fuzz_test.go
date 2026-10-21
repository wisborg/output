package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// FuzzPlot reads a file as a stranger may write it and charts whatever
// reads, in braille 60 by 16 characters, smoothed and not, and lists its
// columns. A CSV is often someone else's, so none of it may panic, and
// what is written must keep what the command promises of a heading or a
// cell: standard output and standard error hold no control character but
// a line's end and are UTF-8, and no line of the chart is wider than 60
// columns.
//
// go test runs the seeds and testdata/fuzz alone; make fuzz searches.
func FuzzPlot(f *testing.F) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.csv"))
	if err != nil {
		f.Fatal(err)
	}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(string(src))
	}
	for _, s := range []string{
		"date,close,volume\n2024-01-02,10.5,100\n2024-01-03,11,\n2024-01-04,NA,120\n",
		"x;y\n1,5;2,25\n2,5;3,75\n",
		"t\ta\tb\n1\t2\t\n2\t\t4\n3\t5\t6\n",
		"\"a,b\",c\n\"1\",2\n3,\"4\"\n",
		"when,v\n03/04/2024,1\n05/04/2024,2\n",
		"ts,v\n2024-03-05 14:30:15 +0100,1\n2024-03-05 14:31:15 +0100,1e308\n",
		"n,\x1b]0;t\x07\n1,2\n2,-3\n",
		"日本語,値\n1,2\n2,3\n",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data string) {
		if len(data) > 4096 {
			return
		}
		for _, args := range [][]string{
			{"--terminal", "braille", "--width", "60", "--height", "16"},
			{"--terminal", "braille", "--width", "60", "--height", "16", "--smooth", "3", "--robust"},
			{"--list"},
		} {
			var out, errOut bytes.Buffer
			_ = run(args, strings.NewReader(data), &out, &errOut, noEnv)
			for _, w := range []*bytes.Buffer{&out, &errOut} {
				s := w.String()
				if !utf8.ValidString(s) || strings.ContainsFunc(s, func(r rune) bool { return r != '\n' && (r < 0x20 || r == 0x7f || r >= 0x80 && r < 0xa0) }) {
					t.Fatalf("%v wrote text not UTF-8 or holding a control character: %q", args, s)
				}
			}
			// In a CJK locale runewidth counts the axes' box-drawing
			// characters two columns, which the rows are not laid out by.
			if args[0] == "--list" || runewidth.DefaultCondition.EastAsianWidth {
				continue
			}
			for _, l := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
				if w := runewidth.StringWidth(l); w > 60 {
					t.Fatalf("%v drew a line %d columns wide: %q", args, w, l)
				}
			}
		}
	})
}

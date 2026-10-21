// Package fuzzcorpus replays the inputs a fuzz search kept, which are
// committed under a package's testdata/corpus/ rather than testdata/fuzz/,
// so that go test does not read them on every run. It is for tests alone,
// and internal so that it is no API anyone else depends on.
package fuzzcorpus

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Env is the variable that asks for the corpus to be replayed: make
// fuzz-replay sets it.
const Env = "FUZZ_CORPUS"

// Strings is every input in dir, a directory of files as go test -fuzz
// writes them for a target taking one string, by file name. The test is
// skipped unless Env is set, so that an everyday run stays quick; a
// directory that is missing, or a file not in that form, fails it.
func Strings(t *testing.T, dir string) map[string]string {
	t.Helper()
	if os.Getenv(Env) == "" {
		t.Skipf("%s is not set: make fuzz-replay replays %s", Env, dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatalf("%s holds no inputs", dir)
	}
	in := map[string]string{}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s, err := parse(string(b))
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		in[e.Name()] = s
	}
	return in
}

// parse reads one file of go test -fuzz's encoding, v1, holding a string.
func parse(b string) (string, error) {
	lines := strings.Split(strings.TrimRight(b, "\n"), "\n")
	if len(lines) != 2 || lines[0] != "go test fuzz v1" {
		return "", fmt.Errorf("not one value in go test fuzz v1")
	}
	v, ok := strings.CutPrefix(lines[1], "string(")
	if !ok || !strings.HasSuffix(v, ")") {
		return "", fmt.Errorf("not a string: %.40q", lines[1])
	}
	return strconv.Unquote(strings.TrimSuffix(v, ")"))
}

package main

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

const regions = `digraph {
	rows = 2
	subgraph cluster_east { label = "us-east-1"; style = lane; columns = 2; dbA; dbB }
	subgraph cluster_west { label = "us-west-2"; style = lane; columns = 2; dbC }
	dbA -> dbB
	dbA -> dbC
}`

func draw(t *testing.T, src string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = run(args, strings.NewReader(src), &out, &errOut, noEnv)
	return out.String(), errOut.String(), err
}

// Not on a terminal, the diagram is drawn as text.
func TestDiagramDrawsText(t *testing.T) {
	out, _, err := draw(t, regions)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"│ dbA ├─────▶│ dbB │", "┼", "us-west-2"} {
		if !strings.Contains(out, want) {
			t.Errorf("no %q in:\n%s", want, out)
		}
	}
}

// -out writes the format its name ends in.
func TestDiagramWritesFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"d.png", "d.svg", "d.txt"} {
		path := filepath.Join(dir, name)
		_, stderr, err := draw(t, regions, "-out", path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stderr, path) {
			t.Errorf("stderr %q does not name %s", stderr, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		switch filepath.Ext(name) {
		case ".png":
			if _, err := png.Decode(bytes.NewReader(data)); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		case ".svg":
			if !bytes.HasPrefix(data, []byte("<svg ")) {
				t.Errorf("%s begins %q", name, data[:min(len(data), 20)])
			}
		case ".txt":
			if !bytes.Contains(data, []byte("dbC")) {
				t.Errorf("%s:\n%s", name, data)
			}
		}
	}
}

func TestDiagramRefuses(t *testing.T) {
	for _, c := range []struct {
		src  string
		args []string
		want string
	}{
		{regions, []string{"-out", "d.jpg"}, "name it .png, .svg or .txt"},
		{regions, []string{"-terminal", "braille"}, "use auto, iterm, kitty or text"},
		{regions, []string{"-theme", "pink"}, "-theme"},
		{"digraph {\n a -- b\n}", nil, "standard input: line 2"},
		{"digraph { subgraph cluster_a { columns=1; a [col=1]; b [col=1, row=1] } }", nil, "both pinned"},
	} {
		_, _, err := draw(t, c.src, c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want an error saying %q", c.args, err, c.want)
		}
	}
}

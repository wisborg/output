package output

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	yaml "go.yaml.in/yaml/v3"
)

func yamlOf(t *testing.T, v any, style YAMLStyle) string {
	t.Helper()
	var b strings.Builder
	if err := WriteYAML(&b, v, style); err != nil {
		t.Fatalf("WriteYAML: %v", err)
	}
	return b.String()
}

// TestWriteYAML_IndentsTwoSpacesPerLevel is the archetypal silent bug in
// this package: the YAML library indents four spaces by default, and
// dropping the SetIndent call produces a document that is still valid YAML
// and still parses to exactly the same value. Nothing but the literal
// indentation catches it, so that is what this asserts -- a key at depth n
// preceded by exactly 2n spaces -- rather than a round trip through the
// parser, which would pass either way.
func TestWriteYAML_IndentsTwoSpacesPerLevel(t *testing.T) {
	v := map[string]any{"top": map[string]any{"mid": map[string]any{"deep": "value"}}}
	depth := map[string]int{"top:": 0, "mid:": 1, "deep:": 2}

	assertIndent := func(got string, spaces int) {
		t.Helper()
		seen := 0
		for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
			trimmed := strings.TrimLeft(line, " ")
			indent := len(line) - len(trimmed)
			for key, level := range depth {
				if strings.HasPrefix(trimmed, key) {
					seen++
					if want := spaces * level; indent != want {
						t.Errorf("indent %d: key %q at depth %d has %d leading spaces, want %d", spaces, key, level, indent, want)
					}
				}
			}
		}
		if seen != len(depth) {
			t.Errorf("found %d of the %d keys in:\n%s", seen, len(depth), got)
		}
	}

	assertIndent(yamlOf(t, v, YAMLStyle{}), DefaultYAMLIndent)

	// Every width the emitter honours must come out literally, not just the
	// one this package defaults to.
	for width := minYAMLIndent; width <= maxYAMLIndent; width++ {
		assertIndent(yamlOf(t, v, YAMLStyle{Indent: width}), width)
	}
}

// TestWriteYAML_WholeDocumentIsWritten checks the last key of a mapping big
// enough to fill an encoder buffer. A stream finished without Encoder.Close
// can truncate into something that still looks like a small plausible
// document -- valid YAML, parses fine, missing data at the end -- so the
// assertion has to be about the LAST key rather than about the output being
// well formed.
func TestWriteYAML_WholeDocumentIsWritten(t *testing.T) {
	const n = 2000
	v := map[string]string{}
	for i := 0; i < n; i++ {
		v[fmt.Sprintf("key_%05d", i)] = strings.Repeat("x", 40)
	}

	got := yamlOf(t, v, YAMLStyle{})
	last := fmt.Sprintf("key_%05d:", n-1)
	if !strings.Contains(got, last) {
		t.Errorf("last key %q missing; output is %d bytes ending %q", last, len(got), tail(got, 60))
	}
	if lines := strings.Count(got, "\n"); lines != n {
		t.Errorf("output has %d lines, want %d (one per key)", lines, n)
	}

	var back map[string]string
	if err := yaml.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("output does not parse: %v", err)
	}
	if len(back) != n {
		t.Errorf("parsed %d keys, want %d", len(back), n)
	}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// TestWriteYAML_UnsupportedTypeIsAnErrorNotAPanic is the reason WriteYAML
// has a recover in it at all. The value of a --format flag must not decide
// whether the program crashes.
func TestWriteYAML_UnsupportedTypeIsAnErrorNotAPanic(t *testing.T) {
	type withChannel struct {
		Ch chan int `yaml:"ch"`
	}
	var b strings.Builder
	err := WriteYAML(&b, withChannel{make(chan int)}, YAMLStyle{})
	if err == nil {
		t.Fatal("WriteYAML of a chan field returned no error")
	}
	if !errors.Is(err, errUnsupportedYAMLType) {
		t.Errorf("err = %v, want one wrapping errUnsupportedYAMLType", err)
	}
	if !strings.Contains(err.Error(), "chan int") {
		t.Errorf("error does not name the offending type: %q", err.Error())
	}
}

// TestWriteYAML_CallersStructTagsAreAnErrorNotAPanic is the same promise for
// the other panics the encoder raises over the caller's own value: struct
// tags it cannot follow, and an inlined map with a key a field has. Each
// crashed a program asked for --format yaml where JSON wrote the same value.
// Nothing is written for any of them.
func TestWriteYAML_CallersStructTagsAreAnErrorNotAPanic(t *testing.T) {
	type dup struct {
		A string `yaml:"a"`
		B string `yaml:"a"`
	}
	type inlineString struct {
		S string `yaml:",inline"`
	}
	type inlineIntKeys struct {
		M map[int]string `yaml:",inline"`
	}
	type badFlag struct {
		S string `yaml:"s,sideways"`
	}
	type inlineClash struct {
		A string            `yaml:"a"`
		M map[string]string `yaml:",inline"`
	}
	for _, c := range []struct {
		name string
		v    any
		want string
	}{
		{"duplicate tags", dup{}, "duplicated key 'a'"},
		{"inline string", inlineString{}, "option ,inline"},
		{"inline int keys", inlineIntKeys{}, "option ,inline"},
		{"unknown flag", badFlag{}, "unsupported flag"},
		{"inline clash", inlineClash{A: "x", M: map[string]string{"a": "y"}}, `cannot have key "a"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			var b strings.Builder
			err := WriteYAML(&b, c.v, YAMLStyle{})
			if !errors.Is(err, errUnsupportedYAMLType) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("err = %v, want one wrapping errUnsupportedYAMLType and saying %q", err, c.want)
			}
			if b.Len() != 0 {
				t.Errorf("wrote %q on failing", b.String())
			}
		})
	}
}

type unexportedInner struct{ A int }

// TestWriteYAML_OrdinaryValuesTheEncoderCannotWrite: a struct embedding an
// unexported type, and a nil pointer held in an interface, are values an
// ordinary program writes and encoding/json writes without complaint. The
// YAML encoder panics on both; WriteYAML says so, and what to change.
func TestWriteYAML_OrdinaryValuesTheEncoderCannotWrite(t *testing.T) {
	type outer struct{ unexportedInner }
	type holder struct{ V any }
	for _, c := range []struct {
		name string
		v    any
		want string
	}{
		{"embedded unexported", outer{unexportedInner{1}}, "inline"},
		{"embedded unexported by pointer", &outer{unexportedInner{1}}, "inline"},
		{"nil Format in an interface", holder{V: (*Format)(nil)}, "set the interface itself to nil"},
		{"nil time in a map", map[string]any{"t": (*time.Time)(nil)}, "set the interface itself to nil"},
	} {
		var b strings.Builder
		err := WriteYAML(&b, c.v, YAMLStyle{})
		if !errors.Is(err, errUnsupportedYAMLType) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want an error saying %q", c.name, err, c.want)
		}
		if b.Len() != 0 {
			t.Errorf("%s: wrote %q", c.name, b.String())
		}
	}
	// A nil pointer in a field of pointer type, and at the top, are fine.
	var tp *time.Time
	for _, v := range []any{struct{ T *time.Time }{tp}, (*Format)(nil)} {
		if err := WriteYAML(io.Discard, v, YAMLStyle{}); err != nil {
			t.Errorf("%#v: %v", v, err)
		}
	}
}

// TestWriteYAML_UpstreamStillPanicsOnOrdinaryValues trips if the encoder's
// panics on those values change: their text is all the shim has to know
// them by.
func TestWriteYAML_UpstreamStillPanicsOnOrdinaryValues(t *testing.T) {
	type outer struct{ unexportedInner }
	for _, v := range []any{outer{unexportedInner{1}}, struct{ V any }{(*Format)(nil)}, map[string]any{"t": (*time.Time)(nil)}} {
		func() {
			defer func() {
				r := recover()
				msg := fmt.Sprint(r)
				if r == nil || !callerPanic(msg) || yamlHint(msg) == "" {
					t.Errorf("go.yaml.in/yaml/v3 on %#v: %#v, which WriteYAML's shim no longer knows", v, r)
				}
			}()
			_, _ = yaml.Marshal(v)
		}()
	}
}

// TestWriteYAML_RepanicsAnythingElse pins the narrowness of the recover. A
// shim that swallowed every panic would turn a genuine encoder bug into a
// plausible-looking error, or into no output and no complaint.
func TestWriteYAML_RepanicsAnythingElse(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a panic from inside the marshalled value was swallowed")
		}
		if got, ok := r.(string); !ok || got != "not the encoder" {
			t.Errorf("recovered %#v, want the original panic value", r)
		}
	}()
	// A MarshalYAML that panics stands in for a bug in the encoder: the
	// panic passes through WriteYAML's shim untouched.
	var b strings.Builder
	_ = WriteYAML(&b, panicMarshaler{}, YAMLStyle{})
	t.Fatal("unreachable: WriteYAML returned instead of re-panicking")
}

type panicMarshaler struct{}

func (panicMarshaler) MarshalYAML() (any, error) {
	panic("not the encoder")
}

// TestWriteYAML_UpstreamStillPanics is a tripwire on the dependency, not on
// this package. WriteYAML's shim recognises one panic value: a string with
// the prefix below. If a version bump changes the panic's type or wording,
// the shim silently stops working and every unsupported type becomes a crash
// again -- so assert the upstream behaviour directly, where the failure
// names the dependency instead of appearing as an unexplained crash.
func TestWriteYAML_UpstreamStillPanics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("go.yaml.in/yaml/v3 no longer panics on an unmarshallable type; WriteYAML's recover shim (and its %q prefix) may now be unnecessary", yamlMarshalPanicPrefix)
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("upstream panicked with %T (%#v), not a string; WriteYAML's shim no longer matches", r, r)
		}
		if !strings.HasPrefix(msg, yamlMarshalPanicPrefix) {
			t.Fatalf("upstream panic %q no longer starts with %q; WriteYAML's shim no longer matches", msg, yamlMarshalPanicPrefix)
		}
		if !strings.Contains(msg, "chan int") {
			t.Errorf("upstream panic %q no longer names the type", msg)
		}
	}()
	_, _ = yaml.Marshal(struct {
		Ch chan int
	}{make(chan int)})
}

// TestWriteYAML_UpstreamStillPanicsOnStructTags is the same tripwire for
// yamlCallerPanics: an error value for tags the encoder cannot follow, and a
// string for an inlined map's clash, each starting as the shim expects.
func TestWriteYAML_UpstreamStillPanicsOnStructTags(t *testing.T) {
	type dup struct {
		A string `yaml:"a"`
		B string `yaml:"a"`
	}
	type clash struct {
		A string            `yaml:"a"`
		M map[string]string `yaml:",inline"`
	}
	for _, c := range []struct {
		v       any
		isError bool
	}{{dup{}, true}, {clash{M: map[string]string{"a": "y"}}, false}} {
		func() {
			defer func() {
				r := recover()
				var msg string
				switch v := r.(type) {
				case error:
					msg = v.Error()
					if !c.isError {
						t.Errorf("upstream panicked with the error %q where it raised a string", msg)
					}
				case string:
					msg = v
					if c.isError {
						t.Errorf("upstream panicked with the string %q where it raised an error", msg)
					}
				default:
					t.Fatalf("go.yaml.in/yaml/v3 panicked with %T (%#v) on %T; WriteYAML's shim no longer matches", r, r, c.v)
				}
				if !callerPanic(msg) {
					t.Errorf("go.yaml.in/yaml/v3 panic %q on %T starts with none of %q; WriteYAML's shim no longer matches", msg, c.v, yamlCallerPanics)
				}
			}()
			_, _ = yaml.Marshal(c.v)
		}()
	}
}

func TestWriteYAML_HonoursStructTags(t *testing.T) {
	type v struct {
		SnakeName string `yaml:"snake_name"`
		Skipped   string `yaml:"-"`
	}
	got := yamlOf(t, v{"x", "y"}, YAMLStyle{})
	if !strings.Contains(got, "snake_name: x") {
		t.Errorf("yaml tag not honoured:\n%s", got)
	}
	if strings.Contains(got, "y") {
		t.Errorf("field tagged \"-\" was written:\n%s", got)
	}
}

// TestWriteYAML_NoDocumentMarkerAndOneTrailingNewline covers the shape of
// the whole stream: a lone document needs no "---", and every writer in this
// package ends its output with exactly one newline so that a program can
// print something after it.
func TestWriteYAML_NoDocumentMarkerAndOneTrailingNewline(t *testing.T) {
	for _, v := range []any{
		map[string]int{"a": 1},
		[]string{"a", "b"},
		"scalar",
		42,
		nil,
	} {
		got := yamlOf(t, v, YAMLStyle{})
		if strings.HasPrefix(got, "---") {
			t.Errorf("%v: output starts with a document marker:\n%s", v, got)
		}
		if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
			t.Errorf("%v: output must end in exactly one newline, got %q", v, got)
		}
	}
}

func TestWriteYAML_NilWritesNull(t *testing.T) {
	if got := yamlOf(t, nil, YAMLStyle{}); got != "null\n" {
		t.Errorf("WriteYAML(nil) = %q, want %q", got, "null\n")
	}
}

// TestWriteYAML_TypedNilPointerWritesNull is WriteJSON's
// TestWriteJSON_NilWritesNull typed-nil case, mirrored directly against
// WriteYAML rather than only through Document (see
// TestDocument_TypedNilDataIsAValue): a (*T)(nil) is a value, not an absent
// interface, and null is the accurate rendering of it.
func TestWriteYAML_TypedNilPointerWritesNull(t *testing.T) {
	type doc struct {
		A int `yaml:"a"`
	}
	var typed *doc
	if got := yamlOf(t, typed, YAMLStyle{}); got != "null\n" {
		t.Errorf("WriteYAML of a typed nil pointer = %q, want %q", got, "null\n")
	}
}

// TestWriteYAML_IndentOutsideTwoToNineIsAnError replaces a test that pinned
// this as a defect. The emitter honours 2 through 9 and RESETS anything else
// to 2 -- not to the nearer bound, to 2 -- so a width it will not honour has
// to be refused here or YAMLStyle{Indent: 10} produces output byte-identical
// to never having set Indent at all: configured, ignored, and silent about
// it. Clamping would be a quieter version of the same surprise, so the error
// names the value and the range instead, and nothing is written.
func TestWriteYAML_IndentOutsideTwoToNineIsAnError(t *testing.T) {
	v := map[string]any{"top": map[string]any{"deep": "value"}}

	for _, indent := range []int{-4, -1, 1, 10, 15, 100} {
		var b strings.Builder
		err := WriteYAML(&b, v, YAMLStyle{Indent: indent})
		if !errors.Is(err, ErrYAMLIndent) {
			t.Errorf("Indent %d: err = %v, want one wrapping ErrYAMLIndent", indent, err)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, fmt.Sprint(indent)) {
			t.Errorf("Indent %d: error does not name the value: %q", indent, msg)
		}
		for _, bound := range []int{minYAMLIndent, maxYAMLIndent} {
			if !strings.Contains(msg, fmt.Sprint(bound)) {
				t.Errorf("Indent %d: error does not name the accepted bound %d: %q", indent, bound, msg)
			}
		}
		if b.Len() != 0 {
			t.Errorf("Indent %d: wrote %d bytes despite the error: %q", indent, b.Len(), b.String())
		}
	}

	// 0 keeps meaning the default rather than being rejected with the rest
	// of the out-of-range values: it is the zero value of the field, which
	// is the caller saying nothing about indentation.
	if got := yamlOf(t, v, YAMLStyle{Indent: 0}); got != yamlOf(t, v, YAMLStyle{Indent: DefaultYAMLIndent}) {
		t.Errorf("Indent 0 is not the documented default of %d: %q", DefaultYAMLIndent, got)
	}
}

// TestWriteYAML_WritesNothingWhenTheEncodeFails is why the document is
// buffered before it reaches the writer. The encoder streams, so without the
// buffer a value it rejects part-way through -- here a big map followed by a
// channel -- leaves tens of kilobytes of truncated YAML on the writer, valid
// looking and cut off mid-value with no trailing newline. That is the one
// output this package's invariant forbids, and it is not fixable by writing
// it down: JSON already writes all of a document or none of it, and YAML has
// to match.
func TestWriteYAML_WritesNothingWhenTheEncodeFails(t *testing.T) {
	rows := map[string]string{}
	for i := 0; i < 5000; i++ {
		rows[fmt.Sprintf("key_%05d", i)] = strings.Repeat("x", 40)
	}
	// The map is encoded first and fine; the channel is reached last.
	v := struct {
		Rows map[string]string `yaml:"rows"`
		Ch   chan int          `yaml:"ch"`
	}{rows, make(chan int)}

	var b strings.Builder
	err := WriteYAML(&b, v, YAMLStyle{})
	if err == nil {
		t.Fatal("no error from a value the encoder cannot marshal")
	}
	if b.Len() != 0 {
		t.Errorf("wrote %d bytes of a failed document; it ends %q", b.Len(), tail(b.String(), 40))
	}
}

// TestWriteYAML_PropagatesWriteErrors covers the one write WriteYAML makes.
// Buffering the document has a second effect worth pinning: the write is now
// this package's own, so the writer's error is wrapped rather than formatted
// into a message by the YAML library, and errors.Is reaches it as it does
// for the other three formats.
func TestWriteYAML_PropagatesWriteErrors(t *testing.T) {
	big := map[string]string{}
	for i := 0; i < 2000; i++ {
		big[fmt.Sprintf("key_%05d", i)] = strings.Repeat("x", 40)
	}
	for _, c := range []struct {
		name string
		v    any
	}{
		{"small document", map[string]int{"a": 1}},
		{"large document", big},
	} {
		err := WriteYAML(&failWriter{n: 0}, c.v, YAMLStyle{})
		if !errors.Is(err, errWrite) {
			t.Errorf("%s: err = %v, want one wrapping %v", c.name, err, errWrite)
		}
	}
}

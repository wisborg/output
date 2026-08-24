package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// failWriter fails on the nth Write. It is duplicated from the table
// package's test helper rather than exported from it: a type that exists to
// break one package's tests is not API, and copying five lines is cheaper
// than a shared testing surface that then has to be kept stable.
type failWriter struct {
	n int
}

var errWrite = errors.New("write failed")

func (f *failWriter) Write(p []byte) (int, error) {
	f.n--
	if f.n < 0 {
		return 0, errWrite
	}
	return len(p), nil
}

func jsonOf(t *testing.T, v any, style JSONStyle) string {
	t.Helper()
	var b strings.Builder
	if err := WriteJSON(&b, v, style); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	return b.String()
}

// TestWriteJSON_DoesNotEscapeHTMLByDefault pins the inversion of
// encoding/json's default. The failure it guards is not a crash: it is a URL
// in a CLI's output that reads correctly to a program and wrongly to the
// person who copies it out of their terminal.
func TestWriteJSON_DoesNotEscapeHTMLByDefault(t *testing.T) {
	v := map[string]string{"url": "https://x/?a&b<c>d"}

	got := jsonOf(t, v, JSONStyle{})
	if !strings.Contains(got, "https://x/?a&b<c>d") {
		t.Errorf("default style escaped HTML characters:\n%s", got)
	}
	if strings.Contains(got, `\u0026`) {
		t.Errorf("default style emitted a \\u0026 escape:\n%s", got)
	}

	got = jsonOf(t, v, JSONStyle{EscapeHTML: true})
	for _, want := range []string{`\u0026`, `\u003c`, `\u003e`} {
		if !strings.Contains(got, want) {
			t.Errorf("EscapeHTML: true did not produce %s:\n%s", want, got)
		}
	}

	// Both spellings must still decode to the same string, or one of them
	// is not an escaping difference but a corruption.
	var back map[string]string
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("escaped output does not parse: %v", err)
	}
	if back["url"] != v["url"] {
		t.Errorf("escaped output decodes to %q, want %q", back["url"], v["url"])
	}
}

// TestWriteJSON_IndentsTwoSpacesPerLevel asserts the invariant rather than a
// golden blob: a key at depth n is preceded by exactly 2n spaces. A golden
// string would encode one right answer for one document; this encodes what
// makes any document right, and fails just as loudly if the indent becomes
// a tab, four spaces, or nothing.
func TestWriteJSON_IndentsTwoSpacesPerLevel(t *testing.T) {
	type inner struct {
		Deep string `json:"deep"`
	}
	type middle struct {
		Mid inner `json:"mid"`
	}
	type outer struct {
		Top middle `json:"top"`
	}
	depth := map[string]int{`"top"`: 1, `"mid"`: 2, `"deep"`: 3}

	got := jsonOf(t, outer{middle{inner{"value"}}}, JSONStyle{})
	seen := 0
	for _, line := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		if strings.ContainsRune(line, '\t') {
			t.Errorf("line %q is indented with a tab", line)
		}
		for key, level := range depth {
			if strings.HasPrefix(trimmed, key) {
				seen++
				if want := 2 * level; indent != want {
					t.Errorf("key %s at depth %d has %d leading spaces, want %d (line %q)", key, level, indent, want, line)
				}
			}
		}
	}
	if seen != len(depth) {
		t.Errorf("found %d of the %d keys in:\n%s", seen, len(depth), got)
	}
	if !strings.HasSuffix(got, "}\n") || strings.HasSuffix(got, "}\n\n") {
		t.Errorf("indented output must end in exactly one newline, got %q", got)
	}
}

// TestWriteJSON_CompactIsOneLine checks both halves of "one line": no
// interior newline, and one at the very end. A compact encoder that dropped
// the trailing newline would concatenate with the next thing the program
// prints.
func TestWriteJSON_CompactIsOneLine(t *testing.T) {
	v := map[string]any{"a": 1, "b": []int{1, 2, 3}, "c": map[string]int{"d": 4}}

	got := jsonOf(t, v, JSONStyle{Compact: true})
	if n := strings.Count(got, "\n"); n != 1 {
		t.Errorf("compact output has %d newlines, want 1:\n%q", n, got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("compact output does not end in a newline: %q", got)
	}
	if strings.Contains(strings.TrimSuffix(got, "\n"), "  ") {
		t.Errorf("compact output contains indentation: %q", got)
	}
}

// TestWriteJSON_UnsupportedTypeSurvivesTheWrapper is why the marshal error
// is wrapped with %w and not formatted with %v. *json.UnsupportedTypeError
// names the type that could not be encoded; flattening it to a string leaves
// the caller with prose to parse.
func TestWriteJSON_UnsupportedTypeSurvivesTheWrapper(t *testing.T) {
	type withChannel struct {
		Ch chan int `json:"ch"`
	}
	var b strings.Builder
	err := WriteJSON(&b, withChannel{make(chan int)}, JSONStyle{})
	if err == nil {
		t.Fatal("WriteJSON of a chan field returned no error")
	}
	var typeErr *json.UnsupportedTypeError
	if !errors.As(err, &typeErr) {
		t.Fatalf("err = %v (%T); want one that errors.As reaches a *json.UnsupportedTypeError through", err, err)
	}
	if got := typeErr.Type.String(); got != "chan int" {
		t.Errorf("recovered error names type %q, want %q", got, "chan int")
	}
	if !strings.Contains(err.Error(), "chan") {
		t.Errorf("error text does not name the offending type: %q", err.Error())
	}
}

// TestWriteJSON_CycleIsAnError records the difference from YAML, which
// cannot survive this at all: encoding/json keeps a visited set and reports
// a cycle as an ordinary error.
func TestWriteJSON_CycleIsAnError(t *testing.T) {
	type node struct {
		Next *node `json:"next"`
	}
	n := &node{}
	n.Next = n

	var b strings.Builder
	err := WriteJSON(&b, n, JSONStyle{})
	if err == nil {
		t.Fatal("WriteJSON of a cyclic value returned no error")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error does not mention a cycle: %q", err.Error())
	}
	var valueErr *json.UnsupportedValueError
	if !errors.As(err, &valueErr) {
		t.Errorf("err = %v (%T); want one wrapping *json.UnsupportedValueError", err, err)
	}
}

// TestWriteJSON_NilWritesNull is the documented asymmetry with
// Document.Write, where a nil Data is an error instead. Here the value is
// the argument the caller passed, and "null" is JSON's word for it.
func TestWriteJSON_NilWritesNull(t *testing.T) {
	if got := jsonOf(t, nil, JSONStyle{}); got != "null\n" {
		t.Errorf("WriteJSON(nil) = %q, want %q", got, "null\n")
	}
	if got := jsonOf(t, nil, JSONStyle{Compact: true}); got != "null\n" {
		t.Errorf("compact WriteJSON(nil) = %q, want %q", got, "null\n")
	}
	// A typed nil pointer is a value, not an absence, and marshals the
	// same way.
	type doc struct {
		A int `json:"a"`
	}
	var typed *doc
	if got := jsonOf(t, typed, JSONStyle{}); got != "null\n" {
		t.Errorf("WriteJSON of a typed nil pointer = %q, want %q", got, "null\n")
	}
}

// TestWriteJSON_WritesNothingWhenTheEncodeFails is the JSON half of the
// all-or-nothing rule that Document.Write documents for both encoders.
// encoding/json marshals into a buffer of its own before writing, so this
// holds today without help; WriteYAML has to buffer explicitly to match it.
// The assertion is here so that a change in either direction shows up as a
// failure rather than as a quietly broken promise.
func TestWriteJSON_WritesNothingWhenTheEncodeFails(t *testing.T) {
	type withChannel struct {
		Rows map[string]string `json:"rows"`
		Ch   chan int          `json:"ch"`
	}
	rows := map[string]string{}
	for i := 0; i < 5000; i++ {
		rows[fmt.Sprintf("key_%05d", i)] = strings.Repeat("x", 40)
	}

	var b strings.Builder
	if err := WriteJSON(&b, withChannel{rows, make(chan int)}, JSONStyle{}); err == nil {
		t.Fatal("no error from a value encoding/json cannot marshal")
	}
	if b.Len() != 0 {
		t.Errorf("wrote %d bytes of a failed document", b.Len())
	}
}

func TestWriteJSON_PropagatesWriteErrors(t *testing.T) {
	for _, style := range []JSONStyle{{}, {Compact: true}} {
		if err := WriteJSON(&failWriter{n: 0}, map[string]int{"a": 1}, style); !errors.Is(err, errWrite) {
			t.Errorf("style %+v: err = %v, want %v", style, err, errWrite)
		}
	}
}

func TestWriteJSON_HonoursStructTags(t *testing.T) {
	type v struct {
		SnakeName string `json:"snake_name"`
		Skipped   string `json:"-"`
	}
	got := jsonOf(t, v{"x", "y"}, JSONStyle{Compact: true})
	if !strings.Contains(got, `"snake_name":"x"`) {
		t.Errorf("json tag not honoured: %s", got)
	}
	if strings.Contains(got, "y") {
		t.Errorf(`field tagged "-" was written: %s`, got)
	}
}

package output

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// DefaultYAMLIndent is the number of spaces per nesting level when
// YAMLStyle.Indent is 0. Two is the prevailing convention, and the one
// Kubernetes, Docker Compose and most hand-written YAML use; the library's
// own default is four.
const DefaultYAMLIndent = 2

// The range of indent widths the YAML emitter actually honours. These are
// the library's numbers, not this package's preference -- see
// YAMLStyle.Indent.
const (
	minYAMLIndent = 2
	maxYAMLIndent = 9
)

// YAMLStyle controls YAML output. The zero value is two-space indentation.
type YAMLStyle struct {
	// Indent is the number of spaces per nesting level. 0 means
	// DefaultYAMLIndent, so the zero YAMLStyle is the conventional
	// rendering and this field only has to be set to depart from it.
	//
	// Any other value must be between 2 and 9. That ceiling is the YAML
	// emitter's, not this package's: it accepts 2 through 9 and RESETS
	// anything else to 2 -- not to the nearer bound, to 2 -- so
	// YAMLStyle{Indent: 10} would silently produce output identical to
	// never having set Indent at all. WriteYAML therefore rejects an
	// out-of-range Indent with an error naming the value and the range,
	// and writes nothing. Clamping 10 to 9 was considered and rejected: it
	// is a quieter version of the same surprise, and a caller who asked
	// for ten spaces is working from an assumption worth correcting rather
	// than approximating.
	Indent int
}

// errUnsupportedYAMLType is what a YAML encoder panic is converted into. It
// is unexported because the panic's text is all the encoder gives us: there
// is no typed error to hand back, and an exported sentinel would promise a
// stability this shim cannot keep if the upstream message changes.
var errUnsupportedYAMLType = errors.New("output: unsupported type for YAML")

// ErrYAMLIndent is returned for a YAMLStyle.Indent the emitter would not
// honour. See YAMLStyle.Indent for the accepted range.
//
// Exported, where errUnsupportedYAMLType is not, because the two differ in
// how firm a promise they can make. This one is this package's own check on
// its own field: it fires exactly when we say it does. The other depends on
// recognising an upstream panic by its text, so a caller matching on it
// would be relying on a dependency's private wording.
var ErrYAMLIndent = errors.New("output: unsupported YAML indent")

// yamlMarshalPanicPrefix is the start of the only panic value this package
// converts into an error. See WriteYAML; TestWriteYAML_UpstreamStillPanics
// is the tripwire that fires when a dependency bump changes it.
const yamlMarshalPanicPrefix = "cannot marshal type: "

// WriteYAML writes v as YAML, indented per style, followed by exactly one
// newline. There is no leading "---": a single document does not need one,
// and it is noise in the common case of a program printing one result.
//
// A nil v writes "null", exactly as WriteJSON does, and for the same reason:
// the value is the argument the caller passed, and null is YAML's word for
// it. See WriteJSON for the asymmetry with Document.Write, where a nil Data
// is instead an error wrapping ErrNoData.
//
// Mapping keys are ordered by the YAML library: map keys in a sorted order
// of its own, struct fields in declaration order. There is no knob for that
// here.
//
// # The document is encoded in memory first
//
// The YAML encoder streams, so a failure part-way through an encode has
// already put a large fragment of a document on the writer -- valid-looking
// YAML, truncated mid-value, with no trailing newline. WriteYAML therefore
// encodes into a buffer and copies to w only once the whole document is
// known to be good, which makes it atomic in the way encoding/json already
// is: on any error, nothing is written at all. That is what lets
// Document.Write promise that output is either empty or ends in exactly one
// newline, rather than promising it for three formats and excusing the
// fourth. The cost is holding the rendered document in memory, which for
// program output is a document a person or a pipeline was about to read
// anyway.
//
// # A type YAML cannot encode is an error here, not a panic
//
// The underlying encoder PANICS on a value it cannot marshal -- a channel or
// a function, where encoding/json returns *json.UnsupportedTypeError. A
// library where the value of a --format flag decides whether the program
// crashes is not one you can build a CLI on, so WriteYAML recovers that one
// panic and returns it as an error wrapping an unexported sentinel. The
// recovery is deliberately narrow: only a panic whose value is a string
// beginning "cannot marshal type: " is converted, and anything else is
// re-panicked, because anything else is a bug in the encoder and swallowing
// it would hide it.
//
// # A cyclic value still takes the process down
//
// This cannot be fixed here. The YAML encoder follows pointers with no
// visited set, so a value that points back at itself is encoded again at
// each level, for ever. What that looks like in practice is not a prompt
// crash: the encoder spins, emitting an ever-deeper nesting of the same data
// and consuming CPU and memory without bound, until the buffer can no longer
// grow or the recursion overflows the stack. Neither end is recoverable -- a
// stack overflow is fatal in Go, not a panic, and recover cannot see it --
// and buffering does not change that, it only moves the unbounded growth
// from the writer into memory. encoding/json detects cycles and reports them
// as errors, so the same Document that writes as JSON can take the program
// down as YAML.
//
// The alternative, a reflective pre-walk of every value looking for cycles,
// would cost that walk on every write to defend against a shape that almost
// never occurs in the data a program actually prints, and would have to
// track the same visited set for types this package knows nothing about. If
// your data can contain cycles -- a doubly linked list, a parent pointer, a
// graph -- break them before handing it to any encoder.
func WriteYAML(w io.Writer, v any, style YAMLStyle) error {
	indent, err := style.indent()
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := encodeYAML(&buf, v, indent); err != nil {
		return err
	}
	// One write of a complete document. bytes.Buffer never short-writes,
	// so anything wrong here is w's, and it is wrapped rather than
	// flattened: unlike the encoder's own write errors, this one is ours
	// to pass on intact, so errors.Is reaches the writer's error.
	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("output: writing YAML: %w", err)
	}
	return nil
}

// encodeYAML renders v into buf. It is separate from WriteYAML so that the
// recover shim covers the encoder and nothing else: a panic from the caller's
// io.Writer is not the encoder saying it cannot represent a value, and must
// not be caught here.
func encodeYAML(buf *bytes.Buffer, v any, indent int) (err error) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		msg, ok := r.(string)
		if !ok || !strings.HasPrefix(msg, yamlMarshalPanicPrefix) {
			// Not the encoder saying "I cannot represent this". Let it
			// through with its stack intact.
			panic(r)
		}
		err = fmt.Errorf("%w: %s", errUnsupportedYAMLType, strings.TrimPrefix(msg, yamlMarshalPanicPrefix))
	}()

	enc := yaml.NewEncoder(buf)
	enc.SetIndent(indent)
	if err := enc.Encode(v); err != nil {
		// Close after a failed Encode would report the wreckage of the
		// first error rather than a second, independent problem.
		return fmt.Errorf("output: writing YAML: %w", err)
	}
	// Close is the encoder's documented way of finishing the stream. This
	// version happens to flush at the end of each document, so the output
	// looks complete without it -- which is exactly why the call must stay:
	// the flushing point is an internal detail, and the failure if it moves
	// is a truncated document rather than an error.
	if err := enc.Close(); err != nil {
		return fmt.Errorf("output: writing YAML: %w", err)
	}
	return nil
}

// indent resolves YAMLStyle.Indent's 0-means-default convention and rejects
// a width the emitter would not honour. The check is here, before anything
// is encoded, so an out-of-range Indent writes nothing.
func (s YAMLStyle) indent() (int, error) {
	if s.Indent == 0 {
		return DefaultYAMLIndent, nil
	}
	if s.Indent < minYAMLIndent || s.Indent > maxYAMLIndent {
		return 0, fmt.Errorf("%w: %d (accepted: 0 for the default of %d, or %d to %d -- the YAML emitter silently resets anything else to %d)",
			ErrYAMLIndent, s.Indent, DefaultYAMLIndent, minYAMLIndent, maxYAMLIndent, DefaultYAMLIndent)
	}
	return s.Indent, nil
}

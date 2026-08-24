package output

import (
	"encoding/json"
	"fmt"
	"io"
)

// JSONStyle controls JSON output. The zero value is what a command-line
// program wants: indented, and with HTML escaping off.
type JSONStyle struct {
	// Compact writes the whole value on one line with no indentation, for
	// a log line or for JSON Lines. The default is indented, because the
	// usual reader of a CLI's JSON is a person looking at a terminal, and
	// the machine reader does not care either way.
	Compact bool

	// EscapeHTML escapes <, > and & as \u003c, \u003e and \u0026.
	//
	// The zero value does NOT escape, which inverts encoding/json's
	// default. That default exists because the standard library's JSON is
	// often embedded in HTML, where those characters can end a script
	// element early. Program output is not: it goes to a terminal or into
	// jq, and there a URL comes out as "https://x/?a\u0026b", which is
	// wrong on sight and stays wrong when the reader copies it. Set this
	// when the JSON really is destined for a web page.
	EscapeHTML bool
}

// WriteJSON writes v as JSON, followed by exactly one newline.
//
// A nil v writes "null" -- JSON's own spelling of "no value", and the only
// honest rendering of the argument it was given. Note the deliberate
// asymmetry with Document.Write, where a nil Data is instead an error
// wrapping ErrNoData: there the nil is a field nobody filled in, which is
// far more often a bug than a value, and the Document knows which format
// needed it. Here the value is the argument, stated by the caller, so
// writing it is what was asked for.
//
// Object keys are sorted by encoding/json (map keys alphabetically, struct
// fields in declaration order); there is no knob for that here.
//
// Marshalling errors are wrapped with %w rather than flattened into text,
// because encoding/json's error types carry things a caller can act on:
// *json.UnsupportedTypeError names the offending type, *json.MarshalerError
// names the type whose MarshalJSON failed and unwraps to its error, and
// *json.UnsupportedValueError covers NaN, +Inf and a cyclic value.
func WriteJSON(w io.Writer, v any, style JSONStyle) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(style.EscapeHTML)
	if !style.Compact {
		// Two spaces, the prevailing convention for JSON and what jq
		// produces, so piping through jq does not reformat the file.
		enc.SetIndent("", "  ")
	}
	// Encode appends the trailing newline itself, in both modes.
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("output: writing JSON: %w", err)
	}
	return nil
}

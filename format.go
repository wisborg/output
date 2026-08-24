package output

import (
	"fmt"
	"strings"
)

// Format selects which shape a Document is written in.
//
// The zero value is Text, so a Format field that nobody set is the plain
// human-readable rendering -- the right default for a command-line program,
// and the one that makes a struct literal with no Format in it do something
// sensible.
type Format int

const (
	// Text is aligned text for a human to read, from the Document's Table.
	Text Format = iota
	// CSV is comma-separated data, from the Document's Table.
	CSV
	// JSON is JSON, from the Document's Data.
	JSON
	// YAML is YAML, from the Document's Data.
	YAML
)

// String returns the format's canonical name: "text", "csv", "json" or
// "yaml". These are the names ParseFormat accepts, so a value that survives
// String and ParseFormat is unchanged, and a flag's default can be printed
// with the same spelling the user has to type.
//
// A Format outside the defined set renders as "Format(n)", following the
// convention of the standard library and of table.Align: an invalid value
// should be conspicuous in a message rather than pretending to be text.
func (f Format) String() string {
	switch f {
	case Text:
		return "text"
	case CSV:
		return "csv"
	case JSON:
		return "json"
	case YAML:
		return "yaml"
	default:
		return fmt.Sprintf("Format(%d)", int(f))
	}
}

// Formats returns every defined format, in declaration order. It is for
// building a flag's help text or a menu without hard-coding the list a
// second time and letting the two drift apart.
//
// Each call returns a fresh slice, so a caller may sort or filter it without
// affecting anyone else.
func Formats() []Format {
	return []Format{Text, CSV, JSON, YAML}
}

// aliases are the spellings accepted in addition to the canonical names.
// Both are names people reach for from other tools -- "table" for text
// output, "yml" for YAML -- and refusing them buys nothing.
//
// It is a slice rather than a map so that the order is the one written here:
// this list is also the tail of the "accepted" list in an error message, and
// a map would shuffle that message between runs.
var aliases = []struct {
	name   string
	format Format
}{
	{"table", Text},
	{"yml", YAML},
}

// ParseFormat turns a format name into a Format. Surrounding space is
// trimmed and case is ignored, so "JSON", " json" and "json" are one thing:
// the name usually comes from a human typing a flag.
//
// The canonical names are those of Format.String; "table" is also accepted
// for Text and "yml" for YAML.
//
// An unrecognised name is an error wrapping ErrUnknownFormat. There is
// deliberately no fallback to Text: a program that silently prints a table
// when asked for JSON produces output its caller cannot parse and no
// message saying why.
func ParseFormat(s string) (Format, error) {
	name := strings.ToLower(strings.TrimSpace(s))
	for _, f := range Formats() {
		if name == f.String() {
			return f, nil
		}
	}
	for _, a := range aliases {
		if name == a.name {
			return a.format, nil
		}
	}
	return 0, fmt.Errorf("%w %q (accepted: %s)", ErrUnknownFormat, s, acceptedNames())
}

// acceptedNames lists every spelling ParseFormat takes, canonical names
// first and then the aliases, for an error message that tells the user what
// to type instead of only that they were wrong.
func acceptedNames() string {
	names := make([]string, 0, len(Formats())+len(aliases))
	for _, f := range Formats() {
		names = append(names, f.String())
	}
	for _, a := range aliases {
		names = append(names, a.name)
	}
	return strings.Join(names, ", ")
}

// Set parses s into f, making *Format a flag.Value. A format flag is then
// one line, validated where the flag is parsed rather than where it is used:
//
//	format := output.JSON // the default
//	flag.Var(&format, "format", "output format")
//
// Because Format.String is the canonical name, the flag package prints the
// default in the same spelling Set accepts.
func (f *Format) Set(s string) error {
	parsed, err := ParseFormat(s)
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}

// MarshalText makes Format encode as its canonical name in JSON, YAML and
// anything else built on encoding.TextMarshaler -- so a Format inside a
// config struct is "json" rather than the integer 2, which would be both
// unreadable and silently wrong the day a format is inserted into the middle
// of the constant block.
func (f Format) MarshalText() ([]byte, error) {
	if !f.valid() {
		return nil, fmt.Errorf("%w %d (accepted: %s)", ErrUnknownFormat, int(f), acceptedNames())
	}
	return []byte(f.String()), nil
}

// UnmarshalText parses a format name, with the same spellings ParseFormat
// takes.
func (f *Format) UnmarshalText(b []byte) error {
	return f.Set(string(b))
}

// valid reports whether f is one of the defined formats. It is written
// against Formats rather than as a range check on the constants, so a format
// added to the list is covered here without a second edit.
func (f Format) valid() bool {
	for _, known := range Formats() {
		if f == known {
			return true
		}
	}
	return false
}

package output

import (
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
)

// TestFormat_ZeroValueIsText pins the default. A Format field nobody set has
// to mean the human-readable rendering; if the constant block is ever
// reordered so that JSON is 0, every program that omits the flag starts
// emitting JSON to a terminal and nothing here would otherwise notice.
func TestFormat_ZeroValueIsText(t *testing.T) {
	var f Format
	if f != Text {
		t.Errorf("zero Format = %d, want Text (%d)", int(f), int(Text))
	}
	if got := f.String(); got != "text" {
		t.Errorf("zero Format.String() = %q, want %q", got, "text")
	}
}

// TestFormat_StringNamesEveryFormat is the check that a format added to the
// constant block and to Formats but forgotten in String cannot ship: the
// fallback spelling "Format(n)" would appear in help text, in error messages
// and in any config file the format is marshalled into.
func TestFormat_StringNamesEveryFormat(t *testing.T) {
	for _, f := range Formats() {
		got := f.String()
		if strings.Contains(got, "Format(") {
			t.Errorf("Format(%d).String() = %q, which is the unnamed-value fallback", int(f), got)
		}
		if got != strings.ToLower(got) || strings.TrimSpace(got) != got || got == "" {
			t.Errorf("Format(%d).String() = %q, want a non-empty lowercase name with no padding", int(f), got)
		}
	}
}

func TestFormat_StringOfUnknownValue(t *testing.T) {
	if got := Format(99).String(); got != "Format(99)" {
		t.Errorf("Format(99).String() = %q, want %q", got, "Format(99)")
	}
}

// TestParseFormat_RoundTrip is the property that makes String safe to print
// as a flag default: whatever a Format prints as, ParseFormat reads back as
// the same Format.
func TestParseFormat_RoundTrip(t *testing.T) {
	for _, want := range Formats() {
		got, err := ParseFormat(want.String())
		if err != nil {
			t.Errorf("ParseFormat(%q): %v", want.String(), err)
			continue
		}
		if got != want {
			t.Errorf("ParseFormat(%q) = %v, want %v", want.String(), got, want)
		}
	}
}

func TestParseFormat_CaseSpaceAndAliases(t *testing.T) {
	for in, want := range map[string]Format{
		"text":    Text,
		"TEXT":    Text,
		"  text ": Text,
		"Table":   Text,
		"csv":     CSV,
		"CSV":     CSV,
		"json":    JSON,
		"Json":    JSON,
		"yaml":    YAML,
		"yml":     YAML,
		"\tYML\n": YAML,
	} {
		got, err := ParseFormat(in)
		if err != nil {
			t.Errorf("ParseFormat(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseFormat(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestParseFormat_UnknownNamesTheInputAndTheAlternatives checks the part of
// an error message that decides whether the user can fix the problem: what
// they typed, and what they could have typed instead. A bare "unknown
// format" tells them neither.
func TestParseFormat_UnknownNamesTheInputAndTheAlternatives(t *testing.T) {
	f, err := ParseFormat("xml")
	if !errors.Is(err, ErrUnknownFormat) {
		t.Fatalf("ParseFormat(\"xml\") err = %v, want one wrapping ErrUnknownFormat", err)
	}
	if f != Text {
		t.Errorf("failed ParseFormat returned %v; the caller must not be handed a usable format", f)
	}
	msg := err.Error()
	if !strings.Contains(msg, "xml") {
		t.Errorf("error does not name the input: %q", msg)
	}
	for _, known := range Formats() {
		if !strings.Contains(msg, known.String()) {
			t.Errorf("error does not offer %q as an alternative: %q", known.String(), msg)
		}
	}
	for _, a := range aliases {
		if !strings.Contains(msg, a.name) {
			t.Errorf("error does not mention the accepted alias %q: %q", a.name, msg)
		}
	}
}

func TestParseFormat_EmptyStringIsAnError(t *testing.T) {
	if _, err := ParseFormat(""); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("ParseFormat(\"\") err = %v, want one wrapping ErrUnknownFormat", err)
	}
}

// TestFormat_AsFlagValue parses a real command line through a real
// flag.FlagSet. Asserting only that Set works would miss the mistake this
// is here for: a Set declared on the value receiver still compiles, still
// satisfies flag.Value through &f, and silently discards the parsed value
// because it wrote to a copy.
func TestFormat_AsFlagValue(t *testing.T) {
	format := Text
	fs := flag.NewFlagSet("prog", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Var(&format, "format", "output format")

	if err := fs.Parse([]string{"-format", "json"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if format != JSON {
		t.Errorf("after -format json, format = %v, want json", format)
	}

	// The default has to print in a spelling Set accepts, or the usage
	// message tells the user to type something that is refused.
	var usage strings.Builder
	fs2 := flag.NewFlagSet("prog", flag.ContinueOnError)
	fs2.SetOutput(&usage)
	def := YAML
	fs2.Var(&def, "format", "output format")
	fs2.PrintDefaults()
	if !strings.Contains(usage.String(), "yaml") {
		t.Errorf("usage does not show the default in its canonical spelling:\n%s", usage.String())
	}

	if err := fs2.Parse([]string{"-format", "toml"}); err == nil {
		t.Error("flag parsing accepted -format toml; an unknown format must fail at parse time")
	}
}

// TestFormats_FreshSlice guards against returning a package-level slice,
// which would let one caller's sort or overwrite reach every other caller.
func TestFormats_FreshSlice(t *testing.T) {
	a, b := Formats(), Formats()
	if len(a) == 0 {
		t.Fatal("Formats() is empty")
	}
	a[0] = Format(99)
	if b[0] == Format(99) {
		t.Error("Formats() returns a shared slice: mutating one result changed another")
	}
	if c := Formats(); c[0] != Text {
		t.Errorf("Formats() was permanently modified by a caller: first element %v", c[0])
	}
}

func TestFormats_ContainsEveryDefinedFormat(t *testing.T) {
	// Formats drives ParseFormat and the error messages, so a format
	// missing from it is unparseable however well its String works.
	seen := map[Format]bool{}
	for _, f := range Formats() {
		if seen[f] {
			t.Errorf("Formats() lists %v twice", f)
		}
		seen[f] = true
	}
	for _, f := range []Format{Text, CSV, JSON, YAML} {
		if !seen[f] {
			t.Errorf("Formats() omits %v", f)
		}
	}
}

func TestFormat_TextMarshalRoundTrip(t *testing.T) {
	for _, want := range Formats() {
		b, err := want.MarshalText()
		if err != nil {
			t.Errorf("MarshalText(%v): %v", want, err)
			continue
		}
		if string(b) != want.String() {
			t.Errorf("MarshalText(%v) = %q, want %q", want, b, want.String())
		}
		var got Format
		if err := got.UnmarshalText(b); err != nil {
			t.Errorf("UnmarshalText(%q): %v", b, err)
			continue
		}
		if got != want {
			t.Errorf("round trip of %v gave %v", want, got)
		}
	}
}

func TestFormat_MarshalTextRejectsUnknownValue(t *testing.T) {
	if b, err := Format(99).MarshalText(); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("Format(99).MarshalText() = %q, %v; want an error wrapping ErrUnknownFormat", b, err)
	} else if !strings.Contains(err.Error(), "99") {
		t.Errorf("error does not name the value: %q", err.Error())
	}
}

func TestFormat_UnmarshalTextRejectsUnknownName(t *testing.T) {
	f := JSON
	if err := f.UnmarshalText([]byte("xml")); !errors.Is(err, ErrUnknownFormat) {
		t.Errorf("UnmarshalText(\"xml\") = %v, want an error wrapping ErrUnknownFormat", err)
	}
	if f != JSON {
		t.Errorf("a failed UnmarshalText overwrote the destination: %v", f)
	}
}

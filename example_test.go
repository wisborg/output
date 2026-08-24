package output_test

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/wisborg/output"
	"github.com/wisborg/output/table"
)

// probe is the result of one comparison: the object a machine gets.
type probe struct {
	Clip     string  `json:"clip" yaml:"clip"`
	Offset   float64 `json:"offset" yaml:"offset"`
	Score    float64 `json:"score" yaml:"score"`
	Detector string  `json:"detector" yaml:"detector"`
}

// Example builds both representations of the same result -- the object for
// JSON and YAML, a simplified table for text and CSV -- and lets the format
// choose. The table deliberately leaves out the detector: it is detail the
// machine wants and the person reading a terminal does not.
func Example() {
	results := []probe{
		{Clip: "corner_1", Offset: 2.70, Score: 0.821, Detector: "phase-correlation"},
		{Clip: "corner_3", Offset: 2.75, Score: 0.366, Detector: "phase-correlation"},
	}

	summary := table.New(
		table.Column{Header: "clip"},
		table.Column{Header: "offset", Align: table.Right, Format: "%+.2fs"},
		table.Column{Header: "score", Align: table.Right, Format: "%.3f"},
	)
	for _, r := range results {
		summary.MustAppend(r.Clip, r.Offset, r.Score)
	}

	doc := output.Document{Data: results, Table: summary}

	if err := doc.Write(os.Stdout, output.Text); err != nil {
		fmt.Println("write:", err)
	}

	// The same document as JSON is the object, detector and all.
	if err := doc.Write(os.Stdout, output.JSON); err != nil {
		fmt.Println("write:", err)
	}
	// Output:
	// clip       offset   score
	// -------------------------
	// corner_1   +2.70s   0.821
	// corner_3   +2.75s   0.366
	// [
	//   {
	//     "clip": "corner_1",
	//     "offset": 2.7,
	//     "score": 0.821,
	//     "detector": "phase-correlation"
	//   },
	//   {
	//     "clip": "corner_3",
	//     "offset": 2.75,
	//     "score": 0.366,
	//     "detector": "phase-correlation"
	//   }
	// ]
}

func ExampleDocument_Write_json() {
	doc := output.Document{
		Data: probe{Clip: "corner_1", Offset: 2.70, Score: 0.821, Detector: "phase-correlation"},
	}
	if err := doc.Write(os.Stdout, output.JSON); err != nil {
		fmt.Println("write:", err)
	}
	// Output:
	// {
	//   "clip": "corner_1",
	//   "offset": 2.7,
	//   "score": 0.821,
	//   "detector": "phase-correlation"
	// }
}

func ExampleDocument_Write_yaml() {
	doc := output.Document{
		Data: probe{Clip: "corner_1", Offset: 2.70, Score: 0.821, Detector: "phase-correlation"},
	}
	if err := doc.Write(os.Stdout, output.YAML); err != nil {
		fmt.Println("write:", err)
	}
	// Output:
	// clip: corner_1
	// offset: 2.7
	// score: 0.821
	// detector: phase-correlation
}

// ExampleDocument_Write_missing shows what happens when the format asks for
// a representation the Document does not have. Nothing is written, and the
// error names the format and can be tested for with errors.Is.
func ExampleDocument_Write_missing() {
	doc := output.Document{Data: probe{Clip: "corner_1"}} // no Table

	err := doc.Write(os.Stdout, output.CSV)
	fmt.Println(err)
	fmt.Println(errors.Is(err, output.ErrNoTable))
	// Output:
	// output: no Table to write as csv
	// true
}

// ExampleFormat_Set uses a Format as a flag value. Set validates at parse
// time, so an unknown name is rejected where the user typed it rather than
// silently falling back to text.
func ExampleFormat_Set() {
	format := output.Text // the default

	fs := flag.NewFlagSet("probe", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // the usage message is not the point here
	fs.Var(&format, "format", "output format")
	if err := fs.Parse([]string{"-format", "YAML"}); err != nil {
		fmt.Println("parse:", err)
	}
	fmt.Println(format)

	if err := fs.Parse([]string{"-format", "xml"}); err != nil {
		fmt.Println(err)
	}
	// Output:
	// yaml
	// invalid value "xml" for flag -format: output: unknown format "xml" (accepted: text, csv, json, yaml, table, yml)
}

// ExampleWriteJSON writes an object without a Document, for a program that
// has no table to offer. Note that HTML characters are left alone, unlike
// encoding/json's default: this output is for a terminal or for jq.
func ExampleWriteJSON() {
	v := map[string]string{"url": "https://example.test/?a=1&b=2"}
	if err := output.WriteJSON(os.Stdout, v, output.JSONStyle{}); err != nil {
		fmt.Println("write:", err)
	}
	if err := output.WriteJSON(os.Stdout, v, output.JSONStyle{Compact: true}); err != nil {
		fmt.Println("write:", err)
	}
	// Output:
	// {
	//   "url": "https://example.test/?a=1&b=2"
	// }
	// {"url":"https://example.test/?a=1&b=2"}
}

func ExampleWriteYAML() {
	v := probe{Clip: "corner_1", Offset: 2.70, Score: 0.821, Detector: "phase-correlation"}
	if err := output.WriteYAML(os.Stdout, v, output.YAMLStyle{}); err != nil {
		fmt.Println("write:", err)
	}
	// Output:
	// clip: corner_1
	// offset: 2.7
	// score: 0.821
	// detector: phase-correlation
}

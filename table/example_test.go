package table_test

import (
	"fmt"
	"os"

	"github.com/wisborg/output/table"
)

func Example() {
	t := table.New(
		table.Column{Header: "clip"},
		table.Column{Header: "offset", Align: table.Right, Format: "%+.2fs"},
		table.Column{Header: "score", Align: table.Right, Format: "%.3f"},
	)
	t.MustAppend("corner_1", 2.70, 0.821)
	t.MustAppend("corner_3", 2.75, 0.366)

	fmt.Print(t)
	// Output:
	// clip       offset   score
	// -------------------------
	// corner_1   +2.70s   0.821
	// corner_3   +2.75s   0.366
}

// ExampleTable_Columns shows a column being re-aligned after its rows are
// already in place. Cells are stored as given and formatted only at render
// time, so this needs no rebuilding of the table.
func ExampleTable_Columns() {
	t := table.New(
		table.Column{Header: "word"},
		table.Column{Header: "n"},
	)
	t.MustAppend("a", 1)
	t.MustAppend("bbbb", 1000)

	// The data has shown that the second column is numeric.
	t.Columns[1].Align = table.Right

	fmt.Print(t)
	// Output:
	// word      n
	// -----------
	// a         1
	// bbbb   1000
}

func ExampleTable_Render_frame() {
	t := table.New(
		table.Column{Header: "a"},
		table.Column{Header: "b", Align: table.Right},
	)
	t.MustAppend("x", 12)

	if err := t.Render(os.Stdout, table.Style{Frame: true}); err != nil {
		fmt.Println("render:", err)
	}
	// Output:
	// +---+----+
	// | a |  b |
	// +---+----+
	// | x | 12 |
	// +---+----+
}

func ExampleTable_Render_multiline() {
	t := table.New(
		table.Column{Header: "name"},
		table.Column{Header: "detail"},
	)
	t.MustAppend("first", "line one\nline two")
	t.MustAppend("second", "single")

	if err := t.Render(os.Stdout, table.Style{Multiline: true}); err != nil {
		fmt.Println("render:", err)
	}
	// Output:
	// name     detail
	// -----------------
	// first    line one
	//          line two
	// second   single
}

// ExampleTable_AppendSeparator groups rows with a horizontal rule. Separators
// keep the position they were added at, so appending more rows afterwards
// does not move them.
func ExampleTable_AppendSeparator() {
	t := table.New(
		table.Column{Header: "group"},
		table.Column{Header: "value", Align: table.Right},
	)
	t.MustAppend("first", 1)
	t.AppendSeparator()
	t.MustAppend("second", 22)
	t.MustAppend("third", 333)

	fmt.Print(t)
	// Output:
	// group    value
	// --------------
	// first        1
	// --------------
	// second      22
	// third      333
}

package dot_test

import (
	"fmt"

	"github.com/wisborg/output/diagram/dot"
)

// Databases in two regions: each region a lane, its databases in a grid of
// two columns -- shared between the lanes, so dbC is under dbA -- and the
// links between them. Without the grid the boxes would be placed by the
// edges, and would move when one of them changed.
func ExampleParse() {
	d, err := dot.Parse(`digraph {
		rows = 2
		subgraph cluster_east { label = "us-east-1"; style = lane; columns = 2; dbA; dbB }
		subgraph cluster_west { label = "us-west-2"; style = lane; columns = 2; dbC }
		dbA -> dbB
		dbA -> dbC [label = "async", style = dashed]
	}`)
	if err != nil {
		fmt.Println(err)
		return
	}
	lay, err := d.Layout()
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, line := range lay.Text() {
		fmt.Println(line)
	}
	// Output:
	// ┌─────┐      ┌─────┐
	// │ dbA ├─────▶│ dbB │    us-east-1
	// └──┬──┘      └─────┘
	//    ╎
	// ───┼─────────────────────────────
	//    ▼ async
	// ┌─────┐
	// │ dbC │                 us-west-2
	// └─────┘
}

package chart

import (
	"strings"
	"testing"
)

// Only the kinds bands have are rows and legend entries, in BandKinds'
// order; a band of a kind BandKinds does not have is not drawn, and
// takes no row.
func TestUsedBandKinds(t *testing.T) {
	c := ramp(1000, 1000)
	c.BandKinds = testKinds
	c.Bands = []Band{{From: 10, To: 20, Kind: 2}, {From: 30, To: 40, Kind: 0}, {From: 50, To: 60, Kind: 7}, {From: 70, To: 80, Kind: -1}}
	if got := c.UsedBandKinds(); len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("used kinds %v, want 0 and 2", got)
	}
	lines := c.Braille(44, 14)
	if len(lines) != 14 || lines[13] != "    ━ intervals   ╲ descents" {
		t.Errorf("lines %d, legend %q", len(lines), lines[len(lines)-1])
	}
	if strings.Contains(strings.Join(lines, "\n"), "╱") {
		t.Error("a kind no band has is marked")
	}
	if BandRows(0) != 0 || BandRows(2) != 3 {
		t.Errorf("BandRows(0), (2) = %d, %d", BandRows(0), BandRows(2))
	}
}

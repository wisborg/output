// Package memory says how much memory a drawing would take, in the words a
// refusal gives it: "about 1.3 GB". It is shared by the packages that
// refuse a drawing as too large and by the commands, and internal so that
// it is no API anyone else depends on.
package memory

import "fmt"

// About is n bytes as an estimate, to two figures in decimal units --
// "about 650 MB", "about 1.3 GB" -- since the bytes a unit of a drawing
// takes were measured, not counted, and more figures would claim more
// than is known.
func About(n float64) string {
	units := []string{"bytes", "kB", "MB", "GB", "TB", "PB", "EB"}
	i := 0
	for n >= 999.5 && i < len(units)-1 {
		n /= 1000
		i++
	}
	if i > 0 && n < 9.95 {
		return fmt.Sprintf("about %.1f %s", n, units[i])
	}
	return fmt.Sprintf("about %.0f %s", n, units[i])
}

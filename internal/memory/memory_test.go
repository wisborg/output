package memory

import "testing"

func TestAbout(t *testing.T) {
	for _, c := range []struct {
		n    float64
		want string
	}{
		{0, "about 0 bytes"},
		{512, "about 512 bytes"},
		{999.6, "about 1.0 kB"},
		{25e6, "about 25 MB"},
		{6.3e6, "about 6.3 MB"},
		{9.96e6, "about 10 MB"},
		{1.3e9, "about 1.3 GB"},
		{650e6, "about 650 MB"},
		{1e11, "about 100 GB"},
		{2e21, "about 2000 EB"},
	} {
		if got := About(c.n); got != c.want {
			t.Errorf("About(%v) = %q, want %q", c.n, got, c.want)
		}
	}
}

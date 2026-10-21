package update

import "testing"

func TestNova(t *testing.T) {
	casos := []struct {
		atual, tag string
		nova       bool
	}{{"v1.0.0", "v1.0.1", true}, {"1.0.0", "v1.0.0", false}, {"v1.2.0", "v1.1.9", false}, {"v0.0.0-dev", "v0.1.0", true}, {"v1.0.0", "v2.0.0", true}}
	for _, c := range casos {
		if got := Nova(c.atual, c.tag); got != c.nova {
			t.Errorf("%s → %s: %v", c.atual, c.tag, got)
		}
	}
}

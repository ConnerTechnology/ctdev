package cmd

import (
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/profile"
)

func TestProfileStats(t *testing.T) {
	p := &profile.Profile{Name: "test", Components: []string{"a", "b", "c"}}
	st := profileStats(p, map[string]bool{"a": true, "c": true, "unrelated": true}, false)
	if st.Installed != 2 || st.Total != 3 {
		t.Errorf("got %d/%d, want 2/3", st.Installed, st.Total)
	}
	if st.Inferred {
		t.Error("inferred should carry through as false")
	}
	// An empty machine against a real profile is drift, not a parse error.
	st = profileStats(p, map[string]bool{}, true)
	if st.Installed != 0 || !st.Inferred {
		t.Errorf("got installed=%d inferred=%v, want 0/true", st.Installed, st.Inferred)
	}
}

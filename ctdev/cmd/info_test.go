package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/profile"
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

func TestCheckoutUnderPrefersCtdev(t *testing.T) {
	tests := []struct {
		name    string
		folders []string
		want    string
	}{
		{"both folders exist", []string{"ctdev", "dotfiles"}, "ctdev"},
		{"only the renamed folder", []string{"ctdev"}, "ctdev"},
		{"only the old folder", []string{"dotfiles"}, "dotfiles"},
		{"neither exists", nil, "dotfiles"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org := t.TempDir()
			for _, f := range tt.folders {
				if err := os.Mkdir(filepath.Join(org, f), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if got, want := checkoutUnder(org), filepath.Join(org, tt.want); got != want {
				t.Errorf("got %s, want %s", got, want)
			}
		})
	}
}

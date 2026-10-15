package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

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

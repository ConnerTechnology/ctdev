package cmd

import (
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/cleanup"
)

func TestHasWork(t *testing.T) {
	cases := []struct {
		name string
		in   cleanup.ScanResult
		want bool
	}{
		{"has bytes", cleanup.ScanResult{Bytes: 1024}, true},
		{"zero bytes", cleanup.ScanResult{Bytes: 0}, false},
		{"zero with none note", cleanup.ScanResult{Bytes: 0, Note: "none"}, false},
		{"unknown with note", cleanup.ScanResult{Bytes: -1, Note: "3 packages"}, true},
		{"unknown but none", cleanup.ScanResult{Bytes: -1, Note: "none"}, false},
	}
	for _, c := range cases {
		if got := hasWork(c.in); got != c.want {
			t.Errorf("%s: hasWork=%v want %v", c.name, got, c.want)
		}
	}
}

func TestSizeLabel(t *testing.T) {
	cases := []struct {
		in   cleanup.ScanResult
		want string
	}{
		{cleanup.ScanResult{Bytes: 1536}, "1.5 KB"},
		{cleanup.ScanResult{Bytes: 1536, Note: "2 revisions"}, "1.5 KB · 2 revisions"},
		{cleanup.ScanResult{Bytes: -1, Note: "3 packages"}, "3 packages"},
		{cleanup.ScanResult{Bytes: 0}, "—"},
	}
	for _, c := range cases {
		if got := sizeLabel(c.in); got != c.want {
			t.Errorf("sizeLabel(%+v)=%q want %q", c.in, got, c.want)
		}
	}
}

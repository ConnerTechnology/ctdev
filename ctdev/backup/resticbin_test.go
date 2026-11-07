package backup

import (
	"strings"
	"testing"
)

// A scheduled run has no shell PATH: ~/.local/bin first everywhere, then
// Homebrew's two prefixes on macOS (Apple silicon, then Intel).
func TestResticCandidates(t *testing.T) {
	for goos, want := range map[string]string{
		"linux":  "/home/example/.local/bin/restic",
		"darwin": "/home/example/.local/bin/restic,/opt/homebrew/bin/restic,/usr/local/bin/restic",
	} {
		if got := strings.Join(resticCandidates(goos, "/home/example"), ","); got != want {
			t.Errorf("%s: %s, want %s", goos, got, want)
		}
	}
}

package component

import (
	"strings"
	"testing"
)

// The source file ends in .sources, which apt reads only as deb822.
func TestPaperAPTSourceIsDeb822(t *testing.T) {
	src := paperAPTSource()
	if strings.HasPrefix(src, "deb ") {
		t.Fatalf("paper-desktop.sources must be deb822, got a one-line entry: %q", src)
	}
	for _, want := range []string{"Types: deb", "URIs: " + paperRepoURL, "Suites: stable", "Components: main", "Signed-By: " + paperKeyring} {
		if !strings.Contains(src, want+"\n") && !strings.HasSuffix(src, want) {
			t.Errorf("source missing %q:\n%s", want, src)
		}
	}
}

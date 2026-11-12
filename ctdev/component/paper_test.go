package component

import (
	"strings"
	"testing"
)

func TestPaperRegistered(t *testing.T) {
	c := FindByName("paper")
	if c == nil {
		t.Fatal("paper component missing from registry")
	}
	if c.Category != CategoryDesktop {
		t.Errorf("expected paper in Desktop, got %v", c.Category)
	}
	if len(c.SupportedOS) != 2 || c.SupportedOS[0] != OSMacOS || c.SupportedOS[1] != OSLinux {
		t.Errorf("expected paper on macOS and Linux, got %v", c.SupportedOS)
	}
	if c.DetectCmd != "paper-desktop" {
		t.Errorf("expected paper detected by the .deb's paper-desktop command on Linux, got %q", c.DetectCmd)
	}
}

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

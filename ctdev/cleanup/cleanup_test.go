package cleanup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
)

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"312 MB":  312 * 1 << 20,
		"1.5G":    int64(1.5 * float64(1<<30)), // 1.5*2^30 is integral
		"240.0M":  240 * 1 << 20,
		"1,024 K": 1024 * 1 << 10,
		"512":     512,
		"0B":      0,
	}
	for in, want := range cases {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q)=%d want %d", in, got, want)
		}
	}
	if parseSize("nonsense") != -1 {
		t.Error("expected -1 for unparseable size")
	}
}

func TestHumanize(t *testing.T) {
	cases := map[int64]string{
		0:             "0 B",
		512:           "512 B",
		1536:          "1.5 KB",
		1 << 20:       "1.0 MB",
		3 * (1 << 30): "3.0 GB",
	}
	for in, want := range cases {
		if got := Humanize(in); got != want {
			t.Errorf("Humanize(%d)=%q want %q", in, got, want)
		}
	}
	if Humanize(-1) != "?" {
		t.Error("unknown size should render as ?")
	}
}

func TestParseAptFreed(t *testing.T) {
	out := "Reading package lists...\nThe following packages will be REMOVED:\n  oldpkg\nAfter this operation, 312 MB disk space will be freed.\n"
	if got := parseAptFreed(out); got != 312*(1<<20) {
		t.Errorf("parseAptFreed=%d want %d", got, 312*(1<<20))
	}
	if got := parseAptFreed("nothing here"); got != 0 {
		t.Errorf("no-freed line should be 0, got %d", got)
	}
}

func TestParseJournalUsage(t *testing.T) {
	out := "Archived and active journals take up 240.0M in the file system."
	if got := parseJournalUsage(out); got != 240*(1<<20) {
		t.Errorf("parseJournalUsage=%d want %d", got, 240*(1<<20))
	}
}

func TestParseBrewFreed(t *testing.T) {
	out := "Removing: /Users/x/Library/Caches/Homebrew/foo... (1.5GB)\n==> This operation would free approximately 1.5GB of disk space."
	if got := parseBrewFreed(out); got != int64(1.5*float64(1<<30)) {
		t.Errorf("parseBrewFreed=%d", got)
	}
}

// The original cleanup flagged any repeated line as a duplicate, so deb822
// .sources files (which all repeat Types:/Components:) produced false positives.
// The deb822-aware audit must only flag a genuinely duplicated repo.
func TestAuditAPTReposDeb822NoFalsePositive(t *testing.T) {
	dir := t.TempDir()
	ubuntu := "Types: deb\nURIs: http://archive.ubuntu.com/ubuntu\nSuites: noble noble-updates\nComponents: main restricted\nEnabled: yes\n"
	security := "Types: deb\nURIs: http://security.ubuntu.com/ubuntu\nSuites: noble-security\nComponents: main restricted\nEnabled: yes\n"
	write(t, dir, "ubuntu.sources", ubuntu)
	write(t, dir, "security.sources", security)

	if dups := auditAPTRepos(dir); len(dups) != 0 {
		t.Errorf("expected no duplicates across distinct deb822 repos, got %v", dups)
	}
}

func TestAuditAPTReposFindsRealDuplicate(t *testing.T) {
	dir := t.TempDir()
	line := "deb [arch=amd64] https://cli.github.com/packages stable main\n"
	write(t, dir, "github.list", line)
	write(t, dir, "github-copy.list", line)
	write(t, dir, "vscode.list", "deb https://packages.microsoft.com/repos/code stable main\n")

	dups := auditAPTRepos(dir)
	if len(dups) != 1 {
		t.Fatalf("expected 1 duplicate, got %d (%v)", len(dups), dups)
	}
	if len(dups[0].Files) != 2 {
		t.Errorf("expected the dup in 2 files, got %v", dups[0].Files)
	}
}

func TestAuditAPTReposCrossFormatDuplicate(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "legacy.list", "deb https://cli.github.com/packages stable main\n")
	write(t, dir, "modern.sources", "Types: deb\nURIs: https://cli.github.com/packages\nSuites: stable\nComponents: main\n")

	if dups := auditAPTRepos(dir); len(dups) != 1 {
		t.Errorf("expected the same repo in .list and .sources to be flagged, got %v", dups)
	}
}

func TestParseSourcesReposSkipsDisabled(t *testing.T) {
	disabled := "Types: deb\nURIs: http://example.com/repo\nSuites: stable\nComponents: main\nEnabled: no\n"
	if keys := parseSourcesRepos(disabled); len(keys) != 0 {
		t.Errorf("disabled stanza should yield no repos, got %v", keys)
	}
}

func TestCatalogGating(t *testing.T) {
	// Unknown OS yields no tasks; Linux/macOS yield a non-empty catalog whose
	// IDs are unique.
	if Catalog(platform.Info{OS: platform.Unknown}) != nil {
		t.Error("unknown OS should have no cleanup tasks")
	}
	for _, os := range []platform.OS{platform.Linux, platform.MacOS} {
		tasks := Catalog(platform.Info{OS: os, PackageManager: pmFor(os)})
		if len(tasks) == 0 {
			t.Errorf("%s catalog is empty", os)
		}
		seen := map[string]bool{}
		for _, tk := range tasks {
			if seen[tk.ID] {
				t.Errorf("%s: duplicate task ID %q", os, tk.ID)
			}
			seen[tk.ID] = true
			if tk.Risk != ReportOnly && tk.Run == nil {
				t.Errorf("%s: actionable task %q has no Run", os, tk.ID)
			}
		}
	}
}

func pmFor(os platform.OS) string {
	if os == platform.MacOS {
		return "brew"
	}
	return "apt"
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

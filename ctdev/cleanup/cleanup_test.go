package cleanup

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Warm platform.Detect() before any test swaps PATH under -shuffle=on, so
// PackageManager detection never caches "unknown" from an empty stub PATH.
func TestMain(m *testing.M) {
	platform.Detect()
	os.Exit(m.Run())
}

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

func TestRotatedLogFiles_FindsNestedGzAndTopLevelRotated(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "apt", "deep")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, root, "syslog.1", "a")
	write(t, root, "kern.old", "b")
	write(t, root, "auth.log.2.gz", "c")
	write(t, root, "syslog", "keep")
	write(t, sub, "history.log.1.gz", "d")
	write(t, sub, "term.log.1", "nested-not-gz")

	got := rotatedLogFiles(root)
	slices.Sort(got)
	want := []string{
		filepath.Join(sub, "history.log.1.gz"),
		filepath.Join(root, "auth.log.2.gz"),
		filepath.Join(root, "kern.old"),
		filepath.Join(root, "syslog.1"),
	}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("rotatedLogFiles() = %v, want %v", got, want)
	}
}

func TestPinRunningKernel_FailsSafeWhenUnameFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stub")
	}
	// Empty PATH: uname fails -> pinRunningKernel must return an error rather
	// than letting apt autoremove --purge proceed without pinning the kernel.
	t.Setenv("PATH", t.TempDir())
	err := pinRunningKernel(context.Background(), sysutil.Opts{Stdout: &bytes.Buffer{}, DryRun: true})
	if err == nil {
		t.Fatal("expected error when uname -r fails, got nil")
	}
}

func TestPinRunningKernel_PinsInstalledKernelOnly(t *testing.T) {
	// Package lookups go through the detected package manager, so the
	// dpkg-query stub only answers where that is apt.
	if runtime.GOOS != "linux" {
		t.Skip("apt kernel pinning is Linux-only")
	}
	binDir := t.TempDir()
	writeExec := func(name, body string) {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExec("uname", "#!/bin/sh\necho 6.8.0-test\n")
	writeExec("dpkg-query", "#!/bin/sh\ncase \"$3\" in linux-image-6.8.0-test) echo 'installed 6.8.0'; exit 0 ;; *) exit 1 ;; esac\n")
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	if err := pinRunningKernel(context.Background(), sysutil.Opts{Stdout: &out, DryRun: true}); err != nil {
		t.Fatalf("pinRunningKernel: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "apt-mark manual linux-image-6.8.0-test") {
		t.Errorf("expected linux-image to be pinned; got:\n%s", got)
	}
	if strings.Contains(got, "linux-headers-6.8.0-test") {
		t.Errorf("uninstalled linux-headers should not be pinned; got:\n%s", got)
	}
}

func TestRemoveAllAndEmptyDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, sub, "a.txt", "hi")

	// DryRun with nil Stdout must not panic and must not delete anything.
	if err := removeAll(sysutil.Opts{DryRun: true}, sub); err != nil {
		t.Fatalf("removeAll dry-run nil stdout: %v", err)
	}
	if err := emptyDir(sysutil.Opts{DryRun: true}, sub); err != nil {
		t.Fatalf("emptyDir dry-run nil stdout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, "a.txt")); err != nil {
		t.Fatalf("dry-run deleted file: %v", err)
	}

	// emptyDir deletes contents while preserving the directory itself.
	if err := emptyDir(sysutil.Opts{}, sub, filepath.Join(dir, "nonexistent")); err != nil {
		t.Fatalf("emptyDir: %v", err)
	}
	entries, err := os.ReadDir(sub)
	if err != nil || len(entries) != 0 {
		t.Errorf("expected empty existing dir, got entries=%v err=%v", entries, err)
	}

	// removeAll removes the directory itself.
	if err := removeAll(sysutil.Opts{}, sub); err != nil {
		t.Fatalf("removeAll: %v", err)
	}
	if _, err := os.Stat(sub); !os.IsNotExist(err) {
		t.Errorf("expected %s removed, stat err=%v", sub, err)
	}
}

func TestSudoRemoveGlob_InsertsDoubleDash(t *testing.T) {
	dir := t.TempDir()
	dashFile := filepath.Join(dir, "-weird.gz")
	if err := os.WriteFile(dashFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := sudoRemoveGlob(context.Background(), sysutil.Opts{Stdout: &out, DryRun: true}, filepath.Join(dir, "*.gz")); err != nil {
		t.Fatalf("sudoRemoveGlob: %v", err)
	}
	if !strings.Contains(out.String(), "rm -f -- "+dashFile) {
		t.Errorf("expected -- separator before glob matches, got:\n%s", out.String())
	}
}

func TestParsersWithStubCommands(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stub")
	}
	binDir := t.TempDir()
	writeExec := func(name, body string) {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExec("docker", "#!/bin/sh\nprintf '1.5GB (50%%)\\n512MB (100%%)\\n'\n")
	writeExec("snap", "#!/bin/sh\nprintf 'Name Version Rev Tracking Publisher Notes\\nfirefox 120 100 latest/stable mozilla -\\nfirefox 119 99 latest/stable mozilla disabled\\n'\n")
	writeExec("dpkg", "#!/bin/sh\nprintf 'ii  curl 8.0 amd64\\nrc  old-pkg 1.0 amd64 removed\\n'\n")
	writeExec("tmutil", "#!/bin/sh\nprintf 'Snapshots for disk /:\\ncom.apple.TimeMachine.2026-06-26-120000.local\\n'\n")
	t.Setenv("PATH", binDir)

	ctx := context.Background()
	wantDocker := int64(1.5*float64(1<<30)) + 512*(1<<20)
	if got := dockerReclaimable(ctx); got != wantDocker {
		t.Errorf("dockerReclaimable() = %d, want %d", got, wantDocker)
	}
	if snaps := disabledSnaps(ctx); len(snaps) != 1 || snaps[0].name != "firefox" || snaps[0].rev != "99" {
		t.Errorf("disabledSnaps() = %+v, want [{firefox 99}]", snaps)
	}
	if pkgs := rcPackages(ctx); !slices.Equal(pkgs, []string{"old-pkg"}) {
		t.Errorf("rcPackages() = %v, want [old-pkg]", pkgs)
	}
	if tm := localSnapshots(ctx); !slices.Equal(tm, []string{"2026-06-26-120000"}) {
		t.Errorf("localSnapshots() = %v, want [2026-06-26-120000]", tm)
	}

	// Unreachable docker returns -1.
	t.Setenv("PATH", t.TempDir())
	if got := dockerReclaimable(ctx); got != -1 {
		t.Errorf("dockerReclaimable without docker = %d, want -1", got)
	}
}

func TestParseSourcesAndListReposEdgeCases(t *testing.T) {
	crlf := "Types: deb\r\nURIs: https://a.example.invalid https://b.example.invalid\r\nSuites: s1 s2\r\nEnabled: yes\r\n\r\n" +
		"Types: deb\r\nURIs: https://c.example.invalid\r\nSuites: s3\r\nEnabled: false\r\n"
	got := parseSourcesRepos(crlf)
	want := []string{
		"https://a.example.invalid s1",
		"https://a.example.invalid s2",
		"https://b.example.invalid s1",
		"https://b.example.invalid s2",
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseSourcesRepos(CRLF) = %v, want %v", got, want)
	}

	list := "# comment\ndeb-src [signed-by=/etc/apt/key.gpg arch=amd64] https://a.example.invalid noble main\nbadline\n"
	if got := parseListRepos(list); !slices.Equal(got, []string{"https://a.example.invalid noble"}) {
		t.Errorf("parseListRepos() = %v", got)
	}

	// Error paths for size/apt/journal parsers.
	for _, bad := range []string{"", "100 XB", "abc MB"} {
		if got := parseSize(bad); got != -1 {
			t.Errorf("parseSize(%q) = %d, want -1", bad, got)
		}
	}
	if got := parseJournalUsage("no marker here"); got != -1 {
		t.Errorf("parseJournalUsage(invalid) = %d, want -1", got)
	}
}

func TestScanAll_RespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tasks := []Task{
		{ID: "a", Scan: func(c context.Context) ScanResult {
			if c.Err() != nil {
				return ScanResult{Bytes: -1, Note: "cancelled"}
			}
			return ScanResult{Bytes: 100}
		}},
		{ID: "nil-scan"},
	}
	res := ScanAll(ctx, tasks)
	if res["a"].Note != "cancelled" {
		t.Errorf("ScanAll did not forward cancelled ctx: %+v", res)
	}
	if _, ok := res["nil-scan"]; ok {
		t.Error("task with nil Scan should be omitted from results")
	}
}

func TestCatalogNeedsRootMetadata(t *testing.T) {
	tasks := Catalog(platform.Info{OS: platform.Linux, PackageManager: "apt"})
	byID := map[string]Task{}
	for _, tk := range tasks {
		byID[tk.ID] = tk
	}
	if tk, ok := byID["thumbnails"]; !ok || tk.NeedsRoot {
		t.Errorf("thumbnails should be a non-root task: %+v", tk)
	}
	if tk, ok := byID["rotated-logs"]; !ok || !tk.NeedsRoot {
		t.Errorf("rotated-logs should require root: %+v", tk)
	}
}

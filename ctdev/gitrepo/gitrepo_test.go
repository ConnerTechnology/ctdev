package gitrepo

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testFile = ".claude/settings.local.json"

func initRepo(t *testing.T) Repo {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	r, err := Find(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeFile creates a repo-relative file, with its directory.
func writeFile(t *testing.T, r Repo, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(r.Path(rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(r.Path(rel), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindOutsideRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	if _, err := Find(context.Background(), t.TempDir()); !errors.Is(err, ErrNotARepo) {
		t.Errorf("got %v, want ErrNotARepo", err)
	}
}

// Anything other than "not a repo" is reported as itself, not dressed up as
// one: here git is not on PATH at all.
func TestFindGitMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := Find(context.Background(), t.TempDir())
	if err == nil || errors.Is(err, ErrNotARepo) {
		t.Fatalf("got %v, want a real error", err)
	}
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("error should wrap exec.ErrNotFound: %v", err)
	}
}

func TestAppendExcludeIsIdempotent(t *testing.T) {
	// The default global excludes file is under XDG_CONFIG_HOME, and may
	// already ignore the file on a developer's machine.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	r := initRepo(t)
	ctx := context.Background()
	exclude, err := r.ExcludeFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(exclude) {
		t.Errorf("exclude path %q is not absolute", exclude)
	}

	writeFile(t, r, testFile)
	if r.IsIgnored(ctx, testFile) {
		t.Fatal("file ignored before anything was excluded")
	}

	added, err := AppendExclude(exclude, testFile)
	if err != nil || !added {
		t.Fatalf("first append: %v %v", added, err)
	}
	added, err = AppendExclude(exclude, testFile)
	if err != nil || added {
		t.Fatalf("second append: %v %v", added, err)
	}
	data, _ := os.ReadFile(exclude)
	if n := strings.Count(string(data), testFile); n != 1 {
		t.Errorf("exclude names the file %d times:\n%s", n, data)
	}
	if !r.IsIgnored(ctx, testFile) {
		t.Error("git does not ignore the file after the append")
	}
}

func TestAppendExcludeAddsNewlineAndSeesAnchoredLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "info", "exclude")
	if added, err := AppendExclude(path, testFile); err != nil || !added {
		t.Fatalf("missing file: %v %v", added, err)
	}

	path = filepath.Join(t.TempDir(), "exclude")
	_ = os.WriteFile(path, []byte("*.log"), 0o644)
	if _, err := AppendExclude(path, testFile); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); string(data) != "*.log\n"+testFile+"\n" {
		t.Errorf("got %q", data)
	}

	_ = os.WriteFile(path, []byte("/"+testFile+"\n"), 0o644)
	if added, _ := AppendExclude(path, testFile); added {
		t.Error("an anchored line should count as present")
	}
}

func TestExcludeFileInWorktree(t *testing.T) {
	r := initRepo(t)
	runGit(t, r.Root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-q", "--allow-empty", "-m", "init")
	wt := filepath.Join(t.TempDir(), "wt")
	runGit(t, r.Root, "worktree", "add", "-q", wt)
	w, err := Find(context.Background(), wt)
	if err != nil {
		t.Fatal(err)
	}
	exclude, err := w.ExcludeFile(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AppendExclude(exclude, testFile); err != nil {
		t.Fatal(err)
	}
	writeFile(t, w, testFile)
	if !w.IsIgnored(context.Background(), testFile) {
		t.Errorf("worktree does not ignore the file via %s", exclude)
	}
}

func TestIsTracked(t *testing.T) {
	r := initRepo(t)
	writeFile(t, r, testFile)
	ctx := context.Background()
	if r.IsTracked(ctx, testFile) {
		t.Fatal("untracked file reported as tracked")
	}
	runGit(t, r.Root, "add", "-f", testFile)
	if !r.IsTracked(ctx, testFile) {
		t.Error("staged file reported as untracked")
	}
}

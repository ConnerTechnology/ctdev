package cmd

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/claudecli/claudeclitest"
)

// repoSetupFakes stands in for everything outside the repo: the tools are
// installed, gh is signed in or not, claude answers as a fresh install, and
// launching Claude Code records what it would start.
type repoSetupFakes struct {
	claude   *claudeclitest.Fake
	logins   int
	launched []string
	// askpass is SUDO_ASKPASS as the launched session would see it.
	askpass string
}

func useRepoSetupFakes(t *testing.T, repo string, loggedIn bool) *repoSetupFakes {
	t.Helper()
	useFakeTool(t, true)
	f := &repoSetupFakes{claude: claudeclitest.New(t)}
	f.claude.Reply("plugin-marketplace-add", `{"outcome":"ok","message":"Successfully added marketplace"}`, 0)
	f.claude.Reply("plugin-install", `{"outcome":"ok","message":"Successfully installed"}`, 0)
	list, _ := json.Marshal([]map[string]any{{"id": pluginID, "scope": "local", "enabled": true, "projectPath": repo}})
	f.claude.Reply("plugin-list", string(list), 0)

	origIn, origLogin, origLaunch := ghLoggedIn, ghLogin, launchClaude
	ghLoggedIn = func(context.Context) bool { return loggedIn }
	ghLogin = func(context.Context) error { f.logins++; loggedIn = true; return nil }
	launchClaude = func(root, prompt string) error {
		f.launched = append(f.launched, root, prompt)
		f.askpass = os.Getenv("SUDO_ASKPASS")
		return nil
	}
	t.Cleanup(func() { ghLoggedIn, ghLogin, launchClaude = origIn, origLogin, origLaunch })
	return f
}

// cloneLikeRepo is a repo with one commit and nothing ignoring
// .claude/settings.local.json, like a fresh clone of someone else's project.
func cloneLikeRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no global excludes file
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	repo := tempGitRepo(t)
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("# Example Vendor tool\n"), 0o644)
	run("add", "README.md")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-q", "-m", "init")
	return repo
}

func gitStatus(t *testing.T, repo string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", repo, "status", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	return string(out)
}

func TestRepoSetupInstallsLocallyAndStartsTheSkill(t *testing.T) {
	repo := cloneLikeRepo(t)
	f := useRepoSetupFakes(t, repo, true)
	t.Chdir(repo)
	t.Setenv("GIT_CONFIG_COUNT", "")
	// What Execute sets up for ctdev itself, which the session must not get.
	script := filepath.Join(t.TempDir(), "ctdev-askpass-test.sh")
	_ = os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o700)
	t.Setenv("SUDO_ASKPASS", script)
	askpassPath, ctdevEnv = script, []string{"SUDO_ASKPASS"}
	t.Cleanup(func() { askpassPath, ctdevEnv = "", nil })

	var err error
	out := captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if f.askpass != "" {
		t.Errorf("the session would inherit SUDO_ASKPASS=%s", f.askpass)
	}
	if _, err := os.Stat(script); !os.IsNotExist(err) {
		t.Error("the askpass script was left behind")
	}
	if env := f.claude.GitConfigEnv(); !strings.Contains(env, "GIT_CONFIG_VALUE_0=!gh auth git-credential") {
		t.Errorf("the marketplace clone didn't get gh's credentials:\n%s", env)
	}
	want := []string{
		"plugin marketplace add " + pluginMarketplace + " --scope local --json",
		"plugin install " + pluginID + " --scope local --json",
		"plugin list --json",
	}
	if got := f.claude.Calls(); !reflect.DeepEqual(got, want) {
		t.Errorf("calls %q\nwant %q", got, want)
	}
	if !reflect.DeepEqual(f.launched, []string{repo, setupSkill}) {
		t.Errorf("launched %q", f.launched)
	}

	// What Claude Code then writes stays out of git.
	_ = os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	_ = os.WriteFile(filepath.Join(repo, settingsLocalFile), []byte(`{"enabledPlugins":{}}`), 0o644)
	if st := gitStatus(t, repo); st != "" {
		t.Errorf("git status not clean:\n%s", st)
	}
	exclude, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))

	// A second run makes the same idempotent calls and changes no file.
	captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude")); string(again) != string(exclude) {
		t.Errorf("exclude changed on rerun:\n%s\n---\n%s", exclude, again)
	}
	if n := strings.Count(string(exclude), settingsLocalFile); n != 1 {
		t.Errorf("exclude names %s %d times", settingsLocalFile, n)
	}
}

func TestRepoSetupUpdatesAStalePlugin(t *testing.T) {
	repo := cloneLikeRepo(t)
	f := useRepoSetupFakes(t, repo, true)
	f.claude.Reply("plugin-install", `{"outcome":"ok","message":"already installed","installedVersion":"1.0.0","availableVersion":"1.1.0"}`, 0)
	t.Chdir(repo)
	var err error
	captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err != nil {
		t.Fatal(err)
	}
	if calls := f.claude.Calls(); len(calls) != 4 || calls[2] != "plugin update "+pluginID+" --scope local" {
		t.Errorf("calls %q", calls)
	}
}

func TestRepoSetupSignsGhInOrStopsInBatch(t *testing.T) {
	repo := cloneLikeRepo(t)
	f := useRepoSetupFakes(t, repo, false)
	t.Chdir(repo)
	var err error
	captureStdout(t, func() { err = setupRepo(context.Background(), true) })
	if err == nil || !strings.Contains(err.Error(), "gh auth login") || len(f.claude.Calls()) != 0 {
		t.Fatalf("batch, signed out: err %v, calls %q", err, f.claude.Calls())
	}

	out := captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err != nil || f.logins != 1 {
		t.Fatalf("interactive: err %v, logins %d\n%s", err, f.logins, out)
	}
}

func TestRepoSetupBatchStopsBeforeClaude(t *testing.T) {
	repo := cloneLikeRepo(t)
	f := useRepoSetupFakes(t, repo, true)
	t.Chdir(repo)
	var err error
	out := captureStdout(t, func() { err = setupRepo(context.Background(), true) })
	if err != nil {
		t.Fatal(err)
	}
	if len(f.launched) != 0 || !strings.Contains(out, `claude "`+setupSkill+`"`) {
		t.Errorf("launched %q; output:\n%s", f.launched, out)
	}
}

func TestRepoSetupFailsWhenThePluginIsNotListed(t *testing.T) {
	repo := cloneLikeRepo(t)
	f := useRepoSetupFakes(t, repo, true)
	f.claude.Reply("plugin-list", `[]`, 0)
	t.Chdir(repo)
	var err error
	captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err == nil || len(f.launched) != 0 {
		t.Errorf("err %v, launched %q", err, f.launched)
	}
}

func TestRepoSetupRefusesACommittedSettingsLocal(t *testing.T) {
	repo := cloneLikeRepo(t)
	f := useRepoSetupFakes(t, repo, true)
	_ = os.MkdirAll(filepath.Join(repo, ".claude"), 0o755)
	_ = os.WriteFile(filepath.Join(repo, settingsLocalFile), []byte("{}\n"), 0o644)
	if out, err := exec.Command("git", "-C", repo, "add", "-f", settingsLocalFile).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	t.Chdir(repo)
	var err error
	captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err == nil || !strings.Contains(err.Error(), "git rm --cached") || len(f.claude.Calls()) != 0 {
		t.Errorf("err %v, calls %q", err, f.claude.Calls())
	}
}

func TestRepoSetupOutsideARepo(t *testing.T) {
	useRepoSetupFakes(t, "", true)
	t.Chdir(t.TempDir())
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	var err error
	captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Errorf("got %v", err)
	}
}

func TestKeepBytesIfSame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.local.json")
	before := []byte("{\n  \"b\": 1,\n  \"a\": 2\n}\n")

	_ = os.WriteFile(path, []byte(`{"a":2,"b":1}`), 0o600)
	if err := keepBytesIfSame(path, before); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(before) {
		t.Errorf("reordered file not restored: %q", got)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %o", fi.Mode().Perm())
	}

	changed := []byte(`{"a":2,"b":1,"c":3}`)
	_ = os.WriteFile(path, changed, 0o644)
	if err := keepBytesIfSame(path, before); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != string(changed) {
		t.Errorf("a real change was undone: %q", got)
	}
}

func TestRepoSetupDryRunChangesNothing(t *testing.T) {
	repo := cloneLikeRepo(t)
	f := useRepoSetupFakes(t, repo, false)
	t.Chdir(repo)
	flagDryRun = true
	t.Cleanup(func() { flagDryRun = false })
	var err error
	out := captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err != nil {
		t.Fatal(err)
	}
	if f.logins != 0 || len(f.claude.Calls()) != 0 || len(f.launched) != 0 {
		t.Errorf("dry run acted: logins %d, calls %q, launched %q", f.logins, f.claude.Calls(), f.launched)
	}
	if exclude, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude")); strings.Contains(string(exclude), settingsLocalFile) {
		t.Error("dry run wrote the exclude file")
	}
	if !strings.Contains(out, "gh auth login") {
		t.Errorf("dry run should say it would sign gh in:\n%s", out)
	}
}

func TestRepoSetupStopsWhenAToolIsDeclined(t *testing.T) {
	repo := cloneLikeRepo(t)
	f := useRepoSetupFakes(t, repo, true)
	useFakeTool(t, false)
	feedStdin(t, "n\n")
	t.Chdir(repo)
	var err error
	captureStdout(t, func() { err = setupRepo(context.Background(), false) })
	if err == nil || !strings.Contains(err.Error(), "needs Git") || len(f.claude.Calls()) != 0 {
		t.Errorf("err %v, calls %q", err, f.claude.Calls())
	}
}

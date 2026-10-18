package cmd

import (
	"bufio"
	"context"
	"strings"
	"testing"
)

// fakeTool stands in for a component a configure category needs: installed
// says whether it starts out present, and installing it records the call and
// makes it present.
type fakeTool struct {
	installed bool
	installs  []string
}

func useFakeTool(t *testing.T, installed bool) *fakeTool {
	t.Helper()
	f := &fakeTool{installed: installed}
	origInstalled, origInstall := toolInstalled, installTool
	toolInstalled = func(string) bool { return f.installed }
	installTool = func(_ context.Context, name string) error {
		f.installs = append(f.installs, name)
		f.installed = true
		return nil
	}
	t.Cleanup(func() { toolInstalled, installTool = origInstalled, origInstall })
	return f
}

func useStdin(t *testing.T, input string) {
	t.Helper()
	orig := stdinScanner
	stdinScanner = bufio.NewScanner(strings.NewReader(input))
	t.Cleanup(func() { stdinScanner = orig })
}

func TestEnsureToolInstalledDoesNothing(t *testing.T) {
	f := useFakeTool(t, true)
	useStdin(t, "")
	ok, err := ensureTool(context.Background(), "aws", "AWS CLI", false)
	if !ok || err != nil || len(f.installs) != 0 {
		t.Errorf("ok=%v err=%v installs=%v; want no prompt and no install", ok, err, f.installs)
	}
}

func TestEnsureToolAsksThenInstalls(t *testing.T) {
	for _, reply := range []string{"y\n", "\n"} { // Enter takes the [Y/n] default
		f := useFakeTool(t, false)
		useStdin(t, reply)
		var ok bool
		var err error
		out := captureStdout(t, func() {
			ok, err = ensureTool(context.Background(), "aws", "AWS CLI", false)
		})
		if !strings.Contains(out, "AWS CLI isn't installed. Install it now?") || !strings.Contains(out, "[Y/n]") {
			t.Errorf("reply %q: expected the install prompt, got:\n%s", reply, out)
		}
		if !ok || err != nil || strings.Join(f.installs, ",") != "aws" {
			t.Errorf("reply %q: ok=%v err=%v installs=%v; want aws installed", reply, ok, err, f.installs)
		}
	}
}

func TestEnsureToolDeclined(t *testing.T) {
	f := useFakeTool(t, false)
	useStdin(t, "n\n")
	var ok bool
	var err error
	out := captureStdout(t, func() {
		ok, err = ensureTool(context.Background(), "git", "Git", false)
	})
	if ok || err != nil || len(f.installs) != 0 {
		t.Errorf("ok=%v err=%v installs=%v; want stop without installing", ok, err, f.installs)
	}
	if !strings.Contains(out, "ctdev install git") {
		t.Errorf("a decline should say how to install later, got:\n%s", out)
	}
}

// Batch installs without reading stdin: an empty reader would otherwise come
// back as a cancelled prompt.
func TestEnsureToolBatchInstallsWithoutAsking(t *testing.T) {
	f := useFakeTool(t, false)
	useStdin(t, "")
	var ok bool
	var err error
	out := captureStdout(t, func() {
		ok, err = ensureTool(context.Background(), "git", "Git", true)
	})
	if !ok || err != nil || strings.Join(f.installs, ",") != "git" {
		t.Errorf("ok=%v err=%v installs=%v; want git installed", ok, err, f.installs)
	}
	if strings.Contains(out, "[Y/n]") {
		t.Errorf("batch mode must not prompt, got:\n%s", out)
	}
}

func TestEnsureToolStillMissingAfterInstall(t *testing.T) {
	f := useFakeTool(t, false)
	installTool = func(_ context.Context, name string) error {
		f.installs = append(f.installs, name)
		return nil
	}
	if ok, err := ensureTool(context.Background(), "aws", "AWS CLI", true); ok || err == nil || !strings.Contains(err.Error(), "PATH") {
		t.Errorf("ok=%v err=%v; want a not-on-PATH error", ok, err)
	}
}

func useBatch(t *testing.T) {
	t.Helper()
	flagBatch = true
	t.Cleanup(func() { flagBatch = false })
}

// `configure aws --batch` on a machine without the CLI installs it, then goes
// on to the profile step.
func TestConfigureAWSBatchInstallsCLI(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	useBatch(t)
	f := useFakeTool(t, false)
	out := captureStdout(t, func() {
		if err := configureAWS(context.Background()); err != nil {
			t.Errorf("configureAWS: %v", err)
		}
	})
	if strings.Join(f.installs, ",") != "aws" {
		t.Errorf("installs = %v, want [aws]", f.installs)
	}
	if !strings.Contains(out, "No AWS profiles found") {
		t.Errorf("expected the wizard to continue to profiles, got:\n%s", out)
	}
}

func TestConfigureGitBatchInstallsGit(t *testing.T) {
	useBatch(t)
	f := useFakeTool(t, false)
	err := configureGit(context.Background())
	if strings.Join(f.installs, ",") != "git" {
		t.Errorf("installs = %v, want [git]", f.installs)
	}
	// With git in place, batch still needs --name/--email, as before.
	if err == nil || !strings.Contains(err.Error(), "--name and --email") {
		t.Errorf("err = %v, want the batch-mode flags error", err)
	}
}

// --show reports what's set; it never installs anything.
func TestConfigureShowNeverInstalls(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	useBatch(t)
	flagConfigShow = true
	t.Cleanup(func() { flagConfigShow = false })
	f := useFakeTool(t, false)
	captureStdout(t, func() {
		_ = configureAWS(context.Background())
		_ = configureGit(context.Background())
	})
	if len(f.installs) != 0 {
		t.Errorf("--show installed %v", f.installs)
	}
}

// `ctdev install aws` and `ctdev install git` chain into their wizards.
func TestInstallChainsIntoAWSAndGitWizards(t *testing.T) {
	for _, name := range []string{"aws", "git"} {
		if !componentHasConfigure(name) || componentWizards[name] == nil {
			t.Errorf("install %s should run its configure wizard", name)
		}
	}
}

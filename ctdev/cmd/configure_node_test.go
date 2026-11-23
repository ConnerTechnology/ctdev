package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeNodenv is a nodenv in a temp root: it answers the --list reads from
// node-build's lists, records every change in calls, and installs,
// uninstalls and sets the global version on disk the way nodenv does.
type fakeNodenv struct {
	root string
}

func useFakeNodenv(t *testing.T, list, listAll string) *fakeNodenv {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".nodenv")
	t.Setenv("HOME", home)
	t.Setenv("NODENV_ROOT", root)
	t.Setenv("NODENV_VERSION", "")
	script := `#!/bin/sh
root="` + root + `"
case "$1 $2" in
"install --list") printf '` + list + `'; exit 0 ;;
"install --list-all") printf '` + listAll + `'; exit 0 ;;
esac
echo "${NODENV_VERSION:+NODENV_VERSION=$NODENV_VERSION }${0##*/} $*" >> "$root/calls"
case "$1" in
install) mkdir -p "$root/versions/$3" ;;
uninstall) rm -rf "$root/versions/$3" ;;
global) echo "$2" > "$root/version" ;;
esac
`
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "nodenv"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return &fakeNodenv{root: root}
}

// version installs v with the given global packages (name@version).
func (f *fakeNodenv) version(t *testing.T, v string, pkgs ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(f.root, "versions", v, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, spec := range pkgs {
		at := strings.LastIndex(spec, "@")
		dir := filepath.Join(f.root, "versions", v, "lib", "node_modules", spec[:at])
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"version":"`+spec[at+1:]+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *fakeNodenv) global(t *testing.T, v string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.root, "version"), []byte(v+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeNodenv) calls(t *testing.T) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(f.root, "calls"))
	return string(b)
}

func (f *fakeNodenv) installed(v string) bool {
	_, err := os.Stat(filepath.Join(f.root, "versions", v))
	return err == nil
}

func useNodeScope(t *testing.T, global, project bool) {
	t.Helper()
	flagNodeGlobal, flagNodeProject = global, project
	t.Cleanup(func() { flagNodeGlobal, flagNodeProject = false, false })
}

// gitProject makes a git repository with a subfolder and moves into the
// subfolder.
func gitProject(t *testing.T) (top string) {
	t.Helper()
	top = t.TempDir()
	if out, err := exec.Command("git", "init", "-q", top).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	sub := filepath.Join(top, "web")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	top, _ = filepath.EvalSymlinks(top)
	return top
}

func writePin(t *testing.T, dir, v string) string {
	t.Helper()
	p := filepath.Join(dir, ".node-version")
	if err := os.WriteFile(p, []byte(v+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const supported = `22.23.5\n24.22.3\n26.14.0\n`

func TestNodeUpdateGlobalMovesPackagesAndDeletesOld(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.version(t, "26.13.1", "ccusage@18.0.3", "@openai/codex@0.171.0", "npm@11.0.0")
	f.global(t, "26.13.1")
	t.Chdir(t.TempDir())

	out := captureStdout(t, func() {
		if err := runNodeUpdate(context.Background(), true); err != nil {
			t.Errorf("update: %v", err)
		}
	})
	plan := `  Updating: Global (no .node-version here)
  Node 26.13.1 → 26.14.0 (latest v26)

  This will:
    1. Install 26.14.0
    2. Reinstall 26.13.1's 2 global npm packages under 26.14.0, at the same versions:
         @openai/codex@0.171.0, ccusage@18.0.3
    3. Make 26.14.0 the global version
    4. Delete 26.13.1 (nodenv uninstall)
`
	if !strings.Contains(out, plan) {
		t.Errorf("plan:\n%s\nwant it to contain:\n%s", out, plan)
	}
	want := "nodenv install --skip-existing 26.14.0\n" +
		"NODENV_VERSION=26.14.0 nodenv exec npm install -g @openai/codex@0.171.0 ccusage@18.0.3\n" +
		"nodenv global 26.14.0\nnodenv rehash\nnodenv uninstall -f 26.13.1\n"
	if got := f.calls(t); got != want {
		t.Errorf("calls:\n%s\nwant:\n%s", got, want)
	}
}

// --global from a folder whose .node-version pins the global version keeps
// it: deleting it would break node in this folder.
func TestNodeUpdateGlobalKeepsVersionPinnedHere(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.version(t, "26.13.1")
	f.global(t, "26.13.1")
	dir := t.TempDir()
	writePin(t, dir, "26.13.1")
	t.Chdir(dir)
	useNodeScope(t, true, false)

	out := captureStdout(t, func() {
		if err := runNodeUpdate(context.Background(), true); err != nil {
			t.Errorf("update: %v", err)
		}
	})
	if !strings.Contains(out, "    3. Keep 26.13.1: the .node-version here pins it\n") {
		t.Errorf("expected the keep step; got:\n%s", out)
	}
	if strings.Contains(f.calls(t), "uninstall") || !f.installed("26.13.1") {
		t.Errorf("26.13.1 was deleted; calls:\n%s", f.calls(t))
	}
}

// A project update rewrites the governing .node-version, found upward, and
// keeps the old version while it is still the global one.
func TestNodeUpdateProjectRewritesPinAndKeepsGlobal(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.version(t, "24.21.0", "ccusage@18.0.3")
	f.global(t, "24.21.0")
	top := gitProject(t)
	pin := writePin(t, top, "24.21.0")

	out := captureStdout(t, func() {
		if err := runNodeUpdate(context.Background(), true); err != nil {
			t.Errorf("update: %v", err)
		}
	})
	for _, want := range []string{
		"  Updating: This project (" + pin + ")\n",
		"    3. Write 24.22.3 to " + pin + "\n",
		"    4. Keep 24.21.0: it is still the global version\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q; got:\n%s", want, out)
		}
	}
	if b, _ := os.ReadFile(pin); string(b) != "24.22.3\n" {
		t.Errorf(".node-version = %q, want 24.22.3", b)
	}
	if calls := f.calls(t); strings.Contains(calls, "global") || strings.Contains(calls, "uninstall") {
		t.Errorf("a project update changed the global version or deleted one:\n%s", calls)
	}
}

func TestNodeUpdateNothingToDo(t *testing.T) {
	f := useFakeNodenv(t, `26.13.1\n`, "")
	f.version(t, "26.13.1")
	f.global(t, "26.13.1")
	t.Chdir(t.TempDir())

	out := captureStdout(t, func() {
		if err := runNodeUpdate(context.Background(), true); err != nil {
			t.Errorf("update: %v", err)
		}
	})
	if !strings.Contains(out, "Global Node 26.13.1 is the latest v26. Nothing to update.") {
		t.Errorf("got:\n%s", out)
	}
	if calls := f.calls(t); calls != "" {
		t.Errorf("nothing to update, but ran:\n%s", calls)
	}
}

// A major node-build has retired updates to its last release, with a pointer
// to a supported one.
func TestNodeUpdateRetiredMajor(t *testing.T) {
	f := useFakeNodenv(t, supported, `20.9.0\n20.20.2\n20.10.0\n22.23.5\n`)
	f.version(t, "20.20.1")
	f.global(t, "20.20.1")
	t.Chdir(t.TempDir())

	out := captureStdout(t, func() {
		if err := runNodeUpdate(context.Background(), true); err != nil {
			t.Errorf("update: %v", err)
		}
	})
	for _, want := range []string{
		"v20 is no longer supported (not in node-build's list). Updating to its last release, 20.20.2.",
		"Node 20.20.1 → 20.20.2 (last v20 release)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q; got:\n%s", want, out)
		}
	}
	if !strings.Contains(f.calls(t), "nodenv global 20.20.2\n") {
		t.Errorf("calls:\n%s", f.calls(t))
	}
}

func TestNodeUninstallRefusesGlobal(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.version(t, "26.13.1")
	f.global(t, "26.13.1")

	var err error
	captureStdout(t, func() { err = runNodeUninstall(context.Background(), "26.13.1", true) })
	want := "26.13.1 is the global version. Pick another global first: ctdev configure node install"
	if err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
	if !f.installed("26.13.1") {
		t.Error("the global version was deleted")
	}
}

// Picking from the list shows what goes with each version, warns when this
// folder pins it, and asks [y/N] before deleting.
func TestNodeUninstallPicksAndConfirms(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.version(t, "26.13.1")
	f.version(t, "24.22.3")
	f.version(t, "24.14.0", "ccusage@18.0.3", "typescript@5.9.2")
	f.global(t, "26.13.1")
	dir := t.TempDir()
	writePin(t, dir, "24.14.0")
	t.Chdir(dir)
	useStdin(t, "3\ny\n")

	out := captureStdout(t, func() {
		if err := runNodeUninstall(context.Background(), "", false); err != nil {
			t.Errorf("uninstall: %v", err)
		}
	})
	for _, want := range []string{
		"1) 26.13.1",
		"global",
		"3) 24.14.0",
		"2 global npm packages",
		"Its 2 global npm packages go with it: ccusage@18.0.3, typescript@5.9.2",
		"pins 24.14.0",
		"[y/N]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q; got:\n%s", want, out)
		}
	}
	if got := f.calls(t); got != "nodenv uninstall -f 24.14.0\n" {
		t.Errorf("calls:\n%s", got)
	}
}

// Making a new version global offers the old global's packages; batch copies
// them and keeps the old version.
func TestNodeInstallGlobalCopiesPackages(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.version(t, "26.13.1", "ccusage@18.0.3")
	f.global(t, "26.13.1")
	t.Chdir(t.TempDir())
	useNodeScope(t, true, false)

	out := captureStdout(t, func() {
		if err := runNodeInstall(context.Background(), "24", true); err != nil {
			t.Errorf("install: %v", err)
		}
	})
	if !strings.Contains(out, "26.13.1 has 1 global npm package: ccusage@18.0.3") {
		t.Errorf("expected the package list; got:\n%s", out)
	}
	want := "nodenv install --skip-existing 24.22.3\nnodenv global 24.22.3\n" +
		"NODENV_VERSION=24.22.3 nodenv exec npm install -g ccusage@18.0.3\nnodenv rehash\n"
	if got := f.calls(t); got != want {
		t.Errorf("calls:\n%s\nwant:\n%s", got, want)
	}
	if !f.installed("26.13.1") {
		t.Error("install deleted the old global version")
	}
}

// Inside a git repository the scope defaults to the project, and the pin goes
// at the repository's top, not the subfolder.
func TestNodeInstallDefaultsToProjectInRepo(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.version(t, "26.13.1")
	f.global(t, "26.13.1")
	top := gitProject(t)
	useStdin(t, "\n2\n") // Enter takes the scope default; 2 is 24.22.3

	out := captureStdout(t, func() {
		if err := runNodeInstall(context.Background(), "", false); err != nil {
			t.Errorf("install: %v", err)
		}
	})
	for _, want := range []string{
		"2) This project (" + top + ", writes .node-version)",
		"Choice [2]:",
		"1) 26.14.0",
		"2) 24.22.3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q; got:\n%s", want, out)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(top, ".node-version")); string(b) != "24.22.3\n" {
		t.Errorf("%s/.node-version = %q, want 24.22.3", top, b)
	}
	if strings.Contains(f.calls(t), "global") {
		t.Errorf("a project install changed the global version:\n%s", f.calls(t))
	}
}

func TestNodeInstallBatchNeedsVersion(t *testing.T) {
	useFakeNodenv(t, supported, "")
	if err := runNodeInstall(context.Background(), "", true); err == nil || !strings.Contains(err.Error(), "--batch needs a version") {
		t.Errorf("err = %v", err)
	}
}

// `ctdev install nodenv` with no Node yet offers to install one.
func TestInstallNodenvOffersANodeVersion(t *testing.T) {
	useFakeNodenv(t, supported, "")
	useStdin(t, "n\n")
	out := captureStdout(t, func() {
		if err := componentWizards["nodenv"](context.Background()); err != nil {
			t.Errorf("wizard: %v", err)
		}
	})
	if !strings.Contains(out, "No Node version installed yet. Install one now?") {
		t.Errorf("got:\n%s", out)
	}
}

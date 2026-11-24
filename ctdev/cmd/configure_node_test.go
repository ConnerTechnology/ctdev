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

// leftBehind gives version v an `npm link`ed package and one whose
// package.json has no version: neither can be reinstalled elsewhere.
func (f *fakeNodenv) leftBehind(t *testing.T, v, linked, unreadable string) {
	t.Helper()
	mods := filepath.Join(f.root, "versions", v, "lib", "node_modules")
	if err := os.MkdirAll(filepath.Join(mods, unreadable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(mods, linked)); err != nil {
		t.Fatal(err)
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

// Linked and unreadable packages can't be reinstalled under another version,
// so every place that moves or deletes packages names them.
func TestNodeCommandsNamePackagesNotMoved(t *testing.T) {
	linked := "Not moved (linked with npm link): my-tool. Re-run npm link in each checkout under "
	unreadable := "Not moved (no version in its package.json): broken. Reinstall it under "

	t.Run("update plan", func(t *testing.T) {
		f := useFakeNodenv(t, supported, "")
		f.version(t, "26.13.1", "ccusage@18.0.3")
		f.leftBehind(t, "26.13.1", "my-tool", "broken")
		f.global(t, "26.13.1")
		t.Chdir(t.TempDir())
		out := captureStdout(t, func() {
			if err := runNodeUpdate(context.Background(), true); err != nil {
				t.Errorf("update: %v", err)
			}
		})
		for _, want := range []string{linked + "26.14.0.", unreadable + "26.14.0", "Reinstall 26.13.1's 1 global npm package"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q; got:\n%s", want, out)
			}
		}
	})
	t.Run("install copy", func(t *testing.T) {
		f := useFakeNodenv(t, supported, "")
		f.version(t, "26.13.1", "ccusage@18.0.3")
		f.leftBehind(t, "26.13.1", "my-tool", "broken")
		f.global(t, "26.13.1")
		t.Chdir(t.TempDir())
		useNodeScope(t, true, false)
		out := captureStdout(t, func() {
			if err := runNodeInstall(context.Background(), "24", true); err != nil {
				t.Errorf("install: %v", err)
			}
		})
		for _, want := range []string{linked + "24.22.3.", unreadable + "24.22.3", "26.13.1 has 1 global npm package: ccusage@18.0.3"} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q; got:\n%s", want, out)
			}
		}
	})
	t.Run("uninstall", func(t *testing.T) {
		f := useFakeNodenv(t, supported, "")
		f.version(t, "26.13.1")
		f.version(t, "24.14.0", "ccusage@18.0.3")
		f.leftBehind(t, "24.14.0", "my-tool", "broken")
		f.global(t, "26.13.1")
		t.Chdir(t.TempDir())
		out := captureStdout(t, func() {
			if err := runNodeUninstall(context.Background(), "24.14.0", true); err != nil {
				t.Errorf("uninstall: %v", err)
			}
		})
		for _, want := range []string{
			"Its 1 global npm package go with it: ccusage@18.0.3",
			"Its npm links go with it too (the checkouts stay): my-tool",
			"So do the packages with no version in their package.json: broken",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("expected %q; got:\n%s", want, out)
			}
		}
	})
}

// A .node-version below the repository's top already governs this folder, so
// a project install rewrites it instead of adding one at the top.
func TestNodeInstallProjectRewritesGoverningPin(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.version(t, "26.13.1")
	f.global(t, "26.13.1")
	top := gitProject(t)
	pin := writePin(t, filepath.Join(top, "web"), "22.23.5")
	useNodeScope(t, false, true)

	out := captureStdout(t, func() {
		if err := runNodeInstall(context.Background(), "24", true); err != nil {
			t.Errorf("install: %v", err)
		}
	})
	if b, _ := os.ReadFile(pin); string(b) != "24.22.3\n" {
		t.Errorf("%s = %q, want 24.22.3; output:\n%s", pin, b, out)
	}
	if want := filepath.Join(top, "web") + " now uses Node 24.22.3"; !strings.Contains(out, want) {
		t.Errorf("expected %q; got:\n%s", want, out)
	}
	if _, err := os.Stat(filepath.Join(top, ".node-version")); err == nil {
		t.Error("wrote a second .node-version at the repository's top")
	}
}

// The first Node from `ctdev install nodenv` goes global even inside a
// repository: with no global, nothing outside this project would have Node.
func TestInstallNodenvFirstNodeDefaultsToGlobalInRepo(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	top := gitProject(t)
	useStdin(t, "y\n\n\n") // install one; Enter takes the scope default, then 26.14.0

	out := captureStdout(t, func() {
		if err := componentWizards["nodenv"](context.Background()); err != nil {
			t.Errorf("wizard: %v", err)
		}
	})
	if !strings.Contains(out, "2) This project") || !strings.Contains(out, "Choice [1]:") {
		t.Errorf("expected the scope menu to default to Global; got:\n%s", out)
	}
	if !strings.Contains(f.calls(t), "nodenv global 26.14.0\n") {
		t.Errorf("calls:\n%s", f.calls(t))
	}
	if _, err := os.Stat(filepath.Join(top, ".node-version")); err == nil {
		t.Error("the first Node was pinned to the project")
	}
}

// A pin that isn't an exact version (a major, lts, an alias) is resolved by
// nodenv itself, so update leaves it and every version alone.
func TestNodeUpdateLeavesInexactPin(t *testing.T) {
	for _, pinned := range []string{"22", "lts"} {
		t.Run(pinned, func(t *testing.T) {
			f := useFakeNodenv(t, supported, `22.23.5\n`)
			f.version(t, "22.20.0")
			f.global(t, "22.20.0")
			dir := t.TempDir()
			pin := writePin(t, dir, pinned)
			t.Chdir(dir)

			out := captureStdout(t, func() {
				if err := runNodeUpdate(context.Background(), true); err != nil {
					t.Errorf("update: %v", err)
				}
			})
			if want := pin + ` says "` + pinned + `", which is not an exact version: nodenv resolves it itself. Nothing changed.`; !strings.Contains(out, want) {
				t.Errorf("expected %q; got:\n%s", want, out)
			}
			if b, _ := os.ReadFile(pin); string(b) != pinned+"\n" {
				t.Errorf(".node-version = %q, want it unchanged", b)
			}
			if calls := f.calls(t); calls != "" {
				t.Errorf("ran:\n%s", calls)
			}
		})
	}
}

// A pin that is empty or says system is the problem, not the global version.
func TestNodeUpdateSaysWhatIsWrong(t *testing.T) {
	cases := []struct{ name, global, pin, want string }{
		{"empty pin", "26.13.1", "", "is empty or says system, so no nodenv Node runs here"},
		{"system pin", "26.13.1", "system", "is empty or says system, so no nodenv Node runs here"},
		{"system global", "system", "-", `the global Node is "system"`},
		{"no global", "", "-", "no global Node version is set"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := useFakeNodenv(t, supported, "")
			if tc.global != "" {
				f.global(t, tc.global)
			}
			dir := t.TempDir()
			if tc.pin != "-" {
				writePin(t, dir, tc.pin)
			}
			t.Chdir(dir)
			var err error
			captureStdout(t, func() { err = runNodeUpdate(context.Background(), true) })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

// A dry run with no nodenv can't read node-build's list, so it says what a
// real run would do and succeeds.
func TestNodeInstallDryRunWithoutNodenv(t *testing.T) {
	cases := []struct {
		arg          string
		global, proj bool
		want         string
	}{
		{"22", true, false, "A real run then installs the latest release of Node 22 and makes it the global version."},
		{"24.22.3", false, true, "A real run then installs Node 24.22.3 and writes it to "},
	}
	for _, tc := range cases {
		t.Run(tc.arg, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("NODENV_ROOT", filepath.Join(home, ".nodenv"))
			t.Setenv("NODENV_VERSION", "")
			t.Setenv("PATH", "/usr/bin:/bin")
			tool := useFakeTool(t, false)
			flagDryRun = true
			t.Cleanup(func() { flagDryRun = false })
			useNodeScope(t, tc.global, tc.proj)
			t.Chdir(t.TempDir())

			var err error
			out := captureStdout(t, func() { err = runNodeInstall(context.Background(), tc.arg, true) })
			if err != nil {
				t.Fatalf("dry run: %v", err)
			}
			if !strings.Contains(out, tc.want) || strings.Join(tool.installs, ",") != "nodenv" {
				t.Errorf("installs = %v; expected %q in:\n%s", tool.installs, tc.want, out)
			}
		})
	}
}

// A "system" global is the user's Node, so inside a repo the default scope
// stays the project and the global is left alone.
func TestNodeInstallSystemGlobalDefaultsToProjectInRepo(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.global(t, "system")
	top := gitProject(t)

	captureStdout(t, func() {
		if err := runNodeInstall(context.Background(), "24", true); err != nil {
			t.Errorf("install: %v", err)
		}
	})
	if b, _ := os.ReadFile(filepath.Join(top, ".node-version")); string(b) != "24.22.3\n" {
		t.Errorf("%s/.node-version = %q, want 24.22.3", top, b)
	}
	if strings.Contains(f.calls(t), "global") {
		t.Errorf("replaced the system global:\n%s", f.calls(t))
	}
}

// With nodenv and no global there is no machine-wide npm, so update must not
// scan whatever npm the current folder's pin resolves to.
func TestScanNPMGlobals_NoGlobalScansNothing(t *testing.T) {
	useFakeNodenv(t, supported, "")
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte("#!/bin/sh\necho '{\"ccusage\":{\"current\":\"1.0.0\",\"latest\":\"2.0.0\"}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	items, err := scanNPMGlobals(context.Background())
	if err != nil || len(items) != 0 {
		t.Errorf("scan = %v, %v; want nothing", items, err)
	}
}

// npm components never install into a system Node, so update leaves its
// packages alone too (and says nothing about a missing npm).
func TestScanNPMGlobals_SystemGlobalScansNothing(t *testing.T) {
	f := useFakeNodenv(t, supported, "")
	f.global(t, "system")
	t.Setenv("PATH", t.TempDir())
	items, err := scanNPMGlobals(context.Background())
	if err != nil || len(items) != 0 {
		t.Errorf("scan = %v, %v; want nothing", items, err)
	}
}

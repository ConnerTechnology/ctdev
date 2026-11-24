package component

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/nodenv"
)

// In a dry run nothing is installed, so on a computer without Node the node
// component in the same run can't have put npm in place yet. The npm-based
// components report the command they would run instead of failing.
func TestNpmComponents_DryRunWithoutNpm(t *testing.T) {
	cases := []struct {
		name    string
		install func(context.Context, ExecOpts) error
		want    string
	}{
		{"ccstatusline", ccstatuslineInstall, "[dry-run] npm install -g ccstatusline"},
		{"ccusage", FindByName("ccusage").GoInstall, "[dry-run] npm install -g ccusage"},
		{"codex", FindByName("codex").GoInstall, "[dry-run] npm install -g @openai/codex"},
		{"devcontainer", devcontainerInstall, "[dry-run] npm install -g @devcontainers/cli"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			useHome(t) // no npm on PATH, no ~/.nodenv
			var out bytes.Buffer
			err := tc.install(context.Background(), ExecOpts{DryRun: true, Force: true, Stdout: &out})
			if err != nil {
				t.Fatalf("dry run without npm: unexpected error: %v", err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("expected %q in output; got:\n%s", tc.want, out.String())
			}
		})
	}
}

// A real run still fails clearly when npm is missing.
func TestNpmPath_RealRunWithoutNpmFails(t *testing.T) {
	useHome(t)
	if _, err := npmPath(execOpts(ExecOpts{})); err == nil {
		t.Fatal("expected an error when npm is missing outside a dry run")
	}
}

func TestNpmGlobalComponents_DryRunUninstall(t *testing.T) {
	cases := map[string]string{
		"ccusage": "[dry-run] npm uninstall -g ccusage",
		"codex":   "[dry-run] npm uninstall -g @openai/codex",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			useHome(t)
			var out bytes.Buffer
			if err := FindByName(name).GoUninstall(context.Background(), ExecOpts{DryRun: true, Stdout: &out}); err != nil {
				t.Fatalf("dry-run uninstall: %v", err)
			}
			if !strings.Contains(out.String(), want) {
				t.Errorf("expected %q in output; got:\n%s", want, out.String())
			}
		})
	}
}

// useHome gives a test an empty $HOME and PATH, and clears the nodenv
// variables a developer's shell may carry.
func useHome(t *testing.T) (home, bin string) {
	t.Helper()
	home, bin = t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin)
	t.Setenv("NODENV_ROOT", "")
	t.Setenv("NODENV_VERSION", "")
	return home, bin
}

// noLTSLookup fails any test that reaches nodejs.org.
func noLTSLookup(t *testing.T) {
	t.Helper()
	orig := latestLTS
	latestLTS = func(context.Context) (nodenv.LTS, error) {
		t.Error("looked up the latest LTS with a Node version already set")
		return nodenv.LTS{}, errors.New("no network in tests")
	}
	t.Cleanup(func() { latestLTS = orig })
}

func writeFakeCmd(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeGlobal(t *testing.T, home, v string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(home, ".nodenv"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".nodenv", "version"), []byte(v+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// recordCalls makes fake nodenv and npm-shim commands that log each call,
// with the NODENV_VERSION it ran under, to the returned file.
func recordCalls(t *testing.T, home string) (log string) {
	t.Helper()
	log = filepath.Join(home, "calls")
	record := "echo \"${NODENV_VERSION:+NODENV_VERSION=$NODENV_VERSION }${0##*/} $*\" >> " + log
	writeFakeCmd(t, filepath.Join(home, ".nodenv", "shims", "npm"), record)
	writeFakeCmd(t, filepath.Join(home, ".nodenv", "bin", "nodenv"), record)
	return log
}

func readCalls(t *testing.T, log string) string {
	t.Helper()
	got, err := os.ReadFile(log)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	return string(got)
}

// A global install through nodenv's npm rehashes, so the package's command has
// a shim in the same shell. Without nodenv's npm there is nothing to rehash.
func TestNpmGlobalInstaller_RehashesNodenv(t *testing.T) {
	home, _ := useHome(t)
	noLTSLookup(t)
	log := recordCalls(t, home)
	writeGlobal(t, home, "24.1.0")

	ctx := context.Background()
	opts := ExecOpts{Force: true, Stdout: io.Discard}
	if err := FindByName("codex").GoInstall(ctx, opts); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := FindByName("codex").GoUninstall(ctx, opts); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	want := "NODENV_VERSION=24.1.0 nodenv exec npm install -g @openai/codex\nnodenv rehash\n" +
		"NODENV_VERSION=24.1.0 nodenv exec npm uninstall -g @openai/codex\nnodenv rehash\n"
	if got := readCalls(t, log); got != want {
		t.Errorf("calls:\n%s\nwant:\n%s", got, want)
	}
}

// An npm outside ~/.nodenv (system or Homebrew) has no shims to refresh.
func TestNpmGlobalInstaller_NoRehashWithoutNodenv(t *testing.T) {
	home, bin := useHome(t)
	log := filepath.Join(home, "calls")
	writeFakeCmd(t, filepath.Join(bin, "npm"), "echo \"${0##*/} $*\" >> "+log)

	opts := ExecOpts{Force: true, Stdout: io.Discard}
	if err := FindByName("codex").GoInstall(context.Background(), opts); err != nil {
		t.Fatalf("install: %v", err)
	}
	if got, want := readCalls(t, log), "npm install -g @openai/codex\n"; got != want {
		t.Errorf("calls:\n%s\nwant:\n%s", got, want)
	}
}

// With nodenv from Homebrew there is no ~/.nodenv/bin/nodenv, so the rehash
// uses nodenv on PATH. If it fails, the package is still installed: warn only.
func TestNpmGlobalInstaller_FailedRehashWarns(t *testing.T) {
	home, bin := useHome(t)
	noLTSLookup(t)
	writeFakeCmd(t, filepath.Join(bin, "nodenv"), `[ "$1" = rehash ] && exit 1; exit 0`)
	writeGlobal(t, home, "24.1.0")

	var out bytes.Buffer
	opts := ExecOpts{Force: true, Stdout: &out}
	if err := FindByName("codex").GoInstall(context.Background(), opts); err != nil {
		t.Fatalf("a failed rehash must not fail the install: %v", err)
	}
	if !strings.Contains(out.String(), "warning: nodenv rehash") {
		t.Errorf("expected a rehash warning; got:\n%s", out.String())
	}
}

// With nodenv but no Node version set, an npm component first installs the
// latest LTS and makes it global; npm's shim would otherwise run nothing.
func TestNpmInstall_InstallsLTSWhenNoNodeIsSet(t *testing.T) {
	for _, name := range []string{"codex", "ccstatusline", "devcontainer"} {
		t.Run(name, func(t *testing.T) {
			home, _ := useHome(t)
			log := recordCalls(t, home)
			orig := latestLTS
			latestLTS = func(context.Context) (nodenv.LTS, error) {
				return nodenv.LTS{Version: "26.13.1", LTSLine: nodenv.LTSLine{Major: 26, Codename: "Lithium"}}, nil
			}
			t.Cleanup(func() { latestLTS = orig })

			var out bytes.Buffer
			if err := FindByName(name).GoInstall(context.Background(), ExecOpts{Stdout: &out}); err != nil {
				t.Fatalf("install: %v", err)
			}
			if want := `No global Node version is set. Installing the latest LTS, Node 26.13.1 (v26 "Lithium")...`; !strings.Contains(out.String(), want) {
				t.Errorf("expected %q; got:\n%s", want, out.String())
			}
			want := "nodenv install --skip-existing 26.13.1\nnodenv global 26.13.1\nNODENV_VERSION=26.13.1 nodenv exec npm install -g "
			if got := readCalls(t, log); !strings.HasPrefix(got, want) {
				t.Errorf("calls:\n%s\nwant them to start:\n%s", got, want)
			}
		})
	}
}

// npm components are machine-wide tools, so inside a project pinned to
// another Node they still install, uninstall and rehash under the global one.
func TestNpmInstall_UsesGlobalInsideProjectPin(t *testing.T) {
	home, _ := useHome(t)
	noLTSLookup(t)
	log := recordCalls(t, home)
	writeGlobal(t, home, "24.1.0")
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".node-version"), []byte("22.23.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)

	ctx := context.Background()
	if err := FindByName("ccusage").GoInstall(ctx, ExecOpts{Stdout: io.Discard}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := FindByName("ccusage").GoUninstall(ctx, ExecOpts{Stdout: io.Discard}); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	want := "NODENV_VERSION=24.1.0 nodenv exec npm install -g ccusage\nnodenv rehash\n" +
		"NODENV_VERSION=24.1.0 nodenv exec npm uninstall -g ccusage\nnodenv rehash\n"
	if got := readCalls(t, log); got != want {
		t.Errorf("calls:\n%s\nwant:\n%s", got, want)
	}

	var out bytes.Buffer
	if err := FindByName("ccusage").GoInstall(ctx, ExecOpts{DryRun: true, Force: true, Stdout: &out}); err != nil {
		t.Fatalf("dry-run install: %v", err)
	}
	nodenvBin := filepath.Join(home, ".nodenv", "bin", "nodenv")
	if want := "[dry-run] NODENV_VERSION=24.1.0 " + nodenvBin + " exec npm install -g ccusage\n"; !strings.Contains(out.String(), want) {
		t.Errorf("dry run:\n%s\nwant it to contain:\n%s", out.String(), want)
	}
}

// With a global version set, nothing that hides it here (NODENV_VERSION=system,
// a .node-version that is empty or says system) makes an npm component
// install a Node or change the global one.
func TestNpmInstall_NeverChangesAnExistingGlobal(t *testing.T) {
	cases := map[string]func(t *testing.T, project string){
		"NODENV_VERSION=system": func(t *testing.T, _ string) { t.Setenv("NODENV_VERSION", "system") },
		"pin says system": func(t *testing.T, project string) {
			_ = os.WriteFile(filepath.Join(project, ".node-version"), []byte("system\n"), 0o644)
		},
		"empty pin": func(t *testing.T, project string) {
			_ = os.WriteFile(filepath.Join(project, ".node-version"), nil, 0o644)
		},
	}
	for name, hide := range cases {
		t.Run(name, func(t *testing.T) {
			home, _ := useHome(t)
			noLTSLookup(t)
			log := recordCalls(t, home)
			writeGlobal(t, home, "24.1.0")
			project := t.TempDir()
			t.Chdir(project)
			hide(t, project)

			if err := FindByName("ccusage").GoInstall(context.Background(), ExecOpts{Stdout: io.Discard}); err != nil {
				t.Fatalf("install: %v", err)
			}
			got := readCalls(t, log)
			if !strings.HasPrefix(got, "NODENV_VERSION=24.1.0 nodenv exec npm install -g ccusage\n") ||
				strings.Contains(got, "nodenv global") || strings.Contains(got, "nodenv install") {
				t.Errorf("calls:\n%s\nwant only npm under the global 24.1.0", got)
			}
		})
	}
}

// A global set to "system" is the user's choice of a Node outside nodenv:
// ctdev doesn't override it, and says how to pick a nodenv one.
func TestNpmInstall_GlobalSystemFailsClearly(t *testing.T) {
	home, _ := useHome(t)
	noLTSLookup(t)
	log := recordCalls(t, home)
	writeGlobal(t, home, "system")

	err := FindByName("ccusage").GoInstall(context.Background(), ExecOpts{Stdout: io.Discard})
	if err == nil || !strings.Contains(err.Error(), `"system"`) || !strings.Contains(err.Error(), "ctdev configure node install --global") {
		t.Errorf("err = %v, want it to name system and how to pick a nodenv Node", err)
	}
	if got := readCalls(t, log); got != "" {
		t.Errorf("ran with a system global:\n%s", got)
	}
}

// An npm component is installed only when the Node that runs here has its
// package: a shim left on PATH by another version does not count.
func TestNpmPackageDetect(t *testing.T) {
	home, bin := useHome(t)
	writeFakeCmd(t, filepath.Join(home, ".nodenv", "shims", "codex"), "exit 0")
	writeFakeCmd(t, filepath.Join(home, ".nodenv", "bin", "nodenv"), "exit 0")
	t.Setenv("PATH", filepath.Join(home, ".nodenv", "shims")+":"+bin)
	codex := FindByName("codex")

	if codex.IsInstalled() {
		t.Error("a shim with no Node version set counted as installed")
	}
	if err := os.MkdirAll(filepath.Join(home, ".nodenv", "versions", "26.13.1"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGlobal(t, home, "26.13.1")
	if codex.IsInstalled() {
		t.Error("a shim from another version counted as installed under 26.13.1")
	}
	pkg := filepath.Join(home, ".nodenv", "versions", "26.13.1", "lib", "node_modules", "@openai", "codex", "package.json")
	if err := os.MkdirAll(filepath.Dir(pkg), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pkg, []byte(`{"version":"0.171.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !codex.IsInstalled() {
		t.Error("26.13.1 has @openai/codex but codex did not count as installed")
	}
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".node-version"), []byte("22.23.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(project)
	if !codex.IsInstalled() {
		t.Error("inside a project pinned to 22.23.5, codex under the global 26.13.1 did not count as installed")
	}

	t.Setenv("HOME", t.TempDir())
	writeFakeCmd(t, filepath.Join(bin, "codex"), "exit 0")
	if !codex.IsInstalled() {
		t.Error("without nodenv, codex on PATH should count as installed")
	}
}

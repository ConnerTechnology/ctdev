package nodenv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Abridged `nodenv install --list-all`: node-build mixes in other runtimes and
// pre-releases, and lists 20.x where a lexical sort would misplace it.
const listAll = `
0.10.48
20.9.0
20.20.2
20.10.0
22.23.5
24.22.3
24.21.0
26.13.1
26.14.0-rc.1
graal+ce-19.2.1
chakracore-10.13.0
`

func TestParseListKeepsStableNewestFirst(t *testing.T) {
	got := ParseList(listAll)
	want := []string{"26.13.1", "24.22.3", "24.21.0", "22.23.5", "20.20.2", "20.10.0", "20.9.0", "0.10.48"}
	if !slices.Equal(got, want) {
		t.Errorf("ParseList = %v, want %v", got, want)
	}
}

func TestLatestOfMajor(t *testing.T) {
	list := ParseList(listAll)
	for major, want := range map[int]string{26: "26.13.1", 24: "24.22.3", 20: "20.20.2", 18: ""} {
		if got := LatestOfMajor(list, major); got != want {
			t.Errorf("LatestOfMajor(%d) = %q, want %q", major, got, want)
		}
	}
	if got, want := LatestPerMajor(list), []string{"26.13.1", "24.22.3", "22.23.5", "20.20.2", "0.10.48"}; !slices.Equal(got, want) {
		t.Errorf("LatestPerMajor = %v, want %v", got, want)
	}
}

func TestResolve(t *testing.T) {
	list := ParseList(listAll)
	tests := []struct {
		arg, want string
		ok        bool
	}{
		{"22", "22.23.5", true},
		{"v24", "24.22.3", true},
		{"24.21.0", "24.21.0", true},
		{"24.0.0", "24.0.0", false},
		{"18", "", false},
		{"lts", "", false},
	}
	for _, tt := range tests {
		got, ok := Resolve(list, tt.arg)
		if got != tt.want || ok != tt.ok {
			t.Errorf("Resolve(%q) = %q, %v; want %q, %v", tt.arg, got, ok, tt.want, tt.ok)
		}
	}
}

func TestPickLTS(t *testing.T) {
	tests := []struct {
		name  string
		index string
		want  LTSLine
		ok    bool
	}{
		{
			// April to October: the new even major is out but not yet LTS.
			name: "new even major still Current",
			index: `[{"version":"v26.0.0","lts":false},{"version":"v25.9.0","lts":false},
				{"version":"v24.22.3","lts":"Krypton"},{"version":"v24.22.2","lts":"Krypton"}]`,
			want: LTSLine{24, "Krypton"}, ok: true,
		},
		{
			name: "even major has turned LTS",
			index: `[{"version":"v27.0.0-alpha.0.1.1","lts":false},{"version":"v26.13.1","lts":"Lithium"},
				{"version":"v26.9.0","lts":false},{"version":"v24.22.3","lts":"Krypton"}]`,
			want: LTSLine{26, "Lithium"}, ok: true,
		},
		{name: "no LTS at all", index: `[{"version":"v1.0.0","lts":false}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rs, err := ParseIndex([]byte(tt.index))
			if err != nil {
				t.Fatal(err)
			}
			got, ok := PickLTS(rs)
			if got != tt.want || ok != tt.ok {
				t.Errorf("PickLTS = %+v, %v; want %+v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func useRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("NODENV_ROOT", root)
	t.Setenv("NODENV_VERSION", "")
	return root
}

func TestGlobalAndProjectPin(t *testing.T) {
	root := useRoot(t)
	project := t.TempDir()
	sub := filepath.Join(project, "web", "src")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	if Global() != "" || GlobalIsSystem() {
		t.Errorf("no global: Global = %q, GlobalIsSystem = %v", Global(), GlobalIsSystem())
	}
	writeFile(t, filepath.Join(root, "version"), "system\n")
	if Global() != "" || !GlobalIsSystem() {
		t.Errorf("global system: Global = %q, GlobalIsSystem = %v", Global(), GlobalIsSystem())
	}
	writeFile(t, filepath.Join(root, "version"), "v26.13.1\n")
	if Global() != "26.13.1" || GlobalIsSystem() {
		t.Errorf("global 26.13.1: Global = %q, GlobalIsSystem = %v", Global(), GlobalIsSystem())
	}
	if file, v := ProjectPin(sub); file != "" || v != "" {
		t.Errorf("no pin: ProjectPin = %q, %q", file, v)
	}
	writeFile(t, filepath.Join(project, ".node-version"), "v22.23.5\n")
	if file, v := ProjectPin(sub); file != filepath.Join(project, ".node-version") || v != "22.23.5" {
		t.Errorf("ProjectPin = %q, %q; want the project's file", file, v)
	}
}

func TestIsExact(t *testing.T) {
	for v, want := range map[string]bool{"24.22.3": true, "v24.22.3": true, "24": false, "24.1": false, "lts": false, "lts/iron": false} {
		if IsExact(v) != want {
			t.Errorf("IsExact(%q) = %v, want %v", v, !want, want)
		}
	}
}

func TestInstalledAndGlobalPackages(t *testing.T) {
	root := useRoot(t)
	mods := filepath.Join(root, "versions", "24.22.3", "lib", "node_modules")
	writeFile(t, filepath.Join(mods, "npm", "package.json"), `{"version":"11.0.0"}`)
	writeFile(t, filepath.Join(mods, "corepack", "package.json"), `{"version":"0.34.0"}`)
	writeFile(t, filepath.Join(mods, "ccusage", "package.json"), `{"version":"18.0.3"}`)
	writeFile(t, filepath.Join(mods, "@openai", "codex", "package.json"), `{"version":"0.171.0"}`)
	writeFile(t, filepath.Join(mods, ".package-lock.json"), `{}`)
	writeFile(t, filepath.Join(mods, "broken", "package.json"), `{"name":"broken"}`)
	linked := t.TempDir()
	writeFile(t, filepath.Join(linked, "package.json"), `{"version":"0.0.1"}`)
	if err := os.Symlink(linked, filepath.Join(mods, "my-tool")); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"26.13.1", "20.20.2"} {
		if err := os.MkdirAll(filepath.Join(root, "versions", v), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	if got, want := Installed(), []string{"20.20.2", "24.22.3", "26.13.1"}; !slices.Equal(got, want) {
		t.Errorf("Installed = %v, want %v", got, want)
	}
	g := GlobalPackages("24.22.3")
	if want := []Package{{"@openai/codex", "0.171.0"}, {"ccusage", "18.0.3"}}; !slices.Equal(g.Packages, want) {
		t.Errorf("Packages = %v, want %v (no npm, corepack, linked or unreadable packages)", g.Packages, want)
	}
	if !slices.Equal(g.Linked, []string{"my-tool"}) || !slices.Equal(g.Unreadable, []string{"broken"}) {
		t.Errorf("Linked = %v, Unreadable = %v; want [my-tool], [broken]", g.Linked, g.Unreadable)
	}
	if !HasPackage("24.22.3", "@openai/codex") || HasPackage("26.13.1", "@openai/codex") || HasPackage("", "ccusage") {
		t.Error("HasPackage should see only the version that has the package")
	}
}

// Every change runs nodenv with the version it targets, so a dry run shows
// exactly what a real run would do.
func TestChangesDryRun(t *testing.T) {
	root := useRoot(t)
	writeFile(t, filepath.Join(root, "bin", "nodenv"), "")
	nodenv := filepath.Join(root, "bin", "nodenv")
	var out bytes.Buffer
	o := sysutil.Opts{DryRun: true, Stdout: &out}
	ctx := context.Background()
	_ = Install(ctx, o, "26.14.0")
	_ = InstallPackages(ctx, o, "26.14.0", []Package{{"ccusage", "18.0.3"}, {"@openai/codex", "0.171.0"}})
	_ = InstallPackages(ctx, o, "26.14.0", nil)
	_ = SetGlobal(ctx, o, "26.14.0")
	_ = WritePin(o, "/work/site/.node-version", "26.14.0")
	_ = Uninstall(ctx, o, "26.13.1")
	want := strings.Join([]string{
		"[dry-run] " + nodenv + " install --skip-existing 26.14.0",
		"[dry-run] NODENV_VERSION=26.14.0 " + nodenv + " exec npm install -g ccusage@18.0.3 @openai/codex@0.171.0",
		"[dry-run] " + nodenv + " global 26.14.0",
		"[dry-run] write /work/site/.node-version: 26.14.0",
		"[dry-run] " + nodenv + " uninstall -f 26.13.1",
	}, "\n") + "\n"
	if out.String() != want {
		t.Errorf("dry run:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestGlobalSelector(t *testing.T) {
	cases := []struct {
		name, global string
		nodenv       bool
		want         string
	}{
		{"version", "24.1.0", true, "24.1.0"},
		{"system", "system", true, "system"},
		{"unset", "", true, ""},
		{"stale version file, nodenv gone", "24.1.0", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("HOME", t.TempDir())
			t.Setenv("NODENV_ROOT", root)
			t.Setenv("PATH", t.TempDir())
			if tc.nodenv {
				if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, "bin", "nodenv"), []byte("#!/bin/sh\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if tc.global != "" {
				if err := os.WriteFile(filepath.Join(root, "version"), []byte(tc.global+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := GlobalSelector(); got != tc.want {
				t.Errorf("GlobalSelector() = %q, want %q", got, tc.want)
			}
		})
	}
}

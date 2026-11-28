package component

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCTopsLatestTag(t *testing.T) {
	for _, tt := range []struct {
		name     string
		releases []ctopsRelease
		want     string
	}{
		{"numeric minor", []ctopsRelease{{TagName: "ctops/v0.9.0"}, {TagName: "ctops/v0.10.0"}}, "ctops/v0.10.0"},
		{"numeric major and patch", []ctopsRelease{{TagName: "ctops/v2.99.99"}, {TagName: "ctops/v10.0.9"}, {TagName: "ctops/v10.0.10"}}, "ctops/v10.0.10"},
		{"unordered", []ctopsRelease{{TagName: "ctops/v1.2.3"}, {TagName: "ctops/v1.2.3"}, {TagName: "ctops/v1.1.99"}}, "ctops/v1.2.3"},
		{"stable only", []ctopsRelease{
			{TagName: "ctops/v9.0.0", IsDraft: true},
			{TagName: "ctops/v8.0.0", IsPrerelease: true},
			{TagName: "ctops/v7.0.0-rc.1"}, {TagName: "ctops/v7.0.0+build"},
			{TagName: "other/v99.0.0"}, {TagName: "v99.0.0"},
			{TagName: "ctops/v1.2.3"},
		}, "ctops/v1.2.3"},
		{"invalid", []ctopsRelease{{TagName: "ctops/v1.2"}, {TagName: "ctops/v01.2.3"}, {TagName: "ctops/v1.2.3.4"}, {TagName: "ctops/v1.a.3"}}, ""},
		{"none", nil, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ctopsLatestTag(tt.releases)
			if got != tt.want || (err != nil) != (tt.want == "") {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestCTopsAsset(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		for _, arch := range []string{"amd64", "arm64"} {
			want := "ctops-" + goos + "-" + arch
			if got, err := ctopsAsset(goos, arch); err != nil || got != want {
				t.Errorf("ctopsAsset(%q, %q) = %q, %v; want %q", goos, arch, got, err, want)
			}
		}
	}
	for _, pair := range [][2]string{{"linux", "386"}, {"darwin", "arm"}, {"windows", "amd64"}} {
		if _, err := ctopsAsset(pair[0], pair[1]); !errors.Is(err, ErrUnsupportedOS) {
			t.Errorf("%v: expected ErrUnsupportedOS, got %v", pair, err)
		}
	}
}

func TestCTopsVerifyChecksum(t *testing.T) {
	data := []byte("synthetic ctops binary")
	sum := fmt.Sprintf("%x", sha256.Sum256(data))
	const asset = "ctops-linux-amd64"
	for _, tt := range []struct {
		name, sums string
		wantErr    bool
	}{
		{"match", sum + "  " + asset + "\n", false},
		{"multiple assets and uppercase", "\n" + sum + "  ctops-linux-arm64\n" + strings.ToUpper(sum) + "  " + asset + "\n", false},
		{"binary marker", sum + " *" + asset + "\n", false},
		{"missing", sum + "  ctops-linux-arm64\n", true},
		{"empty", "", true},
		{"mismatch", strings.Repeat("0", 64) + "  " + asset, true},
		{"bad hex", strings.Repeat("g", 64) + "  " + asset, true},
		{"short hash", "1234  " + asset, true},
		{"duplicate", sum + "  " + asset + "\n" + sum + "  " + asset, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := ctopsVerifyChecksum(asset, data, []byte(tt.sums)); (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseCTopsVersion(t *testing.T) {
	for out, want := range map[string]string{
		"ctops version 0.10.0\n": "0.10.0",
		"ctops version 1.2.3":    "1.2.3",
		"":                       "",
		"other version 1.2.3":    "",
		"ctops version dev":      "",
		"ctops version 1.2":      "",
	} {
		if got := parseCTopsVersion(out); got != want {
			t.Errorf("parseCTopsVersion(%q) = %q, want %q", out, got, want)
		}
	}
}

// All gh calls stay inside this shell stub, with no real gh on PATH.
func ctopsTestEnv(t *testing.T) (dest, log string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	bin := t.TempDir()
	for _, name := range []string{"install", "rm"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, filepath.Join(bin, name)); err != nil {
			t.Fatal(err)
		}
	}
	log = filepath.Join(t.TempDir(), "gh-calls")
	t.Setenv("CTOPS_TEST_LOG", log)
	t.Setenv("CTOPS_TEST_AUTH_FAIL", "")
	t.Setenv("CTOPS_TEST_LIST_FAIL", "")
	t.Setenv("CTOPS_TEST_NO_ASSET", "")
	t.Setenv("CTOPS_TEST_RELEASES", `[{"tagName":"ctops/v0.9.0"},{"tagName":"ctops/v0.10.0"}]`)
	data := "#!/bin/sh\nprintf 'ctops version 0.10.0\\n'\n"
	t.Setenv("CTOPS_TEST_BINARY", data)
	t.Setenv("CTOPS_TEST_SUM", fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
	stub := `#!/bin/sh
printf '%s\n' "$*" >> "$CTOPS_TEST_LOG"
case "$1 $2" in
  'auth status')
    [ "$*" = 'auth status --hostname github.com --active' ] || exit 2
    [ -z "$CTOPS_TEST_AUTH_FAIL" ] ;;
  'release list')
    [ "$*" = 'release list --repo github.com/ConnerTechnology/infrastructure --json tagName,isDraft,isPrerelease --limit 100' ] || exit 2
    printf 'warning: synthetic gh notice\n' >&2
    if [ -n "$CTOPS_TEST_LIST_FAIL" ]; then
      printf 'HTTP 404: Not Found\n' >&2
      exit 1
    fi
    printf '%s' "$CTOPS_TEST_RELEASES" ;;
  'release download')
    shift 3
    [ "$1 $2" = '--repo github.com/ConnerTechnology/infrastructure' ] || exit 2
    shift 2
    [ "$1" = '--pattern' ] || exit 2
    asset=$2
    shift 2
    [ "$1 $2 $3" = '--pattern SHA256SUMS --dir' ] || exit 2
    dir=$4
    [ -n "$CTOPS_TEST_NO_ASSET" ] || printf '%s' "$CTOPS_TEST_BINARY" > "$dir/$asset"
    if [ "$CTOPS_TEST_SUM" = missing ]; then
      printf '' > "$dir/SHA256SUMS"
    else
      printf '%s  %s\n' "$CTOPS_TEST_SUM" "$asset" > "$dir/SHA256SUMS"
    fi ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	dest, err := ctopsUserBin()
	if err != nil {
		t.Fatal(err)
	}
	return dest, log
}

func TestCTopsInstallLifecycle(t *testing.T) {
	dest, log := ctopsTestEnv(t)
	var out bytes.Buffer
	opts := ExecOpts{Stdout: &out}
	if err := ctopsInstall(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	c := FindByName("ctops")
	if !c.IsInstalled() || CTopsVersion(context.Background()) != "0.10.0" {
		t.Fatal("installed binary was not detected or did not report its version")
	}
	fi, err := os.Stat(dest)
	if err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("installed mode: %v, %v", fi, err)
	}
	calls, err := os.ReadFile(log)
	if err != nil || !strings.Contains(string(calls), "release download ctops/v0.10.0 --repo ") {
		t.Fatalf("wrong release downloaded: %s, %v", calls, err)
	}
	t.Setenv("CTOPS_TEST_AUTH_FAIL", "1")
	if err := ctopsInstall(context.Background(), opts); err != nil || !strings.Contains(out.String(), "ctops already installed") {
		t.Fatalf("installed check: %v, %s", err, out.String())
	}
	opts.Force = true
	if err := ctopsInstall(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "gh auth login") || !strings.Contains(err.Error(), "private") {
		t.Fatalf("force must require private repo auth: %v", err)
	}
	if err := ctopsUninstall(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if c.IsInstalled() || CTopsVersion(context.Background()) != "" {
		t.Fatal("uninstalled binary still detected")
	}
}

func TestCTopsInstallFailsClosed(t *testing.T) {
	for _, sum := range []string{"missing", strings.Repeat("0", 64), "bad"} {
		t.Run(sum, func(t *testing.T) {
			dest, _ := ctopsTestEnv(t)
			t.Setenv("CTOPS_TEST_SUM", sum)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				t.Fatal(err)
			}
			const old = "old synthetic binary"
			if err := os.WriteFile(dest, []byte(old), 0o755); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := ctopsInstall(context.Background(), ExecOpts{Force: true, Stdout: &out}); err == nil {
				t.Fatal("installed an unchecked download")
			}
			if data, err := os.ReadFile(dest); err != nil || string(data) != old {
				t.Fatalf("old binary changed: %q, %v", data, err)
			}
		})
	}
}

func TestCTopsLatestVersionErrors(t *testing.T) {
	for _, body := range []string{"not JSON", "[]", `[{"tagName":"other/v1.0.0"}]`} {
		t.Run(body, func(t *testing.T) {
			ctopsTestEnv(t)
			t.Setenv("CTOPS_TEST_RELEASES", body)
			if _, err := CTopsLatestVersion(context.Background()); err == nil {
				t.Fatal("expected release lookup error")
			}
		})
	}
}

func TestCTopsDryRunAndCancellation(t *testing.T) {
	dest, log := ctopsTestEnv(t)
	var out bytes.Buffer
	opts := ExecOpts{DryRun: true, Stdout: &out}
	if err := ctopsInstall(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if err := ctopsUninstall(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("dry-run executed gh: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(dest)); !os.IsNotExist(err) {
		t.Fatalf("dry-run created install directory: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ctopsInstall(ctx, ExecOpts{Stdout: &out}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled install: %v", err)
	}
}

func TestCTopsRegistry(t *testing.T) {
	c := FindByName("ctops")
	if c == nil {
		t.Fatal("ctops missing from registry")
	}
	if c.Category != CategoryInfra || c.Root != RootNever || c.Detect == nil || c.GoInstall == nil || c.GoUninstall == nil {
		t.Fatalf("wrong ctops registration: %+v", c)
	}
	if !reflect.DeepEqual(c.SupportedOS, []OS{OSLinux, OSMacOS}) || !reflect.DeepEqual(ResolveDependencies(Registry, []string{"ctops"}), []string{"gh", "ctops"}) {
		t.Fatalf("wrong platforms or dependencies: %+v", c)
	}
	if InstallNeedsRoot("apt", []string{"ctops"}, true) || UninstallNeedsRoot("apt", []string{"ctops"}) {
		t.Fatal("ctops asked for root")
	}
}

func TestCTopsGHStderrNeverReachesParsedOutput(t *testing.T) {
	ctopsTestEnv(t)
	if got, err := CTopsLatestVersion(context.Background()); err != nil || got != "0.10.0" {
		t.Fatalf("a gh warning on stderr broke the release list: %q, %v", got, err)
	}
	t.Setenv("CTOPS_TEST_LIST_FAIL", "1")
	if _, err := CTopsLatestVersion(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 404: Not Found") {
		t.Fatalf("gh's own error must reach the user, got %v", err)
	}
}

func TestCTopsMissingAssetSaysSo(t *testing.T) {
	dest, _ := ctopsTestEnv(t)
	t.Setenv("CTOPS_TEST_NO_ASSET", "1")
	var out strings.Builder
	err := ctopsInstall(context.Background(), ExecOpts{Stdout: &out, Force: true})
	if err == nil || !strings.Contains(err.Error(), "has no ctops-") {
		t.Fatalf("want a missing-asset error, got %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("nothing may be installed without the asset")
	}
}

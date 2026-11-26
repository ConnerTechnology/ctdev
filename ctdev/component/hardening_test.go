package component

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func TestResolveDependencies_EdgeCases(t *testing.T) {
	reg := []Component{
		{Name: "a", Dependencies: []string{"b"}},
		{Name: "b", Dependencies: []string{"a"}}, // cycle
		{Name: "base"},
		{Name: "left", Dependencies: []string{"base"}},
		{Name: "right", Dependencies: []string{"base"}},
		{Name: "top", Dependencies: []string{"left", "right"}},
	}

	t.Run("diamond dependency emitted once before dependents", func(t *testing.T) {
		got := ResolveDependencies(reg, []string{"top", "top"})
		want := []string{"base", "left", "right", "top"}
		if !slices.Equal(got, want) {
			t.Errorf("ResolveDependencies(diamond) = %v, want %v", got, want)
		}
	})

	t.Run("cycle terminates without infinite recursion", func(t *testing.T) {
		got := ResolveDependencies(reg, []string{"a"})
		if len(got) != 2 || !slices.Contains(got, "a") || !slices.Contains(got, "b") {
			t.Errorf("ResolveDependencies(cycle) = %v, want [b a] or [a b]", got)
		}
	})

	t.Run("unknown name passed through safely", func(t *testing.T) {
		got := ResolveDependencies(reg, []string{"unknown-xyz"})
		if !slices.Equal(got, []string{"unknown-xyz"}) {
			t.Errorf("ResolveDependencies(unknown) = %v, want [unknown-xyz]", got)
		}
	})
}

func TestExecutor_UnsupportedPMAndErrors(t *testing.T) {
	c := &Component{
		Name: "test-pm",
		GoInstall: func(context.Context, ExecOpts) error {
			return sysutil.ErrUnsupportedPM
		},
		GoUninstall: func(context.Context, ExecOpts) error {
			return errors.New("uninstall boom")
		},
	}

	res := Install(context.Background(), c, ExecOpts{Stdout: &bytes.Buffer{}})
	if !res.Skipped || res.Err != nil {
		t.Errorf("Install(ErrUnsupportedPM) = Skipped:%v Err:%v, want Skipped:true Err:nil", res.Skipped, res.Err)
	}

	ures := Uninstall(context.Background(), c, ExecOpts{Stdout: &bytes.Buffer{}})
	if ures.Skipped || ures.Err == nil || !strings.Contains(ures.Err.Error(), "uninstall boom") {
		t.Errorf("Uninstall(boom) = Skipped:%v Err:%v, want error propagated", ures.Skipped, ures.Err)
	}
}

func TestPrimaryKeyFingerprints(t *testing.T) {
	t.Run("extracts primary keys and skips subkeys", func(t *testing.T) {
		colons := strings.Join([]string{
			"tru::1:1700000000:0:3:1:5",
			"pub:-:4096:1:AAA:1700000000:::-:::scESC::::::23::0:",
			"fpr:::::::::1111111111111111111111111111111111111111:",
			"sub:-:4096:1:BBB:1700000000::::::e::::::23:",
			"fpr:::::::::2222222222222222222222222222222222222222:",
			"pub:-:4096:1:CCC:1700000000:::-:::scESC::::::23::0:",
			"fpr:::::::::3333333333333333333333333333333333333333:",
		}, "\n")
		got, err := primaryKeyFingerprints(colons)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{
			"1111111111111111111111111111111111111111",
			"3333333333333333333333333333333333333333",
		}
		if !slices.Equal(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	})

	for _, rec := range []string{"sec", "ssb"} {
		t.Run("rejects secret key "+rec, func(t *testing.T) {
			if _, err := primaryKeyFingerprints(rec + ":-:4096:1:AAA:::\n"); err == nil {
				t.Errorf("expected error for %s record", rec)
			}
		})
	}
}

func TestFetchKubectlStableVersion(t *testing.T) {
	status := http.StatusOK
	body := "v1.33.0\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	orig := kubectlStableURL
	kubectlStableURL = srv.URL
	t.Cleanup(func() { kubectlStableURL = orig })

	ver, err := fetchKubectlStableVersion(context.Background())
	if err != nil || ver != "v1.33.0" {
		t.Fatalf("fetchKubectlStableVersion() = %q, %v; want v1.33.0", ver, err)
	}

	status = http.StatusServiceUnavailable
	body = "unavailable"
	if _, err := fetchKubectlStableVersion(context.Background()); err == nil || !strings.Contains(err.Error(), "HTTP 503") {
		t.Errorf("expected HTTP 503 error, got %v", err)
	}
}

func TestGitSpiceRegistryDetectIgnoresGhostscript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stub")
	}
	c := FindByName("git-spice")
	if c == nil || c.Detect == nil {
		t.Fatal("git-spice must wire custom Detect func to distinguish from Ghostscript")
	}

	binDir := t.TempDir()
	// Fake Ghostscript binary named `gs`.
	if err := os.WriteFile(filepath.Join(binDir, "gs"), []byte("#!/bin/sh\necho '10.02.1'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)
	if c.IsInstalled() {
		t.Error("git-spice.IsInstalled() returned true for Ghostscript")
	}

	// Real git-spice output.
	if err := os.WriteFile(filepath.Join(binDir, "gs"), []byte("#!/bin/sh\necho 'git-spice 0.12.0'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !c.IsInstalled() {
		t.Error("git-spice.IsInstalled() returned false for git-spice")
	}
}

func TestBetterbirdReplace_RestoresOldInstallOnFailedSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell stub")
	}
	dir := t.TempDir()
	installDir := filepath.Join(dir, "betterbird")
	staging := filepath.Join(dir, "staging")
	tarPath := filepath.Join(dir, "betterbird.tar.xz")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(installDir, "marker"), []byte("old-install"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Stub sudo + tar + mv: tar populates a valid new tree; the second mv
	// (putting newTree into installDir) fails, triggering rollback of oldTree.
	binDir := t.TempDir()
	stateFile := filepath.Join(dir, "mv-count")
	writeExec := func(name, body string) {
		if err := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeExec("sudo", "#!/bin/sh\nexec \"$@\"\n")
	writeExec("mkdir", "#!/bin/sh\n/bin/mkdir \"$@\"\n")
	writeExec("rm", "#!/bin/sh\n/bin/rm \"$@\"\n")
	writeExec("tar", `#!/bin/sh
/bin/mkdir -p "`+staging+`/betterbird"
printf '#!/bin/sh\n' > "`+staging+`/betterbird/betterbird"
/bin/chmod 0755 "`+staging+`/betterbird/betterbird"
`)
	writeExec("mv", `#!/bin/sh
n=$(/bin/cat "`+stateFile+`" 2>/dev/null || echo 0)
n=$((n + 1))
echo "$n" > "`+stateFile+`"
if [ "$n" -eq 2 ]; then
  echo "simulated swap failure" >&2
  exit 1
fi
exec /bin/mv "$@"
`)
	t.Setenv("PATH", binDir)

	var out bytes.Buffer
	err := betterbirdReplace(context.Background(), sysutil.Opts{Stdout: &out}, tarPath, staging, installDir)
	if err == nil {
		t.Fatal("expected error from simulated swap failure")
	}
	got, readErr := os.ReadFile(filepath.Join(installDir, "marker"))
	if readErr != nil || string(got) != "old-install" {
		t.Errorf("rollback failed to restore old install: data=%q err=%v", got, readErr)
	}
	if _, statErr := os.Stat(staging); !os.IsNotExist(statErr) {
		t.Errorf("staging dir was not cleaned up: %v", statErr)
	}
}

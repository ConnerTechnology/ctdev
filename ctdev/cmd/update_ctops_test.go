package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/checklist"
)

func TestScanCTops(t *testing.T) {
	for _, tt := range []struct {
		name, current string
		authFail      bool
		wantItems     int
		wantErr       bool
	}{
		{"behind", "0.9.0", false, 1, false},
		{"current", "0.10.0", false, 0, false},
		{"ahead", "0.11.0", false, 0, false},
		{"missing skips gh", "", true, 0, false},
		{"auth failure", "0.9.0", true, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			if tt.current != "" {
				bin := filepath.Join(home, ".local", "bin", "ctops")
				if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'ctops version "+tt.current+"\\n'\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			authStatus := 0
			if tt.authFail {
				authStatus = 1
			}
			fakeBins(t, map[string]string{"gh": fmt.Sprintf(`
case "$1 $2" in
  'auth status') exit %d ;;
  'release list') printf '%%s' '[{"tagName":"ctops/v0.9.0"},{"tagName":"ctops/v0.10.0"},{"tagName":"ctops/v9.0.0","isDraft":true}]' ;;
  *) exit 2 ;;
esac`, authStatus)})
			items, err := scanCTops(context.Background())
			if (err != nil) != tt.wantErr || len(items) != tt.wantItems {
				t.Fatalf("scanCTops() = %+v, %v", items, err)
			}
			if len(items) == 1 && items[0] != (checklist.UpdateItem{Name: "ctops", Source: "cli", CurrentVer: "0.9.0", NewVer: "0.10.0"}) {
				t.Fatalf("unexpected update: %+v", items[0])
			}
		})
	}
}

func TestCTopsUpdateStep(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	install, err := exec.LookPath("install")
	if err != nil {
		t.Fatal(err)
	}
	data := "synthetic updated ctops binary\n"
	t.Setenv("CTOPS_TEST_INSTALL", install)
	t.Setenv("CTOPS_TEST_BINARY", data)
	t.Setenv("CTOPS_TEST_SUM", fmt.Sprintf("%x", sha256.Sum256([]byte(data))))
	fakeBins(t, map[string]string{
		"install": "exec \"$CTOPS_TEST_INSTALL\" \"$@\"",
		"gh": `
case "$1 $2" in
  'auth status') exit 0 ;;
  'release download')
    [ "$3" = 'ctops/v0.10.0' ] || exit 2
    shift 3
    shift 2
    asset=$2
    shift 2
    dir=$4
    printf '%s' "$CTOPS_TEST_BINARY" > "$dir/$asset"
    printf '%s  %s\n' "$CTOPS_TEST_SUM" "$asset" > "$dir/SHA256SUMS" ;;
  *) exit 2 ;;
esac`,
	})
	item := checklist.UpdateItem{Name: "ctops", Source: "cli", CurrentVer: "0.9.0", NewVer: "0.10.0"}
	if updateNeedsRoot([]checklist.UpdateItem{item}) {
		t.Fatal("ctops update asked for root")
	}
	steps := buildUpdateSteps([]checklist.UpdateItem{item})
	if len(steps) != 1 {
		t.Fatalf("expected one ctops update step, got %d", len(steps))
	}
	var out bytes.Buffer
	if err := steps[0].run(context.Background(), sysutil.Opts{Stdout: &out, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ctops/v0.10.0") {
		t.Fatalf("dry-run didn't name the selected release: %s", out.String())
	}
	dest := filepath.Join(os.Getenv("HOME"), ".local", "bin", "ctops")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old synthetic ctops binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := steps[0].run(context.Background(), sysutil.Opts{Stdout: &out, NoSudoPrompt: true}); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(dest); err != nil || string(got) != data {
		t.Fatalf("update didn't replace installed binary: %q, %v", got, err)
	}
}

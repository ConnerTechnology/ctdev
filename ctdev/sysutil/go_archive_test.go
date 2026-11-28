//go:build unix

package sysutil

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGoArchiveReplacement(t *testing.T) {
	for _, tc := range []string{"upgrade", "invalid archive", "missing executable", "failed activation", "interrupted activation", "foreign staging directory"} {
		t.Run(tc, func(t *testing.T) {
			base := t.TempDir()
			live := filepath.Join(base, "go")
			if err := os.MkdirAll(filepath.Join(live, "bin"), 0755); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(live, "bin", "go")
			if err := os.WriteFile(bin, []byte("old"), 0755); err != nil {
				t.Fatal(err)
			}
			stage := live + ".ctdev-install"
			args := []string{"-ec", replaceGoArchive, "ctdev", live}
			archive := filepath.Join(base, "candidate.tar.gz")
			writeGoTestArchive(t, archive, tc != "missing executable")
			if tc == "invalid archive" {
				if err := os.WriteFile(archive, []byte("invalid"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			data, err := os.ReadFile(archive)
			if err != nil {
				t.Fatal(err)
			}
			args = append(args, archive, fmt.Sprintf("%x", sha256.Sum256(data)))
			cmd := exec.CommandContext(t.Context(), "sh", args...)
			switch tc {
			case "failed activation", "interrupted activation":
				realMV, err := exec.LookPath("mv")
				if err != nil {
					t.Fatal(err)
				}
				// Fail the rename of the fully extracted candidate after the
				// old install has been moved. SIGKILL bypasses the exit trap.
				script := "#!/bin/sh\n" + `if [ "$1" = "$FAIL_SOURCE" ]; then
    [ "$INTERRUPT" != yes ] || kill -KILL "$PPID"
    exit 1
fi
exec "$REAL_MV" "$@"
`
				if err := os.WriteFile(filepath.Join(base, "mv"), []byte(script), 0755); err != nil {
					t.Fatal(err)
				}
				interrupt := "no"
				if tc == "interrupted activation" {
					interrupt = "yes"
				}
				cmd.Env = append(os.Environ(), "PATH="+base+":"+os.Getenv("PATH"), "FAIL_SOURCE="+filepath.Join(stage, "go"), "REAL_MV="+realMV, "INTERRUPT="+interrupt)
			case "foreign staging directory":
				if err := os.Mkdir(stage, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(stage, "example"), []byte("keep"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			out, err := cmd.CombinedOutput()
			if (err == nil) != (tc == "upgrade") {
				t.Fatalf("replacement: %s (%v)", out, err)
			}
			want := "old"
			if tc == "upgrade" {
				want = "new"
			}
			if tc == "interrupted activation" {
				if got, err := os.ReadFile(filepath.Join(stage, "previous", "bin", "go")); err != nil || string(got) != "old" {
					t.Fatalf("lost interrupted installation: %q (%v)", got, err)
				}
				// Even a failed retry restores the previous executable first.
				if err := os.WriteFile(archive, []byte("bad checksum"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := exec.CommandContext(t.Context(), "sh", args...).Run(); err == nil {
					t.Fatal("bad retry succeeded")
				}
			}
			if got, err := os.ReadFile(bin); err != nil || string(got) != want {
				t.Errorf("live bin/go = %q, want %q (%v)", got, want, err)
			}
			if tc == "foreign staging directory" {
				if got, err := os.ReadFile(filepath.Join(stage, "example")); err != nil || string(got) != "keep" {
					t.Fatalf("lost unrelated staging contents: %q (%v)", got, err)
				}
			} else if _, err := os.Stat(stage); !os.IsNotExist(err) {
				t.Errorf("staging directory still exists: %v", err)
			}
		})
	}
}

func writeGoTestArchive(t *testing.T, name string, executable bool) {
	t.Helper()
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	z := gzip.NewWriter(f)
	tw := tar.NewWriter(z)
	mode := int64(0644)
	if executable {
		mode = 0755
	}
	if err := tw.WriteHeader(&tar.Header{Name: "go/bin/go", Mode: mode, Size: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

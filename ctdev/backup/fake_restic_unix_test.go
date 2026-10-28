//go:build unix

package backup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRestic installs a stand-in restic at ~/.local/bin/restic, where ctdev
// looks before PATH. It answers version, makes a repository on init, and
// fails every key command, so a creation fails right after init. With
// FAKE_RESTIC_FIFO set, the key command instead reports on the fifo and
// waits, so a test can interrupt it. It logs each call with its repository.
func fakeRestic(t *testing.T, m *Machine) (log string) {
	t.Helper()
	bin := filepath.Join(m.Home, ".local", "bin", "restic")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
echo "$1 | $RESTIC_REPOSITORY" >> "$FAKE_RESTIC_LOG"
case "$1" in
version) echo "restic 0.19.1 compiled with go1.26.4 on linux/amd64" ;;
init) case "$RESTIC_REPOSITORY" in /*) mkdir -p "$RESTIC_REPOSITORY/keys" && touch "$RESTIC_REPOSITORY/config" ;; esac ;;
key)
  if [ -n "$FAKE_RESTIC_FIFO" ]; then echo started > "$FAKE_RESTIC_FIFO"; exec sleep 60; fi
  echo "Fatal: example failure" >&2; exit 1 ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	log = filepath.Join(t.TempDir(), "restic.log")
	t.Setenv("FAKE_RESTIC_LOG", log)
	return log
}

func TestResticFromLocalBinIsPreferred(t *testing.T) {
	m := testMachine(t, "example-laptop")
	// With no ~/.local/bin/restic it falls through: to Homebrew's on a Mac
	// that has one, else restic on PATH.
	if got := m.resticBin(); strings.HasPrefix(got, m.Home) {
		t.Errorf("with no ~/.local/bin/restic: %q", got)
	}
	fakeRestic(t, m)
	if got, want := m.resticBin(), filepath.Join(m.Home, ".local", "bin", "restic"); got != want {
		t.Errorf("resticBin = %q, want %q", got, want)
	}
}

//go:build unix

package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
)

// A --all session measures the space stored once, not on every return to
// its menu. A restic in ~/.local/bin, where ctdev looks first, logs each call
// and runs the real one.
func TestAllSessionMeasuresStoredOnce(t *testing.T) {
	m, _ := stopFixture(t)
	real, err := exec.LookPath("restic")
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "restic.log")
	bin := filepath.Join(m.Home, ".local", "bin", "restic")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho \"$*\" >> \"" + log + "\"\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// List backups, Back from the folders, then List backups and Back again,
	// then Quit: the menu comes up three times.
	saved := runScreen
	t.Cleanup(func() { runScreen = saved })
	menus := 0
	runScreen = func(screen tea.Model) error {
		switch s := screen.(type) {
		case *backupscreen.Model:
			menus++
			if menus < 3 {
				s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			} else {
				s.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
			}
		case *backupscreen.Folders:
			s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		default:
			t.Fatalf("unexpected screen %T", s)
		}
		return nil
	}
	var sessionErr error
	out := captureStdout(t, func() { sessionErr = allSession(context.Background(), m, t.TempDir()) })
	if sessionErr != nil {
		t.Fatalf("%v\n%s", sessionErr, out)
	}
	if menus != 3 {
		t.Fatalf("the menu came up %d times, want 3", menus)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	stats := 0
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "stats ") {
			stats++
		}
	}
	if stats != 1 {
		t.Errorf("restic stats ran %d times in the session, want once:\n%s", stats, data)
	}
}

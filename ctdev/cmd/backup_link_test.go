package cmd

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
)

// Configure on a new folder offers "Connect to an existing folder's
// backups". In the folder list, enter continues the chosen folder's history;
// Back, q or esc starts a new history and Configure carries on; Ctrl+C
// cancels Configure.
func TestConfigureLinkPicker(t *testing.T) {
	m, stopped := stopFixture(t)
	old, err := backup.ReadConfig(stopped)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.StopBackingUp(stopped); err != nil {
		t.Fatal(err)
	}
	fakeSchedulerInstall(t)
	ctx := context.Background()

	// Enter goes last: once a live folder holds the stopped folder's ID, the
	// list no longer offers it.
	for _, tc := range []struct {
		name      string
		key       tea.KeyPressMsg
		cancelled bool
		continues bool // takes the stopped folder's ID
	}{
		{name: "esc", key: tea.KeyPressMsg{Code: tea.KeyEscape}},
		{name: "q", key: tea.KeyPressMsg{Code: 'q', Text: "q"}},
		{name: "ctrl+c", key: tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, cancelled: true},
		{name: "enter", key: tea.KeyPressMsg{Code: tea.KeyEnter}, continues: true},
	} {
		name := tc.name
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(os.Getenv("HOME"), "Sites", strings.ReplaceAll(name, "+", "-"))
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			saved, savedScreen := stdinScanner, runScreen
			t.Cleanup(func() { stdinScanner, runScreen = saved, savedScreen })
			stdinScanner = bufio.NewScanner(strings.NewReader(strings.Join([]string{
				"",                 // destination: 1) example-backups
				"2",                // Connect to an existing folder's backups
				"",                 // Daily
				"", "", "", "", "", // default retention
			}, "\n") + "\n"))
			picked := false
			runScreen = func(screen tea.Model) error {
				list, ok := screen.(*backupscreen.Folders)
				if !ok {
					t.Fatalf("unexpected screen %T", screen)
				}
				picked = true
				list.Update(tc.key)
				return nil
			}

			var cfgErr error
			out := captureStdout(t, func() { cfgErr = configureFolder(ctx, m, m.Status(ctx, dir)) })
			if !picked {
				t.Fatalf("the folder list wasn't shown:\n%s", out)
			}
			cfg, readErr := backup.ReadConfig(dir)
			if tc.cancelled {
				if !errors.Is(cfgErr, errPromptCancelled) || !os.IsNotExist(readErr) {
					t.Errorf("want Configure cancelled with no config written, got %v, %v", cfgErr, readErr)
				}
				return
			}
			if cfgErr != nil || readErr != nil {
				t.Fatalf("Configure: %v, %v\n%s", cfgErr, readErr, out)
			}
			if (cfg.ID == old.ID) != tc.continues {
				t.Errorf("ID %s, the stopped folder's %s: continues = %v, want %v", cfg.ID, old.ID, cfg.ID == old.ID, tc.continues)
			}
		})
	}
}

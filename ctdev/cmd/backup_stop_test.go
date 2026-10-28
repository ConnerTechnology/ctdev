package cmd

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
)

// answerYes stands in for the screens: it says yes to each question and
// types typed where a name is asked for.
func answerYes(t *testing.T, typed string) {
	t.Helper()
	saved := runScreen
	t.Cleanup(func() { runScreen = saved })
	runScreen = func(screen tea.Model) error {
		send := func(k tea.KeyPressMsg) { screen.Update(k) }
		switch s := screen.(type) {
		case *backupscreen.Confirm:
			send(tea.KeyPressMsg{Code: 'y', Text: "y"})
		case *backupscreen.ConfirmDeleteAll:
			for _, r := range typed {
				send(tea.KeyPressMsg{Code: r, Text: string(r)})
			}
			send(tea.KeyPressMsg{Code: tea.KeyEnter})
		default:
			t.Fatalf("unexpected screen %T", s)
		}
		return nil
	}
}

// fakeSchedulerRemoval counts Stop backing up's scheduler removals.
func fakeSchedulerRemoval(t *testing.T) *int {
	t.Helper()
	n := new(int)
	saved := removeSchedulerIfIdle
	removeSchedulerIfIdle = func(*backup.Machine, context.Context) (bool, error) { *n++; return true, nil }
	t.Cleanup(func() { removeSchedulerIfIdle = saved })
	return n
}

// stopFixture is a configured, listed folder whose destination is a real
// local restic repository opened by this computer's key.
func stopFixture(t *testing.T) (*backup.Machine, string) {
	t.Helper()
	if _, err := exec.LookPath("restic"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("restic is not on PATH; CI must install it before the Go tests")
		}
		t.Skip("restic is not installed; install it (ctdev install restic) to run this test")
	}
	dir := backupFolderForTest(t)
	t.Setenv("RESTIC_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	repo := filepath.Join(t.TempDir(), "repo")
	init := exec.Command("restic", "init", "--repo", repo)
	init.Env = append(os.Environ(), "RESTIC_PASSWORD=example-computer-key")
	if out, err := init.CombinedOutput(); err != nil {
		t.Fatalf("restic init: %v\n%s", err, out)
	}
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: &bytes.Buffer{}})
	if err != nil {
		t.Fatal(err)
	}
	env := filepath.Join(m.Dir, "destinations", "example-backups.env")
	if err := os.MkdirAll(filepath.Dir(env), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("RESTIC_REPOSITORY="+repo+"\nCTDEV_COMPUTER_KEY=example-computer-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := m.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	cfg := backup.NewConfig("example-backups")
	ctx := context.Background()
	if err := m.Configure(ctx, d, dir, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Backup(ctx, d, dir, &cfg); err != nil {
		t.Fatal(err)
	}
	return m, dir
}

// Stop backing up takes the folder off the list and then removes the
// hourly runner if no folder is left, whether or not it deletes the
// backups too.
func TestStopBackingUpRemovesTheSchedulerWhenIdle(t *testing.T) {
	for name, typed := range map[string]string{
		"keep the backups":   "",
		"delete the backups": "website",
	} {
		t.Run(name, func(t *testing.T) {
			m, dir := stopFixture(t)
			removals := fakeSchedulerRemoval(t)
			if typed == "" {
				// Say yes to stopping, no to deleting.
				saved := runScreen
				t.Cleanup(func() { runScreen = saved })
				asked := 0
				runScreen = func(screen tea.Model) error {
					asked++
					key := tea.KeyPressMsg{Code: 'n', Text: "n"}
					if asked == 1 {
						key = tea.KeyPressMsg{Code: 'y', Text: "y"}
					}
					screen.Update(key)
					return nil
				}
			} else {
				answerYes(t, typed)
			}
			ctx := context.Background()
			var err error
			captureStdout(t, func() { err = stopBackingUp(ctx, m, m.Status(ctx, dir)) })
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, backup.ConfigFileName)); !os.IsNotExist(err) {
				t.Fatal("the folder is still configured")
			}
			if *removals != 1 {
				t.Errorf("RemoveSchedulerIfIdle called %d times, want once", *removals)
			}
		})
	}
}

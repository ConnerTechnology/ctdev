//go:build unix

package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
)

// I lost this computer can't be run again for a computer it already
// removed, so when it stops it says what it did and which item finishes it;
// Replace backend credentials is that item, behind the master password.
func TestALostComputerFlowThatStopsSaysHowToFinish(t *testing.T) {
	real, err := exec.LookPath("restic")
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("restic is not on PATH; CI must install it before the Go tests")
		}
		t.Skip("restic is not installed; install it (ctdev install restic) to run this test")
	}
	t.Setenv("RESTIC_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	ctx := context.Background()
	const master = "correct horse example staple"
	laptop := exampleMachine(t, "example-laptop")
	setup, err := laptop.CreateDestination(ctx, backup.NewDestination{Name: "example-backups", Address: filepath.Join(t.TempDir(), "repo")}, master)
	if err != nil {
		t.Fatal(err)
	}
	ld := setup.Destination
	for _, host := range []string{"example-desktop", "example-server"} {
		if _, err := exampleMachine(t, host).ConnectDestination(ctx, setup.Kit, master); err != nil {
			t.Fatal(err)
		}
	}
	computer := func(host string) backup.Computer {
		cs, err := ld.Computers(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range cs {
			if c.Hostname == host {
				return c
			}
		}
		t.Fatalf("no %s", host)
		return backup.Computer{}
	}
	confirmEveryScreen(t)

	// It stops at step 2: restic can't add the new account key.
	laptop.Restic = filepath.Join(t.TempDir(), "restic")
	script := "#!/bin/sh\nif [ \"$1\" = key ] && [ \"$2\" = add ]; then echo 'Fatal: example failure' >&2; exit 1; fi\nexec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(laptop.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	typeLines(t, master)
	var flowErr error
	out := captureStdout(t, func() { flowErr = lostComputer(ctx, ld, computer("example-desktop")) })
	laptop.Restic = ""
	if flowErr == nil {
		t.Fatalf("want the failure reported:\n%s", out)
	}
	for _, want := range []string{"So far it removed example-desktop's key. The Secret Key and the backend credentials are unchanged",
		"Emergency Kit and password, then Regenerate Secret Key, then Replace backend credentials"} {
		if !strings.Contains(out, want) {
			t.Errorf("stopped at step 2: output is missing %q:\n%s", want, out)
		}
	}

	// It stops at the end: the new kit is never shown.
	typeLines(t, master)
	out = captureStdout(t, func() { flowErr = lostComputer(ctx, ld, computer("example-server")) })
	if !errors.Is(flowErr, errPromptCancelled) {
		t.Fatalf("want the kit's prompt cancelled, got %v:\n%s", flowErr, out)
	}
	for _, want := range []string{"made a new Secret Key and saved the new backend credentials. The new kit wasn't shown",
		"then Show Emergency Kit"} {
		if !strings.Contains(out, want) {
			t.Errorf("stopped at the kit: output is missing %q:\n%s", want, out)
		}
	}

	// Replace backend credentials, from Emergency Kit and password: three
	// wrong master passwords change nothing; the right one shows the kit.
	saved := runScreen
	t.Cleanup(func() { runScreen = saved })
	menus := 0
	runScreen = func(screen tea.Model) error {
		m, ok := screen.(*backupscreen.Model)
		if !ok {
			t.Fatalf("unexpected screen %T", screen)
		}
		menus++
		if menus%2 == 0 {
			m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
			return nil
		}
		for range 3 {
			m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if m.Chosen() != backupscreen.ReplaceCredentials {
			t.Fatalf("chose %v, want Replace backend credentials", m.Chosen())
		}
		return nil
	}
	typeLines(t, "wrong example one", "wrong example two", "wrong example three")
	out = captureStdout(t, func() { _, flowErr = kitAndPassword(ctx, laptop, ld) })
	if flowErr == nil || !strings.Contains(flowErr.Error(), "three wrong tries") || strings.Contains(out, "New Emergency Kit") {
		t.Errorf("wrong master passwords: %v\n%s", flowErr, out)
	}
	menus = 0
	typeLines(t, master, "")
	out = captureStdout(t, func() { _, flowErr = kitAndPassword(ctx, laptop, ld) })
	if flowErr != nil {
		t.Fatalf("%v\n%s", flowErr, out)
	}
	for _, want := range []string{"no credentials to revoke", "This computer opens example-backups with the new credentials",
		"New Emergency Kit", "Secret Key:", "Paste an updated Emergency Kit"} {
		if !strings.Contains(out, want) {
			t.Errorf("Replace backend credentials: output is missing %q:\n%s", want, out)
		}
	}
}

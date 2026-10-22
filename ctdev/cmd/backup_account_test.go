package cmd

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
)

func exampleMachine(t *testing.T, host string) *backup.Machine {
	t.Helper()
	home := t.TempDir()
	return &backup.Machine{
		Dir: filepath.Join(home, ".config", "ctdev", "backup"), Home: home,
		Hostname: host, Username: "example", Opts: sysutil.Opts{Stdout: &bytes.Buffer{}},
	}
}

func secretKeyOf(t *testing.T, kit string) string {
	t.Helper()
	k, err := backup.ParseKit(kit)
	if err != nil {
		t.Fatal(err)
	}
	return k.SecretKey
}

func typeLines(t *testing.T, lines ...string) {
	t.Helper()
	saved := stdinScanner
	t.Cleanup(func() { stdinScanner = saved })
	stdinScanner = bufio.NewScanner(strings.NewReader(strings.Join(lines, "\n") + "\n"))
}

// confirmEveryScreen answers every confirmation y, and fails on any other screen.
func confirmEveryScreen(t *testing.T) *int {
	t.Helper()
	saved := runScreen
	t.Cleanup(func() { runScreen = saved })
	n := new(int)
	runScreen = func(screen tea.Model) error {
		c, ok := screen.(*backupscreen.Confirm)
		if !ok {
			t.Fatalf("unexpected screen %T", screen)
		}
		*n++
		c.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
		return nil
	}
	return n
}

// "I lost this computer" from the laptop, on a local destination: the
// desktop is locked out, the Secret Key is new, and the server takes the new
// kit by pasting it, without the master password.
func TestLostComputerFlowAndPastingTheNewKit(t *testing.T) {
	if _, err := exec.LookPath("restic"); err != nil {
		if os.Getenv("CI") != "" {
			t.Fatal("restic is not on PATH; CI must install it before the Go tests")
		}
		t.Skip("restic is not installed; install it (ctdev install restic) to run this test")
	}
	t.Setenv("RESTIC_CACHE_DIR", filepath.Join(t.TempDir(), "cache"))
	ctx := context.Background()
	const master = "correct horse example staple"
	laptop, desktop, server := exampleMachine(t, "example-laptop"), exampleMachine(t, "example-desktop"), exampleMachine(t, "example-server")
	setup, err := laptop.CreateDestination(ctx, backup.NewDestination{Name: "example-backups", Address: filepath.Join(t.TempDir(), "repo")}, master)
	if err != nil {
		t.Fatal(err)
	}
	ld := setup.Destination
	if _, err := desktop.ConnectDestination(ctx, setup.Kit, master); err != nil {
		t.Fatal(err)
	}
	if _, err := server.ConnectDestination(ctx, setup.Kit, master); err != nil {
		t.Fatal(err)
	}
	var lost backup.Computer
	cs, err := ld.Computers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Hostname == "example-desktop" {
			lost = c
		}
	}

	// Three wrong master passwords change nothing.
	confirms := confirmEveryScreen(t)
	typeLines(t, "wrong example one", "wrong example two", "wrong example three")
	var flowErr error
	captureStdout(t, func() { flowErr = lostComputer(ctx, ld, lost) })
	if flowErr == nil || !strings.Contains(flowErr.Error(), "three wrong tries") {
		t.Fatalf("wrong master passwords: %v", flowErr)
	}
	if after, _ := ld.Computers(ctx); len(after) != 3 {
		t.Fatalf("a wrong master password changed the computers: %+v", after)
	}

	typeLines(t, "wrong example one", master, "", "")
	out := captureStdout(t, func() { flowErr = lostComputer(ctx, ld, lost) })
	if flowErr != nil {
		t.Fatalf("%v\n%s", flowErr, out)
	}
	if *confirms != 2 {
		t.Errorf("confirmed %d times, want once per run", *confirms)
	}
	for _, want := range []string{"1. Removed example-desktop's key", "2. Made a new Secret Key", "no credentials to revoke",
		"3. This computer opens example-backups", "New Emergency Kit", "Secret Key:", "Paste an updated Emergency Kit"} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	if d, err := desktop.Destination("example-backups"); err != nil {
		t.Fatal(err)
	} else if _, err := d.Computers(ctx); !errors.Is(err, backup.ErrWrongPassword) {
		t.Errorf("the lost desktop still opens the destination: %v", err)
	}
	saved, err := laptop.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	a, err := saved.Unlock(ctx, master)
	if err != nil {
		t.Fatal(err)
	}
	kit := a.Kit()
	parsed, err := backup.ParseKit(kit)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, parsed.SecretKey) || parsed.SecretKey == secretKeyOf(t, setup.Kit) {
		t.Errorf("the kit shown isn't the saved one:\n%s", out)
	}
	if _, err := exampleMachine(t, "example-tablet").ConnectDestination(ctx, setup.Kit, master); !errors.Is(err, backup.ErrWrongPassword) {
		t.Errorf("the old kit still connects: %v", err)
	}

	// The server pastes the new kit where it would connect: no master
	// password is asked, and its key vouches for the kit.
	typeLines(t, append(strings.Split(strings.TrimRight(kit, "\n"), "\n"), "")...)
	var sd *backup.Destination
	out = captureStdout(t, func() { sd, flowErr = connectDestination(ctx, server) })
	if flowErr != nil {
		t.Fatalf("%v\n%s", flowErr, out)
	}
	if strings.Contains(out, "Master password") {
		t.Errorf("pasting an updated kit asked for the master password:\n%s", out)
	}
	if _, err := sd.Unlock(ctx, master); err != nil {
		t.Errorf("the server after pasting the new kit: %v", err)
	}
}

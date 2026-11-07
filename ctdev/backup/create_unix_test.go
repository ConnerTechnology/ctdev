//go:build unix

package backup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestFailedCreateExpandsTildeAndRemovesTheRepository(t *testing.T) {
	m := testMachine(t, "example-laptop")
	log := fakeRestic(t, m)
	_, err := m.CreateDestination(context.Background(), NewDestination{Name: "example-backups", Address: "~/Backups"}, exampleMaster)
	if err == nil {
		t.Fatal("want the fake key add to fail the creation")
	}
	repo := filepath.Join(m.Home, "Backups")
	if data, _ := os.ReadFile(log); !strings.Contains(string(data), "init | "+repo+"\n") {
		t.Errorf("init didn't run at %s:\n%s", repo, data)
	}
	if _, err := os.Stat(repo); !os.IsNotExist(err) {
		t.Errorf("the half-created repository at %s is still there", repo)
	}
	if !strings.Contains(err.Error(), "removed") {
		t.Errorf("error should say the repository was removed: %v", err)
	}
	if names, _ := m.Destinations(); len(names) != 0 {
		t.Errorf("saved %v, want nothing", names)
	}
}

func TestFailedCreateEmptiesAnExistingEmptyFolder(t *testing.T) {
	m := testMachine(t, "example-laptop")
	fakeRestic(t, m)
	usb := filepath.Join(m.Home, "usb")
	if err := os.Mkdir(usb, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateDestination(context.Background(), NewDestination{Name: "example-backups", Address: usb}, exampleMaster); err == nil {
		t.Fatal("want a failure")
	}
	entries, err := os.ReadDir(usb)
	if err != nil || len(entries) != 0 {
		t.Errorf("%s: %d entries left, err %v; want it there and empty", usb, len(entries), err)
	}
}

func TestFailedCreateOnARemoteBackendSaysToDeleteIt(t *testing.T) {
	m := testMachine(t, "example-laptop")
	fakeRestic(t, m)
	addr := SFTPAddress("backup@nas.example.invalid", "/srv/backups")
	_, err := m.CreateDestination(context.Background(), NewDestination{Name: "example-backups", Address: addr}, exampleMaster)
	if err == nil || !strings.Contains(err.Error(), addr+" was left half-created") || !strings.Contains(err.Error(), "delete it at the provider") {
		t.Errorf("want a plain note about the half-created repository, got %v", err)
	}
}

// Ctrl-C partway through creation cleans up like any other failure.
func TestInterruptedCreateRemovesTheRepository(t *testing.T) {
	m := testMachine(t, "example-laptop")
	fakeRestic(t, m)
	fifo := filepath.Join(t.TempDir(), "started")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_RESTIC_FIFO", fifo)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		// Opening the fifo blocks until the fake key add writes to it.
		data, _ := os.ReadFile(fifo)
		_ = data
		cancel()
	}()
	repo := filepath.Join(m.Home, "Backups")
	_, err := m.CreateDestination(ctx, NewDestination{Name: "example-backups", Address: repo}, exampleMaster)
	if err == nil {
		t.Fatal("want the interrupted creation to fail")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "removed") {
		t.Errorf("err = %v", err)
	}
	if _, err := os.Stat(repo); !os.IsNotExist(err) {
		t.Errorf("the half-created repository at %s is still there", repo)
	}
}

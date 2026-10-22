//go:build unix

package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forged adds a line to kit that restic would run as a password command:
// it creates marker and prints password, so restic would still open the
// repository and nothing would look wrong (CTD-130).
func forged(t *testing.T, kit, password string) (forgedKit, marker string) {
	t.Helper()
	marker = filepath.Join(t.TempDir(), "forged-command-ran")
	line := "RESTIC_PASSWORD_COMMAND: sh -c 'touch " + marker + "; printf %s " + password + "'"
	return strings.TrimRight(kit, "\n") + "\n" + line + "\n", marker
}

func neverRan(t *testing.T, marker string) {
	t.Helper()
	if _, err := os.Stat(marker); err == nil {
		t.Error("the kit's RESTIC_PASSWORD_COMMAND ran")
	}
}

func refusesTheLine(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "RESTIC_PASSWORD_COMMAND") {
		t.Errorf("want the kit refused, naming RESTIC_PASSWORD_COMMAND; got %v", err)
	}
}

// Connecting with a forged kit is refused before restic runs, and saves
// nothing.
func TestConnectingRefusesAKitThatSetsResticsEnvironment(t *testing.T) {
	requireRestic(t)
	_, s := createExample(t, testMachine(t, "example-laptop"))
	kit, marker := forged(t, s.Kit, accountPassword(exampleMaster, mustSecretKey(t, s.Kit)))
	desktop := testMachine(t, "example-desktop")
	_, err := desktop.ConnectDestination(context.Background(), kit, exampleMaster)
	refusesTheLine(t, err)
	neverRan(t, marker)
	if names, _ := desktop.Destinations(); len(names) != 0 {
		t.Errorf("saved %v", names)
	}
}

// Pasting a forged kit on a computer that's set up is refused, and its
// destination file is unchanged.
func TestPastingRefusesAKitThatSetsResticsEnvironment(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, s := createExample(t, m)
	before, err := os.ReadFile(m.destinationFile(d.Name))
	if err != nil {
		t.Fatal(err)
	}
	kit, marker := forged(t, s.Kit, d.computerKey)
	_, err = m.UpdateFromKit(context.Background(), kit)
	refusesTheLine(t, err)
	neverRan(t, marker)
	if after, _ := os.ReadFile(m.destinationFile(d.Name)); string(after) != string(before) {
		t.Error("the destination file changed")
	}
}

// A Recovery Code reset from a forged kit is refused before restic runs.
func TestRecoveringRefusesAKitThatSetsResticsEnvironment(t *testing.T) {
	requireRestic(t)
	_, s := createExample(t, testMachine(t, "example-laptop"))
	kit, marker := forged(t, s.Kit, s.RecoveryCode)
	tablet := testMachine(t, "example-tablet")
	_, err := tablet.RecoverFromKit(context.Background(), kit, s.RecoveryCode, "a new example password", false)
	refusesTheLine(t, err)
	neverRan(t, marker)
	if names, _ := tablet.Destinations(); len(names) != 0 {
		t.Errorf("saved %v", names)
	}
}

// A destination file that already holds such a line, as one saved by a
// v17.0.0 connect could, isn't loaded, and restic never runs with it.
func TestADestinationFileWithAnUnexpectedNameIsRefused(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	marker := filepath.Join(t.TempDir(), "forged-command-ran")
	f, err := os.OpenFile(m.destinationFile(d.Name), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("RESTIC_PASSWORD_COMMAND=sh -c 'touch " + marker + "; printf %s " + d.computerKey + "'\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	loaded, err := m.Destination(d.Name)
	if err == nil {
		_, _ = loaded.Computers(context.Background())
	}
	if err == nil || !strings.Contains(err.Error(), "RESTIC_PASSWORD_COMMAND") || !strings.Contains(err.Error(), m.destinationFile(d.Name)) {
		t.Errorf("want the file refused, naming the line and the file; got %v", err)
	}
	neverRan(t, marker)
}

func mustSecretKey(t *testing.T, kit string) secretKey {
	t.Helper()
	sk, err := parseSecretKey(secretKeyFrom(t, kit))
	if err != nil {
		t.Fatal(err)
	}
	return sk
}

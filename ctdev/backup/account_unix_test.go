//go:build unix

package backup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// failKeyRemove points m at a restic that runs the real one for everything
// but `key remove`, which fails: a failure after a key swap has added the
// new key and before it removed the old one.
func failKeyRemove(t *testing.T, m *Machine) {
	t.Helper()
	real, err := exec.LookPath("restic")
	if err != nil {
		t.Fatal(err)
	}
	m.Restic = filepath.Join(t.TempDir(), "restic")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = key ] && [ \"$2\" = remove ]; then echo 'Fatal: example failure' >&2; exit 1; fi\n" +
		"exec \"" + real + "\" \"$@\"\n"
	if err := os.WriteFile(m.Restic, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// When the old account key can't be removed, the new one is already in
// place: both master passwords unlock, never neither. Trying again finishes
// the swap.
func TestAMasterPasswordChangeThatFailsMidwayLeavesAWorkingKey(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	const newMaster = "a new example password"

	failKeyRemove(t, m)
	if err := unlock(t, d, exampleMaster).ChangeMasterPassword(ctx, newMaster); err == nil {
		t.Fatal("want the failed removal reported")
	}
	m.Restic = ""
	unlock(t, d, exampleMaster)
	a := unlock(t, d, newMaster)

	if err := a.ChangeMasterPassword(ctx, newMaster+" again"); err != nil {
		t.Fatal(err)
	}
	for _, old := range []string{exampleMaster, newMaster} {
		if _, err := d.Unlock(ctx, old); !errors.Is(err, ErrWrongPassword) {
			t.Errorf("%q after the retry: %v, want ErrWrongPassword", old, err)
		}
	}
	unlock(t, d, newMaster+" again")
}

// A new Secret Key whose old account key can't be removed is still saved
// and its kit returned; one that can't be saved here is taken back out, so
// the old kit and Secret Key go on working.
func TestARegeneratedSecretKeyThatFailsMidwayLeavesAWorkingKit(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, s := createExample(t, m)

	// Saving fails: a file stands where the destinations folder was.
	a := unlock(t, d, exampleMaster)
	dir := filepath.Dir(m.destinationFile(d.Name))
	if err := os.Rename(dir, dir+".moved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir, []byte("example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := a.RegenerateSecretKey(ctx)
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir+".moved", dir); err != nil {
		t.Fatal(err)
	}
	if err == nil {
		t.Fatal("want the failed save reported")
	}
	if got := unlock(t, d, exampleMaster).Kit(); got != s.Kit {
		t.Error("a Secret Key that wasn't saved replaced the kit")
	}
	if _, err := testMachine(t, "example-tablet").ConnectDestination(ctx, s.Kit, exampleMaster); err != nil {
		t.Errorf("the old kit after the failed regeneration: %v", err)
	}

	// Removing the old account key fails: the new kit is in force anyway.
	failKeyRemove(t, m)
	kit, err := unlock(t, d, exampleMaster).RegenerateSecretKey(ctx)
	m.Restic = ""
	if err == nil || kit == "" {
		t.Fatalf("RegenerateSecretKey = %q, %v; want the new kit and the failed removal", kit, err)
	}
	saved, err := m.Destination(d.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got := unlock(t, saved, exampleMaster).Kit(); got != kit {
		t.Error("the new Secret Key wasn't saved")
	}
	if _, err := testMachine(t, "example-tablet").ConnectDestination(ctx, kit, exampleMaster); err != nil {
		t.Errorf("the new kit: %v", err)
	}
}

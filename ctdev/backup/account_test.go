package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// fleet is three computers set up on one destination in a local repository:
// the laptop created it, and the desktop and the server connected with its
// kit and master password. Each has its own config dir.
type fleet struct {
	laptop, desktop, server *Machine
	ld, dd, sd              *Destination
	setup                   Setup
}

// fleetTemplate is one fleet set up for the whole package run, since each
// connect costs restic several deliberately slow key derivations. Each test
// gets a copy of its repository and config dirs.
var fleetTemplate struct {
	once  sync.Once
	dir   string
	setup Setup
	err   error
}

var fleetHosts = []string{"example-laptop", "example-desktop", "example-server"}

// fleetMachine is host's machine under root, as the fleet lays them out.
func fleetMachine(root, host string) *Machine {
	home := filepath.Join(root, host)
	return &Machine{
		Dir: filepath.Join(home, ".config", "ctdev", "backup"), Home: home,
		Hostname: host, Username: "example", Opts: sysutil.Opts{Stdout: &bytes.Buffer{}},
	}
}

func newFleet(t *testing.T) fleet {
	t.Helper()
	ctx := context.Background()
	_, s := createExample(t, testMachine(t, "example-laptop"))
	fleetTemplate.once.Do(func() {
		if fleetTemplate.dir, fleetTemplate.err = os.MkdirTemp("", "ctdev-backup-fleet-"); fleetTemplate.err != nil {
			return
		}
		repo := filepath.Join(fleetTemplate.dir, "repo")
		k, err := ParseKit(s.Kit)
		if err != nil {
			fleetTemplate.err = err
			return
		}
		if fleetTemplate.err = os.CopyFS(repo, os.DirFS(k.Address)); fleetTemplate.err != nil {
			return
		}
		laptop := fleetMachine(fleetTemplate.dir, "example-laptop")
		d := *s.Destination
		d.Address, d.m = repo, laptop
		if fleetTemplate.err = laptop.save(&d); fleetTemplate.err != nil {
			return
		}
		fleetTemplate.setup = s
		fleetTemplate.setup.Destination = &d
		fleetTemplate.setup.Kit = strings.Replace(s.Kit, k.Address, repo, 1)
		for _, host := range fleetHosts[1:] {
			if _, fleetTemplate.err = fleetMachine(fleetTemplate.dir, host).ConnectDestination(ctx, fleetTemplate.setup.Kit, exampleMaster); fleetTemplate.err != nil {
				return
			}
		}
	})
	if fleetTemplate.err != nil {
		t.Fatal(fleetTemplate.err)
	}

	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(fleetTemplate.dir)); err != nil {
		t.Fatal(err)
	}
	oldRepo, repo := filepath.Join(fleetTemplate.dir, "repo"), filepath.Join(root, "repo")
	var ms []*Machine
	var ds []*Destination
	for _, host := range fleetHosts {
		m := fleetMachine(root, host)
		file := m.destinationFile("example-backups")
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(strings.Replace(string(data), oldRepo, repo, 1)), 0o600); err != nil {
			t.Fatal(err)
		}
		d, err := m.Destination("example-backups")
		if err != nil {
			t.Fatal(err)
		}
		ms, ds = append(ms, m), append(ds, d)
	}
	f := fleet{laptop: ms[0], desktop: ms[1], server: ms[2], ld: ds[0], dd: ds[1], sd: ds[2], setup: fleetTemplate.setup}
	f.setup.Destination = ds[0]
	f.setup.Kit = strings.Replace(fleetTemplate.setup.Kit, oldRepo, repo, 1)
	return f
}

// opens reports whether a computer's saved key still opens the destination.
func opens(t *testing.T, m *Machine, name string) error {
	t.Helper()
	d, err := m.Destination(name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Computers(context.Background())
	return err
}

func computerNamed(t *testing.T, d *Destination, host string) Computer {
	t.Helper()
	cs, err := d.Computers(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cs {
		if c.Hostname == host {
			return c
		}
	}
	t.Fatalf("no computer %s in %+v", host, cs)
	return Computer{}
}

func unlock(t *testing.T, d *Destination, master string) *Account {
	t.Helper()
	a, err := d.Unlock(context.Background(), master)
	if err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	return a
}

// Removing the desktop's key locks the desktop out; the laptop and the
// server keep working. The laptop can't remove its own key.
func TestRemovingAComputerLocksOnlyThatComputerOut(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	f := newFleet(t)

	a := unlock(t, f.ld, exampleMaster)
	if err := a.RemoveComputer(ctx, computerNamed(t, f.ld, "example-laptop")); !errors.Is(err, ErrThisComputer) {
		t.Errorf("removing this computer's own key: %v, want ErrThisComputer", err)
	}
	if err := a.RemoveComputer(ctx, computerNamed(t, f.ld, "example-desktop")); err != nil {
		t.Fatal(err)
	}
	if err := opens(t, f.desktop, f.dd.Name); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("the removed desktop: %v, want ErrWrongPassword", err)
	}
	for _, m := range []*Machine{f.laptop, f.server} {
		if err := opens(t, m, f.ld.Name); err != nil {
			t.Errorf("%s after removing the desktop: %v", m.Hostname, err)
		}
	}
	cs, err := f.ld.Computers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var hosts []string
	for _, c := range cs {
		hosts = append(hosts, c.Hostname)
	}
	if strings.Join(hosts, ",") != "example-laptop,example-server" {
		t.Errorf("computers = %v", hosts)
	}
}

// Every account action starts at Unlock, which a wrong master password
// can't pass; and a Recovery Code that isn't one opens nothing.
func TestAWrongMasterPasswordBlocksEveryAccountAction(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	f := newFleet(t)
	if a, err := f.ld.Unlock(ctx, "not the example password"); !errors.Is(err, ErrWrongPassword) || a != nil {
		t.Errorf("Unlock with a wrong master password = %v, %v; want ErrWrongPassword and no account", a, err)
	}
	if _, err := f.ld.Recover(ctx, newRecoveryCode(), "a new example password", false); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("Recover with a wrong Recovery Code: %v, want ErrWrongPassword", err)
	}
	// The Recovery Code isn't the master password either.
	if _, err := f.ld.Unlock(ctx, f.setup.RecoveryCode); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("Unlock with the Recovery Code: %v, want ErrWrongPassword", err)
	}
	if got := unlock(t, f.ld, exampleMaster).Kit(); got != f.setup.Kit {
		t.Errorf("Kit() =\n%s\nwant the kit shown at creation:\n%s", got, f.setup.Kit)
	}
}

// After a master password change the old password fails, the new one works,
// the kit is the same, and every computer's key still works.
func TestChangingTheMasterPasswordKeepsTheKitAndTheComputers(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	f := newFleet(t)
	const newMaster = "a new example password"

	if err := unlock(t, f.dd, exampleMaster).ChangeMasterPassword(ctx, newMaster); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ld.Unlock(ctx, exampleMaster); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("the old master password: %v, want ErrWrongPassword", err)
	}
	if a := unlock(t, f.ld, newMaster); a.Kit() != f.setup.Kit {
		t.Error("the kit changed with the master password")
	}
	// A new computer connects with the same kit and the new password.
	if _, err := testMachine(t, "example-tablet").ConnectDestination(ctx, f.setup.Kit, newMaster); err != nil {
		t.Errorf("connect with the new master password: %v", err)
	}
	for _, m := range []*Machine{f.laptop, f.desktop, f.server} {
		if err := opens(t, m, f.ld.Name); err != nil {
			t.Errorf("%s: %v", m.Hostname, err)
		}
	}
}

// After a new Secret Key, the old kit with the right password opens nothing
// and the new kit does; this computer saved the new Secret Key.
func TestRegeneratingTheSecretKeyRetiresTheOldKit(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	f := newFleet(t)

	kit, err := unlock(t, f.ld, exampleMaster).RegenerateSecretKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if kit == f.setup.Kit || secretKeyFrom(t, kit) == secretKeyFrom(t, f.setup.Kit) {
		t.Fatal("the new kit has the old Secret Key")
	}
	if _, err := testMachine(t, "example-tablet").ConnectDestination(ctx, f.setup.Kit, exampleMaster); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("the old kit: %v, want ErrWrongPassword", err)
	}
	if _, err := testMachine(t, "example-tablet").ConnectDestination(ctx, kit, exampleMaster); err != nil {
		t.Errorf("the new kit: %v", err)
	}
	// This computer saved it: unlocking here uses the new Secret Key.
	again, err := f.laptop.Destination(f.ld.Name)
	if err != nil {
		t.Fatal(err)
	}
	if got := unlock(t, again, exampleMaster).Kit(); got != kit {
		t.Errorf("the saved kit is\n%s\nwant\n%s", got, kit)
	}
	// The desktop still has the old Secret Key: its key backs up, but the
	// master password no longer unlocks there until it pastes the new kit.
	if err := opens(t, f.desktop, f.dd.Name); err != nil {
		t.Errorf("the desktop's key: %v", err)
	}
	if _, err := f.dd.Unlock(ctx, exampleMaster); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("unlock on the desktop with the old Secret Key: %v, want ErrWrongPassword", err)
	}
}

// Pasting the new kit on a computer that's set up saves its Secret Key and
// credentials once its own key opens the repository, with no master password.
// A kit its key can't open changes nothing.
func TestPastingAnUpdatedKitOnASetUpComputer(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	f := newFleet(t)
	kit, err := unlock(t, f.ld, exampleMaster).RegenerateSecretKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(f.desktop.destinationFile(f.dd.Name))
	if err != nil {
		t.Fatal(err)
	}

	// A kit for another repository under the same name: the desktop's key
	// doesn't open it, so nothing is saved.
	other := testMachine(t, "example-other")
	_, otherSetup := createExample(t, other)
	if _, err := f.desktop.UpdateFromKit(ctx, otherSetup.Kit); err == nil {
		t.Error("a kit the computer's key can't open was accepted")
	}
	if after, _ := os.ReadFile(f.desktop.destinationFile(f.dd.Name)); string(after) != string(before) {
		t.Error("a rejected kit changed the saved destination")
	}
	// A kit for a destination this computer isn't set up on is for connecting.
	if _, err := testMachine(t, "example-tablet").UpdateFromKit(ctx, kit); err == nil {
		t.Error("UpdateFromKit on a computer without the destination should fail")
	}

	d, err := f.desktop.UpdateFromKit(ctx, kit)
	if err != nil {
		t.Fatal(err)
	}
	if got := unlock(t, d, exampleMaster).Kit(); got != kit {
		t.Errorf("after pasting, the desktop's kit is\n%s\nwant\n%s", got, kit)
	}

	// A removed computer's key opens nothing, so its paste is refused.
	if err := unlock(t, f.ld, exampleMaster).RemoveComputer(ctx, computerNamed(t, f.ld, "example-server")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.server.UpdateFromKit(ctx, kit); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("a removed computer pasting the kit: %v, want ErrWrongPassword", err)
	}
}

// The kit plus the Recovery Code set a new master password and Secret Key.
// The Recovery Code keeps working unless a new one is made.
func TestRecoveryCodeResetsTheMasterPasswordAndSecretKey(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	f := newFleet(t)
	const newMaster = "a new example password"

	r, err := f.dd.Recover(ctx, f.setup.RecoveryCode, newMaster, false)
	if err != nil {
		t.Fatal(err)
	}
	if r.RecoveryCode != "" {
		t.Error("no new Recovery Code was asked for")
	}
	if secretKeyFrom(t, r.Kit) == secretKeyFrom(t, f.setup.Kit) {
		t.Error("the Secret Key wasn't replaced")
	}
	if _, err := testMachine(t, "example-tablet").ConnectDestination(ctx, r.Kit, newMaster); err != nil {
		t.Errorf("the new kit and master password: %v", err)
	}
	for _, try := range []struct{ kit, master string }{{f.setup.Kit, exampleMaster}, {r.Kit, exampleMaster}} {
		if _, err := testMachine(t, "example-tablet").ConnectDestination(ctx, try.kit, try.master); !errors.Is(err, ErrWrongPassword) {
			t.Errorf("an old kit or password still connects: %v", err)
		}
	}
	unlock(t, r.Destination, newMaster)

	// The used code still works; asking for a new one retires it.
	r2, err := f.ld.Recover(ctx, f.setup.RecoveryCode, exampleMaster, true)
	if err != nil {
		t.Fatalf("the used Recovery Code: %v", err)
	}
	if r2.RecoveryCode == "" || r2.RecoveryCode == f.setup.RecoveryCode {
		t.Fatalf("new Recovery Code = %q", r2.RecoveryCode)
	}
	if _, err := f.ld.Recover(ctx, f.setup.RecoveryCode, exampleMaster, false); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("the replaced Recovery Code: %v, want ErrWrongPassword", err)
	}
	if _, err := f.ld.Recover(ctx, r2.RecoveryCode, newMaster, false); err != nil {
		t.Errorf("the new Recovery Code: %v", err)
	}
	for _, m := range []*Machine{f.laptop, f.desktop, f.server} {
		if err := opens(t, m, f.ld.Name); err != nil {
			t.Errorf("%s: %v", m.Hostname, err)
		}
	}
}

// When connecting fails for want of the master password, the kit and the
// Recovery Code set a new one and set this computer up.
func TestRecoveryCodeFromTheKitSetsUpANewComputer(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	f := newFleet(t)
	const newMaster = "a new example password"
	tablet := testMachine(t, "example-tablet")
	if _, err := tablet.RecoverFromKit(ctx, f.setup.Kit, "WRONGCOD-EEXAMPLE-EXAMPLEX-EXAMPLEX", newMaster, false); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("a wrong Recovery Code: %v, want ErrWrongPassword", err)
	}
	if names, _ := tablet.Destinations(); len(names) != 0 {
		t.Errorf("a failed recovery saved %v", names)
	}
	r, err := tablet.RecoverFromKit(ctx, f.setup.Kit, f.setup.RecoveryCode, newMaster, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := opens(t, tablet, f.ld.Name); err != nil {
		t.Errorf("the tablet after recovering: %v", err)
	}
	if got := unlock(t, r.Destination, newMaster).Kit(); got != r.Kit {
		t.Errorf("the tablet's kit is\n%s\nwant\n%s", got, r.Kit)
	}
}

// The lost computer's backend credentials are replaced only once this
// computer's key opens the repository with them.
func TestSetCredentialsSavesOnlyWhatOpensTheRepository(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	f := newFleet(t)
	a := unlock(t, f.ld, exampleMaster)
	// A local destination takes no credentials.
	if err := a.SetCredentials(ctx, map[string]string{"B2_ACCOUNT_ID": "example"}); err == nil {
		t.Error("credentials a local destination doesn't use were accepted")
	}
	if err := a.SetCredentials(ctx, nil); err != nil {
		t.Errorf("no credentials for a local destination: %v", err)
	}
}

// ctdev labels its own keys ctdev-account@ctdev and ctdev-recovery@ctdev.
// A computer named ctdev would label its key with that host, so creating,
// connecting and recovering all refuse it, including as user ctdev-account,
// whose key would carry the account key's exact label.
func TestAComputerNamedCtdevIsRefused(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	_, s := createExample(t, testMachine(t, "example-laptop"))
	for _, user := range []string{"example", "ctdev-account"} {
		m := testMachine(t, "ctdev")
		m.Username = user
		refused := func(what string, err error) {
			t.Helper()
			if err == nil || !strings.Contains(err.Error(), "hostname is ctdev") {
				t.Errorf("%s as %s@ctdev: want the hostname refused, got %v", what, user, err)
			}
		}
		_, err := m.CreateDestination(ctx, NewDestination{Name: "example-new", Address: filepath.Join(t.TempDir(), "repo")}, exampleMaster)
		refused("create", err)
		_, err = m.ConnectDestination(ctx, s.Kit, exampleMaster)
		refused("connect", err)
		_, err = m.RecoverFromKit(ctx, s.Kit, s.RecoveryCode, "a new example password", false)
		refused("recover", err)
		if names, _ := m.Destinations(); len(names) != 0 {
			t.Errorf("%s@ctdev saved %v", user, names)
		}
	}
	// The kit still connects a computer with an ordinary name, so nothing
	// above touched the destination.
	if _, err := testMachine(t, "example-desktop").ConnectDestination(ctx, s.Kit, exampleMaster); err != nil {
		t.Error(err)
	}
}

// The account key and the Recovery Code's key aren't computers: removing
// either as one is refused, and both keep working.
func TestRemoveComputerRefusesTheAccountAndRecoveryKeys(t *testing.T) {
	requireRestic(t)
	ctx := context.Background()
	m := testMachine(t, "example-laptop")
	d, s := createExample(t, m)
	keys, err := d.keysHinted(ctx, d.computerKey, d.computerKeyID)
	if err != nil {
		t.Fatal(err)
	}
	a := unlock(t, d, exampleMaster)
	for _, k := range keys {
		if !labeled(k, accountUser) && !labeled(k, recoveryUser) {
			continue
		}
		if err := a.RemoveComputer(ctx, Computer{ID: k.ID, Hostname: k.HostName}); !errors.Is(err, ErrNotAComputer) {
			t.Errorf("removing %s@%s: %v, want ErrNotAComputer", k.UserName, k.HostName, err)
		}
	}
	if _, err := d.keys(ctx, s.RecoveryCode); err != nil {
		t.Errorf("the Recovery Code after the refused removal: %v", err)
	}
	unlock(t, d, exampleMaster)
}

// A Recovery Code opens the repository only through the Recovery Code's
// key: this computer's own key, typed as one, is refused, even though
// restic opens the repository with it.
func TestAComputerKeyIsNotARecoveryCode(t *testing.T) {
	requireRestic(t)
	m := testMachine(t, "example-laptop")
	d, _ := createExample(t, m)
	if _, err := d.Recover(context.Background(), d.computerKey, "a new example password", false); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("Recover with this computer's key: %v, want ErrWrongPassword", err)
	}
	unlock(t, d, exampleMaster)
}

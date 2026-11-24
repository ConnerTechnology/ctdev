//go:build unix

package backupapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

func wrongTries() []any {
	return []any{Values{"master": "wrong example one"}, Values{"master": "wrong example two"}, Values{"master": "wrong example three"}}
}

func anyContains(lines []string, want string) bool {
	return slices.ContainsFunc(lines, func(l string) bool { return strings.Contains(l, want) })
}

// "I lost this computer" from the laptop, on a local destination: three
// wrong master passwords change nothing; then the desktop is locked out, the
// Secret Key is new, and the new kit is shown once.
func TestLostComputer(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t, filepath.Join(t.TempDir(), "repo"), nil)
	lost := f.computer(t, "example-desktop")

	ui := &fakeUI{t: t, Confirms: []any{true}, Forms: wrongTries()}
	err := lostComputer(ctx, ui, f.dest, lost)
	if err == nil || !strings.Contains(err.Error(), "Three wrong tries") {
		t.Fatalf("wrong master passwords: %v", err)
	}
	if after, _ := f.dest.Computers(ctx); len(after) != 3 {
		t.Fatalf("a wrong master password changed the computers: %+v", after)
	}
	forms := requestsOf[Form](ui)
	if forms[1].Fields[0].Error != "That master password doesn't open example-backups." ||
		!strings.Contains(forms[1].Intro, "2 tries left") || !strings.Contains(forms[2].Intro, "1 try left") {
		t.Errorf("a wrong try should say so under the field, with the tries left: %+v", forms[1:])
	}

	ui = &fakeUI{t: t, Confirms: []any{true}, Forms: []any{Values{"master": "wrong example one"}, Values{"master": exampleMaster}},
		Kits: []error{nil}}
	if err := lostComputer(ctx, ui, f.dest, lost); err != nil {
		t.Fatalf("%v; said %+v", err, ui.Said)
	}
	c := requestsOf[Confirm](ui)
	if len(c) != 1 || !c[0].Destructive || !strings.Contains(c[0].Question, "Remove example-desktop's key from example-backups, make a new Secret Key and take new backend credentials?") ||
		!strings.Contains(c[0].Consequence, "every other computer") {
		t.Errorf("confirm = %+v", c)
	}
	var detail []string
	for _, r := range requestsOf[Running](ui) {
		detail = append(detail, r.Detail)
		if r.Label != "Checking the master password…" && r.ChangesKeysOn != "example-backups" {
			t.Errorf("%q changes keys, so it can't be cancelled", r.Label)
		}
	}
	if !anyContains(detail, "no credentials to revoke") {
		t.Errorf("a local destination should say it has no credentials to revoke: %q", detail)
	}
	kits := requestsOf[KitScreen](ui)
	if len(kits) != 1 || !kits[0].New {
		t.Fatalf("kits = %+v, want one new kit", kits)
	}
	k := kits[0]
	for _, want := range []string{"✓ 1. Removed example-desktop's key.", "✓ 2. Made a new Secret Key", "✓ 3. This computer opens example-backups"} {
		if !strings.Contains(k.Page.Body, want) {
			t.Errorf("the checklist is missing %q:\n%s", want, k.Page.Body)
		}
	}
	if !anyContains(ui.said(Info), "Paste an updated Emergency Kit") {
		t.Errorf("said %+v, want where to paste the new kit", ui.Said)
	}

	if d, err := f.desktop.Destination("example-backups"); err != nil {
		t.Fatal(err)
	} else if _, err := d.Computers(ctx); !errors.Is(err, backup.ErrWrongPassword) {
		t.Errorf("the lost desktop still opens the destination: %v", err)
	}
	saved, err := f.laptop.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	a, err := saved.Unlock(ctx, exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	if k.Kit != a.Kit() || secretKeyOf(t, k.Kit) == secretKeyOf(t, f.kit) {
		t.Errorf("the kit shown isn't the new one saved:\n%s", k.Kit)
	}
	if _, err := machineOn(t, f.laptop.Restic, "example-tablet").ConnectDestination(ctx, f.kit, exampleMaster); !errors.Is(err, backup.ErrWrongPassword) {
		t.Errorf("the old kit still connects: %v", err)
	}
}

// I lost this computer can't be run again for a computer it already
// removed, so when it stops it says what it did and which item finishes it,
// on the screen and after ctdev exits.
func TestALostComputerFlowThatStopsSaysHowToFinish(t *testing.T) {
	ctx := context.Background()
	address := filepath.Join(t.TempDir(), "repo")
	f := newAccountFixture(t, address, nil)

	// It stops at step 2: restic can't add the new account key.
	failAdd := filepath.Join(standinRepo(f.root, address), "fail-add")
	if err := os.WriteFile(failAdd, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ui := &fakeUI{t: t, Confirms: []any{true}, Forms: []any{Values{"master": exampleMaster}}, Dashboards: []any{"e"}}
	if err := lostComputer(ctx, ui, f.dest, f.computer(t, "example-desktop")); !errors.Is(err, errToKitMenu) {
		t.Fatalf("err = %v, want e to open Emergency Kit and password", err)
	}
	os.Remove(failAdd)
	if !slices.Equal(ui.said(Warning), []string{"I lost this computer stopped. So far it removed example-desktop's key. The Secret Key and the backend credentials are unchanged."}) ||
		!slices.Equal(ui.said(Plain), []string{"To finish, run ctdev backup --all, choose Emergency Kit and password, then Regenerate Secret Key, then Replace backend credentials."}) {
		t.Errorf("stopped at step 2: said %+v", ui.Said)
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 1 || dash[0].Page.Badge != (Badge{Attention, "STOPPED"}) || !strings.Contains(dash[0].Page.Body, "✗ 2. Make a new Secret Key") ||
		!strings.Contains(dash[0].Page.Body, "example failure") || len(dash[0].Message) != 2 {
		t.Errorf("stopped screen = %+v", dash)
	}

	// It stops at the end: the new kit is left unsaved.
	ui = &fakeUI{t: t, Confirms: []any{true}, Forms: []any{Values{"master": exampleMaster}}, Kits: []error{ErrBack}, Dashboards: []any{"esc"}}
	if err := lostComputer(ctx, ui, f.dest, f.computer(t, "example-server")); err != nil {
		t.Fatalf("err = %v, want esc back to Computers", err)
	}
	if !anyContains(ui.said(Warning), "made a new Secret Key and saved the new backend credentials. The new kit wasn't shown") ||
		!anyContains(ui.said(Plain), "then Show Emergency Kit.") {
		t.Errorf("stopped at the kit: said %+v", ui.Said)
	}

	// Quitting partway still leaves the lines to print after exit.
	f = newAccountFixture(t, filepath.Join(t.TempDir(), "repo"), nil)
	ui = &fakeUI{t: t, Confirms: []any{true}, Forms: []any{Values{"master": exampleMaster}}}
	if err := lostComputer(ctx, ui, f.dest, f.computer(t, "example-desktop")); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want the quit passed on", err)
	}
	if !anyContains(ui.said(Plain), "then Show Emergency Kit.") || len(requestsOf[Dashboard](ui)) != 0 {
		t.Errorf("quit at the kit: said %+v", ui.Said)
	}
}

// Replace backend credentials, from Emergency Kit and password: three wrong
// master passwords change nothing; the right one shows the new kit.
func TestReplaceBackendCredentials(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t, filepath.Join(t.TempDir(), "repo"), nil)
	const replace = 3 // Replace backend credentials, in the menu

	ui := &fakeUI{t: t, Picks: []any{replace}, Forms: wrongTries()}
	if _, err := kitAndPassword(ctx, ui, Deps{Machine: f.laptop}, f.dest); err != ErrCancelled {
		t.Fatalf("err = %v, want the menu again, then the script's end", err)
	}
	if got := ui.said(Error); !slices.Equal(got, []string{"Three wrong tries; nothing was changed."}) || len(requestsOf[KitScreen](ui)) != 0 {
		t.Errorf("wrong master passwords: said %+v", ui.Said)
	}
	if menus := requestsOf[Picker](ui); len(menus) != 2 || menus[1].Cursor != replace {
		t.Errorf("menus = %+v, want the menu again with the cursor kept", menus)
	}

	ui = &fakeUI{t: t, Picks: []any{replace}, Forms: []any{Values{"master": exampleMaster}}, Kits: []error{nil}}
	if _, err := kitAndPassword(ctx, ui, Deps{Machine: f.laptop}, f.dest); err != ErrCancelled {
		t.Fatalf("err = %v", err)
	}
	if !slices.Equal(ui.said(OK), []string{"✓ This computer opens example-backups with the new credentials."}) {
		t.Errorf("said %+v", ui.Said)
	}
	kits := requestsOf[KitScreen](ui)
	if len(kits) != 1 || !kits[0].New || !strings.Contains(kits[0].Kit, "Secret Key:") || kits[0].Page.Title != "example-backups › Emergency Kit and password › Replace backend credentials" {
		t.Errorf("kits = %+v", kits)
	}
}

// New B2 credentials that don't open the destination are asked for again,
// with the reason under the fields, then the old ones are revoked.
func TestNewCredentialsAreAskedForAgainUntilTheyOpenIt(t *testing.T) {
	ctx := context.Background()
	creds := map[string]string{"B2_ACCOUNT_ID": "000000000000000000000001", "B2_ACCOUNT_KEY": "K000xxxxxxxxxxxxxxxxxxxxxxxxxxx"}
	f := newAccountFixture(t, "b2:example-bucket:backups", creds)
	a, err := f.dest.Unlock(ctx, exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	ui := &fakeUI{t: t, Notices: []error{nil}, Forms: []any{
		Values{"B2_ACCOUNT_ID": "000000000000000000000002", "B2_ACCOUNT_KEY": "wrong"},
		Values{"B2_ACCOUNT_ID": "000000000000000000000002", "B2_ACCOUNT_KEY": "K000yyyyyyyyyyyyyyyyyyyyyyyyyyy"},
	}}
	page := func(step string) Page { return Page{Title: "example", Subtitle: step} }
	if err := replaceCredentials(ctx, ui, a, f.dest, page, nil, "Cancel"); err != nil {
		t.Fatalf("%v", err)
	}
	forms := requestsOf[Form](ui)
	if len(forms) != 2 || forms[0].Fields[0].Kind != Text || forms[0].Fields[1].Kind != Secret ||
		!strings.Contains(forms[0].Intro, "Click Create New Key.") {
		t.Fatalf("forms = %+v, want the B2 steps, the keyID echoed and the key masked", forms)
	}
	if e := forms[1].Fields[0].Error; !strings.HasPrefix(e, "Those didn't open example-backups (") || !strings.HasSuffix(e, "nothing was saved. Enter them again.") {
		t.Errorf("second try's error = %q", e)
	}
	n := requestsOf[Notice](ui)
	if len(n) != 1 || n[0].Heading != "Now revoke the old credentials, which the lost computer still has:" || len(n[0].Steps) != 2 {
		t.Errorf("revoke = %+v", n)
	}
	saved, err := f.laptop.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	if kit := mustUnlock(t, saved).Kit(); !strings.Contains(kit, "K000yyyy") {
		t.Errorf("the new credentials weren't saved:\n%s", kit)
	}
}

func mustUnlock(t *testing.T, d *backup.Destination) *backup.Account {
	t.Helper()
	a, err := d.Unlock(context.Background(), exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// Remove this computer asks first, warns that a lost computer needs more,
// and removes the key behind the master password.
func TestRemoveComputer(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t, filepath.Join(t.TempDir(), "repo"), nil)
	desktop := f.computer(t, "example-desktop")

	ui := &fakeUI{t: t, Confirms: []any{false}}
	if err := removeComputer(ctx, ui, f.dest, desktop); !errors.Is(err, errDeclined) || len(requestsOf[Form](ui)) != 0 {
		t.Errorf("declined: err = %v, requests %+v", err, ui.Requests)
	}
	c := requestsOf[Confirm](ui)[0]
	if c.Question != "Remove example-desktop's key?" || !c.Destructive || !strings.Contains(c.Consequence, "example-desktop can no longer open example-backups.") ||
		!strings.Contains(c.Warning, "choose I lost this computer instead") {
		t.Errorf("confirm = %+v", c)
	}

	ui = &fakeUI{t: t, Confirms: []any{true}, Forms: []any{Values{"master": exampleMaster}}}
	if err := removeComputer(ctx, ui, f.dest, desktop); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ui.said(OK), []string{"✓ Removed example-desktop's key: it can no longer open example-backups."}) {
		t.Errorf("said %+v", ui.Said)
	}
	if cs, _ := f.dest.Computers(ctx); len(cs) != 2 {
		t.Errorf("computers after = %+v", cs)
	}
}

// Change master password: the current one, then the new one twice; a short
// or mismatched one is asked again.
func TestChangeMasterPassword(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t, filepath.Join(t.TempDir(), "repo"), nil)
	const newMaster = "another example password"
	ui := &fakeUI{t: t, Confirms: []any{true}, Forms: []any{
		Values{"master": exampleMaster},
		Values{"master": newMaster, "again": "something else entirely"},
		Values{"master": newMaster, "again": newMaster},
	}}
	if err := changeMasterPassword(ctx, ui, f.dest); err != nil {
		t.Fatal(err)
	}
	forms := requestsOf[Form](ui)
	if len(forms) != 3 || forms[2].Fields[1].Error != "Those don't match; try again." || forms[1].Fields[0].Validate("short") != "Use at least 10 characters." {
		t.Errorf("forms = %+v", forms)
	}
	if !slices.Equal(ui.said(OK), []string{"✓ The master password is changed. The Emergency Kit and every computer's key keep working."}) {
		t.Errorf("said %+v", ui.Said)
	}
	if _, err := f.dest.Unlock(ctx, newMaster); err != nil {
		t.Errorf("the new master password doesn't unlock: %v", err)
	}
}

// Regenerate Secret Key shows the new kit once, and where to paste it.
func TestRegenerateSecretKey(t *testing.T) {
	ctx := context.Background()
	f := newAccountFixture(t, filepath.Join(t.TempDir(), "repo"), nil)
	ui := &fakeUI{t: t, Confirms: []any{true}, Forms: []any{Values{"master": exampleMaster}}, Kits: []error{nil}}
	if err := regenerateSecretKey(ctx, ui, f.dest); err != nil {
		t.Fatal(err)
	}
	kits := requestsOf[KitScreen](ui)
	if len(kits) != 1 || !kits[0].New || secretKeyOf(t, kits[0].Kit) == secretKeyOf(t, f.kit) || kits[0].Lines[0].Text != "The old Emergency Kit stops working." {
		t.Errorf("kits = %+v", kits)
	}
	if !anyContains(ui.said(Info), "Paste an updated Emergency Kit") {
		t.Errorf("said %+v", ui.Said)
	}
}

// quitsAt is the fake UI with ctdev quitting, as SIGTERM or a closed
// terminal does, just as the Run labelled label starts.
type quitsAt struct {
	*fakeUI
	label  string
	cancel context.CancelFunc
}

func (q quitsAt) Run(ctx context.Context, r Running, fn func(context.Context, func(Progress)) error) error {
	if r.Label == q.label {
		q.cancel()
	}
	return q.fakeUI.Run(ctx, r, fn)
}

// Quitting while Regenerate Secret Key runs still makes the new key, and
// says, after exit, where to see the kit, never the kit itself.
func TestQuittingWhileRegeneratingSaysWhereTheNewKitIs(t *testing.T) {
	f := newAccountFixture(t, filepath.Join(t.TempDir(), "repo"), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ui := &fakeUI{t: t, Picks: []any{2}, Confirms: []any{true}, Forms: []any{Values{"master": exampleMaster}}}
	q := quitsAt{fakeUI: ui, label: "Making a new Secret Key (this takes a few seconds)…", cancel: cancel}
	if _, err := kitAndPassword(ctx, q, Deps{Machine: f.laptop}, f.dest); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want the quit passed on", err)
	}
	a, err := f.dest.Unlock(context.Background(), exampleMaster)
	if err != nil || a.Kit() == f.kit {
		t.Fatalf("the Secret Key wasn't regenerated: %v", err)
	}
	if got := ui.said(Warning); !slices.Equal(got, []string{newKitNotShown}) {
		t.Errorf("said %+v", ui.Said)
	}
	for _, l := range ui.Said {
		if strings.Contains(l.Text, "Secret Key:") {
			t.Errorf("said a secret: %q", l.Text)
		}
	}
}

// Quitting while a Recovery Code reset runs still finishes it, and says
// that the new kit and code weren't shown, never the kit or the code.
func TestQuittingWhileResettingSaysTheNewCodeWasntShown(t *testing.T) {
	laptop, setup := createdKit(t)
	desktop := another(t, laptop, "example-desktop")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const newMaster = "new example password"
	ui := &fakeUI{t: t, Picks: []any{1}, Forms: []any{Values{"kit": setup.Kit}, Values{"master": "wrong example password"},
		Values{"code": setup.RecoveryCode, "master": newMaster, "again": newMaster, "newcode": "yes"}}}
	q := quitsAt{fakeUI: ui, label: "Resetting…", cancel: cancel}
	if _, err := connectDestination(ctx, q, desktop); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want the quit passed on", err)
	}
	dest, err := desktop.Machine.Destination("example-backups")
	if err != nil {
		t.Fatalf("the reset didn't save the destination: %v", err)
	}
	if _, err := dest.Unlock(context.Background(), newMaster); err != nil {
		t.Fatalf("the new master password doesn't open it: %v", err)
	}
	want := []string{newKitNotShown, "A new Recovery Code was made too, and it can't be shown again: the old one no longer works."}
	if got := ui.said(Warning); !slices.Equal(got, want) {
		t.Errorf("warnings = %q", got)
	}
	for _, l := range ui.Said {
		if strings.Contains(l.Text, "Secret Key:") || strings.Contains(l.Text, setup.RecoveryCode) {
			t.Errorf("said a secret: %q", l.Text)
		}
	}
}

// Quitting while I lost this computer makes the new Secret Key still makes
// it, and the lines printed after exit say so and what's left.
func TestQuittingWhileTheLostComputerFlowRegeneratesSaysSo(t *testing.T) {
	f := newAccountFixture(t, filepath.Join(t.TempDir(), "repo"), nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ui := &fakeUI{t: t, Confirms: []any{true}, Forms: []any{Values{"master": exampleMaster}}}
	q := quitsAt{fakeUI: ui, label: "2. Making a new Secret Key…", cancel: cancel}
	if err := lostComputer(ctx, q, f.dest, f.computer(t, "example-desktop")); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v, want the quit passed on", err)
	}
	if a, err := f.dest.Unlock(context.Background(), exampleMaster); err != nil || a.Kit() == f.kit {
		t.Fatalf("the Secret Key wasn't regenerated: %v", err)
	}
	if !anyContains(ui.said(Warning), "made a new Secret Key, saved on this computer") ||
		!anyContains(ui.said(Plain), "then Replace backend credentials, which also shows the new kit.") {
		t.Errorf("said %+v", ui.Said)
	}
}

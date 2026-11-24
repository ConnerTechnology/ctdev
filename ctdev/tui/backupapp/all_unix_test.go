//go:build unix

package backupapp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// oneFolder is a snapshots.json with one backup of ~/Projects/website from
// the laptop.
const oneFolder = `[{"id":"0123456789abcdef","short_id":"01234567","time":"2026-11-22T21:12:00Z","hostname":"example-laptop",` +
	`"tags":["ctdev-folder:11111111-1111-4111-8111-111111111111","ctdev-path:~/Projects/website"],` +
	`"summary":{"total_files_processed":1,"total_bytes_processed":15,"data_added":15}}]`

// allFixture is the laptop set up on example-backups, a local destination
// whose stand-in repository holds snapshots, and the laptop's Deps.
func allFixture(t *testing.T, snapshots string) (accountFixture, Deps, string) {
	t.Helper()
	address := filepath.Join(t.TempDir(), "repo")
	f := newAccountFixture(t, address, nil)
	repo := standinRepo(f.root, address)
	if err := os.WriteFile(filepath.Join(repo, "snapshots.json"), []byte(snapshots), 0o644); err != nil {
		t.Fatal(err)
	}
	return f, Deps{Machine: f.laptop, Cwd: t.TempDir()}, repo
}

func standinCalls(t *testing.T, root, prefix string) int {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(root, "restic.log"))
	n := 0
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, prefix) {
			n++
		}
	}
	return n
}

// A --all session measures the space stored once, not on every return to
// its dashboard (L1, L2).
func TestAllMeasuresStoredOnce(t *testing.T) {
	f, d, _ := allFixture(t, oneFolder)
	ui := &fakeUI{t: t, Dashboards: []any{"l", "l", "q"}, Picks: []any{ErrBack, ErrBack}}
	if err := allFlow(context.Background(), ui, d); err != nil {
		t.Fatalf("%v; said %+v", err, ui.Said)
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 3 {
		t.Fatalf("the dashboard came up %d times, want 3", len(dash))
	}
	if n := standinCalls(t, f.root, "stats "); n != 1 {
		t.Errorf("restic stats ran %d times in the session, want once", n)
	}
	if n := standinCalls(t, f.root, "snapshots "); n != 3 {
		t.Errorf("restic snapshots ran %d times, want once per return to the dashboard", n)
	}
	body := dash[0].Page.Body
	for _, want := range []string{"COMPUTERS", "3", "with a key to example-backups", "from 1 computer", "2.0 kB", "every backup of every folder"} {
		if !strings.Contains(body, want) {
			t.Errorf("the dashboard is missing %q:\n%s", want, body)
		}
	}
	if dash[1].Focus != "l" {
		t.Errorf("focus after List backups = %q, want it kept", dash[1].Focus)
	}
	if p := requestsOf[Picker](ui); p[0].Page.Title != "example-backups › Folders" || p[0].Items[0].Group != "example-laptop (this computer)" {
		t.Errorf("folders = %+v", p[0])
	}
}

// With no folders, nothing is measured and List backups is dimmed (L3).
func TestAllWithNoFolders(t *testing.T) {
	f, d, _ := allFixture(t, "[]")
	ui := &fakeUI{t: t, Dashboards: []any{"q"}}
	if err := allFlow(context.Background(), ui, d); err != nil {
		t.Fatal(err)
	}
	if n := standinCalls(t, f.root, "stats "); n != 0 {
		t.Errorf("restic stats ran %d times with nothing stored", n)
	}
	dash := requestsOf[Dashboard](ui)[0]
	if !dash.Actions[0].Dimmed || !strings.Contains(dash.Page.Body, "nothing backed up here yet") {
		t.Errorf("dashboard = %+v", dash)
	}
}

// Picking a folder opens its backups with the destination, and returning
// reads the destination again.
func TestAllOpensAFolder(t *testing.T) {
	_, d, _ := allFixture(t, oneFolder)
	ui := &fakeUI{t: t, Dashboards: []any{"l"}, Picks: []any{0}}
	if err := allFlow(context.Background(), ui, d); err != ErrCancelled {
		t.Fatalf("err = %v", err)
	}
	// browseBackups is a stub until its slice lands, so its error comes back
	// to the dashboard.
	if !anyContains(ui.said(Error), "browsing backups isn't built yet") {
		t.Errorf("said %+v, want List backups to reach browseBackups", ui.Said)
	}
}

// Computers lists every computer, this one marked; picking one shows its
// actions, dimmed for this computer (M1 to M3).
func TestAllComputers(t *testing.T) {
	_, d, _ := allFixture(t, oneFolder)
	ui := &fakeUI{t: t, Dashboards: []any{"c", "esc", "esc", "q"}, Picks: []any{1, 0, ErrBack}}
	if err := allFlow(context.Background(), ui, d); err != nil {
		t.Fatal(err)
	}
	p := requestsOf[Picker](ui)
	var hosts []string
	for _, it := range p[0].Items {
		hosts = append(hosts, it.Cells[0]+"|"+it.Cells[2])
	}
	if got := strings.Join(hosts, ","); !strings.Contains(got, "example-desktop|,") || !strings.Contains(got, "example-laptop|") ||
		strings.Count(got, "this computer") != 1 || len(hosts) != 3 {
		t.Errorf("computers = %q", got)
	}
	dash := requestsOf[Dashboard](ui)
	// The dashboard, then example-laptop (this one), then example-desktop.
	this, other := dash[1], dash[2]
	if !this.Actions[0].Dimmed || !this.Actions[1].Dimmed || !strings.Contains(this.Page.Body, "remove it from another computer") {
		t.Errorf("this computer = %+v", this)
	}
	if other.Actions[0].Dimmed || other.Actions[1].Dimmed || other.Page.Title != "example-backups › Computers › example-desktop" {
		t.Errorf("another computer = %+v", other)
	}
}

// A computer with no destination is offered Connect or Create, and comes
// back to the offer when either doesn't finish (K1 to K3).
func TestAllWithNoDestination(t *testing.T) {
	bin, _ := keyStandin(t)
	d := Deps{Machine: machineOn(t, bin, "example-laptop"), Cwd: t.TempDir()}
	ui := &fakeUI{t: t, Picks: []any{1, 0}}
	if err := allFlow(context.Background(), ui, d); err != ErrCancelled {
		t.Fatalf("err = %v", err)
	}
	p := requestsOf[Picker](ui)
	if len(p) != 3 || p[0].Page.Badge != (Badge{Attention, "NOT SET UP"}) || !p[0].Quit || p[1].Cursor != 1 {
		t.Errorf("offers = %+v", p)
	}
	// Both are stubs until the Configure slice lands.
	if got := ui.said(Error); len(got) != 2 || !strings.Contains(got[0], "creating a destination") || !strings.Contains(got[1], "connecting a destination") {
		t.Errorf("said %+v, want Create, then Connect, reached", ui.Said)
	}
}

// With several destinations, --all asks which (K5).
func TestAllAsksWhichDestination(t *testing.T) {
	f, d, _ := allFixture(t, "[]")
	if _, err := f.laptop.CreateDestination(context.Background(),
		backup.NewDestination{Name: "example-usb", Address: filepath.Join(t.TempDir(), "usb")}, exampleMaster); err != nil {
		t.Fatal(err)
	}
	ui := &fakeUI{t: t, Picks: []any{1}, Dashboards: []any{"q"}}
	if err := allFlow(context.Background(), ui, d); err != nil {
		t.Fatal(err)
	}
	p := requestsOf[Picker](ui)[0]
	if len(p.Items) != 2 || !strings.Contains(p.Items[1].Cells[0], "example-usb") || !p.Quit {
		t.Errorf("which = %+v", p)
	}
	if got := requestsOf[Dashboard](ui)[0].Page.Title; got != "example-usb" {
		t.Errorf("opened %q, want the one picked", got)
	}
}

// A destination that can't be read offers to paste the new kit; q quits
// with the read error (K6).
func TestAllOffersThePasteWhenItCantRead(t *testing.T) {
	_, d, repo := allFixture(t, "[]")
	if err := os.WriteFile(filepath.Join(repo, "fail-list"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	ui := &fakeUI{t: t, Dashboards: []any{"p", "q"}}
	err := allFlow(context.Background(), ui, d)
	if err == nil || !strings.Contains(err.Error(), "example failure") {
		t.Fatalf("err = %v, want the read error", err)
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 2 || dash[0].Page.Badge != (Badge{Danger, "CAN'T READ"}) || dash[0].Actions[0].Key != "p" ||
		!strings.HasPrefix(dash[0].Message[0].Text, "Can't read example-backups: ") {
		t.Errorf("offer = %+v", dash)
	}
	// pasteUpdatedKit is a stub until the Configure slice lands.
	if !anyContains(ui.said(Error), "pasting a kit") {
		t.Errorf("said %+v, want p to reach pasteUpdatedKit", ui.Said)
	}
}

// A key that no longer opens the destination ends the session: a new kit
// can't help (K7).
func TestAllEndsWhenTheKeyNoLongerOpensIt(t *testing.T) {
	f, d, _ := allFixture(t, "[]")
	this := f.computer(t, "example-laptop")
	desktop, err := f.desktop.Destination("example-backups")
	if err != nil {
		t.Fatal(err)
	}
	// The laptop's key is removed from the desktop.
	da, err := desktop.Unlock(context.Background(), exampleMaster)
	if err != nil {
		t.Fatal(err)
	}
	this.ThisComputer = false
	if err := da.RemoveComputer(context.Background(), this); err != nil {
		t.Fatal(err)
	}
	ui := &fakeUI{t: t, Dashboards: []any{"q"}}
	err = allFlow(context.Background(), ui, d)
	if err == nil || !strings.Contains(err.Error(), "this computer's key no longer opens example-backups") {
		t.Fatalf("err = %v", err)
	}
	dash := requestsOf[Dashboard](ui)
	if len(dash) != 1 || dash[0].Page.Badge != (Badge{Danger, "NO ACCESS"}) || len(dash[0].Actions) != 1 {
		t.Errorf("screen = %+v", dash)
	}
	if errors.Is(err, backup.ErrWrongPassword) {
		t.Errorf("err = %v, want it said in words", err)
	}
}

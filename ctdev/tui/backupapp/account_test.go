package backupapp

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/charmbracelet/x/ansi"
)

// exampleComputers is backup.Destination.Computers on example-desktop: the
// account and Recovery Code keys are never in it.
func exampleComputers() []backup.Computer {
	return []backup.Computer{
		{ID: "aaaa1111", Hostname: "example-desktop", Added: time.Date(2026, 10, 2, 9, 0, 0, 0, time.Local), ThisComputer: true},
		{ID: "bbbb2222", Hostname: "example-laptop", Added: time.Date(2026, 9, 14, 18, 30, 0, 0, time.Local)},
	}
}

// Computers lists each computer by hostname and date added, this one
// marked, and never a key's ID (M1).
func TestComputersListsEachComputerWithThisOneMarked(t *testing.T) {
	p := computersPicker(Page{}, exampleComputers(), 0)
	view := ansi.Strip(strings.Join(newPicker(p).view(frameWidth, 0).content, "\n"))
	order := []string{"COMPUTER", "ADDED", "example-desktop", "Oct 2, 2026", "this computer", "example-laptop", "Sep 14, 2026"}
	at := 0
	for _, want := range order {
		i := strings.Index(view[at:], want)
		if i < 0 {
			t.Fatalf("want %q after position %d, in order %v:\n%s", want, at, order, view)
		}
		at += i + len(want)
	}
	if strings.Count(view, "this computer") != 1 || strings.Contains(view, "aaaa1111") {
		t.Errorf("one computer is this one, and no key's ID shows:\n%s", view)
	}
}

// Another computer offers Remove · I lost this computer · Back, with
// capital letters since both are destructive; this one can't be removed
// from itself, so both are dimmed and focus starts on Back (M2, M3).
func TestComputerActions(t *testing.T) {
	cs := exampleComputers()
	d := computerDashboard("example-backups", cs[1])
	var keys []string
	for _, a := range d.Actions {
		keys = append(keys, a.Key+" "+a.Label)
		if a.Dimmed {
			t.Errorf("%s is dimmed for another computer", a.Label)
		}
	}
	if got := strings.Join(keys, ","); got != "R Remove this computer,L I lost this computer,esc Back" {
		t.Errorf("actions = %s", got)
	}
	this := computerDashboard("example-backups", cs[0])
	if b := newDashboard(this); !this.Actions[0].Dimmed || !this.Actions[1].Dimmed || this.Actions[b.cursor].Key != "esc" {
		t.Errorf("this computer: actions %+v, focus %d", this.Actions, b.cursor)
	}
	if !strings.Contains(ansi.Strip(this.Page.Body), "This computer: remove it from another computer that's set up.") {
		t.Errorf("this computer's screen should say where to remove it from:\n%s", this.Page.Body)
	}
}

// The kit menu is split by whether an action asks for the master password
// (P1).
func TestKitMenu(t *testing.T) {
	var actions []kitAction
	for _, a := range []struct {
		label   string
		unlocks bool
	}{{"Show Emergency Kit", true}, {"Use a Recovery Code", false}} {
		actions = append(actions, kitAction{label: a.label, unlocks: a.unlocks})
	}
	p := kitMenu("example-backups", actions, 0)
	if p.Items[0].Group != "NEEDS THE MASTER PASSWORD" || p.Items[1].Group != "WITHOUT IT" {
		t.Errorf("groups = %+v", p.Items)
	}
	if p.Page.Title != "example-backups › Emergency Kit and password" {
		t.Errorf("title = %q", p.Page.Title)
	}
}

// Each backend has its own way to replace credentials: B2 and S3 ask for new
// ones, SFTP revokes the lost computer's SSH key, a local path has none.
func TestProviderSteps(t *testing.T) {
	for address, want := range map[string]struct {
		fields []string
		revoke bool
		note   string
	}{
		"b2:example-bucket:backups":                        {[]string{"B2_ACCOUNT_ID", "B2_ACCOUNT_KEY"}, true, ""},
		"s3:s3.us-east-1.amazonaws.com/example-bucket":     {[]string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}, true, ""},
		"s3:https://s3.example.invalid/example-bucket":     {[]string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}, true, "documentation has the exact clicks"},
		"sftp:example@backup.example.invalid:/srv/backups": {nil, true, "over SFTP"},
		"/media/example/usb/backups":                       {nil, false, "no credentials to revoke"},
	} {
		s := providerSteps(address)
		var fields []string
		for _, f := range s.fields {
			fields = append(fields, f.name)
		}
		if strings.Join(fields, ",") != strings.Join(want.fields, ",") || (s.revoke != nil) != want.revoke || !strings.Contains(s.note, want.note) {
			t.Errorf("%s: %+v", address, s)
		}
		if len(s.fields) > 0 && (s.fields[0].kind != Text || s.fields[1].kind != Secret) {
			t.Errorf("%s: an ID echoes and a key is masked: %+v", address, s.fields)
		}
	}
}

// The new kit's lines fit the frame, and keep where to paste it whole.
func TestNewKitLines(t *testing.T) {
	var text []string
	for _, l := range newKitLines()[1:] {
		if len(l.Text) > frameWidth-4 {
			t.Errorf("%q is wider than the frame", l.Text)
		}
		text = append(text, l.Text)
	}
	if strings.Join(text, " ") != pasteOnOtherComputers {
		t.Errorf("lines = %q", text)
	}
}

// The lost-computer and remove confirmations fit the frame, and only y goes
// ahead (M4, N1).
func TestAccountConfirmationsFitTheFrame(t *testing.T) {
	c := exampleComputers()[1]
	dest := exampleDest()
	for name, flow := range map[string]func(*fakeUI) error{
		"remove": func(ui *fakeUI) error { return removeComputer(context.Background(), ui, dest, c) },
		"lost":   func(ui *fakeUI) error { return lostComputer(context.Background(), ui, dest, c) },
	} {
		ui := &fakeUI{t: t, Confirms: []any{false}}
		if err := flow(ui); !errors.Is(err, errDeclined) || len(ui.Requests) != 1 {
			t.Errorf("%s: declined: %v, requests %+v", name, err, ui.Requests)
			continue
		}
		cf := requestsOf[Confirm](ui)[0]
		if !cf.Destructive {
			t.Errorf("%s: only y goes ahead", name)
		}
		for _, l := range newConfirm(cf).view(frameWidth, 0).content {
			if w := ansi.StringWidth(l); w > frameWidth {
				t.Errorf("%s: %q is %d wide", name, ansi.Strip(l), w)
			}
		}
	}
}

// The new credentials' values line up past their longest label (N3).
func TestNewCredentialsLineUp(t *testing.T) {
	ui := &fakeUI{t: t}
	page := func(string) Page { return Page{Title: "example"} }
	if err := replaceCredentials(context.Background(), ui, nil, exampleDest(), page, nil, "Cancel"); !errors.Is(err, ErrCancelled) {
		t.Fatalf("err = %v", err)
	}
	for _, l := range newForm(requestsOf[Form](ui)[0]).view(frameWidth, 0).content {
		// "NEW B2 APPLICATIONKEY" is 21 wide: the first value starts 3 after it.
		if l := []rune(ansi.Strip(l)); strings.Contains(string(l), "NEW B2 KEYID") && slices.Index(l, '█') != 2+21+3 {
			t.Errorf("the keyID's value starts at %d:\n%s", slices.Index(l, '█'), string(l))
		}
	}
}

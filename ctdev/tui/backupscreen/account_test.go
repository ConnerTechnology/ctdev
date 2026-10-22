package backupscreen

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// exampleComputers is backup.Destination.Computers on example-desktop: the
// account and Recovery Code keys are never in it.
func exampleComputers() []backup.Computer {
	return []backup.Computer{
		{ID: "aaaa1111", Hostname: "example-desktop", Added: time.Date(2026, 10, 2, 9, 0, 0, 0, time.Local), ThisComputer: true},
		{ID: "bbbb2222", Hostname: "example-laptop", Added: time.Date(2026, 9, 14, 18, 30, 0, 0, time.Local)},
	}
}

func TestComputersListsEachComputerWithThisOneMarked(t *testing.T) {
	c := NewComputers("example-backups", exampleComputers())
	view := plain(c.View().Content)
	if first := strings.Split(view, "\n")[0]; first != "Computers with access to example-backups" {
		t.Errorf("title = %q", first)
	}
	order := []string{"example-desktop", "added Oct 2, 2026", "(this computer)", "example-laptop", "added Sep 14, 2026", "Back"}
	at := 0
	for _, want := range order {
		i := strings.Index(view[at:], want)
		if i < 0 {
			t.Fatalf("want %q after position %d, in order %v:\n%s", want, at, order, view)
		}
		at += i + len(want)
	}
	if strings.Count(view, "(this computer)") != 1 {
		t.Errorf("one computer is this one:\n%s", view)
	}
	for _, never := range []string{"ctdev-account", "ctdev-recovery", "aaaa1111"} {
		if strings.Contains(view, never) {
			t.Errorf("the list shows %q:\n%s", never, view)
		}
	}
}

func TestComputersPickOneOrGoBack(t *testing.T) {
	c := NewComputers("example-backups", exampleComputers())
	send(c, key(tea.KeyDown), key(tea.KeyEnter))
	if got, ok := c.Chosen(); !ok || got.Hostname != "example-laptop" {
		t.Errorf("chosen = %q, %v; want example-laptop", got.Hostname, ok)
	}
	for _, keys := range [][]tea.KeyPressMsg{
		{key(tea.KeyEscape)},
		{{Code: 'q', Text: "q"}},
		{key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyEnter)}, // Back
	} {
		c = NewComputers("example-backups", exampleComputers())
		send(c, keys...)
		if _, ok := c.Chosen(); ok || !c.Done() {
			t.Errorf("%v: should go back without choosing", keys)
		}
	}
}

// Another computer offers Remove · I lost this computer · Back; this one
// can't be removed from itself, so both are dimmed.
func TestComputerActions(t *testing.T) {
	cs := exampleComputers()
	m := NewComputerActions(cs[1])
	labels, dimmed := items(m)
	if got, want := strings.Join(labels, ","), "Remove this computer,I lost this computer,Back"; got != want {
		t.Errorf("items = %s, want %s", got, want)
	}
	if len(dimmed) != 0 {
		t.Errorf("dimmed = %v, want none", dimmed)
	}
	if got := firstLine(m); got != "example-laptop · added Sep 14, 2026" {
		t.Errorf("header = %q", got)
	}
	press(m, key(tea.KeyDown), key(tea.KeyEnter))
	if m.Chosen() != LostComputer {
		t.Errorf("chosen = %v, want I lost this computer", m.Chosen())
	}

	m = NewComputerActions(cs[0])
	if _, dimmed := items(m); strings.Join(dimmed, ",") != "Remove this computer,I lost this computer" {
		t.Errorf("this computer: dimmed = %v", dimmed)
	}
	if !strings.Contains(plain(m.View().Content), "remove it from another computer") {
		t.Errorf("this computer's screen should say where to remove it from:\n%s", plain(m.View().Content))
	}
	press(m, key(tea.KeyEnter))
	if m.Chosen() != GoBack {
		t.Errorf("this computer: chosen = %v, want Back", m.Chosen())
	}
	// esc goes back, and says so.
	m = NewComputerActions(cs[1])
	if !strings.Contains(plain(m.View().Content), "esc back") {
		t.Error("the help line should say esc goes back")
	}
	press(m, key(tea.KeyEscape))
	if m.Chosen() != GoBack {
		t.Errorf("esc: chosen = %v, want Back", m.Chosen())
	}
}

func TestKitMenu(t *testing.T) {
	labels, dimmed := items(NewKitMenu("example-backups"))
	want := "Show Emergency Kit,Change master password,Regenerate Secret Key,Replace backend credentials,Use a Recovery Code,Paste an updated Emergency Kit,Back"
	if got := strings.Join(labels, ","); got != want {
		t.Errorf("items = %s\nwant %s", got, want)
	}
	if len(dimmed) != 0 {
		t.Errorf("dimmed = %v", dimmed)
	}
	for i, action := range []Action{ShowKit, ChangePassword, RegenerateKey, ReplaceCredentials, UseRecoveryCode, PasteKit, GoBack} {
		m := NewKitMenu("example-backups")
		for range i {
			press(m, key(tea.KeyDown))
		}
		press(m, key(tea.KeyEnter))
		if m.Chosen() != action {
			t.Errorf("item %d: chosen = %v, want %v", i, m.Chosen(), action)
		}
	}
}

// Each action's confirmation says what it does, and only y goes ahead.
func TestAccountConfirmations(t *testing.T) {
	for name, tc := range map[string]struct {
		c    *Confirm
		says []string
	}{
		"remove":     {ConfirmRemoveComputer("example-laptop", "example-backups"), []string{"Remove example-laptop's key", "can no longer open example-backups"}},
		"lost":       {ConfirmLostComputer("example-laptop", "example-backups"), []string{"example-laptop", "new Secret Key", "new backend credentials", "every other computer"}},
		"password":   {ConfirmChangePassword("example-backups"), []string{"Change the master password", "Emergency Kit stays the same"}},
		"regenerate": {ConfirmRegenerate("example-backups"), []string{"new Secret Key", "old Emergency Kit stops working", "every other computer"}},
		"recovery":   {ConfirmNewRecoveryCode(), []string{"new Recovery Code", "old one stops working"}},
	} {
		view := plain(tc.c.View().Content)
		for _, want := range append(tc.says, "[y/N]") {
			if !strings.Contains(view, want) {
				t.Errorf("%s: %q is missing %q", name, view, want)
			}
		}
		for k, yes := range map[string]bool{"y": true, "n": false, "enter": false} {
			c := *tc.c
			msg := tea.KeyPressMsg{Code: rune(k[0]), Text: k}
			if k == "enter" {
				msg = key(tea.KeyEnter)
			}
			send(&c, msg)
			if !c.finished() || c.Confirmed() != yes {
				t.Errorf("%s, %s: confirmed %v, want %v", name, k, c.Confirmed(), yes)
			}
		}
	}
}

package backupscreen

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// exampleBackups are two backups of ~/Projects/website, oldest first, the
// way backup.Destination.Snapshots returns them.
func exampleBackups() []backup.Snapshot {
	return []backup.Snapshot{
		{ID: "aaaaaaaa11111111", ShortID: "aaaaaaaa", Time: time.Date(2026, 10, 20, 21, 10, 0, 0, time.Local),
			Host: "example-laptop", Files: 4800, Size: 1_150_000_000, Added: 1_140_000_000, FilesNew: 4800, Duration: 95 * time.Second},
		{ID: "bbbbbbbb22222222", ShortID: "bbbbbbbb", Time: time.Date(2026, 10, 21, 21, 12, 0, 0, time.Local),
			Host: "example-desktop", Files: 4812, Size: 1_200_000_000, Added: 12_000_000, FilesNew: 12, FilesChanged: 3, Duration: 4 * time.Second},
	}
}

func typeText(s string) []tea.KeyPressMsg {
	var keys []tea.KeyPressMsg
	for _, r := range s {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

func send(m tea.Model, keys ...tea.KeyPressMsg) {
	for _, k := range keys {
		m.Update(k)
	}
}

func TestListShowsTheHeaderAndBackupsNewestFirst(t *testing.T) {
	l := NewList("~/Projects/website", exampleBackups(), 1_600_000_000)
	view := plain(l.View().Content)
	lines := strings.Split(view, "\n")
	if lines[0] != "Backups of ~/Projects/website · 2 backups · 1.6 GB stored" {
		t.Errorf("header = %q", lines[0])
	}
	newest := strings.Index(view, "Oct 21, 9:12 PM")
	oldest := strings.Index(view, "Oct 20, 9:10 PM")
	if newest < 0 || oldest < 0 || newest > oldest {
		t.Fatalf("want both backups, newest first:\n%s", view)
	}
	for _, want := range []string{"1.2 GB", "12.0 MB new", "4,812 files", "latest", "1.1 GB new", "4,800 files"} {
		if !strings.Contains(view, want) {
			t.Errorf("list is missing %q:\n%s", want, view)
		}
	}
	if strings.Count(view, "latest") != 1 {
		t.Errorf("only the newest is latest:\n%s", view)
	}
}

func TestListPicksABackupOrGoesBack(t *testing.T) {
	l := NewList("~/Projects/website", exampleBackups(), 1)
	send(l, key(tea.KeyDown), key(tea.KeyEnter))
	if got, ok := l.Chosen(); !ok || got.ShortID != "aaaaaaaa" {
		t.Errorf("chosen = %v, %v; want the older backup, second in the list", got.ShortID, ok)
	}

	l = NewList("~/Projects/website", exampleBackups(), 1)
	send(l, key(tea.KeyEscape))
	if _, ok := l.Chosen(); ok || !l.Done() {
		t.Error("esc should go back without choosing")
	}

	l = NewList("~/Projects/website", exampleBackups(), 1)
	send(l, key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyEnter))
	if _, ok := l.Chosen(); ok || !l.Done() {
		t.Error("Back, after the backups, should go back without choosing")
	}
}

func TestDetailShowsTheBackupAndItsMenu(t *testing.T) {
	snap := exampleBackups()[1]
	d := NewDetail("~/Projects/website", snap, true)
	view := plain(d.View().Content)
	for _, want := range []string{
		"Backup of ~/Projects/website · Oct 21, 9:12 PM · latest",
		"Size", "1.2 GB",
		"Files", "4,812 files",
		"New data", "12.0 MB",
		"Changes", "3 changed · 12 added",
		"Duration", "4s",
		"Computer", "example-desktop",
		"ID", "bbbbbbbb",
		"Download as zip", "Delete this backup", "Back",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("detail is missing %q:\n%s", want, view)
		}
	}

	for _, tc := range []struct {
		keys []tea.KeyPressMsg
		want DetailAction
	}{
		{[]tea.KeyPressMsg{key(tea.KeyEnter)}, Download},
		{[]tea.KeyPressMsg{key(tea.KeyDown), key(tea.KeyEnter)}, DeleteOne},
		{[]tea.KeyPressMsg{key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyEnter)}, Back},
		{[]tea.KeyPressMsg{key(tea.KeyEscape)}, Back},
	} {
		d := NewDetail("~/Projects/website", snap, true)
		send(d, tc.keys...)
		if d.Chosen() != tc.want {
			t.Errorf("chosen = %v, want %v", d.Chosen(), tc.want)
		}
	}
}

func TestConfirmIsNoUnlessYes(t *testing.T) {
	for name, tc := range map[string]struct {
		keys []tea.KeyPressMsg
		want bool
	}{
		"y":     {typeText("y"), true},
		"Y":     {typeText("Y"), true},
		"n":     {typeText("n"), false},
		"enter": {[]tea.KeyPressMsg{key(tea.KeyEnter)}, false},
		"esc":   {[]tea.KeyPressMsg{key(tea.KeyEscape)}, false},
	} {
		c := NewConfirm("Delete the backup from Oct 21, 9:12 PM?")
		if !strings.Contains(plain(c.View().Content), "Delete the backup from Oct 21, 9:12 PM? [y/N]") {
			t.Errorf("%s: view = %q", name, plain(c.View().Content))
		}
		send(c, tc.keys...)
		if !c.Done() || c.Confirmed() != tc.want {
			t.Errorf("%s: done %v, confirmed %v; want confirmed %v", name, c.Done(), c.Confirmed(), tc.want)
		}
	}
}

func TestDeleteAllAsksThenHasYouTypeTheFolderName(t *testing.T) {
	c := NewDeleteAll("~/Projects/website", "website", 14)
	if view := plain(c.View().Content); !strings.Contains(view, "Delete all 14 backups of ~/Projects/website? [y/N]") {
		t.Errorf("view = %q", view)
	}
	send(c, typeText("y")...)
	if c.Done() {
		t.Fatal("y alone must not delete")
	}
	if view := plain(c.View().Content); !strings.Contains(view, "Type website to confirm:") {
		t.Errorf("view = %q", view)
	}

	// A wrong name doesn't confirm; it says so and lets you try again.
	send(c, typeText("webs")...)
	send(c, key(tea.KeyEnter))
	if c.Done() {
		t.Fatal("a wrong name must not confirm")
	}
	if view := plain(c.View().Content); !strings.Contains(view, "That isn't website") {
		t.Errorf("view = %q", view)
	}

	send(c, typeText("website")...)
	send(c, key(tea.KeyEnter))
	if !c.Done() || !c.Confirmed() {
		t.Errorf("done %v, confirmed %v after typing the name", c.Done(), c.Confirmed())
	}
}

// Stop backing up's "also delete" has already asked [y/N], so it goes
// straight to typing the name.
func TestTypeNameAsksOnlyForTheName(t *testing.T) {
	c := NewTypeName("website")
	view := plain(c.View().Content)
	if strings.Contains(view, "[y/N]") || !strings.Contains(view, "Type website to confirm:") {
		t.Errorf("view = %q", view)
	}
	send(c, typeText("y")...)
	send(c, key(tea.KeyEnter))
	if c.Done() {
		t.Fatal("y isn't the folder's name")
	}
	send(c, typeText("website")...)
	send(c, key(tea.KeyEnter))
	if !c.Done() || !c.Confirmed() {
		t.Errorf("done %v, confirmed %v after typing the name", c.Done(), c.Confirmed())
	}

	c = NewTypeName("website")
	send(c, key(tea.KeyEscape))
	if !c.Done() || c.Confirmed() {
		t.Errorf("esc: done %v, confirmed %v; want cancelled", c.Done(), c.Confirmed())
	}
}

func TestDeleteAllCanBeTypedWithCorrectionsOrCancelled(t *testing.T) {
	c := NewDeleteAll("~/Projects/website", "website", 2)
	send(c, typeText("y")...)
	send(c, typeText("websitx")...)
	send(c, key(tea.KeyBackspace))
	send(c, typeText("e")...)
	send(c, key(tea.KeyEnter))
	if !c.Confirmed() {
		t.Error("backspace should correct the typed name")
	}

	for name, keys := range map[string][]tea.KeyPressMsg{
		"n":              typeText("n"),
		"enter":          {key(tea.KeyEnter)},
		"esc at name":    append(typeText("y"), key(tea.KeyEscape)),
		"ctrl+c at name": append(typeText("y"), tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}),
	} {
		c := NewDeleteAll("~/Projects/website", "website", 2)
		send(c, keys...)
		if !c.Done() || c.Confirmed() {
			t.Errorf("%s: done %v, confirmed %v; want cancelled", name, c.Done(), c.Confirmed())
		}
	}
}

package backupscreen

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
)

// exampleOverview is a destination two computers back up to, as
// backup.Destination.Overview returns it on example-desktop.
func exampleOverview() backup.Overview {
	return backup.Overview{
		Computers: 2, FolderCount: 3,
		Folders: []backup.FoundFolder{
			{Ref: backup.Ref{FolderID: "11111111-1111-4111-8111-111111111111"}, Host: "example-desktop", Path: "~/Documents/notes",
				Backups: 3, Latest: time.Date(2026, 10, 21, 21, 12, 0, 0, time.Local), Size: 52_000},
			{Ref: backup.Ref{Path: "~/Downloads/scans", Host: "example-laptop"}, Host: "example-laptop", Path: "~/Downloads/scans",
				Backups: 1, Latest: time.Date(2026, 10, 2, 8, 5, 0, 0, time.Local), Size: 3_400_000},
			{Ref: backup.Ref{FolderID: "22222222-2222-4222-8222-222222222222"}, Host: "example-laptop", Path: "~/Projects/website",
				Backups: 14, Latest: time.Date(2026, 10, 20, 21, 10, 0, 0, time.Local), Size: 1_200_000_000},
		},
	}
}

func TestAllHeader(t *testing.T) {
	m := NewAll("example-backups", "Backblaze B2", exampleOverview(), 1_600_000_000)
	if got, want := firstLine(m), "example-backups (Backblaze B2) · 2 computers · 3 folders · 1.6 GB stored"; got != want {
		t.Errorf("header = %q\nwant     %q", got, want)
	}
	m = NewAll("example-backups", "local path", backup.Overview{Computers: 1, FolderCount: 1}, 30)
	if got, want := firstLine(m), "example-backups (local path) · 1 computer · 1 folder · 30 B stored"; got != want {
		t.Errorf("header = %q\nwant     %q", got, want)
	}
}

// Computers and Emergency Kit and password are shown, dimmed, until they're
// built; List backups is dimmed only when there's nothing to list.
func TestAllMenu(t *testing.T) {
	for name, tc := range map[string]struct {
		o      backup.Overview
		dimmed string
	}{
		"with folders": {exampleOverview(), "Computers,Emergency Kit and password"},
		"empty":        {backup.Overview{Computers: 1}, "List backups,Computers,Emergency Kit and password"},
	} {
		labels, dimmed := items(NewAll("example-backups", "local path", tc.o, 0))
		if got, want := strings.Join(labels, ","), "List backups,Computers,Emergency Kit and password,Quit"; got != want {
			t.Errorf("%s: items = %s\nwant %s", name, got, want)
		}
		if got := strings.Join(dimmed, ","); got != tc.dimmed {
			t.Errorf("%s: dimmed = %q, want %q", name, got, tc.dimmed)
		}
	}

	m := NewAll("example-backups", "local path", exampleOverview(), 0)
	press(m, key(tea.KeyEnter))
	if m.Chosen() != ListBackups {
		t.Errorf("chosen = %v, want List backups", m.Chosen())
	}
	m = NewAll("example-backups", "local path", exampleOverview(), 0)
	press(m, key(tea.KeyDown), key(tea.KeyEnter))
	if m.Chosen() != Quit {
		t.Errorf("chosen = %v, want Quit: the dimmed items are skipped", m.Chosen())
	}
}

func TestFoldersAreGroupedByComputer(t *testing.T) {
	f := NewFolders("Folders in example-backups", exampleOverview().Folders, "example-desktop")
	view := plain(f.View().Content)
	lines := strings.Split(view, "\n")
	if lines[0] != "Folders in example-backups" {
		t.Errorf("title = %q", lines[0])
	}
	order := []string{
		"example-desktop (this computer)", "~/Documents/notes",
		"example-laptop", "~/Downloads/scans", "~/Projects/website", "Back",
	}
	at := 0
	for _, want := range order {
		i := strings.Index(view[at:], want)
		if i < 0 {
			t.Fatalf("want %q after position %d, in order %v:\n%s", want, at, order, view)
		}
		at += i + len(want)
	}
	for _, want := range []string{"Oct 21, 9:12 PM", "52.0 kB", "3 backups", "1 backup", "14 backups", "1.2 GB"} {
		if !strings.Contains(view, want) {
			t.Errorf("list is missing %q:\n%s", want, view)
		}
	}
	if strings.Count(view, "(this computer)") != 1 {
		t.Errorf("only example-desktop is this computer:\n%s", view)
	}
}

func TestFoldersPickAFolderOrGoBack(t *testing.T) {
	f := NewFolders("Folders", exampleOverview().Folders, "example-desktop")
	send(f, key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyEnter))
	if got, ok := f.Chosen(); !ok || got.Path != "~/Projects/website" || !f.Done() {
		t.Errorf("chosen = %q, %v; want ~/Projects/website, third in the list", got.Path, ok)
	}

	f = NewFolders("Folders", exampleOverview().Folders, "example-desktop")
	send(f, key(tea.KeyEscape))
	if _, ok := f.Chosen(); ok || !f.Done() {
		t.Error("esc should go back without choosing")
	}

	// q and esc go Back; only Ctrl+C cancels what the list is part of.
	for name, k := range map[string]tea.KeyPressMsg{
		"q":      {Code: 'q', Text: "q"},
		"esc":    key(tea.KeyEscape),
		"ctrl+c": {Code: 'c', Mod: tea.ModCtrl},
	} {
		f = NewFolders("Folders", exampleOverview().Folders, "example-desktop")
		send(f, k)
		if _, ok := f.Chosen(); ok || !f.Done() || f.Cancelled() != (name == "ctrl+c") {
			t.Errorf("%s: chosen %v, done %v, cancelled %v", name, ok, f.Done(), f.Cancelled())
		}
	}

	f = NewFolders("Folders", exampleOverview().Folders, "example-desktop")
	send(f, key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyDown), key(tea.KeyEnter))
	if _, ok := f.Chosen(); ok || !f.Done() {
		t.Error("Back, after the folders, should go back without choosing")
	}
}

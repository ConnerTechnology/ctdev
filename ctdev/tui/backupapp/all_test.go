package backupapp

import (
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/charmbracelet/x/ansi"
)

// exampleOverview is a destination two computers back up to, as
// backup.Destination.Overview returns it on example-desktop.
func exampleOverview() backup.Overview {
	return backup.Overview{
		Computers: 2, FolderCount: 3,
		Folders: []backup.FoundFolder{
			{Ref: backup.Ref{FolderID: "11111111-1111-4111-8111-111111111111"}, Host: "example-desktop", Path: "~/Documents/notes",
				Backups: 3, Latest: now.Add(-2 * time.Hour), Size: 52_000},
			{Ref: backup.Ref{Path: "~/Downloads/scans", Host: "example-laptop"}, Host: "example-laptop", Path: "~/Downloads/scans",
				Backups: 1, Latest: now.Add(-3 * 24 * time.Hour), Size: 3_400_000},
			{Ref: backup.Ref{FolderID: "22222222-2222-4222-8222-222222222222"}, Host: "example-laptop", Path: "~/Projects/website",
				Backups: 14, Latest: now.Add(-14 * time.Hour), Size: 1_200_000_000},
		},
	}
}

func exampleDest() *backup.Destination {
	return &backup.Destination{Name: "example-backups", Address: "b2:example-bucket:backups"}
}

// The dashboard's rows are what the destination holds; List backups is
// dimmed, and focus starts on Computers, only with nothing to list.
func TestAllDashboard(t *testing.T) {
	d := allDashboard(exampleDest(), exampleOverview(), 1_600_000_000, "")
	if d.Page.Title != "example-backups" || !strings.Contains(ansi.Strip(d.Page.Subtitle), "Backblaze B2") {
		t.Errorf("page = %+v", d.Page)
	}
	body := ansi.Strip(d.Page.Body)
	for _, want := range []string{"COMPUTERS", "with a key to example-backups", "FOLDERS", "from 2 computers", "STORED", "1.6 GB"} {
		if !strings.Contains(body, want) {
			t.Errorf("rows are missing %q:\n%s", want, body)
		}
	}
	var keys []string
	for _, a := range d.Actions {
		keys = append(keys, a.Key+" "+a.Label)
	}
	if got := strings.Join(keys, ","); got != "l List backups,c Computers,e Emergency Kit and password,q Quit" {
		t.Errorf("actions = %s", got)
	}
	if b := newDashboard(d); d.Actions[b.cursor].Key != "l" {
		t.Errorf("focus = %d, want List backups", b.cursor)
	}

	empty := allDashboard(exampleDest(), backup.Overview{Computers: 1}, 0, "")
	if b := newDashboard(empty); !empty.Actions[0].Dimmed || empty.Actions[b.cursor].Key != "c" {
		t.Errorf("empty: actions %+v, focus %d", empty.Actions, b.cursor)
	}
	if body := ansi.Strip(empty.Page.Body); !strings.Contains(body, "nothing backed up here yet") || !strings.Contains(body, "0 B") {
		t.Errorf("empty rows:\n%s", body)
	}
}

// Folders are grouped by computer in Overview's order, this one first and
// marked once, each with its last backup, size and number of backups.
func TestFoldersAreGroupedByComputer(t *testing.T) {
	p := foldersPicker("example-backups", exampleOverview().Folders, "example-desktop", 0, now)
	view := ansi.Strip(strings.Join(newPicker(p).view(frameWidth, 0).content, "\n"))
	order := []string{"FOLDER", "LAST BACKUP", "SIZE", "BACKUPS",
		"example-desktop (this computer)", "~/Documents/notes",
		"example-laptop", "~/Downloads/scans", "~/Projects/website"}
	at := 0
	for _, want := range order {
		i := strings.Index(view[at:], want)
		if i < 0 {
			t.Fatalf("want %q after position %d, in order %v:\n%s", want, at, order, view)
		}
		at += i + len(want)
	}
	for _, want := range []string{"2 hours ago", "52.0 kB", "3 backups", "1 backup", "14 backups", "1.2 GB", "3 days ago"} {
		if !strings.Contains(view, want) {
			t.Errorf("list is missing %q:\n%s", want, view)
		}
	}
	if strings.Count(view, "(this computer)") != 1 {
		t.Errorf("only example-desktop is this computer:\n%s", view)
	}
	if p.Page.Title != "example-backups › Folders" {
		t.Errorf("title = %q", p.Page.Title)
	}
}

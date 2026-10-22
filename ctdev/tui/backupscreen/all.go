package backupscreen

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// NewAll builds the `ctdev backup --all` screen for destination name, whose
// backend is backend and whose backups take stored bytes: a header line with
// what the destination holds, then
// List backups · Computers · Emergency Kit and password · Quit. Computers and
// Emergency Kit and password are dimmed until they're built (CTD-87).
func NewAll(name, backend string, o backup.Overview, stored int64) *Model {
	m := &Model{status: AllHeader(name, backend, o, stored), items: []Item{
		{Action: ListBackups, Label: "List backups", Dimmed: len(o.Folders) == 0},
		{Action: Computers, Label: "Computers", Dimmed: true},
		{Action: EmergencyKit, Label: "Emergency Kit and password", Dimmed: true},
		{Action: Quit, Label: "Quit"},
	}}
	m.cursor = m.next(-1, 1)
	return m
}

// AllHeader is the --all screen's first line:
// "<destination> (<backend>) · N computers · M folders · X stored".
func AllHeader(name, backend string, o backup.Overview, stored int64) string {
	return styles.Title.Render(fmt.Sprintf("%s (%s) · %s · %s · %s stored",
		name, backend, count(o.Computers, "computer"), count(o.FolderCount, "folder"), Bytes(stored))) + "\n"
}

// count is "1 folder" or "3 folders".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return thousands(n) + " " + noun + "s"
}

// Folders lists backed-up folders grouped by computer, in the order
// backup.Destination.Overview gives them (this computer first), with each
// one's path, latest backup, its size and the number of backups, then Back.
type Folders struct {
	title   string
	folders []backup.FoundFolder
	this    string // this computer's hostname
	cursor  int    // len(folders) is Back
	chosen  int    // index into folders, or -1
	done    bool
}

// NewFolders builds the list under title. this is this computer's hostname,
// whose group is marked.
func NewFolders(title string, folders []backup.FoundFolder, this string) *Folders {
	return &Folders{title: title, folders: folders, this: this, chosen: -1}
}

// Chosen is the folder picked, if one was.
func (f *Folders) Chosen() (backup.FoundFolder, bool) {
	if f.chosen < 0 {
		return backup.FoundFolder{}, false
	}
	return f.folders[f.chosen], true
}

// Done reports whether the screen is finished: a folder was picked, or Back.
func (f *Folders) Done() bool { return f.done }

func (f *Folders) Init() tea.Cmd { return nil }

func (f *Folders) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return f, nil
	}
	switch k.String() {
	case "up", "k", "shift+tab":
		f.cursor = max(f.cursor-1, 0)
	case "down", "j", "tab":
		f.cursor = min(f.cursor+1, len(f.folders))
	case "q", "esc", "ctrl+c":
		f.done = true
		return f, tea.Quit
	case "enter", "space":
		if f.cursor < len(f.folders) {
			f.chosen = f.cursor
		}
		f.done = true
		return f, tea.Quit
	}
	return f, nil
}

func (f *Folders) View() tea.View {
	var b strings.Builder
	b.WriteString(styles.Title.Render(f.title) + "\n")
	if f.done {
		return tea.NewView(b.String())
	}
	b.WriteString(folderLines(f.folders, f.this, f.cursor))
	b.WriteString("\n" + menuLine("Back", f.cursor == len(f.folders)))
	b.WriteString("\n  " + styles.Help.Render("↑/↓ move · enter open · esc back") + "\n")
	return tea.NewView(b.String())
}

// FolderList is the folders, grouped by computer as Folders shows them, as
// plain lines with no cursor, for output without a terminal.
func FolderList(folders []backup.FoundFolder, this string) string {
	return folderLines(folders, this, -1)
}

// folderLines renders the folders under a heading per computer, this one
// marked, with the one at cursor selected.
func folderLines(folders []backup.FoundFolder, this string, cursor int) string {
	var b strings.Builder
	width := 0
	for _, it := range folders {
		width = max(width, len([]rune(it.Path)))
	}
	host := ""
	for i, it := range folders {
		if i == 0 || it.Host != host {
			host = it.Host
			heading := host
			if host == this {
				heading += " (this computer)"
			}
			b.WriteString("\n  " + styles.Value.Render(heading) + "\n")
		}
		pad := strings.Repeat(" ", width-len([]rune(it.Path)))
		line := fmt.Sprintf("%s%s  %-16s  %8s  %s", it.Path, pad, when(it.Latest), Bytes(it.Size), count(it.Backups, "backup"))
		b.WriteString(menuLine(line, i == cursor))
	}
	return b.String()
}

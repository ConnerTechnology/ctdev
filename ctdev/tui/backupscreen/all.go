package backupscreen

import (
	"fmt"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

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

// FolderList is the folders under a heading per computer, this one marked,
// in the order backup.Destination.Overview gives them (this computer first).
func FolderList(folders []backup.FoundFolder, this string) string {
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
		b.WriteString("    " + line + "\n")
	}
	return b.String()
}

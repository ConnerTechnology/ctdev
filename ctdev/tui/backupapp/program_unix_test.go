//go:build unix

package backupapp

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// Under --dry-run, restic's commands stay in the terminal after the program
// exits, in order with the result lines, as they did before it was
// full-screen.
func TestDryRunCommandsArePrintedAfterExit(t *testing.T) {
	d, _ := standinFolder(t, true, backs, forgets, "example-backups")
	d.Machine.Opts.DryRun = true
	p := startProgram(t, d, folderFlow)
	p.term.waitFor("Back up now")
	io.WriteString(p.keys, "b")
	p.term.waitFor("All of ~/Projects/website")
	io.WriteString(p.keys, "\r")
	p.term.waitFor("[dry-run] restic forget")
	io.WriteString(p.keys, "q")
	io.WriteString(p.keys, "q")
	if err := <-p.done; err != nil {
		t.Fatal(err)
	}
	after := ansi.Strip(p.after.String())
	backup, forget := strings.Index(after, "[dry-run] restic backup"), strings.Index(after, "[dry-run] restic forget")
	if backup < 0 || forget < backup {
		t.Errorf("after exit:\n%s\nwant the backup's command, then the keep rules'", after)
	}
}

// While the program runs, restic has no controlling terminal. Quitting while
// it's still stopping says what ctdev waits on, after the screen closes.
func TestQuittingWhileResticStopsSaysSo(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "session")
	d, _ := standinFolder(t, true, `ps -o pgid= -p $$ | tr -d ' ' > "`+marker+`"; echo $$ >> "`+marker+`"
  trap 'sleep 1; exit 130' INT
  sleep 60 >/dev/null 2>&1 &
  wait $!`, forgets, "example-backups")
	p := startProgram(t, d, folderFlow)
	p.term.waitFor("Back up now")
	io.WriteString(p.keys, "b")
	p.term.waitFor("All of ~/Projects/website")
	io.WriteString(p.keys, "\r")
	p.term.waitFor("BACKING UP")
	io.WriteString(p.keys, "\x03")
	p.term.waitFor("Cancelling…")
	io.WriteString(p.keys, "\x03")
	if err := <-p.done; err != nil {
		t.Fatal(err)
	}
	if after := ansi.Strip(p.after.String()); !strings.Contains(after, "Waiting for restic to stop…") {
		t.Errorf("after exit: %q, want the wait said", after)
	}
	data, _ := os.ReadFile(marker)
	if f := strings.Fields(string(data)); len(f) != 2 || f[0] != f[1] {
		t.Errorf("restic's session and pid: %q, want restic to lead its own session", data)
	}
	if d.Machine.NoTerminal {
		t.Error("NoTerminal is still set after the program exits")
	}
}

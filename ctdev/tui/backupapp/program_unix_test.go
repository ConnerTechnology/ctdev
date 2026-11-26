//go:build unix

package backupapp

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

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
	if err := p.quit(); err != nil {
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
//
// The stand-in tells the test when it is running and will take its time to
// stop: it installs its INT trap, then reports its session and pid through a
// FIFO. Ctrl-C waits for that, because the screen shows the backup running
// before restic has started.
func TestQuittingWhileResticStopsSaysSo(t *testing.T) {
	dir := t.TempDir()
	fifo := filepath.Join(dir, "ready")
	release := filepath.Join(dir, "release")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(release, 0o600); err != nil {
		t.Fatal(err)
	}
	ready := make(chan []byte, 1)
	go func() {
		data, _ := os.ReadFile(fifo)
		ready <- data
	}()
	d, _ := standinFolder(t, true, `trap 'cat "`+release+`" >/dev/null; exit 130' INT
  { ps -o pgid= -p $$ | tr -d ' '; echo $$; } > "`+fifo+`"
  sleep 60 >/dev/null 2>&1 &
  wait $!`, forgets, "example-backups")
	p := startProgram(t, d, folderFlow)
	p.term.waitFor("Back up now")
	io.WriteString(p.keys, "b")
	p.term.waitFor("All of ~/Projects/website")
	io.WriteString(p.keys, "\r")
	var session []byte
	select {
	case session = <-ready:
	case <-time.After(10 * time.Second):
		t.Fatalf("restic never reported ready on:\n%s", p.term.text())
	}
	io.WriteString(p.keys, "\x03")
	p.term.waitFor("Cancelling…")
	io.WriteString(p.keys, "\x03")
	p.after.waitFor(t, "Waiting for restic to stop…")
	if err := os.WriteFile(release, []byte("go\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := <-p.done; err != nil {
		t.Fatal(err)
	}
	if f := strings.Fields(string(session)); len(f) != 2 || f[0] != f[1] {
		t.Errorf("restic's session and pid: %q, want restic to lead its own session", session)
	}
	if d.Machine.NoTerminal {
		t.Error("NoTerminal is still set after the program exits")
	}
}

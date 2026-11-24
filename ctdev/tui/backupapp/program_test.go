package backupapp

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/charmbracelet/x/ansi"
)

// The badge ends at the frame's right edge, however long the title.
func TestTitleLineBadgeEndsAtTheEdge(t *testing.T) {
	for _, p := range []Page{
		{Title: "~/Projects/website", Badge: Badge{Healthy, "HEALTHY"}},
		{Title: "~/Projects/website", Context: "you're in ~/Projects/website/src", Badge: Badge{Danger, "DESTINATION NOT SET UP"}},
	} {
		line := ansi.Strip(titleLine(p, frameWidth))
		if ansi.StringWidth(line) != frameWidth || !strings.HasSuffix(line, p.Badge.Text+" ") {
			t.Errorf("%q is %d wide, want the badge ending at %d", line, ansi.StringWidth(line), frameWidth)
		}
	}
}

// A model showing a dashboard, as the program would.
func showing(dryRun bool) *model {
	m := &model{dryRun: dryRun}
	m.Update(request{page: Page{Title: "~/Projects/website", Badge: Badge{Healthy, "HEALTHY"}, Subtitle: "backed up to example-backups",
		Body: Row("LAST BACKUP", "14 hours ago")}, step: newDashboard(Dashboard{
		Message: []Line{{Error, "the state's own message"}}, Actions: []Action{{Key: "b", Label: "Back up now"}, {Key: "q", Label: "Quit"}},
	}), reply: make(chan answer, 1)})
	return m
}

func TestFrame(t *testing.T) {
	got := ansi.Strip(showing(false).render())
	lines := strings.Split(got, "\n")
	if len(lines) < 9 || !strings.HasPrefix(lines[0], "~/Projects/website") || lines[1] != "backed up to example-backups" ||
		!strings.Contains(lines[3], "LAST BACKUP") || !strings.HasPrefix(lines[5], "────") || !strings.Contains(got, "the state's own message") ||
		!strings.Contains(got, " b  Back up now ") || !strings.HasSuffix(got, "←/→ choose · enter run · or press the letter") {
		t.Errorf("frame:\n%s", got)
	}

	dry := ansi.Strip(showing(true).render())
	if !strings.Contains(dry, "▲ DRY RUN") || strings.Contains(dry, "HEALTHY") || !strings.Contains(dry, "--dry-run: nothing is changed") {
		t.Errorf("dry run frame:\n%s", dry)
	}
}

// Result lines replace the dashboard's message until the next key; under
// --dry-run, restic's commands show as a log under a heading.
func TestResultLinesShowUntilTheNextKey(t *testing.T) {
	m := showing(true)
	m.Update(sayMsg{OK, "✓ Backed up 1 file (15 B), 15 B new, as 01234567."})
	m.Update(logMsg("[dry-run] restic backup . --json"))
	got := ansi.Strip(m.render())
	if !strings.Contains(got, "✓ Backed up") || !strings.Contains(got, "Dry run  nothing is changed. This is what would run:") ||
		!strings.Contains(got, "[dry-run] restic backup") || strings.Contains(got, "the state's own message") {
		t.Errorf("with result lines:\n%s", got)
	}
	m.Update(key("right"))
	if got := ansi.Strip(m.render()); strings.Contains(got, "✓ Backed up") || !strings.Contains(got, "the state's own message") {
		t.Errorf("after a key:\n%s", got)
	}
}

func TestLineWriter(t *testing.T) {
	var got []string
	w := &lineWriter{post: func(s string) { got = append(got, s) }}
	io.WriteString(w, "[dry-run] restic backup")
	io.WriteString(w, " .\r\n\nsecond\nunfinished")
	if strings.Join(got, "|") != "[dry-run] restic backup .|second" {
		t.Errorf("lines = %q", got)
	}
}

// terminal is the program's output, read as plain text, with a way to wait
// for something to appear on it.
type terminal struct {
	t       *testing.T
	mu      sync.Mutex
	out     bytes.Buffer
	changed chan struct{}
}

func (term *terminal) Write(p []byte) (int, error) {
	term.mu.Lock()
	term.out.Write(p)
	term.mu.Unlock()
	select {
	case term.changed <- struct{}{}:
	default:
	}
	return len(p), nil
}

// waitFor returns once s has been drawn.
func (term *terminal) waitFor(s string) {
	term.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		term.mu.Lock()
		seen := strings.Contains(ansi.Strip(term.out.String()), s)
		term.mu.Unlock()
		if seen {
			return
		}
		select {
		case <-term.changed:
		case <-timeout:
			term.t.Fatalf("%q never appeared on:\n%s", s, ansi.Strip(term.out.String()))
		}
	}
}

// The real program: the dashboard draws, keys reach the flow, a result line
// shows, q quits, and the result is printed after the program exits.
func TestProgramSmoke(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, "Projects", "website")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	in, keys := io.Pipe()
	term := &terminal{t: t, changed: make(chan struct{}, 1)}
	saved := programOptions
	programOptions = func() []tea.ProgramOption {
		return []tea.ProgramOption{tea.WithInput(in), tea.WithOutput(term), tea.WithWindowSize(96, 40)}
	}
	t.Cleanup(func() { programOptions = saved })

	var after bytes.Buffer
	done := make(chan error)
	go func() { done <- run(context.Background(), Deps{Machine: m, Cwd: dir}, &after, folderFlow) }()

	term.waitFor("NOT BACKED UP")
	term.waitFor("no destination on this computer yet")
	io.WriteString(keys, "b")
	term.waitFor("This computer has no backup destination yet. Set one up first.")
	io.WriteString(keys, "\r")
	term.waitFor("isn't built yet")
	io.WriteString(keys, "q")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ansi.Strip(after.String()), "setting up a destination isn't built yet") {
		t.Errorf("after exit: %q, want the result line", after.String())
	}
}

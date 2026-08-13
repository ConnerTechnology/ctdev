package progress

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestProgressBarResizesToWidth(t *testing.T) {
	val := New([]string{"docker"}, ModeInstall, false)
	m := &val

	// Narrow terminal clamps to the floor; wide terminal clamps to the ceiling.
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 24})
	if got := updated.(*Model).progressBar.Width(); got != 10 {
		t.Errorf("width 20: expected bar clamped to 10, got %d", got)
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 300, Height: 24})
	if got := updated.(*Model).progressBar.Width(); got != 80 {
		t.Errorf("width 300: expected bar clamped to 80, got %d", got)
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	if got := updated.(*Model).progressBar.Width(); got != 44 {
		t.Errorf("width 60: expected bar 44 (60-16), got %d", got)
	}
}

func TestProgressModelInit(t *testing.T) {
	m := New([]string{"docker", "btop"}, ModeInstall, false)
	if len(m.components) != 2 {
		t.Errorf("expected 2 components, got %d", len(m.components))
	}
	if m.components[0].Status != StatusWaiting {
		t.Error("expected initial status to be waiting")
	}
}

func TestProgressModelInstallDone(t *testing.T) {
	val := New([]string{"docker", "btop"}, ModeInstall, false)
	m := &val

	updated, _ := m.Update(InstallStartMsg{Name: "docker"})
	m = updated.(*Model)
	if m.components[0].Status != StatusRunning {
		t.Error("expected docker to be running")
	}

	updated, _ = m.Update(InstallDoneMsg{Name: "docker", Duration: 3 * time.Second})
	m = updated.(*Model)
	if m.components[0].Status != StatusDone {
		t.Error("expected docker to be done")
	}
	if m.donePercent() != 0.5 {
		t.Errorf("expected 50%% done, got %f", m.donePercent())
	}
}

func TestProgressModelInstallFail(t *testing.T) {
	val := New([]string{"docker"}, ModeInstall, false)
	m := &val

	updated, _ := m.Update(InstallFailMsg{Name: "docker", Error: "apt lock", Duration: time.Second})
	m = updated.(*Model)
	if m.components[0].Status != StatusFailed {
		t.Error("expected docker to be failed")
	}
	if m.components[0].Error != "apt lock" {
		t.Errorf("expected 'apt lock', got %s", m.components[0].Error)
	}
}

func TestProgressOutputTail(t *testing.T) {
	lines := appendTail(nil, "a", 3)
	lines = appendTail(lines, "b", 3)
	lines = appendTail(lines, "c", 3)
	lines = appendTail(lines, "d", 3)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "b" {
		t.Errorf("expected 'b' first, got %s", lines[0])
	}
}

func TestProgressModelInstallSkip(t *testing.T) {
	val := New([]string{"docker"}, ModeInstall, false)
	m := &val

	updated, _ := m.Update(InstallStartMsg{Name: "docker"})
	m = updated.(*Model)
	if m.components[0].Status != StatusRunning {
		t.Error("expected docker to be running")
	}

	updated, cmd := m.Update(InstallSkipMsg{Name: "docker"})
	m = updated.(*Model)
	if m.components[0].Status != StatusSkipped {
		t.Error("expected docker to be skipped")
	}
	if cmd == nil {
		t.Error("expected a progress-bar SetPercent command on skip")
	}
	if got := m.donePercent(); got != 1.0 {
		t.Errorf("donePercent after sole skip = %f, want 1.0", got)
	}
}

func TestProgressModelMixedDoneAndSkip(t *testing.T) {
	val := New([]string{"a", "b", "c"}, ModeInstall, false)
	m := &val

	updated, _ := m.Update(InstallDoneMsg{Name: "a", Duration: time.Second})
	m = updated.(*Model)
	updated, _ = m.Update(InstallSkipMsg{Name: "b"})
	m = updated.(*Model)
	// 2 of 3 resolved (1 done, 1 skipped)
	if got := m.donePercent(); got != 2.0/3.0 {
		t.Errorf("donePercent = %f, want %f", got, 2.0/3.0)
	}
	updated, _ = m.Update(InstallSkipMsg{Name: "c"})
	m = updated.(*Model)
	if got := m.donePercent(); got != 1.0 {
		t.Errorf("donePercent after all resolved = %f, want 1.0", got)
	}
}

func TestProgressModelInstallOutput(t *testing.T) {
	val := New([]string{"docker"}, ModeInstall, false)
	m := &val

	updated, _ := m.Update(InstallStartMsg{Name: "docker"})
	m = updated.(*Model)

	updated, _ = m.Update(InstallOutputMsg{Name: "docker", Line: "Downloading..."})
	m = updated.(*Model)
	updated, _ = m.Update(InstallOutputMsg{Name: "docker", Line: "Installing..."})
	m = updated.(*Model)

	if len(m.components[0].Output) != 2 {
		t.Errorf("expected 2 output lines, got %d", len(m.components[0].Output))
	}
	if m.components[0].Output[0] != "Downloading..." {
		t.Errorf("expected 'Downloading...', got %q", m.components[0].Output[0])
	}
	if m.components[0].Output[1] != "Installing..." {
		t.Errorf("expected 'Installing...', got %q", m.components[0].Output[1])
	}
}

func TestSummaryReplaysFailureOutput(t *testing.T) {
	val := New([]string{"docker", "btop"}, ModeInstall, false)
	m := &val

	updated, _ := m.Update(InstallStartMsg{Name: "docker"})
	m = updated.(*Model)
	updated, _ = m.Update(InstallOutputMsg{Name: "docker", Line: "E: Unable to locate package docker-ce"})
	m = updated.(*Model)
	updated, _ = m.Update(InstallFailMsg{Name: "docker", Error: "exit status 100", Duration: time.Second})
	m = updated.(*Model)

	// The failure tail lives in the printed report, not the final frame — the
	// frame has to stay short enough that the inline renderer can't truncate it.
	report := m.SummaryReport()
	if !strings.Contains(report, "Unable to locate package") {
		t.Errorf("expected the failure's output replayed in the report, got:\n%s", report)
	}

	summary := m.viewSummary()
	if !strings.Contains(summary, "finished with 1 failure") {
		t.Errorf("expected an honest failure header, got:\n%s", summary)
	}
	if !strings.Contains(summary, "docker") {
		t.Errorf("expected the failed name in the final frame, got:\n%s", summary)
	}

	done, failed, skipped, notRun := m.Counts()
	if done != 0 || failed != 1 || skipped != 0 || notRun != 1 {
		t.Errorf("Counts() = %d,%d,%d,%d; want 0,1,0,1", done, failed, skipped, notRun)
	}
}

// TestSummaryFrameStaysBounded is the regression guard for the truncated-output
// bug: the final frame used to render every component plus a replayed tail per
// failure, and Bubble Tea's inline renderer drops lines from the *top* of any
// frame taller than the terminal — so on a big update run the headline and the
// first results vanished. The frame must now be short regardless of run size.
func TestSummaryFrameStaysBounded(t *testing.T) {
	names := make([]string, 60)
	for i := range names {
		names[i] = fmt.Sprintf("pkg-%02d", i)
	}
	val := New(names, ModeUpdate, false)
	m := &val

	for i, name := range names {
		updated, _ := m.Update(InstallStartMsg{Name: name})
		m = updated.(*Model)
		// Every third step fails, each with a long output tail to replay.
		if i%3 == 0 {
			for line := 0; line < 30; line++ {
				updated, _ = m.Update(InstallOutputMsg{Name: name, Line: fmt.Sprintf("output line %d", line)})
				m = updated.(*Model)
			}
			updated, _ = m.Update(InstallFailMsg{Name: name, Error: "exit status 1", Duration: time.Second})
		} else {
			updated, _ = m.Update(InstallDoneMsg{Name: name, Duration: time.Second})
		}
		m = updated.(*Model)
	}
	updated, _ := m.Update(AllDoneMsg{})
	m = updated.(*Model)

	// Comfortably inside a short terminal, with room for the shell prompt after.
	const maxFrameLines = 12
	frame := m.View().Content
	if got := strings.Count(frame, "\n"); got > maxFrameLines {
		t.Errorf("final frame is %d lines, want at most %d — it will be truncated by the inline renderer:\n%s",
			got, maxFrameLines, frame)
	}

	// The detail still has to be reachable, just not through the frame.
	report := m.SummaryReport()
	if !strings.Contains(report, "pkg-59") {
		t.Error("expected every component in SummaryReport, pkg-59 missing")
	}
	if !strings.Contains(report, "output line 29") {
		t.Error("expected failure output tails in SummaryReport")
	}
}

// A short, fully successful run is already described by the final frame; the
// report would just repeat it.
func TestSummaryReportEmptyForTrivialRun(t *testing.T) {
	val := New([]string{"jq"}, ModeInstall, false)
	m := &val

	updated, _ := m.Update(InstallStartMsg{Name: "jq"})
	m = updated.(*Model)
	updated, _ = m.Update(InstallDoneMsg{Name: "jq", Duration: time.Second})
	m = updated.(*Model)

	if report := m.SummaryReport(); report != "" {
		t.Errorf("expected no report for a single-item clean run, got:\n%s", report)
	}
}

func TestProgressDryRunLabelled(t *testing.T) {
	val := New([]string{"docker"}, ModeInstall, true)
	m := &val
	if view := m.viewProgress(); !strings.Contains(view, "(dry run)") {
		t.Errorf("expected '(dry run)' in progress view, got: %s", view)
	}
}

func TestProgressModelUninstallMode(t *testing.T) {
	val := New([]string{"docker"}, ModeUninstall, false)
	m := &val

	view := m.viewProgress()
	if !strings.Contains(view, "Uninstalling") {
		t.Errorf("expected 'Uninstalling' in view, got: %s", view)
	}

	updated, _ := m.Update(InstallDoneMsg{Name: "docker", Duration: time.Second})
	m = updated.(*Model)
	updated, _ = m.Update(AllDoneMsg{})
	m = updated.(*Model)

	summary := m.viewSummary()
	if !strings.Contains(summary, "Uninstall complete") {
		t.Errorf("expected 'Uninstall complete' in summary, got: %s", summary)
	}
}

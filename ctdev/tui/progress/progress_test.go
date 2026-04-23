package progress

import (
	"strings"
	"testing"
	"time"
)

func TestProgressModelInit(t *testing.T) {
	m := New([]string{"docker", "btop"}, ModeInstall)
	if len(m.components) != 2 {
		t.Errorf("expected 2 components, got %d", len(m.components))
	}
	if m.components[0].Status != StatusWaiting {
		t.Error("expected initial status to be waiting")
	}
}

func TestProgressModelInstallDone(t *testing.T) {
	val := New([]string{"docker", "btop"}, ModeInstall)
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
	val := New([]string{"docker"}, ModeInstall)
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
	val := New([]string{"docker"}, ModeInstall)
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
	val := New([]string{"a", "b", "c"}, ModeInstall)
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
	val := New([]string{"docker"}, ModeInstall)
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

func TestProgressModelUninstallMode(t *testing.T) {
	val := New([]string{"docker"}, ModeUninstall)
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

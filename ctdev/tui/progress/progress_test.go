package progress

import (
	"testing"
	"time"
)

func TestProgressModelInit(t *testing.T) {
	m := New([]string{"docker", "btop"})
	if len(m.components) != 2 {
		t.Errorf("expected 2 components, got %d", len(m.components))
	}
	if m.components[0].Status != StatusWaiting {
		t.Error("expected initial status to be waiting")
	}
}

func TestProgressModelInstallDone(t *testing.T) {
	m := New([]string{"docker", "btop"})

	updated, _ := m.Update(InstallStartMsg{Name: "docker"})
	m = updated.(Model)
	if m.components[0].Status != StatusRunning {
		t.Error("expected docker to be running")
	}

	updated, _ = m.Update(InstallDoneMsg{Name: "docker", Duration: 3 * time.Second})
	m = updated.(Model)
	if m.components[0].Status != StatusDone {
		t.Error("expected docker to be done")
	}
	if m.donePercent() != 0.5 {
		t.Errorf("expected 50%% done, got %f", m.donePercent())
	}
}

func TestProgressModelInstallFail(t *testing.T) {
	m := New([]string{"docker"})

	updated, _ := m.Update(InstallFailMsg{Name: "docker", Error: "apt lock", Duration: time.Second})
	m = updated.(Model)
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

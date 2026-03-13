package wizard

import (
	"testing"
)

func testSteps() []Step {
	return []Step{
		{
			Title:       "GPU Drivers",
			Description: "Configure GPU drivers",
			Options: []Option{
				{Label: "Install NVIDIA driver", Enabled: true},
				{Label: "Sign kernel module", Enabled: true},
			},
		},
		{
			Title:       "Audio",
			Description: "Configure audio",
			Options: []Option{
				{Label: "Install PipeWire", Enabled: true},
				{Label: "Enable Bluetooth", AlreadyDone: true},
			},
		},
	}
}

func TestWizardNavigation(t *testing.T) {
	m := New(testSteps())
	if m.currentStep != 0 {
		t.Error("expected to start at step 0")
	}
	// Move to next step
	m.completed[0] = true
	m.currentStep = 1
	m.cursor = 0
	if m.currentStep != 1 {
		t.Error("expected step 1")
	}
}

func TestWizardToggleOption(t *testing.T) {
	m := New(testSteps())
	// First option starts enabled
	if !m.steps[0].Options[0].Enabled {
		t.Error("expected first option to be enabled")
	}
	// Toggle it off
	m.steps[0].Options[0].Enabled = false
	if m.steps[0].Options[0].Enabled {
		t.Error("expected first option to be disabled after toggle")
	}
}

func TestWizardAlreadyDoneNotToggleable(t *testing.T) {
	m := New(testSteps())
	m.currentStep = 1
	// Option at index 1 is AlreadyDone
	opt := m.steps[1].Options[1]
	if !opt.AlreadyDone {
		t.Error("expected bluetooth to be already done")
	}
}

func TestWizardQuit(t *testing.T) {
	m := New(testSteps())
	m.quitting = true
	result := m.GetResult()
	if !result.Quit {
		t.Error("expected quit result")
	}
}

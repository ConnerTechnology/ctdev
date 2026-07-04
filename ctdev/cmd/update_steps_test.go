package cmd

import (
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/tui/checklist"
)

func TestBuildUpdateSteps_GroupsAndOrders(t *testing.T) {
	items := []checklist.UpdateItem{
		{Name: "pihole", Source: "docker"},
		{Name: "vim", Source: "apt"},
		{Name: "go", Source: "runtime", CurrentVer: "1.25", NewVer: "1.26"},
		{Name: "curl", Source: "apt"},
		{Name: "org.gimp.GIMP", Source: "flatpak"},
	}

	steps := buildUpdateSteps(items)
	var names []string
	for _, s := range steps {
		names = append(names, s.name)
	}

	want := []string{
		"apt (2 packages)", // apt items batch into one step
		"flatpak: org.gimp.GIMP",
		"go 1.25 → 1.26",
		"docker: pihole",
	}
	if len(names) != len(want) {
		t.Fatalf("steps = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("step[%d] = %q, want %q", i, names[i], want[i])
		}
	}
	for i, s := range steps {
		if s.run == nil {
			t.Errorf("step[%d] %q has nil run func", i, s.name)
		}
	}
}

func TestBuildUpdateSteps_UnknownRuntimeAndCLIDropped(t *testing.T) {
	items := []checklist.UpdateItem{
		{Name: "mystery-runtime", Source: "runtime"},
		{Name: "mystery-tool", Source: "cli"},
	}
	if steps := buildUpdateSteps(items); len(steps) != 0 {
		t.Errorf("expected no steps for unknown runtime/cli items, got %d", len(steps))
	}
}

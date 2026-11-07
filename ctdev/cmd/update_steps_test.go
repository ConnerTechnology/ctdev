package cmd

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/checklist"
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

// The apt upgrade step runs from a progress screen with no TTY, so it goes
// through sysutil.APTGet: no prompts, a wait on the dpkg lock, old conffiles kept.
func TestBuildUpdateSteps_AptRunsUnattended(t *testing.T) {
	steps := buildUpdateSteps([]checklist.UpdateItem{{Name: "curl", Source: "apt"}})
	var buf bytes.Buffer
	if err := steps[0].run(context.Background(), sysutil.Opts{Stdout: &buf, DryRun: true}); err != nil {
		t.Fatal(err)
	}
	want := "env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 -o Acquire::Retries=3 " +
		"-o Dpkg::Options::=--force-confold install --only-upgrade -y curl\n"
	if !strings.HasSuffix(buf.String(), want) {
		t.Errorf("got %q, want it to end with %q", buf.String(), want)
	}
}

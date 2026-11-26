package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/cleanup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/charmbracelet/x/ansi"
)

func TestHasWork(t *testing.T) {
	cases := []struct {
		name string
		in   cleanup.ScanResult
		want bool
	}{
		{"has bytes", cleanup.ScanResult{Bytes: 1024}, true},
		{"zero bytes", cleanup.ScanResult{Bytes: 0}, false},
		{"zero with none note", cleanup.ScanResult{Bytes: 0, Note: "none"}, false},
		{"unknown with note", cleanup.ScanResult{Bytes: -1, Note: "3 packages"}, true},
		{"unknown but none", cleanup.ScanResult{Bytes: -1, Note: "none"}, false},
	}
	for _, c := range cases {
		if got := hasWork(c.in); got != c.want {
			t.Errorf("%s: hasWork=%v want %v", c.name, got, c.want)
		}
	}
}

func TestSizeLabel(t *testing.T) {
	cases := []struct {
		in   cleanup.ScanResult
		want string
	}{
		{cleanup.ScanResult{Bytes: 1536}, "1.5 KB"},
		{cleanup.ScanResult{Bytes: 1536, Note: "2 revisions"}, "1.5 KB · 2 revisions"},
		{cleanup.ScanResult{Bytes: -1, Note: "3 packages"}, "3 packages"},
		{cleanup.ScanResult{Bytes: 0}, "—"},
	}
	for _, c := range cases {
		if got := sizeLabel(c.in); got != c.want {
			t.Errorf("sizeLabel(%+v)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestPrintActionableTotalsOnlySafeTasks(t *testing.T) {
	tasks := []cleanup.Task{
		{ID: "safe", Name: "Example safe cache", Risk: cleanup.Safe},
		{ID: "opt", Name: "Example trash", Risk: cleanup.OptIn},
	}
	results := map[string]cleanup.ScanResult{
		"safe": {Bytes: 1 << 20},
		"opt":  {Bytes: 1 << 30},
	}
	var buf bytes.Buffer
	printActionable(&buf, tasks, results)
	out := ansi.Strip(buf.String())

	optAt := strings.Index(out, "not cleaned in batch")
	if optAt < 0 {
		t.Fatalf("opt-in tasks not set apart:\n%s", out)
	}
	cleaned, skipped := out[:optAt], out[optAt:]
	if !strings.Contains(cleaned, "Reclaimable ≈ 1.0 MB") {
		t.Errorf("reclaimable total should count only the safe task:\n%s", out)
	}
	if strings.Contains(cleaned, "Example trash") || !strings.Contains(skipped, "Example trash") {
		t.Errorf("opt-in task should be listed only under the opt-in section:\n%s", out)
	}
	if !strings.Contains(skipped, "Opt-in ≈ 1.0 GB") {
		t.Errorf("opt-in section should carry its own total:\n%s", out)
	}
}

func TestCleanupNeedsRoot(t *testing.T) {
	tasks := []cleanup.Task{
		{ID: "thumbs", NeedsRoot: false},
		{ID: "apt", NeedsRoot: true},
	}
	if cleanupNeedsRoot(tasks, map[string]bool{"thumbs": true}) {
		t.Error("user-only selection should not require root")
	}
	if !cleanupNeedsRoot(tasks, map[string]bool{"thumbs": true, "apt": true}) {
		t.Error("selection containing a root task should require root")
	}
}

func TestExecuteCleanupTasks_CountsFailuresAndReclaimed(t *testing.T) {
	tasks := []cleanup.Task{
		{ID: "ok", Name: "OK cache", Run: func(context.Context, sysutil.Opts) error { return nil }},
		{ID: "fail", Name: "Broken cache", Run: func(context.Context, sysutil.Opts) error { return errors.New("boom") }},
		{ID: "unselected", Name: "Skipped", Run: func(context.Context, sysutil.Opts) error { t.Error("unselected task ran"); return nil }},
	}
	results := map[string]cleanup.ScanResult{
		"ok":   {Bytes: 2048},
		"fail": {Bytes: 4096},
	}
	var out bytes.Buffer
	reclaimed, failed := executeCleanupTasks(context.Background(), sysutil.Opts{Stdout: &out}, tasks, map[string]bool{"ok": true, "fail": true}, results)
	if reclaimed != 2048 {
		t.Errorf("reclaimed = %d, want 2048 (only succeeded task)", reclaimed)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
	if !strings.Contains(out.String(), "warning: boom") {
		t.Errorf("expected failure warning in output:\n%s", out.String())
	}
}

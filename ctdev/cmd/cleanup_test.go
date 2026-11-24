package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/cleanup"
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

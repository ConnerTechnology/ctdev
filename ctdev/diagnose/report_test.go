package diagnose

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderSeparatesLongNamesFromDetail(t *testing.T) {
	checks := []Check{
		{ID: "a", Name: "Controller certificate", Group: GroupNetwork},
		{ID: "b", Name: "An example check name well past the column", Group: GroupNetwork},
		{ID: "c", Name: "Short", Group: GroupNetwork},
	}
	results := map[string]Result{
		"a": {Severity: Warn, Detail: "self-signed", Advice: "trust it"},
		"b": {Severity: Warn, Detail: "long detail"},
		"c": {Severity: OK, Detail: "short detail"},
	}
	out := ansi.Strip(Render(Report{Checks: checks, Results: results, Facts: Facts{Hostname: "h"}}, 80))
	lines := strings.Split(out, "\n")

	detailCol := -1
	for _, want := range []struct{ name, detail string }{
		{"Controller certificate", "self-signed"},
		{"An example check name well past the column", "long detail"},
		{"Short", "short detail"},
	} {
		var line string
		for _, l := range lines {
			if strings.Contains(l, want.name) {
				line = l
			}
		}
		if !strings.Contains(line, want.detail) {
			t.Fatalf("%q and its detail are not on one line:\n%s", want.name, out)
		}
		if strings.Contains(line, want.name+want.detail) {
			t.Errorf("%q runs into its detail: %q", want.name, line)
		}
		col := ansi.StringWidth(line[:strings.Index(line, want.detail)])
		if detailCol == -1 {
			detailCol = col
		} else if col != detailCol {
			t.Errorf("detail for %q starts at column %d, want %d:\n%s", want.name, col, detailCol, out)
		}
	}

	for _, l := range lines {
		if i := strings.Index(l, "→ trust it"); i >= 0 && ansi.StringWidth(l[:i]) != detailCol {
			t.Errorf("advice starts at column %d, want %d under the detail:\n%s", ansi.StringWidth(l[:i]), detailCol, out)
		}
	}
}

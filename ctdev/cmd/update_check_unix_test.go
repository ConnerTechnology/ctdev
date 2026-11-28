//go:build unix

package cmd

import (
	"strings"
	"testing"
)

// Drive the command with only synthetic sources so a partial scan exercises
// the same status and output an unattended --check caller sees.
func TestUpdateCheckReportsCoverage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("NODENV_ROOT", t.TempDir())
	oldCheck, oldNoRefresh, oldVersion := flagCheck, flagNoRefresh, version
	flagCheck, flagNoRefresh, version = true, false, "dev"
	t.Cleanup(func() { flagCheck, flagNoRefresh, version = oldCheck, oldNoRefresh, oldVersion })

	const found = "printf '%s\\n' 'example-tool/testing 2.0 amd64 [upgradable from: 1.0]'"
	for _, tc := range []struct {
		name         string
		bins         map[string]string
		failedSource string
		wantUpdate   bool
	}{
		{"current", nil, "", false},
		{"one failing source", map[string]string{"flatpak": "exit 1"}, "flatpak", false},
		{"updates and failure", map[string]string{"flatpak": "exit 1", "apt": found}, "flatpak", true},
		{"failed version probe", map[string]string{"kubectl": "exit 1"}, "kubectl", false},
		{"updates only", map[string]string{"apt": found}, "", true},
		{"stale catalog", map[string]string{"brew": `if [ "$1" = update ]; then exit 1; fi; echo '{"formulae":[],"casks":[]}'`}, "brew update failed", false},
		{"npm registry error", map[string]string{"npm": `echo '{"error":{"code":"ENETUNREACH"}}'; exit 1`}, "npm", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fakeBins(t, tc.bins)
			var err error
			out := captureStdout(t, func() { err = runUpdate(updateCmd, nil) })
			if tc.failedSource == "" && err != nil {
				t.Fatal(err)
			}
			if tc.failedSource != "" && (err == nil || !strings.Contains(err.Error(), tc.failedSource)) {
				t.Errorf("error = %v, want incomplete scan naming %s", err, tc.failedSource)
			}
			if got := strings.Contains(out, "Everything is up to date."); got != (tc.failedSource == "" && !tc.wantUpdate) {
				t.Errorf("wrong up-to-date verdict: %s", out)
			}
			if strings.Contains(out, "example-tool") != tc.wantUpdate {
				t.Errorf("wrong update list: %s", out)
			}
		})
	}
}

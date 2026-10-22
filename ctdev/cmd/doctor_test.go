package cmd

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/diagnose"
	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// --strict fails the run on failures only; an sshd that is off is reported as
// information and must not trip it.
func TestStrictIgnoresInfoRows(t *testing.T) {
	results := map[string]diagnose.Result{
		"service.ssh":    {Severity: diagnose.Info, Detail: "off"},
		"service.docker": {Severity: diagnose.OK, Detail: "running"},
	}
	if n := countFailures(results); n != 0 {
		t.Errorf("countFailures = %d, want 0", n)
	}
	results["service.tailscaled"] = diagnose.Result{Severity: diagnose.Fail, Detail: "tailscaled is inactive"}
	if n := countFailures(results); n != 1 {
		t.Errorf("countFailures = %d, want 1 for a stopped tailscaled", n)
	}
}

// doctor is the one caller that knows what ctdev installed; the facts it hands
// the catalog have to carry that, on top of what GatherFacts found.
func TestDoctorFactsCarryInstalledComponents(t *testing.T) {
	gather := func(context.Context, platform.Info) diagnose.Facts {
		return diagnose.Facts{Hostname: "test-host", SSHUnitInstalled: true}
	}
	installed := func() map[string]bool { return map[string]bool{"git": true, "docker": true} }

	f := gatherDoctorFacts(context.Background(), platform.Info{OS: platform.Linux}, gather, installed)

	if !f.Components["git"] || !f.Components["docker"] || len(f.Components) != 2 {
		t.Errorf("Components = %v, want git and docker", f.Components)
	}
	if f.Hostname != "test-host" || !f.SSHUnitInstalled {
		t.Errorf("gathered facts were lost: %+v", f)
	}
}

// Doctor's Backups section comes from ~/.config/ctdev/backup, judged by the
// backup package and handed to diagnose as facts.
func TestDoctorFactsCarryBackupHealth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m, err := backup.ThisMachine(sysutil.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if got, errText := doctorBackupFacts(m); len(got) != 0 || errText != "" {
		t.Errorf("with nothing scheduled: %v, %q", got, errText)
	}

	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(home, "Projects", "website")
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(m.Dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("folders.json", `{"folders":[{"path":"`+folder+`","id":"00000000-0000-4000-8000-000000000000","schedule":"weekly","added":"2026-01-01T00:00:00Z"}]}`)
	write("state.json", `{"folders":{"`+folder+`":{"last_attempt":"2026-01-02T00:00:00Z","last_success":"2026-01-01T00:00:00Z","error":"example failure","retention_error":"example retention failure"}}}`)

	got, errText := doctorBackupFacts(m)
	want := diagnose.BackupFolder{
		Path: "~/Projects/website", Schedule: "weekly", State: diagnose.BackupFailed,
		LastSuccess: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Error: "example failure",
		RetentionError: "example retention failure",
	}
	if errText != "" || len(got) != 1 || got[0].Path != want.Path || got[0].Schedule != want.Schedule ||
		got[0].State != want.State || !got[0].LastSuccess.Equal(want.LastSuccess) || got[0].Error != want.Error ||
		got[0].RetentionError != want.RetentionError {
		t.Errorf("facts = %+v, %q; want %+v", got, errText, want)
	}

	write("state.json", "not json")
	if _, errText := doctorBackupFacts(m); errText == "" {
		t.Error("unreadable state gave no error")
	}
}

// Each destination's data check reaches doctor from the copy the hourly
// runner keeps in state.json, for the destinations this computer has.
func TestDoctorFactsCarryTheDataCheck(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	m, err := backup.ThisMachine(sysutil.Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if got, errText := doctorDataCheckFacts(m); len(got) != 0 || errText != "" {
		t.Errorf("with nothing set up: %v, %q", got, errText)
	}
	if err := os.MkdirAll(filepath.Join(m.Dir, "destinations"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "destinations", "example-backups.env"), []byte("RESTIC_REPOSITORY=/example/repo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	state := `{"folders":{},"destinations":{` +
		`"example-backups":{"data_check":{"host":"example-desktop","started":"2026-10-20T03:00:00Z","finished":"2026-10-20T03:10:00Z","slice":3,"ok":false,"step":"check","error":"damaged pack 0123abcd"}},` +
		`"example-gone":{"data_check":{"host":"example-desktop","started":"2026-10-20T03:00:00Z","finished":"2026-10-20T03:10:00Z","slice":3,"ok":true}}}}`
	if err := os.WriteFile(filepath.Join(m.Dir, "state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	got, errText := doctorDataCheckFacts(m)
	want := diagnose.BackupDataCheck{
		Destination: "example-backups", Finished: time.Date(2026, 10, 20, 3, 10, 0, 0, time.UTC),
		Host: "example-desktop", Slice: 3, Slices: 20, Step: "check", Error: "damaged pack 0123abcd",
	}
	if errText != "" || len(got) != 1 || !got[0].Finished.Equal(want.Finished) {
		t.Fatalf("facts = %+v, %q; want only %+v", got, errText, want)
	}
	got[0].Finished = want.Finished
	if got[0] != want {
		t.Errorf("facts = %+v, want %+v", got[0], want)
	}
}

// The set-up instructions promise that setting CTDEV_UNIFI_API_KEY is enough.
// Requiring the endpoint separately made that promise false, and left the tool
// refusing to talk to a controller it had just finished identifying.
func TestUnifiEndpointDefaultsToGateway(t *testing.T) {
	t.Setenv("CTDEV_UNIFI_API_KEY", "test-key")
	flagDoctorUnifi = ""
	flagDoctorBatch()

	facts := diagnose.Facts{Gateway: netip.MustParseAddr("10.2.2.1")}
	creds := integrationCreds(context.Background(), facts)

	if got := creds.UniFi.Endpoint; got != "https://10.2.2.1" {
		t.Errorf("endpoint = %q, want it defaulted to the gateway", got)
	}
	if !creds.Any() {
		t.Error("a key plus a known gateway should be usable credentials")
	}
}

// An explicit endpoint always wins: the controller is not always the gateway.
func TestUnifiExplicitEndpointWins(t *testing.T) {
	t.Setenv("CTDEV_UNIFI_API_KEY", "test-key")
	t.Setenv("CTDEV_UNIFI_HOST", "https://10.9.9.9:8443")
	flagDoctorUnifi = ""
	flagDoctorBatch()

	creds := integrationCreds(context.Background(), diagnose.Facts{
		Gateway: netip.MustParseAddr("10.2.2.1"),
	})
	if got := creds.UniFi.Endpoint; got != "https://10.9.9.9:8443" {
		t.Errorf("endpoint = %q, want the explicitly configured controller", got)
	}
}

// Without a key there is nothing to point anywhere, and no probe should run.
func TestNoKeyMeansNoIntegration(t *testing.T) {
	t.Setenv("CTDEV_UNIFI_API_KEY", "")
	t.Setenv("CTDEV_UNIFI_HOST", "")
	flagDoctorUnifi = ""
	flagDoctorBatch()

	creds := integrationCreds(context.Background(), diagnose.Facts{
		Gateway: netip.MustParseAddr("10.2.2.1"),
	})
	if creds.Any() {
		t.Errorf("no credentials should mean no integration, got %+v", creds)
	}
}

// flagDoctorBatch keeps the one-shot prompt from blocking a test run.
func flagDoctorBatch() { flagBatch = true }

// Doctor is built to diagnose machines we do not manage, so it must never ask a
// stranger for their password on its own initiative. Root is opt-in, and the
// default is the half of that contract worth pinning.
func TestDoctorRootFlagDefaultsOff(t *testing.T) {
	f := doctorCmd.Flags().Lookup("root")
	if f == nil {
		t.Fatal("doctor has no --root flag")
	}
	if f.DefValue != "false" {
		t.Errorf("--root default = %q, want %q", f.DefValue, "false")
	}
}

package cmd

import (
	"context"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/diagnose"
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
// runner keeps in state.json, for the destinations this computer's
// scheduled folders use; with none scheduled, there are none.
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
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Dated relative to now: a check over two weeks old is overdue.
	finished := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)
	started := finished.Add(-10 * time.Minute)
	at := `"started":"` + started.Format(time.RFC3339) + `","finished":"` + finished.Format(time.RFC3339) + `"`
	state := `{"folders":{},"destinations":{` +
		`"example-backups":{"data_check":{"host":"example-desktop",` + at + `,"slice":3,"ok":false,"step":"check","error":"damaged pack 0123abcd"}},` +
		`"example-unused":{"data_check":{"host":"example-desktop",` + at + `,"slice":3,"ok":true}}}}`
	if err := os.WriteFile(filepath.Join(m.Dir, "state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, errText := doctorDataCheckFacts(m); len(got) != 0 || errText != "" {
		t.Errorf("with nothing scheduled: %v, %q; want none", got, errText)
	}
	folders := `{"folders":[{"path":"` + filepath.Join(home, "website") + `","id":"00000000-0000-4000-8000-000000000000","schedule":"daily","destination":"example-backups"}]}`
	if err := os.WriteFile(filepath.Join(m.Dir, "folders.json"), []byte(folders), 0o600); err != nil {
		t.Fatal(err)
	}
	got, errText := doctorDataCheckFacts(m)
	want := diagnose.BackupDataCheck{
		Destination: "example-backups", Finished: finished,
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
	flagDoctorBatch(t)

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
	flagDoctorBatch(t)

	creds := integrationCreds(context.Background(), diagnose.Facts{
		Gateway: netip.MustParseAddr("10.2.2.1"),
	})
	if got := creds.UniFi.Endpoint; got != "https://10.9.9.9:8443" {
		t.Errorf("endpoint = %q, want the explicitly configured controller", got)
	}
}

// Without a key there is nothing to point anywhere, and no probe should run.
func TestNoKeyMeansNoIntegration(t *testing.T) {
	for _, k := range []string{
		"CTDEV_UNIFI_API_KEY", "CTDEV_UNIFI_HOST", "CTDEV_UNIFI_USER", "CTDEV_UNIFI_PASSWORD", "CTDEV_UNIFI_SITE",
		"CTDEV_SYNOLOGY_HOST", "CTDEV_SYNOLOGY_USER", "CTDEV_SYNOLOGY_PASSWORD",
		"CTDEV_PROXMOX_HOST", "CTDEV_PROXMOX_TOKEN_ID", "CTDEV_PROXMOX_SECRET",
	} {
		t.Setenv(k, "")
	}
	flagDoctorBatch(t)

	creds := integrationCreds(context.Background(), diagnose.Facts{
		Gateway: netip.MustParseAddr("10.2.2.1"),
	})
	if creds.Any() {
		t.Errorf("no credentials should mean no integration, got %+v", creds)
	}
}

// flagDoctorBatch keeps the one-shot prompt from blocking a test run, and
// restores the package-level flags on cleanup so tests stay order-independent.
func flagDoctorBatch(t *testing.T) {
	t.Helper()
	savedBatch, savedUnifi, savedUser := flagBatch, flagDoctorUnifi, flagDoctorUnifiUser
	flagBatch = true
	flagDoctorUnifi = ""
	flagDoctorUnifiUser = ""
	t.Cleanup(func() {
		flagBatch = savedBatch
		flagDoctorUnifi = savedUnifi
		flagDoctorUnifiUser = savedUser
	})
}

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

func TestWriteDoctorReport_StdoutAndFile(t *testing.T) {
	saved := flagDoctorReport
	t.Cleanup(func() { flagDoctorReport = saved })

	rep := diagnose.Report{
		Version: "20.10.0",
		Started: time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC),
		Facts:   diagnose.Facts{Hostname: "example-host"},
	}

	// --report=- writes Markdown to stdout without creating a file.
	flagDoctorReport = reportToStdout
	out := captureStdout(t, func() {
		if err := writeDoctorReport(rep); err != nil {
			t.Errorf("writeDoctorReport(-): %v", err)
		}
	})
	if !strings.Contains(out, "# Diagnostic report — example-host") {
		t.Errorf("expected markdown report on stdout, got:\n%s", out)
	}

	// Explicit path writes Markdown to disk.
	outPath := filepath.Join(t.TempDir(), "report.md")
	flagDoctorReport = outPath
	if err := writeDoctorReport(rep); err != nil {
		t.Fatalf("writeDoctorReport(file): %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read written report: %v", err)
	}
	if !strings.Contains(string(data), "example-host") {
		t.Errorf("written report missing hostname:\n%s", data)
	}
}

func TestOnlyNetworkGroups(t *testing.T) {
	checks := []diagnose.Check{
		{ID: "net.gw", Group: diagnose.GroupNetwork},
		{ID: "dns.pub", Group: diagnose.GroupInternet},
		{ID: "hw.cpu", Group: diagnose.GroupHardware},
	}
	got := onlyNetworkGroups(checks)
	if len(got) != 2 || got[0].ID != "net.gw" || got[1].ID != "dns.pub" {
		t.Errorf("onlyNetworkGroups() = %+v, want [net.gw dns.pub]", got)
	}
}

func TestDoctorCertificatePinsArePerAppliance(t *testing.T) {
	flagDoctorBatch(t)
	t.Setenv("CTDEV_UNIFI_CERT_SHA256", "example-unifi-pin")
	t.Setenv("CTDEV_SYNOLOGY_CERT_SHA256", "example-synology-pin")
	t.Setenv("CTDEV_PROXMOX_CERT_SHA256", "example-proxmox-pin")
	creds := integrationCreds(t.Context(), diagnose.Facts{})
	if creds.UniFi.CertSHA256 != "example-unifi-pin" || creds.Synology.CertSHA256 != "example-synology-pin" || creds.Proxmox.CertSHA256 != "example-proxmox-pin" {
		t.Fatal("doctor did not pass each appliance its own certificate pin")
	}
}

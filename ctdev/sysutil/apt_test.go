package sysutil

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
)

func TestAddAPTKeyringDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := AddAPTKeyring(context.Background(), o, "https://example.com/key.gpg", "/etc/apt/keyrings/example.gpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run]") {
		t.Error("expected dry-run prefix in output")
	}
	if !strings.Contains(buf.String(), "example.com") {
		t.Error("expected URL in dry-run output")
	}
}

func TestAddAPTSourceDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	err := AddAPTSource(context.Background(), o, "deb [arch=amd64] https://example.com stable main", "example.list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run]") {
		t.Error("expected dry-run prefix")
	}
	if !strings.Contains(buf.String(), "(mode 0644)") {
		t.Errorf("source must be world-readable, got %q", buf.String())
	}
}

// Every apt-get ctdev runs must be safe with no TTY and a busy dpkg lock: a
// fresh VM's boot script runs installs while unattended-upgrades holds it.
func TestAPTGetArgv(t *testing.T) {
	unattended := "env DEBIAN_FRONTEND=noninteractive apt-get -o DPkg::Lock::Timeout=300 -o Acquire::Retries=3"
	confold := " -o Dpkg::Options::=--force-confold"
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"install", "-y", "-qq", "jq"}, unattended + confold + " install -y -qq jq"},
		{[]string{"install", "--only-upgrade", "-y", "curl"}, unattended + confold + " install --only-upgrade -y curl"},
		{[]string{"dist-upgrade", "-y"}, unattended + confold + " dist-upgrade -y"},
		{[]string{"update", "-qq"}, unattended + " update -qq"},
		{[]string{"remove", "-y", "jq"}, unattended + " remove -y jq"},
		{[]string{"autoremove", "--purge", "-y"}, unattended + " autoremove --purge -y"},
		{[]string{"clean"}, unattended + " clean"},
		{[]string{"purge", "-y", "oldpkg"}, unattended + " purge -y oldpkg"},
		{[]string{"install", "-y", "/tmp/x.deb"}, unattended + confold + " install -y /tmp/x.deb"},
	}
	for _, tt := range tests {
		if got := strings.Join(aptGetArgv(tt.args...), " "); got != tt.want {
			t.Errorf("aptGetArgv(%v)\n got %q\nwant %q", tt.args, got, tt.want)
		}
	}
}

// The package helpers go through APTGet, so they carry the same options —
// and through SudoRun, so sudo wraps `env` and the variable survives it.
func TestPackageHelpersRunUnattended(t *testing.T) {
	if platform.Detect().PackageManager != "apt" {
		t.Skip("InstallPackage only reaches apt-get on an apt system")
	}
	var buf bytes.Buffer
	o := Opts{Stdout: &buf, DryRun: true}
	ctx := context.Background()
	if err := InstallPackage(ctx, o, "jq"); err != nil {
		t.Fatal(err)
	}
	if err := APTUpdate(ctx, o); err != nil {
		t.Fatal(err)
	}
	prefix := "[dry-run] sudo env DEBIAN_FRONTEND=noninteractive apt-get "
	if IsRoot() {
		prefix = "[dry-run] env DEBIAN_FRONTEND=noninteractive apt-get "
	}
	want := prefix + "-o DPkg::Lock::Timeout=300 -o Acquire::Retries=3 -o Dpkg::Options::=--force-confold install -y -qq jq\n" +
		prefix + "-o DPkg::Lock::Timeout=300 -o Acquire::Retries=3 update -qq\n"
	if buf.String() != want {
		t.Errorf("got\n%s\nwant\n%s", buf.String(), want)
	}
}

// fakeAPTGet puts an apt-get (and a sudo, for a non-root run) first on PATH
// that runs script, and returns a function counting how often it ran.
func fakeAPTGet(t *testing.T, script string) func() int {
	t.Helper()
	bin := t.TempDir()
	calls := filepath.Join(bin, "calls")
	body := "#!/bin/sh\necho x >> '" + calls + "'\nn=$(wc -l < '" + calls + "')\n" + script
	for _, name := range []string{"sudo", "apt-get"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	delay := aptLockRetryDelay
	aptLockRetryDelay = 0
	t.Cleanup(func() { aptLockRetryDelay = delay })
	return func() int {
		data, _ := os.ReadFile(calls)
		return strings.Count(string(data), "x")
	}
}

// apt-get update ignores DPkg::Lock::Timeout and fails at once on the lists
// lock, which apt-daily holds on a VM's first boot; APTGet waits it out.
func TestAPTUpdateRetriesWhileListsLockHeld(t *testing.T) {
	calls := fakeAPTGet(t, `if [ "$n" -lt 3 ]; then
  echo "E: Could not get lock /var/lib/apt/lists/lock. It is held by process 42 (apt-get)" >&2
  exit 100
fi
`)
	var buf bytes.Buffer
	if err := APTUpdate(context.Background(), Opts{Stdout: &buf}); err != nil {
		t.Fatalf("want success once the lock is released, got %v\n%s", err, buf.String())
	}
	if got := calls(); got != 3 {
		t.Errorf("apt-get ran %d times, want 3", got)
	}
}

// An install's download takes the archives lock, which DPkg::Lock::Timeout
// doesn't cover either.
func TestAPTGetRetriesWhileArchivesLockHeld(t *testing.T) {
	calls := fakeAPTGet(t, `if [ "$n" -lt 2 ]; then
  echo "E: Could not get lock /var/cache/apt/archives/lock. It is held by process 42 (apt-get)" >&2
  exit 100
fi
`)
	if err := APTGet(context.Background(), Opts{Stdout: &bytes.Buffer{}}, "install", "-y", "jq"); err != nil {
		t.Fatalf("want success once the lock is released, got %v", err)
	}
	if got := calls(); got != 2 {
		t.Errorf("apt-get ran %d times, want 2", got)
	}
}

// Any other failure (a bad repo, no network) is reported straight away.
func TestAPTUpdateDoesNotRetryOtherFailures(t *testing.T) {
	calls := fakeAPTGet(t, `echo "E: The repository 'https://example.invalid stable Release' does not have a Release file." >&2
exit 100
`)
	if err := APTUpdate(context.Background(), Opts{Stdout: &bytes.Buffer{}}); err == nil {
		t.Fatal("want the failure returned")
	}
	if got := calls(); got != 1 {
		t.Errorf("apt-get ran %d times, want 1", got)
	}
}

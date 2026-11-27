package backup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

func TestCheckAddress_TrimsBeforeIsLocal(t *testing.T) {
	m := testMachine(t, "example-laptop")
	for _, tc := range []struct {
		in, want string
	}{
		{"  b2:example-bucket:ctdev  ", "b2:example-bucket:ctdev"},
		{"\ts3:https://s3.example.invalid/bucket/ctdev\n", "s3:https://s3.example.invalid/bucket/ctdev"},
		{"  sftp:nas.example.invalid:/srv/backups  ", "sftp:nas.example.invalid:/srv/backups"},
	} {
		got, err := m.CheckAddress(tc.in, "")
		if err != nil {
			t.Fatalf("CheckAddress(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("CheckAddress(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNearestConfig_IgnoresDirectoryNamedConfig(t *testing.T) {
	root := t.TempDir()
	// A directory named .ctdev-backup.json must not be mistaken for a config file.
	sub := filepath.Join(root, "sub")
	fakeDir := filepath.Join(sub, ConfigFileName)
	if err := os.MkdirAll(fakeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := nearestConfig(sub)
	if err != nil {
		t.Fatalf("nearestConfig: %v", err)
	}
	if got != "" {
		t.Errorf("nearestConfig matched directory %q, want empty", got)
	}

	// If a real config file exists in the parent, walking past the directory finds it.
	if err := os.WriteFile(filepath.Join(root, ConfigFileName), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = nearestConfig(sub)
	if err != nil || got != root {
		t.Errorf("nearestConfig(sub) = %q, %v; want parent %q", got, err, root)
	}
}

func TestDestinationName_RejectsTraversalAndInvalid(t *testing.T) {
	m := testMachine(t, "example-laptop")
	for _, bad := range []string{"", ".", "..", "../evil", "a/b", "-leading", "UPPER", "has space"} {
		if _, err := m.Destination(bad); err == nil {
			t.Errorf("Destination(%q) succeeded, want rejection", bad)
		}
	}
}

func TestParseSecretKey_RejectsNonCanonicalTrailingBits(t *testing.T) {
	k := newSecretKey()
	valid := k.String()
	if _, err := parseSecretKey(valid); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	// A 16-byte (128-bit) value encodes to 26 base32 chars (130 bits), leaving
	// 2 trailing padding bits that must be zero. Flip the last char to one with
	// a non-zero low bit (e.g. 'A' -> 'B', or any char -> next char in alphabet).
	last := valid[len(valid)-1]
	alt := byte('B')
	if last == 'B' {
		alt = 'D'
	}
	nonCanonical := valid[:len(valid)-1] + string(alt)
	if _, err := parseSecretKey(nonCanonical); err == nil {
		t.Errorf("parseSecretKey(%q) accepted non-canonical trailing base32 bits", nonCanonical)
	}
}

func TestTagValueAndUntagValue_RoundTrip(t *testing.T) {
	for _, s := range []string{
		"~/Projects/a,b,c",
		"~/100% done, really%2C",
		"plain-path",
		"",
	} {
		enc := tagValue(s)
		if strings.Contains(enc, ",") {
			t.Errorf("tagValue(%q) = %q still contains comma", s, enc)
		}
		if got := untagValue(enc); got != s {
			t.Errorf("untagValue(tagValue(%q)) = %q", s, got)
		}
	}
}

func TestCorruptFoldersAndStateJSONReturnErrors(t *testing.T) {
	m := testMachine(t, "example-laptop")
	if err := os.MkdirAll(m.Dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "folders.json"), []byte("{truncated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Health(); err == nil {
		t.Error("Health() succeeded on corrupt folders.json, want error")
	}

	if err := os.WriteFile(filepath.Join(m.Dir, "state.json"), []byte("{truncated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.lastRun(filepath.Join(m.Home, "Projects")); err == nil {
		t.Error("lastRun() succeeded on corrupt state.json, want error")
	}
}

func TestInstallLaunchd_RollsBackPlistOnFailedBootstrap(t *testing.T) {
	m := testMachine(t, "example-laptop")
	m.GOOS = "darwin"
	m.Exe = "/usr/local/bin/ctdev"
	m.run = func(_ context.Context, _ sysutil.Opts, name string, args ...string) error {
		if name == "launchctl" && len(args) > 0 && args[0] == "bootstrap" {
			return errors.New("bootstrap refused")
		}
		return nil
	}

	err := m.installLaunchd(context.Background())
	if err == nil {
		t.Fatal("expected error when launchctl bootstrap fails")
	}
	plist := filepath.Join(m.Home, "Library", "LaunchAgents", launchdLabel+".plist")
	if _, statErr := os.Stat(plist); !os.IsNotExist(statErr) {
		t.Errorf("failed launchctl bootstrap left plist behind at %s (err=%v)", plist, statErr)
	}
}

func TestStatusWriter_ChunkedLines(t *testing.T) {
	var seen []BackupProgress
	var kept bytes.Buffer
	w := &statusWriter{
		progress: func(p BackupProgress) { seen = append(seen, p) },
		kept:     &kept,
	}

	// Deliver one status line across two writes, followed by a summary line.
	part1 := `{"message_type":"status","percent_done":0.5,"files_done":3,`
	part2 := `"total_files":6,"current_files":["a.txt"]}` + "\n" + `{"message_type":"summary","snapshot_id":"abc"}` + "\n"
	_, _ = w.Write([]byte(part1))
	if len(seen) != 0 {
		t.Fatalf("emitted progress before newline: %+v", seen)
	}
	_, _ = w.Write([]byte(part2))
	if len(seen) != 1 || seen[0].FilesDone != 3 || seen[0].CurrentFile != "a.txt" {
		t.Errorf("chunked progress = %+v, want 1 item with FilesDone=3, CurrentFile=a.txt", seen)
	}
	if !strings.Contains(kept.String(), `"message_type":"summary"`) {
		t.Errorf("summary line missing from kept: %q", kept.String())
	}
}

func TestOversizedResticLinePreservesSummary(t *testing.T) {
	// Leading whitespace keeps this a valid status line even when split
	// immediately before its JSON object.
	prefix := bytes.Repeat([]byte(" "), maxJSONLine+1)
	status := []byte(`{"message_type":"status"}` + "\n")
	summary := []byte(`{"message_type":"summary","snapshot_id":"abc"}`)
	stream := bytes.Join([][]byte{prefix, status, summary}, nil)
	for _, split := range []int{0, maxJSONLine, len(prefix)} {
		var kept bytes.Buffer
		w := statusWriter{kept: &kept, progress: func(BackupProgress) {
			t.Errorf("split %d: parsed part of an oversized line", split)
		}}
		w.Write(stream[:split])
		w.Write(stream[split:])
		w.flush()
		if !bytes.Equal(kept.Bytes(), stream) {
			t.Errorf("split %d: oversized line or summary changed", split)
		}
		snap, err := backupSummary(kept.Bytes())
		if err != nil || snap.ID != "abc" {
			t.Errorf("split %d: snapshot ID %q, error %v", split, snap.ID, err)
		}
	}
}

func TestResticMessage_JSONAndPlain(t *testing.T) {
	fallback := errors.New("exit status 1")
	if got := resticMessage([]byte(`{"message_type":"exit_error","message":"repo locked"}`+"\n"), fallback); got != "repo locked" {
		t.Errorf("JSON exit_error = %q, want repo locked", got)
	}
	if got := resticMessage([]byte("line1\nFatal: disk full\n"), fallback); got != "Fatal: disk full" {
		t.Errorf("plain stderr = %q, want Fatal: disk full", got)
	}
	if got := resticMessage(nil, fallback); got != "exit status 1" {
		t.Errorf("empty stderr = %q, want fallback error", got)
	}
}

func TestScrubbedEnv_StripsInheritedSecrets(t *testing.T) {
	m := testMachine(t, "example-laptop")
	d := &Destination{Name: "ex", Address: "/tmp/repo", m: m}
	for _, k := range scrubbedEnv {
		t.Setenv(k, "inherited-poison-"+k)
	}
	env := d.env("my-pass", "hint1")
	got := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}
	for _, k := range scrubbedEnv {
		if strings.HasPrefix(got[k], "inherited-poison-") {
			t.Errorf("inherited %s was not scrubbed: %q", k, got[k])
		}
	}
	if got["RESTIC_PASSWORD"] != "my-pass" || got["RESTIC_REPOSITORY"] != "/tmp/repo" || got["RESTIC_KEY_HINT"] != "hint1" {
		t.Errorf("expected ctdev vars set last, got %v", got)
	}
}

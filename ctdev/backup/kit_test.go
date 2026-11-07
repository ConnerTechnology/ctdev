package backup

import (
	"regexp"
	"strings"
	"testing"
)

func TestSecretKeyFormatAndParse(t *testing.T) {
	k := newSecretKey()
	s := k.String()
	if !regexp.MustCompile(`^CT1-[A-Z2-7]{6}(-[A-Z2-7]{5}){4}$`).MatchString(s) {
		t.Fatalf("Secret Key %q isn't CT1- plus groups", s)
	}
	// Typed back in lower case, with spaces instead of dashes, it's the same key.
	typed := strings.ToLower(strings.ReplaceAll(s, "-", " "))
	got, err := parseSecretKey(typed)
	if err != nil {
		t.Fatal(err)
	}
	if got != k {
		t.Errorf("parse(%q) = %s, want %s", typed, got, s)
	}
	for _, bad := range []string{"", "CT1-ABC", "XX1-" + s[4:], s + "A"} {
		if _, err := parseSecretKey(bad); err == nil {
			t.Errorf("parse(%q): want an error", bad)
		}
	}
}

func TestAccountPasswordNeedsBothSecrets(t *testing.T) {
	a, b := newSecretKey(), newSecretKey()
	p := accountPassword("example master", a)
	if p != accountPassword("example master", a) {
		t.Fatal("the account password must be deterministic")
	}
	if p == accountPassword("example master", b) || p == accountPassword("other master", a) {
		t.Error("changing either secret must change the account password")
	}
	if strings.Contains(p, "example master") {
		t.Error("the account password must not contain the master password")
	}
}

// The expected value came from libsodium (PyNaCl's argon2id.kdf with
// opslimit 3 and memlimit 64 MiB), not from this code, so the derivation is
// pinned to the standard Argon2id and to these parameters.
func TestAccountPasswordKnownAnswer(t *testing.T) {
	var k secretKey
	for i := range k {
		k[i] = byte(i)
	}
	const want = "513bf0e0b88fc0e18a848f9421a28f5d70e8a9a15a36fbf32871cbe16d936845"
	if got := accountPassword("correct horse example staple", k); got != want {
		t.Errorf("accountPassword = %s, want %s", got, want)
	}
}

func TestKitRoundTrip(t *testing.T) {
	k := newSecretKey()
	kit := Kit{
		Destination: "example-backups",
		Address:     "b2:example-bucket:website",
		Credentials: map[string]string{"B2_ACCOUNT_ID": "example-id", "B2_ACCOUNT_KEY": "example-key"},
		SecretKey:   k.String(),
	}
	text := kit.String()
	if !strings.Contains(text, "Master password:") {
		t.Errorf("kit has no line for the master password:\n%s", text)
	}
	got, err := ParseKit(text)
	if err != nil {
		t.Fatal(err)
	}
	if got.Destination != kit.Destination || got.Address != kit.Address || got.SecretKey != kit.SecretKey ||
		got.Credentials["B2_ACCOUNT_ID"] != "example-id" || got.Credentials["B2_ACCOUNT_KEY"] != "example-key" {
		t.Errorf("ParseKit = %+v, want %+v", got, kit)
	}

	// A pasted kit may come with Windows line endings and indentation.
	pasted := "  " + strings.ReplaceAll(text, "\n", "\r\n  ")
	if got, err := ParseKit(pasted); err != nil || got.Address != kit.Address {
		t.Errorf("pasted kit: %+v, %v", got, err)
	}

	if _, err := ParseKit("Destination: example-backups\n"); err == nil {
		t.Error("a kit without an address or Secret Key must be rejected")
	}
}

// A kit's credential lines are an allowlist: only the names its backend
// uses. Anything else, such as a variable restic or the loader would act on,
// refuses the whole kit and names the line (CTD-130).
func TestParseKitAcceptsOnlyTheBackendsCredentials(t *testing.T) {
	sk := newSecretKey().String()
	b2 := Kit{Destination: "example-backups", Address: "b2:example-bucket:website",
		Credentials: map[string]string{"B2_ACCOUNT_ID": "example-id", "B2_ACCOUNT_KEY": "example-key"}, SecretKey: sk}.String()
	local := Kit{Destination: "example-backups", Address: "/srv/example-backups", SecretKey: sk}.String()
	for name, tc := range map[string]struct{ kit, line string }{
		"a password command":          {local, "RESTIC_PASSWORD_COMMAND: sh -c 'printf %s example'"},
		"a preloaded library":         {b2, "LD_PRELOAD: /tmp/example.so"},
		"another backend's":           {b2, "AWS_ACCESS_KEY_ID: example-id"},
		"restic's repository":         {b2, "RESTIC_REPOSITORY: /srv/elsewhere"},
		"credentials on a local path": {local, "B2_ACCOUNT_ID: example-id"},
	} {
		label, _, _ := strings.Cut(tc.line, ":")
		_, err := ParseKit(tc.kit + tc.line + "\n")
		if err == nil || !strings.Contains(err.Error(), label) {
			t.Errorf("%s: want the kit refused, naming %s; got %v", name, label, err)
		}
	}
	if _, err := ParseKit(b2); err != nil {
		t.Errorf("a well-formed B2 kit: %v", err)
	}
	if _, err := ParseKit(local); err != nil {
		t.Errorf("a well-formed local kit: %v", err)
	}
}

// restic's environment holds only the backend's credentials from the
// destination, never a RESTIC_ name or anything else, and ctdev's own
// RESTIC_ variables come last, so nothing earlier overrides them (CTD-130).
func TestResticEnvironmentTakesOnlyTheBackendsCredentials(t *testing.T) {
	t.Setenv("RESTIC_PASSWORD_COMMAND", "sh -c 'printf %s inherited'")
	d := &Destination{Address: "b2:example-bucket:website", m: &Machine{}, credentials: map[string]string{
		"B2_ACCOUNT_ID": "example-id", "B2_ACCOUNT_KEY": "example-key",
		"RESTIC_PASSWORD_COMMAND": "sh -c 'printf %s forged'", "RESTIC_PASSWORD": "forged",
		"RESTIC_REPOSITORY": "/srv/elsewhere", "LD_PRELOAD": "/tmp/example.so",
	}}
	env := d.env("example-password", "")
	at := map[string]int{}
	for i, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "RESTIC_PASSWORD_COMMAND", "LD_PRELOAD":
			t.Errorf("restic's environment has %s", kv)
		case "RESTIC_PASSWORD":
			if v != "example-password" {
				t.Errorf("RESTIC_PASSWORD = %q", v)
			}
		case "RESTIC_REPOSITORY":
			if v != d.Address {
				t.Errorf("RESTIC_REPOSITORY = %q", v)
			}
		}
		at[k] = i
	}
	for _, k := range []string{"B2_ACCOUNT_ID", "B2_ACCOUNT_KEY", "RESTIC_REPOSITORY", "RESTIC_PASSWORD"} {
		if _, ok := at[k]; !ok {
			t.Errorf("restic's environment has no %s", k)
		}
	}
	for _, k := range []string{"RESTIC_REPOSITORY", "RESTIC_PASSWORD"} {
		if at[k] < at["B2_ACCOUNT_ID"] || at[k] < at["B2_ACCOUNT_KEY"] {
			t.Errorf("%s comes before the credentials: %v", k, env)
		}
	}
	if n := strings.Count(strings.Join(env, "\n"), "RESTIC_PASSWORD="); n != 1 {
		t.Errorf("RESTIC_PASSWORD is set %d times", n)
	}
}

func TestBackendAddresses(t *testing.T) {
	for _, tc := range []struct {
		addr, label string
		creds       []string
	}{
		{B2Address("example-bucket", "website"), "Backblaze B2", []string{"B2_ACCOUNT_ID", "B2_ACCOUNT_KEY"}},
		{S3Address("s3.amazonaws.com", "example-bucket", "website"), "S3", []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}},
		{SFTPAddress("backup@nas.example.invalid", "/srv/backups"), "SFTP", nil},
		{"/media/example/usb/backups", "local path", nil},
	} {
		if got := BackendLabel(tc.addr); got != tc.label {
			t.Errorf("BackendLabel(%q) = %q, want %q", tc.addr, got, tc.label)
		}
		if got := CredentialNames(tc.addr); strings.Join(got, ",") != strings.Join(tc.creds, ",") {
			t.Errorf("CredentialNames(%q) = %v, want %v", tc.addr, got, tc.creds)
		}
	}
	if got := B2Address("example-bucket", "website"); got != "b2:example-bucket:website" {
		t.Errorf("B2Address = %q", got)
	}
	if got := S3Address("s3.example.invalid", "example-bucket", "website"); got != "s3:https://s3.example.invalid/example-bucket/website" {
		t.Errorf("S3Address = %q", got)
	}
	if got := SFTPAddress("backup@nas.example.invalid", "/srv/backups"); got != "sftp:backup@nas.example.invalid:/srv/backups" {
		t.Errorf("SFTPAddress = %q", got)
	}
}

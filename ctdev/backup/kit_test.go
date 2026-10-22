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

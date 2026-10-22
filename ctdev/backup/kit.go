package backup

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/crypto/argon2"
)

// The two-secret model is ADR-0016: a destination's account key is a restic
// key whose password is Argon2id(master password, Secret Key), so neither
// secret alone opens the repository and a stolen bucket can't be guessed
// offline without the Secret Key.

// secretKeyPrefix versions the Secret Key format, the way 1Password's "A3-"
// does, so a later format can be told apart.
const secretKeyPrefix = "CT1-"

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// secretKey is 128 random bits.
type secretKey [16]byte

func newSecretKey() secretKey {
	var k secretKey
	_, _ = rand.Read(k[:])
	return k
}

// String is CT1- plus the key in base32, in groups of 6, 5, 5, 5 and 5.
func (k secretKey) String() string {
	s := b32.EncodeToString(k[:]) // 26 characters
	return secretKeyPrefix + s[:6] + "-" + s[6:11] + "-" + s[11:16] + "-" + s[16:21] + "-" + s[21:]
}

// parseSecretKey reads a Secret Key as a person might type it back: any case,
// with dashes or spaces between the groups.
func parseSecretKey(s string) (secretKey, error) {
	var k secretKey
	clean := strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
	body, ok := strings.CutPrefix(clean, strings.TrimSuffix(secretKeyPrefix, "-"))
	if !ok {
		return k, errors.New("a Secret Key starts with " + secretKeyPrefix)
	}
	b, err := b32.DecodeString(body)
	if err != nil || len(b) != len(k) {
		return k, errors.New("that Secret Key isn't complete: check it against the Emergency Kit")
	}
	copy(k[:], b)
	// Reject trailing bits that don't round-trip, so a typo in the last
	// character isn't silently accepted as a different key.
	if strings.ReplaceAll(strings.TrimPrefix(k.String(), secretKeyPrefix), "-", "") != body {
		return k, errors.New("that Secret Key isn't valid: check it against the Emergency Kit")
	}
	return k, nil
}

// Argon2id parameters for the account key: RFC 9106's second recommended
// option (t=3, 64 MiB), which keeps a guess slow without stalling a laptop.
// One lane rather than the RFC's four, because libsodium fixes parallelism at
// one: any libsodium-based tool can then rederive the key without ctdev.
const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 1
	argonKeyLen  = 32
)

// accountPassword is the restic password of the account key.
func accountPassword(master string, k secretKey) string {
	return hex.EncodeToString(argon2.IDKey([]byte(master), k[:], argonTime, argonMemory, argonThreads, argonKeyLen))
}

// randomPassword is a restic password for a computer key: 256 random bits.
func randomPassword() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// newRecoveryCode is the Recovery Code: 160 random bits shown as four groups
// of eight base32 characters, which is also the restic password of its key.
func newRecoveryCode() string {
	b := make([]byte, 20)
	_, _ = rand.Read(b)
	s := b32.EncodeToString(b) // 32 characters
	return s[:8] + "-" + s[8:16] + "-" + s[16:24] + "-" + s[24:]
}

// Kit is the Emergency Kit: everything but the master password that a new
// computer needs to open the destination.
type Kit struct {
	Destination string
	Address     string
	Credentials map[string]string
	SecretKey   string
}

const (
	kitTitle        = "ctdev Emergency Kit"
	kitDestination  = "Destination"
	kitAddress      = "Address"
	kitSecretKey    = "Secret Key"
	kitMasterPrompt = "Master password"
)

// String renders the kit as text to save in a password manager or print. The
// master password line is left blank to be written in by hand.
func (k Kit) String() string {
	var b strings.Builder
	b.WriteString(kitTitle + "\n")
	row := func(label, value string) {
		fmt.Fprintf(&b, "%-22s %s\n", label+":", value)
	}
	row(kitDestination, k.Destination)
	row(kitAddress, k.Address)
	names := make([]string, 0, len(k.Credentials))
	for name := range k.Credentials {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		row(name, k.Credentials[name])
	}
	row(kitSecretKey, k.SecretKey)
	row(kitMasterPrompt, "______________________________")
	return b.String()
}

var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// ParseKit reads a kit pasted back in. It tolerates indentation, Windows line
// endings and the title line, and ignores the master password line.
func ParseKit(text string) (Kit, error) {
	k := Kit{Credentials: map[string]string{}}
	for _, line := range strings.Split(text, "\n") {
		label, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		label, value = strings.TrimSpace(label), strings.TrimSpace(value)
		switch {
		case label == kitDestination:
			k.Destination = value
		case label == kitAddress:
			k.Address = value
		case label == kitSecretKey:
			k.SecretKey = value
		case envName.MatchString(label):
			k.Credentials[label] = value
		}
	}
	switch {
	case k.Destination == "":
		return k, errors.New("the kit has no Destination line")
	case k.Address == "":
		return k, errors.New("the kit has no Address line")
	case k.SecretKey == "":
		return k, errors.New("the kit has no Secret Key line")
	}
	if _, err := parseSecretKey(k.SecretKey); err != nil {
		return k, err
	}
	for _, name := range CredentialNames(k.Address) {
		if k.Credentials[name] == "" {
			return k, fmt.Errorf("the kit has no %s line", name)
		}
	}
	return k, nil
}

// B2Address is the restic repository for a Backblaze B2 bucket and path.
func B2Address(bucket, path string) string {
	return "b2:" + bucket + ":" + path
}

// S3Address is the restic repository for an S3 or S3-compatible bucket. The
// endpoint is a host (https is assumed) or a full URL.
func S3Address(endpoint, bucket, path string) string {
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	addr := "s3:" + strings.TrimSuffix(endpoint, "/") + "/" + bucket
	if path != "" {
		addr += "/" + strings.Trim(path, "/")
	}
	return addr
}

// SFTPAddress is the restic repository on a server reached over SSH, which
// authenticates with the user's own SSH keys.
func SFTPAddress(userHost, path string) string {
	return "sftp:" + userHost + ":" + path
}

// BackendLabel names the kind of destination an address is, for status.
func BackendLabel(address string) string {
	switch {
	case strings.HasPrefix(address, "b2:"):
		return "Backblaze B2"
	case strings.HasPrefix(address, "s3:"):
		return "S3"
	case strings.HasPrefix(address, "sftp:"):
		return "SFTP"
	default:
		return "local path"
	}
}

// CredentialNames lists the environment variables restic needs for an
// address's backend.
func CredentialNames(address string) []string {
	switch {
	case strings.HasPrefix(address, "b2:"):
		return []string{"B2_ACCOUNT_ID", "B2_ACCOUNT_KEY"}
	case strings.HasPrefix(address, "s3:"):
		return []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY"}
	default:
		return nil
	}
}

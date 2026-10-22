package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ConnerTechnology/ctdev/ctdev/secretfile"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Machine is this computer as backup sees it: its config dir, its home (for
// ~/ paths), and the hostname its key and backups are labeled with.
type Machine struct {
	Dir      string // ~/.config/ctdev/backup
	Home     string
	Hostname string
	Username string
	Restic   string // the restic binary; "" means restic on PATH
	Opts     sysutil.Opts
}

// ThisMachine is the machine ctdev is running on, as the current user.
func ThisMachine(o sysutil.Opts) (*Machine, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	name := os.Getenv("USER")
	if u, err := user.Current(); err == nil {
		name = u.Username
	}
	return &Machine{
		Dir:      filepath.Join(home, ".config", "ctdev", "backup"),
		Home:     home,
		Hostname: host,
		Username: name,
		Opts:     o,
	}, nil
}

// resticBin is the restic to run: the first of resticCandidates that is
// there, else restic on PATH. A scheduled run (systemd, launchd) doesn't get
// the user's shell PATH, so the usual places are tried by name first.
func (m *Machine) resticBin() string {
	if m.Restic != "" {
		return m.Restic
	}
	for _, p := range resticCandidates(runtime.GOOS, m.Home) {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() && fi.Mode()&0o111 != 0 {
			return p
		}
	}
	return "restic"
}

// resticCandidates are where restic is looked for before PATH:
// ~/.local/bin, where `ctdev install restic` puts it on Linux, and on macOS
// Homebrew's prefixes for Apple silicon and Intel.
func resticCandidates(goos, home string) []string {
	var c []string
	if home != "" {
		c = append(c, filepath.Join(home, ".local", "bin", "restic"))
	}
	if goos == "darwin" {
		c = append(c, "/opt/homebrew/bin/restic", "/usr/local/bin/restic")
	}
	return c
}

func (m *Machine) out() io.Writer {
	if m.Opts.Stdout != nil {
		return m.Opts.Stdout
	}
	return os.Stdout
}

// Destination is a restic repository this machine can open with its own
// computer key.
type Destination struct {
	Name    string
	Address string

	m             *Machine
	credentials   map[string]string
	secretKey     string
	computerKey   string
	computerKeyID string
}

// Backend names the kind of destination: Backblaze B2, S3, SFTP or a local path.
func (d *Destination) Backend() string { return BackendLabel(d.Address) }

// NewDestination is what creating a destination takes, besides the master password.
type NewDestination struct {
	Name        string
	Address     string            // the restic repository: a path, b2:…, s3:… or sftp:…
	Credentials map[string]string // the backend's, by environment variable name
}

// Setup is a newly created destination and the two things shown once: the
// Emergency Kit and the Recovery Code.
type Setup struct {
	Destination  *Destination
	Kit          string
	RecoveryCode string
}

// ErrDryRun means a step that can't be previewed was skipped.
var ErrDryRun = errors.New("dry run: destinations aren't created or connected")

var destinationName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Key labels. restic records a user and host on every key; ctdev uses them
// to tell its keys apart, since restic gives every key the same power.
const (
	accountUser  = "ctdev-account"
	recoveryUser = "ctdev-recovery"
	ctdevHost    = "ctdev"
)

// CreateDestination initializes a new restic repository with three keys
// (ADR-0016): the account key, whose password is derived from the master
// password and a new Secret Key and never stored; this computer's random key,
// labeled with its hostname; and the Recovery Code's key. It saves the
// destination for this machine and returns the kit and Recovery Code to show.
func (m *Machine) CreateDestination(ctx context.Context, nd NewDestination, master string) (Setup, error) {
	if m.Opts.DryRun {
		return Setup{}, ErrDryRun
	}
	if err := m.checkNewName(nd.Name); err != nil {
		return Setup{}, err
	}
	if master == "" {
		return Setup{}, errors.New("the master password can't be empty")
	}
	address, err := m.CheckNewAddress(nd.Address, "")
	if err != nil {
		return Setup{}, err
	}
	sk := newSecretKey()
	d := &Destination{
		Name: nd.Name, Address: address, m: m,
		credentials: nd.Credentials, secretKey: sk.String(), computerKey: randomPassword(),
	}
	// Note what was at a local path before, so a failure removes only what
	// restic made there.
	var site localSite
	if isLocal(address) {
		site = inspectLocal(address)
	}

	// Find restic first, so a missing one isn't mistaken for a failed init.
	if _, err := checkRestic(ctx, m.resticBin()); err != nil {
		return Setup{}, err
	}
	// restic init labels its first key with whoever ran it. Init with a
	// throwaway key, add the three labeled keys, then remove the throwaway.
	initial := randomPassword()
	if _, err := d.restic(ctx, run{password: initial, args: []string{"init", "--json"}}); err != nil {
		if strings.Contains(err.Error(), "already") {
			return Setup{}, fmt.Errorf("%s already holds a restic repository: connect to it with its Emergency Kit, or choose another address", address)
		}
		return Setup{}, abandon(d, site, fmt.Errorf("create the repository at %s: %w", address, err))
	}
	s, err := m.finishCreate(ctx, d, master, sk, initial)
	if err != nil {
		// Nobody has a kit for this repository yet, so it must not be left
		// behind, including after Ctrl-C.
		return Setup{}, abandon(d, site, err)
	}
	return s, nil
}

// abandon cleans up after a creation that failed past restic init. A local
// repository is removed as far as inspectLocal says is safe; one at a remote
// backend can't be removed from here, so the error says what to delete.
func abandon(d *Destination, site localSite, err error) error {
	if !isLocal(d.Address) {
		return fmt.Errorf("%w; the repository at %s was left half-created: delete it at the provider before trying again", err, d.Address)
	}
	return site.cleanup(err)
}

// finishCreate adds the three keys to a just-initialized repository, removes
// the throwaway key, and saves the destination.
func (m *Machine) finishCreate(ctx context.Context, d *Destination, master string, sk secretKey, initial string) (Setup, error) {
	recovery := newRecoveryCode()
	account := accountPassword(master, sk)
	accountID, err := d.addKey(ctx, initial, accountUser, ctdevHost, account)
	if err != nil {
		return Setup{}, err
	}
	if _, err := d.addKey(ctx, initial, recoveryUser, ctdevHost, recovery); err != nil {
		return Setup{}, err
	}
	if d.computerKeyID, err = d.addKey(ctx, initial, m.Username, m.Hostname, d.computerKey); err != nil {
		return Setup{}, err
	}
	keys, err := d.keys(ctx, initial)
	if err != nil {
		return Setup{}, err
	}
	for _, k := range keys {
		if k.Current {
			if _, err := d.restic(ctx, run{password: account, keyHint: accountID, args: []string{"key", "remove", "--retry-lock", retryLock, k.ID}}); err != nil {
				return Setup{}, fmt.Errorf("remove the setup key: %w", err)
			}
		}
	}

	if err := m.save(d); err != nil {
		return Setup{}, err
	}
	kit := Kit{Destination: d.Name, Address: d.Address, Credentials: d.credentials, SecretKey: d.secretKey}
	return Setup{Destination: d, Kit: kit.String(), RecoveryCode: recovery}, nil
}

// ConnectDestination sets this machine up on an existing destination from its
// Emergency Kit and the master password. It opens the repository with the
// account key first, so a wrong password or kit saves nothing, then adds this
// computer's own key, labeled with its hostname.
func (m *Machine) ConnectDestination(ctx context.Context, kitText, master string) (*Destination, error) {
	if m.Opts.DryRun {
		return nil, ErrDryRun
	}
	kit, err := ParseKit(kitText)
	if err != nil {
		return nil, err
	}
	if err := m.checkNewName(kit.Destination); err != nil {
		return nil, err
	}
	sk, err := parseSecretKey(kit.SecretKey)
	if err != nil {
		return nil, err
	}
	address, err := m.CheckAddress(kit.Address, "")
	if err != nil {
		return nil, err
	}
	d := &Destination{
		Name: kit.Destination, Address: address, m: m,
		credentials: kit.Credentials, secretKey: sk.String(), computerKey: randomPassword(),
	}
	account := accountPassword(master, sk)
	if _, err := d.keys(ctx, account); err != nil {
		return nil, err
	}
	if d.computerKeyID, err = d.addKey(ctx, account, m.Username, m.Hostname, d.computerKey); err != nil {
		return nil, err
	}
	if err := m.save(d); err != nil {
		// The key would open the repository with a password nobody kept.
		// restic won't remove the key a run opened with, so use the account key.
		rm := run{password: account, args: []string{"key", "remove", "--retry-lock", retryLock, d.computerKeyID}}
		if _, rerr := d.restic(context.WithoutCancel(ctx), rm); rerr != nil {
			return nil, fmt.Errorf("%w; removing the new key %s from the destination also failed: %v", err, short(d.computerKeyID), rerr)
		}
		return nil, err
	}
	return d, nil
}

func (m *Machine) checkNewName(name string) error {
	if !destinationName.MatchString(name) {
		return fmt.Errorf("%q can't be a destination name: use lower-case letters, digits, dots, dashes and underscores", name)
	}
	if _, err := os.Stat(m.destinationFile(name)); err == nil {
		return fmt.Errorf("this computer already has a destination named %s", name)
	}
	return nil
}

// Destinations lists the destinations this machine is set up on, by name.
func (m *Machine) Destinations() ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(m.Dir, "destinations"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".env"); ok && !e.IsDir() && destinationName.MatchString(name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// Destination opens a destination this machine is set up on.
func (m *Machine) Destination(name string) (*Destination, error) {
	if !destinationName.MatchString(name) {
		return nil, fmt.Errorf("%q isn't a destination name", name)
	}
	data, err := os.ReadFile(m.destinationFile(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("destination %s isn't set up on this computer", name)
	}
	if err != nil {
		return nil, err
	}
	vals := secretfile.Parse(data)
	d := &Destination{
		Name: name, Address: vals["RESTIC_REPOSITORY"], m: m,
		credentials: map[string]string{},
		secretKey:   vals["CTDEV_SECRET_KEY"], computerKey: vals["CTDEV_COMPUTER_KEY"],
		computerKeyID: vals["CTDEV_COMPUTER_KEY_ID"],
	}
	for _, k := range CredentialNames(d.Address) {
		d.credentials[k] = vals[k]
	}
	if d.Address == "" || d.computerKey == "" {
		return nil, fmt.Errorf("%s is incomplete", m.destinationFile(name))
	}
	return d, nil
}

func (m *Machine) destinationFile(name string) string {
	return filepath.Join(m.Dir, "destinations", name+".env")
}

// save writes the destination's file, owner-only: it holds the backend
// credentials, the Secret Key and this computer's key, never the master
// password or the account key.
func (m *Machine) save(d *Destination) error {
	var b strings.Builder
	b.WriteString("# ctdev backup destination " + d.Name + ". Secret: keep this file private.\n")
	fmt.Fprintf(&b, "RESTIC_REPOSITORY=%s\n", d.Address)
	names := make([]string, 0, len(d.credentials))
	for k := range d.credentials {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		fmt.Fprintf(&b, "%s=%s\n", k, d.credentials[k])
	}
	fmt.Fprintf(&b, "CTDEV_SECRET_KEY=%s\n", d.secretKey)
	fmt.Fprintf(&b, "CTDEV_COMPUTER_KEY=%s\n", d.computerKey)
	fmt.Fprintf(&b, "CTDEV_COMPUTER_KEY_ID=%s\n", d.computerKeyID)
	return secretfile.WritePrivate(m.destinationFile(d.Name), []byte(b.String()))
}

// addKey adds a restic key and returns its ID.
func (d *Destination) addKey(ctx context.Context, password, user, host, newPassword string) (string, error) {
	out, err := d.restic(ctx, run{
		password:    password,
		newPassword: newPassword,
		args:        []string{"key", "add", "--retry-lock", retryLock, "--user", user, "--host", host, "--new-password-file", "/dev/fd/3"},
	})
	if err != nil {
		return "", fmt.Errorf("add a key for %s: %w", host, err)
	}
	// restic key add has no JSON output; it prints "saved new key with ID <id>".
	m := savedKeyID.FindSubmatch(out)
	if m == nil {
		return "", fmt.Errorf("restic key add didn't report the new key's ID: %q", out)
	}
	return string(m[1]), nil
}

var savedKeyID = regexp.MustCompile(`saved new key with ID ([0-9a-f]+)`)

type resticKey struct {
	ID       string `json:"id"`
	Current  bool   `json:"current"`
	UserName string `json:"userName"`
	HostName string `json:"hostName"`
	Created  string `json:"created"`
}

func (d *Destination) keys(ctx context.Context, password string) ([]resticKey, error) {
	out, err := d.restic(ctx, run{password: password, args: []string{"key", "list", "--json"}})
	if err != nil {
		return nil, err
	}
	return parseKeys(out)
}

func parseKeys(out []byte) ([]resticKey, error) {
	var keys []resticKey
	if err := json.Unmarshal(out, &keys); err != nil {
		return nil, fmt.Errorf("read restic's key list: %w", err)
	}
	return keys, nil
}

// Computer is one computer with access to a destination.
type Computer struct {
	Hostname     string
	Added        time.Time
	ThisComputer bool
}

// Computers lists the computers that have a key on the destination, by
// hostname, leaving out the account key and the Recovery Code.
func (d *Destination) Computers(ctx context.Context) ([]Computer, error) {
	out, err := d.restic(ctx, d.asComputer("", "key", "list", "--json"))
	if err != nil {
		return nil, err
	}
	keys, err := parseKeys(out)
	if err != nil {
		return nil, err
	}
	var cs []Computer
	for _, k := range keys {
		if k.HostName == ctdevHost && (k.UserName == accountUser || k.UserName == recoveryUser) {
			continue
		}
		added, _ := time.ParseInLocation("2006-01-02 15:04:05", k.Created, time.Local)
		cs = append(cs, Computer{Hostname: k.HostName, Added: added, ThisComputer: k.Current})
	}
	sort.Slice(cs, func(i, j int) bool { return cs[i].Hostname < cs[j].Hostname })
	return cs, nil
}

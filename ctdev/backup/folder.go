// Package backup is per-folder backup: a folder configured once is backed up
// whole, with restic, to a destination this machine knows. It owns finding the
// backed-up folder, the folder's config file, this machine's state under
// ~/.config/ctdev/backup, destinations and their keys (ADR-0016), and the
// restic runs. The `ctdev backup` command is a thin layer over it.
package backup

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/secretfile"
)

// ConfigFileName marks a backed-up folder. It sits at the folder's root and is
// meant to be committed, so it never holds a secret.
const ConfigFileName = ".ctdev-backup.json"

// Schedule is how long after the last successful backup the next one is due.
type Schedule string

const (
	Hourly Schedule = "hourly"
	Daily  Schedule = "daily"
	Weekly Schedule = "weekly"
)

// Schedules lists the choices Configure offers, in menu order.
var Schedules = []Schedule{Hourly, Daily, Weekly}

// Keep is a folder's retention: how many backups to keep per period.
type Keep struct {
	Hourly  int `json:"hourly"`
	Daily   int `json:"daily"`
	Weekly  int `json:"weekly"`
	Monthly int `json:"monthly"`
	Yearly  int `json:"yearly"`
}

// DefaultKeep is the retention a new folder starts with (CTD-87: compare Time
// Machine's and Kopia's defaults).
var DefaultKeep = Keep{Hourly: 24, Daily: 7, Weekly: 4, Monthly: 12, Yearly: 3}

// Config is the folder's .ctdev-backup.json. These four fields are all of it:
// credentials and keys live in ~/.config/ctdev, never in the folder.
type Config struct {
	ID          string   `json:"id"`
	Destination string   `json:"destination"`
	Schedule    Schedule `json:"schedule"`
	Keep        Keep     `json:"keep"`
}

// NewConfig is a fresh folder config: a new random ID, Daily, default keep.
func NewConfig(destination string) Config {
	return Config{ID: newUUID(), Destination: destination, Schedule: Daily, Keep: DefaultKeep}
}

// Folder is where `ctdev backup` was run and the backed-up folder it is in.
// Root and Config are empty when nothing at or above Cwd is set up.
type Folder struct {
	Cwd    string
	Root   string
	Config *Config
	// ConfigErr is why the config file at Root can't be read. Root is still
	// the folder's, so nothing treats it as a folder that isn't backed up.
	ConfigErr error
}

// Find walks up from cwd to the nearest folder holding a .ctdev-backup.json,
// the way git finds its repository.
func Find(cwd string) (Folder, error) {
	cwd, err := filepath.Abs(cwd)
	if err != nil {
		return Folder{}, err
	}
	f := Folder{Cwd: cwd}
	root, err := nearestConfig(cwd)
	if err != nil || root == "" {
		return f, err
	}
	f.Root = root
	cfg, err := ReadConfig(root)
	if err != nil {
		f.ConfigErr = err
		return f, nil
	}
	f.Config = &cfg
	return f, nil
}

// nearestConfig returns the closest directory at or above dir that holds a
// config file, or "".
func nearestConfig(dir string) (string, error) {
	for {
		_, err := os.Stat(filepath.Join(dir, ConfigFileName))
		if err == nil {
			return dir, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// ReadConfig reads the config file at the root of a backed-up folder.
func ReadConfig(root string) (Config, error) {
	var cfg Config
	path := filepath.Join(root, ConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.validate(); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// validate checks a config read from disk. The file may be hand-edited or
// come from a clone, and its ID ends up in a restic tag, where a comma would
// split it in two.
func (c Config) validate() error {
	switch {
	case !uuidShape.MatchString(c.ID):
		return fmt.Errorf("id %q isn't a UUID", c.ID)
	case !destinationName.MatchString(c.Destination):
		return fmt.Errorf("destination %q isn't a destination name", c.Destination)
	case !slices.Contains(Schedules, c.Schedule):
		return fmt.Errorf("schedule %q isn't hourly, daily or weekly", c.Schedule)
	}
	k := c.Keep
	for _, n := range []int{k.Hourly, k.Daily, k.Weekly, k.Monthly, k.Yearly} {
		if n < 0 {
			return errors.New("keep counts can't be negative")
		}
	}
	return nil
}

// WriteConfig writes the folder's config file. It is world-readable on
// purpose: it holds no secret and is meant to be committed.
func WriteConfig(root string, cfg Config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return secretfile.WriteAtomic(filepath.Join(root, ConfigFileName), append(data, '\n'), 0o644)
}

// InsideError refuses to configure a folder that is already part of a
// backed-up folder: it would be backed up twice.
type InsideError struct {
	Root string
}

func (e *InsideError) Error() string {
	return "already backed up as part of " + e.Root
}

// CheckConfigurable says whether dir can be configured as a backed-up folder.
// It refuses, with an *InsideError, a new folder inside a backed-up one, and
// returns the backed-up folders below dir, which would then be in two folders'
// backups, so the caller can warn. A folder with its own config file is
// already a backed-up folder, so reconfiguring it is never refused, even when
// a folder above it was backed up later.
func CheckConfigurable(dir string) (contained []string, err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	_, statErr := os.Stat(filepath.Join(dir, ConfigFileName))
	if parent := filepath.Dir(dir); parent != dir && statErr != nil {
		root, err := nearestConfig(parent)
		if err != nil {
			return nil, err
		}
		if root != "" {
			return nil, &InsideError{Root: root}
		}
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable directory can't hold a config we could act on.
			if d != nil && d.IsDir() && p != dir {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() && d.Name() == ConfigFileName && filepath.Dir(p) != dir {
			contained = append(contained, filepath.Dir(p))
		}
		return nil
	})
	return contained, err
}

// DisplayPath shows p the way a person on this machine would type it, with
// the home directory as ~. Backups are tagged with this form, so a folder's
// path reads the same on every computer that shares a home layout.
func DisplayPath(home, p string) string {
	if home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + filepath.ToSlash(rest)
	}
	return p
}

// newUUID returns a random (version 4) UUID.
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

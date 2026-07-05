// Package profile defines machine profiles: named, declarative descriptions of
// what a machine should run — a component list plus the configure categories
// applied at their recommended values. `ctdev apply <name>` realizes a profile;
// `ctdev diff <name>` reports drift from it.
//
// The canonical profiles under profiles/ are embedded in the binary, so a
// freshly installed ctdev can bootstrap a machine without a repo clone. Files
// in ~/.config/ctdev/profiles/ are merged in and win on name conflicts, so a
// local variant can override a built-in without forking the repo.
package profile

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

//go:embed profiles/*.toml
var builtins embed.FS

// Profile is one machine role.
type Profile struct {
	Name        string   `toml:"-"`
	Description string   `toml:"description"`
	Components  []string `toml:"components"`
	Configure   []string `toml:"configure"`
	// Notes prints after a successful apply — the profile's own next-steps
	// runbook (interactive wizards to run, manual steps like tailscale up).
	Notes  string `toml:"notes"`
	Source string `toml:"-"` // "built-in" or the file path it was loaded from
}

// userDir is where local profile overrides live.
func userDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ctdev", "profiles")
}

func parse(name string, b []byte, source string) (*Profile, error) {
	var p Profile
	if err := toml.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("profile %s: %w", name, err)
	}
	p.Name = name
	p.Source = source
	if len(p.Components) == 0 && len(p.Configure) == 0 {
		return nil, fmt.Errorf("profile %s: no components or configure categories", name)
	}
	return &p, nil
}

// Load returns the named profile — a user file when present, else the built-in.
func Load(name string) (*Profile, error) {
	if dir := userDir(); dir != "" {
		path := filepath.Join(dir, name+".toml")
		if b, err := os.ReadFile(path); err == nil {
			return parse(name, b, path)
		}
	}
	if b, err := builtins.ReadFile("profiles/" + name + ".toml"); err == nil {
		return parse(name, b, "built-in")
	}
	names := Names()
	return nil, fmt.Errorf("unknown profile %q (available: %s)", name, strings.Join(names, ", "))
}

// List returns every available profile (built-ins merged with user files,
// user files winning on name conflicts), sorted by name. Unparseable files
// are skipped — List is for display; Load reports their errors.
func List() []Profile {
	byName := map[string]*Profile{}

	if entries, err := builtins.ReadDir("profiles"); err == nil {
		for _, e := range entries {
			name := strings.TrimSuffix(e.Name(), ".toml")
			if b, err := builtins.ReadFile("profiles/" + e.Name()); err == nil {
				if p, err := parse(name, b, "built-in"); err == nil {
					byName[name] = p
				}
			}
		}
	}
	if dir := userDir(); dir != "" {
		if entries, err := os.ReadDir(dir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.HasSuffix(e.Name(), ".toml") {
					continue
				}
				name := strings.TrimSuffix(e.Name(), ".toml")
				path := filepath.Join(dir, e.Name())
				if b, err := os.ReadFile(path); err == nil {
					if p, err := parse(name, b, path); err == nil {
						byName[name] = p
					}
				}
			}
		}
	}

	out := make([]Profile, 0, len(byName))
	for _, p := range byName {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Names returns the sorted names of every available profile.
func Names() []string {
	list := List()
	names := make([]string, len(list))
	for i, p := range list {
		names[i] = p.Name
	}
	return names
}

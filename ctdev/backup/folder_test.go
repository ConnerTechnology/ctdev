package backup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir string, cfg Config) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
}

func TestFindWalksUpToTheNearestConfig(t *testing.T) {
	root := filepath.Join(t.TempDir(), "website")
	cfg := NewConfig("example-backups")
	writeConfig(t, root, cfg)
	docs := filepath.Join(root, "docs", "drafts")
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, cwd := range []string{root, docs} {
		f, err := Find(cwd)
		if err != nil {
			t.Fatal(err)
		}
		if f.Root != root || f.Cwd != cwd {
			t.Errorf("Find(%s) = root %q cwd %q, want root %q", cwd, f.Root, f.Cwd, root)
		}
		if f.Config == nil || f.Config.ID != cfg.ID {
			t.Errorf("Find(%s) config = %+v, want ID %s", cwd, f.Config, cfg.ID)
		}
	}
}

func TestFindWithNothingSetUp(t *testing.T) {
	cwd := t.TempDir()
	f, err := Find(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if f.Root != "" || f.Config != nil {
		t.Errorf("want no backed-up folder, got root %q", f.Root)
	}
	if f.Cwd != cwd {
		t.Errorf("Cwd = %q, want %q", f.Cwd, cwd)
	}
}

func TestConfigureIsRefusedInsideABackedUpFolder(t *testing.T) {
	root := filepath.Join(t.TempDir(), "website")
	writeConfig(t, root, NewConfig("example-backups"))
	inner := filepath.Join(root, "docs")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := CheckConfigurable(inner)
	var inside *InsideError
	if !errors.As(err, &inside) {
		t.Fatalf("want an InsideError, got %v", err)
	}
	if inside.Root != root {
		t.Errorf("Root = %q, want %q", inside.Root, root)
	}

	// The backed-up folder itself can be reconfigured.
	if _, err := CheckConfigurable(root); err != nil {
		t.Errorf("reconfiguring the folder itself: %v", err)
	}
}

// A folder that was backed up before its parent was can still be
// reconfigured: it has its own config, so it isn't a new folder inside one.
func TestAnInnerBackedUpFolderCanBeReconfigured(t *testing.T) {
	outer := filepath.Join(t.TempDir(), "Projects")
	inner := filepath.Join(outer, "website")
	writeConfig(t, inner, NewConfig("example-backups"))
	writeConfig(t, outer, NewConfig("example-backups"))
	if _, err := CheckConfigurable(inner); err != nil {
		t.Errorf("reconfiguring the inner folder: %v", err)
	}
	// A new folder beside it, inside the outer one, is still refused.
	if err := os.MkdirAll(filepath.Join(outer, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	var inside *InsideError
	if _, err := CheckConfigurable(filepath.Join(outer, "notes")); !errors.As(err, &inside) {
		t.Errorf("a new folder inside: want an InsideError, got %v", err)
	}
}

func TestConfigureWarnsWhenTheFolderContainsBackedUpFolders(t *testing.T) {
	outer := t.TempDir()
	a := filepath.Join(outer, "website")
	b := filepath.Join(outer, "deep", "notes")
	writeConfig(t, a, NewConfig("example-backups"))
	writeConfig(t, b, NewConfig("example-backups"))

	contained, err := CheckConfigurable(outer)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(contained)
	if want := []string{b, a}; !reflect.DeepEqual(contained, want) {
		t.Errorf("contained = %v, want %v", contained, want)
	}
}

func TestConfigFileHoldsOnlyTheFourFields(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, NewConfig("example-backups"))
	data, err := os.ReadFile(filepath.Join(dir, ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if want := []string{"destination", "id", "keep", "schedule"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("keys = %v, want %v", keys, want)
	}
}

func TestReadConfigRejectsABadFile(t *testing.T) {
	good := `"destination": "example-backups", "schedule": "daily", "keep": {"hourly": 24, "daily": 7, "weekly": 4, "monthly": 12, "yearly": 3}`
	for name, body := range map[string]string{
		"no id":            `{` + good + `}`,
		"id not a UUID":    `{"id": "website", ` + good + `}`,
		"id with a comma":  `{"id": "0b6c2a4e-1f3d-4c5b-9a8e-7d6f5e4c3b2a,x", ` + good + `}`,
		"unknown schedule": `{"id": "0b6c2a4e-1f3d-4c5b-9a8e-7d6f5e4c3b2a", "destination": "example-backups", "schedule": "monthly", "keep": {}}`,
		"negative keep":    `{"id": "0b6c2a4e-1f3d-4c5b-9a8e-7d6f5e4c3b2a", "destination": "example-backups", "schedule": "daily", "keep": {"daily": -1}}`,
		"no destination":   `{"id": "0b6c2a4e-1f3d-4c5b-9a8e-7d6f5e4c3b2a", "schedule": "daily", "keep": {}}`,
		"not JSON":         `id = website`,
	} {
		dir := t.TempDir()
		path := filepath.Join(dir, ConfigFileName)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ReadConfig(dir)
		if err == nil || !strings.Contains(err.Error(), path) {
			t.Errorf("%s: want an error naming %s, got %v", name, path, err)
		}
	}

	dir := t.TempDir()
	ok := `{"id": "0b6c2a4e-1f3d-4c5b-9a8e-7d6f5e4c3b2a", ` + good + `}`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(ok), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadConfig(dir); err != nil {
		t.Errorf("a good file: %v", err)
	}
}

func TestNewConfigDefaults(t *testing.T) {
	cfg := NewConfig("example-backups")
	if cfg.Schedule != Daily {
		t.Errorf("Schedule = %q, want daily", cfg.Schedule)
	}
	want := Keep{Hourly: 24, Daily: 7, Weekly: 4, Monthly: 12, Yearly: 3}
	if cfg.Keep != want {
		t.Errorf("Keep = %+v, want %+v", cfg.Keep, want)
	}
	if len(cfg.ID) != 36 || cfg.ID == NewConfig("x").ID {
		t.Errorf("ID %q should be a fresh UUID", cfg.ID)
	}
}

func TestDisplayPath(t *testing.T) {
	home := "/home/example"
	for in, want := range map[string]string{
		"/home/example":                  "~",
		"/home/example/Projects/website": "~/Projects/website",
		"/home/examples":                 "/home/examples",
		"/srv/data":                      "/srv/data",
	} {
		if got := DisplayPath(home, in); got != want {
			t.Errorf("DisplayPath(%q) = %q, want %q", in, got, want)
		}
	}
}

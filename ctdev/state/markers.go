package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type InstallMarker struct {
	InstalledAt time.Time `json:"installed_at"`
	Version     string    `json:"version"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type MarkerStore struct {
	dir string
}

func NewMarkerStore(dir string) *MarkerStore {
	return &MarkerStore{dir: dir}
}

func DefaultMarkerStore() *MarkerStore {
	return NewMarkerStore(filepath.Join(StateDir(), "components"))
}

func (inst *MarkerStore) Save(name string, m InstallMarker) error {
	if err := os.MkdirAll(inst.dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	final := filepath.Join(inst.dir, name+".json")
	// Atomic write: stage in a same-directory temp file then rename. A crash
	// mid-write leaves the previous marker intact instead of an empty/corrupt
	// JSON file that would fail to Load next run.
	tmp, err := os.CreateTemp(inst.dir, name+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(0644); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, final); err != nil {
		os.Remove(tmpPath)
		return err
	}
	return nil
}

func (inst *MarkerStore) Load(name string) (InstallMarker, error) {
	var m InstallMarker
	data, err := os.ReadFile(filepath.Join(inst.dir, name+".json"))
	if err != nil {
		return m, err
	}
	err = json.Unmarshal(data, &m)
	return m, err
}

func (inst *MarkerStore) Exists(name string) bool {
	_, err := os.Stat(filepath.Join(inst.dir, name+".json"))
	return err == nil
}

func (inst *MarkerStore) Remove(name string) error {
	return os.Remove(filepath.Join(inst.dir, name+".json"))
}

func (inst *MarkerStore) List() ([]string, error) {
	entries, err := os.ReadDir(inst.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) == ".json" {
			names = append(names, strings.TrimSuffix(name, ".json"))
		}
	}
	return names, nil
}

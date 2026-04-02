package state

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	return os.WriteFile(filepath.Join(inst.dir, name+".json"), data, 0644)
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
			names = append(names, name[:len(name)-5])
		}
	}
	return names, nil
}

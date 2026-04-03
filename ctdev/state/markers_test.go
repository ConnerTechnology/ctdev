package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarkerRoundTrip(t *testing.T) {
	dir := t.TempDir()
	ms := NewMarkerStore(dir)

	now := time.Now().Truncate(time.Second)
	marker := InstallMarker{
		InstalledAt: now,
		Version:     "1.0.0",
		UpdatedAt:   now,
	}

	if err := ms.Save("docker", marker); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := ms.Load("docker")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if !got.InstalledAt.Equal(marker.InstalledAt) {
		t.Errorf("installed_at: got %v, want %v", got.InstalledAt, marker.InstalledAt)
	}
	if got.Version != "1.0.0" {
		t.Errorf("version: got %s, want 1.0.0", got.Version)
	}
}

func TestMarkerExists(t *testing.T) {
	dir := t.TempDir()
	ms := NewMarkerStore(dir)

	if ms.Exists("docker") {
		t.Error("expected docker to not exist")
	}

	now := time.Now()
	_ = ms.Save("docker", InstallMarker{InstalledAt: now, UpdatedAt: now})

	if !ms.Exists("docker") {
		t.Error("expected docker to exist")
	}
}

func TestMarkerRemove(t *testing.T) {
	dir := t.TempDir()
	ms := NewMarkerStore(dir)

	now := time.Now()
	_ = ms.Save("docker", InstallMarker{InstalledAt: now, UpdatedAt: now})
	_ = ms.Remove("docker")

	if ms.Exists("docker") {
		t.Error("expected docker to be removed")
	}
}

func TestMarkerList(t *testing.T) {
	t.Run("empty dir", func(t *testing.T) {
		dir := t.TempDir()
		ms := NewMarkerStore(dir)
		got, err := ms.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected nil/empty, got %v", got)
		}
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		ms := NewMarkerStore("/tmp/ctdev-test-nonexistent-dir-" + t.Name())
		got, err := ms.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("json files return names", func(t *testing.T) {
		dir := t.TempDir()
		ms := NewMarkerStore(dir)
		now := time.Now()
		_ = ms.Save("docker", InstallMarker{InstalledAt: now, UpdatedAt: now})
		_ = ms.Save("git", InstallMarker{InstalledAt: now, UpdatedAt: now})

		got, err := ms.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("expected 2 names, got %d: %v", len(got), got)
		}
		names := map[string]bool{got[0]: true, got[1]: true}
		if !names["docker"] || !names["git"] {
			t.Errorf("expected docker and git, got %v", got)
		}
	})

	t.Run("only json files returned", func(t *testing.T) {
		dir := t.TempDir()
		ms := NewMarkerStore(dir)
		now := time.Now()
		_ = ms.Save("docker", InstallMarker{InstalledAt: now, UpdatedAt: now})
		_ = os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0644)

		got, err := ms.List()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != "docker" {
			t.Errorf("expected [docker], got %v", got)
		}
	})
}


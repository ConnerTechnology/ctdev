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

func TestMigrateOldMarkers(t *testing.T) {
	oldDir := t.TempDir()
	newDir := t.TempDir()

	// Create old-style marker
	oldFile := filepath.Join(oldDir, "docker.installed")
	os.WriteFile(oldFile, []byte("2026-01-15T10:30:00Z"), 0644)

	ms := NewMarkerStore(newDir)
	migrated, err := MigrateOldMarkers(oldDir, ms)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if migrated != 1 {
		t.Errorf("expected 1 migrated, got %d", migrated)
	}

	got, err := ms.Load("docker")
	if err != nil {
		t.Fatalf("load after migrate: %v", err)
	}
	if got.Version != "unknown" {
		t.Errorf("version: got %s, want unknown", got.Version)
	}
}

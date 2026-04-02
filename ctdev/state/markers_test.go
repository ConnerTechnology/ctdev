package state

import (
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


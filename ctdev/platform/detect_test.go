package platform

import (
	"runtime"
	"testing"
)

func TestDetectOS(t *testing.T) {
	info := Detect()
	if runtime.GOOS == "linux" {
		if info.OS != Linux {
			t.Errorf("expected Linux, got %s", info.OS)
		}
		if info.PackageManager == "" {
			t.Error("expected package manager to be detected")
		}
	} else if runtime.GOOS == "darwin" {
		if info.OS != MacOS {
			t.Errorf("expected MacOS, got %s", info.OS)
		}
		if info.PackageManager != "brew" {
			t.Errorf("expected brew, got %s", info.PackageManager)
		}
	}
	if info.Arch == "" {
		t.Error("expected arch to be detected")
	}
}

func TestDetectArch(t *testing.T) {
	arch := detectArch()
	if arch != "amd64" && arch != "arm64" {
		t.Errorf("unexpected arch: %s", arch)
	}
}

func TestDetectReturnsSameInstance(t *testing.T) {
	a := Detect()
	b := Detect()
	if a.OS != b.OS {
		t.Errorf("expected same OS, got %s and %s", a.OS, b.OS)
	}
	if a.Arch != b.Arch {
		t.Errorf("expected same Arch, got %s and %s", a.Arch, b.Arch)
	}
	if a.Distro != b.Distro {
		t.Errorf("expected same Distro, got %s and %s", a.Distro, b.Distro)
	}
	if a.PackageManager != b.PackageManager {
		t.Errorf("expected same PackageManager, got %s and %s", a.PackageManager, b.PackageManager)
	}
}

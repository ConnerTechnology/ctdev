package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type OS string

const (
	Linux   OS = "linux"
	MacOS   OS = "macos"
	Unknown OS = "unknown"
)

type Info struct {
	OS             OS
	Distro         string // "linuxmint", "ubuntu", etc.
	Arch           string // "amd64", "arm64"
	PackageManager string // "apt", "dnf", "pacman", "brew"
	IsContainer    bool
}

func Detect() Info {
	info := Info{
		OS:   detectOS(),
		Arch: detectArch(),
	}
	info.Distro = detectDistro()
	info.PackageManager = detectPackageManager(info.OS)
	info.IsContainer = detectContainer()
	return info
}

func detectOS() OS {
	switch runtime.GOOS {
	case "linux":
		return Linux
	case "darwin":
		return MacOS
	default:
		return Unknown
	}
}

func detectArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

func detectDistro() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			return strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
	}
	return ""
}

func detectPackageManager(osType OS) string {
	if osType == MacOS {
		return "brew"
	}
	managers := []struct {
		cmd  string
		name string
	}{
		{"apt", "apt"},
		{"dnf", "dnf"},
		{"pacman", "pacman"},
	}
	for _, m := range managers {
		if _, err := exec.LookPath(m.cmd); err == nil {
			return m.name
		}
	}
	return "unknown"
}

func detectContainer() bool {
	if os.Getenv("REMOTE_CONTAINERS") != "" || os.Getenv("CODESPACES") != "" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	return false
}

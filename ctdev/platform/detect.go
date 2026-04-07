package platform

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
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
	Codename       string // "noble", "jammy", etc. (UBUNTU_CODENAME or VERSION_CODENAME)
	Arch           string // "amd64", "arm64"
	PackageManager string // "apt", "dnf", "pacman", "brew"
	IsContainer    bool
}

var (
	cachedInfo Info
	detectOnce sync.Once
)

func Detect() Info {
	detectOnce.Do(func() {
		cachedInfo = detect()
	})
	return cachedInfo
}

func detect() Info {
	info := Info{
		OS:   detectOS(),
		Arch: detectArch(),
	}
	info.Distro, info.Codename = detectDistro()
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
	return runtime.GOARCH
}

func detectDistro() (string, string) {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", ""
	}
	var id, codename, versionCodename string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "ID=") {
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "UBUNTU_CODENAME=") {
			codename = strings.Trim(strings.TrimPrefix(line, "UBUNTU_CODENAME="), "\"")
		}
		if strings.HasPrefix(line, "VERSION_CODENAME=") {
			versionCodename = strings.Trim(strings.TrimPrefix(line, "VERSION_CODENAME="), "\"")
		}
	}
	if codename == "" {
		codename = versionCodename
	}
	return id, codename
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

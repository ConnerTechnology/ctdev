package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type SystemInfo struct {
	Platform    Info
	Hostname    string
	Shell       string
	DotfilesDir string
	CPUModel    string
	CPUThreads  int
	MemoryGB    int
}

func GatherSystemInfo(dotfilesDir string) SystemInfo {
	info := SystemInfo{
		Platform:    Detect(),
		DotfilesDir: dotfilesDir,
		CPUThreads:  runtime.NumCPU(),
	}
	info.Hostname, _ = os.Hostname()
	info.Shell = filepath.Base(os.Getenv("SHELL"))
	info.CPUModel = readCPUModel()
	info.MemoryGB = readMemoryGB()
	return info
}

func readCPUModel() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err != nil {
			return "unknown"
		}
		return strings.TrimSpace(string(out))
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "model name") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

func readMemoryGB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		var bytes int
		fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &bytes)
		return snapToStandardSize(bytes / (1024 * 1024 * 1024))
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			var kb int
			fmt.Sscanf(line, "MemTotal: %d kB", &kb)
			return snapToStandardSize(kb / (1024 * 1024))
		}
	}
	return 0
}

func snapToStandardSize(gb int) int {
	sizes := []int{4, 8, 16, 32, 64, 128, 256}
	for _, s := range sizes {
		if gb <= s {
			return s
		}
	}
	return gb
}

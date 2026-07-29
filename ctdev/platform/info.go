package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type GPUInfo struct {
	Name   string
	Driver string
}

type NetworkAdapter struct {
	Name      string
	Type      string // "ethernet", "wifi"
	Interface string
}

type SystemInfo struct {
	Platform    Info
	Hostname    string
	Shell       string
	DotfilesDir string
	CPUModel    string
	CPUThreads  int
	MemoryGB    int
	// MemUsedKB/MemTotalKB are live consumption, distinct from MemoryGB (the
	// installed capacity). Both are 0 where /proc/meminfo isn't available.
	MemUsedKB  int64
	MemTotalKB int64
	Uptime     time.Duration // 0 where /proc/uptime isn't available
	Kernel     string        // uname -r
	GPUs       []GPUInfo
	Network    []NetworkAdapter
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
	info.MemUsedKB, info.MemTotalKB = readMemUsageKB()
	info.Uptime = readUptime()
	info.Kernel = readKernel()
	info.GPUs = readGPUs()
	info.Network = readNetworkAdapters()
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
	if m := parseCPUModel(string(data)); m != "" {
		return m
	}
	// Generic ARM fallback: the device-tree board name (NUL-terminated).
	if b, err := os.ReadFile("/proc/device-tree/model"); err == nil {
		if m := strings.TrimSpace(strings.Trim(string(b), "\x00")); m != "" {
			return m
		}
	}
	return "unknown"
}

// parseCPUModel extracts a CPU/board name from /proc/cpuinfo. x86 kernels give
// a per-core "model name"; arm64 kernels don't, but Raspberry Pi kernels append
// a board "Model" line (e.g. "Raspberry Pi 5 Model B Rev 1.0") — without this
// fallback a Pi reports "unknown".
func parseCPUModel(cpuinfo string) string {
	boardModel := ""
	for _, line := range strings.Split(cpuinfo, "\n") {
		if strings.HasPrefix(line, "model name") {
			if _, v, ok := strings.Cut(line, ":"); ok {
				return strings.TrimSpace(v)
			}
		}
		if strings.HasPrefix(line, "Model") {
			if _, v, ok := strings.Cut(line, ":"); ok {
				boardModel = strings.TrimSpace(v)
			}
		}
	}
	return boardModel
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

// readMemUsageKB returns live memory use as (used, total) in 1K blocks. Used is
// total minus available — the figure that reflects real pressure, since cache
// and buffers are reclaimable.
func readMemUsageKB() (used, total int64) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	return parseMemUsageKB(string(data))
}

func parseMemUsageKB(meminfo string) (used, total int64) {
	var availKB int64
	for _, line := range strings.Split(meminfo, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			total, _ = strconv.ParseInt(fields[1], 10, 64)
		case "MemAvailable:":
			availKB, _ = strconv.ParseInt(fields[1], 10, 64)
		}
	}
	if total == 0 {
		return 0, 0
	}
	return total - availKB, total
}

// readKernel returns the running kernel release, the first thing worth knowing
// when a driver or suspend problem turns up.
func readKernel() string {
	if data, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		return strings.TrimSpace(string(data))
	}
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// readUptime returns how long this machine has been up (Linux; 0 elsewhere).
func readUptime() time.Duration {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0
	}
	return time.Duration(secs) * time.Second
}

func readGPUs() []GPUInfo {
	// Try nvidia-smi first — gives clean names even for new devices
	if gpus := readNvidiaGPUs(); len(gpus) > 0 {
		return gpus
	}

	// Linux fallback via lspci
	out, err := exec.Command("lspci").Output()
	if err != nil {
		// macOS fallback
		return readMacGPUs()
	}

	var gpus []GPUInfo
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "VGA") || strings.Contains(line, "3D controller") || strings.Contains(line, "Display controller") {
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				gpus = append(gpus, GPUInfo{Name: strings.TrimSpace(parts[1])})
			}
		}
	}
	return gpus
}

func readNvidiaGPUs() []GPUInfo {
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,driver_version", "--format=csv,noheader").Output()
	if err != nil {
		return nil
	}
	var gpus []GPUInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ", ", 2)
		gpu := GPUInfo{Name: strings.TrimSpace(parts[0])}
		if len(parts) == 2 {
			gpu.Driver = strings.TrimSpace(parts[1])
		}
		gpus = append(gpus, gpu)
	}
	return gpus
}

func readMacGPUs() []GPUInfo {
	out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
	if err != nil {
		return nil
	}
	var gpus []GPUInfo
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Chipset Model:") {
			name := strings.TrimSpace(strings.TrimPrefix(line, "Chipset Model:"))
			gpus = append(gpus, GPUInfo{Name: name})
		}
	}
	return gpus
}

func readNetworkAdapters() []NetworkAdapter {
	// Try ip link (Linux)
	out, err := exec.Command("ip", "-o", "link", "show").Output()
	if err == nil {
		return parseIPLink(string(out))
	}
	// macOS fallback
	out, err = exec.Command("networksetup", "-listallhardwareports").Output()
	if err == nil {
		return parseMacNetworkSetup(string(out))
	}
	return nil
}

func parseIPLink(output string) []NetworkAdapter {
	var adapters []NetworkAdapter
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		iface := strings.TrimRight(fields[1], ":")
		// Skip loopback and virtual interfaces
		if iface == "lo" || strings.HasPrefix(iface, "veth") || strings.HasPrefix(iface, "br-") ||
			strings.HasPrefix(iface, "docker") || strings.HasPrefix(iface, "virbr") {
			continue
		}
		adapter := NetworkAdapter{Interface: iface}
		if strings.HasPrefix(iface, "wl") {
			adapter.Type = "wifi"
			adapter.Name = readWifiName(iface)
		} else if strings.HasPrefix(iface, "en") || strings.HasPrefix(iface, "eth") {
			adapter.Type = "ethernet"
			adapter.Name = readEthernetName(iface)
		} else {
			continue
		}
		adapters = append(adapters, adapter)
	}
	return adapters
}

func readWifiName(iface string) string {
	// Try to get the hardware description from /sys
	path := fmt.Sprintf("/sys/class/net/%s/device/uevent", iface)
	data, err := os.ReadFile(path)
	if err != nil {
		return "Wi-Fi"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "PCI_ID=") || strings.HasPrefix(line, "DRIVER=") {
			// Get friendly name via lspci
			out, err := exec.Command("lspci").Output()
			if err != nil {
				return "Wi-Fi"
			}
			for _, l := range strings.Split(string(out), "\n") {
				if strings.Contains(l, "Network controller") || strings.Contains(l, "Wireless") {
					parts := strings.SplitN(l, ": ", 2)
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
	}
	return "Wi-Fi"
}

func readEthernetName(iface string) string {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return "Ethernet"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "Ethernet controller") {
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "Ethernet"
}

func parseMacNetworkSetup(output string) []NetworkAdapter {
	var adapters []NetworkAdapter
	var current NetworkAdapter
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Hardware Port:") {
			name := strings.TrimPrefix(line, "Hardware Port: ")
			current = NetworkAdapter{Name: name}
			if strings.Contains(strings.ToLower(name), "wi-fi") {
				current.Type = "wifi"
			} else if strings.Contains(strings.ToLower(name), "ethernet") || strings.Contains(strings.ToLower(name), "thunderbolt") {
				current.Type = "ethernet"
			}
		}
		if strings.HasPrefix(line, "Device:") && current.Name != "" {
			current.Interface = strings.TrimPrefix(line, "Device: ")
			if current.Type != "" && macIfaceActive(current.Interface) {
				adapters = append(adapters, current)
			}
			current = NetworkAdapter{}
		}
	}
	return adapters
}

func macIfaceActive(iface string) bool {
	out, err := exec.Command("ifconfig", iface).Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), "status: active")
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

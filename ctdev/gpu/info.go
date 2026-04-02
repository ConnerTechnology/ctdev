package gpu

import (
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"os/exec"
)

// StatusCheck holds the result of a single GPU signing status check.
type StatusCheck struct {
	Name   string
	Pass   bool
	Detail string
}

// GatherStatus runs all GPU signing detection checks and returns structured results.
// If Secure Boot is disabled, it returns early with just that check since signing is not needed.
func GatherStatus() []StatusCheck {
	var checks []StatusCheck

	// 1. Secure Boot
	sbEnabled := IsSecureBootEnabled()
	if !sbEnabled {
		checks = append(checks, StatusCheck{
			Name:   "Secure Boot",
			Pass:   true,
			Detail: "disabled (signing not required)",
		})
		return checks
	}
	checks = append(checks, StatusCheck{
		Name:   "Secure Boot",
		Pass:   true,
		Detail: "enabled",
	})

	// 2. NVIDIA driver
	nvidiaLoaded := IsNvidiaLoaded()
	if nvidiaLoaded {
		version := GetDriverVersion()
		variant := DetectVariant()
		checks = append(checks, StatusCheck{
			Name:   "NVIDIA driver",
			Pass:   true,
			Detail: fmt.Sprintf("%s (%s kernel module)", version, variant),
		})
	} else {
		backend := GetRenderingBackend()
		var detail string
		if backend == "llvmpipe" {
			detail = "not loaded (falling back to software rendering)"
		} else {
			detail = fmt.Sprintf("not loaded (using %s)", backend)
		}
		checks = append(checks, StatusCheck{
			Name:   "NVIDIA driver",
			Pass:   false,
			Detail: detail,
		})
	}

	// 3. MOK key exists
	if MOKKeyExists() {
		checks = append(checks, StatusCheck{
			Name:   "MOK key exists",
			Pass:   true,
			Detail: fmt.Sprintf("%s (MOK.priv, MOK.der)", MOKDir),
		})
	} else {
		checks = append(checks, StatusCheck{
			Name:   "MOK key",
			Pass:   false,
			Detail: fmt.Sprintf("not found at %s", MOKDir),
		})
	}

	// 4. MOK directory clutter
	clutter := FindMOKClutter()
	if len(clutter) > 0 {
		var names []string
		for _, f := range clutter {
			parts := strings.Split(f, "/")
			names = append(names, parts[len(parts)-1])
		}
		checks = append(checks, StatusCheck{
			Name:   "MOK directory",
			Pass:   false,
			Detail: fmt.Sprintf("unnecessary files: %s", strings.Join(names, ", ")),
		})
	}

	// 5. DKMS signing configured
	if DKMSSigningConfigured() {
		checks = append(checks, StatusCheck{
			Name:   "DKMS signing",
			Pass:   true,
			Detail: "configured",
		})
	} else {
		checks = append(checks, StatusCheck{
			Name:   "DKMS signing",
			Pass:   false,
			Detail: fmt.Sprintf("not configured in %s", DKMSFrameworkConf),
		})
	}

	// 6. MOK key enrolled
	if MOKKeyEnrolled() {
		checks = append(checks, StatusCheck{
			Name:   "MOK key enrolled",
			Pass:   true,
			Detail: "in firmware",
		})
	} else {
		detail := "not enrolled"
		if MOKKeyExists() {
			detail = "exists but not enrolled (reboot required)"
		}
		checks = append(checks, StatusCheck{
			Name:   "MOK key",
			Pass:   false,
			Detail: detail,
		})
	}

	// 7. Module signature matches — only if nvidia is loaded
	if nvidiaLoaded {
		if ModuleSignatureValid() {
			checks = append(checks, StatusCheck{
				Name:   "Module signature",
				Pass:   true,
				Detail: "matches enrolled MOK key",
			})
		} else {
			signer := GetModuleSigner()
			var detail string
			if signer != "" {
				detail = fmt.Sprintf("signed by '%s' (does not match MOK key)", signer)
			} else {
				detail = "unsigned"
			}
			checks = append(checks, StatusCheck{
				Name:   "Module signature",
				Pass:   false,
				Detail: detail,
			})
		}
	}

	return checks
}

// ShowHardwareInfo writes detailed GPU hardware info to w.
func ShowHardwareInfo(w io.Writer) {
	indent := "  "

	if runtime.GOOS == "darwin" {
		showMacHardwareInfo(w, indent)
		return
	}

	showLinuxHardwareInfo(w, indent)
}

func showMacHardwareInfo(w io.Writer, indent string) {
	out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
	if err != nil {
		fmt.Fprintf(w, "%sNo GPU information available\n", indent)
		return
	}

	found := false
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Chipset Model:") ||
			strings.Contains(trimmed, "VRAM") ||
			strings.HasPrefix(trimmed, "Metal:") {
			fmt.Fprintf(w, "%s%s\n", indent, trimmed)
			found = true
		}
	}
	if !found {
		fmt.Fprintf(w, "%sNo GPU information available\n", indent)
	}
}

func showLinuxHardwareInfo(w io.Writer, indent string) {
	// Check if NVIDIA hardware is present via lspci
	hasNvidiaHW := false
	lspciOut, lspciErr := exec.Command("lspci").Output()
	if lspciErr == nil {
		for _, line := range strings.Split(string(lspciOut), "\n") {
			if strings.Contains(strings.ToLower(line), "nvidia") {
				hasNvidiaHW = true
				break
			}
		}
	}

	if hasNvidiaHW {
		if IsNvidiaLoaded() {
			showNvidiaDetailedInfo(w, indent)
		} else {
			showNvidiaUnloadedInfo(w, indent)
		}

		// Show any other GPUs (AMD/Intel) via lspci
		if lspciErr == nil {
			for _, line := range strings.Split(string(lspciOut), "\n") {
				lower := strings.ToLower(line)
				if (strings.Contains(lower, "vga") || strings.Contains(lower, "3d") || strings.Contains(lower, "display")) &&
					!strings.Contains(lower, "nvidia") {
					parts := strings.SplitN(line, ": ", 2)
					if len(parts) == 2 {
						fmt.Fprintf(w, "%sOther: %s\n", indent, strings.TrimSpace(parts[1]))
					}
				}
			}
		}
		return
	}

	// No NVIDIA — show whatever GPUs lspci finds
	if lspciErr == nil {
		count := 0
		for _, line := range strings.Split(string(lspciOut), "\n") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "vga") || strings.Contains(lower, "3d controller") || strings.Contains(lower, "display controller") {
				parts := strings.SplitN(line, ": ", 2)
				if len(parts) == 2 {
					fmt.Fprintf(w, "%s%s\n", indent, strings.TrimSpace(parts[1]))
					count++
				}
			}
		}
		if count == 0 {
			fmt.Fprintf(w, "%sNo GPU detected\n", indent)
		}
		return
	}

	fmt.Fprintf(w, "%sNo GPU information available (install pciutils)\n", indent)
}

func showNvidiaDetailedInfo(w io.Writer, indent string) {
	// Get CUDA version from nvidia-smi header
	cudaVersion := ""
	if smiOut, err := exec.Command("nvidia-smi").Output(); err == nil {
		for _, line := range strings.Split(string(smiOut), "\n") {
			if strings.Contains(line, "CUDA Version") {
				// Format: "| CUDA Version: 12.4     |"
				idx := strings.Index(line, "CUDA Version:")
				if idx >= 0 {
					rest := strings.TrimSpace(line[idx+len("CUDA Version:"):])
					rest = strings.TrimRight(rest, " |")
					cudaVersion = strings.TrimSpace(rest)
				}
				break
			}
		}
	}

	query := "name,memory.used,memory.total,power.draw,power.limit,temperature.gpu,driver_version"
	out, err := exec.Command("nvidia-smi", "--query-gpu="+query, "--format=csv,noheader,nounits").Output()
	if err != nil {
		// nvidia-smi failed — fall back to basic info
		model := getGPUModel()
		fmt.Fprintf(w, "%sNVIDIA: %s\n", indent, orDefault(model, "Unknown model"))
		fmt.Fprintf(w, "%s  Driver: Not responding (try reloading)\n", indent)
		return
	}

	gpuCount := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 7 {
			continue
		}
		gpuCount++

		name := strings.TrimSpace(parts[0])
		memUsed := strings.TrimSpace(parts[1])
		memTotal := strings.TrimSpace(parts[2])
		powerDraw := strings.TrimSpace(parts[3])
		powerCap := strings.TrimSpace(parts[4])
		temp := strings.TrimSpace(parts[5])
		driver := strings.TrimSpace(parts[6])

		memUsedGB := mibToGB(memUsed)
		memTotalGB := mibToGB(memTotal)

		fmt.Fprintf(w, "%sNVIDIA GPU %d:\n", indent, gpuCount)
		fmt.Fprintf(w, "%s  Model: %s\n", indent, name)
		fmt.Fprintf(w, "%s  Memory: %s GB used / %s GB total\n", indent, memUsedGB, memTotalGB)
		fmt.Fprintf(w, "%s  Power: %sW / %sW\n", indent, powerDraw, powerCap)
		fmt.Fprintf(w, "%s  Temperature: %sC\n", indent, temp)
		if gpuCount == 1 {
			fmt.Fprintf(w, "%s  Driver: %s\n", indent, driver)
			if cudaVersion != "" {
				fmt.Fprintf(w, "%s  CUDA: %s\n", indent, cudaVersion)
			}
		}
	}

	if gpuCount == 0 {
		model := getGPUModel()
		fmt.Fprintf(w, "%sNVIDIA: %s\n", indent, orDefault(model, "Unknown model"))
		fmt.Fprintf(w, "%s  Driver: Not responding (try reloading)\n", indent)
	}
}

func showNvidiaUnloadedInfo(w io.Writer, indent string) {
	model := getGPUModel()
	fmt.Fprintf(w, "%sNVIDIA: %s\n", indent, orDefault(model, "Unknown NVIDIA GPU"))
	fmt.Fprintf(w, "%s  Driver: Not loaded\n", indent)

	backend := GetRenderingBackend()
	if backend == "llvmpipe" {
		fmt.Fprintf(w, "%s  Status: Using software rendering (llvmpipe)\n", indent)
	}

	if IsSecureBootEnabled() {
		fmt.Fprintf(w, "%s  Note: Secure Boot enabled - driver may need signing\n", indent)
	}
}

// getGPUModel returns the GPU model from lspci.
func getGPUModel() string {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "nvidia") &&
			(strings.Contains(lower, "vga") || strings.Contains(lower, "3d") || strings.Contains(lower, "display")) {
			parts := strings.SplitN(line, ": ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// mibToGB converts a MiB string to a GB string with one decimal place.
func mibToGB(mib string) string {
	v, err := strconv.ParseFloat(mib, 64)
	if err != nil {
		return "?"
	}
	return fmt.Sprintf("%.1f", v/1024)
}

func orDefault(s, def string) string {
	if s != "" {
		return s
	}
	return def
}

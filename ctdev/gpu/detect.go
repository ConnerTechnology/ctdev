package gpu

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	MOKDir            = "/var/lib/shim-signed/mok"
	MOKPriv           = MOKDir + "/MOK.priv"
	MOKCert           = MOKDir + "/MOK.der"
	DKMSFrameworkConf = "/etc/dkms/framework.conf"
	DKMSConfDir       = "/etc/dkms/framework.conf.d"
	DKMSConf          = DKMSConfDir + "/sign-modules.conf"
	DKMSSignScript    = "/etc/dkms/sign-module.sh"
)

// IsSecureBootEnabled checks mokutil --sb-state for "SecureBoot enabled".
func IsSecureBootEnabled() bool {
	out, err := exec.Command("mokutil", "--sb-state").Output()
	if err != nil {
		return false
	}
	return parseSecureBootState(string(out))
}

func parseSecureBootState(output string) bool {
	return strings.Contains(output, "SecureBoot enabled")
}

// IsNvidiaLoaded checks if the nvidia kernel module is loaded via lsmod.
func IsNvidiaLoaded() bool {
	out, err := exec.Command("lsmod").Output()
	if err != nil {
		return false
	}
	return parseLsmodNvidia(string(out))
}

func parseLsmodNvidia(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "nvidia ") {
			return true
		}
	}
	return false
}

// GetDriverVersion returns the NVIDIA driver version string.
// Tries nvidia-smi first, falls back to modinfo.
func GetDriverVersion() string {
	out, err := exec.Command("nvidia-smi", "--query-gpu=driver_version", "--format=csv,noheader").Output()
	if err == nil {
		if v := parseNvidiaSmiVersion(string(out)); v != "" {
			return v
		}
	}
	out, err = exec.Command("modinfo", "nvidia").Output()
	if err != nil {
		return ""
	}
	return parseModinfoVersion(string(out))
}

func parseNvidiaSmiVersion(output string) string {
	line := strings.TrimSpace(strings.Split(output, "\n")[0])
	if strings.Contains(line, "NVIDIA-SMI has failed") {
		return ""
	}
	return line
}

func parseModinfoVersion(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		}
	}
	return ""
}

// DetectVariant returns "open", "closed", or "unknown" based on the nvidia module license.
func DetectVariant() string {
	out, err := exec.Command("modinfo", "nvidia").Output()
	if err != nil {
		return "unknown"
	}
	return parseModinfoVariant(string(out))
}

func parseModinfoVariant(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "license:") {
			license := strings.TrimSpace(strings.TrimPrefix(line, "license:"))
			if strings.Contains(license, "MIT") || strings.Contains(license, "GPL") {
				return "open"
			}
			if license != "" {
				return "closed"
			}
		}
	}
	return "unknown"
}

// GetRenderingBackend returns "nvidia", "llvmpipe", or "unknown" based on glxinfo output.
func GetRenderingBackend() string {
	out, err := exec.Command("glxinfo").Output()
	if err != nil {
		return "unknown"
	}
	return parseGlxinfoRenderer(string(out))
}

func parseGlxinfoRenderer(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, "OpenGL renderer") {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "llvmpipe") {
				return "llvmpipe"
			}
			if strings.Contains(lower, "nvidia") {
				return "nvidia"
			}
			return "unknown"
		}
	}
	return "unknown"
}

// MOKKeyExists returns true if both MOK.priv and MOK.der exist.
func MOKKeyExists() bool {
	if _, err := os.Stat(MOKPriv); err != nil {
		return false
	}
	if _, err := os.Stat(MOKCert); err != nil {
		return false
	}
	return true
}

// MOKKeyEnrolled checks if the MOK certificate fingerprint appears in mokutil --list-enrolled.
func MOKKeyEnrolled() bool {
	if !MOKKeyExists() {
		return false
	}
	fpOut, err := exec.Command("openssl", "x509", "-inform", "DER", "-in", MOKCert, "-fingerprint", "-noout").Output()
	if err != nil {
		return false
	}
	fingerprint := parseOpenSSLFingerprint(string(fpOut))
	if fingerprint == "" {
		return false
	}
	enrolled, err := exec.Command("mokutil", "--list-enrolled").Output()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(enrolled)), strings.ToLower(fingerprint))
}

func parseOpenSSLFingerprint(output string) string {
	// Output looks like: "SHA1 Fingerprint=AA:BB:CC:..."
	parts := strings.SplitN(output, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

// DKMSFrameworkConfConfigured checks if /etc/dkms/framework.conf has correct
// uncommented mok_signing_key and mok_certificate lines.
func DKMSFrameworkConfConfigured() bool {
	data, err := os.ReadFile(DKMSFrameworkConf)
	if err != nil {
		return false
	}
	return parseDKMSFrameworkConf(string(data))
}

func parseDKMSFrameworkConf(content string) bool {
	hasKey := false
	hasCert := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "mok_signing_key="+MOKPriv {
			hasKey = true
		}
		if line == "mok_certificate="+MOKCert {
			hasCert = true
		}
	}
	return hasKey && hasCert
}

// DKMSSigningConfigured returns true if framework.conf is configured or legacy conf.d files exist.
func DKMSSigningConfigured() bool {
	if DKMSFrameworkConfConfigured() {
		return true
	}
	if _, err := os.Stat(DKMSConf); err != nil {
		return false
	}
	if _, err := os.Stat(DKMSSignScript); err != nil {
		return false
	}
	return true
}

// ModuleSignatureValid compares the nvidia module signer against the MOK certificate CN.
func ModuleSignatureValid() bool {
	signer := GetModuleSigner()
	if signer == "" {
		return false
	}
	cnOut, err := exec.Command("openssl", "x509", "-inform", "DER", "-in", MOKCert, "-subject", "-noout").Output()
	if err != nil {
		return false
	}
	cn := parseOpenSSLSubjectCN(string(cnOut))
	return cn != "" && signer == cn
}

func parseOpenSSLSubjectCN(output string) string {
	// Output looks like: "subject=CN = Custom NVIDIA Module Signing"
	// or "subject= /CN=Custom NVIDIA Module Signing"
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "CN"); idx >= 0 {
			rest := line[idx+2:]
			// Skip past "CN", then optional whitespace, "=", optional whitespace
			rest = strings.TrimLeft(rest, " ")
			rest = strings.TrimPrefix(rest, "=")
			rest = strings.TrimLeft(rest, " ")
			if rest != "" {
				return rest
			}
		}
	}
	return ""
}

// FindMOKClutter returns files in MOKDir that are not MOK.priv or MOK.der.
func FindMOKClutter() []string {
	entries, err := os.ReadDir(MOKDir)
	if err != nil {
		return nil
	}
	var clutter []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() != "MOK.priv" && e.Name() != "MOK.der" {
			clutter = append(clutter, filepath.Join(MOKDir, e.Name()))
		}
	}
	return clutter
}

// GetModuleSigner returns the signer field from modinfo nvidia.
func GetModuleSigner() string {
	out, err := exec.Command("modinfo", "nvidia").Output()
	if err != nil {
		return ""
	}
	return parseModinfoSigner(string(out))
}

func parseModinfoSigner(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "signer:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "signer:"))
		}
	}
	return ""
}

// GetNvidiaDKMSInfo returns the nvidia DKMS module info (e.g. "nvidia/550.120").
func GetNvidiaDKMSInfo() string {
	out, err := exec.Command("dkms", "status").Output()
	if err != nil {
		return ""
	}
	return parseDKMSStatus(string(out))
}

func parseDKMSStatus(output string) string {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(strings.ToLower(line), "nvidia") {
			// Format: "nvidia/550.120, 6.8.0-51-generic, x86_64: installed"
			parts := strings.SplitN(line, ",", 2)
			return strings.TrimSpace(parts[0])
		}
	}
	return ""
}

// FindNvidiaModules finds nvidia .ko files in the DKMS directory for the running kernel.
func FindNvidiaModules() []string {
	kernelOut, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return nil
	}
	kernel := strings.TrimSpace(string(kernelOut))
	return findNvidiaModulesInDir(fmt.Sprintf("/lib/modules/%s/updates/dkms", kernel))
}

func findNvidiaModulesInDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var modules []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "nvidia") &&
			(strings.HasSuffix(name, ".ko") || strings.HasSuffix(name, ".ko.zst") || strings.HasSuffix(name, ".ko.xz")) {
			modules = append(modules, filepath.Join(dir, name))
		}
	}
	return modules
}

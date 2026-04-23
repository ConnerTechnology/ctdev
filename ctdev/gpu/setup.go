package gpu

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Opts controls the behavior of GPU setup actions.
type Opts struct {
	Stdout  io.Writer
	Stdin   io.Reader
	DryRun  bool
	Force   bool
	Verbose bool
}

// CreateMOKKeypair generates a new MOK key pair (MOK.priv + MOK.der).
func CreateMOKKeypair(ctx context.Context, opts Opts) error {
	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "[dry-run] Would create MOK key pair at %s\n", MOKDir)
		return nil
	}

	if err := sudoRun(ctx, opts, "mkdir", "-p", MOKDir); err != nil {
		return fmt.Errorf("create MOK directory: %w", err)
	}

	if err := sudoRun(ctx, opts, "openssl", "req", "-new", "-x509", "-newkey", "rsa:2048",
		"-keyout", MOKPriv,
		"-outform", "DER",
		"-out", MOKCert,
		"-days", "36500",
		"-subj", "/CN=Custom NVIDIA Module Signing/",
		"-nodes"); err != nil {
		return fmt.Errorf("generate MOK keypair: %w", err)
	}

	if err := sudoRun(ctx, opts, "chmod", "600", MOKPriv); err != nil {
		return fmt.Errorf("set MOK.priv permissions: %w", err)
	}
	if err := sudoRun(ctx, opts, "chmod", "644", MOKCert); err != nil {
		return fmt.Errorf("set MOK.der permissions: %w", err)
	}

	return nil
}

// ConfigureDKMSFramework configures /etc/dkms/framework.conf for automatic module signing.
func ConfigureDKMSFramework(ctx context.Context, opts Opts) error {
	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "[dry-run] Would configure %s\n", DKMSFrameworkConf)
		return nil
	}

	// Backup
	backup := fmt.Sprintf("%s.backup-%s", DKMSFrameworkConf, time.Now().Format("20060102-150405"))
	if err := sudoRun(ctx, opts, "cp", DKMSFrameworkConf, backup); err != nil {
		return fmt.Errorf("backup framework.conf: %w", err)
	}

	// Remove existing mok_signing_key and mok_certificate lines (commented or not)
	if err := sudoRun(ctx, opts, "sed", "-i", `/^[#[:space:]]*mok_signing_key=/d`, DKMSFrameworkConf); err != nil {
		return fmt.Errorf("remove old mok_signing_key lines: %w", err)
	}
	if err := sudoRun(ctx, opts, "sed", "-i", `/^[#[:space:]]*mok_certificate=/d`, DKMSFrameworkConf); err != nil {
		return fmt.Errorf("remove old mok_certificate lines: %w", err)
	}

	// Append correct values using tee
	keyLine := fmt.Sprintf("mok_signing_key=%s", MOKPriv)
	certLine := fmt.Sprintf("mok_certificate=%s", MOKCert)

	if err := sudoTeeAppend(ctx, opts, DKMSFrameworkConf, keyLine); err != nil {
		return fmt.Errorf("write mok_signing_key: %w", err)
	}
	if err := sudoTeeAppend(ctx, opts, DKMSFrameworkConf, certLine); err != nil {
		return fmt.Errorf("write mok_certificate: %w", err)
	}

	return nil
}

// ConfigureDKMSLegacy configures DKMS signing via the conf.d approach.
func ConfigureDKMSLegacy(ctx context.Context, opts Opts) error {
	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "[dry-run] Would configure DKMS signing at %s\n", DKMSConf)
		return nil
	}

	if err := sudoRun(ctx, opts, "mkdir", "-p", DKMSConfDir); err != nil {
		return fmt.Errorf("create conf.d directory: %w", err)
	}

	confContent := fmt.Sprintf("mok_signing_key=\"%s\"\nmok_certificate=\"%s\"\nsign_tool=\"%s\"\n",
		MOKPriv, MOKCert, DKMSSignScript)
	if err := sudoTeeWrite(ctx, opts, DKMSConf, confContent); err != nil {
		return fmt.Errorf("write DKMS conf: %w", err)
	}

	scriptContent := "#!/bin/bash\n/lib/modules/\"$1\"/build/scripts/sign-file sha256 \"$2\" \"$3\" \"$4\"\n"
	if err := sudoTeeWrite(ctx, opts, DKMSSignScript, scriptContent); err != nil {
		return fmt.Errorf("write sign script: %w", err)
	}

	if err := sudoRun(ctx, opts, "chmod", "+x", DKMSSignScript); err != nil {
		return fmt.Errorf("make sign script executable: %w", err)
	}

	return nil
}

// EnrollMOK imports the MOK certificate for enrollment at next reboot.
// The user will be prompted for a one-time password via stdin.
func EnrollMOK(ctx context.Context, opts Opts) error {
	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "[dry-run] Would run: mokutil --import %s\n", MOKCert)
		return nil
	}

	cmd := exec.Command("sudo", "mokutil", "--import", MOKCert)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stdout
	cmd.Stdin = opts.Stdin
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mokutil --import: %w", err)
	}
	return nil
}

// RebuildDKMS removes and reinstalls the NVIDIA DKMS module to trigger signing.
func RebuildDKMS(ctx context.Context, opts Opts) error {
	info := GetNvidiaDKMSInfo()
	if info == "" {
		return fmt.Errorf("no NVIDIA DKMS module found")
	}

	kernelOut, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return fmt.Errorf("get kernel version: %w", err)
	}
	kernel := strings.TrimSpace(string(kernelOut))

	if opts.DryRun {
		fmt.Fprintf(opts.Stdout, "[dry-run] Would remove: dkms remove %s -k %s\n", info, kernel)
		fmt.Fprintf(opts.Stdout, "[dry-run] Would install: dkms install %s -k %s\n", info, kernel)
		return nil
	}

	fmt.Fprintf(opts.Stdout, "Removing NVIDIA DKMS module: %s for kernel %s\n", info, kernel)
	// Ignore remove errors (module might not be installed for this kernel)
	_ = sudoRun(ctx, opts, "dkms", "remove", info, "-k", kernel, "--no-depmod")

	fmt.Fprintf(opts.Stdout, "Rebuilding NVIDIA DKMS module: %s for kernel %s\n", info, kernel)
	if err := sudoRun(ctx, opts, "dkms", "install", info, "-k", kernel); err != nil {
		return fmt.Errorf("DKMS rebuild failed: %w", err)
	}

	fmt.Fprintf(opts.Stdout, "DKMS rebuild complete\n")
	return nil
}

// SignNvidiaModules manually signs NVIDIA kernel modules for the current kernel.
func SignNvidiaModules(ctx context.Context, opts Opts) error {
	kernelOut, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return fmt.Errorf("get kernel version: %w", err)
	}
	kernel := strings.TrimSpace(string(kernelOut))
	signFile := fmt.Sprintf("/lib/modules/%s/build/scripts/sign-file", kernel)

	if !MOKKeyExists() {
		return fmt.Errorf("MOK keys not found — run 'ctdev gpu setup' first")
	}

	modules := FindNvidiaModules()
	if len(modules) == 0 {
		return fmt.Errorf("no NVIDIA modules found for kernel %s", kernel)
	}

	signedCount := 0
	for _, module := range modules {
		if opts.DryRun {
			fmt.Fprintf(opts.Stdout, "[dry-run] Would sign: %s\n", module)
			signedCount++
			continue
		}

		actual := module
		compressed := false
		var compressFormat string

		if strings.HasSuffix(module, ".zst") {
			actual = strings.TrimSuffix(module, ".zst")
			compressFormat = "zst"
			compressed = true
			_ = sudoRun(ctx, opts, "zstd", "-d", "-f", module, "-o", actual)
		} else if strings.HasSuffix(module, ".xz") {
			actual = strings.TrimSuffix(module, ".xz")
			compressFormat = "xz"
			compressed = true
			_ = sudoRun(ctx, opts, "xz", "-d", "-k", "-f", module)
		}

		if err := sudoRun(ctx, opts, signFile, "sha256", MOKPriv, MOKCert, actual); err != nil {
			fmt.Fprintf(opts.Stdout, "Failed to sign: %s\n", module)
			continue
		}

		fmt.Fprintf(opts.Stdout, "Signed: %s\n", module)
		signedCount++

		if compressed {
			switch compressFormat {
			case "zst":
				_ = sudoRun(ctx, opts, "zstd", "-f", actual, "-o", module)
				_ = sudoRun(ctx, opts, "rm", "-f", actual)
			case "xz":
				_ = sudoRun(ctx, opts, "xz", "-f", actual)
			}
		}
	}

	if !opts.DryRun {
		fmt.Fprintf(opts.Stdout, "Signed %d module(s)\n", signedCount)
	}
	return nil
}

// CleanMOKClutter prompts the user to remove unexpected files from MOKDir.
func CleanMOKClutter(ctx context.Context, opts Opts) error {
	clutter := FindMOKClutter()
	if len(clutter) == 0 {
		return nil
	}

	fmt.Fprintf(opts.Stdout, "Found unnecessary files in %s:\n", MOKDir)
	for _, f := range clutter {
		fmt.Fprintf(opts.Stdout, "  %s\n", f)
	}
	fmt.Fprintln(opts.Stdout)

	if !promptYesNo(opts, "Remove these files? [Y/n] ", true) {
		return nil
	}

	for _, f := range clutter {
		if opts.DryRun {
			fmt.Fprintf(opts.Stdout, "[dry-run] Would remove: %s\n", f)
			continue
		}
		if err := sudoRun(ctx, opts, "rm", "-f", f); err != nil {
			fmt.Fprintf(opts.Stdout, "Warning: could not remove %s: %v\n", f, err)
		} else {
			fmt.Fprintf(opts.Stdout, "Removed: %s\n", f)
		}
	}
	return nil
}

// RunSetup orchestrates the full GPU signing setup flow.
func RunSetup(ctx context.Context, opts Opts) error {
	if runtime.GOOS == "darwin" {
		return fmt.Errorf("GPU driver signing setup is not applicable on macOS")
	}

	fmt.Fprintln(opts.Stdout, "GPU Signing Setup")
	fmt.Fprintln(opts.Stdout)

	// Pre-flight: Secure Boot check
	if !IsSecureBootEnabled() {
		fmt.Fprintln(opts.Stdout, "Warning: Secure Boot is disabled. Driver signing is not required.")
		fmt.Fprintln(opts.Stdout)
		if !promptYesNo(opts, "Continue anyway? [y/N] ", false) {
			fmt.Fprintln(opts.Stdout, "Setup cancelled.")
			return nil
		}
	}

	// Check for NVIDIA DKMS
	if GetNvidiaDKMSInfo() == "" {
		fmt.Fprintln(opts.Stdout, "NVIDIA DKMS module not found.")
		fmt.Fprintln(opts.Stdout, "Install the NVIDIA driver first.")
		fmt.Fprintln(opts.Stdout, "Recommended: nvidia-driver-*-open (better Secure Boot support for RTX 30+)")
		return fmt.Errorf("NVIDIA DKMS module not found")
	}

	// Show driver variant warning
	variant := DetectVariant()
	if variant == "closed" {
		fmt.Fprintln(opts.Stdout, "Warning: Using closed-source NVIDIA kernel module.")
		fmt.Fprintln(opts.Stdout, "The open kernel module (nvidia-driver-*-open) is recommended for")
		fmt.Fprintln(opts.Stdout, "RTX 30-series and newer GPUs with better Secure Boot compatibility.")
		fmt.Fprintln(opts.Stdout)
	}

	// Check if already configured
	if MOKKeyExists() && DKMSSigningConfigured() && MOKKeyEnrolled() && ModuleSignatureValid() {
		if !opts.Force {
			fmt.Fprintln(opts.Stdout, "GPU signing is already fully configured.")
			return nil
		}
	}

	// Step 1: Create MOK keypair
	fmt.Fprintln(opts.Stdout, "Step 1: MOK Key Pair")
	if MOKKeyExists() && !opts.Force {
		fmt.Fprintf(opts.Stdout, "MOK keys already exist at %s\n", MOKDir)
	} else {
		if err := CreateMOKKeypair(ctx, opts); err != nil {
			return fmt.Errorf("failed to create MOK key pair: %w", err)
		}
		fmt.Fprintf(opts.Stdout, "Created MOK key pair at %s\n", MOKDir)
	}

	// Clean MOK clutter
	if err := CleanMOKClutter(ctx, opts); err != nil {
		fmt.Fprintf(opts.Stdout, "Warning: clutter cleanup failed: %v\n", err)
	}
	fmt.Fprintln(opts.Stdout)

	// Step 2: Configure DKMS framework.conf
	fmt.Fprintln(opts.Stdout, "Step 2: DKMS Configuration")
	if DKMSFrameworkConfConfigured() && !opts.Force {
		fmt.Fprintln(opts.Stdout, "DKMS framework.conf already configured")
	} else {
		if err := ConfigureDKMSFramework(ctx, opts); err != nil {
			fmt.Fprintf(opts.Stdout, "Could not configure framework.conf, trying conf.d approach: %v\n", err)
			if err := ConfigureDKMSLegacy(ctx, opts); err != nil {
				return fmt.Errorf("failed to configure DKMS signing: %w", err)
			}
			fmt.Fprintln(opts.Stdout, "DKMS signing configured (via conf.d)")
		} else {
			fmt.Fprintf(opts.Stdout, "Configured %s\n", DKMSFrameworkConf)
		}
	}
	fmt.Fprintln(opts.Stdout)

	// Step 3: Enroll MOK
	needsReboot := false
	fmt.Fprintln(opts.Stdout, "Step 3: MOK Key Enrollment")
	if MOKKeyEnrolled() && !opts.Force {
		fmt.Fprintln(opts.Stdout, "MOK key is already enrolled in firmware")
	} else {
		needsReboot = true
		fmt.Fprintln(opts.Stdout, "You will be prompted to create a one-time password.")
		fmt.Fprintln(opts.Stdout, "Remember this password - you'll need it at the next reboot.")
		fmt.Fprintln(opts.Stdout)
		if err := EnrollMOK(ctx, opts); err != nil {
			return fmt.Errorf("failed to import MOK key: %w", err)
		}
		fmt.Fprintln(opts.Stdout, "MOK key queued for enrollment")
	}
	fmt.Fprintln(opts.Stdout)

	// Step 4: DKMS rebuild
	fmt.Fprintln(opts.Stdout, "Step 4: DKMS Rebuild")
	if ModuleSignatureValid() && !opts.Force {
		fmt.Fprintln(opts.Stdout, "NVIDIA modules already signed with correct key")
	} else {
		if err := RebuildDKMS(ctx, opts); err != nil {
			fmt.Fprintf(opts.Stdout, "DKMS rebuild failed, falling back to manual signing: %v\n", err)
			if err := SignNvidiaModules(ctx, opts); err != nil {
				fmt.Fprintf(opts.Stdout, "Warning: manual signing also failed: %v\n", err)
			}
		}
	}
	fmt.Fprintln(opts.Stdout)

	// Reboot instructions
	if !opts.DryRun && needsReboot {
		printRebootInstructions(opts.Stdout, false)
	}

	return nil
}

// RunRecover re-enrolls an existing MOK key after a CMOS/firmware reset.
func RunRecover(ctx context.Context, opts Opts) error {
	if runtime.GOOS == "darwin" {
		return fmt.Errorf("GPU driver signing recovery is not applicable on macOS")
	}

	fmt.Fprintln(opts.Stdout, "GPU Signing Recovery (CMOS Reset)")
	fmt.Fprintln(opts.Stdout)
	fmt.Fprintln(opts.Stdout, "Re-enrolling existing MOK key after CMOS/firmware reset.")
	fmt.Fprintln(opts.Stdout)

	if !MOKKeyExists() {
		fmt.Fprintf(opts.Stdout, "No MOK keys found at %s\n", MOKDir)
		fmt.Fprintln(opts.Stdout, "Cannot recover - keys do not exist on disk.")
		fmt.Fprintln(opts.Stdout, "Run 'ctdev gpu setup' instead to create new keys.")
		return fmt.Errorf("MOK keys not found")
	}

	fmt.Fprintf(opts.Stdout, "Found MOK certificate: %s\n", MOKCert)

	if MOKKeyEnrolled() {
		fmt.Fprintln(opts.Stdout, "MOK key is already enrolled in firmware. No recovery needed.")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Re-enrolling MOK key")
	fmt.Fprintln(opts.Stdout, "You will be prompted to create a one-time password.")
	fmt.Fprintln(opts.Stdout, "Remember this password - you'll need it at the next reboot.")
	fmt.Fprintln(opts.Stdout)

	if err := EnrollMOK(ctx, opts); err != nil {
		return fmt.Errorf("failed to import MOK key: %w", err)
	}
	fmt.Fprintln(opts.Stdout, "MOK key queued for enrollment")
	fmt.Fprintln(opts.Stdout)

	if !opts.DryRun {
		printRebootInstructions(opts.Stdout, true)
	}

	return nil
}

// sudoRun executes a command with sudo, honoring ctx for cancellation.
func sudoRun(ctx context.Context, opts Opts, name string, args ...string) error {
	allArgs := append([]string{name}, args...)
	cmd := exec.CommandContext(ctx, "sudo", allArgs...)
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stdout
	if opts.Verbose {
		fmt.Fprintf(opts.Stdout, "  $ sudo %s %s\n", name, strings.Join(args, " "))
	}
	return cmd.Run()
}

// sudoTeeAppend appends a line to a file using sudo tee -a.
func sudoTeeAppend(ctx context.Context, opts Opts, path, line string) error {
	cmd := exec.CommandContext(ctx, "sudo", "tee", "-a", path)
	cmd.Stdin = strings.NewReader(line + "\n")
	cmd.Stderr = opts.Stdout
	// Suppress stdout from tee (it echoes back)
	return cmd.Run()
}

// sudoTeeWrite writes content to a file using sudo tee (overwrite).
func sudoTeeWrite(ctx context.Context, opts Opts, path, content string) error {
	cmd := exec.CommandContext(ctx, "sudo", "tee", path)
	cmd.Stdin = strings.NewReader(content)
	cmd.Stderr = opts.Stdout
	return cmd.Run()
}

// promptYesNo asks a yes/no question and returns the user's answer.
// defaultYes controls the behavior when the user presses Enter without typing.
func promptYesNo(opts Opts, prompt string, defaultYes bool) bool {
	fmt.Fprint(opts.Stdout, prompt)
	scanner := bufio.NewScanner(opts.Stdin)
	if !scanner.Scan() {
		return defaultYes
	}
	response := strings.TrimSpace(scanner.Text())
	if response == "" {
		return defaultYes
	}
	return strings.HasPrefix(strings.ToLower(response), "y")
}

func printRebootInstructions(w io.Writer, isCMOSRecovery bool) {
	title := "REBOOT REQUIRED - MOK Enrollment"
	if isCMOSRecovery {
		title = "REBOOT REQUIRED - MOK Re-Enrollment"
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "═══════════════════════════════════════════════════════════════")
	fmt.Fprintf(w, "   %s\n", title)
	fmt.Fprintln(w, "═══════════════════════════════════════════════════════════════")
	fmt.Fprintln(w)
	if isCMOSRecovery {
		fmt.Fprintln(w, "   Your CMOS reset cleared the enrolled MOK key from firmware.")
		fmt.Fprintln(w, "   The key files on disk are still intact.")
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "   1. Reboot your computer now")
	fmt.Fprintln(w, "   2. Watch for the blue 'MOK Manager' screen")
	fmt.Fprintln(w, "   3. Select 'Enroll MOK'")
	fmt.Fprintln(w, "   4. Select 'Continue'")
	fmt.Fprintln(w, "   5. Select 'Yes' to confirm")
	fmt.Fprintln(w, "   6. Enter the password you just set")
	fmt.Fprintln(w, "   7. Select 'Reboot'")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "   After reboot, run 'ctdev gpu info' to verify.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "═══════════════════════════════════════════════════════════════")
	fmt.Fprintln(w)
}

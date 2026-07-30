package component

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// This file holds the modern-CLI components whose package or binary names
// differ per platform (ripgrep→rg, fd-find→fdfind, bat→batcat), plus the ones
// with a post-install step (lazygit, smartmontools, syncthing). Components
// with matching names everywhere use SimplePackageInstaller in the registry.

// linkUserBin symlinks ~/.local/bin/<name> → the resolved target binary, so
// Debian's renamed binaries (fdfind, batcat) answer to their upstream names.
// Best-effort: an existing file is never clobbered.
func linkUserBin(stdout io.Writer, name, target string) {
	src, err := exec.LookPath(target)
	if err != nil {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return
	}
	dest := filepath.Join(bin, name)
	if _, err := os.Lstat(dest); err == nil {
		return
	}
	if err := os.Symlink(src, dest); err == nil {
		fmt.Fprintf(stdout, "Linked %s → %s\n", dest, src)
	}
}

func ripgrepInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	if !opts.Force && sysutil.CommandExists("rg") {
		fmt.Fprintln(opts.Stdout, "ripgrep already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing ripgrep...")
	return sysutil.InstallPackage(ctx, o, "ripgrep")
}

func fdInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	if !opts.Force && alreadyInstalled("fd") {
		fmt.Fprintln(opts.Stdout, "fd already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing fd...")
	pkg := "fd"
	if p.PackageManager == "apt" {
		pkg = "fd-find" // Debian renames the package and the binary (fdfind)
	}
	if err := sysutil.InstallPackage(ctx, o, pkg); err != nil {
		return err
	}
	if !o.DryRun && p.PackageManager == "apt" {
		linkUserBin(opts.Stdout, "fd", "fdfind")
	}
	return nil
}

func fdUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing fd...")
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Remove(filepath.Join(home, ".local", "bin", "fd"))
	}
	pkg := "fd"
	if p.PackageManager == "apt" {
		pkg = "fd-find"
	}
	return sysutil.RemovePackage(ctx, o, pkg)
}

func batInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	if !opts.Force && alreadyInstalled("bat") {
		fmt.Fprintln(opts.Stdout, "bat already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing bat...")
	if err := sysutil.InstallPackage(ctx, o, "bat"); err != nil {
		return err
	}
	if !o.DryRun && p.PackageManager == "apt" {
		// Debian installs the binary as batcat to dodge a name clash.
		linkUserBin(opts.Stdout, "bat", "batcat")
	}
	return nil
}

func batUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing bat...")
	if home, err := os.UserHomeDir(); err == nil {
		_ = os.Remove(filepath.Join(home, ".local", "bin", "bat"))
	}
	return sysutil.RemovePackage(ctx, o, "bat")
}

func lazygitInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("lazygit") {
		fmt.Fprintln(opts.Stdout, "lazygit already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing lazygit...")

	if p.OS == platform.MacOS {
		return sysutil.InstallPackage(ctx, o, "lazygit")
	}

	// Not in Debian/Ubuntu repos — install the checksum-verified GitHub binary.
	ver, err := sysutil.DownloadGitHubBinary(ctx, o, sysutil.GitHubBinarySpec{
		Repo: "jesseduffield/lazygit",
		ArchiveURL: func(ver, goos, goarch string) string {
			arch := goarch
			if goarch == "amd64" {
				arch = "x86_64"
			}
			return fmt.Sprintf("https://github.com/jesseduffield/lazygit/releases/download/v%s/lazygit_%s_%s_%s.tar.gz", ver, ver, goos, arch)
		},
		ChecksumURL: func(ver, goos, goarch string) string {
			return fmt.Sprintf("https://github.com/jesseduffield/lazygit/releases/download/v%s/checksums.txt", ver)
		},
		BinaryPath:  func(goos, goarch string) string { return "lazygit" },
		InstallDest: "/usr/local/bin/lazygit",
		ArchFormat:  "tar.gz",
	})
	if err != nil {
		return err
	}
	if !o.DryRun {
		fmt.Fprintf(opts.Stdout, "lazygit %s installed\n", ver)
	}
	return nil
}

func lazygitUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing lazygit...")
	if p.OS == platform.MacOS {
		return sysutil.RemovePackage(ctx, o, "lazygit")
	}
	return sysutil.SudoRun(ctx, o, "rm", "-f", "/usr/local/bin/lazygit")
}

func smartmontoolsInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.PackageManager != "apt" {
		return unsupportedPMError("smartmontools", p.PackageManager)
	}
	if !opts.Force && alreadyInstalled("smartmontools") {
		fmt.Fprintln(opts.Stdout, "smartmontools already installed")
	} else {
		fmt.Fprintln(opts.Stdout, "Installing smartmontools...")
		if err := sysutil.InstallPackage(ctx, o, "smartmontools"); err != nil {
			return err
		}
	}

	if o.DryRun {
		return nil
	}

	// smartd exits (status 17, "No devices to monitor") and lands in
	// systemctl --failed when there's no SMART-capable disk — which is the norm
	// on an SD-card / eMMC Raspberry Pi. Only enable the daemon when a device
	// scan actually finds something to watch.
	if !hasSMARTDevices(ctx) {
		fmt.Fprintln(opts.Stdout, "smartctl installed. No SMART-capable disks found (e.g. an SD-card-only Pi),")
		fmt.Fprintln(opts.Stdout, "so the smartd monitoring service was not enabled.")
		return nil
	}

	// smartd polls SMART attributes and logs pre-failure warnings to the
	// journal. The Debian unit is "smartmontools"; older releases use "smartd".
	unit := "smartmontools"
	if err := sysutil.ServiceEnable(ctx, o, unit); err != nil {
		unit = "smartd"
		if err := sysutil.ServiceEnable(ctx, o, unit); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not enable the smartd service: %v\n", err)
			return nil
		}
	}
	if err := sysutil.ServiceStart(ctx, o, unit); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: smartd installed but did not start: %v\n", err)
		return nil
	}
	fmt.Fprintf(opts.Stdout, "smartd running — disk warnings land in: journalctl -u %s\n", unit)
	return nil
}

// hasSMARTDevices reports whether any SMART-capable block device is present for
// smartd to monitor. Uses sudo (cached during install) since probing devices
// needs root; matches smartd's own DEVICESCAN via `smartctl --scan`.
func hasSMARTDevices(ctx context.Context) bool {
	out, err := captureRoot(ctx, "smartctl", "--scan")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

func smartmontoolsUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing smartmontools...")
	return sysutil.RemovePackage(ctx, o, "smartmontools")
}

func syncthingInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("syncthing") {
		fmt.Fprintln(opts.Stdout, "syncthing already installed")
	} else {
		fmt.Fprintln(opts.Stdout, "Installing syncthing...")
		if err := sysutil.InstallPackage(ctx, o, "syncthing"); err != nil {
			return err
		}
	}

	// Starting it is the user's call (it opens a device on the tailnet/LAN);
	// print the per-OS one-liner instead of auto-enabling.
	if p.OS == platform.MacOS {
		fmt.Fprintln(opts.Stdout, "Start it with: brew services start syncthing")
	} else {
		fmt.Fprintln(opts.Stdout, "Start it per user: systemctl --user enable --now syncthing")
		fmt.Fprintln(opts.Stdout, "  ('ctdev configure linger' keeps user services running without a login)")
	}
	fmt.Fprintln(opts.Stdout, "Web UI: http://127.0.0.1:8384 (loopback-only by default — keep it that way)")
	return nil
}

func syncthingUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing syncthing...")
	_ = sysutil.Run(ctx, o, "systemctl", "--user", "disable", "--now", "syncthing")
	return sysutil.RemovePackage(ctx, o, "syncthing")
}

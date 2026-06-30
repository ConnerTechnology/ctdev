package cleanup

import (
	"context"
	"fmt"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func linuxTasks(info platform.Info) []Task {
	var tasks []Task
	apt := info.PackageManager == "apt" && commandExists("apt-get")

	if apt {
		tasks = append(tasks,
			Task{
				ID: "apt-autoremove", Name: "Orphaned packages & old kernels", Group: "Packages",
				Detail: "apt autoremove --purge", Risk: Safe,
				Scan: func(ctx context.Context) ScanResult {
					return ScanResult{Bytes: parseAptFreed(captureOut(ctx, "apt-get", "-s", "autoremove", "--purge"))}
				},
				Run: func(ctx context.Context, o sysutil.Opts) error {
					// Mint omits apt's apt-auto-removal hook, so the protect-list
					// that normally shields the running kernel from autoremove
					// isn't generated. Pin it manually first so --purge can never
					// pull the kernel we're booted on out from under us.
					if err := pinRunningKernel(ctx, o); err != nil {
						return err
					}
					return sysutil.SudoRun(ctx, o, "apt-get", "autoremove", "--purge", "-y")
				},
			},
			Task{
				ID: "apt-clean", Name: "APT package cache", Group: "Packages",
				Detail: "/var/cache/apt/archives", Risk: Safe,
				Scan: func(ctx context.Context) ScanResult {
					return ScanResult{Bytes: measurePaths(ctx, "/var/cache/apt/archives")}
				},
				Run: func(ctx context.Context, o sysutil.Opts) error {
					return sysutil.SudoRun(ctx, o, "apt-get", "clean")
				},
			},
			Task{
				ID: "apt-rc-purge", Name: "Config of removed packages", Group: "Packages",
				Detail: "dpkg --purge (rc state)", Risk: Safe,
				Scan: func(ctx context.Context) ScanResult {
					pkgs := rcPackages(ctx)
					if len(pkgs) == 0 {
						return ScanResult{Bytes: 0, Note: "none"}
					}
					return ScanResult{Bytes: -1, Note: fmt.Sprintf("%d packages", len(pkgs))}
				},
				Run: func(ctx context.Context, o sysutil.Opts) error {
					pkgs := rcPackages(ctx)
					if len(pkgs) == 0 {
						return nil
					}
					return sysutil.SudoRun(ctx, o, "dpkg", append([]string{"--purge"}, pkgs...)...)
				},
			},
		)
	}

	if commandExists("journalctl") {
		tasks = append(tasks, Task{
			ID: "journal", Name: "Systemd journal logs", Group: "Logs & crashes",
			Detail: "vacuum to 7 days", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: parseJournalUsage(captureOut(ctx, "journalctl", "--disk-usage")), Note: "current size"}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				return sysutil.SudoRun(ctx, o, "journalctl", "--vacuum-time=7d")
			},
		})
	}

	if pathExists("/var/crash") || pathExists("/var/lib/systemd/coredump") {
		tasks = append(tasks, Task{
			ID: "coredumps", Name: "Crash dumps & coredumps", Group: "Logs & crashes",
			Detail: "/var/crash, /var/lib/systemd/coredump", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx, "/var/crash", "/var/lib/systemd/coredump")}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				if err := sudoEmptyDir(ctx, o, "/var/crash"); err != nil {
					return err
				}
				return sudoEmptyDir(ctx, o, "/var/lib/systemd/coredump")
			},
		})
	}

	tasks = append(tasks, rotatedLogsTask())

	if commandExists("snap") {
		tasks = append(tasks, Task{
			ID: "snap-old", Name: "Old snap revisions", Group: "Snap & Flatpak",
			Detail: "disabled revisions", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				revs := disabledSnaps(ctx)
				if len(revs) == 0 {
					return ScanResult{Bytes: 0, Note: "none"}
				}
				var paths []string
				for _, r := range revs {
					paths = append(paths, fmt.Sprintf("/var/lib/snapd/snaps/%s_%s.snap", r.name, r.rev))
				}
				return ScanResult{Bytes: measurePaths(ctx, paths...), Note: fmt.Sprintf("%d revisions", len(revs))}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				for _, r := range disabledSnaps(ctx) {
					if err := sysutil.SudoRun(ctx, o, "snap", "remove", r.name, "--revision="+r.rev); err != nil {
						return err
					}
				}
				return nil
			},
		})
	}

	if commandExists("flatpak") {
		tasks = append(tasks, Task{
			ID: "flatpak-unused", Name: "Unused Flatpak runtimes", Group: "Snap & Flatpak",
			Detail: "flatpak uninstall --unused", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				n := countLines(captureOut(ctx, "flatpak", "list", "--runtime", "--columns=application"))
				_ = n
				return ScanResult{Bytes: -1, Note: "if any unused"}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				return sysutil.Run(ctx, o, "flatpak", "uninstall", "--unused", "-y")
			},
		})
	}

	// ~/.cache/thumbnails and a full ~/.cache wipe (opt-in, broader).
	tasks = append(tasks,
		Task{
			ID: "thumbnails", Name: "Thumbnail cache", Group: "Caches",
			Detail: "~/.cache/thumbnails", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx, "~/.cache/thumbnails")}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error { return removeAll(o, "~/.cache/thumbnails") },
		},
		Task{
			ID: "user-cache", Name: "User cache (~/.cache)", Group: "Caches",
			Detail: "may sign you out of some apps", Risk: OptIn,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx, "~/.cache")}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error { return emptyDir(o, "~/.cache") },
		},
	)

	if commandExists("docker") {
		tasks = append(tasks, dockerTask())
	}

	tasks = append(tasks, trashTaskLinux())

	// Report-only: duplicate apt repositories (deb822-aware; never auto-edited).
	if apt {
		tasks = append(tasks, Task{
			ID: "apt-repo-audit", Name: "Duplicate APT repositories", Group: "Repositories",
			Detail: "/etc/apt/sources.list.d", Risk: ReportOnly,
			Scan: func(ctx context.Context) ScanResult {
				dups := auditAPTRepos("/etc/apt/sources.list.d")
				if len(dups) == 0 {
					return ScanResult{Bytes: 0, Note: "none"}
				}
				return ScanResult{Bytes: -1, Note: fmt.Sprintf("%d duplicated across files", len(dups))}
			},
		})
	}

	return tasks
}

func rotatedLogsTask() Task {
	globs := []string{"/var/log/*.gz", "/var/log/*.1", "/var/log/*.old", "/var/log/**/*.gz"}
	return Task{
		ID: "rotated-logs", Name: "Rotated/compressed logs", Group: "Logs & crashes",
		Detail: "/var/log/*.gz, *.1", Risk: OptIn,
		Scan: func(ctx context.Context) ScanResult {
			return ScanResult{Bytes: measurePaths(ctx, globs...)}
		},
		Run: func(ctx context.Context, o sysutil.Opts) error {
			return sudoRemoveGlob(ctx, o, globs...)
		},
	}
}

func trashTaskLinux() Task {
	return Task{
		ID: "trash", Name: "Trash", Group: "Trash",
		Detail: "~/.local/share/Trash", Risk: OptIn,
		Scan: func(ctx context.Context) ScanResult {
			return ScanResult{Bytes: measurePaths(ctx, "~/.local/share/Trash")}
		},
		Run: func(ctx context.Context, o sysutil.Opts) error {
			return emptyDir(o, "~/.local/share/Trash/files", "~/.local/share/Trash/info")
		},
	}
}

// --- linux scan helpers ---

type snapRev struct{ name, rev string }

// disabledSnaps lists snap revisions marked "disabled" (superseded, safe to drop).
func disabledSnaps(ctx context.Context) []snapRev {
	out := captureOut(ctx, "snap", "list", "--all")
	var revs []snapRev
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		// Name Version Rev Tracking Publisher Notes
		if len(f) < 6 || f[0] == "Name" {
			continue
		}
		if strings.Contains(f[len(f)-1], "disabled") {
			revs = append(revs, snapRev{name: f[0], rev: f[2]})
		}
	}
	return revs
}

// pinRunningKernel marks the running kernel's image/header packages as manually
// installed so `apt-get autoremove --purge` won't remove the kernel we're booted
// on. Only installed packages are pinned — apt-mark errors on absent ones, and
// the headers aren't always present.
func pinRunningKernel(ctx context.Context, o sysutil.Opts) error {
	rel := captureOut(ctx, "uname", "-r")
	if rel == "" {
		return nil
	}
	for _, pkg := range []string{"linux-image-" + rel, "linux-headers-" + rel} {
		if !sysutil.IsPackageInstalled(pkg) {
			continue
		}
		if err := sysutil.SudoRun(ctx, o, "apt-mark", "manual", pkg); err != nil {
			return err
		}
	}
	return nil
}

// rcPackages lists packages in the "rc" state (removed, config files remain).
func rcPackages(ctx context.Context) []string {
	out := captureOut(ctx, "dpkg", "-l")
	var pkgs []string
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "rc ") {
			if f := strings.Fields(line); len(f) >= 2 {
				pkgs = append(pkgs, f[1])
			}
		}
	}
	return pkgs
}

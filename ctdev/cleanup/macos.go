package cleanup

import (
	"context"
	"fmt"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func macTasks(info platform.Info) []Task {
	var tasks []Task

	if commandExists("brew") {
		tasks = append(tasks, Task{
			ID: "brew", Name: "Homebrew old versions & cache", Group: "Packages",
			Detail: "brew cleanup + autoremove", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: parseBrewFreed(captureOut(ctx, "brew", "cleanup", "-n"))}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				if err := sysutil.Run(ctx, o, "brew", "cleanup"); err != nil {
					return err
				}
				return sysutil.Run(ctx, o, "brew", "autoremove")
			},
		})
	}

	tasks = append(tasks,
		Task{
			ID: "user-caches", Name: "Application caches", Group: "Caches",
			Detail: "~/Library/Caches", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx, "~/Library/Caches")}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error { return emptyDir(o, "~/Library/Caches") },
		},
		Task{
			ID: "user-logs", Name: "Application logs", Group: "Logs",
			Detail: "~/Library/Logs", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx, "~/Library/Logs")}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error { return emptyDir(o, "~/Library/Logs") },
		},
		Task{
			ID: "dev-caches", Name: "Developer tool caches", Group: "Developer",
			Detail: "npm, yarn, pip, go-build", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx,
					"~/.npm/_cacache", "~/Library/Caches/Yarn", "~/Library/Caches/pip", "~/Library/Caches/go-build")}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				return removeAll(o, "~/.npm/_cacache", "~/Library/Caches/Yarn", "~/Library/Caches/pip", "~/Library/Caches/go-build")
			},
		},
	)

	// Xcode is a big reclaim for developers; all of these regenerate.
	if pathExists("~/Library/Developer/Xcode") || pathExists("~/Library/Developer/CoreSimulator") {
		xcodePaths := []string{
			"~/Library/Developer/Xcode/DerivedData",
			"~/Library/Developer/Xcode/*DeviceSupport",
			"~/Library/Developer/CoreSimulator/Caches",
		}
		tasks = append(tasks, Task{
			ID: "xcode", Name: "Xcode DerivedData, device support, sim caches", Group: "Developer",
			Detail: "rebuildable Xcode junk", Risk: Safe,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx, xcodePaths...)}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				if err := removeAll(o, xcodePaths...); err != nil {
					return err
				}
				if commandExists("xcrun") && !o.DryRun {
					return sysutil.Run(ctx, o, "xcrun", "simctl", "delete", "unavailable")
				}
				return nil
			},
		})
	}

	if commandExists("docker") {
		tasks = append(tasks, dockerTask())
	}

	tasks = append(tasks,
		Task{
			ID: "trash", Name: "Trash", Group: "Trash",
			Detail: "~/.Trash", Risk: OptIn,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx, "~/.Trash")}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error { return emptyDir(o, "~/.Trash") },
		},
		Task{
			ID: "ds-store", Name: ".DS_Store files", Group: "Trash",
			Detail: "cosmetic Finder clutter", Risk: OptIn,
			Scan: func(ctx context.Context) ScanResult {
				home, _ := homeDir()
				n := countLines(captureOut(ctx, "find", home, "-name", ".DS_Store", "-type", "f"))
				return ScanResult{Bytes: -1, Note: fmt.Sprintf("%d files", n)}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				home, _ := homeDir()
				if o.DryRun {
					fmt.Fprintf(o.Stdout, "  [dry-run] find %s -name .DS_Store -delete\n", home)
					return nil
				}
				return sysutil.Run(ctx, o, "find", home, "-name", ".DS_Store", "-type", "f", "-delete")
			},
		},
		Task{
			ID: "system-caches", Name: "System caches", Group: "Caches",
			Detail: "/Library/Caches (sudo)", Risk: OptIn,
			Scan: func(ctx context.Context) ScanResult {
				return ScanResult{Bytes: measurePaths(ctx, "/Library/Caches")}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error { return sudoEmptyDir(ctx, o, "/Library/Caches") },
		},
	)

	if commandExists("tmutil") {
		tasks = append(tasks, Task{
			ID: "tm-snapshots", Name: "Time Machine local snapshots", Group: "Backups",
			Detail: "frees purgeable space", Risk: OptIn,
			Scan: func(ctx context.Context) ScanResult {
				snaps := localSnapshots(ctx)
				if len(snaps) == 0 {
					return ScanResult{Bytes: 0, Note: "none"}
				}
				return ScanResult{Bytes: -1, Note: fmt.Sprintf("%d snapshots", len(snaps))}
			},
			Run: func(ctx context.Context, o sysutil.Opts) error {
				for _, d := range localSnapshots(ctx) {
					if err := sysutil.SudoRun(ctx, o, "tmutil", "deletelocalsnapshots", d); err != nil {
						return err
					}
				}
				return nil
			},
		})
	}

	// Report-only: local iOS device backups are user data — surfaced, never deleted.
	tasks = append(tasks, Task{
		ID: "ios-backups", Name: "iOS device backups", Group: "Found (not cleaned)",
		Detail: "~/Library/Application Support/MobileSync/Backup", Risk: ReportOnly,
		Scan: func(ctx context.Context) ScanResult {
			return ScanResult{Bytes: measurePaths(ctx, "~/Library/Application Support/MobileSync/Backup"), Note: "user data"}
		},
	})

	return tasks
}

// localSnapshots returns the Time Machine local snapshot date tokens for /.
func localSnapshots(ctx context.Context) []string {
	out := captureOut(ctx, "tmutil", "listlocalsnapshots", "/")
	var dates []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		// "com.apple.TimeMachine.2026-06-26-120000.local"
		const prefix = "com.apple.TimeMachine."
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		d := strings.TrimPrefix(line, prefix)
		d = strings.TrimSuffix(d, ".local")
		if d != "" {
			dates = append(dates, d)
		}
	}
	return dates
}

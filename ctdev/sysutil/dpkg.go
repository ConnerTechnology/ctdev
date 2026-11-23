package sysutil

import (
	"context"
	"os/exec"
	"strings"
)

// DebVersion returns an installed Debian package's version, or "" when it
// isn't installed. `dpkg -s` alone isn't enough: `apt-get remove` leaves the
// package in dpkg's database in the config-files state, and `dpkg -s` still
// exits 0 for it.
func DebVersion(ctx context.Context, pkg string) string {
	out, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${db:Status-Status} ${Version}\n", pkg).Output()
	if err != nil {
		return ""
	}
	return parseDebStatusVersion(string(out))
}

// DebInstalled reports whether a Debian package is installed, as opposed to
// absent or removed with its config files left behind.
func DebInstalled(ctx context.Context, pkg string) bool {
	return DebVersion(ctx, pkg) != ""
}

// parseDebStatusVersion reads dpkg-query's "<status> <version>" lines, one per
// architecture the package is known for, and returns the first installed one.
func parseDebStatusVersion(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if ver, ok := strings.CutPrefix(strings.TrimSpace(line), "installed "); ok {
			return ver
		}
	}
	return ""
}

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
	out, err := exec.CommandContext(ctx, "dpkg-query", "-W", "-f=${db:Status-Status} ${Version}", pkg).Output()
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

// parseDebStatusVersion reads "<status> <version>" from dpkg-query.
func parseDebStatusVersion(out string) string {
	status, ver, _ := strings.Cut(strings.TrimSpace(out), " ")
	if status != "installed" {
		return ""
	}
	return ver
}

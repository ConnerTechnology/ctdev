package sysutil

import (
	"context"
	"os/exec"
	"strings"
)

// PiholeContainer is the docker container name used for a containerized Pi-hole.
const PiholeContainer = "pihole"

// PiholeContainerized reports whether Pi-hole runs as a docker container named
// "pihole" (as opposed to a native host install).
func PiholeContainerized() bool {
	if !CommandExists("docker") {
		return false
	}
	out, err := exec.Command("docker", "ps", "--filter", "name=^/"+PiholeContainer+"$", "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == PiholeContainer
}

// PiholeAvailable reports whether Pi-hole is present at all — either as the
// container or a native host install.
func PiholeAvailable() bool {
	return CommandExists("pihole") || PiholeContainerized()
}

// PiholeRun runs a Pi-hole family command (e.g. "pihole","allow",… or
// "pihole-FTL","--config",…) against the active Pi-hole: via `docker exec` when
// containerized, else with sudo on the host.
func PiholeRun(ctx context.Context, o Opts, args ...string) error {
	if PiholeContainerized() {
		return Run(ctx, o, "docker", append([]string{"exec", PiholeContainer}, args...)...)
	}
	return SudoRun(ctx, o, args[0], args[1:]...)
}

// PiholeCapture runs a Pi-hole family command and returns its stdout, targeting
// the container when containerized, else sudo on the host.
func PiholeCapture(ctx context.Context, args ...string) (string, error) {
	var cmd *exec.Cmd
	if PiholeContainerized() {
		cmd = exec.CommandContext(ctx, "docker", append([]string{"exec", PiholeContainer}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, "sudo", append([]string{"-n"}, args...)...)
	}
	out, err := cmd.Output()
	return string(out), err
}

// PiholeReload restarts the active Pi-hole so config changes take effect.
func PiholeReload(ctx context.Context, o Opts) error {
	if PiholeContainerized() {
		return Run(ctx, o, "docker", "restart", PiholeContainer)
	}
	return SudoRun(ctx, o, "systemctl", "restart", "pihole-FTL")
}

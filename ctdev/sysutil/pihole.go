package sysutil

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PiholeContainer is the docker container name used for a containerized Pi-hole.
const PiholeContainer = "pihole"

// ContainerRunning reports whether a docker container with the given name is
// currently running.
func ContainerRunning(name string) bool {
	if !CommandExists("docker") {
		return false
	}
	out, err := exec.Command("docker", "ps", "--filter", "name=^/"+name+"$", "--format", "{{.Names}}").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == name
}

// ContainerConfigImage returns the image reference a container was created
// from, or "" when no such container exists. This is the image as written at
// create time, so it still identifies the container after a local rebuild has
// moved the tag to a new image ID — which `docker ps` would report as a bare
// hash.
func ContainerConfigImage(ctx context.Context, name string) string {
	if !CommandExists("docker") {
		return ""
	}
	out, err := exec.CommandContext(ctx, "docker", "inspect", "-f", "{{.Config.Image}}", name).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// PiholeContainerized reports whether Pi-hole runs as a docker container named
// "pihole" (as opposed to a native host install).
func PiholeContainerized() bool {
	return ContainerRunning(PiholeContainer)
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
		cmd = SudoNoPrompt(ctx, args[0], args[1:]...)
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

// PiholeDnsmasqDir is where Pi-hole's dnsmasq drop-ins live: the bind-mounted
// ~/pihole/etc-dnsmasq.d for the container stack (no sudo needed), or
// /etc/dnsmasq.d for a native install.
func PiholeDnsmasqDir() string {
	if PiholeContainerized() {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, "pihole", "etc-dnsmasq.d")
	}
	return "/etc/dnsmasq.d"
}

// PiholeWriteDnsmasq writes (or, with empty content, removes) one dnsmasq
// drop-in by file name. Pi-hole reads the directory on restart — call
// PiholeReload afterwards.
func PiholeWriteDnsmasq(ctx context.Context, o Opts, name, content string) error {
	dest := filepath.Join(PiholeDnsmasqDir(), name)
	if content == "" {
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] remove %s\n", dest)
			return nil
		}
		if PiholeContainerized() {
			err := os.Remove(dest)
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		return SudoRun(ctx, o, "rm", "-f", dest)
	}
	if PiholeContainerized() {
		if o.DryRun {
			fmt.Fprintf(o.Stdout, "[dry-run] write dnsmasq drop-in → %s\n", dest)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, []byte(content), 0o644)
	}
	return SudoWriteFile(ctx, o, content, dest)
}

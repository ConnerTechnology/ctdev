package diagnose

import (
	"context"
	"strconv"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// The providers in this file need no credentials at all: Pi-hole answers on a
// local socket or through the container it runs in, and Docker answers on its
// own socket. They cost nothing to run and cover two of the most common causes
// of "the internet is broken" and "the server is broken" respectively.

// checkPihole reports on a Pi-hole running on this machine.
//
// A Pi-hole with blocking switched off, or one whose upstreams have stopped
// answering, presents to everyone in the house as "the internet is broken" —
// and it is the one piece of the chain nobody thinks to check.
func checkPihole(ctx context.Context, _ Facts) Result {
	if !sysutil.PiholeAvailable() {
		return skipf("no Pi-hole on this machine")
	}

	blocking, err := sysutil.PiholeCapture(ctx, "pihole-FTL", "--config", "dns.blocking.active")
	if err != nil {
		return skipf("could not read the Pi-hole configuration")
	}
	upstreams, _ := sysutil.PiholeCapture(ctx, "pihole-FTL", "--config", "dns.upstreams")

	if strings.TrimSpace(blocking) == "false" {
		return warnf("Re-enable it with 'pihole enable'. Someone probably disabled it temporarily and it stayed that way.",
			"Pi-hole is running but ad blocking is switched off")
	}

	list := parsePiholeUpstreams(upstreams)
	if len(list) == 0 {
		return failf("Set an upstream resolver in the Pi-hole settings. With none configured it cannot answer anything.",
			"Pi-hole has no upstream resolvers configured")
	}

	res := okf("blocking active, %d upstream(s): %s", len(list), strings.Join(list, ", "))
	res.Data = map[string]string{"upstreams": strings.Join(list, ", ")}
	return res
}

// parsePiholeUpstreams reads the bracketed list pihole-FTL prints, e.g.
// "[ 127.0.0.1#5335, 1.1.1.1 ]".
func parsePiholeUpstreams(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "[")
	raw = strings.TrimSuffix(raw, "]")

	var out []string
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

// checkDocker reports containers that are not simply running.
//
// ctdev already manages Docker stacks, so this is close to home: a container
// in a restart loop, one the kernel killed for memory, or one failing its own
// health check is the usual answer to "the server stopped working" — and
// `docker ps` alone doesn't make any of it obvious.
func checkDocker(ctx context.Context, _ Facts) Result {
	if !commandExists("docker") {
		return skipf("Docker is not installed")
	}
	out, err := captureErr(ctx, "docker", "ps", "-a", "--no-trunc",
		"--format", "{{.Names}}|{{.State}}|{{.Status}}")
	if err != nil {
		// Not being in the docker group is a permissions answer, not a fault.
		return skipf("could not query Docker (is this user in the docker group?)")
	}

	containers := parseDockerPS(out)
	if len(containers) == 0 {
		return skipf("no containers on this machine")
	}
	return dockerVerdict(containers)
}

// DockerContainer is one container's state as `docker ps` reports it.
type DockerContainer struct {
	Name   string
	State  string
	Status string
}

// Restarting reports a container caught in a restart loop. Docker keeps
// retrying forever, so this can persist for weeks without anyone noticing.
func (c DockerContainer) Restarting() bool { return c.State == "restarting" }

// Unhealthy reports a container failing the health check it defines itself.
func (c DockerContainer) Unhealthy() bool { return strings.Contains(c.Status, "(unhealthy)") }

// ExitedBadly reports a container that stopped with a non-zero status. Exit 0
// is a job that finished; anything else is a crash.
func (c DockerContainer) ExitedBadly() bool {
	if c.State != "exited" {
		return false
	}
	code, found := dockerExitCode(c.Status)
	return found && code != 0
}

// dockerExitCode pulls the code out of "Exited (127) 27 hours ago".
func dockerExitCode(status string) (int, bool) {
	_, rest, found := strings.Cut(status, "(")
	if !found {
		return 0, false
	}
	digits, _, found := strings.Cut(rest, ")")
	if !found {
		return 0, false
	}
	code, err := strconv.Atoi(strings.TrimSpace(digits))
	return code, err == nil
}

func parseDockerPS(out string) []DockerContainer {
	var list []DockerContainer
	for _, line := range lines(out) {
		parts := strings.SplitN(line, "|", 3)
		if len(parts) != 3 {
			continue
		}
		list = append(list, DockerContainer{
			Name:   strings.TrimSpace(parts[0]),
			State:  strings.TrimSpace(parts[1]),
			Status: strings.TrimSpace(parts[2]),
		})
	}
	return list
}

func dockerVerdict(containers []DockerContainer) Result {
	var restarting, unhealthy, crashed []string
	running := 0

	for _, c := range containers {
		switch {
		case c.Restarting():
			restarting = append(restarting, c.Name)
		case c.Unhealthy():
			unhealthy = append(unhealthy, c.Name)
		case c.ExitedBadly():
			crashed = append(crashed, c.Name)
		case c.State == "running":
			running++
		}
	}

	switch {
	case len(restarting) > 0:
		return failf("Docker will retry these forever without telling anyone. 'docker logs <name>' shows why it keeps dying.",
			"%s stuck in a restart loop", strings.Join(restarting, ", "))

	case len(unhealthy) > 0:
		return failf("The container is running but failing its own health check, so whatever depends on it is broken too.",
			"%s reporting unhealthy", strings.Join(unhealthy, ", "))

	case len(crashed) > 0:
		return warnf("These exited with an error rather than finishing. 'docker logs <name>' has the reason.",
			"%s exited with an error", strings.Join(crashed, ", "))

	default:
		return okf("%d of %d containers running", running, len(containers))
	}
}

// checkDockerLogs catches unbounded container logs eating the disk.
//
// Docker's default json-file driver has no size limit, so one chatty container
// can quietly consume a filesystem over months. It presents as a full disk with
// no obvious culprit, because the files live somewhere nobody looks.
func checkDockerLogs(ctx context.Context, f Facts) Result {
	if !commandExists("docker") {
		return skipf("Docker is not installed")
	}
	if !f.Root {
		// The log files are root-owned; say so rather than guessing.
		return skipf("needs root — re-run with sudo to measure container logs")
	}

	out, gotRoot := sudoCapture(ctx, "du", "-sk", "/var/lib/docker/containers")
	if !gotRoot {
		return skipf("needs root — re-run with sudo to measure container logs")
	}

	kb := parseLeadingKB(out)
	if kb <= 0 {
		return skipf("could not measure container logs")
	}

	const logWarnKB = 5 << 20 // 5 GiB
	if kb >= logWarnKB {
		return warnf("Docker's default log driver has no size limit. Set max-size in /etc/docker/daemon.json, then recreate the containers.",
			"container logs and metadata use %s", sysutil.HumanKB(kb))
	}
	return okf("container logs use %s", sysutil.HumanKB(kb))
}

// checkTailscale reports what Tailscale itself knows about this machine's path
// to the internet — NAT type and relay use — which explains slow or failing
// peer connections that look like nothing from the local stack.
func checkTailscale(ctx context.Context, _ Facts) Result {
	if !commandExists("tailscale") {
		return skipf("Tailscale is not installed")
	}
	out, err := captureErr(ctx, "tailscale", "status", "--json")
	if err != nil || out == "" {
		return skipf("Tailscale is installed but not running")
	}
	if strings.Contains(out, `"BackendState": "NeedsLogin"`) {
		return warnf("Run 'tailscale up' to sign in. Anything relying on the tailnet is unreachable until then.",
			"Tailscale is installed but not signed in")
	}
	if !strings.Contains(out, `"BackendState": "Running"`) {
		return infof("Tailscale is installed but not connected")
	}
	return okf("connected")
}

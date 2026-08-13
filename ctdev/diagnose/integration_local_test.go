package diagnose

import (
	"strings"
	"testing"
)

func TestParsePiholeUpstreams(t *testing.T) {
	// pihole-FTL prints a bracketed list; the local recursive resolver form
	// carries a port suffix that has to survive.
	got := parsePiholeUpstreams("[ 127.0.0.1#5335, 1.1.1.1 ]")
	if len(got) != 2 || got[0] != "127.0.0.1#5335" || got[1] != "1.1.1.1" {
		t.Errorf("parsePiholeUpstreams = %v", got)
	}
	if got := parsePiholeUpstreams("[]"); len(got) != 0 {
		t.Errorf("empty list = %v, want none", got)
	}
}

func TestParseDockerPS(t *testing.T) {
	// Real `docker ps -a` output from a machine with a crashed devcontainer.
	const out = `foil-it-up_devcontainer-devcontainer-1|exited|Exited (127) 27 hours ago
foil-it-up_devcontainer-redis-1|running|Up 17 hours (healthy)
foil-it-up_devcontainer-postgres-1|running|Up 17 hours (healthy)`

	got := parseDockerPS(out)
	if len(got) != 3 {
		t.Fatalf("got %d containers, want 3", len(got))
	}
	if !got[0].ExitedBadly() {
		t.Error("exit 127 is a crash")
	}
	if got[1].ExitedBadly() || got[1].Unhealthy() || got[1].Restarting() {
		t.Errorf("a healthy running container should be flagged for nothing: %+v", got[1])
	}
}

func TestDockerExitCode(t *testing.T) {
	tests := map[string]int{
		"Exited (127) 27 hours ago": 127,
		"Exited (0) 2 days ago":     0,
		"Up 17 hours (healthy)":     -1,
	}
	for status, want := range tests {
		code, found := dockerExitCode(status)
		if want < 0 {
			if found {
				t.Errorf("%q should carry no exit code", status)
			}
			continue
		}
		if !found || code != want {
			t.Errorf("dockerExitCode(%q) = %d/%v, want %d", status, code, found, want)
		}
	}
}

func TestDockerVerdict(t *testing.T) {
	healthy := []DockerContainer{
		{Name: "redis", State: "running", Status: "Up 17 hours (healthy)"},
		{Name: "web", State: "running", Status: "Up 2 days"},
	}
	if got := dockerVerdict(healthy).Severity; got != OK {
		t.Errorf("healthy stack = %v, want OK", got)
	}

	// A container Docker retries forever is the worst case: it never
	// surfaces an error anywhere anyone looks.
	loop := append([]DockerContainer{}, healthy...)
	loop = append(loop, DockerContainer{Name: "worker", State: "restarting", Status: "Restarting (1) 3 seconds ago"})
	res := dockerVerdict(loop)
	if res.Severity != Fail || !strings.Contains(res.Detail, "worker") {
		t.Errorf("restart loop = %v %q", res.Severity, res.Detail)
	}

	unhealthy := append([]DockerContainer{}, healthy...)
	unhealthy[0].Status = "Up 17 hours (unhealthy)"
	if got := dockerVerdict(unhealthy).Severity; got != Fail {
		t.Errorf("unhealthy container = %v, want Fail", got)
	}

	// A job that finished cleanly is not a fault.
	finished := append([]DockerContainer{}, healthy...)
	finished = append(finished, DockerContainer{Name: "migrate", State: "exited", Status: "Exited (0) 2 days ago"})
	if got := dockerVerdict(finished).Severity; got != OK {
		t.Errorf("cleanly exited job = %v, want OK", got)
	}

	crashed := append([]DockerContainer{}, healthy...)
	crashed = append(crashed, DockerContainer{Name: "api", State: "exited", Status: "Exited (137) 1 hour ago"})
	if got := dockerVerdict(crashed).Severity; got != Warn {
		t.Errorf("crashed container = %v, want Warn", got)
	}
}

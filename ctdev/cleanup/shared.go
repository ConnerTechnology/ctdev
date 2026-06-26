package cleanup

import (
	"context"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// dockerTask prunes dangling images, stopped containers, unused networks, and
// build cache (not the `-a` "all unused images" sweep). Shared by both OSes.
func dockerTask() Task {
	return Task{
		ID: "docker-prune", Name: "Docker dangling images & build cache", Group: "Docker",
		Detail: "docker system prune", Risk: OptIn,
		Scan: func(ctx context.Context) ScanResult {
			return ScanResult{Bytes: dockerReclaimable(ctx)}
		},
		Run: func(ctx context.Context, o sysutil.Opts) error {
			if err := sysutil.Run(ctx, o, "docker", "system", "prune", "-f"); err != nil {
				return err
			}
			return sysutil.Run(ctx, o, "docker", "builder", "prune", "-f")
		},
	}
}

// dockerReclaimable sums the reclaimable column from `docker system df`. Returns
// -1 when the daemon isn't reachable.
func dockerReclaimable(ctx context.Context) int64 {
	out := captureOut(ctx, "docker", "system", "df", "--format", "{{.Reclaimable}}")
	if out == "" {
		return -1
	}
	var total int64
	any := false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// "1.8GB (72%)" -> "1.8GB"
		if idx := strings.Index(line, "("); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if b := parseSize(line); b >= 0 {
			total += b
			any = true
		}
	}
	if !any {
		return -1
	}
	return total
}

package setup

import (
	"context"
	"os/exec"
	"strings"
)

// runOutput is a read-only probe helper used by apply logic that needs to
// parse command output (e.g. `systemctl is-enabled`). It bypasses sysutil.Run
// deliberately because we need stdout as a string, not streamed to a writer.
func runOutput(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// startDetached spawns a long-running background process without waiting for
// it. Used for daemons like xbindkeys that should stay alive after ctdev
// returns. Errors are silently dropped because the caller cannot meaningfully
// react — the alternative is leaving a stale foreground process.
func startDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}

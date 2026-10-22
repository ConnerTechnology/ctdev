package setup

import (
	"context"

	"github.com/ConnerTechnology/ctdev/ctdev/gpu"
)

// NvidiaSigningNotNeeded reports why this machine doesn't need the NVIDIA
// driver signed, or "" when it does. Signing only matters under Secure Boot
// (a legacy BIOS boot has none), and a machine whose Secure Boot state can't
// be read is treated as needing it. `configure gpu` also uses it to skip the
// signing flow.
func NvidiaSigningNotNeeded(ctx context.Context) string {
	if gpu.SecureBootState(ctx) == gpu.SecureBootOff {
		return "Secure Boot is off, so the driver loads unsigned"
	}
	return ""
}

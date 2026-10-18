package setup

import (
	"context"

	"github.com/ConnerTechnology/ctdev/ctdev/gpu"
)

// NvidiaSigningNotNeeded reports why this machine doesn't need the NVIDIA
// driver signed, or "" when it does. Signing only matters under Secure Boot,
// and a machine whose Secure Boot state can't be read is treated as needing
// it. `configure gpu` also uses it to skip the signing flow.
func NvidiaSigningNotNeeded(ctx context.Context) string {
	if gpu.IsSecureBootDisabled(ctx) {
		return "Secure Boot is off, so the driver loads unsigned"
	}
	return ""
}

// nvidiaSuspendNotNeeded reports why this machine doesn't need the NVIDIA
// suspend services, or "" when it does: they only matter on a machine that
// can suspend.
func nvidiaSuspendNotNeeded(ctx context.Context) string {
	if detectSuspendMasked(ctx) == "enabled" {
		return "suspend is masked, so this machine never suspends"
	}
	return ""
}

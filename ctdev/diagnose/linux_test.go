package diagnose

import "testing"

// The failure this guards against is silent: without the memory controller,
// Docker reports every per-container CPU, memory and network figure as zero,
// which renders as idle containers rather than as broken instrumentation.
func TestMemoryCgroupVerdict(t *testing.T) {
	// The controller list a stock Raspberry Pi OS boot produces — the firmware
	// prepends cgroup_disable=memory to the device-tree bootargs.
	pi := []string{"cpuset", "cpu", "io", "pids"}
	res := memoryCgroupVerdict(pi)
	if res.Severity != Warn {
		t.Errorf("missing memory controller = %v, want Warn", res.Severity)
	}
	if res.Advice == "" {
		t.Error("a warning the user cannot act on is not worth printing")
	}

	fixed := memoryCgroupVerdict([]string{"cpuset", "cpu", "io", "memory", "pids"})
	if fixed.Severity != OK {
		t.Errorf("memory controller present = %v, want OK", fixed.Severity)
	}
}

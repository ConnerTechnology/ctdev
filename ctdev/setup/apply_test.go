package setup

import "testing"

func TestGrubVarCommand(t *testing.T) {
	args := grubVarArgs("GRUB_TIMEOUT", "10")
	if len(args) == 0 {
		t.Error("expected non-empty args")
	}
}

func TestDconfWriteArgs(t *testing.T) {
	args := dconfWriteArgs("/org/cinnamon/desktop/sound/event-sounds", "false")
	if args[0] != "dconf" {
		t.Errorf("expected dconf, got %s", args[0])
	}
}

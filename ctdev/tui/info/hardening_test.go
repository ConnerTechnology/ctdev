package info

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderDiskBar_NegativeAndZeroWidthNoPanic(t *testing.T) {
	got := ansi.Strip(renderDiskBar(-15, 10))
	if got != strings.Repeat("░", 10) {
		t.Errorf("renderDiskBar(-15, 10) = %q, want 10 empty blocks", got)
	}
	if got := renderDiskBar(50, 0); got != "" {
		t.Errorf("renderDiskBar(50, 0) = %q, want empty", got)
	}
}

func TestParseDiskInfo_PreservesSpacesInMountPaths(t *testing.T) {
	dfOut := `Filesystem 1024-blocks Used Available Capacity Mounted on
/dev/disk3s1 500000000 200000000 300000000 40% /
/dev/disk4s1 100000000 50000000 50000000 50% /Volumes/Macintosh HD
`
	disks := parseDiskInfo(dfOut)
	if len(disks) != 2 {
		t.Fatalf("got %d disks, want 2: %+v", len(disks), disks)
	}
	if disks[0].Mount != "/" || disks[1].Mount != "/Volumes/Macintosh HD" {
		t.Errorf("mounts = [%q, %q], want [/ /Volumes/Macintosh HD]", disks[0].Mount, disks[1].Mount)
	}
}

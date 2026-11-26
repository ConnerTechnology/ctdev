package backupscreen

import (
	"errors"
	"strings"
	"testing"

	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/charmbracelet/x/ansi"
)

func TestStatus_UnconfiguredMissingDestNeverBackedUpAndReadErr(t *testing.T) {
	t.Run("unconfigured folder with no destinations", func(t *testing.T) {
		got := ansi.Strip(Status(backup.Status{
			Folder: backup.Folder{Cwd: "/home/u/docs"},
			Target: "/home/u/docs",
		}, "/home/u"))
		for _, want := range []string{
			"Not backed up: ~/docs",
			"none on this computer",
			"Last backup", "never",
			"none: this folder isn't configured",
			"Backups", "none",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in:\n%s", want, got)
			}
		}
	})

	t.Run("missing destination and read error", func(t *testing.T) {
		got := ansi.Strip(Status(backup.Status{
			Folder:             backup.Folder{Cwd: "/home/u/docs", Root: "/home/u/docs", Config: &backup.Config{Destination: "nas", Schedule: backup.Daily}},
			Target:             "/home/u/docs",
			Destination:        "nas",
			DestinationMissing: true,
			ReadErr:            errors.New("permission denied"),
		}, "/home/u"))
		if !strings.Contains(got, "nas (not set up on this computer)") {
			t.Errorf("missing DestinationMissing message:\n%s", got)
		}
		if !strings.Contains(got, "permission denied") {
			t.Errorf("missing ReadErr in status:\n%s", got)
		}
	})
}

func TestThousands_Boundaries(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{999, "999"},
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{-1000, "-1,000"},
	}
	for _, tc := range cases {
		if got := thousands(tc.n); got != tc.want {
			t.Errorf("thousands(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

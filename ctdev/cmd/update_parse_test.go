package cmd

import (
	"testing"
)

func TestParseAPTUpgradable(t *testing.T) {
	output := "Listing...\nlibglib2.0-0/noble-updates 2.80.5-1ubuntu1 amd64 [upgradable from: 2.80.0-6ubuntu3.1]\nlinux-image-6.8.0-50-generic/noble-updates 6.8.0-50.51 amd64 [upgradable from: 6.8.0-49.49]\n"

	items := parseAPTUpgradable(output)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "libglib2.0-0" {
		t.Errorf("expected libglib2.0-0, got %s", items[0].Name)
	}
	if items[0].Source != "apt" {
		t.Errorf("expected source apt, got %s", items[0].Source)
	}
	if !items[1].IsKernel {
		t.Error("expected linux-image to be flagged as kernel")
	}
}

func TestParseAPTUpgradableEmpty(t *testing.T) {
	items := parseAPTUpgradable("Listing...\n")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseBrewOutdated(t *testing.T) {
	output := "node (21.0.0) < 22.0.0\npython (3.12.0) < 3.13.0\n"

	items := parseBrewOutdated(output)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "node" {
		t.Errorf("expected node, got %s", items[0].Name)
	}
	if items[0].CurrentVer != "21.0.0" {
		t.Errorf("expected 21.0.0, got %s", items[0].CurrentVer)
	}
	if items[0].NewVer != "22.0.0" {
		t.Errorf("expected 22.0.0, got %s", items[0].NewVer)
	}
}

func TestParseBrewOutdatedEmpty(t *testing.T) {
	items := parseBrewOutdated("")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseFlatpakUpdates(t *testing.T) {
	output := "org.mozilla.firefox\t132.0\norg.signal.Signal\t7.30.0"

	items := parseFlatpakUpdates(output)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "org.mozilla.firefox" {
		t.Errorf("expected firefox, got %s", items[0].Name)
	}
}

func TestParseFlatpakUpdatesEmpty(t *testing.T) {
	items := parseFlatpakUpdates("")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseNPMOutdatedJSON(t *testing.T) {
	content := `{
  "typescript": {
    "current": "5.3.0",
    "wanted": "5.4.0",
    "latest": "5.4.0",
    "dependent": "global",
    "location": "/usr/lib"
  }
}`
	items, err := parseNPMOutdatedJSON(content)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Name != "typescript" {
		t.Errorf("expected typescript, got %s", items[0].Name)
	}
	if items[0].CurrentVer != "5.3.0" {
		t.Errorf("expected 5.3.0, got %s", items[0].CurrentVer)
	}
}

func TestParseNPMOutdatedJSONEmpty(t *testing.T) {
	items, err := parseNPMOutdatedJSON("{}")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"3.16.1", 3},
		{"", 0},
		{"10.0.0", 10},
		{"0.1.2", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := majorVersion(tt.input)
		if got != tt.want {
			t.Errorf("majorVersion(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseBrewCaskOutdatedEmpty(t *testing.T) {
	items := parseBrewCaskOutdated("")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseBrewCaskOutdatedUnmanagedCask(t *testing.T) {
	// A cask not in managedCasks should be filtered out
	items := parseBrewCaskOutdated("some-unknown-cask (1.0.0) != 2.0.0\n")
	if len(items) != 0 {
		t.Errorf("expected 0 items for unmanaged cask, got %d", len(items))
	}
}

func TestParseAPTUpgradableMalformedLine(t *testing.T) {
	// Line with [upgradable but missing "from:" — currentVer should be empty
	output := "pkg/noble 2.0 amd64 [upgradable]\n"
	items := parseAPTUpgradable(output)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].CurrentVer != "" {
		t.Errorf("expected empty currentVer, got %q", items[0].CurrentVer)
	}
}

func TestParseAPTUpgradableNoUpgradableKeyword(t *testing.T) {
	items := parseAPTUpgradable("some random line\nanother line\n")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseBrewOutdatedFewerThan4Fields(t *testing.T) {
	// Lines with fewer than 4 fields should be skipped
	items := parseBrewOutdated("node (21.0.0)\nonly-two fields\n")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseNPMOutdatedJSONInvalid(t *testing.T) {
	_, err := parseNPMOutdatedJSON("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

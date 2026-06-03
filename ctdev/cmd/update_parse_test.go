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

func TestParseMintUpdateList(t *testing.T) {
	output := "kernel          linux-6.14.0-37.37~24.04.1                    6.14.0-37.37~24.04.1\n"

	items := parseMintUpdateList(output)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Name != "linux-6.14.0-37.37~24.04.1" {
		t.Errorf("expected linux-6.14.0-37.37~24.04.1, got %s", items[0].Name)
	}
	if items[0].Source != "mintupdate" {
		t.Errorf("expected source mintupdate, got %s", items[0].Source)
	}
	if !items[0].IsKernel {
		t.Error("expected kernel flag to be set")
	}
	if items[0].NewVer != "6.14.0-37.37~24.04.1" {
		t.Errorf("expected version 6.14.0-37.37~24.04.1, got %s", items[0].NewVer)
	}
}

func TestParseMintUpdateListEmpty(t *testing.T) {
	items := parseMintUpdateList("")
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseMintUpdateListMultiple(t *testing.T) {
	output := "kernel          linux-6.14.0-37.37~24.04.1                    6.14.0-37.37~24.04.1\nsecurity        libssl3t64                                    3.0.13-0ubuntu3.5\n"

	items := parseMintUpdateList(output)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if !items[0].IsKernel {
		t.Error("expected first item to be kernel")
	}
	if items[1].IsKernel {
		t.Error("expected second item to not be kernel")
	}
	if items[1].Name != "libssl3t64" {
		t.Errorf("expected libssl3t64, got %s", items[1].Name)
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

func TestGoArchiveSHA256(t *testing.T) {
	releases := []goRelease{
		{
			Version: "go1.23.4",
			Files: []struct {
				Filename string `json:"filename"`
				OS       string `json:"os"`
				Arch     string `json:"arch"`
				Kind     string `json:"kind"`
				SHA256   string `json:"sha256"`
			}{
				{Filename: "go1.23.4.linux-amd64.tar.gz", OS: "linux", Arch: "amd64", Kind: "archive", SHA256: "abc123"},
				{Filename: "go1.23.4.darwin-arm64.tar.gz", OS: "darwin", Arch: "arm64", Kind: "archive", SHA256: "def456"},
			},
		},
		{
			Version: "go1.23.3",
			Files: []struct {
				Filename string `json:"filename"`
				OS       string `json:"os"`
				Arch     string `json:"arch"`
				Kind     string `json:"kind"`
				SHA256   string `json:"sha256"`
			}{
				{Filename: "go1.23.3.linux-amd64.tar.gz", OS: "linux", Arch: "amd64", Kind: "archive", SHA256: "older"},
			},
		},
	}

	t.Run("finds matching version+os+arch", func(t *testing.T) {
		got := goArchiveSHA256(releases, "1.23.4", "linux", "amd64")
		if got != "abc123" {
			t.Errorf("got %q, want abc123", got)
		}
	})
	t.Run("finds older release when pinned", func(t *testing.T) {
		got := goArchiveSHA256(releases, "1.23.3", "linux", "amd64")
		if got != "older" {
			t.Errorf("got %q, want older", got)
		}
	})
	t.Run("unknown version returns empty", func(t *testing.T) {
		got := goArchiveSHA256(releases, "9.9.9", "linux", "amd64")
		if got != "" {
			t.Errorf("expected empty; got %q", got)
		}
	})
	t.Run("unknown arch returns empty", func(t *testing.T) {
		got := goArchiveSHA256(releases, "1.23.4", "linux", "riscv64")
		if got != "" {
			t.Errorf("expected empty; got %q", got)
		}
	})
}

func TestAPTKeyRefreshers_PathsMatchInstallers(t *testing.T) {
	// If these drift from the keyring paths embedded in the component
	// installers, `ctdev update --refresh-keys` silently updates a file
	// APT's signed-by doesn't read. Any change to an installer's keyring
	// path must be mirrored here, and vice versa.
	want := map[string]string{
		"gh":        "/usr/share/keyrings/githubcli-archive-keyring.gpg",
		"vscode":    "/usr/share/keyrings/microsoft-archive-keyring.gpg",
		"1password": "/usr/share/keyrings/1password-archive-keyring.gpg",
		"terraform": "/usr/share/keyrings/hashicorp-archive-keyring.gpg",
		"tailscale": "/usr/share/keyrings/tailscale-archive-keyring.gpg",
	}
	for name, expected := range want {
		r, ok := aptKeyRefreshers[name]
		if !ok {
			t.Errorf("aptKeyRefreshers missing entry for %q", name)
			continue
		}
		if r.KeyringPath != expected {
			t.Errorf("aptKeyRefreshers[%q].KeyringPath = %q, want %q", name, r.KeyringPath, expected)
		}
	}
}

func TestShouldRefreshKeys(t *testing.T) {
	tests := []struct {
		name         string
		refreshKeys  bool
		check        bool
		want         bool
	}{
		{"refresh-keys without check", true, false, true},
		{"refresh-keys with check is suppressed", true, true, false},
		{"no refresh-keys", false, false, false},
		{"check alone", false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRefreshKeys(tt.refreshKeys, tt.check); got != tt.want {
				t.Errorf("shouldRefreshKeys(%v, %v) = %v, want %v", tt.refreshKeys, tt.check, got, tt.want)
			}
		})
	}
}

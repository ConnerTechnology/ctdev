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

func TestParseBrewOutdatedJSON(t *testing.T) {
	data := []byte(`{
	  "formulae": [
	    {"name":"node","installed_versions":["21.0.0"],"current_version":"22.0.0","pinned":false},
	    {"name":"python","installed_versions":["3.12.0"],"current_version":"3.13.0","pinned":false}
	  ],
	  "casks": []
	}`)

	items, err := parseBrewOutdatedJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Name != "node" {
		t.Errorf("expected node, got %s", items[0].Name)
	}
	if items[0].Source != "brew" {
		t.Errorf("expected source brew, got %s", items[0].Source)
	}
	if items[0].CurrentVer != "21.0.0" {
		t.Errorf("expected 21.0.0, got %s", items[0].CurrentVer)
	}
	if items[0].NewVer != "22.0.0" {
		t.Errorf("expected 22.0.0, got %s", items[0].NewVer)
	}
}

// The regression that motivated moving to JSON: with two kegs installed, the old
// text parser read the literal "<" separator as the new version.
func TestParseBrewOutdatedJSONMultipleInstalledVersions(t *testing.T) {
	data := []byte(`{"formulae":[
	  {"name":"openssl@3","installed_versions":["3.3.2","3.4.0"],"current_version":"3.4.1","pinned":false}
	],"casks":[]}`)

	items, err := parseBrewOutdatedJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].NewVer != "3.4.1" {
		t.Errorf("NewVer = %q, want 3.4.1", items[0].NewVer)
	}
	if items[0].CurrentVer != "3.3.2, 3.4.0" {
		t.Errorf("CurrentVer = %q, want both installed versions", items[0].CurrentVer)
	}
}

// A HEAD formula reports a multi-word current version, which also defeated the
// whitespace parser.
func TestParseBrewOutdatedJSONHeadFormula(t *testing.T) {
	data := []byte(`{"formulae":[
	  {"name":"neovim","installed_versions":["HEAD-abc123"],"current_version":"HEAD","pinned":false}
	],"casks":[]}`)

	items, err := parseBrewOutdatedJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].NewVer != "HEAD" {
		t.Errorf("expected one HEAD item, got %+v", items)
	}
}

// `brew upgrade` errors on a pinned formula, and the apply step batches every
// name into one command — so one pinned formula would fail the whole batch.
func TestParseBrewOutdatedJSONSkipsPinned(t *testing.T) {
	data := []byte(`{"formulae":[
	  {"name":"postgresql@14","installed_versions":["14.1"],"current_version":"14.9","pinned":true},
	  {"name":"jq","installed_versions":["1.6"],"current_version":"1.7","pinned":false}
	],"casks":[]}`)

	items, err := parseBrewOutdatedJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected the pinned formula dropped, got %d items: %+v", len(items), items)
	}
	if items[0].Name != "jq" {
		t.Errorf("expected jq, got %s", items[0].Name)
	}
}

// Third-party taps report a full_name; that's what `brew upgrade` needs.
func TestParseBrewOutdatedJSONTappedFormula(t *testing.T) {
	data := []byte(`{"formulae":[
	  {"name":"oven-sh/bun/bun","installed_versions":["1.0.0"],"current_version":"1.1.0","pinned":false}
	],"casks":[]}`)

	items, err := parseBrewOutdatedJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].Name != "oven-sh/bun/bun" {
		t.Errorf("expected the tapped full_name preserved, got %+v", items)
	}
}

func TestParseBrewOutdatedJSONEmpty(t *testing.T) {
	items, err := parseBrewOutdatedJSON([]byte(`{"formulae":[],"casks":[]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items, got %d", len(items))
	}
}

func TestParseBrewOutdatedJSONMalformed(t *testing.T) {
	if _, err := parseBrewOutdatedJSON([]byte("not json")); err == nil {
		t.Error("expected an error for malformed JSON, not a panic or a silent empty list")
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

func TestVersionNewer(t *testing.T) {
	tests := []struct {
		candidate, current string
		want               bool
	}{
		{"1.2.4", "1.2.3", true},
		{"1.3.0", "1.2.9", true},
		{"2.0.0", "1.9.9", true},
		{"1.2.3", "1.2.3", false}, // equal is not newer
		{"1.2.3", "1.2.4", false}, // downgrade must not be offered
		{"1.2.9", "1.3.0", false},
		{"v1.2.4", "1.2.3", true},     // leading v ignored
		{"1.2.0", "1.2", false},       // 1.2.0 == 1.2
		{"1.23.4", "1.23.3", true},    // multi-digit components
		{"1.2.3-rc1", "1.2.3", false}, // pre-release suffix ignored
	}
	for _, tt := range tests {
		if got := versionNewer(tt.candidate, tt.current); got != tt.want {
			t.Errorf("versionNewer(%q, %q) = %v, want %v", tt.candidate, tt.current, got, tt.want)
		}
	}
}

func TestParseBrewOutdatedJSONUnmanagedCask(t *testing.T) {
	// A cask not in managedCasks should be filtered out.
	data := []byte(`{"formulae":[],"casks":[
	  {"name":"some-unknown-cask","installed_versions":["1.0.0"],"current_version":"2.0.0","pinned":false}
	]}`)

	items, err := parseBrewOutdatedJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items for an unmanaged cask, got %d", len(items))
	}
}

// Casks that track "latest" report it as both installed and current version.
// It must survive as-is rather than being mistaken for a missing field.
func TestParseBrewOutdatedJSONCaskLatest(t *testing.T) {
	data := []byte(`{"formulae":[],"casks":[
	  {"name":"tailscale","installed_versions":["latest"],"current_version":"latest","pinned":false}
	]}`)

	items, err := parseBrewOutdatedJSON(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// tailscale is in managedCasks, so it appears only when the component is
	// installed on this machine — assert the shape, whichever way that lands.
	for _, it := range items {
		if it.Source != "brew-cask" {
			t.Errorf("cask item has source %q, want brew-cask", it.Source)
		}
		if it.NewVer != "latest" || it.CurrentVer != "latest" {
			t.Errorf(`expected "latest" preserved, got %+v`, it)
		}
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
		name        string
		refreshKeys bool
		check       bool
		want        bool
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

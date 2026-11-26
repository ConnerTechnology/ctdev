package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSource(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestAPTSignedByPaths(t *testing.T) {
	dir := t.TempDir()

	// One-line format written by GitHub's own instructions, not ctdev's path.
	writeSource(t, dir, "github-cli.list",
		"deb [arch=amd64 signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\n")
	// deb822 format the vscode package rewrites over ctdev's .list file.
	writeSource(t, dir, "vscode.sources", `### THIS FILE IS AUTOMATICALLY CONFIGURED ###
Types: deb
URIs: https://packages.microsoft.com/repos/code
Suites: stable
Components: main
Architectures: amd64
Signed-By: /usr/share/keyrings/microsoft.gpg
`)
	// 1Password ships both: a .sources file and a .list holding only a
	// commented-out deb line. The comment must not count.
	writeSource(t, dir, "1password.list",
		"# deb [arch=amd64 signed-by=/tmp/commented-out.gpg] https://downloads.1password.com/linux/debian/amd64 stable main\n")
	writeSource(t, dir, "1password.sources", `Types: deb
URIs: https://downloads.1password.com/linux/debian/amd64
Suites: stable
Components: main
Signed-By: /usr/share/keyrings/1password-archive-keyring.gpg
`)
	// Two files for one repo, both real: refresh both, once each.
	writeSource(t, dir, "hashicorp.list",
		"deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com noble main\n")
	writeSource(t, dir, "hashicorp-old.list",
		"deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com jammy main\n"+
			"deb-src [signed-by=/etc/apt/keyrings/hashicorp-src.gpg] https://apt.releases.hashicorp.com jammy main\n")
	// Inline armored key in a deb822 stanza is not a path.
	writeSource(t, dir, "docker.sources", `Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: noble
Components: stable
Signed-By:
 -----BEGIN PGP PUBLIC KEY BLOCK-----
 .
 mQINBFit2ioBEADhWpZ8/wvZ6hUTiXOwQHXMAlaFHcPH9hAtr4F1y2+OYdbtMuth
 -----END PGP PUBLIC KEY BLOCK-----
`)
	// Unrelated file that must never match.
	writeSource(t, dir, "ubuntu.sources", `Types: deb
URIs: http://archive.ubuntu.com/ubuntu
Suites: noble
Components: main
Signed-By: /usr/share/keyrings/ubuntu-archive-keyring.gpg
`)

	tests := []struct {
		name string
		repo string
		want []string
	}{
		{"one-line file with a foreign path", "https://cli.github.com/packages",
			[]string{"/etc/apt/keyrings/githubcli-archive-keyring.gpg"}},
		{"deb822 file", "https://packages.microsoft.com/repos/code",
			[]string{"/usr/share/keyrings/microsoft.gpg"}},
		{"commented deb line is ignored", "https://downloads.1password.com/linux/debian",
			[]string{"/usr/share/keyrings/1password-archive-keyring.gpg"}},
		{"several files, deduplicated", "https://apt.releases.hashicorp.com",
			[]string{"/etc/apt/keyrings/hashicorp-src.gpg", "/usr/share/keyrings/hashicorp-archive-keyring.gpg"}},
		{"inline key is not a path", "https://download.docker.com/linux/ubuntu", nil},
		{"repo not on disk", "https://pkgs.tailscale.com/stable/ubuntu", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aptSignedByPaths(dir, tt.repo)
			if len(got) != len(tt.want) {
				t.Fatalf("aptSignedByPaths(%q) = %v, want %v", tt.repo, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("aptSignedByPaths(%q)[%d] = %q, want %q", tt.repo, i, got[i], tt.want[i])
				}
			}
		})
	}

	t.Run("missing directory yields nothing", func(t *testing.T) {
		if got := aptSignedByPaths(filepath.Join(dir, "nope"), "https://cli.github.com/packages"); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestAPTKeyringTargets(t *testing.T) {
	dir := t.TempDir()
	writeSource(t, dir, "github-cli.list",
		"deb [signed-by=/etc/apt/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main\n")

	t.Run("source on disk wins over the installer default", func(t *testing.T) {
		got := aptKeyringTargets(dir, aptKeyRefreshers["gh"])
		want := []string{"/etc/apt/keyrings/githubcli-archive-keyring.gpg"}
		if len(got) != 1 || got[0] != want[0] {
			t.Errorf("got %v, want %v", got, want)
		}
	})
	t.Run("falls back to the installer path when no source references the repo", func(t *testing.T) {
		got := aptKeyringTargets(dir, aptKeyRefreshers["terraform"])
		want := aptKeyRefreshers["terraform"].KeyringPath
		if len(got) != 1 || got[0] != want {
			t.Errorf("got %v, want [%s]", got, want)
		}
	})
}

package component

// APTRepoKey holds the signing-key and repository URL metadata shared between
// APT-backed component installers and `ctdev update --refresh-keys` so they stay
// in sync by construction.
type APTRepoKey struct {
	KeyURL      string
	KeyringPath string
	RepoURL     string
}

// APTRepoKeys maps component names to their APT signing key and repo URL prefix.
var APTRepoKeys = map[string]APTRepoKey{
	"gh": {
		KeyURL:      "https://cli.github.com/packages/githubcli-archive-keyring.gpg",
		KeyringPath: "/usr/share/keyrings/githubcli-archive-keyring.gpg",
		RepoURL:     "https://cli.github.com/packages",
	},
	"vscode": {
		KeyURL:      "https://packages.microsoft.com/keys/microsoft.asc",
		KeyringPath: "/usr/share/keyrings/microsoft-archive-keyring.gpg",
		RepoURL:     "https://packages.microsoft.com/repos/code",
	},
	"1password": {
		KeyURL:      "https://downloads.1password.com/linux/keys/1password.asc",
		KeyringPath: "/usr/share/keyrings/1password-archive-keyring.gpg",
		RepoURL:     "https://downloads.1password.com/linux/debian",
	},
	"terraform": {
		KeyURL:      "https://apt.releases.hashicorp.com/gpg",
		KeyringPath: "/usr/share/keyrings/hashicorp-archive-keyring.gpg",
		RepoURL:     "https://apt.releases.hashicorp.com",
	},
	"tailscale": {
		KeyURL:      "https://pkgs.tailscale.com/stable/ubuntu/noble.noarmor.gpg",
		KeyringPath: "/usr/share/keyrings/tailscale-archive-keyring.gpg",
		RepoURL:     "https://pkgs.tailscale.com/stable",
	},
}

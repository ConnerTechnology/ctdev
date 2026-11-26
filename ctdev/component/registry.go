package component

import (
	"context"

	"github.com/ConnerTechnology/ctdev/ctdev/nodenv"
)

// Registry lists every component ctdev can install. Root is omitted on most
// entries: the zero value (RootWhenMissing) describes a package-manager
// install, which is what the majority are. Declare it when the component does
// privileged work on every run (RootAlways) or none at all (RootNever) — see
// RootNeed.
var Registry = []Component{
	{Name: "1password", Description: "1Password password manager", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectApps: []string{"/Applications/1Password.app"}, BrewNeedsRoot: true, GoInstall: onePasswordInstall, GoUninstall: onePasswordUninstall, Tags: []string{"password", "security"}},
	{Name: "age", Description: "age file encryption tool", Category: CategorySecurity, SupportedOS: []OS{OSAny}, GoInstall: ageInstall, GoUninstall: ageUninstall, Tags: []string{"encryption", "crypto"}},
	{Name: "aws", Description: "AWS CLI v2", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Detect: awsV2Installed, GoInstall: awsInstall, GoUninstall: awsUninstall, Tags: []string{"amazon", "cloud"}},
	{Name: "beszel", Description: "Beszel server/container monitoring (Docker)", Category: CategoryInfra, SupportedOS: []OS{OSLinux}, Dependencies: []string{"docker"}, DetectPath: "$HOME/beszel/docker-compose.yml", Root: RootNever, GoInstall: beszelInstall, GoUninstall: beszelUninstall, Tags: []string{"docker", "monitoring", "homelab"}},
	{Name: "betterbird", Description: "Betterbird email client (Thunderbird fork)", Category: CategoryDesktop, SupportedOS: []OS{OSLinux}, DetectPath: "/opt/betterbird/betterbird", GoInstall: betterbirdInstall, GoUninstall: betterbirdUninstall, Tags: []string{"email", "thunderbird"}},
	{Name: "btop", Description: "Resource monitor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("btop"), GoUninstall: SimplePackageUninstaller("btop"), Tags: []string{"monitor", "htop"}},
	{Name: "bat", Description: "cat with syntax highlighting", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: batInstall, GoUninstall: batUninstall, Tags: []string{"cat", "pager"}},
	{Name: "bun", Description: "JavaScript runtime and package manager", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Root: RootNever, GoInstall: bunInstall, GoUninstall: bunUninstall, Tags: []string{"javascript", "node"}},
	{Name: "caddy", Description: "Caddy reverse proxy (Cloudflare DNS-01 wildcard)", Category: CategoryInfra, SupportedOS: []OS{OSLinux}, Dependencies: []string{"docker"}, DetectPath: "$HOME/caddy/docker-compose.yml", Root: RootAlways, GoInstall: caddyInstall, GoUninstall: caddyUninstall, Tags: []string{"caddy", "proxy", "homelab"}},
	{Name: "ccstatusline", Description: "Claude Code status line", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"nodenv"}, Detect: npmPackageDetect("ccstatusline", "ccstatusline"), Root: RootNever, GoInstall: ccstatuslineInstall, GoUninstall: ccstatuslineUninstall, Tags: []string{"ai", "claude", "statusline"}},
	{Name: "ccusage", Description: "Claude Code token usage and cost reports", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"nodenv"}, Detect: npmPackageDetect("ccusage", "ccusage"), Root: RootNever, GoInstall: npmGlobalInstaller("ccusage", "ccusage"), GoUninstall: npmGlobalUninstaller("ccusage", "ccusage"), Tags: []string{"ai", "claude", "usage"}},
	{Name: "chrome", Description: "Google Chrome browser", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectCmd: "google-chrome", DetectApps: []string{"/Applications/Google Chrome.app"}, GoInstall: chromeInstall, GoUninstall: chromeUninstall, Tags: []string{"browser", "web"}},
	{Name: "cleanmymac", Description: "CleanMyMac system cleaner", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, DetectApps: []string{"/Applications/CleanMyMac.app", "/Applications/CleanMyMac X.app"}, BrewNeedsRoot: true, GoInstall: cleanmymacInstall, GoUninstall: cleanmymacUninstall, Tags: []string{"cleanup", "disk"}},
	{Name: "claude-code", Description: "Claude Code CLI and configuration", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"ccstatusline"}, DetectCmd: "claude", Root: RootNever, GoInstall: claudeCodeInstall, GoUninstall: claudeCodeUninstall, Tags: []string{"ai", "anthropic"}},
	{Name: "claude-desktop", Description: "Claude desktop application", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS, OSLinux}, DetectCmd: "claude-desktop", DetectApps: []string{"/Applications/Claude.app"}, GoInstall: claudeDesktopInstall, GoUninstall: claudeDesktopUninstall, Tags: []string{"ai", "anthropic"}},
	{Name: "codex", Description: "OpenAI Codex CLI coding agent", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"nodenv"}, Detect: npmPackageDetect("codex", "@openai/codex"), Root: RootNever, GoInstall: npmGlobalInstaller("codex", "@openai/codex"), GoUninstall: npmGlobalUninstaller("codex", "@openai/codex"), Tags: []string{"ai", "openai"}},
	{Name: "dbeaver", Description: "DBeaver database tool", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectApps: []string{"/Applications/DBeaver.app"}, GoInstall: dbeaverInstall, GoUninstall: dbeaverUninstall, Tags: []string{"database", "sql"}},
	{Name: "devcontainer", Description: "Dev Containers CLI + dx wrapper", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"nodenv"}, Detect: npmPackageDetect("devcontainer", "@devcontainers/cli"), Root: RootNever, GoInstall: devcontainerInstall, GoUninstall: devcontainerUninstall, Tags: []string{"docker", "vscode"}},
	{Name: "direnv", Description: "Per-directory environment variables", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("direnv"), GoUninstall: SimplePackageUninstaller("direnv"), Tags: []string{"env", "shell"}},
	{Name: "docker", Description: "Docker container runtime", Category: CategoryCLI, SupportedOS: []OS{OSAny}, DetectApps: []string{"/Applications/Docker.app"}, BrewNeedsRoot: true, GoInstall: dockerInstall, GoUninstall: dockerUninstall, Tags: []string{"container", "devops"}},
	{Name: "doctl", Description: "DigitalOcean CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: doctlInstall, GoUninstall: doctlUninstall, Tags: []string{"cloud", "digitalocean"}},
	{Name: "earlyoom", Description: "Early OOM killer for Linux", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: earlyoomInstall, GoUninstall: earlyoomUninstall, Tags: []string{"memory", "oom"}},
	{Name: "endorctl", Description: "Endor Labs CLI (dependency and secret scanning)", Category: CategorySecurity, SupportedOS: []OS{OSAny}, GoInstall: endorctlInstall, GoUninstall: endorctlUninstall, Tags: []string{"sca", "scanning"}},
	{Name: "fd", Description: "Fast, friendly find alternative", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: fdInstall, GoUninstall: fdUninstall, Tags: []string{"find", "search"}},
	{Name: "ffmpeg", Description: "Audio and video converter and recorder", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("ffmpeg"), GoUninstall: SimplePackageUninstaller("ffmpeg"), Tags: []string{"video", "audio"}},
	{Name: "fzf", Description: "Fuzzy finder for the shell", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("fzf"), GoUninstall: SimplePackageUninstaller("fzf"), Tags: []string{"fuzzy", "search"}},
	{Name: "fonts", Description: "Nerd Fonts for terminal", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectApps: []string{"$HOME/.local/share/fonts/FiraCodeNerdFont-Bold.ttf", "$HOME/Library/Fonts/FiraCodeNerdFont-Bold.ttf"}, Root: RootNever, GoInstall: fontsInstall, GoUninstall: fontsUninstall, Tags: []string{"nerd", "terminal"}},
	{Name: "antigravity-cli", Description: "Google Antigravity CLI coding agent (agy)", Category: CategoryCLI, SupportedOS: []OS{OSAny}, DetectCmd: "agy", Root: RootNever, GoInstall: antigravityInstall, GoUninstall: antigravityUninstall, Tags: []string{"ai", "google"}},
	{Name: "gh", Description: "GitHub CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: ghInstall, GoUninstall: ghUninstall, Tags: []string{"github", "git"}},
	{Name: "git", Description: "Git configuration and aliases", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, GoInstall: gitInstall, GoUninstall: gitUninstall, Tags: []string{"vcs", "version"}},
	{Name: "git-spice", Description: "Git Spice stacked branches tool", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Detect: func() bool { return isGitSpiceInstalled(context.Background()) }, GoInstall: gitSpiceInstall, GoUninstall: gitSpiceUninstall, Tags: []string{"git", "stacked"}},
	// No DetectPath: DetectPath is exclusive in IsInstalled, so pinning it to
	// the tarball location made a distro-packaged Go read as missing. goInstalled
	// checks both /usr/local/go/bin/go and PATH, matching goInstall.
	{Name: "go", Description: "Go toolchain (official tarball)", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, Detect: goInstalled, GoInstall: goInstall, GoUninstall: goUninstall, Tags: []string{"golang", "runtime"}},
	{Name: "gws", Description: "Google Workspace CLI (Drive, Gmail, Calendar, and more)", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: gwsInstall, GoUninstall: gwsUninstall, Tags: []string{"google", "workspace"}},
	{Name: "helm", Description: "Kubernetes package manager", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"kubectl"}, GoInstall: helmInstall, GoUninstall: helmUninstall, Tags: []string{"kubernetes", "k8s"}},
	{Name: "herdr", Description: "Background runtime for coding agents: sessions keep running when you disconnect", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: herdrInstall, GoUninstall: herdrUninstall, Tags: []string{"agents", "sessions"}},
	{Name: "herdr-desktop", Description: "Herdr desktop app: terminals and coding agents that keep running when you close the window", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS, OSLinux}, Detect: herdrDesktopInstalled, Root: RootNever, GoInstall: herdrDesktopInstall, GoUninstall: herdrDesktopUninstall, Tags: []string{"agents", "terminal"}},
	{Name: "jq", Description: "JSON processor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("jq"), GoUninstall: SimplePackageUninstaller("jq"), Tags: []string{"json", "parser"}},
	{Name: "kopia", Description: "Kopia backup tool, pinned binary only", Category: CategoryInfra, SupportedOS: []OS{OSLinux}, Detect: kopiaPinnedInstalled, GoInstall: kopiaInstall, GoUninstall: kopiaUninstall, Tags: []string{"backup", "kopia"}},
	{Name: "kubectl", Description: "Kubernetes CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: kubectlInstall, GoUninstall: kubectlUninstall, Tags: []string{"kubernetes", "k8s"}},
	{Name: "lazygit", Description: "Terminal UI for git", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: lazygitInstall, GoUninstall: lazygitUninstall, Tags: []string{"git", "tui"}},
	{Name: "linear", Description: "Linear issue tracker", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, DetectApps: []string{"/Applications/Linear.app"}, GoInstall: linearInstall, GoUninstall: linearUninstall, Tags: []string{"issues", "project"}},
	{Name: "logi-options", Description: "Logitech Options+", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, DetectApps: []string{"/Applications/Logi Options+.app", "/Applications/Logi Options.app", "/Applications/logioptionsplus.app"}, BrewNeedsRoot: true, GoInstall: logiOptionsInstall, GoUninstall: logiOptionsUninstall, Tags: []string{"logitech", "mouse"}},
	{Name: "mosh", Description: "Mobile shell (SSH that survives roaming/sleep)", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("mosh"), GoUninstall: SimplePackageUninstaller("mosh"), Tags: []string{"ssh", "remote"}},
	{Name: "nodenv", Description: "nodenv and node-build (Node version manager)", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, Detect: nodenv.Available, Root: RootNever, GoInstall: nodenvInstall, GoUninstall: nodenvUninstall, Tags: []string{"javascript", "nodejs", "node"}},
	{Name: "nomachine", Description: "NoMachine remote desktop server", Category: CategoryDesktop, SupportedOS: []OS{OSLinux}, DetectPath: "/usr/NX/bin/nxserver", Root: RootAlways, GoInstall: nomachineInstall, GoUninstall: nomachineUninstall, Tags: []string{"remote", "desktop", "vnc"}},
	{Name: "orca", Description: "Orca: run coding agents in parallel worktrees", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS, OSLinux}, Detect: orcaInstalled, GoInstall: orcaInstall, GoUninstall: orcaUninstall, Tags: []string{"ai", "agents"}},
	{Name: "paper", Description: "Paper design tool for interfaces and prototypes", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS, OSLinux}, DetectCmd: "paper-desktop", DetectApps: []string{"/Applications/Paper.app"}, GoInstall: paperInstall, GoUninstall: paperUninstall, Tags: []string{"design", "ui"}},
	{Name: "pihole", Description: "Pi-hole network-wide DNS ad blocker (Docker)", Category: CategoryInfra, SupportedOS: []OS{OSLinux}, Dependencies: []string{"docker"}, DetectPath: "$HOME/pihole/docker-compose.yml", Root: RootNever, GoInstall: piholeInstall, GoUninstall: piholeUninstall, Tags: []string{"dns", "adblock", "homelab"}},
	{Name: "portainer", Description: "Portainer CE Docker management UI (Docker)", Category: CategoryInfra, SupportedOS: []OS{OSLinux}, Dependencies: []string{"docker"}, DetectPath: "$HOME/portainer/docker-compose.yml", Root: RootNever, GoInstall: portainerInstall, GoUninstall: portainerUninstall, Tags: []string{"docker", "ui", "homelab"}},
	{Name: "python", Description: "Python via pyenv", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "$HOME/.pyenv", GoInstall: pythonInstall, GoUninstall: pythonUninstall, Tags: []string{"pyenv", "python3"}},
	{Name: "restic", Description: "restic backup program", Category: CategoryInfra, SupportedOS: []OS{OSMacOS, OSLinux}, Detect: resticInstalled, Root: RootNever, GoInstall: resticInstall, GoUninstall: resticUninstall, Tags: []string{"backup", "snapshot"}},
	{Name: "ripgrep", Description: "Fast recursive grep", Category: CategoryCLI, SupportedOS: []OS{OSAny}, DetectCmd: "rg", GoInstall: ripgrepInstall, GoUninstall: SimplePackageUninstaller("ripgrep"), Tags: []string{"grep", "search"}},
	{Name: "ruby", Description: "Ruby via rbenv", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "$HOME/.rbenv", GoInstall: rubyInstall, GoUninstall: rubyUninstall, Tags: []string{"rbenv", "rails"}},
	{Name: "rust", Description: "Rust toolchain via rustup (rustc, cargo)", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "$HOME/.cargo/bin/rustup", GoInstall: rustInstall, GoUninstall: rustUninstall, Tags: []string{"rustup", "cargo"}},
	{Name: "shellcheck", Description: "Shell script linter", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("shellcheck"), GoUninstall: SimplePackageUninstaller("shellcheck"), Tags: []string{"lint", "bash"}},
	{Name: "slack", Description: "Slack messaging", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS, OSLinux}, DetectApps: []string{"/Applications/Slack.app"}, GoInstall: slackInstall, GoUninstall: slackUninstall, Tags: []string{"messaging", "chat"}},
	{Name: "smartmontools", Description: "SMART disk-health monitoring (smartd)", Category: CategorySystem, SupportedOS: []OS{OSLinux}, DetectCmd: "smartctl", Root: RootAlways, GoInstall: smartmontoolsInstall, GoUninstall: smartmontoolsUninstall, Tags: []string{"disk", "health", "homelab"}},
	{Name: "solaar", Description: "Logitech Unifying/Bolt receiver manager", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: solaarInstall, GoUninstall: solaarUninstall, Tags: []string{"logitech", "bluetooth"}},
	{Name: "sops", Description: "Mozilla SOPS secrets manager", Category: CategorySecurity, SupportedOS: []OS{OSAny}, GoInstall: sopsInstall, GoUninstall: sopsUninstall, Tags: []string{"secrets", "encrypt"}},
	{Name: "ssh", Description: "SSH server, started at boot (Remote Login on macOS)", Category: CategorySystem, SupportedOS: []OS{OSAny}, BrewNeedsRoot: true, Detect: sshServerStartsAtBoot, Root: RootAlways, GoInstall: sshInstall, GoUninstall: sshUninstall, Tags: []string{"ssh", "remote", "server"}},
	{Name: "syncthing", Description: "Peer-to-peer file sync between your machines", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: syncthingInstall, GoUninstall: syncthingUninstall, Tags: []string{"sync", "files"}},
	{Name: "t3code", Description: "T3 Code CLI: one UI for Claude Code, Codex and other coding agents", Category: CategoryCLI, SupportedOS: []OS{OSMacOS, OSLinux}, Detect: t3codeInstalled, Root: RootNever, GoInstall: t3codeInstall, GoUninstall: t3codeUninstall, Tags: []string{"ai", "agents"}},
	{Name: "t3code-desktop", Description: "T3 Code desktop app", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS, OSLinux}, Detect: t3codeDesktopInstalled, GoInstall: t3codeDesktopInstall, GoUninstall: t3codeDesktopUninstall, Tags: []string{"ai", "agents"}},
	{Name: "tailscale", Description: "Tailscale VPN", Category: CategorySecurity, SupportedOS: []OS{OSAny}, BrewNeedsRoot: true, GoInstall: tailscaleInstall, GoUninstall: tailscaleUninstall, Tags: []string{"vpn", "network"}},
	{Name: "terraform", Description: "Terraform infrastructure tool", Category: CategoryInfra, SupportedOS: []OS{OSAny}, GoInstall: terraformInstall, GoUninstall: terraformUninstall, Tags: []string{"iac", "cloud"}},
	{Name: "tmux", Description: "Terminal multiplexer", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: tmuxInstall, GoUninstall: tmuxUninstall, Tags: []string{"terminal", "session"}},
	{Name: "tradingview", Description: "TradingView desktop charts", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectCmd: "tradingview", DetectApps: []string{"/Applications/TradingView.app"}, GoInstall: tradingviewInstall, GoUninstall: tradingviewUninstall, Tags: []string{"trading", "charts", "finance"}},
	{Name: "vscode", Description: "Visual Studio Code", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectCmd: "code", DetectApps: []string{"/Applications/Visual Studio Code.app"}, GoInstall: vscodeInstall, GoUninstall: vscodeUninstall, Tags: []string{"editor", "ide"}},
	{Name: "weston", Description: "Weston Wayland compositor", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: SimplePackageInstaller("weston"), GoUninstall: SimplePackageUninstaller("weston"), Tags: []string{"wayland", "compositor"}},
	{Name: "zoxide", Description: "Smarter cd that learns your directories", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("zoxide"), GoUninstall: SimplePackageUninstaller("zoxide"), Tags: []string{"cd", "shell"}},
	{Name: "zsh", Description: "Zsh, Oh My Zsh, Pure prompt, plugins", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "$HOME/.oh-my-zsh", GoInstall: zshInstall, GoUninstall: zshUninstall, Tags: []string{"shell", "ohmyzsh"}},
}

func InstalledSet() map[string]bool {
	installed := make(map[string]bool)
	for i := range Registry {
		if Registry[i].IsInstalled() {
			installed[Registry[i].Name] = true
		}
	}
	return installed
}

func FindByName(name string) *Component {
	for i := range Registry {
		if Registry[i].Name == name {
			return &Registry[i]
		}
	}
	return nil
}

func AllNames() []string {
	names := make([]string, len(Registry))
	for i, c := range Registry {
		names[i] = c.Name
	}
	return names
}

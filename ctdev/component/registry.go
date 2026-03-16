package component

var Registry = []Component{
	{Name: "1password", Description: "1Password password manager", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/1password/install.sh", BashUninstall: "components/1password/uninstall.sh", Tags: []string{"password", "security"}},
	{Name: "age", Description: "age file encryption tool", Category: CategorySecurity, SupportedOS: []OS{OSAny}, BashInstall: "components/age/install.sh", BashUninstall: "components/age/uninstall.sh", Tags: []string{"encryption", "crypto"}},
	{Name: "btop", Description: "Resource monitor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: btopInstall, GoUninstall: btopUninstall, Tags: []string{"monitor", "htop"}},
	{Name: "bun", Description: "JavaScript runtime and package manager", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/bun/install.sh", BashUninstall: "components/bun/uninstall.sh", Tags: []string{"javascript", "node"}},
	{Name: "chatgpt", Description: "ChatGPT desktop application", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, GoInstall: chatgptInstall, GoUninstall: chatgptUninstall, Tags: []string{"ai", "openai"}},
	{Name: "chrome", Description: "Google Chrome browser", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectCmd: "google-chrome", BashInstall: "components/chrome/install.sh", BashUninstall: "components/chrome/uninstall.sh", Tags: []string{"browser", "web"}},
	{Name: "cleanmymac", Description: "CleanMyMac system cleaner", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, GoInstall: cleanmymacInstall, GoUninstall: cleanmymacUninstall, Tags: []string{"cleanup", "disk"}},
	{Name: "claude-code", Description: "Claude Code CLI and configuration", Category: CategoryCLI, SupportedOS: []OS{OSAny}, DetectCmd: "claude", BashInstall: "components/claude-code/install.sh", BashUninstall: "components/claude-code/uninstall.sh", Tags: []string{"ai", "anthropic"}},
	{Name: "claude-desktop", Description: "Claude desktop application", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, GoInstall: claudeDesktopInstall, GoUninstall: claudeDesktopUninstall, Tags: []string{"ai", "anthropic"}},
	{Name: "codex", Description: "OpenAI Codex CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/codex/install.sh", BashUninstall: "components/codex/uninstall.sh", Tags: []string{"ai", "openai"}},
	{Name: "dbeaver", Description: "DBeaver database tool", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/dbeaver/install.sh", BashUninstall: "components/dbeaver/uninstall.sh", Tags: []string{"database", "sql"}},
	{Name: "docker", Description: "Docker container runtime", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/docker/install.sh", BashUninstall: "components/docker/uninstall.sh", Tags: []string{"container", "devops"}},
	{Name: "doctl", Description: "DigitalOcean CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/doctl/install.sh", BashUninstall: "components/doctl/uninstall.sh", Tags: []string{"cloud", "digitalocean"}},
	{Name: "earlyoom", Description: "Early OOM killer for Linux", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: earlyoomInstall, GoUninstall: earlyoomUninstall, Tags: []string{"memory", "oom"}},
	{Name: "fonts", Description: "Nerd Fonts for terminal", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "$HOME/.local/share/fonts/FiraCodeNerdFont-Bold.ttf", BashInstall: "components/fonts/install.sh", BashUninstall: "components/fonts/uninstall.sh", Tags: []string{"nerd", "terminal"}},
	{Name: "gh", Description: "GitHub CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/gh/install.sh", BashUninstall: "components/gh/uninstall.sh", Tags: []string{"github", "git"}},
	{Name: "ghostty", Description: "Ghostty terminal emulator", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/ghostty/install.sh", BashUninstall: "components/ghostty/uninstall.sh", Tags: []string{"terminal", "emulator"}},
	{Name: "git", Description: "Git configuration and aliases", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, BashInstall: "components/git/install.sh", BashUninstall: "components/git/uninstall.sh", Tags: []string{"vcs", "version"}},
	{Name: "git-spice", Description: "Git Spice stacked branches tool", Category: CategoryCLI, SupportedOS: []OS{OSAny}, DetectCmd: "gs", BashInstall: "components/git-spice/install.sh", BashUninstall: "components/git-spice/uninstall.sh", Tags: []string{"git", "stacked"}},
	{Name: "helm", Description: "Kubernetes package manager", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"kubectl"}, BashInstall: "components/helm/install.sh", BashUninstall: "components/helm/uninstall.sh", Tags: []string{"kubernetes", "k8s"}},
	{Name: "jq", Description: "JSON processor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: jqInstall, GoUninstall: jqUninstall, Tags: []string{"json", "parser"}},
	{Name: "kubectl", Description: "Kubernetes CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, BashInstall: "components/kubectl/install.sh", BashUninstall: "components/kubectl/uninstall.sh", Tags: []string{"kubernetes", "k8s"}},
	{Name: "linear", Description: "Linear issue tracker", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, GoInstall: linearInstall, GoUninstall: linearUninstall, Tags: []string{"issues", "project"}},
	{Name: "logi-options", Description: "Logitech Options+", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, GoInstall: logiOptionsInstall, GoUninstall: logiOptionsUninstall, Tags: []string{"logitech", "mouse"}},
	{Name: "node", Description: "Node.js via nodenv", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, BashInstall: "components/node/install.sh", BashUninstall: "components/node/uninstall.sh", Tags: []string{"javascript", "nodejs"}},
	{Name: "ruby", Description: "Ruby via rbenv", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, BashInstall: "components/ruby/install.sh", BashUninstall: "components/ruby/uninstall.sh", Tags: []string{"rbenv", "rails"}},
	{Name: "shellcheck", Description: "Shell script linter", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: shellcheckInstall, GoUninstall: shellcheckUninstall, Tags: []string{"lint", "bash"}},
	{Name: "slack", Description: "Slack messaging", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, BashInstall: "components/slack/install.sh", BashUninstall: "components/slack/uninstall.sh", Tags: []string{"messaging", "chat"}},
	{Name: "solaar", Description: "Logitech Unifying/Bolt receiver manager", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: solaarInstall, GoUninstall: solaarUninstall, Tags: []string{"logitech", "bluetooth"}},
	{Name: "sops", Description: "Mozilla SOPS secrets manager", Category: CategorySecurity, SupportedOS: []OS{OSAny}, BashInstall: "components/sops/install.sh", BashUninstall: "components/sops/uninstall.sh", Tags: []string{"secrets", "encrypt"}},
	{Name: "tailscale", Description: "Tailscale VPN", Category: CategorySecurity, SupportedOS: []OS{OSAny}, BashInstall: "components/tailscale/install.sh", BashUninstall: "components/tailscale/uninstall.sh", Tags: []string{"vpn", "network"}},
	{Name: "terraform", Description: "Terraform infrastructure tool", Category: CategoryInfra, SupportedOS: []OS{OSAny}, BashInstall: "components/terraform/install.sh", BashUninstall: "components/terraform/uninstall.sh", Tags: []string{"iac", "cloud"}},
	{Name: "tmux", Description: "Terminal multiplexer", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: tmuxInstall, GoUninstall: tmuxUninstall, Tags: []string{"terminal", "session"}},
	{Name: "vscode", Description: "Visual Studio Code", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectCmd: "code", BashInstall: "components/vscode/install.sh", BashUninstall: "components/vscode/uninstall.sh", Tags: []string{"editor", "ide"}},
	{Name: "zsh", Description: "Zsh, Oh My Zsh, Pure prompt, plugins", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "$HOME/.oh-my-zsh", BashInstall: "components/zsh/install.sh", BashUninstall: "components/zsh/uninstall.sh", Tags: []string{"shell", "ohmyzsh"}},
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

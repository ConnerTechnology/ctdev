package component

var Registry = []Component{
	{Name: "1password", Description: "1Password password manager", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectApps: []string{"/Applications/1Password.app"}, GoInstall: onePasswordInstall, GoUninstall: onePasswordUninstall, Tags: []string{"password", "security"}},
	{Name: "age", Description: "age file encryption tool", Category: CategorySecurity, SupportedOS: []OS{OSAny}, GoInstall: ageInstall, GoUninstall: ageUninstall, Tags: []string{"encryption", "crypto"}},
	{Name: "btop", Description: "Resource monitor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("btop"), GoUninstall: SimplePackageUninstaller("btop"), Tags: []string{"monitor", "htop"}},
	{Name: "bun", Description: "JavaScript runtime and package manager", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: bunInstall, GoUninstall: bunUninstall, Tags: []string{"javascript", "node"}},
	{Name: "chatgpt", Description: "ChatGPT desktop application", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, DetectApps: []string{"/Applications/ChatGPT.app"}, GoInstall: chatgptInstall, GoUninstall: chatgptUninstall, Tags: []string{"ai", "openai"}},
	{Name: "chrome", Description: "Google Chrome browser", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectCmd: "google-chrome", DetectApps: []string{"/Applications/Google Chrome.app"}, GoInstall: chromeInstall, GoUninstall: chromeUninstall, Tags: []string{"browser", "web"}},
	{Name: "cleanmymac", Description: "CleanMyMac system cleaner", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, DetectApps: []string{"/Applications/CleanMyMac.app", "/Applications/CleanMyMac X.app"}, GoInstall: cleanmymacInstall, GoUninstall: cleanmymacUninstall, Tags: []string{"cleanup", "disk"}},
	{Name: "claude-code", Description: "Claude Code CLI and configuration", Category: CategoryCLI, SupportedOS: []OS{OSAny}, DetectCmd: "claude", GoInstall: claudeCodeInstall, GoUninstall: claudeCodeUninstall, Tags: []string{"ai", "anthropic"}},
	{Name: "claude-desktop", Description: "Claude desktop application", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, DetectApps: []string{"/Applications/Claude.app"}, GoInstall: claudeDesktopInstall, GoUninstall: claudeDesktopUninstall, Tags: []string{"ai", "anthropic"}},
	{Name: "codex", Description: "OpenAI Codex CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: codexInstall, GoUninstall: codexUninstall, Tags: []string{"ai", "openai"}},
	{Name: "dbeaver", Description: "DBeaver database tool", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectApps: []string{"/Applications/DBeaver.app"}, GoInstall: dbeaverInstall, GoUninstall: dbeaverUninstall, Tags: []string{"database", "sql"}},
	{Name: "devcontainer", Description: "Dev Containers CLI + dx wrapper", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"node"}, DetectCmd: "devcontainer", GoInstall: devcontainerInstall, GoUninstall: devcontainerUninstall, Tags: []string{"docker", "vscode"}},
	{Name: "docker", Description: "Docker container runtime", Category: CategoryCLI, SupportedOS: []OS{OSAny}, DetectApps: []string{"/Applications/Docker.app"}, GoInstall: dockerInstall, GoUninstall: dockerUninstall, Tags: []string{"container", "devops"}},
	{Name: "doctl", Description: "DigitalOcean CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: doctlInstall, GoUninstall: doctlUninstall, Tags: []string{"cloud", "digitalocean"}},
	{Name: "earlyoom", Description: "Early OOM killer for Linux", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: earlyoomInstall, GoUninstall: earlyoomUninstall, Tags: []string{"memory", "oom"}},
	{Name: "fonts", Description: "Nerd Fonts for terminal", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectApps: []string{"$HOME/.local/share/fonts/FiraCodeNerdFont-Bold.ttf", "$HOME/Library/Fonts/FiraCodeNerdFont-Bold.ttf"}, GoInstall: fontsInstall, GoUninstall: fontsUninstall, Tags: []string{"nerd", "terminal"}},
	{Name: "gh", Description: "GitHub CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: ghInstall, GoUninstall: ghUninstall, Tags: []string{"github", "git"}},
	{Name: "ghostty", Description: "Ghostty terminal emulator", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectApps: []string{"/Applications/Ghostty.app"}, GoInstall: ghosttyInstall, GoUninstall: ghosttyUninstall, Tags: []string{"terminal", "emulator"}},
	{Name: "git", Description: "Git configuration and aliases", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, GoInstall: gitInstall, GoUninstall: gitUninstall, Tags: []string{"vcs", "version"}},
	{Name: "git-spice", Description: "Git Spice stacked branches tool", Category: CategoryCLI, SupportedOS: []OS{OSAny}, DetectCmd: "gs", GoInstall: gitSpiceInstall, GoUninstall: gitSpiceUninstall, Tags: []string{"git", "stacked"}},
	{Name: "go", Description: "Go toolchain (official tarball)", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "/usr/local/go/bin/go", GoInstall: goInstall, GoUninstall: goUninstall, Tags: []string{"golang", "runtime"}},
	{Name: "helm", Description: "Kubernetes package manager", Category: CategoryCLI, SupportedOS: []OS{OSAny}, Dependencies: []string{"kubectl"}, GoInstall: helmInstall, GoUninstall: helmUninstall, Tags: []string{"kubernetes", "k8s"}},
	{Name: "jq", Description: "JSON processor", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("jq"), GoUninstall: SimplePackageUninstaller("jq"), Tags: []string{"json", "parser"}},
	{Name: "kubectl", Description: "Kubernetes CLI", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: kubectlInstall, GoUninstall: kubectlUninstall, Tags: []string{"kubernetes", "k8s"}},
	{Name: "linear", Description: "Linear issue tracker", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, DetectApps: []string{"/Applications/Linear.app"}, GoInstall: linearInstall, GoUninstall: linearUninstall, Tags: []string{"issues", "project"}},
	{Name: "logi-options", Description: "Logitech Options+", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS}, DetectApps: []string{"/Applications/Logi Options+.app", "/Applications/Logi Options.app", "/Applications/logioptionsplus.app"}, GoInstall: logiOptionsInstall, GoUninstall: logiOptionsUninstall, Tags: []string{"logitech", "mouse"}},
	{Name: "node", Description: "Node.js via nodenv", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "$HOME/.nodenv", GoInstall: nodeInstall, GoUninstall: nodeUninstall, Tags: []string{"javascript", "nodejs"}},
	{Name: "ruby", Description: "Ruby via rbenv", Category: CategoryRuntime, SupportedOS: []OS{OSAny}, DetectPath: "$HOME/.rbenv", GoInstall: rubyInstall, GoUninstall: rubyUninstall, Tags: []string{"rbenv", "rails"}},
	{Name: "shellcheck", Description: "Shell script linter", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: SimplePackageInstaller("shellcheck"), GoUninstall: SimplePackageUninstaller("shellcheck"), Tags: []string{"lint", "bash"}},
	{Name: "slack", Description: "Slack messaging", Category: CategoryDesktop, SupportedOS: []OS{OSMacOS, OSLinux}, DetectApps: []string{"/Applications/Slack.app"}, GoInstall: slackInstall, GoUninstall: slackUninstall, Tags: []string{"messaging", "chat"}},
	{Name: "solaar", Description: "Logitech Unifying/Bolt receiver manager", Category: CategorySystem, SupportedOS: []OS{OSLinux}, GoInstall: solaarInstall, GoUninstall: solaarUninstall, Tags: []string{"logitech", "bluetooth"}},
	{Name: "sops", Description: "Mozilla SOPS secrets manager", Category: CategorySecurity, SupportedOS: []OS{OSAny}, GoInstall: sopsInstall, GoUninstall: sopsUninstall, Tags: []string{"secrets", "encrypt"}},
	{Name: "tailscale", Description: "Tailscale VPN", Category: CategorySecurity, SupportedOS: []OS{OSAny}, GoInstall: tailscaleInstall, GoUninstall: tailscaleUninstall, Tags: []string{"vpn", "network"}},
	{Name: "terraform", Description: "Terraform infrastructure tool", Category: CategoryInfra, SupportedOS: []OS{OSAny}, GoInstall: terraformInstall, GoUninstall: terraformUninstall, Tags: []string{"iac", "cloud"}},
	{Name: "tmux", Description: "Terminal multiplexer", Category: CategoryCLI, SupportedOS: []OS{OSAny}, GoInstall: tmuxInstall, GoUninstall: tmuxUninstall, Tags: []string{"terminal", "session"}},
	{Name: "vscode", Description: "Visual Studio Code", Category: CategoryDesktop, SupportedOS: []OS{OSAny}, DetectCmd: "code", DetectApps: []string{"/Applications/Visual Studio Code.app"}, GoInstall: vscodeInstall, GoUninstall: vscodeUninstall, Tags: []string{"editor", "ide"}},
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

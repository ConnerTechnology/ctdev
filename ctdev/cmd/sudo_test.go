package cmd

import (
	"testing"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/checklist"
)

func TestSudoPlan(t *testing.T) {
	tests := []struct {
		name          string
		access        sysutil.SudoAccess
		batch         bool
		wantPrompt    bool
		wantWarnEmpty bool
	}{
		{"already root needs nothing", sysutil.AlreadyRoot, false, false, true},
		{"cached credential needs nothing", sysutil.SudoCached, false, false, true},
		{"no sudo at all warns", sysutil.SudoUnavailable, false, false, false},
		{"no sudo in batch warns", sysutil.SudoUnavailable, true, false, false},
		{"password needed prompts when interactive", sysutil.SudoNeedsPassword, false, true, true},
		{"password needed warns in batch — nothing can type it", sysutil.SudoNeedsPassword, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrompt, gotWarn := sudoPlan(tt.access, tt.batch)
			if gotPrompt != tt.wantPrompt {
				t.Errorf("needPrompt = %v, want %v", gotPrompt, tt.wantPrompt)
			}
			if (gotWarn == "") != tt.wantWarnEmpty {
				t.Errorf("warn = %q, wantEmpty = %v", gotWarn, tt.wantWarnEmpty)
			}
		})
	}
}

// The prompt has to be gated on work that genuinely escalates. Before this, a
// Mac upgrading only Homebrew formulae — which never touch root — was asked for
// a password on every `ctdev update`.
func TestUpdateNeedsRoot(t *testing.T) {
	tests := []struct {
		name  string
		items []checklist.UpdateItem
		want  bool
	}{
		{"nothing selected", nil, false},
		{"apt escalates", []checklist.UpdateItem{{Source: "apt", Name: "curl"}}, true},
		{"brew formulae stay in the brew prefix", []checklist.UpdateItem{{Source: "brew", Name: "jq"}}, false},
		{"brew casks escalate internally", []checklist.UpdateItem{{Source: "brew-cask", Name: "docker"}}, true},
		{"docker stacks can rebuild as root", []checklist.UpdateItem{{Source: "docker", Name: "pihole"}}, true},
		{"cli tools install into /usr/local/bin", []checklist.UpdateItem{{Source: "cli", Name: "helm"}}, true},
		{"go runtime replaces /usr/local/go", []checklist.UpdateItem{{Source: "runtime", Name: "go"}}, true},
		{"bun upgrades in $HOME", []checklist.UpdateItem{{Source: "runtime", Name: "bun"}}, false},
		{"nodenv stays in $HOME", []checklist.UpdateItem{{Source: "runtime", Name: "node (nodenv)"}}, false},
		{"rbenv stays in $HOME", []checklist.UpdateItem{{Source: "runtime", Name: "ruby"}}, false},
		{"flatpak uses polkit, not our sudo", []checklist.UpdateItem{{Source: "flatpak", Name: "org.x.Y"}}, false},
		{"npm globals", []checklist.UpdateItem{{Source: "npm", Name: "typescript"}}, false},
		{"oh-my-zsh git pull", []checklist.UpdateItem{{Source: "git", Name: "oh-my-zsh"}}, false},
		{"ctdev self-update targets ~/.local/bin", []checklist.UpdateItem{{Source: "ctdev", Name: "ctdev"}}, false},
		{
			"one privileged item among many is enough",
			[]checklist.UpdateItem{{Source: "brew", Name: "jq"}, {Source: "brew-cask", Name: "tailscale"}},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := updateNeedsRoot(tt.items); got != tt.want {
				t.Errorf("updateNeedsRoot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldRefreshBrew(t *testing.T) {
	tests := []struct {
		dryRun, noRefresh, want bool
	}{
		{false, false, true},
		{true, false, false},
		{false, true, false},
		{true, true, false},
	}
	for _, tt := range tests {
		if got := shouldRefreshBrew(tt.dryRun, tt.noRefresh); got != tt.want {
			t.Errorf("shouldRefreshBrew(dryRun=%v, noRefresh=%v) = %v, want %v",
				tt.dryRun, tt.noRefresh, got, tt.want)
		}
	}
}

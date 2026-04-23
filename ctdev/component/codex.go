package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func codexInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("codex") {
		fmt.Fprintln(opts.Stdout, "Codex already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Codex...")

	// Codex CLI is an npm package on all platforms
	if !sysutil.CommandExists("node") {
		return fmt.Errorf("node.js is required to install codex; run: ctdev install node")
	}

	return sysutil.Run(ctx, o, "npm", "install", "-g", "@openai/codex")
}

func codexUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Codex...")

	if sysutil.CommandExists("npm") {
		return sysutil.Run(ctx, o, "npm", "uninstall", "-g", "@openai/codex")
	}
	return nil
}

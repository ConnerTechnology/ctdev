package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func codexInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	if !opts.Force && sysutil.CommandExists("codex") {
		fmt.Fprintln(opts.Stdout, "Codex already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Codex...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskInstall(o, "codex")
	}

	// Linux: install via npm
	if !sysutil.CommandExists("node") {
		return fmt.Errorf("node.js is required to install codex; run: ctdev install node")
	}

	return sysutil.Run(o, "npm", "install", "-g", "@openai/codex")
}

func codexUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing Codex...")

	if p.OS == platform.MacOS {
		return sysutil.BrewCaskRemove(o, "codex")
	}

	if sysutil.CommandExists("npm") {
		return sysutil.Run(o, "npm", "uninstall", "-g", "@openai/codex")
	}
	return nil
}

package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// ccusage reports Claude Code token usage and cost from the local session
// logs. Installed from npm on every OS so there is one code path.
func ccusageInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	if !opts.Force && alreadyInstalled("ccusage") {
		fmt.Fprintln(opts.Stdout, "ccusage already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing ccusage...")
	npm, err := npmPath()
	if err != nil {
		return err
	}
	if err := sysutil.Run(ctx, o, npm, "install", "-g", "ccusage"); err != nil {
		return fmt.Errorf("npm install ccusage: %w", err)
	}
	return nil
}

func ccusageUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing ccusage...")

	npm, err := npmPath()
	if err != nil {
		return err
	}
	return sysutil.Run(ctx, o, npm, "uninstall", "-g", "ccusage")
}

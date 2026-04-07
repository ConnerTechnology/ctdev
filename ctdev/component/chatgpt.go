package component

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func chatgptInstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	if !opts.Force && dirExists("/Applications/ChatGPT.app") {
		fmt.Fprintln(opts.Stdout, "ChatGPT already installed")
		return nil
	}
	fmt.Fprintln(opts.Stdout, "Installing ChatGPT...")
	return sysutil.BrewCaskInstall(o, "chatgpt")
}

func chatgptUninstall(ctx context.Context, opts ExecOpts) error {
	if platform.Detect().OS != platform.MacOS {
		return ErrUnsupportedOS
	}
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing ChatGPT...")
	return sysutil.BrewCaskRemove(o, "chatgpt")
}

package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func zshInstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	omzDir := filepath.Join(home, ".oh-my-zsh")

	if !opts.Force && dirExists(omzDir) {
		fmt.Fprintln(opts.Stdout, "Zsh already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Zsh...")

	// Ensure zsh is installed
	if !sysutil.CommandExists("zsh") {
		if err := sysutil.InstallPackage(o, "zsh"); err != nil {
			return fmt.Errorf("install zsh: %w", err)
		}
	}

	// Change default shell to zsh
	if err := sysutil.Run(o, "chsh", "-s", "/usr/bin/zsh"); err != nil {
		// Non-fatal: may need interactive auth
		fmt.Fprintf(opts.Stdout, "warning: could not change shell: %v\n", err)
	}

	// Install Oh My Zsh
	if !dirExists(omzDir) {
		fmt.Fprintln(opts.Stdout, "Installing Oh My Zsh...")
		if err := sysutil.Run(o, "bash", "-c",
			"curl -fsSL https://raw.github.com/ohmyzsh/ohmyzsh/master/tools/install.sh | bash -s -- --unattended"); err != nil {
			return fmt.Errorf("install oh-my-zsh: %w", err)
		}
	}

	// Pure prompt
	pureDir := filepath.Join(home, ".zsh", "pure")
	if err := ensureGitClone(o, "https://github.com/sindresorhus/pure.git", pureDir); err != nil {
		return fmt.Errorf("clone pure prompt: %w", err)
	}

	funcDir := filepath.Join(home, ".zsh", "functions")
	if !o.DryRun {
		os.MkdirAll(funcDir, 0755)
	}
	if err := symlinkOrDryRun(o, filepath.Join(pureDir, "pure.zsh"), filepath.Join(funcDir, "prompt_pure_setup")); err != nil {
		return err
	}
	if err := symlinkOrDryRun(o, filepath.Join(pureDir, "async.zsh"), filepath.Join(funcDir, "async")); err != nil {
		return err
	}

	// Plugins
	customPlugins := filepath.Join(omzDir, "custom", "plugins")
	if err := ensureGitClone(o, "https://github.com/zsh-users/zsh-autosuggestions", filepath.Join(customPlugins, "zsh-autosuggestions")); err != nil {
		return fmt.Errorf("clone zsh-autosuggestions: %w", err)
	}
	if err := ensureGitClone(o, "https://github.com/zsh-users/zsh-completions.git", filepath.Join(customPlugins, "zsh-completions")); err != nil {
		return fmt.Errorf("clone zsh-completions: %w", err)
	}

	// Deploy configs from embedded FS
	deploys := map[string]string{
		"configs/zsh/aliases.zsh":  filepath.Join(omzDir, "custom", "aliases.zsh"),
		"configs/zsh/exports.zsh":  filepath.Join(omzDir, "custom", "exports.zsh"),
		"configs/zsh/path.zsh":     filepath.Join(home, ".zsh", "path.zsh"),
		"configs/zsh/.zshrc":       filepath.Join(home, ".zshrc"),
	}

	for src, dst := range deploys {
		if err := deployOrDryRun(o, src, dst); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not deploy %s: %v\n", filepath.Base(src), err)
		}
	}

	// ctdev completions
	zfuncDir := filepath.Join(home, ".zfunc")
	if !o.DryRun {
		os.MkdirAll(zfuncDir, 0755)
	}
	if err := deployOrDryRun(o, "configs/zsh/completions/_ctdev", filepath.Join(zfuncDir, "_ctdev")); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not deploy completions: %v\n", err)
	}

	// Deploy exports.local.zsh only if dest doesn't exist
	localExports := filepath.Join(omzDir, "custom", "exports.local.zsh")
	if !o.DryRun {
		if _, err := os.Stat(localExports); os.IsNotExist(err) {
			if err := sysutil.DeployFileFromFS(Configs, "configs/zsh/exports.local.zsh", localExports); err == nil {
				fmt.Fprintln(opts.Stdout, "Created exports.local.zsh - customize it!")
			}
		}
	} else {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy exports.local.zsh if not exists\n")
	}

	fmt.Fprintln(opts.Stdout, "Zsh installation complete")
	return nil
}

func zshUninstall(ctx context.Context, opts ExecOpts) error {
	o := sysutil.Opts{Stdout: opts.Stdout, DryRun: opts.DryRun}
	fmt.Fprintln(opts.Stdout, "Removing zsh configuration...")

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	omzCustom := filepath.Join(home, ".oh-my-zsh", "custom")

	toRemove := []string{
		filepath.Join(omzCustom, "aliases.zsh"),
		filepath.Join(omzCustom, "exports.zsh"),
		filepath.Join(omzCustom, "path.zsh"),
		filepath.Join(omzCustom, "exports.local.zsh"),
		filepath.Join(home, ".zshrc"),
		filepath.Join(home, ".zfunc", "_ctdev"),
	}

	toDirRemove := []string{
		filepath.Join(home, ".zsh", "pure"),
		filepath.Join(omzCustom, "plugins", "zsh-autosuggestions"),
		filepath.Join(omzCustom, "plugins", "zsh-completions"),
	}

	if o.DryRun {
		for _, f := range toRemove {
			fmt.Fprintf(o.Stdout, "[dry-run] rm %s\n", f)
		}
		for _, d := range toDirRemove {
			fmt.Fprintf(o.Stdout, "[dry-run] rm -rf %s\n", d)
		}
		return nil
	}

	for _, f := range toRemove {
		os.Remove(f)
	}
	for _, d := range toDirRemove {
		os.RemoveAll(d)
	}

	fmt.Fprintln(opts.Stdout, "Oh My Zsh kept. Run 'uninstall_oh_my_zsh' to remove it.")
	return nil
}

func symlinkOrDryRun(o sysutil.Opts, src, dst string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] symlink %s → %s\n", src, dst)
		return nil
	}
	return sysutil.SafeSymlink(src, dst)
}

func deployOrDryRun(o sysutil.Opts, srcEmbed, dst string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy %s → %s\n", filepath.Base(srcEmbed), dst)
		return nil
	}
	return sysutil.DeployFileFromFS(Configs, srcEmbed, dst)
}

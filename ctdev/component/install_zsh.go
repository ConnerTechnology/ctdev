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

	// Symlink configs from dotfiles
	dotfiles := findDotfilesRoot()
	zshDir := filepath.Join(dotfiles, "components", "zsh")

	symlinks := map[string]string{
		filepath.Join(zshDir, "aliases.zsh"):  filepath.Join(omzDir, "custom", "aliases.zsh"),
		filepath.Join(zshDir, "exports.zsh"):  filepath.Join(omzDir, "custom", "exports.zsh"),
		filepath.Join(zshDir, "path.zsh"):     filepath.Join(home, ".zsh", "path.zsh"),
		filepath.Join(zshDir, ".zshrc"):        filepath.Join(home, ".zshrc"),
	}

	for src, dst := range symlinks {
		if err := symlinkOrDryRun(o, src, dst); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not symlink %s: %v\n", filepath.Base(src), err)
		}
	}

	// ctdev completions
	zfuncDir := filepath.Join(home, ".zfunc")
	if !o.DryRun {
		os.MkdirAll(zfuncDir, 0755)
	}
	completionsSrc := filepath.Join(zshDir, "completions", "_ctdev")
	if _, err := os.Stat(completionsSrc); err == nil {
		if err := symlinkOrDryRun(o, completionsSrc, filepath.Join(zfuncDir, "_ctdev")); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not symlink completions: %v\n", err)
		}
	}

	// Copy exports.local.zsh only if it doesn't exist
	localExports := filepath.Join(omzDir, "custom", "exports.local.zsh")
	if !o.DryRun {
		if _, err := os.Stat(localExports); os.IsNotExist(err) {
			src := filepath.Join(zshDir, "exports.local.zsh")
			if data, err := os.ReadFile(src); err == nil {
				if err := os.WriteFile(localExports, data, 0644); err == nil {
					fmt.Fprintln(opts.Stdout, "Created exports.local.zsh - customize it!")
				}
			}
		}
	} else {
		fmt.Fprintf(o.Stdout, "[dry-run] copy exports.local.zsh if not exists\n")
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

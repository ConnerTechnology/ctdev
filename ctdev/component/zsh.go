package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

func zshInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	omzDir := filepath.Join(home, ".oh-my-zsh")

	// Phase 1: Install zsh, oh-my-zsh, plugins (skip if already present)
	if opts.Force || !dirExists(omzDir) {
		fmt.Fprintln(opts.Stdout, "Installing Zsh...")

		if !sysutil.CommandExists("zsh") {
			if err := sysutil.InstallPackage(ctx, o, "zsh"); err != nil {
				return fmt.Errorf("install zsh: %w", err)
			}
		}

		zshPath := "/usr/bin/zsh"
		if which, err := exec.LookPath("zsh"); err == nil {
			zshPath = which
		}
		if err := sysutil.Run(ctx, o, "chsh", "-s", zshPath); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not change shell: %v\n", err)
		}

		if !dirExists(omzDir) {
			fmt.Fprintln(opts.Stdout, "Installing Oh My Zsh...")
			if err := sysutil.Run(ctx, o, "bash", "-c",
				"curl -fsSL https://raw.github.com/ohmyzsh/ohmyzsh/master/tools/install.sh | bash -s -- --unattended"); err != nil {
				return fmt.Errorf("install oh-my-zsh: %w", err)
			}
		}

		pureDir := filepath.Join(home, ".zsh", "pure")
		if err := ensureGitClone(ctx, o, "https://github.com/sindresorhus/pure.git", pureDir); err != nil {
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

		customPlugins := filepath.Join(omzDir, "custom", "plugins")
		if err := ensureGitClone(ctx, o, "https://github.com/zsh-users/zsh-autosuggestions", filepath.Join(customPlugins, "zsh-autosuggestions")); err != nil {
			return fmt.Errorf("clone zsh-autosuggestions: %w", err)
		}
		if err := ensureGitClone(ctx, o, "https://github.com/zsh-users/zsh-completions.git", filepath.Join(customPlugins, "zsh-completions")); err != nil {
			return fmt.Errorf("clone zsh-completions: %w", err)
		}
	} else {
		fmt.Fprintln(opts.Stdout, "Zsh already installed, updating configs...")
	}

	// Phase 2: Always deploy configs (keeps dotfiles in sync).
	// Order is fixed so output and backup timestamps are reproducible across runs.
	deploys := []struct{ src, dst string }{
		{"configs/zsh/.zshrc", filepath.Join(home, ".zshrc")},
		{"configs/zsh/aliases.zsh", filepath.Join(omzDir, "custom", "aliases.zsh")},
		{"configs/zsh/exports.zsh", filepath.Join(omzDir, "custom", "exports.zsh")},
		{"configs/zsh/path.zsh", filepath.Join(home, ".zsh", "path.zsh")},
	}

	for _, d := range deploys {
		if err := deployOrDryRun(o, d.src, d.dst); err != nil {
			fmt.Fprintf(opts.Stdout, "warning: could not deploy %s: %v\n", filepath.Base(d.src), err)
		}
	}

	// ctdev completions — generate via cobra so they stay in sync automatically
	zfuncDir := filepath.Join(home, ".zfunc")
	if !o.DryRun {
		os.MkdirAll(zfuncDir, 0755)
		compFile := filepath.Join(zfuncDir, "_ctdev")
		if ctdevPath, err := exec.LookPath("ctdev"); err == nil {
			out, err := exec.Command(ctdevPath, "completion", "zsh").Output()
			if err == nil && len(out) > 0 {
				if err := os.WriteFile(compFile, out, 0644); err != nil {
					fmt.Fprintf(opts.Stdout, "warning: could not write completions: %v\n", err)
				}
			} else {
				fmt.Fprintf(opts.Stdout, "warning: could not generate completions: %v\n", err)
			}
		} else {
			fmt.Fprintf(opts.Stdout, "warning: ctdev not in PATH, skipping completions\n")
		}
	} else {
		fmt.Fprintf(o.Stdout, "[dry-run] generate ctdev zsh completions → %s/_ctdev\n", zfuncDir)
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
	o := execOpts(opts)
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
		filepath.Join(home, ".zsh", "path.zsh"),
		filepath.Join(home, ".zsh", "functions", "prompt_pure_setup"),
		filepath.Join(home, ".zsh", "functions", "async"),
	}

	toDirRemove := []string{
		filepath.Join(home, ".zsh", "pure"),
		filepath.Join(home, ".zsh", "functions"),
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

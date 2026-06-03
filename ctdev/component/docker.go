package component

import (
	"context"
	"fmt"
	"io"
	"os/user"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

var dockerPackages = []string{
	"docker-ce", "docker-ce-cli", "containerd.io",
	"docker-buildx-plugin", "docker-compose-plugin",
}

func dockerInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if !opts.Force && sysutil.CommandExists("docker") {
		fmt.Fprintln(opts.Stdout, "Docker already installed")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "Installing Docker...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskInstall(ctx, o, "docker")

	case "apt":
		// Determine distro base for Docker repo URL
		distroBase := "ubuntu"
		if p.Distro == "debian" {
			distroBase = "debian"
		}

		keyring := "/etc/apt/keyrings/docker.asc"
		keyURL := fmt.Sprintf("https://download.docker.com/linux/%s/gpg", distroBase)

		if err := sysutil.SudoRun(ctx, o, "install", "-m", "0755", "-d", "/etc/apt/keyrings"); err != nil {
			return fmt.Errorf("create keyrings dir: %w", err)
		}
		if err := sysutil.AddAPTKeyring(ctx, o, keyURL, keyring); err != nil {
			return fmt.Errorf("add docker GPG key: %w", err)
		}
		if err := sysutil.SudoRun(ctx, o, "chmod", "a+r", keyring); err != nil {
			return fmt.Errorf("chmod keyring: %w", err)
		}

		codename := p.Codename
		if codename == "" {
			codename = "noble"
		}
		repoLine := fmt.Sprintf("deb [arch=%s signed-by=%s] https://download.docker.com/linux/%s %s stable",
			p.Arch, keyring, distroBase, codename)
		if err := sysutil.AddAPTSource(ctx, o, repoLine, "docker.list"); err != nil {
			return fmt.Errorf("add docker repo: %w", err)
		}

		if err := sysutil.APTUpdate(ctx, o); err != nil {
			return err
		}
		if err := sysutil.InstallPackage(ctx, o, dockerPackages...); err != nil {
			return err
		}

		_ = sysutil.ServiceEnable(ctx, o, "docker")
		_ = sysutil.ServiceStart(ctx, o, "docker")
		addUserToDockerGroup(ctx, o, opts.Stdout)
		return nil

	default:
		return unsupportedPMError("docker", p.PackageManager)
	}
}

func addUserToDockerGroup(ctx context.Context, o sysutil.Opts, w io.Writer) {
	u, err := user.Current()
	if err != nil || u.Uid == "0" {
		return
	}
	_ = sysutil.SudoRun(ctx, o, "usermod", "-aG", "docker", u.Username)
	fmt.Fprintf(w, "Added %s to docker group. Log out and back in for this to take effect.\n", u.Username)
}

func dockerUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Docker...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(ctx, o, "docker")
	case "apt":
		return sysutil.RemovePackage(ctx, o, dockerPackages...)
	default:
		return ErrUnsupportedOS
	}
}

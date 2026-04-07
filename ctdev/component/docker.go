package component

import (
	"context"
	"fmt"
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
		return sysutil.BrewCaskInstall(o, "docker")

	case "apt":
		// Determine distro base for Docker repo URL
		distroBase := "ubuntu"
		if p.Distro == "debian" {
			distroBase = "debian"
		}

		keyring := "/etc/apt/keyrings/docker.asc"
		keyURL := fmt.Sprintf("https://download.docker.com/linux/%s/gpg", distroBase)

		if err := sysutil.SudoRun(o, "install", "-m", "0755", "-d", "/etc/apt/keyrings"); err != nil {
			return fmt.Errorf("create keyrings dir: %w", err)
		}
		if err := sysutil.AddAPTKeyring(o, keyURL, keyring); err != nil {
			return fmt.Errorf("add docker GPG key: %w", err)
		}
		if err := sysutil.SudoRun(o, "chmod", "a+r", keyring); err != nil {
			return fmt.Errorf("chmod keyring: %w", err)
		}

		codename := p.Codename
		if codename == "" {
			codename = "noble"
		}
		repoLine := fmt.Sprintf("deb [arch=%s signed-by=%s] https://download.docker.com/linux/%s %s stable",
			p.Arch, keyring, distroBase, codename)
		if err := sysutil.AddAPTSource(o, repoLine, "docker.list"); err != nil {
			return fmt.Errorf("add docker repo: %w", err)
		}

		if err := sysutil.APTUpdate(o); err != nil {
			return err
		}
		if err := sysutil.InstallPackage(o, dockerPackages...); err != nil {
			return err
		}

		_ = sysutil.ServiceEnable(o, "docker")
		_ = sysutil.ServiceStart(o, "docker")
		addUserToDockerGroup(o, opts.Stdout)
		return nil

	case "dnf":
		if err := sysutil.SudoRun(o, "dnf", "config-manager", "--add-repo",
			"https://download.docker.com/linux/fedora/docker-ce.repo"); err != nil {
			return fmt.Errorf("add docker repo: %w", err)
		}
		args := append([]string{"install", "-y"}, dockerPackages...)
		if err := sysutil.SudoRun(o, "dnf", args...); err != nil {
			return err
		}

		_ = sysutil.ServiceEnable(o, "docker")
		_ = sysutil.ServiceStart(o, "docker")
		addUserToDockerGroup(o, opts.Stdout)
		return nil

	case "pacman":
		if err := sysutil.SudoRun(o, "pacman", "-S", "--noconfirm", "docker", "docker-compose"); err != nil {
			return err
		}

		_ = sysutil.ServiceEnable(o, "docker")
		_ = sysutil.ServiceStart(o, "docker")
		addUserToDockerGroup(o, opts.Stdout)
		return nil

	default:
		return fmt.Errorf("docker install not supported for package manager: %s", p.PackageManager)
	}
}

func addUserToDockerGroup(o sysutil.Opts, w interface{ Write([]byte) (int, error) }) {
	u, err := user.Current()
	if err != nil || u.Uid == "0" {
		return
	}
	_ = sysutil.SudoRun(o, "usermod", "-aG", "docker", u.Username)
	fmt.Fprintf(w, "Added %s to docker group. Log out and back in for this to take effect.\n", u.Username)
}

func dockerUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Docker...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(o, "docker")
	case "apt":
		return sysutil.RemovePackage(o, dockerPackages...)
	case "dnf":
		return sysutil.RemovePackage(o, dockerPackages...)
	case "pacman":
		return sysutil.RemovePackage(o, "docker", "docker-compose")
	default:
		return ErrUnsupportedOS
	}
}

package component

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"

	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

var dockerPackages = []string{
	"docker-ce", "docker-ce-cli", "containerd.io",
	"docker-buildx-plugin", "docker-compose-plugin",
}

const (
	dockerKeyring = "/etc/apt/keyrings/docker.asc"

	// dockerKeyFingerprint is "Docker Release (CE deb) <docker@docker.com>",
	// the key behind download.docker.com/linux/{debian,ubuntu}/gpg. Docker's
	// install pages printed it until 2021 and no longer do, so the pin is
	// checked against the key Docker serves: testdata/docker.asc is a copy,
	// and TestDockerKeyFingerprintMatchesDocker fails if the two drift.
	dockerKeyFingerprint = "9DC858229FC7DD38854AE2D88D81803C0EBFCD88"

	dockerDaemonConfig = "/etc/docker/daemon.json"

	// json-file is Docker's default driver and keeps every log line forever;
	// on a 10 GB cloud node that fills the disk. 3 x 10 MB per container caps
	// it. It only applies to containers created after the daemon reads it.
	dockerDaemonJSON = `{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "10m",
    "max-file": "3"
  }
}
`
)

// dockerRunning reports whether the docker service is up.
func dockerRunning(ctx context.Context) bool {
	return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "docker.service").Run() == nil
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
		// Verify against the app bundle rather than the CLI: Docker Desktop only
		// links `docker` onto PATH after its first launch.
		return sysutil.BrewCaskInstallVerified(ctx, o, "docker",
			func() bool { return alreadyInstalled("docker") })

	case "apt":
		// Determine distro base for Docker repo URL
		distroBase := "ubuntu"
		if p.Distro == "debian" {
			distroBase = "debian"
		}

		keyURL := fmt.Sprintf("https://download.docker.com/linux/%s/gpg", distroBase)

		if err := sysutil.SudoRun(ctx, o, "install", "-m", "0755", "-d", "/etc/apt/keyrings"); err != nil {
			return fmt.Errorf("create keyrings dir: %w", err)
		}
		if err := dockerAddKey(ctx, o, keyURL, dockerKeyring); err != nil {
			return err
		}

		codename := p.Codename
		if codename == "" {
			codename = "noble"
		}
		repoLine := fmt.Sprintf("deb [arch=%s signed-by=%s] https://download.docker.com/linux/%s %s stable",
			p.Arch, dockerKeyring, distroBase, codename)
		if err := sysutil.AddAPTSource(ctx, o, repoLine, "docker.list"); err != nil {
			return fmt.Errorf("add docker repo: %w", err)
		}

		if err := sysutil.APTUpdate(ctx, o); err != nil {
			return err
		}

		// Written before the packages so the daemon their postinst starts
		// already has it. A daemon that was running before (a --force
		// reinstall) only reads it on restart.
		wasRunning := dockerRunning(ctx)
		wroteConfig, err := dockerWriteDaemonConfig(ctx, o, dockerDaemonConfig)
		if err != nil {
			return err
		}
		if err := sysutil.InstallPackage(ctx, o, dockerPackages...); err != nil {
			return err
		}

		// A docker that installed but won't start is a failed install — every
		// compose-based component (pihole, caddy, beszel, portainer) would
		// otherwise fail later with a much more confusing error.
		if err := sysutil.ServiceEnable(ctx, o, "docker"); err != nil {
			return fmt.Errorf("enable docker service: %w", err)
		}
		if err := dockerStartService(ctx, o, wasRunning && wroteConfig); err != nil {
			return err
		}
		addUserToDockerGroup(ctx, o, opts.Stdout)
		return nil

	default:
		return unsupportedPMError("docker", p.PackageManager)
	}
}

// dockerAddKey downloads Docker's apt key and installs it only if it is the key
// Docker signs its packages with. What gets installed is gpg's re-export of
// that one public key, never the downloaded bytes, so nothing else a tampered
// download carried can reach the keyring. It goes in ASCII-armored, as Docker's
// own install steps put it: apt reads an armored .asc named by signed-by.
func dockerAddKey(ctx context.Context, o sysutil.Opts, keyURL, keyring string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] download %s, check fingerprint %s → %s\n", keyURL, dockerKeyFingerprint, keyring)
		return nil
	}
	if !sysutil.CommandExists("gpg") {
		// A fresh image ships with empty package lists, where installing
		// gnupg fails with "no installation candidate".
		if err := sysutil.APTUpdate(ctx, o); err != nil {
			return err
		}
		if err := sysutil.InstallPackage(ctx, o, "gnupg"); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp("", "docker-key-*.asc")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	if err := sysutil.DownloadFile(ctx, keyURL, tmp.Name()); err != nil {
		return fmt.Errorf("download docker apt key: %w", err)
	}
	key, err := verifiedPublicKey(ctx, tmp.Name(), dockerKeyFingerprint)
	if err != nil {
		return fmt.Errorf("docker apt key from %s is not Docker's signing key, not installing: %w", keyURL, err)
	}
	return sysutil.SudoWriteFileMode(ctx, o, string(key), keyring, "0644")
}

// dockerWriteDaemonConfig writes the log-rotation daemon.json when there is
// none, and reports whether it did. An existing file is the owner's: it is
// never overwritten, even when it lacks log rotation.
func dockerWriteDaemonConfig(ctx context.Context, o sysutil.Opts, path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("check %s: %w", path, err)
	}
	if err := sysutil.SudoRun(ctx, o, "install", "-m", "0755", "-d", filepath.Dir(path)); err != nil {
		return false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := sysutil.SudoWriteFileMode(ctx, o, dockerDaemonJSON, path, "0644"); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// dockerStartService starts docker, or restarts it when a daemon already
// running has to pick up a daemon.json written under it. Docker can't reload
// log settings live.
func dockerStartService(ctx context.Context, o sysutil.Opts, restart bool) error {
	if restart {
		if err := sysutil.SudoRun(ctx, o, "systemctl", "restart", "docker.service"); err != nil {
			return fmt.Errorf("restart docker service: %w", err)
		}
		return nil
	}
	if err := sysutil.ServiceStart(ctx, o, "docker"); err != nil {
		return fmt.Errorf("start docker service: %w", err)
	}
	return nil
}

func addUserToDockerGroup(ctx context.Context, o sysutil.Opts, w io.Writer) {
	u, err := user.Current()
	if err != nil || u.Uid == "0" {
		return
	}
	_ = sysutil.SudoRun(ctx, o, "usermod", "-aG", "docker", u.Username)
	fmt.Fprintf(w, "Added %s to docker group. Log out and back in for this to take effect.\n", u.Username)
}

// removeUserFromDockerGroup reverses addUserToDockerGroup. Best-effort: gpasswd
// exits non-zero if the user isn't a member, which we ignore.
func removeUserFromDockerGroup(ctx context.Context, o sysutil.Opts) {
	u, err := user.Current()
	if err != nil || u.Uid == "0" {
		return
	}
	_ = sysutil.SudoRun(ctx, o, "gpasswd", "-d", u.Username, "docker")
}

func dockerUninstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing Docker...")

	switch p.PackageManager {
	case "brew":
		return sysutil.BrewCaskRemove(ctx, o, "docker")
	case "apt":
		if err := sysutil.RemovePackage(ctx, o, dockerPackages...); err != nil {
			return err
		}
		removeUserFromDockerGroup(ctx, o)
		return sysutil.RemoveAPTRepo(ctx, o, "docker.list", dockerKeyring)
	default:
		return ErrUnsupportedOS
	}
}

package component

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// composeStack captures what the docker-compose components (pihole, caddy,
// beszel, portainer) share: a stack directory under $HOME named after the
// component, files deployed verbatim from the embedded configs, the same
// preflight checks, and a data-preserving "down" on uninstall. Stack-specific
// behavior (extra data dirs, .env gating, multi-phase up) stays in each
// installer.
type composeStack struct {
	Name  string      // component name; the stack lives in ~/<Name>/
	Files [][2]string // {path under configs/<Name>/, path under the stack dir}
}

func (s composeStack) dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, s.Name)
}

func (s composeStack) composePath() string {
	return filepath.Join(s.dir(), "docker-compose.yml")
}

// preflight runs the checks every compose-stack installer starts with —
// apt-only, docker present, dry-run preview. done=true means this was a dry
// run (the preview has been printed) and the installer should return nil.
func (s composeStack) preflight(o sysutil.Opts) (done bool, err error) {
	if pm := platform.Detect().PackageManager; pm != "apt" {
		return false, unsupportedPMError(s.Name, pm)
	}
	if !sysutil.CommandExists("docker") {
		return false, fmt.Errorf("docker is required — install the 'docker' component first")
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] deploy %s stack → %s and docker compose up\n", s.Name, s.dir())
		return true, nil
	}
	return false, nil
}

// deploy writes the stack's files from the embedded configs into the stack
// dir (parent directories are created as needed).
func (s composeStack) deploy() error {
	for _, f := range s.Files {
		src := "configs/" + s.Name + "/" + f[0]
		if err := sysutil.DeployFileFromFS(Configs, src, filepath.Join(s.dir(), f[1])); err != nil {
			return fmt.Errorf("deploy %s: %w", f[0], err)
		}
	}
	return nil
}

// up brings the stack up detached.
func (s composeStack) up(ctx context.Context, o sysutil.Opts) error {
	return sysutil.Run(ctx, o, "docker", "compose", "-f", s.composePath(), "up", "-d")
}

// down stops the stack's containers when the compose file exists, then prints
// keptMsg — uninstallers stop the stack but never delete its data dirs. sudo
// matches how the stack was brought up (caddy runs its compose through sudo).
func (s composeStack) down(ctx context.Context, opts ExecOpts, keptMsg string, sudo bool) {
	o := execOpts(opts)
	if _, err := os.Stat(s.composePath()); err == nil {
		// Best-effort: a stack that's already gone shouldn't fail the uninstall.
		if sudo {
			_ = sysutil.SudoRun(ctx, o, "docker", "compose", "-f", s.composePath(), "down")
		} else {
			_ = sysutil.Run(ctx, o, "docker", "compose", "-f", s.composePath(), "down")
		}
	}
	fmt.Fprintln(opts.Stdout, keptMsg)
}

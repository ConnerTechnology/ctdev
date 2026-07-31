package sysutil

import (
	"context"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// LoginShell returns the current user's login shell from the passwd database,
// so it reflects a `chsh` change immediately (unlike $SHELL, which only updates
// on the next login). Falls back to $SHELL when passwd can't be read — the
// normal case on macOS, where accounts live in Directory Services rather than
// /etc/passwd.
func LoginShell(ctx context.Context) string {
	if u, err := user.Current(); err == nil {
		if out, err := exec.CommandContext(ctx, "getent", "passwd", u.Username).Output(); err == nil {
			fields := strings.Split(strings.TrimSpace(string(out)), ":")
			if len(fields) >= 7 && fields[6] != "" {
				return fields[6]
			}
		}
	}
	return os.Getenv("SHELL")
}

// SetLoginShell points the current user's login shell at shell.
//
// chsh has to run as root. Unprivileged, it authenticates the caller through
// PAM, and the user a devcontainer runs as has no password to authenticate
// with — so the change fails, the terminal keeps opening the old shell, and
// the deployed zsh config is never seen. Root skips PAM entirely.
func SetLoginShell(ctx context.Context, o Opts, shell string) error {
	u, err := user.Current()
	if err != nil {
		return err
	}
	// chsh rejects a shell that isn't listed in /etc/shells, which is where a
	// zsh from Homebrew or a source build lands.
	if err := registerShell(ctx, o, shell); err != nil {
		return err
	}
	return SudoRun(ctx, o, "chsh", "-s", shell, u.Username)
}

func registerShell(ctx context.Context, o Opts, shell string) error {
	const shellsFile = "/etc/shells"
	data, err := os.ReadFile(shellsFile)
	if err != nil {
		// No /etc/shells (macOS keeps one, but a stripped image may not) —
		// leave it to chsh to accept or reject the shell.
		return nil
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == shell {
			return nil
		}
	}
	return SudoRun(ctx, o, "bash", "-c", "echo "+shell+" >> "+shellsFile)
}

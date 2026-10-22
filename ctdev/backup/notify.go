package backup

import (
	"context"
	"os"
	"os/exec"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// Notifier sends a desktop notification.
type Notifier interface {
	Notify(ctx context.Context, title, body string) error
}

// desktopNotifier notifies with notify-send on Linux and osascript on macOS,
// and does nothing where there's no desktop session or no tool to send with.
type desktopNotifier struct {
	goos     string
	opts     sysutil.Opts
	env      func(string) string
	lookPath func(string) (string, error)
}

func (m *Machine) notifier() Notifier {
	if m.Notifier != nil {
		return m.Notifier
	}
	return desktopNotifier{goos: m.goos(), opts: m.Opts, env: os.Getenv, lookPath: exec.LookPath}
}

func (n desktopNotifier) Notify(ctx context.Context, title, body string) error {
	switch n.goos {
	case "linux":
		// notify-send talks to the session's D-Bus. A user timer inherits it
		// from the user's systemd; a machine with no one logged in to a
		// desktop has none, and gets no notification.
		if n.env("DBUS_SESSION_BUS_ADDRESS") == "" {
			return nil
		}
		bin, err := n.lookPath("notify-send")
		if err != nil {
			return nil
		}
		return sysutil.Run(ctx, n.opts, bin, "--app-name", "ctdev", title, body)
	case "darwin":
		// A launchd agent in ~/Library/LaunchAgents runs in the user's GUI
		// session, so there is a desktop to notify. The text goes in as
		// arguments, never into the script, so it needs no quoting.
		bin, err := n.lookPath("osascript")
		if err != nil {
			return nil
		}
		return sysutil.Run(ctx, n.opts, bin,
			"-e", "on run argv",
			"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
			"-e", "end run", title, body)
	}
	return nil
}

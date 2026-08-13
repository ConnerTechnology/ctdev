package cmd

import (
	"os"
)

// askpassScript is what sudo runs when it needs a password and nothing can type
// one. It supplies none and explains why: sudo shows the helper's stderr, and
// under the progress TUI that stderr is the pipe ctdev captures and replays in
// the failure tail, so this is the message the user actually ends up reading.
const askpassScript = `#!/bin/sh
echo 'ctdev: a sudo password is needed here, but the progress display owns the terminal.' >&2
echo "  Run 'sudo -v' first and re-run, or use --batch to skip the interactive steps." >&2
exit 1
`

// setupAskpass points SUDO_ASKPASS at a small helper so that a tool which
// escalates on its own behalf fails loudly instead of hanging. Homebrew opts
// into this: its sudo_prefix adds `-A` whenever SUDO_ASKPASS is set, and sudo
// with -A errors out when the helper supplies nothing — instead of prompting on
// /dev/tty, where the prompt is invisible behind the TUI and the keystrokes are
// eaten by it.
//
// This is safe to set process-wide. sudo consults an askpass helper only when a
// password is actually required, so a credential cached by ensureSudo is
// unaffected, and ctdev's own `sudo -v` passes neither -A nor -n. An explicit
// user setting wins.
//
// The helper is a generated script rather than ctdev itself: the env marker that
// would let the binary recognize itself is inherited by *every* child, and ctdev
// does re-invoke itself for legitimate reasons (the zsh component shells out to
// `ctdev completion zsh`), which would then get the refusal instead of doing
// their job.
//
// Returns a cleanup function that removes the script; it is a no-op when no
// script was created.
func setupAskpass() func() {
	noop := func() {}
	if _, ok := os.LookupEnv("SUDO_ASKPASS"); ok {
		return noop
	}
	f, err := os.CreateTemp("", "ctdev-askpass-*.sh")
	if err != nil {
		return noop
	}
	path := f.Name()
	if _, err := f.WriteString(askpassScript); err != nil {
		f.Close()
		os.Remove(path)
		return noop
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return noop
	}
	if err := os.Chmod(path, 0o700); err != nil {
		os.Remove(path)
		return noop
	}
	os.Setenv("SUDO_ASKPASS", path)
	return func() { os.Remove(path) }
}

// setupHomebrewEnv keeps Homebrew from deciding, mid-step, to go download its
// API catalog — that stalls the scan spinner and can fire again inside an
// upgrade. Staleness is handled instead by refreshBrew, which runs an explicit
// `brew update` before scanning, the same shape as refreshAptIndex.
//
// Setting these on ctdev's own environment covers every child: sysutil.Run never
// assigns cmd.Env, and the scanners shell out through raw exec.CommandContext
// and never see sysutil.Opts at all.
func setupHomebrewEnv() {
	for _, key := range []string{"HOMEBREW_NO_AUTO_UPDATE", "HOMEBREW_NO_ENV_HINTS"} {
		if _, ok := os.LookupEnv(key); !ok {
			os.Setenv(key, "1")
		}
	}
}

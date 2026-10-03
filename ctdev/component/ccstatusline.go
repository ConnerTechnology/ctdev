package component

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
)

// ccstatusline renders the Claude Code status line. Installed from npm on
// every OS so there is one code path; the layout ships with it.
func ccstatuslineInstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	// Phase 1: install the binary (skip if present unless --force)
	if opts.Force || !alreadyInstalled("ccstatusline") {
		fmt.Fprintln(opts.Stdout, "Installing ccstatusline...")
		npm, err := npmPath()
		if err != nil {
			return err
		}
		if err := sysutil.Run(ctx, o, npm, "install", "-g", "ccstatusline"); err != nil {
			return fmt.Errorf("npm install ccstatusline: %w", err)
		}
	} else {
		fmt.Fprintln(opts.Stdout, "ccstatusline already installed")
	}

	// Phase 2: always deploy the layout (keeps dotfiles in sync)
	dst, err := ccstatuslineConfigPath()
	if err != nil {
		return err
	}
	if err := deployOrDryRun(o, "configs/ccstatusline/settings.json", dst); err != nil {
		return fmt.Errorf("deploy ccstatusline config: %w", err)
	}

	// Turn it on in Claude Code. A settings file we can't parse is left alone.
	if err := enableClaudeStatusLine(o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not enable the Claude Code status line: %v\n", err)
	}
	return nil
}

func ccstatuslineUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)
	fmt.Fprintln(opts.Stdout, "Removing ccstatusline...")

	npm, err := npmPath()
	if err != nil {
		return err
	}
	if err := sysutil.Run(ctx, o, npm, "uninstall", "-g", "ccstatusline"); err != nil {
		return fmt.Errorf("npm uninstall ccstatusline: %w", err)
	}

	if err := disableClaudeStatusLine(o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not remove the Claude Code status line: %v\n", err)
	}

	// Remove the layout only once the package is gone
	dst, err := ccstatuslineConfigPath()
	if err != nil {
		return err
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] rm %s\n", dst)
		return nil
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove ccstatusline config: %w", err)
	}
	return nil
}

func ccstatuslineConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "ccstatusline", "settings.json"), nil
}

// ccstatuslineStatusLine is the Claude Code statusLine entry ccstatusline owns.
var ccstatuslineStatusLine = map[string]any{
	"type":            "command",
	"command":         "ccstatusline",
	"padding":         0,
	"refreshInterval": 10,
}

// enableClaudeStatusLine points Claude Code's statusLine at ccstatusline.
func enableClaudeStatusLine(o sysutil.Opts) error {
	want, err := json.Marshal(ccstatuslineStatusLine)
	if err != nil {
		return err
	}
	return editClaudeSettings(o, "set statusLine", true, func(settings map[string]json.RawMessage) bool {
		if sameJSON(settings["statusLine"], want) {
			return false
		}
		settings["statusLine"] = want
		return true
	})
}

// disableClaudeStatusLine removes Claude Code's statusLine key when it runs
// ccstatusline. A status line pointed at another tool is left alone.
func disableClaudeStatusLine(o sysutil.Opts) error {
	return editClaudeSettings(o, "remove statusLine", false, func(settings map[string]json.RawMessage) bool {
		var sl struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(settings["statusLine"], &sl) != nil || sl.Command != "ccstatusline" {
			return false
		}
		delete(settings, "statusLine")
		return true
	})
}

// editClaudeSettings applies edit to ~/.claude/settings.json and writes the
// result only when edit reports a change. Other values are kept byte-exact,
// a symlinked file is written through to its target, and the file's mode is
// kept. A file that isn't a JSON object is left alone.
func editClaudeSettings(o sysutil.Opts, action string, create bool, edit func(map[string]json.RawMessage) bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude", "settings.json")

	mode := os.FileMode(0o644)
	settings := map[string]json.RawMessage{}
	switch info, err := os.Stat(path); {
	case errors.Is(err, os.ErrNotExist):
		if !create {
			return nil
		}
	case err != nil:
		return err
	default:
		if path, err = filepath.EvalSymlinks(path); err != nil {
			return err
		}
		mode = info.Mode().Perm()
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if len(bytes.TrimSpace(data)) > 0 {
			if !json.Valid(data) {
				return fmt.Errorf("%s is not valid JSON, left unchanged", path)
			}
			if err := json.Unmarshal(data, &settings); err != nil || settings == nil {
				return fmt.Errorf("%s is not a JSON object, left unchanged", path)
			}
		}
	}

	if !edit(settings) {
		return nil
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] %s in %s\n", action, path)
		return nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return err
	}
	if err := sysutil.DeployFile(buf.Bytes(), path); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

// sameJSON reports whether two JSON values are equal, ignoring layout and key order.
func sameJSON(a, b []byte) bool {
	if a == nil || b == nil {
		return false
	}
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}

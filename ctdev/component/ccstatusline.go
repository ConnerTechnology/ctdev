package component

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	if err := updateClaudeStatusLine(o, true); err != nil {
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

	if err := updateClaudeStatusLine(o, false); err != nil {
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

// updateClaudeStatusLine sets (enable) or removes the statusLine key in
// ~/.claude/settings.json, leaving every other key as it is.
func updateClaudeStatusLine(o sysutil.Opts, enable bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, ".claude", "settings.json")

	settings := map[string]any{}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if !enable {
			return nil
		}
	case err != nil:
		return err
	case len(bytes.TrimSpace(data)) > 0:
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("%s is not valid JSON, left unchanged: %w", path, err)
		}
	}

	if enable {
		settings["statusLine"] = ccstatuslineStatusLine
	} else {
		if _, ok := settings["statusLine"]; !ok {
			return nil
		}
		delete(settings, "statusLine")
	}

	if o.DryRun {
		action := "set"
		if !enable {
			action = "remove"
		}
		fmt.Fprintf(o.Stdout, "[dry-run] %s statusLine in %s\n", action, path)
		return nil
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(settings); err != nil {
		return err
	}
	return sysutil.DeployFile(buf.Bytes(), path)
}

package cmd

import (
	"context"
	"fmt"

	"github.com/ConnerTechnology/ctdev/ctdev/component"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var configureClaudeCodeCmd = &cobra.Command{
	Use:   "claude-code",
	Short: "Review drift in the Claude Code settings ctdev owns",
	Long: "Compare ~/.claude/settings.json and ~/.claude/CLAUDE.md with ctdev's baseline. " +
		"For each file that differs, show the diff and ask before backing it up and " +
		"replacing it (--force replaces without asking, --dry-run only shows the diff). " +
		"'ctdev install claude-code' runs this after installing.",
	RunE: runConfigureClaudeCode,
}

func init() {
	configureCmd.AddCommand(configureClaudeCodeCmd)
}

func runConfigureClaudeCode(cmd *cobra.Command, args []string) error {
	return cancelToClean(reviewClaudeCode(cmdContext(cmd), claudeCodeReview{
		dryRun:      flagDryRun,
		force:       flagForce,
		interactive: !isBatchMode(),
	}))
}

type claudeCodeReview struct {
	dryRun      bool // show what would change, write nothing
	force       bool // replace drifted files without asking
	interactive bool // a terminal is there to show the diff and ask
}

// reviewClaudeCode brings ~/.claude in line with ctdev's baseline. It runs
// after the install's progress screen (which can't show a diff or ask) and as
// `ctdev configure claude-code`. Every question is asked before anything is
// written, so cancelling a prompt really does change nothing.
func reviewClaudeCode(ctx context.Context, r claudeCodeReview) error {
	drifts, err := component.ClaudeCodeDrift()
	if err != nil {
		return err
	}

	var replace []component.ClaudeCodeFileDrift
	for _, d := range drifts {
		if d.Missing {
			if r.dryRun {
				fmt.Printf("[dry-run] write %s\n", d.Path)
			} else {
				replace = append(replace, d)
			}
			continue
		}

		fmt.Println()
		fmt.Println(styles.Warning.Render(fmt.Sprintf("%s has drifted from ctdev's copy.", d.Path)))
		if !r.dryRun && !r.force && !r.interactive {
			fmt.Println("  Left unchanged. Run ctdev install claude-code in a terminal to review it.")
			continue
		}
		if d.LinkTarget != "" {
			fmt.Printf("  It is a link to %s; replacing it leaves that file untouched.\n", d.LinkTarget)
		}
		fmt.Println(styles.Dimmed.Render("  - yours   + ctdev's copy"))
		fmt.Print(d.Diff)

		switch {
		case r.dryRun:
			fmt.Println("  [dry-run] left unchanged")
		case r.force:
			replace = append(replace, d)
		default:
			yes, err := promptYesNoCtx(ctx, "Back it up and replace it with ctdev's copy?", false)
			if err != nil {
				return err
			}
			if yes {
				replace = append(replace, d)
			} else {
				fmt.Printf("  Left %s unchanged.\n", d.Path)
			}
		}
	}

	for _, d := range replace {
		backup, err := component.ReplaceClaudeCodeFile(d)
		if err != nil {
			return fmt.Errorf("replace %s: %w", d.Path, err)
		}
		if d.Missing {
			fmt.Printf("Wrote %s.\n", d.Path)
		} else {
			fmt.Printf("Replaced %s%s.\n", d.Path, d.ReplacedNote(backup))
		}
	}

	local, exists, err := component.ClaudeCodeSettingsLocalPath()
	if err != nil {
		return err
	}
	if exists {
		if r.dryRun {
			fmt.Printf("[dry-run] back up and remove %s (Claude Code never reads it)\n", local)
		} else {
			backup, err := component.BackupFile(local)
			if err != nil {
				return err
			}
			fmt.Printf("Removed %s (Claude Code never reads it); backup at %s.\n", local, backup)
		}
	}

	if len(drifts) == 0 && !exists {
		fmt.Println("Claude Code settings match ctdev's copy.")
	}
	return nil
}

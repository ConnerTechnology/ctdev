package cmd

import (
	"context"
	"fmt"
	"os"
	"slices"

	"github.com/ConnerTechnology/ctdev/ctdev/component"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/term"
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
	return cancelToClean(reviewClaudeCode(cmdContext(cmd), claudeCodeReviewFromFlags()))
}

// canPromptClaudeCode reports whether someone is there to answer: a terminal on
// both ends and no --batch. Unlike isBatchMode it ignores ACCESSIBLE, since the
// review is plain lines a screen reader handles fine.
func canPromptClaudeCode() bool {
	return !flagBatch && term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}

// reviewClaudeCodeAfterInstall runs the review when claude-code was part of an
// install and is now there (or this is a dry run), even if something else in
// the same run failed.
func reviewClaudeCodeAfterInstall(ctx context.Context, resolved []string) error {
	if !slices.Contains(resolved, "claude-code") {
		return nil
	}
	if c := component.FindByName("claude-code"); !flagDryRun && (c == nil || !c.IsInstalled()) {
		return nil
	}
	return reviewClaudeCode(ctx, claudeCodeReviewFromFlags())
}

type claudeCodeReview struct {
	dryRun      bool // show what would change, write nothing
	force       bool // replace drifted files without asking
	interactive bool // a terminal is there to show the diff and ask
}

// claudeCodeReviewFromFlags is the review the command-line flags ask for.
func claudeCodeReviewFromFlags() claudeCodeReview {
	return claudeCodeReview{
		dryRun:      flagDryRun,
		force:       flagForce,
		interactive: canPromptClaudeCode(),
	}
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

		if !r.dryRun && !r.force && !r.interactive {
			fmt.Printf("%s has drifted from ctdev's copy; run ctdev install claude-code to review\n", d.Path)
			continue
		}
		fmt.Println()
		fmt.Println(styles.Warning.Render(fmt.Sprintf("%s has drifted from ctdev's copy.", d.Path)))
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

	local, exists, err := component.StrayClaudeSettingsLocal()
	if err != nil {
		return err
	}
	if exists {
		if r.dryRun {
			fmt.Printf("[dry-run] back up and remove %s (Claude Code never reads it)\n", local)
		} else {
			backup, err := sysutil.BackupFile(local)
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

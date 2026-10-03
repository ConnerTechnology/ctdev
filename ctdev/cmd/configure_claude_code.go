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
		"replacing it. 'ctdev install claude-code' runs this after installing.",
	RunE: runConfigureClaudeCode,
}

func init() {
	configureCmd.AddCommand(configureClaudeCodeCmd)
}

func runConfigureClaudeCode(cmd *cobra.Command, args []string) error {
	return cancelToClean(configureClaudeCode(cmdContext(cmd)))
}

// configureClaudeCode shows each drifted owned file's diff and asks before
// replacing it. A missing file is written without asking: there's nothing to lose.
func configureClaudeCode(ctx context.Context) error {
	drifts, err := component.ClaudeCodeDrift()
	if err != nil {
		return err
	}
	changed := false
	for _, d := range drifts {
		if !d.Missing {
			changed = true
			fmt.Println()
			fmt.Println(styles.Warning.Render(fmt.Sprintf("%s has drifted from ctdev's copy.", d.Path)))
			if d.LinkTarget != "" {
				fmt.Printf("  It is a link to %s; replacing it leaves that file untouched.\n", d.LinkTarget)
			}
			fmt.Println(styles.Dimmed.Render("  - yours   + ctdev's copy"))
			fmt.Print(d.Diff)
			yes, err := promptYesNoCtx(ctx, "Back it up and replace it with ctdev's copy?", false)
			if err != nil {
				return err
			}
			if !yes {
				fmt.Printf("  Left %s unchanged.\n", d.Path)
				continue
			}
		}
		backup, err := component.ReplaceClaudeCodeFile(d)
		if err != nil {
			return err
		}
		if d.Missing {
			fmt.Printf("  Wrote %s.\n", d.Path)
		} else {
			fmt.Printf("  Replaced %s%s.\n", d.Path, d.ReplacedNote(backup))
		}
	}
	if !changed {
		fmt.Println("Claude Code settings match ctdev's copy.")
	}
	return nil
}

package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// backupCmd holds the place of per-folder backup while it is rebuilt (CTD-87).
// The old whole-machine commands under it are gone, so `ctdev backup now` and
// the rest are unknown commands rather than silently doing nothing.
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Back up the folder you're in (coming soon)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), "Per-folder backup is coming soon (CTD-87).")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(backupCmd)
}

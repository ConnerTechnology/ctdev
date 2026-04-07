package cmd

import (
	comp "github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/spf13/cobra"
)

func completeComponentNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return comp.AllNames(), cobra.ShellCompDirectiveNoFileComp
}

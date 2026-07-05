package main

import (
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/cmd"
)

var (
	version      = "dev"
	dotfilesRoot = "" // set via -ldflags at build time
)

func main() {
	cmd.SetVersion(version)
	cmd.SetDotfilesPath(dotfilesRoot)
	// rootCmd sets SilenceErrors, so this is the single place errors print.
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

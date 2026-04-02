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
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

package main

import (
	"fmt"
	"os"

	"github.com/ConnerTechnology/ctdev/ctdev/cmd"
)

var (
	version  = "dev"
	repoRoot = "" // can be pinned with -ldflags; no build does today, so the lookup in cmd/info.go decides
)

func main() {
	cmd.SetVersion(version)
	cmd.SetRepoPath(repoRoot)
	// rootCmd sets SilenceErrors, so this is the single place errors print.
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

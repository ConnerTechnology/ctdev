package cmd

import (
	"fmt"
	"os"

	"github.com/ConnerTechnology/dotfiles/ctdev/state"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version string

	flagVerbose bool
	flagDryRun  bool
	flagForce   bool
	flagBatch   bool
)

func SetVersion(v string) {
	version = v
}

var rootCmd = &cobra.Command{
	Use:     "ctdev",
	Short:   "Development environment manager",
	Long:    "ctdev manages your development environment — install components, update packages, and configure your system.",
	Version: "",
}

func Execute() error {
	rootCmd.Version = version
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVarP(&flagDryRun, "dry-run", "n", false, "preview changes without applying")
	rootCmd.PersistentFlags().BoolVarP(&flagForce, "force", "f", false, "force re-run install scripts")
	rootCmd.PersistentFlags().BoolVar(&flagBatch, "batch", false, "non-interactive mode")

	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Migrate old markers on first run
		ms := state.DefaultMarkerStore()
		oldDir := state.ConfigDir()
		migrated, err := state.MigrateOldMarkers(oldDir, ms)
		if err != nil {
			return nil // don't fail on migration errors
		}
		if migrated > 0 && flagVerbose {
			fmt.Printf("Migrated %d old install markers\n", migrated)
		}
		return nil
	}
}

func initConfig() {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("$HOME/.config/ctdev")
	viper.AutomaticEnv()
	_ = viper.ReadInConfig()
}

func isBatchMode() bool {
	if flagBatch {
		return true
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return true
	}
	return fi.Mode()&os.ModeCharDevice == 0
}


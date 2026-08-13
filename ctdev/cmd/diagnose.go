package cmd

import (
	"fmt"
	"os"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/diagnose"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

var (
	flagDiagnoseDeep    bool
	flagDiagnoseNetwork bool
	flagDiagnoseStrict  bool
)

var diagnoseCmd = &cobra.Command{
	Use:     "diagnose",
	Aliases: []string{"doctor"},
	Short:   "Diagnose this machine's network and hardware",
	Long: "Produce a health report for the machine you're sitting at, whether or not\n" +
		"ctdev manages it: link and Wi-Fi quality, addressing, gateway, DNS, internet\n" +
		"reachability, disks, memory, and thermals — each with a plain-English\n" +
		"recommendation.\n\n" +
		"Nothing is changed. Every check is read-only, root is never required (checks\n" +
		"that need it are skipped and say so), no telemetry is sent, and no data\n" +
		"leaves the machine beyond the diagnostic probes themselves.",
	RunE: runDiagnose,
}

func init() {
	rootCmd.AddCommand(diagnoseCmd)

	diagnoseCmd.Flags().BoolVar(&flagDiagnoseDeep, "deep", false,
		"include slow probes (speed test, Wi-Fi scan, path trace)")
	diagnoseCmd.Flags().BoolVar(&flagDiagnoseNetwork, "network", false,
		"only run the network and internet checks")
	diagnoseCmd.Flags().BoolVar(&flagDiagnoseStrict, "strict", false,
		"exit non-zero when a check fails")
}

func runDiagnose(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	info := platform.Detect()

	width, isTTY := infoTerminalSize()
	if isTTY && os.Getenv("NO_COLOR") == "" {
		styles.SetDarkBackground(lipgloss.HasDarkBackground(os.Stdin, os.Stdout))
	}

	// Gathering is quick but not instant, and a blank terminal reads as a hang.
	fmt.Fprintln(os.Stderr, styles.Dimmed.Render("Diagnosing… (nothing is changed)"))

	started := time.Now()
	facts := diagnose.GatherFacts(ctx, info)

	checks := diagnose.Select(diagnose.Catalog(info, facts), flagDiagnoseDeep, true)
	if flagDiagnoseNetwork {
		checks = onlyNetworkGroups(checks)
	}
	results := diagnose.RunAll(ctx, checks, facts)

	report := diagnose.Report{
		Facts:    facts,
		Version:  version,
		Started:  started,
		Elapsed:  time.Since(started),
		Deep:     flagDiagnoseDeep,
		Checks:   checks,
		Results:  results,
		Findings: diagnose.Diagnose(results, facts),
	}

	out := diagnose.Render(report, width)
	if !isTTY || os.Getenv("NO_COLOR") != "" {
		out = ansi.Strip(out)
	}
	fmt.Print(out)

	// Exit 0 by default: main prints "Error: …" for a non-nil error, which is
	// the wrong framing for "your Wi-Fi signal is weak". --strict opts into the
	// failing exit for unattended use.
	if flagDiagnoseStrict {
		if n := countFailures(results); n > 0 {
			return fmt.Errorf("%d check(s) failed", n)
		}
	}
	return nil
}

func onlyNetworkGroups(checks []diagnose.Check) []diagnose.Check {
	out := make([]diagnose.Check, 0, len(checks))
	for _, c := range checks {
		if c.Group == diagnose.GroupNetwork || c.Group == diagnose.GroupInternet {
			out = append(out, c)
		}
	}
	return out
}

func countFailures(results map[string]diagnose.Result) int {
	n := 0
	for _, r := range results {
		if r.Severity == diagnose.Fail {
			n++
		}
	}
	return n
}

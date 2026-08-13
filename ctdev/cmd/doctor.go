package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/diagnose"
	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
)

var (
	flagDoctorDeep    bool
	flagDoctorNetwork bool
	flagDoctorStrict  bool
	flagDoctorRedact  bool
	flagDoctorReport  string

	flagDoctorUnifi          string
	flagDoctorUnifiUser      string
	flagDoctorNoIntegrations bool
)

// reportToStdout is the --report value that writes the Markdown to stdout
// instead of a file, following the usual "-" convention.
const reportToStdout = "-"

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Diagnose this machine's network and hardware",
	Long: "Produce a health report for the machine you're sitting at, whether or not\n" +
		"ctdev manages it: link and Wi-Fi quality, addressing, gateway, DNS, internet\n" +
		"reachability, disks, memory, and thermals — each with a plain-English\n" +
		"recommendation.\n\n" +
		"Nothing is changed. Every check is read-only, root is never required (checks\n" +
		"that need it are skipped and say so), no telemetry is sent, and no data\n" +
		"leaves the machine beyond the diagnostic probes themselves.",
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)

	doctorCmd.Flags().BoolVar(&flagDoctorDeep, "deep", false,
		"include slow probes (speed test, Wi-Fi scan, path trace)")
	doctorCmd.Flags().BoolVar(&flagDoctorNetwork, "network", false,
		"only run the network and internet checks")
	doctorCmd.Flags().BoolVar(&flagDoctorStrict, "strict", false,
		"exit non-zero when a check fails")
	doctorCmd.Flags().BoolVar(&flagDoctorRedact, "redact", false,
		"mask the SSID, MAC addresses, and public IP so the report is safe to share")
	// NoOptDefVal lets --report stand alone and pick its own filename, while
	// --report=<path> still writes where you say.
	doctorCmd.Flags().StringVar(&flagDoctorReport, "report", "",
		"also write a Markdown report (bare flag picks a filename; '-' writes to stdout)")
	doctorCmd.Flags().Lookup("report").NoOptDefVal = reportAutoName

	doctorCmd.Flags().StringVar(&flagDoctorUnifi, "unifi", "",
		"UniFi controller URL to read (needs CTDEV_UNIFI_API_KEY)")
	doctorCmd.Flags().StringVar(&flagDoctorUnifiUser, "unifi-user", "",
		"UniFi admin username for legacy :8443 controllers (needs CTDEV_UNIFI_PASSWORD)")
	doctorCmd.Flags().BoolVar(&flagDoctorNoIntegrations, "no-integrations", false,
		"never call a vendor API, even with credentials available")
}

// reportAutoName marks "--report given with no value", which resolves to a
// generated filename once the hostname and timestamp are known.
const reportAutoName = "\x00auto"

// integrationCreds assembles vendor credentials for this run.
//
// Nothing here writes to the machine being diagnosed. Credentials come from
// the flags you typed or from the environment, and live only as long as the
// process — which is what makes the one-line ephemeral installer safe to point
// at somebody else's laptop.
func integrationCreds(ctx context.Context) diagnose.Integrations {
	var in diagnose.Integrations

	in.UniFi.Endpoint = firstNonEmpty(flagDoctorUnifi, os.Getenv("CTDEV_UNIFI_HOST"))
	in.UniFi.APIKey = os.Getenv("CTDEV_UNIFI_API_KEY")
	in.UniFi.Username = firstNonEmpty(flagDoctorUnifiUser, os.Getenv("CTDEV_UNIFI_USER"))
	in.UniFi.Password = os.Getenv("CTDEV_UNIFI_PASSWORD")
	in.UniFi.Site = os.Getenv("CTDEV_UNIFI_SITE")

	in.Synology.Endpoint = os.Getenv("CTDEV_SYNOLOGY_HOST")
	in.Synology.Username = os.Getenv("CTDEV_SYNOLOGY_USER")
	in.Synology.Password = os.Getenv("CTDEV_SYNOLOGY_PASSWORD")

	in.Proxmox.Endpoint = os.Getenv("CTDEV_PROXMOX_HOST")
	in.Proxmox.TokenID = os.Getenv("CTDEV_PROXMOX_TOKEN_ID")
	in.Proxmox.Secret = os.Getenv("CTDEV_PROXMOX_SECRET")

	promptForMissingSecrets(ctx, &in)
	return in
}

// promptForMissingSecrets asks for a credential once, in memory, for this run
// only.
//
// This is the field path: you're standing in someone's utility room, you have
// the controller password, and you do not want to leave it on their machine.
// Nothing typed here is written anywhere — not to a config file, not to the
// environment, not to shell history — and the prompt does not echo.
//
// It only ever fires when you named an endpoint yourself, so an unattended run
// can never block waiting for input.
func promptForMissingSecrets(ctx context.Context, in *diagnose.Integrations) {
	if isBatchMode() {
		return
	}

	if in.UniFi.Endpoint != "" && in.UniFi.APIKey == "" && in.UniFi.Password == "" {
		label := "UniFi API key for " + in.UniFi.Endpoint
		if in.UniFi.Username != "" {
			label = "UniFi password for " + in.UniFi.Username + "@" + in.UniFi.Endpoint
		}
		fmt.Fprintln(os.Stderr, styles.Dimmed.Render("Not stored anywhere — used for this run only."))

		secret, err := promptSecretRequiredCtx(ctx, label, "")
		if err != nil {
			return
		}
		if in.UniFi.Username != "" {
			in.UniFi.Password = secret
		} else {
			in.UniFi.APIKey = secret
		}
	}
}

func runDoctor(cmd *cobra.Command, args []string) error {
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

	checks := diagnose.Select(diagnose.Catalog(info, facts), flagDoctorDeep, true)
	if flagDoctorNetwork {
		checks = onlyNetworkGroups(checks)
	}
	results := diagnose.RunAll(ctx, checks, facts)

	// Vendor probes run only under --deep and only with credentials. Without
	// them the gear check still names what it found and how to grant access.
	if flagDoctorDeep && !flagDoctorNoIntegrations {
		vendorChecks, vendorResults := diagnose.CollectIntegrations(ctx, integrationCreds(ctx))
		checks = append(checks, vendorChecks...)
		for id, res := range vendorResults {
			results[id] = res
		}
	}

	report := diagnose.Report{
		Facts:    facts,
		Version:  version,
		Started:  started,
		Elapsed:  time.Since(started),
		Deep:     flagDoctorDeep,
		Checks:   checks,
		Results:  results,
		Findings: diagnose.Diagnose(results, facts),
	}

	// Redaction happens once, before anything renders, so the terminal view
	// and the saved file can never disagree about what was masked.
	if flagDoctorRedact {
		report = diagnose.Redact(report)
	}

	out := diagnose.Render(report, width)
	if !isTTY || os.Getenv("NO_COLOR") != "" {
		out = ansi.Strip(out)
	}
	fmt.Print(out)

	if flagDoctorReport != "" {
		if err := writeDoctorReport(report); err != nil {
			// The report is a bonus; failing to save it must not discard the
			// diagnosis already on screen.
			fmt.Fprintln(os.Stderr, styles.Warning.Render("Could not write the report: "+err.Error()))
		}
	}

	// Exit 0 by default: main prints "Error: …" for a non-nil error, which is
	// the wrong framing for "your Wi-Fi signal is weak". --strict opts into the
	// failing exit for unattended use.
	if flagDoctorStrict {
		if n := countFailures(results); n > 0 {
			return fmt.Errorf("%d check(s) failed", n)
		}
	}
	return nil
}

// writeDoctorReport saves the Markdown report. It writes to the working
// directory rather than anywhere under $HOME: on a machine you're only
// visiting, leaving a file in someone's home directory is a change you didn't
// ask permission for.
func writeDoctorReport(report diagnose.Report) error {
	md := diagnose.Markdown(report)

	if flagDoctorReport == reportToStdout {
		fmt.Print("\n" + md)
		return nil
	}

	path := flagDoctorReport
	if path == reportAutoName {
		path = diagnose.ReportFilename(report)
	}
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return err
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	fmt.Fprintln(os.Stderr, styles.Dimmed.Render("Report written to "+abs))
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

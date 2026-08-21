package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var (
	flagBrainRemote   string
	flagBrainBranch   string
	flagBrainRepo     string
	flagBrainTokenRef string
	flagBrainTriage   string
	flagBrainSync     string
)

var configureBrainCmd = &cobra.Command{
	Use:   "brain",
	Short: "Configure the brain checkout, schedule, and Claude credential",
	Long: "Set where the agent org is checked out, when its scheduled runs fire, and the " +
		"Claude Code subscription token they authenticate with.\n\n" +
		"The token comes from `claude setup-token`, run once on a machine that has a " +
		"browser, and belongs in 1Password. It is stored here encrypted to this host's " +
		"key — never in plaintext on disk, never in the dotfiles repo, and never fetched " +
		"over the network at run time.",
	RunE: runConfigureBrain,
}

func init() {
	configureBrainCmd.Flags().StringVar(&flagBrainRepo, "repo", "", "checkout path (default "+component.BrainRepoDir+")")
	configureBrainCmd.Flags().StringVar(&flagBrainRemote, "remote", "", "git remote to track")
	configureBrainCmd.Flags().StringVar(&flagBrainBranch, "branch", "", "branch to track")
	configureBrainCmd.Flags().StringVar(&flagBrainTokenRef, "token-ref", "", "1Password reference for the Claude token, e.g. op://Private/Claude Code (node)/credential")
	configureBrainCmd.Flags().StringVar(&flagBrainTriage, "triage-schedule", "", "OnCalendar for the triage timer, ';'-separated (default \""+component.BrainDefaultTriageSchedule+"\")")
	configureBrainCmd.Flags().StringVar(&flagBrainSync, "sync-schedule", "", "OnCalendar for the checkout sync timer (default \""+component.BrainDefaultSyncSchedule+"\")")
	configureCmd.AddCommand(configureBrainCmd)
}

func runConfigureBrain(cmd *cobra.Command, args []string) error {
	return cancelToClean(configureBrain(cmdContext(cmd)))
}

// configureBrain writes /etc/ctdev/brain.conf, applies the timer schedules, and
// stores the Claude credential. Reused by `ctdev install brain` so install =
// install + configure, the same shape caddy and mcp-email-server follow.
func configureBrain(ctx context.Context) error {
	if !component.BrainDeployed() {
		return fmt.Errorf("brain is not installed — run 'ctdev install brain' first")
	}

	conf := component.BrainReadConf()
	if flagConfigShow {
		return showBrainConfig(ctx, conf)
	}

	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	defaults := component.BrainDefaults(hostShortName())

	repo := firstNonEmpty(flagBrainRepo, conf["BRAIN_REPO"], defaults["BRAIN_REPO"])
	remote := firstNonEmpty(flagBrainRemote, conf["BRAIN_REMOTE"], defaults["BRAIN_REMOTE"])
	branch := firstNonEmpty(flagBrainBranch, conf["BRAIN_BRANCH"], defaults["BRAIN_BRANCH"])
	tokenRef := firstNonEmpty(flagBrainTokenRef, conf["BRAIN_TOKEN_REF"])
	triage := firstNonEmpty(flagBrainTriage, conf["BRAIN_TRIAGE_ONCALENDAR"], component.BrainDefaultTriageSchedule)
	sync := firstNonEmpty(flagBrainSync, conf["BRAIN_SYNC_ONCALENDAR"], component.BrainDefaultSyncSchedule)

	if !isBatchMode() {
		fmt.Println(styles.Title.Render("brain"))
		fmt.Println()
		fmt.Println(styles.Dimmed.Render("  The agent org, its schedule, and the credential the schedule runs under."))
		fmt.Println()
		var err error
		if repo, err = promptWithDefaultCtx(ctx, "Checkout path", repo); err != nil {
			return err
		}
		if remote, err = promptWithDefaultCtx(ctx, "Git remote", remote); err != nil {
			return err
		}
		if branch, err = promptWithDefaultCtx(ctx, "Branch", branch); err != nil {
			return err
		}
		if triage, err = promptWithDefaultCtx(ctx, "Triage schedule (OnCalendar, ';'-separated)", triage); err != nil {
			return err
		}
		if sync, err = promptWithDefaultCtx(ctx, "Checkout sync schedule (OnCalendar)", sync); err != nil {
			return err
		}
		fmt.Println()
	}

	if err := component.BrainWriteConf(ctx, o, map[string]string{
		"BRAIN_REPO":              repo,
		"BRAIN_REMOTE":            remote,
		"BRAIN_BRANCH":            branch,
		"BRAIN_TRIAGE_ONCALENDAR": triage,
		"BRAIN_SYNC_ONCALENDAR":   sync,
		"BRAIN_TOKEN_REF":         tokenRef,
	}); err != nil {
		return err
	}
	if err := component.BrainWriteSchedule(ctx, o, triage, sync); err != nil {
		return err
	}
	fmt.Printf("  %s %s\n", styles.Value.Render("schedule"), styles.Dimmed.Render(strings.ReplaceAll(triage, ";", "  ")))

	if err := brainConfigureCredential(ctx, o, tokenRef); err != nil {
		return err
	}

	// A checkout that never reached the remote is the normal state on a fresh
	// node: the deploy key has to be added to the repo by hand first.
	if !component.BrainCheckoutPresent() {
		fmt.Println()
		fmt.Println(styles.Dimmed.Render("  No checkout yet. Add the node's deploy key to the repo, then re-run"))
		fmt.Println(styles.Dimmed.Render("  'ctdev install brain'. The key is printed by that command."))
		return nil
	}

	if component.BrainConfigured() {
		if err := component.BrainEnableTimers(ctx, o); err != nil {
			return err
		}
		fmt.Println()
		fmt.Println("brain configured — timers enabled.")
		fmt.Println(styles.Dimmed.Render("  Trigger a run now:  sudo systemctl start brain-triage.service"))
		fmt.Println(styles.Dimmed.Render("  Watch it:           journalctl -fu brain-triage.service"))
	}
	return nil
}

// brainConfigureCredential stores the Claude Code subscription token. Prefers a
// 1Password reference when one is configured and `op` can read it here;
// otherwise asks for the token directly. It never writes a plaintext fallback
// file, and it leaves an existing credential alone unless something new is
// supplied — re-running configure to change the schedule must not demand the
// token again.
func brainConfigureCredential(ctx context.Context, o sysutil.Opts, tokenRef string) error {
	have := component.BrainCredentialStored()

	if tokenRef != "" {
		token, err := component.BrainReadTokenRef(ctx, tokenRef)
		if err == nil && token != "" {
			if err := component.BrainStoreCredential(ctx, o, token); err != nil {
				return err
			}
			fmt.Printf("  %s %s\n", styles.Value.Render("credential"),
				styles.Dimmed.Render("read from "+tokenRef+", encrypted to this host"))
			return nil
		}
		fmt.Printf("  %s %s\n", styles.Value.Render("credential"),
			styles.Dimmed.Render("could not read "+tokenRef+" here: "+errText(err)))
	}

	if isBatchMode() {
		if have {
			return nil
		}
		// There is nothing sensible to apply without asking, and inventing a
		// plaintext fallback is exactly what this design refuses to do.
		fmt.Println(styles.Dimmed.Render("  No credential stored. Run 'ctdev configure brain' interactively to add one."))
		return nil
	}

	fmt.Println()
	if have {
		fmt.Println(styles.Dimmed.Render("  A credential is already stored. Press Enter to keep it."))
	} else {
		fmt.Println(styles.Dimmed.Render("  Get a token with `claude setup-token` on a machine that has a browser"))
		fmt.Println(styles.Dimmed.Render("  (it needs a Pro/Max/Team/Enterprise plan and lasts a year), keep the"))
		fmt.Println(styles.Dimmed.Render("  master copy in 1Password, and paste it here."))
	}
	token, err := promptSecretCtx(ctx, "Claude Code token", "")
	if err != nil {
		return err
	}
	if token == "" {
		if !have {
			fmt.Println(styles.Dimmed.Render("  Skipped — the triage timer stays disabled until a credential exists."))
		}
		return nil
	}
	if err := component.BrainStoreCredential(ctx, o, token); err != nil {
		return err
	}
	fmt.Printf("  %s %s\n", styles.Value.Render("credential"),
		styles.Dimmed.Render("encrypted to this host at "+component.BrainCredPath))
	return nil
}

func showBrainConfig(ctx context.Context, conf map[string]string) error {
	fmt.Println(styles.Title.Render("brain"))
	fmt.Println()

	label := styles.Label(14)
	row := func(k, v string) { fmt.Printf("  %s %s\n", label.Render(k), styles.Value.Render(v)) }

	row("checkout", component.BrainRepo())
	row("remote", firstNonEmpty(conf["BRAIN_REMOTE"], component.BrainDefaultRemote))
	row("branch", firstNonEmpty(conf["BRAIN_BRANCH"], component.BrainDefaultBranch))
	row("service acct", component.BrainUser+":"+component.BrainGroup)
	row("state", component.BrainStateDir)
	row("triage", strings.ReplaceAll(component.BrainTriageSchedule(), ";", "  "))
	row("sync", component.BrainSyncSchedule())

	row("cloned", yesNo(component.BrainCheckoutPresent()))
	row("credential", credentialState(conf["BRAIN_TOKEN_REF"]))
	row("timers", timerState(ctx))
	if v := component.BrainClaudeVersion(ctx); v != "" {
		row("claude", v)
	}
	if last := component.BrainLastRun(); last != "" {
		row("last run", last)
	}
	if pub := component.BrainDeployPublicKey(); pub != "" {
		fmt.Println()
		fmt.Println(styles.Dimmed.Render("  deploy key (needs write access on the repo):"))
		fmt.Printf("  %s\n", styles.Dimmed.Render(pub))
	}
	return nil
}

func credentialState(ref string) string {
	if !component.BrainCredentialStored() {
		return "not stored — run 'ctdev configure brain'"
	}
	s := "stored, encrypted to this host"
	if ref != "" {
		s += " (master copy: " + ref + ")"
	}
	return s
}

func timerState(ctx context.Context) string {
	var on []string
	for _, t := range []string{"brain-triage.timer", "brain-sync.timer"} {
		if component.BrainTimerEnabled(ctx, t) {
			on = append(on, strings.TrimSuffix(strings.TrimPrefix(t, "brain-"), ".timer"))
		}
	}
	if len(on) == 0 {
		return "disabled"
	}
	return "enabled: " + strings.Join(on, ", ")
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func errText(err error) string {
	if err == nil {
		return "empty value"
	}
	return err.Error()
}

func hostShortName() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "localhost"
	}
	return strings.SplitN(h, ".", 2)[0]
}

package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	comp "github.com/ConnerTechnology/ctdev/ctdev/component"
	"github.com/ConnerTechnology/ctdev/ctdev/platform"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/progress"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// toolInstalled and installTool are vars so tests can fake the tool a
// configure category needs, instead of installing it on the test machine.
var (
	toolInstalled = func(name string) bool {
		c := comp.FindByName(name)
		return c != nil && c.IsInstalled()
	}
	installTool = func(ctx context.Context, name string) error {
		resolved := comp.ResolveDependencies(comp.Registry, []string{name})
		if !flagDryRun && comp.InstallNeedsRoot(platform.Detect().PackageManager, resolved, false) {
			if err := ensureSudo(ctx); err != nil {
				return fmt.Errorf("sudo required for install: %w", err)
			}
		}
		return runWithProgress(ctx, progressOperation{mode: progress.ModeInstall, names: resolved})
	}
)

// ensureTool installs the component a configure category drives (the AWS CLI
// for `configure aws`, git for `configure git`) when it's missing: in batch
// mode without asking, otherwise after a [Y/n]. ok is false when the user said
// no, and the caller stops there.
func ensureTool(ctx context.Context, name, label string, batch bool) (ok bool, err error) {
	if toolInstalled(name) {
		return true, nil
	}
	if !batch {
		yes, err := promptYesNoCtx(ctx, fmt.Sprintf("%s isn't installed. Install it now?", label), true)
		if err != nil {
			return false, err
		}
		if !yes {
			fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  Skipped. Install it later with: ctdev install %s", name)))
			return false, nil
		}
	} else {
		fmt.Printf("%s isn't installed. Installing it first.\n", label)
	}
	if err := installTool(ctx, name); err != nil {
		return false, fmt.Errorf("install %s: %w", name, err)
	}
	if !flagDryRun && !toolInstalled(name) {
		return false, fmt.Errorf("installed %s, but it isn't on PATH yet; open a new shell and run this again", name)
	}
	return true, nil
}

// runAWSConfigureSSO hands the terminal to `aws configure sso`; a var so tests
// don't need the AWS CLI or an SSO portal.
var runAWSConfigureSSO = func(ctx context.Context) error {
	c := exec.CommandContext(ctx, "aws", "configure", "sso")
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

// offerAWSConfigureSSO runs when ~/.aws/config has no profiles: rather than
// exit, offer to create one with `aws configure sso`, then return the profiles
// it left behind. Batch mode can't drive the SSO login, so it only says how.
func offerAWSConfigureSSO(ctx context.Context, batch bool) ([]string, error) {
	const hint = "No AWS profiles found in ~/.aws/config. Set one up with: aws configure sso"
	if batch {
		fmt.Println(hint)
		return nil, nil
	}
	fmt.Println("No AWS profiles found in ~/.aws/config.")
	yes, err := promptYesNoCtx(ctx, "Set one up now with 'aws configure sso'?", true)
	if err != nil {
		return nil, err
	}
	if !yes {
		fmt.Println(styles.Dimmed.Render("  Skipped. Run 'aws configure sso', then 'ctdev configure aws'."))
		return nil, nil
	}
	if flagDryRun {
		fmt.Println("[dry-run] would run: aws configure sso")
		return nil, nil
	}
	if err := runAWSConfigureSSO(ctx); err != nil {
		return nil, fmt.Errorf("aws configure sso: %w", err)
	}
	profiles, _ := sysutil.ReadAWSProfiles()
	if len(profiles) == 0 {
		fmt.Println(hint)
	}
	return profiles, nil
}

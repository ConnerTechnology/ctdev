package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

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

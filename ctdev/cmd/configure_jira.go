package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ConnerTechnology/ctdev/ctdev/claudecli"
	"github.com/ConnerTechnology/ctdev/ctdev/gitrepo"
	"github.com/ConnerTechnology/ctdev/ctdev/jira"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var configureJiraCmd = &cobra.Command{
	Use:   "jira",
	Short: "Connect this repo's Claude Code to Jira Cloud, and keep your API token for scripts",
	Long: "Set up Jira Cloud for the git repo you are in. Claude Code gets Atlassian's Rovo MCP " +
		"server (`atlassian`) at local scope, kept in ~/.claude.json for this repo only; it signs " +
		"in as you with OAuth the first time it connects. Scripts that call Jira's REST API use " +
		"your Atlassian API token instead, entered here, checked with the site, and stored only on " +
		"this machine in ~/.config/ctdev/jira/<site>.env (owner-only). They read it with " +
		"`ctdev jira auth-header --site <site>`. Nothing is written in the repo.\n\n" +
		"With --show, report the repo's Atlassian server and each saved site, and change nothing.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cancelToClean(configureJira(cmdContext(cmd)))
	},
}

// newJiraClient is swapped in tests to point at an httptest server.
var newJiraClient = jira.NewClient

func init() {
	configureCmd.AddCommand(configureJiraCmd)
}

func configureJira(ctx context.Context) error {
	if isBatchMode() && !flagConfigShow {
		// The API token can only come from the person it belongs to.
		return errors.New("the Jira API token must be entered interactively")
	}
	return jiraWizard(ctx)
}

// jiraWizard is configureJira past the batch-mode gate, which tests call
// directly because a test's stdout is never a terminal.
func jiraWizard(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := gitrepo.Find(ctx, cwd)
	if err != nil {
		return err
	}
	local, hasLocal, err := claudecli.LocalMCPServer(repo.Root, jira.ServerName)
	if err != nil {
		return err
	}
	sites, err := jira.Sites()
	if err != nil {
		return err
	}
	client := newJiraClient()
	label := styles.Label(14)

	fmt.Println(styles.Title.Render("Jira for Claude Code"))
	fmt.Println()
	fmt.Printf("  %s %s\n", label.Render("Repo:"), styles.Value.Render(repo.Root))
	server := styles.Dimmed.Render("not set up in this repo")
	switch {
	case hasLocal && claudecli.SameEntry(local, jira.MCPEntry()):
		server = styles.Value.Render("set up (" + jira.MCPURL + ")")
	case hasLocal:
		server = styles.Warning.Render("a different atlassian server; setting up offers to replace it")
	}
	fmt.Printf("  %s %s\n", label.Render("MCP server:"), server)
	for _, site := range sites {
		printJiraSite(ctx, client, site)
	}
	if flagConfigShow {
		return nil
	}
	fmt.Println()

	if ok, err := ensureTool(ctx, "claude-code", "Claude Code", false); !ok || err != nil {
		return err
	}
	site, err := pickJiraSite(ctx, sites)
	if err != nil {
		return err
	}
	need := true
	if creds, err := jira.LoadCredentials(site); err == nil {
		if _, err := client.Myself(ctx, site, creds); err == nil {
			need, err = promptYesNoCtx(ctx, fmt.Sprintf("%s already has a working API token. Enter a new one?", site), false)
			if err != nil {
				return err
			}
		}
	}
	if need {
		saved, err := enterJiraCredentials(ctx, client, site)
		if err != nil {
			return err
		}
		if !saved {
			fmt.Println(styles.Warning.Render("  API token not saved; Claude Code was left as it was."))
			return nil
		}
	}
	fmt.Println()

	if done, err := addLocalServer(ctx, repo.Root, jira.ServerName, local, hasLocal, jira.MCPEntry()); err != nil || !done {
		return err
	}
	fmt.Println()
	fmt.Println(styles.Success.Render("  Restart Claude Code in this folder. The first time, run /mcp and sign in to Atlassian."))
	fmt.Println(styles.Dimmed.Render("  Scripts get the API token's header with: ctdev jira auth-header --site " + site))
	return nil
}

// printJiraSite prints one saved site and whether its token works.
func printJiraSite(ctx context.Context, client *jira.Client, site string) {
	label := styles.Label(14)
	fmt.Printf("  %s %s\n", label.Render("Site:"), styles.Value.Render(jira.BaseURL(site))+" "+styles.Dimmed.Render(jira.CredentialsPath(site)))
	creds, err := jira.LoadCredentials(site)
	if err == nil {
		var name string
		if name, err = client.Myself(ctx, site, creds); err == nil {
			fmt.Printf("  %s %s\n", label.Render("API token:"), styles.Success.Render("works, as "+name))
			return
		}
	}
	fmt.Printf("  %s %s\n", label.Render("API token:"), styles.Error.Render(err.Error()))
}

// pickJiraSite offers the sites this machine has a token for, or a new one.
func pickJiraSite(ctx context.Context, sites []string) (string, error) {
	if len(sites) > 0 {
		fmt.Println("  Jira sites on this computer:")
		for i, s := range sites {
			fmt.Printf("  %d) %s\n", i+1, jira.BaseURL(s))
		}
		fmt.Printf("  %d) Another site\n", len(sites)+1)
		fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
		choice, err := promptChoiceCtx(ctx, 1)
		if err != nil {
			return "", err
		}
		fmt.Println()
		if choice >= 1 && choice <= len(sites) {
			return sites[choice-1], nil
		}
	}
	fmt.Println(styles.Dimmed.Render("  The Jira site is the address you open Jira at, like example.atlassian.net."))
	for {
		in, err := promptRequiredCtx(ctx, "Jira site", "")
		if err != nil {
			return "", err
		}
		site, err := jira.NormalizeSite(in)
		if err != nil {
			fmt.Println(styles.Warning.Render("  " + err.Error()))
			continue
		}
		return site, nil
	}
}

// enterJiraCredentials walks the user through creating an API token and saves
// it once the site accepts it. saved is false when the user gives up.
func enterJiraCredentials(ctx context.Context, client *jira.Client, site string) (saved bool, err error) {
	fmt.Println(styles.Header.Render("  Create an Atlassian API token"))
	fmt.Println("    1. Open " + jira.TokensURL + " and log in")
	fmt.Println("    2. Click Create API token (not the one with scopes)")
	fmt.Printf("    3. Name it: ctdev (%s)\n", shortHostname())
	fmt.Println("    4. Choose an expiration date (at most a year), then click Create")
	fmt.Println("    5. Click Copy to clipboard. The token is shown only once.")
	fmt.Println()

	for {
		email, err := promptRequiredCtx(ctx, "Atlassian account email", "")
		if err != nil {
			return false, err
		}
		token, err := promptSecretRequiredCtx(ctx, "API token", "")
		if err != nil {
			return false, err
		}
		creds := jira.Credentials{Email: email, Token: token}
		fmt.Println(styles.Dimmed.Render("  Checking it with " + jira.BaseURL(site) + "..."))
		name, err := client.Myself(ctx, site, creds)
		if err != nil {
			fmt.Println(styles.Error.Render("  " + err.Error()))
			again, perr := promptYesNoCtx(ctx, "Enter them again?", true)
			if perr != nil {
				return false, perr
			}
			if !again {
				return false, nil
			}
			continue
		}
		fmt.Println(styles.Success.Render("  Jira knows it as " + name))
		if flagDryRun {
			fmt.Printf("  [dry-run] would save the API token to %s\n", jira.CredentialsPath(site))
			return true, nil
		}
		if err := jira.SaveCredentials(site, creds); err != nil {
			return false, fmt.Errorf("could not save the API token: %w", err)
		}
		fmt.Println(styles.Success.Render("  Saved to " + jira.CredentialsPath(site)))
		return true, nil
	}
}

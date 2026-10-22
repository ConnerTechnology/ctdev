package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/claudecli"
	"github.com/ConnerTechnology/ctdev/ctdev/gitrepo"
	"github.com/ConnerTechnology/ctdev/ctdev/linear"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var configureLinearCmd = &cobra.Command{
	Use:   "linear",
	Short: "Connect this repo's Claude Code to Linear as this computer's app",
	Long: "Set up the Linear MCP server for the git repo you are in. Claude Code reaches Linear " +
		"as an OAuth app (one per computer), so what a session does there shows up as the app " +
		"and notifies you. The app's client ID and secret are entered here and stored only on " +
		"this machine, in ~/.config/ctdev/linear/<workspace>.env (owner-only). Claude Code gets " +
		"a `linear` server at local scope (`claude mcp add-json --scope local`, kept in " +
		"~/.claude.json for this repo only) whose headersHelper is `ctdev linear mcp-headers`. " +
		"Nothing is written in the repo.\n\n" +
		"With --show, report the repo's workspace and whether it connects, and change nothing.",
	Args: cobra.NoArgs,
	RunE: runConfigureLinear,
}

func init() {
	configureCmd.AddCommand(configureLinearCmd)
}

func runConfigureLinear(cmd *cobra.Command, args []string) error {
	return cancelToClean(configureLinear(cmdContext(cmd)))
}

func configureLinear(ctx context.Context) error {
	if isBatchMode() && !flagConfigShow {
		// The client secret can only come from the person who created the app.
		return errors.New("Linear credentials must be entered interactively")
	}
	return linearWizard(ctx)
}

// linearWizard is configureLinear past the batch-mode gate, which tests call
// directly because a test's stdout is never a terminal.
func linearWizard(ctx context.Context) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := gitrepo.Find(ctx, cwd)
	if err != nil {
		return err
	}
	local, hasLocal, err := claudecli.LocalMCPServer(repo.Root, linear.ServerName)
	if err != nil {
		return err
	}
	current, hasCurrent := "", false
	if hasLocal {
		current, hasCurrent = linear.EntryWorkspace(local)
	}
	// Before CTD-109 ctdev wrote the server into .mcp.json. Its workspace is
	// the default here, so moving a repo to local scope asks nothing new.
	mcpJSON, err := os.ReadFile(repo.Path(linear.MCPJSONFile))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	legacy, hasLegacy := linear.MCPJSONWorkspace(mcpJSON)
	client := newLinearClient()

	fmt.Println(styles.Title.Render("Linear for Claude Code"))
	fmt.Println()
	fmt.Printf("  %s %s\n", styles.Label(14).Render("Repo:"), styles.Value.Render(repo.Root))
	var st linear.Status
	switch {
	case hasCurrent:
		st = client.CheckStatus(ctx, current)
		printLinearStatus(st)
	case hasLocal:
		fmt.Printf("  %s %s\n", styles.Label(14).Render("Workspace:"), styles.Dimmed.Render("not set up in this repo"))
		fmt.Println(styles.Dimmed.Render("  This repo has a local linear server that doesn't use ctdev; setting up offers to replace it."))
	case hasLegacy:
		current, hasCurrent = legacy, true
		fmt.Printf("  %s %s\n", styles.Label(14).Render("Workspace:"), styles.Dimmed.Render("not set up locally; .mcp.json names "+legacy))
		st = client.CheckStatus(ctx, current)
		printLinearStatus(st)
	default:
		fmt.Printf("  %s %s\n", styles.Label(14).Render("Workspace:"), styles.Dimmed.Render("not set up in this repo"))
	}
	if flagConfigShow {
		return nil
	}
	fmt.Println()

	if ok, err := ensureTool(ctx, "claude-code", "Claude Code", false); err != nil {
		return err
	} else if !ok {
		return errors.New("Claude Code isn't installed; nothing was changed")
	}

	ws, needCreds, err := chooseLinearWorkspace(ctx, client, current, hasCurrent, st)
	if err != nil {
		return err
	}
	if needCreds {
		saved, err := enterLinearCredentials(ctx, client, ws)
		if err != nil {
			return err
		}
		if !saved {
			fmt.Println(styles.Warning.Render("  Credentials not saved; Claude Code was left as it was."))
			return nil
		}
	}
	fmt.Println()

	if done, err := addLocalServer(ctx, repo.Root, linear.ServerName, local, hasLocal, linear.MCPEntry(ws)); err != nil || !done {
		return err
	}
	if linear.HasMCPJSONServer(mcpJSON) {
		fmt.Println(styles.Dimmed.Render("  .mcp.json also has a linear server. Claude Code uses the local one; remove the"))
		fmt.Println(styles.Dimmed.Render("  .mcp.json entry when nothing else needs it."))
	}

	fmt.Println()
	if !flagDryRun {
		printLinearStatus(client.CheckStatus(ctx, ws))
		fmt.Println()
	}
	fmt.Println(styles.Success.Render("  Restart Claude Code in this folder so it connects to the Linear server."))
	return nil
}

// printLinearStatus prints a workspace's status block.
func printLinearStatus(st linear.Status) {
	label := styles.Label(14)
	fmt.Printf("  %s %s\n", label.Render("Workspace:"), styles.Value.Render(st.Workspace))
	creds := styles.Warning.Render("missing")
	if st.HasCredentials {
		creds = styles.Value.Render("saved") + " " + styles.Dimmed.Render(linear.CredentialsPath(st.Workspace))
	}
	fmt.Printf("  %s %s\n", label.Render("Credentials:"), creds)
	if !st.HasCredentials {
		return
	}
	if st.Connected {
		teams := strings.Join(st.TeamKeys(), ", ")
		if teams == "" {
			teams = "none"
		}
		fmt.Printf("  %s %s\n", label.Render("Linear:"),
			styles.Success.Render(fmt.Sprintf("connected as %s, teams %s", orDash(st.Actor), teams)))
		return
	}
	fmt.Printf("  %s %s\n", label.Render("Linear:"), styles.Error.Render(fmt.Sprint(st.Err)))
}

// chooseLinearWorkspace picks the workspace the repo should use, and whether
// credentials have to be entered for it.
func chooseLinearWorkspace(ctx context.Context, client *linear.Client, current string, hasCurrent bool, st linear.Status) (string, bool, error) {
	if hasCurrent {
		fmt.Printf("  1) Keep %s\n", current)
		fmt.Printf("  2) Enter a new client ID and secret for %s\n", current)
		fmt.Println("  3) Switch this repo to another workspace")
		fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
		choice, err := promptChoiceCtx(ctx, 1)
		if err != nil {
			return "", false, err
		}
		fmt.Println()
		switch choice {
		case 2:
			return current, true, nil
		case 3:
			return pickLinearWorkspace(ctx, client)
		default:
			need, err := linearNeedsCredentials(ctx, st)
			return current, need, err
		}
	}
	return pickLinearWorkspace(ctx, client)
}

// pickLinearWorkspace offers the workspaces this machine has credentials for,
// or a new one.
func pickLinearWorkspace(ctx context.Context, client *linear.Client) (string, bool, error) {
	names, err := linear.Workspaces()
	if err != nil {
		return "", false, err
	}
	if len(names) > 0 {
		fmt.Println("  Workspaces on this computer:")
		for i, n := range names {
			fmt.Printf("  %d) %s\n", i+1, n)
		}
		fmt.Printf("  %d) A new workspace\n", len(names)+1)
		fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
		choice, err := promptChoiceCtx(ctx, 1)
		if err != nil {
			return "", false, err
		}
		fmt.Println()
		if choice >= 1 && choice <= len(names) {
			ws := names[choice-1]
			st := client.CheckStatus(ctx, ws)
			printLinearStatus(st)
			fmt.Println()
			need, err := linearNeedsCredentials(ctx, st)
			return ws, need, err
		}
	}

	fmt.Println(styles.Dimmed.Render("  Name the workspace: a short lowercase name for the Linear workspace,"))
	fmt.Println(styles.Dimmed.Render("  used for the credentials file. Letters, digits and dashes."))
	for {
		ws, err := promptRequiredCtx(ctx, "Workspace name", "")
		if err != nil {
			return "", false, err
		}
		if err := linear.ValidateWorkspace(ws); err != nil {
			fmt.Println(styles.Warning.Render("  " + err.Error()))
			continue
		}
		if _, err := linear.LoadCredentials(ws); err == nil {
			st := client.CheckStatus(ctx, ws)
			printLinearStatus(st)
			fmt.Println()
			need, err := linearNeedsCredentials(ctx, st)
			return ws, need, err
		}
		fmt.Println()
		return ws, true, nil
	}
}

// linearNeedsCredentials decides, from a workspace's status, whether to ask
// for credentials: always when there are none, by offer when they fail.
func linearNeedsCredentials(ctx context.Context, st linear.Status) (bool, error) {
	if !st.HasCredentials {
		return true, nil
	}
	if st.Connected {
		return false, nil
	}
	return promptYesNoCtx(ctx, fmt.Sprintf("%s doesn't connect. Enter a new client ID and secret?", st.Workspace), true)
}

// enterLinearCredentials walks the user through creating this computer's app
// and saves its credentials once Linear has issued a token with them. saved is
// false when the user gives up.
func enterLinearCredentials(ctx context.Context, client *linear.Client, ws string) (saved bool, err error) {
	fmt.Println(styles.Header.Render("  Create this computer's Linear app"))
	fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  In Linear, in the %s workspace:", ws)))
	fmt.Println("    1. Open https://linear.app/settings/api/applications/new")
	fmt.Printf("    2. Name it: Claude Code (%s)\n", shortHostname())
	fmt.Println("    3. Turn on Client credentials")
	fmt.Println("    4. Create the app")
	fmt.Println("    5. Copy the client ID and the client secret. The secret is shown only once.")
	fmt.Println()

	for {
		id, err := promptRequiredCtx(ctx, "Client ID", "")
		if err != nil {
			return false, err
		}
		secret, err := promptSecretRequiredCtx(ctx, "Client secret", "")
		if err != nil {
			return false, err
		}
		creds := linear.Credentials{ClientID: id, ClientSecret: secret}
		fmt.Println(styles.Dimmed.Render("  Checking them with Linear..."))
		if _, err := client.FetchToken(ctx, creds); err != nil {
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
		if flagDryRun {
			fmt.Printf("  [dry-run] would save the credentials to %s\n", linear.CredentialsPath(ws))
			return true, nil
		}
		if err := linear.SaveCredentials(ws, creds); err != nil {
			return false, fmt.Errorf("could not save the credentials: %w", err)
		}
		// The cached token belongs to the old app, or to a secret since rotated.
		if err := linear.ClearCache(ws); err != nil {
			return false, fmt.Errorf("could not clear the old token: %w", err)
		}
		fmt.Println(styles.Success.Render("  Saved to " + linear.CredentialsPath(ws)))
		return true, nil
	}
}

func shortHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "this computer"
	}
	h, _, _ = strings.Cut(h, ".")
	return h
}

// addLocalServer adds an MCP server at local scope for the repo, asking
// first when a different server of that name is already there. done is false
// when the user declined to replace it.
func addLocalServer(ctx context.Context, root, name string, existing json.RawMessage, hasExisting bool, want json.RawMessage) (done bool, err error) {
	if hasExisting && claudecli.SameEntry(existing, want) {
		fmt.Println(styles.Dimmed.Render("  The local " + name + " server is already set up."))
		return true, nil
	}
	if hasExisting {
		fmt.Printf("  This repo already has a local %s server:\n", name)
		fmt.Println(indentBlock(string(existing), "    "))
		fmt.Println("  It would become:")
		fmt.Println(indentBlock(string(want), "    "))
		replace, err := promptYesNoCtx(ctx, "Replace it?", true)
		if err != nil {
			return false, err
		}
		if !replace {
			fmt.Println(styles.Warning.Render("  Left the " + name + " server as it was."))
			return false, nil
		}
	}
	if flagDryRun {
		fmt.Printf("  [dry-run] would add the %s server at local scope:\n%s\n", name, indentBlock(string(want), "    "))
		return true, nil
	}
	if hasExisting {
		if err := claudecli.RemoveLocalMCPServer(ctx, root, name); err != nil {
			return false, err
		}
	}
	if err := claudecli.AddLocalMCPServer(ctx, root, name, want); err != nil {
		if hasExisting {
			// Put the old one back rather than leave the repo with neither,
			// even when the failure was a Ctrl-C.
			if rerr := claudecli.AddLocalMCPServer(context.WithoutCancel(ctx), root, name, existing); rerr != nil {
				return false, fmt.Errorf("%w; the old %s server was removed and could not be put back: %v", err, name, rerr)
			}
			return false, fmt.Errorf("%w; the old %s server was put back", err, name)
		}
		return false, err
	}
	fmt.Println(styles.Success.Render("  Added the " + name + " server for this repo, at local scope"))
	return true, nil
}

func indentBlock(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

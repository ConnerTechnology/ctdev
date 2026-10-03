package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

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
		"this machine, in ~/.config/ctdev/linear/<workspace>.env (owner-only). The repo gets a " +
		"`linear` server in .mcp.json whose headersHelper is `ctdev linear mcp-headers`, and " +
		"the server is pre-approved in .claude/settings.local.json.\n\n" +
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
	repo, err := linear.RepoRoot(ctx, cwd)
	if err != nil {
		return err
	}
	mcpPath := repo.Path(linear.MCPJSONFile)
	existing, err := os.ReadFile(mcpPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// Checked before any prompt, so a broken file stops the run before
	// credentials are saved for a repo that can't then be wired up.
	if err := linear.ValidateMCPJSON(existing); err != nil {
		return fmt.Errorf("%s: %w; fix or remove it, then run this again", mcpPath, err)
	}
	if !flagConfigShow {
		if err := validateSettingsLocal(repo); err != nil {
			return err
		}
	}
	current, hasCurrent := linear.CurrentWorkspace(existing)
	client := newLinearClient()

	fmt.Println(styles.Title.Render("Linear for Claude Code"))
	fmt.Println()
	fmt.Printf("  %s %s\n", styles.Label(14).Render("Repo:"), styles.Value.Render(repo.Root))
	var st linear.Status
	if hasCurrent {
		st = client.CheckStatus(ctx, current)
		printLinearStatus(st)
	} else {
		fmt.Printf("  %s %s\n", styles.Label(14).Render("Workspace:"), styles.Dimmed.Render("not set up in this repo"))
		if linear.HasLinearServer(existing) {
			fmt.Println(styles.Dimmed.Render("  .mcp.json has a linear server that doesn't use ctdev; setting up offers to replace it."))
		}
	}
	if flagConfigShow {
		return nil
	}
	fmt.Println()

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
			fmt.Println(styles.Warning.Render("  Credentials not saved; the repo was left as it was."))
			return nil
		}
	}
	fmt.Println()

	own, err := promptLinearOwnership(ctx, repo)
	if err != nil {
		return err
	}
	fmt.Println()

	if done, err := writeLinearRepoFiles(ctx, repo, existing, ws, own); err != nil || !done {
		return err
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

// promptLinearOwnership asks whether .mcp.json is committed (the user's own
// repo) or kept local (someone else's). The default follows the remotes.
func promptLinearOwnership(ctx context.Context, repo linear.Repo) (own bool, err error) {
	def := 2
	if linear.OwnRepoDefault(repo.Remotes(ctx)) {
		def = 1
	}
	fmt.Println("  Whose repo is this?")
	fmt.Println("  1) Your repo (commit .mcp.json)")
	fmt.Println("  2) Someone else's repo (keep .mcp.json local)")
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("Choice [%d]:", def)))
	choice, err := promptChoiceCtx(ctx, def)
	if err != nil {
		return false, err
	}
	if choice != 1 && choice != 2 {
		choice = def
	}
	return choice == 1, nil
}

// writeLinearRepoFiles writes .mcp.json and .claude/settings.local.json and
// keeps them out of git where they should be. done is false when the user
// declined to replace an existing linear server.
func writeLinearRepoFiles(ctx context.Context, repo linear.Repo, existing []byte, ws string, own bool) (done bool, err error) {
	plan, err := linear.PlanMCPJSON(existing, ws)
	if err != nil {
		return false, err
	}
	if plan.Previous != nil && plan.Changed {
		fmt.Println("  .mcp.json already has a linear server:")
		fmt.Println(indentBlock(string(plan.Previous), "    "))
		fmt.Println("  It would become:")
		fmt.Println(indentBlock(string(linear.MCPEntry(ws)), "    "))
		replace, err := promptYesNoCtx(ctx, "Replace it?", true)
		if err != nil {
			return false, err
		}
		if !replace {
			fmt.Println(styles.Warning.Render("  Left .mcp.json as it was; this repo is not connected to " + ws + "."))
			return false, nil
		}
	}
	mcpPath := repo.Path(linear.MCPJSONFile)
	if plan.Changed || existing == nil {
		if err := writeRepoFile(mcpPath, plan.Content); err != nil {
			return false, err
		}
	} else {
		fmt.Println(styles.Dimmed.Render("  .mcp.json already points at " + ws))
	}

	settingsPath := repo.Path(linear.SettingsLocalFile)
	settings, err := os.ReadFile(settingsPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	merged, changed, err := linear.MergeSettingsLocal(settings)
	if err != nil {
		return false, err
	}
	if changed {
		if err := writeRepoFile(settingsPath, merged); err != nil {
			return false, err
		}
	}

	exclude, err := repo.ExcludeFile(ctx)
	if err != nil {
		return false, err
	}
	// Every run, not just when ctdev created it: settings.local.json is
	// per-machine by design, and one that git would track is a leak waiting to
	// be committed whoever made it.
	if !repo.IsIgnored(ctx, linear.SettingsLocalFile) {
		if err := addExclude(exclude, linear.SettingsLocalFile); err != nil {
			return false, err
		}
		if repo.IsTracked(ctx, linear.SettingsLocalFile) {
			fmt.Println(styles.Warning.Render("  " + linear.SettingsLocalFile + " is already committed in this repo; excluding it won't untrack it (git rm --cached it)."))
		}
	}

	if own {
		if repo.IsIgnored(ctx, linear.MCPJSONFile) {
			fmt.Println(styles.Warning.Render("  git ignores .mcp.json here; remove it from " + exclude + " (or .gitignore) to commit it."))
		} else if plan.Changed || !repo.IsTracked(ctx, linear.MCPJSONFile) {
			fmt.Println(styles.Value.Render("  Commit .mcp.json so the repo's other machines use it too."))
		}
		return true, nil
	}
	if err := addExclude(exclude, linear.MCPJSONFile); err != nil {
		return false, err
	}
	if repo.IsTracked(ctx, linear.MCPJSONFile) {
		fmt.Println(styles.Warning.Render("  .mcp.json is already committed in this repo, so excluding it won't hide the change; don't commit it."))
	}
	return true, nil
}

func addExclude(exclude, line string) error {
	if flagDryRun {
		fmt.Printf("  [dry-run] would add %s to %s\n", line, exclude)
		return nil
	}
	added, err := linear.AppendExclude(exclude, line)
	if err != nil {
		return fmt.Errorf("could not update %s: %w", exclude, err)
	}
	if added {
		fmt.Println(styles.Success.Render(fmt.Sprintf("  Added %s to %s", line, exclude)))
	}
	return nil
}

// validateSettingsLocal checks the repo's settings.local.json can be merged
// into, before anything is prompted for or saved.
func validateSettingsLocal(repo linear.Repo) error {
	path := repo.Path(linear.SettingsLocalFile)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, _, err := linear.MergeSettingsLocal(data); err != nil {
		return fmt.Errorf("%s: %w; fix or remove it, then run this again", path, err)
	}
	return nil
}

// writeRepoFile writes a repo file atomically, keeping its mode when it
// exists and 0644 when it is new.
func writeRepoFile(path string, data []byte) error {
	if flagDryRun {
		fmt.Printf("  [dry-run] would write %s:\n%s", path, indentBlock(string(data), "    "))
		fmt.Println()
		return nil
	}
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := linear.WriteFileAtomic(path, data, mode); err != nil {
		return err
	}
	fmt.Println(styles.Success.Render("  Wrote " + path))
	return nil
}

func indentBlock(s, prefix string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"

	"github.com/ConnerTechnology/ctdev/ctdev/claudecli"
	"github.com/ConnerTechnology/ctdev/ctdev/gitrepo"
	"github.com/ConnerTechnology/ctdev/ctdev/secretfile"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

// The connertechnology plugin, from the engineering library (CON-146). The
// marketplace is pinned to a tag, and third-party marketplaces don't
// auto-update, so a repo gets a new library version only when ctdev's pin
// moves and `ctdev repo setup` runs again.
const (
	pluginMarketplace = "ConnerTechnology/engineering@connertechnology--v1.0.0"
	pluginID          = "connertechnology@connertechnology"
	setupSkill        = "/connertechnology:setup"
)

// settingsLocalFile is where Claude Code keeps a repo's local-scope settings,
// the plugin's marketplace and enabledPlugins among them.
const settingsLocalFile = ".claude/settings.local.json"

var flagRepoMarketplace string

var repoCmd = &cobra.Command{
	Use:   "repo",
	Short: "Set up the git repo you are in",
}

var repoSetupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Install what this repo needs for Claude Code, then start the setup skill",
	Long: "Run inside any git repo, yours or someone else's. Installs git, gh and Claude Code when " +
		"they're missing, signs gh in to GitHub if it isn't, adds the connertechnology marketplace " +
		"(" + pluginMarketplace + ", private, cloned over gh's login) and installs the " +
		pluginID + " plugin, both at local scope. Then it starts Claude Code on " + setupSkill +
		", which asks how this repo should be set up and writes .claude/settings.local.json.\n\n" +
		"Nothing is committed: .claude/settings.local.json is kept out of git through the repo's " +
		".git/info/exclude when nothing ignores it already. Running it again changes nothing unless " +
		"ctdev's pinned version moved. With --batch or --dry-run it stops before starting Claude Code.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cancelToClean(repoSetup(cmdContext(cmd)))
	},
}

func init() {
	repoSetupCmd.Flags().StringVar(&flagRepoMarketplace, "marketplace", pluginMarketplace, "marketplace source (a local checkout of the engineering repo, to try a change)")
	_ = repoSetupCmd.Flags().MarkHidden("marketplace")
	repoCmd.AddCommand(repoSetupCmd)
	rootCmd.AddCommand(repoCmd)
}

// ghLoggedIn, ghLogin and launchClaude are vars so tests don't reach GitHub
// or start a session.
var (
	ghLoggedIn = func(ctx context.Context) bool {
		return exec.CommandContext(ctx, "gh", "auth", "status", "--hostname", "github.com").Run() == nil
	}
	ghLogin = func(ctx context.Context) error {
		c := exec.CommandContext(ctx, "gh", "auth", "login", "--hostname", "github.com")
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		return c.Run()
	}
	launchClaude = claudecli.Launch
)

func repoSetup(ctx context.Context) error {
	return setupRepo(ctx, isBatchMode())
}

// setupRepo is repoSetup with batch mode passed in, which tests set because a
// test's stdout is never a terminal.
func setupRepo(ctx context.Context, batch bool) error {
	// git first: finding the repo needs it.
	for _, t := range []struct{ name, label string }{{"git", "Git"}, {"gh", "GitHub CLI"}, {"claude-code", "Claude Code"}} {
		ok, err := ensureTool(ctx, t.name, t.label, batch)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("repo setup needs %s; nothing was changed", t.label)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := gitrepo.Find(ctx, cwd)
	if err != nil {
		return err
	}

	fmt.Println(styles.Title.Render("Claude Code for this repo"))
	fmt.Println()
	fmt.Printf("  %s %s\n", styles.Label(14).Render("Repo:"), styles.Value.Render(repo.Root))
	fmt.Println()

	if !ghLoggedIn(ctx) {
		if batch {
			return errors.New("gh is not signed in to github.com; run 'gh auth login', then run this again")
		} else if flagDryRun {
			fmt.Println("  [dry-run] gh isn't signed in to github.com; would run: gh auth login --hostname github.com")
		} else {
			fmt.Println("  gh isn't signed in to github.com, which the private engineering repo needs.")
			if err := ghLogin(ctx); err != nil {
				return fmt.Errorf("gh auth login: %w", err)
			}
			fmt.Println()
		}
	}

	if err := excludeSettingsLocal(ctx, repo); err != nil {
		return err
	}
	if flagDryRun {
		fmt.Printf("  [dry-run] would add the marketplace %s and install %s, at local scope\n", flagRepoMarketplace, pluginID)
		fmt.Printf("  [dry-run] would start: claude %q\n", setupSkill)
		return nil
	}
	settingsPath := repo.Path(settingsLocalFile)
	before, err := os.ReadFile(settingsPath)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := installPlugin(ctx, repo.Root, flagRepoMarketplace); err != nil {
		return err
	}
	if err := keepBytesIfSame(settingsPath, before); err != nil {
		return err
	}

	if batch {
		fmt.Println()
		fmt.Printf("  Next, in a terminal here: claude %q\n", setupSkill)
		return nil
	}
	fmt.Println()
	fmt.Println(styles.Dimmed.Render("  Starting Claude Code on " + setupSkill + "..."))
	releaseProcessState()
	return launchClaude(repo.Root, setupSkill)
}

// excludeSettingsLocal keeps .claude/settings.local.json out of git before
// Claude Code writes to it. It is per-machine by design, and one that git
// would track is a leak waiting to be committed.
func excludeSettingsLocal(ctx context.Context, repo gitrepo.Repo) error {
	if repo.IsTracked(ctx, settingsLocalFile) {
		return fmt.Errorf("%s is committed in this repo, so local settings would show up in git; untrack it (git rm --cached %s) or set up from a clone where it isn't", settingsLocalFile, settingsLocalFile)
	}
	if repo.IsIgnored(ctx, settingsLocalFile) {
		return nil
	}
	exclude, err := repo.ExcludeFile(ctx)
	if err != nil {
		return err
	}
	if flagDryRun {
		fmt.Printf("  [dry-run] would add %s to %s\n", settingsLocalFile, exclude)
		return nil
	}
	added, err := gitrepo.AppendExclude(exclude, settingsLocalFile)
	if err != nil {
		return fmt.Errorf("could not update %s: %w", exclude, err)
	}
	if added {
		fmt.Println(styles.Success.Render(fmt.Sprintf("  Added %s to %s", settingsLocalFile, exclude)))
	}
	return nil
}

// keepBytesIfSame puts a file's old bytes back when its JSON says the same as
// before. Re-declaring a marketplace that is already there makes Claude Code
// rewrite settings.local.json in its own key order, and a rerun should leave
// the file exactly as it was.
func keepBytesIfSame(path string, before []byte) error {
	if before == nil {
		return nil
	}
	after, err := os.ReadFile(path)
	if err != nil || bytes.Equal(before, after) || !claudecli.SameEntry(before, after) {
		return nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	return secretfile.WriteAtomic(path, before, fi.Mode().Perm())
}

// installPlugin adds the marketplace and installs the plugin at local scope,
// then checks Claude Code lists it for this repo. Each step is a no-op when
// already done, so a rerun only moves a plugin whose pin changed.
func installPlugin(ctx context.Context, root, marketplace string) error {
	fmt.Println(styles.Dimmed.Render("  Adding the marketplace " + marketplace + "..."))
	if _, err := claudecli.AddMarketplace(ctx, root, marketplace, claudecli.GitHubCredentialEnv()...); err != nil {
		return fmt.Errorf("%w\n  Check that 'gh auth status' is signed in as an account that can read ConnerTechnology/engineering", err)
	}
	res, err := claudecli.InstallPlugin(ctx, root, pluginID)
	if err != nil {
		return err
	}
	if res.Stale() {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  Updating %s from %s to %s...", pluginID, res.InstalledVersion, res.AvailableVersion)))
		if err := claudecli.UpdatePlugin(ctx, root, pluginID); err != nil {
			return err
		}
	}
	ok, err := claudecli.LocalPluginInstalled(ctx, root, pluginID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("claude doesn't list %s at local scope for this repo after installing it; run 'claude plugin list' here to see why", pluginID)
	}
	fmt.Println(styles.Success.Render("  " + pluginID + " is installed for this repo, at local scope"))
	return nil
}

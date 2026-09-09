package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/piholelists"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/listdiff"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

const (
	gravityDB = "/etc/pihole/gravity.db"
	// embeddedLists is the version-controlled copy shipped inside the binary;
	// defaultExportPath is where it lives in a checkout of this repo.
	embeddedLists     = "configs/pihole/lists.toml"
	defaultExportPath = "ctdev/component/configs/pihole/lists.toml"
)

var (
	flagPiholeFrom string
	flagPiholeTo   string
)

var piholeCmd = &cobra.Command{
	Use:   "pihole",
	Short: "Manage this node's Pi-hole lists",
	Long: "Version-control the allow/deny/regex lists and adlists that otherwise live only in gravity.db.\n\n" +
		"  ctdev pihole sync      apply lists.toml to this Pi-hole (interactive)\n" +
		"  ctdev pihole export    write this Pi-hole's current lists back into lists.toml\n\n" +
		"Both run on the Pi-hole node itself, against the container or a native install.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown pihole subcommand %q (use sync or export)", args[0])
		}
		return cmd.Help()
	},
}

var piholeSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Apply lists.toml to this Pi-hole",
	Long: "Diff lists.toml against this Pi-hole's gravity.db and apply what you check.\n\n" +
		"Additions and updates are checked by default; removals are not — leaving a removal " +
		"unchecked keeps that entry on the Pi-hole. Use --dry-run to print the diff and " +
		"change nothing.",
	Args: cobra.NoArgs,
	RunE: runPiholeSync,
}

var piholeExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write this Pi-hole's lists into lists.toml",
	Long:  "Read every list out of gravity.db and write it to lists.toml (default " + defaultExportPath + ") so it can be committed.",
	Args:  cobra.NoArgs,
	RunE:  runPiholeExport,
}

func init() {
	piholeSyncCmd.Flags().StringVar(&flagPiholeFrom, "from", "", "read lists from this file instead of the built-in one")
	piholeExportCmd.Flags().StringVar(&flagPiholeTo, "to", defaultExportPath, "write the lists to this file")
	piholeCmd.AddCommand(piholeSyncCmd, piholeExportCmd)
	rootCmd.AddCommand(piholeCmd)
}

// gravity runs SQL against the node's gravity.db, through `docker exec` for the
// container or sudo for a native install (sysutil decides which).
type gravity struct {
	o sysutil.Opts
}

func (inst gravity) Query(ctx context.Context, sql string) ([]byte, error) {
	out, err := sysutil.PiholeCapture(ctx, "pihole-FTL", "sqlite3", "-json", gravityDB, sql)
	return []byte(out), err
}

func (inst gravity) Exec(ctx context.Context, sql string) error {
	return sysutil.PiholeRun(ctx, inst.o, "pihole-FTL", "sqlite3", gravityDB, sql)
}

func runPiholeSync(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	if !sysutil.PiholeAvailable() {
		return fmt.Errorf("pihole is not installed (host or container) on this node")
	}

	file, err := loadLists(flagPiholeFrom)
	if err != nil {
		return err
	}
	ex := gravity{o: sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}}
	live, err := piholelists.ReadLive(ctx, ex)
	if err != nil {
		return fmt.Errorf("read Pi-hole lists: %w", err)
	}
	groups, err := piholelists.ReadGroups(ctx, ex)
	if err != nil {
		return fmt.Errorf("read Pi-hole groups: %w", err)
	}

	changes := piholelists.Diff(file, live)
	if len(changes) == 0 {
		fmt.Println(styles.Success.Render("Pi-hole matches lists.toml."))
		return nil
	}

	if flagDryRun {
		printDiff(os.Stdout, changes)
		return nil
	}
	if isBatchMode() {
		return fmt.Errorf("ctdev pihole sync needs a terminal to pick what to apply; run it with --dry-run to see the %d change(s)", len(changes))
	}

	m := listdiff.New(changes)
	p := tea.NewProgram(&m)
	result, err := p.Run()
	resetTerminal()
	if err != nil {
		return err
	}
	picked := result.(*listdiff.Model).GetResult()
	if picked.Quit || len(picked.Selected) == 0 {
		fmt.Println("Nothing applied.")
		return nil
	}

	if err := ensureSudo(ctx); err != nil {
		return fmt.Errorf("sudo required: %w", err)
	}
	if err := piholelists.Apply(ctx, ex, picked.Selected, groups); err != nil {
		return fmt.Errorf("apply changes: %w", err)
	}

	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
	// Gravity is a full re-download of every adlist, so only pay for it when an
	// adlist actually changed; domainlist edits reload in under a second.
	if piholelists.NeedsGravity(picked.Selected) {
		fmt.Println("Rebuilding gravity (pihole -g)...")
		err = sysutil.PiholeRun(ctx, o, "pihole", "-g")
	} else {
		err = sysutil.PiholeRun(ctx, o, "pihole", "reloadlists")
	}
	if err != nil {
		return fmt.Errorf("reload Pi-hole: %w", err)
	}

	added, updated, removed := countByOp(picked.Selected)
	fmt.Println(styles.Success.Render(fmt.Sprintf("Applied: %d added, %d updated, %d removed.", added, updated, removed)))

	if left := unrecordedRemovals(changes, picked.Selected); left > 0 {
		fmt.Printf("\n%d entries on this Pi-hole are not in lists.toml. To record them, run\n", left)
		fmt.Printf("  ctdev pihole export --to <checkout>/%s\n", defaultExportPath)
		fmt.Println("and commit the result.")
	}
	return nil
}

func runPiholeExport(cmd *cobra.Command, args []string) error {
	ctx := cmdContext(cmd)
	if !sysutil.PiholeAvailable() {
		return fmt.Errorf("pihole is not installed (host or container) on this node")
	}

	ex := gravity{o: sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}}
	live, err := piholelists.ReadLive(ctx, ex)
	if err != nil {
		return fmt.Errorf("read Pi-hole lists: %w", err)
	}

	if flagDryRun {
		fmt.Printf("[dry-run] would write %d entries to %s\n", len(live), flagPiholeTo)
		return nil
	}
	if dir := filepath.Dir(flagPiholeTo); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(flagPiholeTo, piholelists.Encode(live), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", flagPiholeTo, err)
	}

	for _, kind := range piholelists.Kinds {
		fmt.Printf("  %-14s %d\n", kind.Label(), len(live.OfKind(kind)))
	}
	fmt.Printf("\nWrote %s — commit it to keep the Pi-hole reproducible.\n", flagPiholeTo)
	return nil
}

// loadLists reads the lists file: the one embedded in the binary, or the path
// given to --from.
func loadLists(from string) (piholelists.Lists, error) {
	if from == "" {
		raw, err := component.Configs.ReadFile(embeddedLists)
		if err != nil {
			return nil, err
		}
		return piholelists.Parse(raw)
	}
	raw, err := os.ReadFile(from)
	if err != nil {
		return nil, err
	}
	lists, err := piholelists.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", from, err)
	}
	return lists, nil
}

func printDiff(w io.Writer, changes []piholelists.Change) {
	var current piholelists.Kind = -1
	for _, c := range changes {
		if c.Entry.Kind != current {
			current = c.Entry.Kind
			fmt.Fprintf(w, "\n%s\n", styles.Header.Render(current.Label()))
		}
		fmt.Fprintf(w, "  %-6s %s", c.Op, c.Entry.Key)
		if detail := c.Detail(); detail != "" {
			fmt.Fprintf(w, "  %s", styles.Dimmed.Render(detail))
		}
		fmt.Fprintln(w)
	}
	added, updated, removed := countByOp(changes)
	fmt.Fprintf(w, "\n%d to add, %d to update, %d on the Pi-hole but not in lists.toml.\n", added, updated, removed)
}

func countByOp(changes []piholelists.Change) (added, updated, removed int) {
	for _, c := range changes {
		switch c.Op {
		case piholelists.Add:
			added++
		case piholelists.Update:
			updated++
		case piholelists.Remove:
			removed++
		}
	}
	return added, updated, removed
}

// unrecordedRemovals counts the entries the user chose to keep on the Pi-hole:
// they exist there but not in lists.toml, so the next sync proposes them again
// until someone exports.
func unrecordedRemovals(all, applied []piholelists.Change) int {
	kept := map[string]bool{}
	for _, c := range all {
		if c.Op == piholelists.Remove {
			kept[c.Entry.Kind.Key()+"\x00"+c.Entry.Key] = true
		}
	}
	for _, c := range applied {
		delete(kept, c.Entry.Kind.Key()+"\x00"+c.Entry.Key)
	}
	return len(kept)
}

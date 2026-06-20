package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
	"github.com/spf13/cobra"
)

// Pi-hole's lists are version-controlled as plain-text files (one entry per
// line) under ctdev/component/configs/pihole/, embedded in the binary. Custom
// DNS records map internal hostnames to private IPs, so they're SOPS-encrypted
// (see .sops.yaml) into hosts/<hostname>.sops.json rather than committed plain.
//
//   ctdev pihole export   # current Pi-hole state → text files (to commit)
//   ctdev pihole import    # text files → Pi-hole, then rebuild gravity

const (
	gravityDB          = "/etc/pihole/gravity.db"
	piholeConfigSubdir = "configs/pihole" // embedded path
	defaultPiholeOut   = "ctdev/component/configs/pihole"
)

// piholeList describes one version-controlled list and where it lives in
// gravity.db. dlType < 0 marks the adlist table; otherwise it's a domainlist
// type (0=allow, 1=deny, 2=allow-regex, 3=deny-regex).
type piholeList struct {
	file     string
	header   string
	dlType   int
	applyCLI []string // pihole CLI verb for import (nil for adlists)
}

var piholeLists = []piholeList{
	{"adlists.txt", "Pi-hole adlists (block-list URLs)", -1, nil},
	{"allowlist.txt", "Pi-hole exact allow domains", 0, []string{"allow"}},
	{"denylist.txt", "Pi-hole exact deny domains", 1, []string{"deny"}},
	{"regex-allow.txt", "Pi-hole allow regex filters", 2, []string{"--allow-regex"}},
	{"regex-deny.txt", "Pi-hole deny regex filters", 3, []string{"--regex"}},
}

var (
	flagPiholeOut  string
	flagPiholeFrom string
)

var piholeCmd = &cobra.Command{
	Use:   "pihole",
	Short: "Version-control Pi-hole lists (export/import)",
	Long:  "Capture Pi-hole's adlists, allow/deny lists, and regex filters as text files for version control, and apply them back to reproduce the setup.",
}

var piholeExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export Pi-hole lists to text files",
	Long:  "Read the current adlists, allow/deny lists, and regex filters from Pi-hole and write them as text files (default: " + defaultPiholeOut + "). Custom DNS records, if any, are SOPS-encrypted.",
	RunE:  runPiholeExport,
}

var piholeImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Apply Pi-hole lists from text files, then rebuild gravity",
	Long:  "Apply the version-controlled adlists, allow/deny lists, and regex filters to Pi-hole and rebuild gravity. Additive: it adds entries, it does not remove ones absent from the files. Reads built-in lists unless --from is given.",
	RunE:  runPiholeImport,
}

func init() {
	piholeExportCmd.Flags().StringVar(&flagPiholeOut, "out", defaultPiholeOut, "directory to write list files to")
	piholeImportCmd.Flags().StringVar(&flagPiholeFrom, "from", "", "directory to read list files from (default: built-in)")
	piholeCmd.AddCommand(piholeExportCmd, piholeImportCmd)
	rootCmd.AddCommand(piholeCmd)
}

func runPiholeExport(cmd *cobra.Command, args []string) error {
	if !sysutil.PiholeAvailable() {
		return fmt.Errorf("pihole is not installed (host or container) on this node")
	}
	ctx := cmdContext(cmd)
	if err := os.MkdirAll(flagPiholeOut, 0o755); err != nil {
		return err
	}

	for _, l := range piholeLists {
		var query string
		if l.dlType < 0 {
			query = "SELECT address FROM adlist WHERE enabled=1 ORDER BY address;"
		} else {
			query = fmt.Sprintf("SELECT domain FROM domainlist WHERE type=%d ORDER BY domain;", l.dlType)
		}
		out, err := gravityQuery(ctx, query)
		if err != nil {
			return fmt.Errorf("read %s: %w", l.file, err)
		}
		entries := nonEmptyLines(out)
		if err := writeListFile(filepath.Join(flagPiholeOut, l.file), l.header, entries); err != nil {
			return err
		}
		fmt.Printf("  %-16s %d entries\n", l.file, len(entries))
	}

	n, err := exportCustomDNS(ctx, flagPiholeOut)
	if err != nil {
		return fmt.Errorf("export custom DNS: %w", err)
	}
	if n > 0 {
		fmt.Printf("  %-16s %d records (SOPS-encrypted)\n", "custom DNS", n)
	}

	fmt.Printf("\n  Exported Pi-hole lists to %s\n", flagPiholeOut)
	return nil
}

func runPiholeImport(cmd *cobra.Command, args []string) error {
	if !sysutil.PiholeAvailable() {
		return fmt.Errorf("pihole is not installed (host or container) on this node")
	}
	ctx := cmdContext(cmd)
	if !flagDryRun {
		if err := ensureSudo(); err != nil {
			return fmt.Errorf("sudo required: %w", err)
		}
	}
	o := sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}

	for _, l := range piholeLists {
		entries, err := readListFile(l.file)
		if err != nil {
			return fmt.Errorf("read %s: %w", l.file, err)
		}
		if len(entries) == 0 {
			continue
		}
		if l.dlType < 0 {
			if err := importAdlists(ctx, o, entries); err != nil {
				return fmt.Errorf("import adlists: %w", err)
			}
		} else {
			cliArgs := append(append([]string{}, l.applyCLI...), "--quiet")
			cliArgs = append(cliArgs, entries...)
			if o.DryRun {
				fmt.Printf("  [dry-run] pihole %s (%d entries)\n", strings.Join(l.applyCLI, " "), len(entries))
			} else if err := sysutil.PiholeRun(ctx, o, append([]string{"pihole"}, cliArgs...)...); err != nil {
				return fmt.Errorf("apply %s: %w", l.file, err)
			}
		}
		fmt.Printf("  %-16s %d entries\n", l.file, len(entries))
	}

	if err := importCustomDNS(ctx, o); err != nil {
		return fmt.Errorf("import custom DNS: %w", err)
	}

	// Rebuild gravity so the adlist changes take effect.
	if o.DryRun {
		fmt.Println("  [dry-run] pihole -g")
		return nil
	}
	fmt.Println("\n  Rebuilding gravity (pihole -g)...")
	return sysutil.PiholeRun(ctx, o, "pihole", "-g")
}

// importAdlists inserts adlist URLs into gravity.db (idempotently) and links
// each to the default group, matching what the web UI does.
func importAdlists(ctx context.Context, o sysutil.Opts, urls []string) error {
	var sb strings.Builder
	for _, url := range urls {
		esc := strings.ReplaceAll(url, "'", "''")
		fmt.Fprintf(&sb, "INSERT INTO adlist (address,enabled,comment) SELECT '%s',1,'managed by ctdev' WHERE NOT EXISTS (SELECT 1 FROM adlist WHERE address='%s');\n", esc, esc)
		fmt.Fprintf(&sb, "INSERT OR IGNORE INTO adlist_by_group (adlist_id,group_id) SELECT id,0 FROM adlist WHERE address='%s';\n", esc)
	}
	if o.DryRun {
		fmt.Printf("  [dry-run] insert %d adlists into gravity.db\n", len(urls))
		return nil
	}
	return sysutil.PiholeRun(ctx, o, "pihole-FTL", "sqlite3", gravityDB, sb.String())
}

// gravityQuery runs a read-only query against gravity.db (container or host
// install) and returns stdout.
func gravityQuery(ctx context.Context, query string) (string, error) {
	return sysutil.PiholeCapture(ctx, "pihole-FTL", "sqlite3", gravityDB, query)
}

// writeListFile writes a managed list file with a header comment.
func writeListFile(path, header string, entries []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s — one per line. Managed by `ctdev pihole`.\n", header)
	b.WriteString("# Apply with `ctdev pihole import`.\n")
	for _, e := range entries {
		b.WriteString(e)
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// readListFile reads a list's entries from --from DIR when set, else from the
// binary's embedded copy. A missing file yields an empty list.
func readListFile(name string) ([]string, error) {
	var data []byte
	var err error
	if flagPiholeFrom != "" {
		data, err = os.ReadFile(filepath.Join(flagPiholeFrom, name))
		if os.IsNotExist(err) {
			return nil, nil
		}
	} else {
		data, err = component.Configs.ReadFile(piholeConfigSubdir + "/" + name)
		if err != nil {
			return nil, nil // not embedded → treat as empty
		}
	}
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(string(data)), nil
}

// nonEmptyLines splits text into trimmed lines, dropping blanks and # comments.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

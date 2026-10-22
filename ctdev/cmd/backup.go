package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/backupscreen"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

// backupCmd is per-folder backup (CTD-87). It's a thin layer over the backup
// package: the status and menu screen, and the prompts behind Configure and
// Back up now.
var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Back up the folder you're in",
	Long: "Show whether the folder you're in is backed up, with its status and a menu. " +
		"ctdev looks up from the current directory for the nearest .ctdev-backup.json, the way " +
		"git finds its repository. Without a terminal, or with --batch, it prints the status " +
		"only, and exits non-zero when the folder isn't backed up or its last backup failed.",
	Args: cobra.NoArgs,
	RunE: runBackup,
}

func init() {
	rootCmd.AddCommand(backupCmd)
}

func runBackup(cmd *cobra.Command, _ []string) error {
	ctx := cmdContext(cmd)
	m, err := backup.ThisMachine(sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun})
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if isBatchMode() {
		return backupStatusOnly(ctx, m, cwd, cmd.OutOrStdout())
	}
	for {
		s := m.Status(ctx, cwd)
		screen := backupscreen.New(s, m.Home)
		if _, err := tea.NewProgram(screen).Run(); err != nil {
			return err
		}
		switch screen.Chosen() {
		case backupscreen.BackUpNow:
			err = backupNow(ctx, m, s)
		case backupscreen.Configure:
			err = configureFolder(ctx, m, s)
		default:
			return nil
		}
		if errors.Is(err, errPromptCancelled) {
			if ctx.Err() != nil {
				return nil
			}
			fmt.Println("\n  Cancelled.")
			err = nil
		}
		if err != nil {
			fmt.Println("\n  " + styles.Error.Render(err.Error()))
		}
		fmt.Println()
	}
}

// backupStatusOnly prints the status for a pipe or --batch, and fails when
// the folder isn't backed up or its last backup failed, so it works as a check.
func backupStatusOnly(ctx context.Context, m *backup.Machine, cwd string, out io.Writer) error {
	s := m.Status(ctx, cwd)
	fmt.Fprint(out, backupscreen.Status(s, m.Home))
	switch {
	case s.Config == nil:
		return fmt.Errorf("%s isn't backed up: run ctdev backup in a terminal and choose Configure", backup.DisplayPath(m.Home, s.Target))
	case s.Failed():
		return fmt.Errorf("the last backup of %s failed", backup.DisplayPath(m.Home, s.Target))
	}
	return nil
}

// backupNow backs up the whole folder after a confirmation. A folder that
// isn't configured is backed up once to this computer's destination, which
// is set up first when there isn't one, and gets no config file or schedule.
func backupNow(ctx context.Context, m *backup.Machine, s backup.Status) error {
	var d *backup.Destination
	var err error
	if s.Config != nil {
		if s.DestinationMissing {
			return fmt.Errorf("destination %s isn't set up on this computer: choose Configure to connect to it", s.Config.Destination)
		}
		d, err = m.Destination(s.Config.Destination)
	} else {
		d, err = pickDestinationForOnce(ctx, m, s.Target)
	}
	if err != nil {
		return err
	}

	where := backup.DisplayPath(m.Home, s.Target)
	fmt.Println()
	ok, err := promptYesNoCtx(ctx, fmt.Sprintf("This backs up all of %s: %s, %s. Continue?",
		where, backupscreen.Bytes(s.Size), backupscreen.Files(s.Files)), true)
	if err != nil || !ok {
		return err
	}
	fmt.Printf("  Backing up %s to %s…\n", where, d.Name)
	snap, err := m.Backup(ctx, d, s.Target, s.Config)
	if err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	if m.Opts.DryRun {
		return nil
	}
	fmt.Println("  " + styles.Success.Render(fmt.Sprintf("✓ Backed up %s (%s), %s new, as %s.",
		backupscreen.Files(snap.Files), backupscreen.Bytes(snap.Size), backupscreen.Bytes(snap.Added), snap.ShortID)))
	if snap.Warning != "" {
		fmt.Println("  " + styles.Warning.Render("Note: "+snap.Warning+"."))
	}
	return nil
}

// pickDestinationForOnce is this computer's destination for a one-off
// backup: the only one, a choice among several, or a new one set up now.
func pickDestinationForOnce(ctx context.Context, m *backup.Machine, folder string) (*backup.Destination, error) {
	names, err := m.Destinations()
	if err != nil {
		return nil, err
	}
	switch len(names) {
	case 0:
		fmt.Println("\n  This computer has no backup destination yet. Set one up first.")
		return chooseDestination(ctx, m, "", folder)
	case 1:
		return m.Destination(names[0])
	}
	fmt.Println("\n  Back up to which destination?")
	for i, n := range names {
		fmt.Printf("    %d) %s\n", i+1, n)
	}
	fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
	choice, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return nil, err
	}
	if choice < 1 || choice > len(names) {
		return nil, fmt.Errorf("invalid choice %d", choice)
	}
	return m.Destination(names[choice-1])
}

// configureFolder sets up the folder (or reconfigures it, starting from its
// current values): destination, schedule and retention.
func configureFolder(ctx context.Context, m *backup.Machine, s backup.Status) error {
	target := s.Target
	where := backup.DisplayPath(m.Home, target)
	if s.Root != "" && s.Root != s.Cwd {
		// From a subfolder, Configure would set up the subfolder inside the
		// backed-up folder; CheckConfigurable refuses that.
		target = s.Cwd
	}
	contained, err := backup.CheckConfigurable(target)
	var inside *backup.InsideError
	if errors.As(err, &inside) {
		fmt.Println("\n  " + styles.Warning.Render("Already backed up as part of "+backup.DisplayPath(m.Home, inside.Root)+"."))
		return nil
	}
	if err != nil {
		return err
	}
	if len(contained) > 0 {
		fmt.Println()
		fmt.Println("  " + styles.Warning.Render("These backed-up folders are inside "+where+", so they'll be in both folders' backups:"))
		for _, c := range contained {
			fmt.Println("    " + backup.DisplayPath(m.Home, c))
		}
		ok, err := promptYesNoCtx(ctx, "Configure "+where+" anyway?", true)
		if err != nil || !ok {
			return err
		}
	}

	cfg := backup.NewConfig("")
	if s.Config != nil {
		cfg = *s.Config
	}
	fmt.Println()
	fmt.Println(styles.Title.Render("  Configure backups of " + where))
	d, err := chooseDestination(ctx, m, cfg.Destination, target)
	if err != nil {
		return err
	}
	cfg.Destination = d.Name
	if cfg.Schedule, err = promptSchedule(ctx, cfg.Schedule); err != nil {
		return err
	}
	if cfg.Keep, err = promptKeep(ctx, cfg.Keep); err != nil {
		return err
	}
	if err := m.Configure(ctx, d, target, cfg); err != nil {
		return err
	}
	if m.Opts.DryRun {
		return nil
	}
	fmt.Println()
	fmt.Println("  " + styles.Success.Render(fmt.Sprintf("✓ %s backs up %s to %s.", where, cfg.Schedule, d.Name)))
	fmt.Println("  " + styles.Dimmed.Render("Scheduled runs come in a later ctdev release; until then, use Back up now."))
	return nil
}

// chooseDestination picks one of this computer's destinations, or sets up a
// new one by creating it or connecting to an existing one. preferred, the
// folder's current destination, is the default.
func chooseDestination(ctx context.Context, m *backup.Machine, preferred, folder string) (*backup.Destination, error) {
	names, err := m.Destinations()
	if err != nil {
		return nil, err
	}
	if preferred != "" && !slices.Contains(names, preferred) {
		fmt.Printf("\n  This folder backs up to %s, which isn't set up on this computer.\n", preferred)
	}
	fmt.Println("\n  Destination:")
	def := 0
	for i, n := range names {
		fmt.Printf("    %d) %s\n", i+1, n)
		if n == preferred || (def == 0 && preferred == "") {
			def = i + 1
		}
	}
	create, connect := len(names)+1, len(names)+2
	fmt.Printf("    %d) Create a new destination\n", create)
	fmt.Printf("    %d) Connect to an existing destination (Emergency Kit)\n", connect)
	if def == 0 {
		def = create
		if preferred != "" {
			def = connect
		}
	}
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("Choice [%d]:", def)))
	choice, err := promptChoiceCtx(ctx, def)
	if err != nil {
		return nil, err
	}
	switch {
	case choice >= 1 && choice <= len(names):
		return m.Destination(names[choice-1])
	case choice == create:
		return createDestination(ctx, m, preferred, folder)
	case choice == connect:
		return connectDestination(ctx, m)
	}
	return nil, fmt.Errorf("invalid choice %d", choice)
}

func createDestination(ctx context.Context, m *backup.Machine, name, folder string) (*backup.Destination, error) {
	if m.Opts.DryRun {
		return nil, backup.ErrDryRun
	}
	fmt.Println()
	name, err := promptRequiredCtx(ctx, "Name for the destination", firstNonEmpty(name, "backups"))
	if err != nil {
		return nil, err
	}
	var address string
	var creds map[string]string
	for {
		if address, creds, err = promptBackend(ctx); err != nil {
			return nil, err
		}
		// Check before asking for anything else: a local path inside the
		// folder would back the repository up into itself.
		if address, err = m.CheckAddress(address, folder); err == nil {
			break
		}
		fmt.Println("  " + styles.Warning.Render(err.Error()+": choose another place."))
	}
	fmt.Println()
	fmt.Println(styles.Dimmed.Render("  The master password and a Secret Key ctdev generates together unlock the backups."))
	fmt.Println(styles.Dimmed.Render("  ctdev never stores the master password: choose one you'll remember."))
	master, err := promptNewMasterPassword(ctx)
	if err != nil {
		return nil, err
	}
	fmt.Println("\n  Creating the destination (this takes a few seconds)…")
	setup, err := m.CreateDestination(ctx, backup.NewDestination{Name: name, Address: address, Credentials: creds}, master)
	if err != nil {
		return nil, err
	}
	if err := showSecrets(ctx, setup); err != nil {
		return nil, err
	}
	return setup.Destination, nil
}

// showSecrets shows the Emergency Kit and the Recovery Code, once, and waits
// while they're saved.
func showSecrets(ctx context.Context, s backup.Setup) error {
	fmt.Println()
	fmt.Println(styles.Title.Render("  Emergency Kit") + styles.Dimmed.Render("  save it in your password manager or print it"))
	fmt.Println()
	for _, line := range strings.Split(strings.TrimRight(s.Kit, "\n"), "\n") {
		fmt.Println("    " + line)
	}
	fmt.Println()
	fmt.Println(styles.Title.Render("  Recovery Code") + styles.Dimmed.Render("  keep it apart from the kit; it resets a forgotten master password"))
	fmt.Println()
	fmt.Println("    " + styles.Value.Render(s.RecoveryCode))
	fmt.Println()
	fmt.Println("  " + styles.Warning.Render("ctdev shows these once. Lose the kit and the master password, with no Recovery Code"))
	fmt.Println("  " + styles.Warning.Render("and no computer still set up, and the backups can't be decrypted by anyone."))
	fmt.Printf("\n  %s ", styles.Dimmed.Render("Press Enter once both are saved."))
	if _, ok := readLineCtx(ctx); !ok {
		return errPromptCancelled
	}
	return nil
}

func connectDestination(ctx context.Context, m *backup.Machine) (*backup.Destination, error) {
	if m.Opts.DryRun {
		return nil, backup.ErrDryRun
	}
	fmt.Println()
	fmt.Println("  Paste the Emergency Kit, then press Enter on an empty line:")
	var lines []string
	for {
		line, ok := readLineCtx(ctx)
		if !ok {
			return nil, errPromptCancelled
		}
		if line != "" {
			lines = append(lines, line)
			continue
		}
		if len(lines) == 0 {
			continue
		}
		if _, err := backup.ParseKit(strings.Join(lines, "\n")); err != nil {
			fmt.Println("  " + styles.Warning.Render(err.Error()+": paste the whole kit again."))
			lines = nil
			continue
		}
		break
	}
	kit := strings.Join(lines, "\n")
	for range 3 {
		master, err := promptSecretRequiredCtx(ctx, "Master password", "")
		if err != nil {
			return nil, err
		}
		fmt.Println("  Checking the kit and password against the destination…")
		d, err := m.ConnectDestination(ctx, kit, master)
		if errors.Is(err, backup.ErrWrongPassword) {
			fmt.Println("  " + styles.Warning.Render("That master password and Secret Key don't open the destination. Nothing was saved."))
			continue
		}
		if err != nil {
			return nil, err
		}
		fmt.Println("  " + styles.Success.Render("✓ This computer is set up on "+d.Name+" with its own key."))
		return d, nil
	}
	return nil, errors.New("three wrong tries; nothing was saved")
}

// promptNewMasterPassword asks twice, since the input is masked.
func promptNewMasterPassword(ctx context.Context) (string, error) {
	for {
		pw, err := promptSecretRequiredCtx(ctx, "Master password", "")
		if err != nil {
			return "", err
		}
		if len(pw) < 10 {
			fmt.Println(styles.Warning.Render("  Use at least 10 characters."))
			continue
		}
		again, err := promptSecretRequiredCtx(ctx, "Type it again", "")
		if err != nil {
			return "", err
		}
		if again == pw {
			return pw, nil
		}
		fmt.Println(styles.Warning.Render("  Those don't match; try again."))
	}
}

// promptBackend asks where backups go and returns the restic repository and
// the backend's credentials.
func promptBackend(ctx context.Context) (string, map[string]string, error) {
	creds := map[string]string{}
	host, _ := os.Hostname()
	fmt.Println("\n  Where should backups go?")
	fmt.Println("    1) Backblaze B2")
	fmt.Println("    2) Amazon S3 or S3-compatible")
	fmt.Println("    3) SFTP (another server over SSH)")
	fmt.Println("    4) Local path or USB drive")
	fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
	choice, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return "", nil, err
	}
	ask := func(label, def string) (string, error) { return promptRequiredCtx(ctx, label, def) }
	secret := func(name, label string) error {
		v, err := promptSecretRequiredCtx(ctx, label, "")
		creds[name] = v
		return err
	}
	switch choice {
	case 1:
		bucket, err := ask("B2 bucket name", "")
		if err != nil {
			return "", nil, err
		}
		path, err := promptWithDefaultCtx(ctx, "Path inside the bucket", "ctdev")
		if err != nil {
			return "", nil, err
		}
		if err := secret("B2_ACCOUNT_ID", "B2 keyID"); err != nil {
			return "", nil, err
		}
		if err := secret("B2_ACCOUNT_KEY", "B2 applicationKey"); err != nil {
			return "", nil, err
		}
		return backup.B2Address(bucket, path), creds, nil
	case 2:
		bucket, err := ask("S3 bucket name", "")
		if err != nil {
			return "", nil, err
		}
		endpoint, err := ask("Endpoint host", "s3.amazonaws.com")
		if err != nil {
			return "", nil, err
		}
		path, err := promptWithDefaultCtx(ctx, "Path inside the bucket", "ctdev")
		if err != nil {
			return "", nil, err
		}
		if err := secret("AWS_ACCESS_KEY_ID", "Access key ID"); err != nil {
			return "", nil, err
		}
		if err := secret("AWS_SECRET_ACCESS_KEY", "Secret access key"); err != nil {
			return "", nil, err
		}
		return backup.S3Address(endpoint, bucket, path), creds, nil
	case 3:
		fmt.Println(styles.Dimmed.Render("  restic signs in with your own SSH keys and config."))
		userHost, err := ask("SFTP user@host", "")
		if err != nil {
			return "", nil, err
		}
		path, err := ask("Path on the server", "/srv/backups/"+firstNonEmpty(host, "ctdev"))
		if err != nil {
			return "", nil, err
		}
		return backup.SFTPAddress(userHost, path), creds, nil
	case 4:
		path, err := ask("Path", "")
		if err != nil {
			return "", nil, err
		}
		return path, creds, nil
	}
	return "", nil, fmt.Errorf("invalid choice %d (pick 1–4)", choice)
}

func promptSchedule(ctx context.Context, current backup.Schedule) (backup.Schedule, error) {
	fmt.Println("\n  How often?")
	def := 2
	for i, s := range backup.Schedules {
		fmt.Printf("    %d) %s%s\n", i+1, strings.ToUpper(string(s[:1])), s[1:])
		if s == current {
			def = i + 1
		}
	}
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("Choice [%d]:", def)))
	choice, err := promptChoiceCtx(ctx, def)
	if err != nil {
		return "", err
	}
	if choice < 1 || choice > len(backup.Schedules) {
		return "", fmt.Errorf("invalid choice %d (pick 1–%d)", choice, len(backup.Schedules))
	}
	return backup.Schedules[choice-1], nil
}

func promptKeep(ctx context.Context, current backup.Keep) (backup.Keep, error) {
	fmt.Println("\n  How many backups to keep:")
	k := current
	for _, f := range []struct {
		label string
		v     *int
	}{{"Hourly", &k.Hourly}, {"Daily", &k.Daily}, {"Weekly", &k.Weekly}, {"Monthly", &k.Monthly}, {"Yearly", &k.Yearly}} {
		for {
			s, err := promptWithDefaultCtx(ctx, f.label, strconv.Itoa(*f.v))
			if err != nil {
				return k, err
			}
			n, err := strconv.Atoi(s)
			if err == nil && n >= 0 {
				*f.v = n
				break
			}
			fmt.Println(styles.Warning.Render("  enter a whole number, 0 or more"))
		}
	}
	return k, nil
}

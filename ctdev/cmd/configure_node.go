package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ConnerTechnology/ctdev/ctdev/nodenv"
	"github.com/ConnerTechnology/ctdev/ctdev/sysutil"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/spf13/cobra"
)

var (
	flagNodeGlobal  bool
	flagNodeProject bool
)

var configureNodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Install, update, and uninstall Node versions through nodenv",
	Long: "Choose which Node runs, for every folder (global) or for this project (.node-version). " +
		"ctdev never changes your Node versions on its own: ctdev update leaves them alone.",
}

var configureNodeInstallCmd = &cobra.Command{
	Use:   "install [version]",
	Short: "Install a Node version and use it globally or for this project",
	Long: "Install a Node version and make it the global version or this project's. A bare major (22) " +
		"means its latest release. Making a new version global offers to copy the old one's global npm packages.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return cancelToClean(runNodeInstall(cmdContext(cmd), firstArg(args), isBatchMode()))
	},
}

var configureNodeUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the Node version in use to the latest release of its major",
	Long: "Update this project's Node version when a .node-version governs this folder, else the global one. " +
		"Global npm packages move to the new version at the same versions, and the old version is deleted.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cancelToClean(runNodeUpdate(cmdContext(cmd), isBatchMode()))
	},
}

var configureNodeUninstallCmd = &cobra.Command{
	Use:   "uninstall [version]",
	Short: "Delete an installed Node version and its global npm packages",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return cancelToClean(runNodeUninstall(cmdContext(cmd), firstArg(args), isBatchMode()))
	},
}

func init() {
	for _, c := range []*cobra.Command{configureNodeInstallCmd, configureNodeUpdateCmd} {
		c.Flags().BoolVar(&flagNodeGlobal, "global", false, "use it for every folder without its own version")
		c.Flags().BoolVar(&flagNodeProject, "project", false, "use it for this project (writes .node-version)")
		c.MarkFlagsMutuallyExclusive("global", "project")
	}
	configureNodeCmd.AddCommand(configureNodeInstallCmd, configureNodeUpdateCmd, configureNodeUninstallCmd)
	configureCmd.AddCommand(configureNodeCmd)
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

func nodeOpts() sysutil.Opts {
	return sysutil.Opts{Stdout: os.Stdout, DryRun: flagDryRun}
}

// nodeScope is where a Node version applies.
type nodeScope int

const (
	scopeGlobal nodeScope = iota
	scopeProject
)

// scopeFlag is the scope --global or --project asked for.
func scopeFlag() (nodeScope, bool) {
	switch {
	case flagNodeGlobal:
		return scopeGlobal, true
	case flagNodeProject:
		return scopeProject, true
	}
	return scopeGlobal, false
}

// gitToplevel is the git repository dir is in, "" when it is in none.
func gitToplevel(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// tildePath shortens a path under $HOME for display.
func tildePath(p string) string {
	home, _ := os.UserHomeDir()
	if home != "" && (p == home || strings.HasPrefix(p, home+string(filepath.Separator))) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}

func requireNodenv() error {
	if !nodenv.Available() {
		return errors.New("nodenv isn't installed. Install it with: ctdev install nodenv")
	}
	return nil
}

// configureNodeAfterInstall is the step `ctdev install nodenv` chains into.
func configureNodeAfterInstall(ctx context.Context) error {
	if len(nodenv.Installed()) > 0 {
		if g := nodenv.Global(); g != "" {
			fmt.Printf("  Global Node: %s\n", g)
		}
		fmt.Println(styles.Dimmed.Render("  Manage Node versions with: ctdev configure node install | update | uninstall"))
		return nil
	}
	yes, err := promptYesNoCtx(ctx, "No Node version installed yet. Install one now?", true)
	if err != nil || !yes {
		return err
	}
	return runNodeInstall(ctx, "", false)
}

func runNodeInstall(ctx context.Context, arg string, batch bool) error {
	if ok, err := ensureTool(ctx, "nodenv", "nodenv", batch); !ok || err != nil {
		return err
	}
	if batch && arg == "" {
		return errors.New("--batch needs a version, e.g. ctdev configure node install 24 --global --batch")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	toplevel := gitToplevel(cwd)
	projectDir := toplevel
	if projectDir == "" {
		projectDir = cwd
	}

	scope, set := scopeFlag()
	if !set {
		def := scopeGlobal
		if toplevel != "" {
			def = scopeProject
		}
		scope = def
		if !batch {
			if scope, err = promptNodeScope(ctx, projectDir, def); err != nil {
				return err
			}
		}
	}

	list, err := nodenv.List(ctx)
	if err != nil {
		return err
	}
	v, err := chooseNodeVersion(ctx, list, arg, cwd)
	if err != nil {
		return err
	}

	o := nodeOpts()
	if !nodenv.IsInstalled(v) {
		fmt.Printf("\n  Installing Node %s...\n", v)
	}
	if err := nodenv.Install(ctx, o, v); err != nil {
		return err
	}
	if scope == scopeProject {
		pin := filepath.Join(projectDir, nodenv.VersionFile)
		if err := nodenv.WritePin(o, pin, v); err != nil {
			return err
		}
		fmt.Printf("  %s now uses Node %s (%s).\n", tildePath(projectDir), v, tildePath(pin))
		return nil
	}

	old := nodenv.Global()
	if err := nodenv.SetGlobal(ctx, o, v); err != nil {
		return err
	}
	fmt.Printf("  Global Node is now %s.\n", v)
	if old == "" || old == v {
		return nil
	}
	pkgs := missingPackages(nodenv.GlobalPackages(old), v)
	if len(pkgs) == 0 {
		return nil
	}
	fmt.Printf("\n  %s has %s: %s\n", old, npmPackages(len(pkgs)), nodenv.Specs(pkgs))
	copyThem := true
	if !batch {
		if copyThem, err = promptYesNoCtx(ctx, fmt.Sprintf("Copy them to %s?", v), true); err != nil {
			return err
		}
	}
	if !copyThem {
		fmt.Println(styles.Dimmed.Render(fmt.Sprintf("  Kept them on %s only. %s is still installed.", old, old)))
		return nil
	}
	if err := nodenv.InstallPackages(ctx, o, v, pkgs); err != nil {
		return err
	}
	if err := nodenv.Rehash(ctx, o); err != nil {
		return err
	}
	fmt.Printf("  Copied them. %s is still installed: ctdev configure node uninstall %s removes it.\n", old, old)
	return nil
}

// missingPackages drops the packages version v already has.
func missingPackages(ps []nodenv.Package, v string) []nodenv.Package {
	return slices.DeleteFunc(ps, func(p nodenv.Package) bool { return nodenv.HasPackage(v, p.Name) })
}

func promptNodeScope(ctx context.Context, projectDir string, def nodeScope) (nodeScope, error) {
	fmt.Println("\n  Use it for:")
	fmt.Println("    1) Global (every folder without its own version)")
	fmt.Printf("    2) This project (%s, writes %s)\n", tildePath(projectDir), nodenv.VersionFile)
	fmt.Printf("  %s ", styles.Dimmed.Render(fmt.Sprintf("Choice [%d]:", def+1)))
	choice, err := promptChoiceCtx(ctx, int(def)+1)
	if err != nil {
		return 0, err
	}
	switch choice {
	case 1:
		return scopeGlobal, nil
	case 2:
		return scopeProject, nil
	}
	return 0, fmt.Errorf("invalid choice %d", choice)
}

// chooseNodeVersion resolves arg against node-build's list, or asks for one of
// its latest releases when arg is empty.
func chooseNodeVersion(ctx context.Context, list []string, arg, cwd string) (string, error) {
	if arg != "" {
		if v, ok := nodenv.Resolve(list, arg); ok {
			return v, nil
		}
		if !strings.Contains(arg, ".") {
			return "", fmt.Errorf("node-build has no Node %s. Supported majors: %s", arg, majors(list))
		}
		// A full version node-build no longer lists by default may still
		// have a definition; nodenv install says if it doesn't.
		return strings.TrimPrefix(arg, "v"), nil
	}
	latest := nodenv.LatestPerMajor(list)
	if len(latest) == 0 {
		return "", errors.New("node-build lists no Node versions; update it with ctdev update")
	}
	global := nodenv.Global()
	_, pinned := nodenv.ProjectPin(cwd)
	fmt.Println("\n  Which Node version? Latest release of each supported major:")
	for i, v := range latest {
		var notes []string
		switch {
		case v == global:
			notes = append(notes, "current global")
		case nodenv.IsInstalled(v):
			notes = append(notes, "installed")
		}
		if v == pinned {
			notes = append(notes, "this project")
		}
		fmt.Printf("    %d) %-10s %s\n", i+1, v, styles.Dimmed.Render(strings.Join(notes, ", ")))
	}
	fmt.Printf("  %s ", styles.Dimmed.Render("Choice [1]:"))
	choice, err := promptChoiceCtx(ctx, 1)
	if err != nil {
		return "", err
	}
	if choice < 1 || choice > len(latest) {
		return "", fmt.Errorf("invalid choice %d", choice)
	}
	return latest[choice-1], nil
}

func npmPackages(n int) string {
	if n == 1 {
		return "1 global npm package"
	}
	return fmt.Sprintf("%d global npm packages", n)
}

func majors(list []string) string {
	var ms []string
	for _, v := range nodenv.LatestPerMajor(list) {
		ms = append(ms, fmt.Sprint(nodenv.Major(v)))
	}
	return strings.Join(ms, ", ")
}

// nodeUpdatePlan is what `configure node update` does, worked out before it
// asks.
type nodeUpdatePlan struct {
	scope    nodeScope
	pinFile  string // the .node-version a project update rewrites
	from, to string
	retired  bool // from's major is gone from node-build's list
	packages []nodenv.Package
	keep     string // why from stays installed; "" deletes it
}

// planNodeUpdate moves from's global packages to to and deletes from, unless
// something here still runs it: the global version, for a project update, or
// this folder's .node-version, for a global one. Other folders aren't
// scanned.
func planNodeUpdate(scope nodeScope, pinFile, from, to, global, pinned string) nodeUpdatePlan {
	p := nodeUpdatePlan{scope: scope, pinFile: pinFile, from: from, to: to}
	if !nodenv.IsInstalled(from) {
		return p
	}
	p.packages = missingPackages(nodenv.GlobalPackages(from), to)
	switch {
	case scope == scopeProject && from == global:
		p.keep = "it is still the global version"
	case scope == scopeGlobal && from == pinned:
		p.keep = fmt.Sprintf("the %s here pins it", nodenv.VersionFile)
	}
	return p
}

func (p nodeUpdatePlan) String() string {
	var b strings.Builder
	if p.scope == scopeProject {
		fmt.Fprintf(&b, "  Updating: This project (%s)\n", tildePath(p.pinFile))
	} else if p.pinFile == "" {
		fmt.Fprintf(&b, "  Updating: Global (no %s here)\n", nodenv.VersionFile)
	} else {
		b.WriteString("  Updating: Global\n")
	}
	which := fmt.Sprintf("latest v%d", nodenv.Major(p.to))
	if p.retired {
		which = fmt.Sprintf("last v%d release", nodenv.Major(p.to))
	}
	fmt.Fprintf(&b, "  Node %s → %s (%s)\n\n  This will:\n", p.from, p.to, which)
	steps := []string{"Install " + p.to}
	if n := len(p.packages); n > 0 {
		steps = append(steps, fmt.Sprintf("Reinstall %s's %s under %s, at the same versions:\n         %s",
			p.from, npmPackages(n), p.to, nodenv.Specs(p.packages)))
	}
	if p.scope == scopeProject {
		steps = append(steps, fmt.Sprintf("Write %s to %s", p.to, tildePath(p.pinFile)))
	} else {
		steps = append(steps, fmt.Sprintf("Make %s the global version", p.to))
	}
	switch {
	case p.keep != "":
		steps = append(steps, fmt.Sprintf("Keep %s: %s", p.from, p.keep))
	case nodenv.IsInstalled(p.from):
		steps = append(steps, fmt.Sprintf("Delete %s (nodenv uninstall)", p.from))
	}
	for i, s := range steps {
		fmt.Fprintf(&b, "    %d. %s\n", i+1, s)
	}
	return b.String()
}

func (p nodeUpdatePlan) apply(ctx context.Context, o sysutil.Opts) error {
	if err := nodenv.Install(ctx, o, p.to); err != nil {
		return err
	}
	if err := nodenv.InstallPackages(ctx, o, p.to, p.packages); err != nil {
		return err
	}
	if p.scope == scopeProject {
		if err := nodenv.WritePin(o, p.pinFile, p.to); err != nil {
			return err
		}
	} else if err := nodenv.SetGlobal(ctx, o, p.to); err != nil {
		return err
	}
	if err := nodenv.Rehash(ctx, o); err != nil {
		return err
	}
	if p.keep == "" && nodenv.IsInstalled(p.from) {
		return nodenv.Uninstall(ctx, o, p.from)
	}
	return nil
}

func runNodeUpdate(ctx context.Context, batch bool) error {
	if err := requireNodenv(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	pinFile, pinned := nodenv.ProjectPin(cwd)
	global := nodenv.Global()
	scope, set := scopeFlag()
	if !set && pinFile != "" {
		scope = scopeProject
	}

	from, label := global, "Global"
	if scope == scopeProject {
		if pinFile == "" {
			return fmt.Errorf("no %s governs this folder. Pin one with: ctdev configure node install --project", nodenv.VersionFile)
		}
		from, label = pinned, "Project"
	}
	if from == "" {
		return errors.New("no global Node version is set. Pick one with: ctdev configure node install")
	}

	list, err := nodenv.List(ctx)
	if err != nil {
		return err
	}
	major := nodenv.Major(from)
	to := nodenv.LatestOfMajor(list, major)
	retired := to == ""
	if retired {
		all, err := nodenv.ListAll(ctx)
		if err != nil {
			return err
		}
		if to = nodenv.LatestOfMajor(all, major); to == "" {
			return fmt.Errorf("node-build has no Node %d at all", major)
		}
		fmt.Printf("  v%d is no longer supported (not in node-build's list). Updating to its last release, %s. "+
			"To move to a supported major, run `ctdev configure node install`.\n", major, to)
	}
	if nodenv.Compare(to, from) <= 0 {
		fmt.Printf("  %s Node %s is the latest v%d. Nothing to update.\n", label, from, major)
		return nil
	}

	plan := planNodeUpdate(scope, pinFile, from, to, global, pinned)
	plan.retired = retired
	fmt.Print("\n" + plan.String())
	if !batch {
		yes, err := promptYesNoCtx(ctx, "Continue?", true)
		if err != nil || !yes {
			return err
		}
	}
	if err := plan.apply(ctx, nodeOpts()); err != nil {
		return err
	}
	fmt.Printf("  %s Node is now %s.\n", label, to)
	return nil
}

// uninstallRefusal says why version v must stay, "" when it can go.
func uninstallRefusal(v, global string) string {
	if v == global {
		return fmt.Sprintf("%s is the global version. Pick another global first: ctdev configure node install", v)
	}
	return ""
}

func runNodeUninstall(ctx context.Context, arg string, batch bool) error {
	if err := requireNodenv(); err != nil {
		return err
	}
	installed := nodenv.Installed()
	slices.Reverse(installed)
	if len(installed) == 0 {
		fmt.Println("  No Node versions are installed.")
		return nil
	}
	if batch && arg == "" {
		return errors.New("--batch needs the version to delete, e.g. ctdev configure node uninstall 24.14.0 --batch")
	}
	global := nodenv.Global()
	v := strings.TrimPrefix(arg, "v")
	if v == "" {
		fmt.Println("\n  Installed Node versions:")
		for i, iv := range installed {
			note := ""
			if iv == global {
				note = "global"
			} else if n := len(nodenv.GlobalPackages(iv)); n > 0 {
				note = npmPackages(n)
			}
			fmt.Printf("    %d) %-10s %s\n", i+1, iv, styles.Dimmed.Render(note))
		}
		fmt.Printf("  %s ", styles.Dimmed.Render("Choice:"))
		choice, err := promptChoiceCtx(ctx, 0)
		if err != nil {
			return err
		}
		if choice < 1 || choice > len(installed) {
			return fmt.Errorf("invalid choice %d", choice)
		}
		v = installed[choice-1]
	} else if !slices.Contains(installed, v) {
		return fmt.Errorf("Node %s isn't installed (installed: %s)", v, strings.Join(installed, ", "))
	}
	if why := uninstallRefusal(v, global); why != "" {
		return errors.New(why)
	}

	if pkgs := nodenv.GlobalPackages(v); len(pkgs) > 0 {
		fmt.Printf("  Its %s go with it: %s\n", npmPackages(len(pkgs)), nodenv.Specs(pkgs))
	}
	cwd, _ := os.Getwd()
	if pinFile, pinned := nodenv.ProjectPin(cwd); pinned == v {
		fmt.Println(styles.Warning.Render(fmt.Sprintf("  %s pins %s: node in this folder fails until you pick another version.", tildePath(pinFile), v)))
	}
	if !batch {
		yes, err := promptYesNoCtx(ctx, fmt.Sprintf("Delete Node %s?", v), false)
		if err != nil || !yes {
			return err
		}
	}
	if err := nodenv.Uninstall(ctx, nodeOpts(), v); err != nil {
		return err
	}
	fmt.Printf("  Deleted Node %s.\n", v)
	return nil
}

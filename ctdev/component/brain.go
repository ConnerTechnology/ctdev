package component

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ConnerTechnology/dotfiles/ctdev/platform"
	"github.com/ConnerTechnology/dotfiles/ctdev/sysutil"
)

// brain provisions ConnerTechnology/AI — the agent org: knowledge, agents,
// skills, commands, and accumulated memory — onto an always-on node, and runs
// its scheduled work there. It is not a compose stack: the repo is a git
// checkout, and the schedule is two systemd timers.
//
// WHERE THINGS ARE. These paths are an API surface, not an implementation
// detail: a later service (the tailnet chat app in the AI repo's docs/vision.md)
// has to find the brain without reverse-engineering a systemd unit. They are
// deliberately not home-relative, so they do not move when the operator changes.
//
//	/srv/brain                       the checkout. FHS: "data served by this
//	                                 system". Owned brain:brain, mode 2770 —
//	                                 setgid, and NOT world-readable, because it
//	                                 holds mortgage, insurance and equity detail
//	/var/lib/brain                   service state. Run digests in ./runs, the
//	                                 lock in ./brain.lock, the deploy key in
//	                                 ./.ssh, the brain user's $HOME. This is
//	                                 where a durable work queue would land
//	/etc/ctdev/brain.conf            the pointer file. 0644 on purpose: it holds
//	                                 no secret, and anything that needs to find
//	                                 the brain reads it instead of guessing
//	/etc/ctdev/brain-claude-token.cred  the Claude subscription token, encrypted
//	                                 to this host's key (see BrainStoreCredential)
//	/etc/ctdev/brain-triage.prompt   the default scheduled prompt, overridden by
//	                                 <repo>/scheduled/triage.md when that exists
//	/usr/local/bin/brain-run         the one entry point both timers call
//
// THE SERVICE ACCOUNT is `brain`, a system user that is neither Thomas nor
// Le'Anna. Both are principals of this system with equal standing, so the
// automation deliberately runs as neither of them, and its commits are
// attributable to the node rather than to a person. A second service account —
// the app, when it exists — joins group `brain` and can read the checkout
// without being the timer's user and without a chown migration.
//
// WHAT IS DELIBERATELY NOT HERE. No MCP server serving knowledge/ or memory/:
// that is Phase 2 and it belongs in the AI repo. Nothing here assumes a local
// checkout is the only way that data will ever be reached — brain-run touches
// the repo through git and the conf file, and a server reading the same
// directory would not have to change any of it.
const (
	// BrainRepoDir is the checkout. Stable, documented, and not under any
	// person's home directory — see the note above.
	BrainRepoDir = "/srv/brain"
	// BrainStateDir is service-private state and the brain user's home.
	BrainStateDir = "/var/lib/brain"
	// BrainConfPath is the pointer file every brain job and every future
	// service reads. Shell-quoted KEY='value' lines, no secrets.
	BrainConfPath = "/etc/ctdev/brain.conf"
	// BrainCredPath holds the Claude subscription token encrypted to this
	// host's key. Useless on any other machine.
	BrainCredPath = "/etc/ctdev/brain-claude-token.cred"
	// BrainPromptPath is the fallback scheduled prompt, used when the checkout
	// does not carry scheduled/triage.md of its own.
	BrainPromptPath = "/etc/ctdev/brain-triage.prompt"
	// BrainRunner is the single entry point for both timers. Also the
	// component's DetectPath.
	BrainRunner = "/usr/local/bin/brain-run"

	// BrainUser and BrainGroup name the service account. Not a human.
	BrainUser  = "brain"
	BrainGroup = "brain"

	// BrainDefaultRemote is the agent org. Overridable in brain.conf so a
	// second node, or a fork, needs no code change.
	BrainDefaultRemote = "git@github.com:ConnerTechnology/AI.git"
	BrainDefaultBranch = "main"

	// brainCredentialID must match LoadCredentialEncrypted= in the unit.
	brainCredentialID = "claude-oauth-token"
)

// brainUnits are deployed and removed together.
var brainUnits = []string{
	"brain-triage.service",
	"brain-triage.timer",
	"brain-sync.service",
	"brain-sync.timer",
}

// brainTimers are the units enable/disable act on.
var brainTimers = []string{"brain-triage.timer", "brain-sync.timer"}

// BrainDeployed reports whether the runner and units are in place.
func BrainDeployed() bool {
	_, err := os.Stat(BrainRunner)
	return err == nil
}

// brainPathExists reports whether a path exists, probing as root when a plain
// stat fails.
//
// Everything the service account owns — /srv/brain at 2770, /var/lib/brain at
// 0750 — is deliberately not traversable by the operator's own account, which
// is the point: the checkout holds the household's financial detail and only
// group `brain` may read it. So a bare os.Stat from `ctdev` running as a human
// reports "missing" for files that are plainly there, and every check built on
// it silently does the wrong thing. The root probe never prompts; on a machine
// where it cannot elevate quietly the answer degrades to "missing", which is
// the safe direction — install re-does work rather than skipping it.
func brainPathExists(path string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	return sysutil.SudoNoPrompt(context.Background(), "test", "-e", path).Run() == nil
}

// BrainReadConf returns the settings in /etc/ctdev/brain.conf. The file is
// world-readable by design, so no sudo is needed — that is the whole point of
// putting the pointer there rather than inside a systemd unit.
func BrainReadConf() map[string]string {
	b, err := os.ReadFile(BrainConfPath)
	if err != nil {
		return map[string]string{}
	}
	return brainParseConf(string(b))
}

// brainParseConf reads shell-quoted KEY='value' lines. parseEnv is not reused:
// it leaves the quotes on, and these values (a git author name, an op:// URI)
// legitimately contain spaces and parentheses.
func brainParseConf(s string) map[string]string {
	m := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		m[strings.TrimSpace(k)] = brainUnquote(strings.TrimSpace(v))
	}
	return m
}

// brainUnquote reverses brainQuote for single-quoted shell values.
func brainUnquote(v string) string {
	if len(v) >= 2 && strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") {
		return strings.ReplaceAll(v[1:len(v)-1], `'\''`, `'`)
	}
	return v
}

// brainQuote renders a value so `.`-sourcing the file in bash yields it back
// exactly. Single quotes because the values are literal: nothing in brain.conf
// wants expansion, and a git author name with a `$` in it should not become
// empty.
func brainQuote(v string) string {
	return "'" + strings.ReplaceAll(v, `'`, `'\''`) + "'"
}

// BrainDefaults returns the settings a fresh node starts from. Kept separate
// from the writer so `--show` and the tests can report them without a node.
func BrainDefaults(hostname string) map[string]string {
	return map[string]string{
		"BRAIN_REPO":      BrainRepoDir,
		"BRAIN_STATE":     BrainStateDir,
		"BRAIN_USER":      BrainUser,
		"BRAIN_GROUP":     BrainGroup,
		"BRAIN_REMOTE":    BrainDefaultRemote,
		"BRAIN_BRANCH":    BrainDefaultBranch,
		"BRAIN_GIT_NAME":  "brain on " + hostname,
		"BRAIN_GIT_EMAIL": "brain@" + hostname,
	}
}

// BrainWriteConf merges values into /etc/ctdev/brain.conf. A merge rather than
// a rewrite because install seeds the paths and configure writes the schedule
// and the 1Password reference — neither may clobber the other, the same rule
// the mail server's .env follows.
func BrainWriteConf(ctx context.Context, o sysutil.Opts, values map[string]string) error {
	merged := BrainReadConf()
	for k, v := range values {
		if v == "" {
			continue
		}
		merged[k] = v
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# managed by `ctdev configure brain` — no secrets here.\n")
	b.WriteString("# Shell-quoted, and sourced by /usr/local/bin/brain-run. Do NOT point a\n")
	b.WriteString("# systemd EnvironmentFile= at this file: its quoting rules are not bash's.\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, brainQuote(merged[k]))
	}

	if err := sysutil.SudoRun(ctx, o, "install", "-d", "-m", "0755", filepath.Dir(BrainConfPath)); err != nil {
		return err
	}
	if err := sysutil.SudoWriteFile(ctx, o, b.String(), BrainConfPath); err != nil {
		return err
	}
	// World-readable on purpose: this is how a service that is not the timer
	// finds the brain.
	return sysutil.SudoRun(ctx, o, "chmod", "0644", BrainConfPath)
}

// BrainRepo returns the configured checkout path, falling back to the default.
func BrainRepo() string {
	if v := BrainReadConf()["BRAIN_REPO"]; v != "" {
		return v
	}
	return BrainRepoDir
}

// BrainCheckoutPresent reports whether the repo has actually been cloned.
func BrainCheckoutPresent() bool {
	return brainPathExists(filepath.Join(BrainRepo(), ".git"))
}

// BrainCredentialStored reports whether the encrypted Claude token is in place.
// The file is root-only, but its existence is not a secret, so probe it as root
// quietly rather than failing the check on a machine without a cached password.
func BrainCredentialStored() bool {
	return brainPathExists(BrainCredPath)
}

// BrainConfigured reports whether the timers have everything they need: a
// checkout to work in and a credential to authenticate with. Enabling them
// before both are true just means two failed units a day.
func BrainConfigured() bool {
	return BrainCheckoutPresent() && BrainCredentialStored()
}

// BrainStoreCredential encrypts a Claude Code subscription token to this host
// and writes it where the triage unit's LoadCredentialEncrypted= will find it.
//
// This is the answer to headless authentication, and it is deliberate. The
// token comes from `claude setup-token` run once on a machine with a browser,
// and 1Password is where it is kept. What it is NOT is `op run` at 07:03:
// unattended `op` needs OP_SERVICE_ACCOUNT_TOKEN, which is itself a long-lived
// secret that would have to sit on this node in plaintext to bootstrap the
// thing meant to stop secrets sitting on this node in plaintext — while adding
// a network round-trip to a box that serves its own DNS. So the vault stays the
// system of record (BRAIN_TOKEN_REF records the op:// URI, which is not
// secret), and what lands here is a blob encrypted to /var/lib/systemd/
// credential.secret. It is inert on any other machine, inert in a restic
// snapshot of /etc, and systemd decrypts it into a private ramfs readable only
// by the unit.
func BrainStoreCredential(ctx context.Context, o sysutil.Opts, token string) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] systemd-creds encrypt → %s\n", BrainCredPath)
		return nil
	}
	if !sysutil.CommandExists("systemd-creds") {
		return fmt.Errorf("systemd-creds not found — needs systemd 250 or newer")
	}
	if err := sysutil.SudoRun(ctx, o, "install", "-d", "-m", "0755", filepath.Dir(BrainCredPath)); err != nil {
		return err
	}

	argv := []string{
		"systemd-creds", "encrypt", "--with-key=host",
		"--name=" + brainCredentialID, "-", BrainCredPath,
	}
	if !sysutil.IsRoot() {
		argv = append([]string{"sudo"}, argv...)
	}
	// The token goes in on stdin, never in argv — /proc/<pid>/cmdline is world
	// readable.
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(token)
	cmd.Stdout = o.Stdout
	cmd.Stderr = o.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("systemd-creds encrypt: %w", err)
	}
	return sysutil.SudoRun(ctx, o, "chmod", "0600", BrainCredPath)
}

// BrainDeployKeyPath is the SSH key the node pushes with.
func BrainDeployKeyPath() string {
	return filepath.Join(BrainStateDir, ".ssh", "id_ed25519")
}

// BrainDeployPublicKey returns the public half of the node's deploy key, or ""
// when it hasn't been generated yet.
//
// The file itself is world-readable — a public key is meant to be copied out —
// but it sits under the service account's 0750 home, so an operator running
// ctdev as themselves cannot traverse to it. Fall back to reading it as root
// rather than leaving the one manual step of this install unprintable.
func BrainDeployPublicKey() string {
	path := BrainDeployKeyPath() + ".pub"
	if b, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(b))
	}
	out, err := captureRoot(context.Background(), "cat", path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// BrainEnsureDeployKey generates the node's git identity if it is missing.
//
// A per-repo deploy key rather than a token, because it is the only credential
// in this design with no bootstrapping problem at all: the private half is
// generated here and never leaves, so nothing has to be transported to the
// node, and revoking it is one click in the repo's settings without touching
// any human's account.
func BrainEnsureDeployKey(ctx context.Context, o sysutil.Opts) error {
	sshDir := filepath.Join(BrainStateDir, ".ssh")
	if err := brainInstallDir(ctx, o, sshDir, "0700"); err != nil {
		return err
	}

	// StrictHostKeyChecking=accept-new, not `no`: the first connection pins
	// GitHub's key, and a later change still fails loudly.
	cfg := "Host github.com\n" +
		"  IdentityFile " + BrainDeployKeyPath() + "\n" +
		"  IdentitiesOnly yes\n" +
		"  StrictHostKeyChecking accept-new\n"
	if err := brainWriteAsUser(ctx, o, filepath.Join(sshDir, "config"), cfg, "0600"); err != nil {
		return err
	}

	if brainPathExists(BrainDeployKeyPath()) {
		return nil
	}
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] ssh-keygen → %s\n", BrainDeployKeyPath())
		return nil
	}
	if err := brainShell(ctx, o, fmt.Sprintf(
		"ssh-keygen -t ed25519 -N '' -C %s -f %s",
		brainQuote(BrainUser+"@"+brainHostname()+" (ctdev brain)"),
		brainQuote(BrainDeployKeyPath()))); err != nil {
		return fmt.Errorf("generate deploy key: %w", err)
	}
	// The public half is meant to be read off the node and pasted into GitHub.
	return sysutil.SudoRun(ctx, o, "chmod", "0644", BrainDeployKeyPath()+".pub")
}

// BrainEnableTimers starts the schedule.
func BrainEnableTimers(ctx context.Context, o sysutil.Opts) error {
	for _, t := range brainTimers {
		if err := sysutil.SudoRun(ctx, o, "systemctl", "enable", "--now", t); err != nil {
			return err
		}
	}
	return nil
}

// BrainDisableTimers stops the schedule. The checkout, memory/, the state
// directory and the credential are all untouched.
func BrainDisableTimers(ctx context.Context, o sysutil.Opts) error {
	var firstErr error
	for _, t := range brainTimers {
		if err := sysutil.SudoRun(ctx, o, "systemctl", "disable", "--now", t); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// BrainSyncCheckout clones the repo, or fetches and fast-forwards an existing
// checkout. Runs as the brain user so every object it writes is owned by the
// service account rather than by root.
func BrainSyncCheckout(ctx context.Context, o sysutil.Opts, remote, branch string) error {
	repo := BrainRepo()
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] clone/update %s → %s\n", remote, repo)
		return nil
	}
	if brainPathExists(filepath.Join(repo, ".git")) {
		return brainShell(ctx, o, fmt.Sprintf("git -C %s fetch --quiet origin %s && git -C %s merge --ff-only %s",
			brainQuote(repo), brainQuote(branch), brainQuote(repo), brainQuote("origin/"+branch)))
	}
	return brainShell(ctx, o, fmt.Sprintf("git clone --quiet --branch %s %s %s",
		brainQuote(branch), brainQuote(remote), brainQuote(repo)))
}

// BrainRunSetup runs the checkout's own scripts/setup.sh as the brain user.
//
// The AI repo owns what "set up" means — symlinking agents, skills and commands
// into ~/.claude, merging settings, installing plugins, registering MCP servers.
// ctdev deliberately does not reimplement any of it, so a change there needs no
// release here. If setup.sh breaks on this architecture, the fix belongs in that
// repo, not in a workaround here.
func BrainRunSetup(ctx context.Context, o sysutil.Opts) error {
	repo := BrainRepo()
	script := filepath.Join(repo, "scripts", "setup.sh")
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] run %s as %s\n", script, BrainUser)
		return nil
	}
	if !brainPathExists(script) {
		return fmt.Errorf("%s not found — is %s the right repo?", script, repo)
	}
	return brainShell(ctx, o, fmt.Sprintf("cd %s && ./scripts/setup.sh", brainQuote(repo)))
}

// BrainTrustWorkspace marks the checkout trusted in the service account's
// ~/.claude.json.
//
// Claude Code ignores a project's .claude/settings.json until the workspace has
// been trusted, and trust is granted by an interactive dialog — which a systemd
// timer can never answer. Left unset, the node silently runs with different
// settings from every laptop (the AI repo's settings.json picks the default
// agent and its permission allowlist), and "the Pi behaves differently" is
// exactly the drift this component exists to remove. ctdev cloned this checkout
// from the configured remote, so granting the trust is a statement of a fact it
// already knows rather than a decision made on the operator's behalf.
func BrainTrustWorkspace(ctx context.Context, o sysutil.Opts) error {
	repo := BrainRepo()
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] mark %s trusted for %s\n", repo, BrainUser)
		return nil
	}
	script := "python3 - " + brainQuote(repo) + " <<'PYEOF'\n" + brainTrustPython + "PYEOF\n"
	return brainShell(ctx, o, script)
}

// brainTrustPython edits ~/.claude.json in place. Written as a merge so it never
// discards the rest of a file that also holds onboarding state and OAuth
// account details.
const brainTrustPython = `
import json, os, sys

repo = sys.argv[1]
path = os.path.expanduser("~/.claude.json")
try:
    with open(path) as f:
        data = json.load(f)
except Exception:
    data = {}
project = data.setdefault("projects", {}).setdefault(repo, {})
if project.get("hasTrustDialogAccepted") is True:
    print("  ok       workspace already trusted")
    sys.exit(0)
project["hasTrustDialogAccepted"] = True
with open(path, "w") as f:
    json.dump(data, f, indent=2)
print("  set      workspace trust for " + repo)
`

// BrainInstallClaudeCode installs Claude Code into the brain user's home.
//
// Not a Dependencies entry on the claude-code component: that one installs for
// whoever runs ctdev and deploys ctdev's own ~/.claude/CLAUDE.md, which is the
// exact file the AI repo's setup.sh wants to symlink. Two owners for one path
// is a fight, and keeping the service account's home entirely setup.sh's is how
// it is avoided.
func BrainInstallClaudeCode(ctx context.Context, o sysutil.Opts) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] install Claude Code for %s\n", BrainUser)
		return nil
	}
	if brainUserHasClaude() {
		return nil
	}
	return brainShell(ctx, o, "curl -fsSL https://claude.ai/install.sh | bash")
}

func brainUserHasClaude() bool {
	return brainPathExists(filepath.Join(BrainStateDir, ".local", "bin", "claude"))
}

// BrainClaudeVersion reports the Claude Code version installed for the service
// account, or "" when it isn't installed.
func BrainClaudeVersion(ctx context.Context) string {
	argv := brainAsUserArgv("claude --version")
	out, err := captureRoot(ctx, argv[0], argv[1:]...)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// brainEnv is the environment install-time commands run in. Spelled out rather
// than inherited from a login shell for two reasons: the systemd units do not
// source a profile either, so this keeps install-time and run-time identical;
// and a login shell on Raspberry Pi OS prints an rfkill warning to stdout that
// would otherwise land in the middle of `ctdev configure brain --show`.
func brainEnv() []string {
	return []string{
		"HOME=" + BrainStateDir,
		"USER=" + BrainUser,
		"LOGNAME=" + BrainUser,
		"SHELL=/bin/bash",
		"PATH=" + BrainStateDir + "/.local/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
	}
}

// brainAsUserArgv builds the argv that runs a shell script as the service
// account. `runuser` rather than `sudo -u`: it behaves identically whether ctdev
// was started as root or reached root through sudo, which `sudo -u` does not.
func brainAsUserArgv(script string) []string {
	argv := []string{"runuser", "-u", BrainUser, "--", "env"}
	argv = append(argv, brainEnv()...)
	return append(argv, "bash", "-c", script)
}

// brainShell runs a command as the brain service account.
func brainShell(ctx context.Context, o sysutil.Opts, script string) error {
	argv := brainAsUserArgv(script)
	return sysutil.SudoRun(ctx, o, argv[0], argv[1:]...)
}

// brainInstallDir creates a directory owned by the service account.
func brainInstallDir(ctx context.Context, o sysutil.Opts, path, mode string) error {
	return sysutil.SudoRun(ctx, o, "install", "-d", "-m", mode,
		"-o", BrainUser, "-g", BrainGroup, path)
}

// brainWriteAsUser writes a file and hands it to the service account.
func brainWriteAsUser(ctx context.Context, o sysutil.Opts, path, content, mode string) error {
	if err := sysutil.SudoWriteFile(ctx, o, content, path); err != nil {
		return err
	}
	if err := sysutil.SudoRun(ctx, o, "chown", BrainUser+":"+BrainGroup, path); err != nil {
		return err
	}
	return sysutil.SudoRun(ctx, o, "chmod", mode, path)
}

func brainHostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "localhost"
	}
	return strings.SplitN(h, ".", 2)[0]
}

// brainEnsureAccount creates the service account if it is missing. A system
// user with a real shell: systemd does not need one, but `runuser -l` does, and
// so does an operator debugging a failed run.
func brainEnsureAccount(ctx context.Context, o sysutil.Opts) error {
	if o.DryRun {
		fmt.Fprintf(o.Stdout, "[dry-run] create system user %s (home %s)\n", BrainUser, BrainStateDir)
		return nil
	}
	if brainUserExists() {
		return nil
	}
	if err := sysutil.SudoRun(ctx, o, "groupadd", "--system", "-f", BrainGroup); err != nil {
		return fmt.Errorf("create group %s: %w", BrainGroup, err)
	}
	return sysutil.SudoRun(ctx, o, "useradd", "--system",
		"--gid", BrainGroup,
		"--home-dir", BrainStateDir,
		"--create-home",
		"--shell", "/bin/bash",
		"--comment", "ctdev brain service account",
		BrainUser)
}

func brainUserExists() bool {
	return exec.Command("id", "-u", BrainUser).Run() == nil
}

// brainDeploy writes an embedded file to a root-owned path.
func brainDeploy(ctx context.Context, o sysutil.Opts, src, dest, mode string) error {
	b, err := Configs.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read embedded %s: %w", src, err)
	}
	if err := sysutil.SudoWriteFile(ctx, o, string(b), dest); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	return sysutil.SudoRun(ctx, o, "chmod", mode, dest)
}

func brainInstall(ctx context.Context, opts ExecOpts) error {
	p := platform.Detect()
	o := execOpts(opts)

	if p.OS != platform.Linux {
		return unsupportedPMError("brain", p.PackageManager)
	}

	if o.DryRun {
		return brainDryRun(opts)
	}

	// git is the transport for the whole component; nothing else works without it.
	if !sysutil.CommandExists("git") {
		if err := sysutil.InstallPackage(ctx, o, "git"); err != nil {
			return err
		}
	}

	fmt.Fprintln(opts.Stdout, "Provisioning the brain...")

	if err := brainEnsureAccount(ctx, o); err != nil {
		return err
	}
	// 2770: setgid so anything the timer creates stays group-owned by `brain`
	// and a second service account in that group can read it — and no world
	// bit, because the checkout holds the household's financial detail.
	if err := brainInstallDir(ctx, o, BrainRepoDir, "2770"); err != nil {
		return err
	}
	if err := brainInstallDir(ctx, o, BrainStateDir, "0750"); err != nil {
		return err
	}
	if err := brainInstallDir(ctx, o, filepath.Join(BrainStateDir, "runs"), "0750"); err != nil {
		return err
	}

	// /etc/ctdev holds the pointer file, the default prompt, and the encrypted
	// credential; create it before anything tries to write into it.
	if err := sysutil.SudoRun(ctx, o, "install", "-d", "-m", "0755", filepath.Dir(BrainConfPath)); err != nil {
		return err
	}
	if err := brainDeploy(ctx, o, "configs/brain/brain-run.sh", BrainRunner, "0755"); err != nil {
		return err
	}
	if err := brainDeploy(ctx, o, "configs/brain/triage.prompt", BrainPromptPath, "0644"); err != nil {
		return err
	}
	for _, u := range brainUnits {
		if err := brainDeploy(ctx, o, "configs/brain/"+u, "/etc/systemd/system/"+u, "0644"); err != nil {
			return err
		}
	}
	if err := sysutil.SudoRun(ctx, o, "systemctl", "daemon-reload"); err != nil {
		return err
	}

	// Seed the pointer file. A merge, so re-running install never overwrites a
	// remote or schedule that configure set.
	if err := BrainWriteConf(ctx, o, BrainDefaults(brainHostname())); err != nil {
		return err
	}
	conf := BrainReadConf()

	if err := BrainEnsureDeployKey(ctx, o); err != nil {
		return err
	}
	if err := BrainInstallClaudeCode(ctx, o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not install Claude Code for %s: %v\n", BrainUser, err)
	}

	remote := firstNonEmptyStr(conf["BRAIN_REMOTE"], BrainDefaultRemote)
	branch := firstNonEmptyStr(conf["BRAIN_BRANCH"], BrainDefaultBranch)

	if err := BrainSyncCheckout(ctx, o, remote, branch); err != nil {
		// Not fatal, and not a stack trace: the overwhelmingly likely cause is
		// that the deploy key has not been added to the repo yet, and what the
		// operator needs is the key, not the git error.
		fmt.Fprintf(opts.Stdout, "\nCould not reach %s: %v\n", remote, err)
		brainPrintDeployKey(opts.Stdout, remote)
		// Only a MISSING checkout is a hard stop. An existing one that could
		// not be refreshed still deserves its setup step: a network blip, an
		// expired key, or a GitHub outage must not silently skip provisioning
		// the node and leave the timers un-enabled with no explanation.
		if !BrainCheckoutPresent() {
			return nil
		}
		fmt.Fprintf(opts.Stdout, "\nCarrying on with the checkout already at %s.\n", BrainRepo())
	}

	if err := BrainRunSetup(ctx, o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: %s scripts/setup.sh did not complete: %v\n", BrainRepo(), err)
		fmt.Fprintln(opts.Stdout, "The checkout is in place; fix setup.sh in the AI repo and re-run 'ctdev install brain'.")
	}
	if err := BrainTrustWorkspace(ctx, o); err != nil {
		fmt.Fprintf(opts.Stdout, "warning: could not mark %s trusted: %v\n", BrainRepo(), err)
		fmt.Fprintln(opts.Stdout, "Scheduled runs will ignore the repo's own .claude/settings.json until it is.")
	}

	if BrainConfigured() {
		if err := BrainEnableTimers(ctx, o); err != nil {
			return err
		}
		fmt.Fprintln(opts.Stdout, "\nbrain configured — triage runs 07:03 and 15:07, sync hourly.")
		return nil
	}

	fmt.Fprintln(opts.Stdout, "\nbrain installed. Timers are NOT enabled yet — finish with:")
	fmt.Fprintln(opts.Stdout, "  ctdev configure brain")
	if !BrainCredentialStored() {
		fmt.Fprintln(opts.Stdout, "It asks for a Claude Code token from `claude setup-token` (run that on a")
		fmt.Fprintln(opts.Stdout, "machine with a browser, keep it in 1Password) and encrypts it to this host.")
	}
	return nil
}

// brainPrintDeployKey tells the operator the one manual step this component
// cannot do for them.
func brainPrintDeployKey(w interface{ Write([]byte) (int, error) }, remote string) {
	pub := BrainDeployPublicKey()
	if pub == "" {
		return
	}
	fmt.Fprintln(w, "\nAdd this node's deploy key to the repo, with write access:")
	fmt.Fprintf(w, "  %s\n", brainRepoSettingsURL(remote))
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  %s\n", pub)
	fmt.Fprintln(w, "\nWrite access is required: the node pushes what its runs learn.")
	fmt.Fprintln(w, "Then re-run: ctdev install brain")
}

// brainRepoSettingsURL turns a git remote into the deploy-keys page for it.
func brainRepoSettingsURL(remote string) string {
	s := strings.TrimSuffix(remote, ".git")
	switch {
	case strings.HasPrefix(s, "git@github.com:"):
		s = "https://github.com/" + strings.TrimPrefix(s, "git@github.com:")
	case strings.HasPrefix(s, "https://github.com/"):
		// already a URL
	default:
		return remote
	}
	return s + "/settings/keys"
}

func brainDryRun(opts ExecOpts) error {
	w := opts.Stdout
	fmt.Fprintln(w, "[dry-run] brain — provision ConnerTechnology/AI and its schedule")
	fmt.Fprintf(w, "[dry-run]   create system user %s:%s (home %s)\n", BrainUser, BrainGroup, BrainStateDir)
	fmt.Fprintf(w, "[dry-run]   create %s (2770 %s:%s) and %s (0750)\n", BrainRepoDir, BrainUser, BrainGroup, BrainStateDir)
	fmt.Fprintf(w, "[dry-run]   deploy %s and %s\n", BrainRunner, BrainPromptPath)
	for _, u := range brainUnits {
		fmt.Fprintf(w, "[dry-run]   deploy /etc/systemd/system/%s\n", u)
	}
	fmt.Fprintf(w, "[dry-run]   write %s (0644, no secrets)\n", BrainConfPath)
	fmt.Fprintf(w, "[dry-run]   generate deploy key %s if missing\n", BrainDeployKeyPath())
	fmt.Fprintf(w, "[dry-run]   install Claude Code for %s\n", BrainUser)
	fmt.Fprintf(w, "[dry-run]   clone %s → %s and run its scripts/setup.sh\n", BrainDefaultRemote, BrainRepoDir)
	fmt.Fprintf(w, "[dry-run]   mark %s trusted in %s/.claude.json\n", BrainRepoDir, BrainStateDir)
	fmt.Fprintln(w, "[dry-run]   enable brain-triage.timer (07:03, 15:07) and brain-sync.timer (hourly)")
	fmt.Fprintln(w, "[dry-run]   nothing was changed")
	return nil
}

// brainUninstall stops the schedule and removes what ctdev put on the system.
// It keeps the checkout, memory/, the state directory, the service account and
// the credential — memory/ holds accumulated learning that exists nowhere else,
// and an uninstall is not a request to discard it.
func brainUninstall(ctx context.Context, opts ExecOpts) error {
	o := execOpts(opts)

	_ = BrainDisableTimers(ctx, o)
	for _, u := range brainUnits {
		_ = sysutil.SudoRun(ctx, o, "rm", "-f", "/etc/systemd/system/"+u)
	}
	_ = sysutil.SudoRun(ctx, o, "rm", "-f", BrainRunner)
	_ = sysutil.SudoRun(ctx, o, "systemctl", "daemon-reload")

	fmt.Fprintln(opts.Stdout, "brain timers stopped and units removed.")
	fmt.Fprintf(opts.Stdout, "Kept: %s (the checkout, including memory/), %s, the %s account,\n",
		BrainRepo(), BrainStateDir, BrainUser)
	fmt.Fprintf(opts.Stdout, "      %s, %s, and the timer schedule drop-ins.\n", BrainConfPath, BrainCredPath)
	fmt.Fprintln(opts.Stdout, "Configuration survives an uninstall so a reinstall picks it up; memory/ is")
	fmt.Fprintln(opts.Stdout, "accumulated learning that exists nowhere else — delete it deliberately or not at all.")
	return nil
}

// firstNonEmptyStr returns the first non-empty argument.
func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// Schedule defaults. Stored in brain.conf as well as the unit files so a
// service that is not systemd can read the schedule without parsing a unit.
// Multiple calendar expressions are separated by ';' — a character no systemd
// calendar spec can contain, unlike ','.
const (
	BrainDefaultTriageSchedule = "*-*-* 07:03:00;*-*-* 15:07:00"
	BrainDefaultSyncSchedule   = "*-*-* *:23:00"
)

// BrainTriageSchedule and BrainSyncSchedule report the configured calendars.
func BrainTriageSchedule() string {
	return firstNonEmptyStr(BrainReadConf()["BRAIN_TRIAGE_ONCALENDAR"], BrainDefaultTriageSchedule)
}

func BrainSyncSchedule() string {
	return firstNonEmptyStr(BrainReadConf()["BRAIN_SYNC_ONCALENDAR"], BrainDefaultSyncSchedule)
}

// BrainScheduleDropIn renders a systemd drop-in that replaces a timer's
// calendar. The bare `OnCalendar=` first is required: without it the values
// below would be appended to the ones the shipped unit already carries, and a
// schedule the operator removed would keep firing.
func BrainScheduleDropIn(schedule string) string {
	var b strings.Builder
	b.WriteString("# managed by `ctdev configure brain`\n[Timer]\nOnCalendar=\n")
	for _, spec := range strings.Split(schedule, ";") {
		if spec = strings.TrimSpace(spec); spec != "" {
			fmt.Fprintf(&b, "OnCalendar=%s\n", spec)
		}
	}
	return b.String()
}

// BrainWriteSchedule applies both timers' calendars as drop-ins, leaving the
// shipped units as the recorded defaults.
func BrainWriteSchedule(ctx context.Context, o sysutil.Opts, triage, sync string) error {
	for _, s := range []struct{ unit, schedule string }{
		{"brain-triage.timer", triage},
		{"brain-sync.timer", sync},
	} {
		dir := "/etc/systemd/system/" + s.unit + ".d"
		if err := sysutil.SudoRun(ctx, o, "install", "-d", "-m", "0755", dir); err != nil {
			return err
		}
		if err := sysutil.SudoWriteFile(ctx, o, BrainScheduleDropIn(s.schedule), dir+"/schedule.conf"); err != nil {
			return err
		}
	}
	return sysutil.SudoRun(ctx, o, "systemctl", "daemon-reload")
}

// BrainReadTokenRef resolves a 1Password op:// reference with the local `op`
// CLI. Available when someone runs configure from a machine where 1Password is
// already unlocked; it is never used by the timers — see BrainStoreCredential
// for why the node does not run `op` unattended.
func BrainReadTokenRef(ctx context.Context, ref string) (string, error) {
	if !sysutil.CommandExists("op") {
		return "", fmt.Errorf("the 1Password CLI (op) is not installed here")
	}
	out, err := captureOutput(ctx, "op", "read", ref)
	if err != nil {
		return "", fmt.Errorf("op read %s: %w", ref, err)
	}
	return strings.TrimSpace(out), nil
}

// BrainTimerEnabled reports whether a brain timer is enabled. Used by --show
// and by install to avoid claiming a schedule that isn't running.
func BrainTimerEnabled(ctx context.Context, unit string) bool {
	out, err := captureOutput(ctx, "systemctl", "is-enabled", unit)
	return err == nil && strings.TrimSpace(out) == "enabled"
}

// BrainLastRun returns the newest triage digest's path, or "" when none exist.
// A run's output is a durable artifact under BrainStateDir, deliberately not a
// Claude Code session transcript: session-local state dies with the window, and
// nothing in this component may depend on it.
func BrainLastRun() string {
	dir := filepath.Join(BrainStateDir, "runs")
	names := brainRunNames(dir)
	var newest string
	for _, n := range names {
		// Stamps are ISO-8601, so lexical order is chronological order.
		if strings.HasSuffix(n, "-triage.md") && n > newest {
			newest = n
		}
	}
	if newest == "" {
		return ""
	}
	return filepath.Join(dir, newest)
}

// brainRunNames lists the run directory, as root when the operator's account
// cannot traverse the service account's home.
func brainRunNames(dir string) []string {
	if entries, err := os.ReadDir(dir); err == nil {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			if !e.IsDir() {
				names = append(names, e.Name())
			}
		}
		return names
	}
	out, err := captureRoot(context.Background(), "ls", "-1", dir)
	if err != nil {
		return nil
	}
	return strings.Fields(out)
}

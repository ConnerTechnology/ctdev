package component

import (
	"strings"
	"testing"
)

// readBrainConfig returns an embedded brain config file, failing the test if it
// is missing — these files are the contract between the Go code and systemd, so
// a rename must break loudly rather than silently deploy nothing.
func readBrainConfig(t *testing.T, name string) string {
	t.Helper()
	b, err := Configs.ReadFile("configs/brain/" + name)
	if err != nil {
		t.Fatalf("read embedded configs/brain/%s: %v", name, err)
	}
	return string(b)
}

func TestBrainQuoteRoundTrip(t *testing.T) {
	// brain.conf carries a git author name and an op:// reference, both of
	// which legitimately contain spaces and punctuation. A value that does not
	// survive the round trip is a config file that silently loses settings.
	for _, v := range []string{
		"brain on ctpi01",
		"op://Private/Claude Code (ctpi01)/credential",
		"*-*-* 07:03:00;*-*-* 15:07:00",
		`a value with a ' quote`,
		`$HOME should stay literal`,
		"",
	} {
		if got := brainUnquote(brainQuote(v)); got != v {
			t.Errorf("round trip of %q gave %q", v, got)
		}
	}
}

func TestBrainParseConfKeepsQuotedSpaces(t *testing.T) {
	conf := brainParseConf(strings.Join([]string{
		"# a comment",
		"",
		"BRAIN_REPO='/srv/brain'",
		"BRAIN_GIT_NAME='brain on ctpi01'",
		"BRAIN_TRIAGE_ONCALENDAR='*-*-* 07:03:00;*-*-* 15:07:00'",
	}, "\n"))

	want := map[string]string{
		"BRAIN_REPO":              "/srv/brain",
		"BRAIN_GIT_NAME":          "brain on ctpi01",
		"BRAIN_TRIAGE_ONCALENDAR": "*-*-* 07:03:00;*-*-* 15:07:00",
	}
	for k, v := range want {
		if conf[k] != v {
			t.Errorf("conf[%s] = %q, want %q", k, conf[k], v)
		}
	}
	if len(conf) != len(want) {
		t.Errorf("parsed %d keys, want %d: %v", len(conf), len(want), conf)
	}
}

func TestBrainDefaultsCarryNoSecret(t *testing.T) {
	// brain.conf is 0644 so anything on the box can find the brain. That is
	// only safe while nothing secret is ever seeded into it.
	for k, v := range BrainDefaults("ctpi01") {
		lower := strings.ToLower(k + " " + v)
		for _, bad := range []string{"token", "password", "secret", "key"} {
			if strings.Contains(lower, bad) {
				t.Errorf("default %s=%q looks like a secret; brain.conf is world-readable", k, v)
			}
		}
	}
}

func TestBrainScheduleDropInResetsBeforeSetting(t *testing.T) {
	// Without the bare `OnCalendar=` first, systemd APPENDS to the schedule the
	// shipped unit already carries, so a calendar the operator removed keeps
	// firing. This is the whole reason the drop-in is not just two lines.
	out := BrainScheduleDropIn("*-*-* 07:03:00;*-*-* 15:07:00")
	lines := []string{}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "OnCalendar=") {
			lines = append(lines, l)
		}
	}
	want := []string{"OnCalendar=", "OnCalendar=*-*-* 07:03:00", "OnCalendar=*-*-* 15:07:00"}
	if len(lines) != len(want) {
		t.Fatalf("got %v, want %v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	if !strings.Contains(out, "[Timer]") {
		t.Error("drop-in is missing its [Timer] section")
	}
}

func TestBrainScheduleDropInSkipsBlanks(t *testing.T) {
	out := BrainScheduleDropIn(" *-*-* *:23:00 ;; ")
	if n := strings.Count(out, "OnCalendar="); n != 2 {
		t.Errorf("expected the reset plus one calendar, got %d OnCalendar lines:\n%s", n, out)
	}
	if !strings.Contains(out, "OnCalendar=*-*-* *:23:00\n") {
		t.Errorf("whitespace was not trimmed from the calendar spec:\n%s", out)
	}
}

func TestBrainTriageUnitLoadsTheEncryptedCredential(t *testing.T) {
	// The credential ID in the unit and the --name= passed to systemd-creds
	// must agree, or the unit starts and finds nothing in $CREDENTIALS_DIRECTORY.
	unit := readBrainConfig(t, "brain-triage.service")
	want := "LoadCredentialEncrypted=" + brainCredentialID + ":" + BrainCredPath
	if !strings.Contains(unit, want) {
		t.Errorf("brain-triage.service must contain %q", want)
	}
}

func TestBrainSyncUnitHasNoCredential(t *testing.T) {
	// A sync is git only. Keeping the credential out of it is what makes the
	// checkout keep tracking origin after the subscription token expires — so a
	// laptop's `git pull` still receives what the last triage committed.
	unit := readBrainConfig(t, "brain-sync.service")
	if strings.Contains(unit, "LoadCredential") {
		t.Error("brain-sync.service must not load the Claude credential")
	}
}

func TestBrainUnitsRunAsTheServiceAccount(t *testing.T) {
	// Neither Thomas nor Le'Anna: both are principals of this system with equal
	// standing, so the automation runs as neither of them.
	for _, name := range []string{"brain-triage.service", "brain-sync.service"} {
		unit := readBrainConfig(t, name)
		if !strings.Contains(unit, "User="+BrainUser+"\n") {
			t.Errorf("%s must run as User=%s", name, BrainUser)
		}
		if !strings.Contains(unit, "Group="+BrainGroup+"\n") {
			t.Errorf("%s must run as Group=%s", name, BrainGroup)
		}
		if !strings.Contains(unit, "ExecStart="+BrainRunner+" ") {
			t.Errorf("%s must call %s", name, BrainRunner)
		}
	}
}

func TestBrainTriageTimerKeepsTheOffMinutes(t *testing.T) {
	// 07:03 and 15:07 are deliberate: every other timer on the node fires on
	// the hour or half hour. Randomizing them would put the triage back in the
	// queue behind apt and restic.
	timer := readBrainConfig(t, "brain-triage.timer")
	for _, spec := range []string{"OnCalendar=*-*-* 07:03:00", "OnCalendar=*-*-* 15:07:00"} {
		if !strings.Contains(timer, spec) {
			t.Errorf("brain-triage.timer must contain %q", spec)
		}
	}
	if !strings.Contains(timer, "RandomizedDelaySec=0") {
		t.Error("brain-triage.timer must not randomize its off-minutes")
	}
	if !strings.Contains(timer, "Persistent=true") {
		t.Error("brain-triage.timer must be Persistent so a triage missed while down still runs")
	}
}

func TestBrainRunnerNeverForcesOrAutoResolves(t *testing.T) {
	// The single-writer property for memory/ rests on this: a rejected push or
	// a conflicting rebase must fail the unit, never be forced past. A force
	// here would delete whatever a laptop pushed in between, turning "memory
	// diverged" into "memory was lost".
	script := readBrainConfig(t, "brain-run.sh")
	for _, forbidden := range []string{
		"push --force", "push -f ", "--force-with-lease",
		"rebase --skip", "checkout --theirs", "checkout --ours",
		"reset --hard",
	} {
		if strings.Contains(script, forbidden) {
			t.Errorf("brain-run.sh must not use %q — it would resolve a divergence by discarding one side", forbidden)
		}
	}
	// And it must actually push, or the node accumulates unpushed learning.
	if !strings.Contains(script, "git_c push") {
		t.Error("brain-run.sh must push what a run committed")
	}
}

func TestBrainRunnerPrefersTheCheckoutsOwnPrompt(t *testing.T) {
	// A scheduled prompt with rules baked into it is a snapshot that goes stale
	// silently. The runner reads the prompt at run time, and the AI repo can
	// take ownership of it without a ctdev release.
	script := readBrainConfig(t, "brain-run.sh")
	if !strings.Contains(script, `$REPO/scheduled/triage.md`) {
		t.Error("brain-run.sh must prefer the checkout's own scheduled/triage.md")
	}
	if !strings.Contains(script, BrainPromptPath) {
		t.Errorf("brain-run.sh must fall back to %s", BrainPromptPath)
	}
}

func TestBrainDefaultPromptPointsRatherThanRestates(t *testing.T) {
	// The failure this guards against happened on 2026-08-20: a prompt written
	// at 09:00 was applying superseded filing rules by 15:07. The prompt must
	// name what to read, not carry a copy of it.
	prompt := readBrainConfig(t, "triage.prompt")
	if !strings.Contains(prompt, "triaging-inbox") || !strings.Contains(prompt, "memory/inbox/") {
		t.Error("the triage prompt must point at the skill and the agent's memory")
	}
	// A prompt that has started restating rules is a prompt that has started
	// going stale. Keep it short enough that drift is visible in review.
	if n := len(strings.Split(strings.TrimSpace(prompt), "\n")); n > 20 {
		t.Errorf("triage.prompt is %d lines — it should point at the rules, not restate them", n)
	}
}

func TestBrainDeployedFilesStayOutOfTheData(t *testing.T) {
	// Uninstall removes exactly what install deployed. That is only safe to do
	// unconditionally while nothing install deploys lives inside the checkout
	// or the state directory — memory/ holds learning that exists nowhere else.
	deployed := append([]string{BrainRunner, BrainPromptPath, BrainConfPath}, brainUnits...)
	for _, path := range deployed {
		if strings.HasPrefix(path, BrainRepoDir) || strings.HasPrefix(path, BrainStateDir) {
			t.Errorf("%s is deployed by install but lives under the data directories", path)
		}
	}
}

func TestBrainRepoSettingsURL(t *testing.T) {
	tests := []struct{ remote, want string }{
		{"git@github.com:ConnerTechnology/AI.git", "https://github.com/ConnerTechnology/AI/settings/keys"},
		{"https://github.com/ConnerTechnology/AI.git", "https://github.com/ConnerTechnology/AI/settings/keys"},
		{"git@gitlab.com:someone/thing.git", "git@gitlab.com:someone/thing.git"},
	}
	for _, tt := range tests {
		if got := brainRepoSettingsURL(tt.remote); got != tt.want {
			t.Errorf("brainRepoSettingsURL(%q) = %q, want %q", tt.remote, got, tt.want)
		}
	}
}

func TestBrainRegistryEntry(t *testing.T) {
	c := FindByName("brain")
	if c == nil {
		t.Fatal("brain missing from the registry")
	}
	if c.Category != CategoryInfra {
		t.Errorf("category = %s, want %s", c.Category, CategoryInfra)
	}
	if len(c.SupportedOS) != 1 || c.SupportedOS[0] != OSLinux {
		t.Errorf("SupportedOS = %v, want [linux] — this is a homelab service", c.SupportedOS)
	}
	// Every run redeploys systemd units and writes /etc, installed or not.
	if c.Root != RootAlways {
		t.Errorf("Root = %v, want RootAlways", c.Root)
	}
	// The runner, not the checkout: a repo cloned by hand must not read as an
	// installed component.
	if c.DetectPath != BrainRunner {
		t.Errorf("DetectPath = %q, want %q", c.DetectPath, BrainRunner)
	}
	var hasAI bool
	for _, tag := range c.Tags {
		if tag == "ai" {
			hasAI = true
		}
	}
	if !hasAI {
		t.Errorf("tags = %v, want one of them to be \"ai\"", c.Tags)
	}
}

func TestBrainRunnerAllowListsMCPRatherThanDenying(t *testing.T) {
	// A deny-list fails open: a server added to the AI repo's mcp/servers.json
	// later would silently be in reach of a session that handles
	// attacker-controlled email. The runner names what the run needs and lets
	// --strict-mcp-config ignore the rest.
	script := readBrainConfig(t, "brain-run.sh")
	if !strings.Contains(script, "--strict-mcp-config") {
		t.Error("brain-run.sh must pass --strict-mcp-config so unnamed MCP servers are ignored")
	}
	if !strings.Contains(script, "--mcp-config") {
		t.Error("brain-run.sh must pass the filtered --mcp-config it builds")
	}
	if strings.Contains(script, "--disallowedTools") {
		t.Error("brain-run.sh must not rely on a deny-list for MCP — it fails open")
	}
}

func TestBrainRunnerDropsTheShellAndTheWeb(t *testing.T) {
	// The session that delegates never needs a shell: brain-run does the git
	// work itself. Verified against a live session — --tools reduces the
	// built-in set to exactly what is named, and leaves MCP tools alone.
	script := readBrainConfig(t, "brain-run.sh")
	if !strings.Contains(script, "--tools") {
		t.Fatal("brain-run.sh must limit the built-in tool set")
	}
	defaults := "BRAIN_CLAUDE_TOOLS:-Task,Read,Glob,Grep,Write,Edit,TodoWrite"
	if !strings.Contains(script, defaults) {
		t.Errorf("the default tool set changed; it must not grow a shell or web access (want %q)", defaults)
	}
	for _, banned := range []string{"Bash", "WebFetch", "WebSearch"} {
		if strings.Contains(script, "BRAIN_CLAUDE_TOOLS:-") &&
			strings.Contains(strings.SplitN(script, "BRAIN_CLAUDE_TOOLS:-", 2)[1][:80], banned) {
			t.Errorf("the default tool set must not include %s — it reads attacker-controlled text", banned)
		}
	}
}

func TestBrainAsUserArgvSpellsOutTheEnvironment(t *testing.T) {
	// The systemd units do not source a profile, so neither should install.
	// Inheriting a login shell also drags Raspberry Pi OS's rfkill warning onto
	// stdout, where it landed in the middle of `configure brain --show`.
	argv := brainAsUserArgv("claude --version")
	joined := strings.Join(argv, " ")
	if strings.Contains(joined, "runuser -l") {
		t.Error("must not use a login shell — its profile writes to stdout")
	}
	for _, want := range []string{"HOME=" + BrainStateDir, "PATH=" + BrainStateDir + "/.local/bin"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv is missing %q: %v", want, argv)
		}
	}
	if argv[len(argv)-2] != "-c" || argv[len(argv)-1] != "claude --version" {
		t.Errorf("script must be the final argument: %v", argv)
	}
}

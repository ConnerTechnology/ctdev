package cmd

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type toolCall struct {
	name  string
	args  []string
	input string
}

func (c toolCall) String() string { return strings.Join(append([]string{c.name}, c.args...), " ") }

// fakeKeyTools gives the test a fresh HOME and records every tool the SSH key
// code runs. Its ssh-keygen writes the key files it was asked for.
type fakeKeyTools struct {
	home     string
	calls    []toolCall
	terminal bytes.Buffer
	ghReady  bool
}

func newFakeKeyTools(t *testing.T) *fakeKeyTools {
	t.Helper()
	f := &fakeKeyTools{home: t.TempDir(), ghReady: true}
	t.Setenv("HOME", f.home)
	stateHome := filepath.Join(f.home, ".local", "state")
	t.Setenv("XDG_STATE_HOME", stateHome)
	// Mark removed SSH settings already retired so runSSHMenu never calls
	// ensureSudo or probes host loginctl/ufw during unit tests.
	if err := os.MkdirAll(filepath.Join(stateHome, "ctdev"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateHome, "ctdev", "ssh-settings-retired"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"SSH_CONNECTION", "SSH_TTY", "WAYLAND_DISPLAY", "DISPLAY"} {
		t.Setenv(v, "")
	}
	savedTerm, savedOut, savedIn, savedHas, savedW, savedOS, savedDry :=
		runTerminalTool, runOutputTool, runInputTool, hasTool, terminalOut, keyGOOS, flagDryRun
	t.Cleanup(func() {
		runTerminalTool, runOutputTool, runInputTool, hasTool, terminalOut, keyGOOS, flagDryRun =
			savedTerm, savedOut, savedIn, savedHas, savedW, savedOS, savedDry
	})
	flagDryRun = false
	keyGOOS = "linux"
	terminalOut = &f.terminal
	hasTool = func(string) bool { return true }
	runTerminalTool = func(_ context.Context, name string, args ...string) error {
		f.calls = append(f.calls, toolCall{name: name, args: args})
		if name == "ssh-keygen" {
			path := args[slices.Index(args, "-f")+1]
			comment := args[slices.Index(args, "-C")+1]
			typ := args[slices.Index(args, "-t")+1]
			if err := os.WriteFile(path, []byte("private"), 0o600); err != nil {
				return err
			}
			return os.WriteFile(path+".pub", []byte("ssh-"+typ+" AAAAexample "+comment+"\n"), 0o644)
		}
		return nil
	}
	runOutputTool = func(_ context.Context, name string, args ...string) (string, error) {
		f.calls = append(f.calls, toolCall{name: name, args: args})
		if name == "gh" && args[0] == "auth" && !f.ghReady {
			return "not logged in", os.ErrNotExist
		}
		return "", nil
	}
	runInputTool = func(_ context.Context, input, name string, args ...string) error {
		f.calls = append(f.calls, toolCall{name: name, args: args, input: input})
		return nil
	}
	return f
}

func (f *fakeKeyTools) sshDir() string { return filepath.Join(f.home, ".ssh") }

// addKey writes a synthetic key pair to ~/.ssh.
func (f *fakeKeyTools) addKey(t *testing.T, name string) {
	t.Helper()
	if err := os.MkdirAll(f.sshDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(f.sshDir(), name)
	if err := os.WriteFile(p, []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p+".pub", []byte("ssh-ed25519 AAAAexample test@example.invalid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeKeyTools) commands() []string {
	var out []string
	for _, c := range f.calls {
		out = append(out, c.String())
	}
	return out
}

func runMenu(t *testing.T, lines ...string) string {
	t.Helper()
	typeLines(t, lines...)
	var err error
	out := captureStdout(t, func() { err = runSSHMenu(context.Background()) })
	if err != nil {
		t.Fatalf("menu: %v\n%s", err, out)
	}
	return out
}

func TestKeyActionsOfferTheClipboardOnlyForOneKey(t *testing.T) {
	if got := keyActionsFor(1); !slices.Contains(got, copyToClipboard) || len(got) != 4 {
		t.Errorf("one key: got %v, want all four actions", got)
	}
	got := keyActionsFor(3)
	want := []keyAction{copyToServer, uploadToGitHub, deleteKeys}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("several keys: got %v, want %v", got, want)
	}
}

func TestKeygenArgsPerType(t *testing.T) {
	want := map[string]string{
		"ed25519":    "-t ed25519 -f /k -C test@example.invalid",
		"ed25519-sk": "-t ed25519-sk -f /k -C test@example.invalid",
		"ecdsa":      "-t ecdsa -f /k -C test@example.invalid",
		"rsa":        "-t rsa -b 4096 -f /k -C test@example.invalid",
	}
	for _, typ := range keyTypes {
		got := strings.Join(newKey{typ: typ, path: "/k", comment: "test@example.invalid"}.keygenArgs(), " ")
		if got != want[typ.name] {
			t.Errorf("%s: got %q, want %q", typ.name, got, want[typ.name])
		}
	}
	if keyTypes[0].name != "ed25519" {
		t.Errorf("default type is %s, want ed25519", keyTypes[0].name)
	}
	if got := keyTypes[1].fileName(); got != "id_ed25519_sk" {
		t.Errorf("ed25519-sk file name: got %s", got)
	}
}

func TestSSHCopyIDArgs(t *testing.T) {
	if got := strings.Join(sshCopyIDArgs("/k.pub", "test@example.invalid", ""), " "); got != "-i /k.pub test@example.invalid" {
		t.Errorf("no port: got %q", got)
	}
	if got := strings.Join(sshCopyIDArgs("/k.pub", "test@example.invalid", "2222"), " "); got != "-i /k.pub -p 2222 test@example.invalid" {
		t.Errorf("port: got %q", got)
	}
}

func TestGitHubKeyArgsPerUse(t *testing.T) {
	argv := func(use githubKeyUse) []string {
		var out []string
		for _, k := range githubKeysFor("ctdev-example", use) {
			out = append(out, strings.Join(k.ghArgs("/k.pub"), " "))
		}
		return out
	}
	auth := "ssh-key add /k.pub --title ctdev-example"
	sign := "ssh-key add /k.pub --title ctdev-example-signing --type signing"
	for use, want := range map[githubKeyUse][]string{
		githubAuthentication: {auth},
		githubSigning:        {sign},
		githubBoth:           {auth, sign},
	} {
		if got := argv(use); !reflect.DeepEqual(got, want) {
			t.Errorf("use %d: got %q, want %q", use, got, want)
		}
	}
}

func TestOSC52EncodesTheText(t *testing.T) {
	seq := osc52("ssh-ed25519 AAAAexample test@example.invalid")
	payload, ok := strings.CutPrefix(seq, "\x1b]52;c;")
	if !ok || !strings.HasSuffix(payload, "\a") {
		t.Fatalf("not an OSC 52 clipboard sequence: %q", seq)
	}
	got, err := base64.StdEncoding.DecodeString(strings.TrimSuffix(payload, "\a"))
	if err != nil || string(got) != "ssh-ed25519 AAAAexample test@example.invalid" {
		t.Errorf("decoded %q, %v", got, err)
	}
}

func TestClipboardForSession(t *testing.T) {
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	cases := []struct {
		name string
		goos string
		env  map[string]string
		has  func(string) bool
		want string
	}{
		{"mac desktop", "darwin", nil, all, "pbcopy"},
		{"mac over ssh", "darwin", map[string]string{"SSH_CONNECTION": "x"}, all, ""},
		{"wayland", "linux", map[string]string{"WAYLAND_DISPLAY": "wayland-0", "DISPLAY": ":0"}, all, "wl-copy"},
		{"x11", "linux", map[string]string{"DISPLAY": ":0"}, all, "xclip"},
		{"linux over ssh, no display", "linux", map[string]string{"SSH_TTY": "/dev/pts/1"}, all, ""},
		{"x11 without xclip", "linux", map[string]string{"DISPLAY": ":0"}, none, ""},
	}
	for _, c := range cases {
		got := clipboardFor(c.goos, func(k string) string { return c.env[k] }, c.has)
		if got.name != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got.name, c.want)
		}
	}
}

func TestCreateKeyDefaultsAndOffersActions(t *testing.T) {
	f := newFakeKeyTools(t)
	// Create a key, ed25519, default name, default comment; then Back, Quit.
	runMenu(t, "2", "", "", "", "5", "4")
	want := filepath.Join(f.sshDir(), "id_ed25519")
	if len(f.calls) != 1 || f.calls[0].name != "ssh-keygen" {
		t.Fatalf("calls: %v", f.commands())
	}
	args := f.calls[0].args
	if got := strings.Join(args[:4], " "); got != "-t ed25519 -f "+want {
		t.Errorf("ssh-keygen args: %q", args)
	}
	if slices.Contains(args, "-N") {
		t.Error("ssh-keygen must ask for the passphrase itself, not get one from ctdev")
	}
	if !strings.Contains(args[len(args)-1], "@") {
		t.Errorf("comment should default to user@hostname, got %q", args[len(args)-1])
	}
}

func TestCreateKeyNeverOverwrites(t *testing.T) {
	f := newFakeKeyTools(t)
	f.addKey(t, "id_rsa")
	f.addKey(t, "id_rsa_github")
	// rsa; id_rsa is taken so it asks the purpose; github gives a taken name,
	// so it re-asks; then a free one.
	runMenu(t, "2", "4", "github", "", "id_rsa", "id_rsa_work", "test@example.invalid", "5", "4")
	if len(f.calls) != 1 {
		t.Fatalf("calls: %v", f.commands())
	}
	want := "-t rsa -b 4096 -f " + filepath.Join(f.sshDir(), "id_rsa_work") + " -C test@example.invalid"
	if got := strings.Join(f.calls[0].args, " "); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if data, _ := os.ReadFile(filepath.Join(f.sshDir(), "id_rsa")); string(data) != "private" {
		t.Error("id_rsa was overwritten")
	}
}

func TestCreateKeyPurposeSuggestion(t *testing.T) {
	f := newFakeKeyTools(t)
	f.addKey(t, "id_ed25519")
	runMenu(t, "2", "1", "Work Laptop", "", "", "5", "4")
	want := filepath.Join(f.sshDir(), "id_ed25519_work_laptop")
	if len(f.calls) != 1 || f.calls[0].args[3] != want {
		t.Fatalf("got %v, want the key at %s", f.commands(), want)
	}
}

func TestCreateKeyAddsToTheKeychainOnMacOS(t *testing.T) {
	f := newFakeKeyTools(t)
	keyGOOS = "darwin"
	runMenu(t, "2", "", "", "", "5", "4")
	want := "ssh-add --apple-use-keychain " + filepath.Join(f.sshDir(), "id_ed25519")
	if cmds := f.commands(); len(cmds) != 2 || cmds[1] != want {
		t.Errorf("got %v, want ssh-keygen then %q", cmds, want)
	}
}

func TestCopySeveralKeysToAServer(t *testing.T) {
	f := newFakeKeyTools(t)
	f.addKey(t, "id_a")
	f.addKey(t, "id_b")
	out := runMenu(t, "1", "1 2", "1", "test@example.invalid", "2222", "4", "4")
	if strings.Contains(out, "clipboard") {
		t.Error("the clipboard must not be offered for several keys")
	}
	want := []string{
		"ssh-copy-id -i " + filepath.Join(f.sshDir(), "id_a.pub") + " -p 2222 test@example.invalid",
		"ssh-copy-id -i " + filepath.Join(f.sshDir(), "id_b.pub") + " -p 2222 test@example.invalid",
	}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUploadKeyToGitHubAsSigning(t *testing.T) {
	f := newFakeKeyTools(t)
	f.addKey(t, "id_a")
	runMenu(t, "1", "1", "2", "2", "5", "4")
	host, _ := os.Hostname()
	want := []string{
		"gh auth status",
		"gh ssh-key add " + filepath.Join(f.sshDir(), "id_a.pub") + " --title ctdev-" + host + "-id_a-signing --type signing",
	}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUploadWithoutGhShowsTheManualSteps(t *testing.T) {
	f := newFakeKeyTools(t)
	f.ghReady = false
	f.addKey(t, "id_a")
	out := runMenu(t, "1", "1", "2", "", "5", "4")
	if got := f.commands(); !reflect.DeepEqual(got, []string{"gh auth status"}) {
		t.Errorf("got %q, want only the gh auth check", got)
	}
	if !strings.Contains(out, "github.com/settings/ssh/new") {
		t.Errorf("no manual steps in:\n%s", out)
	}
}

func TestClipboardOverSSHUsesOSC52(t *testing.T) {
	f := newFakeKeyTools(t)
	t.Setenv("SSH_CONNECTION", "192.0.2.1 50000 192.0.2.2 22")
	f.addKey(t, "id_a")
	runMenu(t, "1", "1", "3", "5", "4")
	if len(f.calls) != 0 {
		t.Errorf("ran %v, want no clipboard tool over SSH", f.commands())
	}
	if f.terminal.String() != osc52("ssh-ed25519 AAAAexample test@example.invalid") {
		t.Errorf("terminal got %q", f.terminal.String())
	}
}

func TestClipboardOnWayland(t *testing.T) {
	f := newFakeKeyTools(t)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	f.addKey(t, "id_a")
	runMenu(t, "1", "1", "3", "5", "4")
	if len(f.calls) != 1 || f.calls[0].name != "wl-copy" || f.calls[0].input != "ssh-ed25519 AAAAexample test@example.invalid" {
		t.Errorf("calls: %+v", f.calls)
	}
}

func TestDeleteNamesBothFilesAndRemovesThem(t *testing.T) {
	f := newFakeKeyTools(t)
	f.addKey(t, "id_a")
	f.addKey(t, "id_b")
	out := runMenu(t, "1", "1", "4", "y", "4")
	if !strings.Contains(out, "~/.ssh/id_a\n") || !strings.Contains(out, "~/.ssh/id_a.pub") {
		t.Errorf("the confirmation should name both files:\n%s", out)
	}
	for _, name := range []string{"id_a", "id_a.pub"} {
		if _, err := os.Stat(filepath.Join(f.sshDir(), name)); !os.IsNotExist(err) {
			t.Errorf("%s still exists", name)
		}
	}
	if _, err := os.Stat(filepath.Join(f.sshDir(), "id_b")); err != nil {
		t.Error("id_b was deleted too")
	}
}

func TestDeleteDefaultsToNo(t *testing.T) {
	f := newFakeKeyTools(t)
	f.addKey(t, "id_a")
	runMenu(t, "1", "1", "4", "", "5", "4")
	if _, err := os.Stat(filepath.Join(f.sshDir(), "id_a")); err != nil {
		t.Error("Enter at the confirmation deleted the key")
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	f := newFakeKeyTools(t)
	f.addKey(t, "id_a")
	flagDryRun = true
	out := runMenu(t,
		"2", "", "", "", // create: prints ssh-keygen, makes nothing
		"1", "1", "1", "test@example.invalid", "", // copy to a server
		"2", "3", // upload to GitHub, both
		"3", // clipboard
		"4", // delete: prints, keeps the files
		"5", "4")
	if len(f.calls) != 0 {
		t.Errorf("dry run ran %v", f.commands())
	}
	if f.terminal.Len() != 0 {
		t.Error("dry run wrote to the clipboard")
	}
	for _, name := range []string{"id_a", "id_a.pub"} {
		if _, err := os.Stat(filepath.Join(f.sshDir(), name)); err != nil {
			t.Errorf("dry run removed %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(f.sshDir(), "id_ed25519")); !os.IsNotExist(err) {
		t.Error("dry run created a key")
	}
	for _, want := range []string{"would run: ssh-keygen -t ed25519", "would run: ssh-copy-id -i", "would run: gh ssh-key add", "--type signing", "would delete"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run output lacks %q:\n%s", want, out)
		}
	}
}

func TestGitSigningKeyUploadAddsBoth(t *testing.T) {
	f := newFakeKeyTools(t)
	typeLines(t, "y")
	captureStdout(t, func() {
		if err := promptGitHubKeyUpload(context.Background(), "/k.pub"); err != nil {
			t.Error(err)
		}
	})
	host, _ := os.Hostname()
	want := []string{
		"gh auth status",
		"gh ssh-key add /k.pub --title ctdev-" + host,
		"gh ssh-key add /k.pub --title ctdev-" + host + "-signing --type signing",
	}
	if got := f.commands(); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestInstallSSHOpensTheMenu(t *testing.T) {
	if !componentHasConfigure("ssh") {
		t.Fatal("ssh should have a configuration step")
	}
	if reflect.ValueOf(componentWizards["ssh"]).Pointer() != reflect.ValueOf(runSSHMenu).Pointer() {
		t.Error("install ssh should open the configure ssh menu")
	}
}

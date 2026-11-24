package backupapp

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// key is the key press a terminal sends for s, as tea.KeyPressMsg.String()
// names it.
func key(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(s)[0], Text: s}
}

func TestKeyNames(t *testing.T) {
	for _, s := range []string{"enter", "esc", "tab", "shift+tab", "up", "down", "left", "right", "backspace", "ctrl+c", "b", "D"} {
		if got := key(s).String(); got != s {
			t.Errorf("key(%q) reads %q", s, got)
		}
	}
}

// press sends keys to s in turn and returns the first answer.
func press(s step, keys ...string) *answer {
	for _, k := range keys {
		if a, _ := s.update(key(k)); a != nil {
			return a
		}
	}
	return nil
}

// screen is the plain text a step shows.
func screen(s step) string {
	v := s.view(96, 0)
	lines := v.content
	for _, n := range v.notes {
		lines = append(lines, n.Text)
	}
	return ansi.Strip(strings.Join(append(lines, v.keys, v.help), "\n"))
}

var exampleBar = Dashboard{Actions: []Action{
	{Key: "c", Label: "Configure"}, {Key: "b", Label: "Back up now (once)"}, {Key: "l", Label: "List backups", Dimmed: true},
	{Key: "D", Label: "Delete all"}, {Key: "q", Label: "Quit"},
}}

func TestActionBar(t *testing.T) {
	for _, c := range []struct {
		keys []string
		want string
	}{
		{[]string{"enter"}, "c"},
		{[]string{"b"}, "b"},
		{[]string{"l", "d", "D"}, "D"}, // a dimmed letter does nothing; Delete all needs a capital
		{[]string{"right", "right", "enter"}, "D"},
		{[]string{"left", "enter"}, "c"},
	} {
		a := press(newDashboard(exampleBar), c.keys...)
		if a == nil || a.value.(Action).Key != c.want {
			t.Errorf("%v → %+v, want %q", c.keys, a, c.want)
		}
	}
	if a := press(newDashboard(exampleBar), "esc"); a == nil || !errors.Is(a.err, ErrBack) {
		t.Errorf("esc → %+v, want ErrBack", a)
	}
	focused := exampleBar
	focused.Focus = "q"
	if a := press(newDashboard(focused), "enter"); a == nil || a.value.(Action).Key != "q" {
		t.Errorf("focus on q, enter → %+v", a)
	}
	if got := screen(newDashboard(exampleBar)); !strings.Contains(got, " c  Configure ") || !strings.Contains(got, "l List backups") {
		t.Errorf("bar:\n%s", got)
	}
}

func TestPicker(t *testing.T) {
	p := Picker{Columns: []Column{{Title: "WHEN"}, {Title: "SIZE", Right: true}}, Cursor: 1, Items: []Item{
		{Cells: []string{"Nov 22, 9:12 PM", "1.2 GB"}}, {Cells: []string{"Nov 21, 9:10 PM", "960 MB"}},
		{Cells: []string{"Nov 20, 9:15 PM", "1.2 GB"}, Dimmed: true}, {Cells: []string{"Nov 19, 9:02 PM", "1.1 GB"}},
	}}
	if a := press(newPicker(p), "enter"); a == nil || a.value != 1 {
		t.Errorf("enter → %+v, want the starting cursor", a)
	}
	if a := press(newPicker(p), "down", "enter"); a == nil || a.value != 3 {
		t.Errorf("down skips the dimmed row: %+v", a)
	}
	if a := press(newPicker(p), "q", "esc"); a == nil || !errors.Is(a.err, ErrBack) {
		t.Errorf("esc → %+v; q does nothing in a list that isn't top-level", a)
	}
	p.Quit = true
	if a := press(newPicker(p), "q"); a == nil || !errors.Is(a.err, ErrBack) {
		t.Errorf("q in a top-level list → %+v", a)
	}
	got := screen(newPicker(p))
	if !strings.Contains(got, "WHEN                SIZE") || !strings.Contains(got, "Nov 21, 9:10 PM   960 MB") {
		t.Errorf("columns don't line up, numbers right-aligned:\n%s", got)
	}
	long := Picker{}
	for range 30 {
		long.Items = append(long.Items, Item{Cells: []string{"example"}})
	}
	if v := newPicker(long).view(96, 24); !strings.Contains(ansi.Strip(strings.Join(v.content, "\n")), "↓ 22 more") {
		t.Errorf("a long list doesn't scroll:\n%s", strings.Join(v.content, "\n"))
	}
}

func TestConfirm(t *testing.T) {
	for name, c := range map[string]struct {
		c    Confirm
		keys []string
		want any // bool, or an error
	}{
		"safe enter":          {Confirm{Yes: "Back up"}, []string{"enter"}, true},
		"safe esc":            {Confirm{Yes: "Back up"}, []string{"esc"}, false},
		"safe ignores others": {Confirm{Yes: "Back up"}, []string{"x", "n"}, false},
		"destructive y":       {Confirm{Yes: "Delete", Destructive: true}, []string{"y"}, true},
		"destructive enter":   {Confirm{Yes: "Delete", Destructive: true}, []string{"enter"}, false},
		"destructive any":     {Confirm{Yes: "Delete", Destructive: true}, []string{"x"}, false},
		"keep enter":          {Confirm{Yes: "Delete them too", Keep: "Keep them", Destructive: true}, []string{"enter"}, false},
		"keep y":              {Confirm{Yes: "Delete them too", Keep: "Keep them", Destructive: true}, []string{"y"}, true},
		"keep esc":            {Confirm{Yes: "Delete them too", Keep: "Keep them", Destructive: true}, []string{"esc"}, ErrBack},
		"type then esc":       {Confirm{Yes: "Delete all", TypeToConfirm: "website"}, []string{"w", "esc"}, false},
	} {
		a := press(newConfirm(c.c), c.keys...)
		switch want := c.want.(type) {
		case error:
			if a == nil || !errors.Is(a.err, want) {
				t.Errorf("%s: %+v, want %v", name, a, want)
			}
		default:
			if a == nil || a.err != nil || a.value != want {
				t.Errorf("%s: %+v, want %v", name, a, want)
			}
		}
	}

	cf := newConfirm(Confirm{Question: "Delete all 4 backups of ~/Projects/website?", Yes: "Delete all", TypeToConfirm: "website"})
	if a := press(cf, "w", "e", "b", "enter"); a != nil {
		t.Fatalf("a wrong name answered %+v", a)
	}
	if got := screen(cf); !strings.Contains(got, "That isn't website; type it exactly, or press esc to cancel.") {
		t.Errorf("wrong name:\n%s", got)
	}
	if a := press(cf, append(strings.Split("website", ""), "enter")...); a == nil || a.value != true {
		t.Errorf("the right name → %+v", a)
	}
}

// A kit for a local destination, whose Secret Key is all zeros.
const exampleKit = `ctdev Emergency Kit
Destination:           example-backups
Address:               /media/example/backups
Secret Key:            CT1-AAAAAA-AAAAA-AAAAA-AAAAA-AAAAA
Master password:       ______________________________
`

func TestForm(t *testing.T) {
	f := Form{Fields: []Field{
		{Key: "name", Label: "NAME", Kind: Text, Value: "example-backups", Required: true},
		{Key: "on", Label: "STORE ON", Kind: Choice, Value: "b2", Options: []Option{{"b2", "Backblaze B2"}, {"local", "Local path or USB drive"}}},
		{Key: "key", Label: "APPLICATION KEY", Kind: Secret, Required: true, Show: func(v Values) bool { return v["on"] == "b2" }},
		{Key: "keep", Label: "DAILY", Kind: Number, Value: "7"},
	}}
	fm := newForm(f)
	if a := press(fm, "enter"); a != nil {
		t.Fatalf("an empty required field submitted: %+v", a)
	}
	if got := screen(fm); !strings.Contains(got, "Required — please enter a value.") || !strings.Contains(got, "▸ APPLICATION KEY") {
		t.Errorf("the empty secret should hold focus with its error:\n%s", got)
	}
	press(fm, "K", "0", "0", "0", "x")
	if got := screen(fm); strings.Contains(got, "K000x") || !strings.Contains(got, "••••••••••") {
		t.Errorf("a secret shows:\n%s", got)
	}
	press(fm, "tab", "backspace", "s", "e", "v", "e", "n")
	if a := press(fm, "enter"); a != nil {
		t.Fatalf("a bad number submitted: %+v", a)
	}
	if got := screen(fm); !strings.Contains(got, "Enter a whole number, 0 or more.") {
		t.Errorf("bad number:\n%s", got)
	}
	press(fm, "ctrl+u")
	fm.values["keep"] = "7"
	a := press(fm, "enter")
	if a == nil || a.value.(Values)["key"] != "K000x" || a.value.(Values)["name"] != "example-backups" {
		t.Fatalf("submit → %+v", a)
	}

	// Choosing Local hides the B2 key, so it's neither required nor returned.
	fm = newForm(f)
	press(fm, "tab", "down")
	if got := screen(fm); strings.Contains(got, "APPLICATION KEY") {
		t.Errorf("the B2 field shows for a local path:\n%s", got)
	}
	if a := press(fm, "enter"); a == nil || a.value.(Values)["on"] != "local" || a.value.(Values)["key"] != "" {
		t.Errorf("submit → %+v", a)
	}
	if a := press(newForm(f), "esc"); a == nil || !errors.Is(a.err, ErrBack) {
		t.Errorf("esc → %+v", a)
	}

	withErr := newForm(Form{Error: "Couldn't create the destination: example failure",
		Fields: []Field{{Key: "master", Label: "MASTER PASSWORD", Kind: Secret, Error: "Use at least 10 characters."}}})
	if got := screen(withErr); !strings.Contains(got, "Couldn't create the destination") || !strings.Contains(got, "Use at least 10 characters.") {
		t.Errorf("errors from a later step:\n%s", got)
	}
}

func TestKitPaste(t *testing.T) {
	f := Form{Fields: []Field{{Key: "kit", Label: "EMERGENCY KIT", Kind: KitPaste, Required: true}}}

	fm := newForm(f)
	if got := screen(fm); !strings.Contains(got, "waiting for a paste…") {
		t.Errorf("before a paste:\n%s", got)
	}
	fm.update(tea.PasteMsg{Content: exampleKit})
	got := screen(fm)
	if !strings.Contains(got, "✓ Pasted an Emergency Kit for example-backups (local path)") || strings.Contains(got, "CT1-") {
		t.Errorf("after a paste:\n%s", got)
	}
	if a := press(fm, "enter"); a == nil || a.value.(Values)["kit"] != exampleKit {
		t.Errorf("submit → %+v", a)
	}

	fm = newForm(f)
	fm.update(tea.PasteMsg{Content: "Destination: example-backups\nAddress: /media/example/backups\n"})
	if got := screen(fm); !strings.Contains(got, "The kit has no Secret Key line: paste the whole kit again.") {
		t.Errorf("a bad paste:\n%s", got)
	}
	if a := press(fm, "enter"); a != nil {
		t.Errorf("a bad paste submitted: %+v", a)
	}

	// Without bracketed paste, the kit arrives as keys: enter ends each
	// line and an empty line ends the kit. Nothing typed is shown.
	fm = newForm(f)
	for _, line := range strings.Split(strings.TrimSpace(exampleKit), "\n") {
		for _, r := range line {
			fm.update(tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		fm.update(key("enter"))
		if got := screen(fm); strings.Contains(got, "example-backups") || strings.Contains(got, "CT1") {
			t.Fatalf("typing echoes:\n%s", got)
		}
	}
	fm.update(key("enter"))
	if got := screen(fm); !strings.Contains(got, "✓ Pasted an Emergency Kit for example-backups") {
		t.Errorf("after typing the kit:\n%s", got)
	}
}

func TestKitView(t *testing.T) {
	k := KitScreen{Kit: exampleKit, RecoveryCode: "XXXXXXXX-XXXXXXXX-XXXXXXXX-XXXXXXXX"}
	kv := newKitView(k)
	got := screen(kv)
	if !strings.Contains(got, "CT1-AAAAAA") || !strings.Contains(got, "XXXXXXXX-XXXXXXXX") || !strings.Contains(got, "enter  I've saved both") {
		t.Errorf("kit:\n%s", got)
	}
	if _, cmd := kv.update(key("c")); cmd == nil || !strings.Contains(screen(kv), "✓ Copied") {
		t.Error("c doesn't copy")
	}
	if a := press(kv, "esc"); a != nil || !strings.Contains(screen(kv), "Leave without saving them?\nctdev shows these once.") {
		t.Errorf("esc should ask first: %+v\n%s", a, screen(kv))
	}
	if a := press(kv, "enter"); a != nil || strings.Contains(screen(kv), "Leave without") {
		t.Errorf("enter on the question stays: %+v", a)
	}
	press(kv, "esc")
	if a := press(kv, "y"); a == nil || !errors.Is(a.err, ErrBack) {
		t.Errorf("y after esc → %+v, want ErrBack", a)
	}
	kv = newKitView(k)
	kv.ctrlC()
	if a := press(kv, "y"); a == nil || !errors.Is(a.err, errQuit) {
		t.Errorf("y after ctrl+c → %+v, want to quit", a)
	}
	if a := press(newKitView(k), "enter"); a == nil || a.err != nil {
		t.Errorf("enter → %+v, want saved", a)
	}
	if v := newKitView(k).view(40, 0); !strings.Contains(ansi.Strip(strings.Join(v.content, "\n")), "Widen the window") {
		t.Errorf("a narrow window shows the kit wrapped:\n%s", strings.Join(v.content, "\n"))
	}
}

func TestRunning(t *testing.T) {
	cancelled := false
	rn := newRunning(Running{Label: "Backing up", Detail: "to example-backups"}, func() { cancelled = true })
	if got := screen(rn); !strings.Contains(got, "Backing up") || !strings.Contains(got, "esc Cancel") {
		t.Errorf("spinner:\n%s", got)
	}
	rn.setProgress(Progress{Percent: 0.62, FilesDone: 2983, FilesTotal: 4812, BytesDone: 744_000_000, BytesTotal: 1_200_000_000, SecondsLeft: 40, CurrentFile: "/src/components/Header.tsx"})
	got := screen(rn)
	for _, want := range []string{"62%", "2,983 of 4,812 files", "744.0 MB of 1.2 GB", "about 40s left", "src/components/Header.tsx", "█", "░"} {
		if !strings.Contains(got, want) {
			t.Errorf("progress lacks %q:\n%s", want, got)
		}
	}
	rn.setProgress(Progress{FilesDone: 12, BytesDone: 4000})
	if got := screen(rn); !strings.Contains(got, "counting…") {
		t.Errorf("while restic counts:\n%s", got)
	}
	press(rn, "esc")
	if !cancelled || !strings.Contains(screen(rn), "Cancelling…") {
		t.Errorf("esc didn't cancel:\n%s", screen(rn))
	}
	if a, _ := rn.ctrlC(); a == nil || !errors.Is(a.err, errQuit) {
		t.Errorf("a second ctrl+c → %+v, want to quit", a)
	}

	keys := newRunning(Running{Label: "Removing example-desktop's key…", ChangesKeysOn: "example-backups"}, func() { t.Error("cancelled a step that changes keys") })
	press(keys, "esc")
	keys.ctrlC()
	if got := screen(keys); !strings.Contains(got, "Can't be cancelled: this step changes the keys on example-backups.") {
		t.Errorf("uncancellable:\n%s", got)
	}
}

// Run returns ErrBack when the user cancelled fn, and fn's error otherwise.
func TestRanTo(t *testing.T) {
	ctx, quit := context.WithCancel(context.Background())
	run, cancel := context.WithCancel(ctx)
	fail := errors.New("example failure")
	if err := ranTo(ctx, run, fail); err != fail {
		t.Errorf("a failure → %v", err)
	}
	cancel()
	if err := ranTo(ctx, run, context.Canceled); !errors.Is(err, ErrBack) {
		t.Errorf("cancelled by the user → %v", err)
	}
	if err := ranTo(ctx, run, nil); err != nil {
		t.Errorf("finished anyway → %v", err)
	}
	quit()
	if err := ranTo(ctx, run, context.Canceled); !errors.Is(err, ErrCancelled) {
		t.Errorf("the program quit → %v", err)
	}
}

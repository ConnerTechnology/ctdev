package backupapp

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
	"github.com/charmbracelet/x/ansi"
)

// kitView is a KitScreen's step. Leaving before "saved" asks first, since
// ctdev shows these once.
type kitView struct {
	k       KitScreen
	copied  bool
	asking  bool // "Leave without saving them?" is up
	quitter bool // and ctrl+c asked it, so leaving quits
}

func newKitView(k KitScreen) *kitView { return &kitView{k: k} }

func (kv *kitView) init() tea.Cmd { return nil }

func (kv *kitView) ctrlC() (*answer, tea.Cmd) {
	if kv.asking {
		return nil, nil
	}
	kv.asking, kv.quitter = true, true
	return nil, nil
}

func (kv *kitView) update(msg tea.Msg) (*answer, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, nil
	}
	if kv.asking {
		switch k.String() {
		case "y":
			if kv.quitter {
				return &answer{err: errQuit}, nil
			}
			return &answer{err: ErrBack}, nil
		case "enter", "esc", "n":
			kv.asking, kv.quitter = false, false
		}
		return nil, nil
	}
	switch k.String() {
	case "enter":
		return &answer{}, nil
	case "c":
		kv.copied = true
		return nil, tea.SetClipboard(kv.copyText())
	case "esc":
		kv.asking = true
	}
	return nil, nil
}

func (kv *kitView) copyText() string {
	text := strings.TrimRight(kv.k.Kit, "\n")
	if kv.k.RecoveryCode != "" {
		text += "\n\nRecovery Code: " + kv.k.RecoveryCode
	}
	return text + "\n"
}

// needs is the width the kit takes unwrapped.
func (kv *kitView) needs() int {
	w := 0
	for _, l := range strings.Split(kv.k.Kit, "\n") {
		w = max(w, 4+ansi.StringWidth(l))
	}
	return w
}

func (kv *kitView) view(width, _ int) stepView {
	k := kv.k
	var lines []string
	if need := kv.needs(); width < need {
		lines = append(lines, "  "+styles.Warning.Render(fmt.Sprintf("Widen the window to at least %d columns to show the Emergency Kit.", need)))
	} else {
		heading := ""
		if k.New {
			heading = "New "
		}
		lines = append(lines, "  "+styles.Header.Render(heading+"Emergency Kit")+"  "+styles.Dimmed.Render("save it in your password manager or print it"), "")
		for _, l := range strings.Split(strings.TrimRight(k.Kit, "\n"), "\n") {
			lines = append(lines, "    "+styles.Value.Render(l))
		}
		if k.RecoveryCode != "" {
			lines = append(lines, "", "  "+styles.Header.Render(heading+"Recovery Code")+"  "+styles.Dimmed.Render("keep it apart from the kit; it resets a forgotten master password"),
				"", "    "+styles.Value.Render(k.RecoveryCode))
		}
	}
	if len(k.Lines) > 0 {
		lines = append(lines, "")
		for _, l := range k.Lines {
			lines = append(lines, "  "+styleLine(l))
		}
	}
	if kv.asking {
		return stepView{
			content: lines,
			notes:   []Line{{Heading, "Leave without saving them?"}, {Info, "ctdev shows these once."}},
			keys:    "  " + dangerChip("y  Leave") + "    " + keyHint("enter", "Stay"),
			help:    "y leaves · enter or esc stays",
		}
	}
	ack, help := "I've saved it", "c copies it where the terminal allows · esc or ctrl+c asks first"
	if k.RecoveryCode != "" {
		ack, help = "I've saved both", "c copies the kit and the code, where the terminal allows it · esc or ctrl+c asks first"
	}
	keys := "  " + chip("enter  "+orDefault(k.Ack, ack)) + "   " + keyHint("c", "Copy")
	if kv.copied {
		keys += "  " + styles.Success.Render("✓ Copied")
	}
	return stepView{content: lines, keys: keys, help: help}
}

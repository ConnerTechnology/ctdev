package backupapp

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// confirm is a Confirm's step.
type confirm struct {
	c     Confirm
	typed string
	wrong bool // the name typed last didn't match
}

func newConfirm(c Confirm) *confirm { return &confirm{c: c} }

func (cf *confirm) init() tea.Cmd { return nil }

func (cf *confirm) update(msg tea.Msg) (*answer, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, nil
	}
	key := k.String()
	no := &answer{value: false}
	if key == "esc" {
		if cf.c.Keep != "" {
			return &answer{err: ErrBack}, nil
		}
		return no, nil
	}
	switch {
	case cf.c.TypeToConfirm != "":
		switch key {
		case "enter":
			if cf.typed == cf.c.TypeToConfirm {
				return &answer{value: true}, nil
			}
			cf.typed, cf.wrong = "", true
		case "backspace":
			if r := []rune(cf.typed); len(r) > 0 {
				cf.typed = string(r[:len(r)-1])
			}
		default:
			cf.typed += k.Text
		}
		return nil, nil
	case key == "y":
		return &answer{value: true}, nil
	case cf.c.Keep != "" && (key == "enter" || key == "n"):
		return no, nil
	case cf.c.Destructive && cf.c.Keep == "":
		return no, nil // y-only: any other answer keeps things as they are
	case key == "enter":
		return &answer{value: true}, nil
	case key == "n":
		return no, nil
	}
	return nil, nil
}

func (cf *confirm) view(width, _ int) stepView {
	c := cf.c
	var lines []string
	if c.Warning != "" {
		lines = append(append(lines, wrap(Line{Warning, c.Warning}, width, 2)...), "")
	}
	lines = append(lines, wrap(Line{Heading, c.Question}, width, 2)...)
	if c.Consequence != "" {
		for _, l := range strings.Split(c.Consequence, "\n") {
			lines = append(lines, "  "+l)
		}
	}
	if c.TypeToConfirm != "" {
		lines = append(lines, "", "▸ "+styles.Dimmed.Render("TYPE "+c.TypeToConfirm+" TO CONFIRM")+"   "+styles.Value.Render(cf.typed+"█"))
		if cf.wrong {
			lines = append(lines, "    "+styles.Error.Render("That isn't "+c.TypeToConfirm+"; type it exactly, or press esc to cancel."))
		}
	}
	no := keyHint("esc", orDefault(c.No, "Cancel"))
	var keys, help string
	switch {
	case c.TypeToConfirm != "":
		keys = dangerChip("enter  " + c.Yes)
	case c.Keep != "":
		keys = chip("enter  "+c.Keep) + "    " + dangerChip("y  "+c.Yes)
		help = "enter or n keeps · y " + strings.ToLower(c.Yes) + " · esc cancels"
	case c.Destructive:
		keys = dangerChip("y  " + c.Yes)
		help = "y " + strings.ToLower(c.Yes) + " · any other answer cancels"
	default:
		keys = chip("enter  " + c.Yes)
	}
	return stepView{content: lines, keys: "  " + keys + "    " + no, help: orDefault(c.Help, help)}
}

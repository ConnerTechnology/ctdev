package backupapp

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// notice is a Notice's step.
type notice struct{ n Notice }

func newNotice(n Notice) *notice { return &notice{n: n} }

func (nt *notice) init() tea.Cmd { return nil }

func (nt *notice) update(msg tea.Msg) (*answer, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return nil, nil
	}
	switch k.String() {
	case "enter":
		return &answer{}, nil
	case "esc":
		return &answer{err: ErrBack}, nil
	}
	return nil, nil
}

func (nt *notice) view(width, _ int) stepView {
	n := nt.n
	var lines []string
	if n.Intro != "" {
		lines = append(lines, strings.Split(n.Intro, "\n")...)
		lines = append(lines, "")
	}
	if n.Heading != "" {
		lines = append(lines, "  "+styles.Header.Render(n.Heading))
	}
	for i, s := range n.Steps {
		num := fmt.Sprintf("%d. ", i+1)
		for j, l := range wrap(Line{Plain, s}, width-2-len(num), 2) {
			pre := "    " + styles.Dimmed.Render(num)
			if j > 0 {
				pre = strings.Repeat(" ", 4+len(num))
			}
			lines = append(lines, pre+strings.TrimPrefix(l, "  "))
		}
	}
	if n.Note != "" {
		lines = append(lines, "", "  "+styles.Dimmed.Render(n.Note))
	}
	keys := "  " + chip("enter  "+n.Ack) + "   " + keyHint("esc", orDefault(n.Back, "Back"))
	return stepView{content: lines, keys: keys, help: orDefault(n.Help, "press enter once that's done")}
}

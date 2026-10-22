package backupscreen

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/ctdev/ctdev/backup"
	"github.com/ConnerTechnology/ctdev/ctdev/tui/styles"
)

// The --all screens for a destination's computers and secrets (ADR-0016):
// the Computers list, a computer's actions, the Emergency Kit and password
// menu, and each action's confirmation.

// added is a key's date, as the Computers list shows it.
func added(c backup.Computer) string {
	return "added " + c.Added.Local().Format("Jan 2, 2006")
}

// ComputerList is Computers: each computer with a key on the destination, by
// hostname and date added, this one marked, then Back. It shows what
// backup.Destination.Computers gives it, which leaves out the account and
// Recovery Code keys.
type ComputerList struct {
	title     string
	computers []backup.Computer
	cursor    int // len(computers) is Back
	chosen    int // index into computers, or -1
	done      bool
}

// NewComputers builds the list for destination name.
func NewComputers(name string, computers []backup.Computer) *ComputerList {
	return &ComputerList{title: "Computers with access to " + name, computers: computers, chosen: -1}
}

// Chosen is the computer picked, if one was.
func (c *ComputerList) Chosen() (backup.Computer, bool) {
	if c.chosen < 0 {
		return backup.Computer{}, false
	}
	return c.computers[c.chosen], true
}

// Done reports whether the screen is finished: a computer was picked, or Back.
func (c *ComputerList) Done() bool { return c.done }

func (c *ComputerList) Init() tea.Cmd { return nil }

func (c *ComputerList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch k.String() {
	case "up", "k", "shift+tab":
		c.cursor = max(c.cursor-1, 0)
	case "down", "j", "tab":
		c.cursor = min(c.cursor+1, len(c.computers))
	case "q", "esc", "ctrl+c":
		c.done = true
		return c, tea.Quit
	case "enter", "space":
		if c.cursor < len(c.computers) {
			c.chosen = c.cursor
		}
		c.done = true
		return c, tea.Quit
	}
	return c, nil
}

func (c *ComputerList) View() tea.View {
	var b strings.Builder
	b.WriteString(styles.Title.Render(c.title) + "\n\n")
	if c.done {
		return tea.NewView(b.String())
	}
	width := 0
	for _, it := range c.computers {
		width = max(width, len([]rune(it.Hostname)))
	}
	for i, it := range c.computers {
		line := it.Hostname + strings.Repeat(" ", width-len([]rune(it.Hostname))) + "  " + added(it)
		if it.ThisComputer {
			line += "  (this computer)"
		}
		b.WriteString(menuLine(line, i == c.cursor))
	}
	b.WriteString(menuLine("Back", c.cursor == len(c.computers)))
	b.WriteString("\n  " + styles.Help.Render("↑/↓ move · enter open · esc back") + "\n")
	return tea.NewView(b.String())
}

// NewComputerActions is one computer's menu: Remove this computer · I lost
// this computer · Back. This computer's own key can't be removed from here,
// so for it both are dimmed.
func NewComputerActions(c backup.Computer) *Model {
	status := styles.Title.Render(c.Hostname+" · "+added(c)) + "\n"
	if c.ThisComputer {
		status += "  " + styles.Dimmed.Render("This computer: remove it from another computer that's set up.") + "\n"
	}
	m := &Model{status: status, sub: true, items: []Item{
		{Action: RemoveComputer, Label: "Remove this computer", Dimmed: c.ThisComputer},
		{Action: LostComputer, Label: "I lost this computer", Dimmed: c.ThisComputer},
		{Action: GoBack, Label: "Back"},
	}}
	m.cursor = m.next(-1, 1)
	return m
}

// NewKitMenu is Emergency Kit and password for destination name.
func NewKitMenu(name string) *Model {
	m := &Model{status: styles.Title.Render("Emergency Kit and password for "+name) + "\n", sub: true, items: []Item{
		{Action: ShowKit, Label: "Show Emergency Kit"},
		{Action: ChangePassword, Label: "Change master password"},
		{Action: RegenerateKey, Label: "Regenerate Secret Key"},
		{Action: ReplaceCredentials, Label: "Replace backend credentials"},
		{Action: UseRecoveryCode, Label: "Use a Recovery Code"},
		{Action: PasteKit, Label: "Paste an updated Emergency Kit"},
		{Action: GoBack, Label: "Back"},
	}}
	m.cursor = m.next(-1, 1)
	return m
}

// ConfirmRemoveComputer confirms removing host's key from destination name.
func ConfirmRemoveComputer(host, name string) *Confirm {
	return NewConfirm(fmt.Sprintf("Remove %s's key? %s can no longer open %s.", host, host, name))
}

// ConfirmLostComputer confirms the lost-computer flow for host.
func ConfirmLostComputer(host, name string) *Confirm {
	return NewConfirm(fmt.Sprintf("Remove %s's key from %s, make a new Secret Key and take new backend credentials? "+
		"You'll paste the new kit once on every other computer.", host, name))
}

// ConfirmChangePassword confirms a master password change.
func ConfirmChangePassword(name string) *Confirm {
	return NewConfirm("Change the master password for " + name + "? The Emergency Kit stays the same.")
}

// ConfirmRegenerate confirms a new Secret Key.
func ConfirmRegenerate(name string) *Confirm {
	return NewConfirm("Make a new Secret Key for " + name + "? The old Emergency Kit stops working, " +
		"and you'll paste the new kit once on every other computer.")
}

// ConfirmNewRecoveryCode asks whether a Recovery Code reset also replaces the
// code.
func ConfirmNewRecoveryCode() *Confirm {
	return NewConfirm("Also make a new Recovery Code? The old one stops working.")
}

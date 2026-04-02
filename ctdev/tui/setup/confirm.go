package setup

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	s "github.com/ConnerTechnology/dotfiles/ctdev/setup"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

// ConfirmModel handles the confirmation screen before applying changes.
type ConfirmModel struct {
	changes   []changeEntry
	confirmed bool
	cancelled bool
	dryRun    bool
}

type changeEntry struct {
	name string
	from string
	to   string
}

func NewConfirm(states []s.SettingState, dryRun bool) ConfirmModel {
	var changes []changeEntry
	for _, st := range states {
		if st.NeedsApply(false) {
			changes = append(changes, changeEntry{name: st.Setting.Name, from: st.CurrentValue, to: st.DesiredValue})
		}
	}
	return ConfirmModel{changes: changes, dryRun: dryRun}
}

func (inst *ConfirmModel) Confirmed() bool { return inst.confirmed }
func (inst *ConfirmModel) Cancelled() bool { return inst.cancelled }

func (inst *ConfirmModel) Update(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "enter":
		inst.confirmed = true
	case "esc", "q", "ctrl+c":
		inst.cancelled = true
	}
}

func (inst *ConfirmModel) View(width, height int) string {
	var b strings.Builder

	if inst.dryRun {
		b.WriteString(styles.Title.Render("[dry-run] Would apply:"))
	} else {
		b.WriteString(styles.Title.Render("Apply Changes?"))
	}
	b.WriteString("\n\n")

	if len(inst.changes) == 0 {
		b.WriteString("  No changes to apply.\n")
	} else {
		// Find max name width for alignment
		maxName := 0
		for _, c := range inst.changes {
			if len(c.name) > maxName {
				maxName = len(c.name)
			}
		}

		for _, c := range inst.changes {
			name := lipgloss.NewStyle().Foreground(styles.Bright).Width(maxName + 2).Render(c.name)
			b.WriteString(fmt.Sprintf("  %s %s → %s\n",
				name,
				styles.Warning.Render(c.from),
				styles.Success.Render(c.to),
			))
		}
	}

	b.WriteString("\n")

	count := len(inst.changes)
	settingsWord := "settings"
	if count == 1 {
		settingsWord = "setting"
	}

	if inst.dryRun {
		b.WriteString(styles.Help.Render(fmt.Sprintf("%d %s would be applied. Esc to go back", count, settingsWord)))
	} else {
		b.WriteString(styles.Help.Render(fmt.Sprintf("%d %s will be applied. Enter to confirm · Esc to go back", count, settingsWord)))
	}

	return b.String()
}

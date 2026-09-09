// Package listdiff is the picker `ctdev pihole sync` shows: one row per
// difference between lists.toml and the live Pi-hole, grouped by list. Like
// tui/checklist it is a thin adapter over tui/multiselect — it supplies the
// grouping, badges and preselection, and maps the selection back to changes.
package listdiff

import (
	"strconv"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/piholelists"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/multiselect"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type Model struct {
	ms      *multiselect.Model
	changes []piholelists.Change
}

type Result struct {
	Selected []piholelists.Change
	Quit     bool
}

func New(changes []piholelists.Change) Model {
	byKind := map[piholelists.Kind][]int{}
	for i, c := range changes {
		byKind[c.Entry.Kind] = append(byKind[c.Entry.Kind], i)
	}

	var groups []multiselect.Group
	for _, kind := range piholelists.Kinds {
		if len(byKind[kind]) == 0 {
			continue
		}
		var items []multiselect.Item
		for _, idx := range byKind[kind] {
			c := changes[idx]
			items = append(items, multiselect.Item{
				ID:         strconv.Itoa(idx),
				Primary:    c.Entry.Key,
				Secondary:  c.Detail(),
				Badges:     []multiselect.Badge{{Text: c.Op.String(), Style: badgeStyle(c.Op)}},
				Selectable: true,
				Bulk:       true,
				// A removal deletes something a person added on the Pi-hole
				// itself, so it is never applied by just pressing Enter —
				// leaving it unchecked keeps it, and sync says how to record it.
				NoPreselect: c.Op == piholelists.Remove,
			})
		}
		groups = append(groups, multiselect.Group{Key: kind.Key(), Title: kind.Label(), Items: items})
	}

	ms := multiselect.New(groups, multiselect.Options{
		Title:        "Pi-hole lists — changes to apply",
		StatusSuffix: "unchecked removals stay on this Pi-hole",
		PreselectAll: true,
	})
	return Model{ms: ms, changes: changes}
}

func badgeStyle(op piholelists.Op) lipgloss.Style {
	switch op {
	case piholelists.Add:
		return styles.BadgeInfo
	case piholelists.Update:
		return styles.BadgeWarn
	default:
		return styles.BadgeDanger
	}
}

func (inst *Model) Init() tea.Cmd { return inst.ms.Init() }

func (inst *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return inst, inst.ms.Update(msg)
}

func (inst *Model) View() tea.View { return inst.ms.View() }

func (inst *Model) GetResult() Result {
	r := inst.ms.Result()
	if r.Quit {
		return Result{Quit: true}
	}
	var out []piholelists.Change
	for _, id := range r.Selected {
		if idx, err := strconv.Atoi(id); err == nil && idx >= 0 && idx < len(inst.changes) {
			out = append(out, inst.changes[idx])
		}
	}
	return Result{Selected: out}
}

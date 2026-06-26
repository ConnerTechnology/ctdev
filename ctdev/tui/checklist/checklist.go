// Package checklist is the interactive "Available Updates" screen. It is a thin
// adapter over tui/multiselect: it groups update items by source, renders the
// version transition and severity badges, and maps the selection back to the
// concrete UpdateItems the caller passes in.
package checklist

import (
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/multiselect"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/styles"
)

type UpdateItem struct {
	Name       string
	Source     string // "apt", "flatpak", "git", "runtime", "brew", "docker", ...
	CurrentVer string
	NewVer     string
	IsMajor    bool
	IsKernel   bool
}

type Model struct {
	ms    *multiselect.Model
	items []UpdateItem
}

type Result struct {
	Selected []UpdateItem
	Quit     bool
}

func New(items []UpdateItem) Model {
	// Group by source, preserving first-seen order (matches scanAll's sort).
	var order []string
	groups := map[string][]int{}
	for i, it := range items {
		if _, ok := groups[it.Source]; !ok {
			order = append(order, it.Source)
		}
		groups[it.Source] = append(groups[it.Source], i)
	}

	// Width of the current-version column, for aligned "cur → new" transitions.
	curW := 0
	for _, it := range items {
		if w := len(it.CurrentVer); w > curW {
			curW = w
		}
	}

	var gs []multiselect.Group
	for _, src := range order {
		var mItems []multiselect.Item
		for _, idx := range groups[src] {
			it := items[idx]
			var badges []multiselect.Badge
			if it.IsMajor {
				badges = append(badges, multiselect.Badge{Text: "MAJOR", Style: styles.BadgeWarn})
			}
			if it.IsKernel {
				badges = append(badges, multiselect.Badge{Text: "KERNEL", Style: styles.BadgeDanger})
			}
			mItems = append(mItems, multiselect.Item{
				ID:         strconv.Itoa(idx),
				Primary:    it.Name,
				Secondary:  fmt.Sprintf("%-*s → %s", curW, it.CurrentVer, it.NewVer),
				Badges:     badges,
				Selectable: true,
				Bulk:       true,
			})
		}
		gs = append(gs, multiselect.Group{Key: src, Title: sourceLabel(src), Items: mItems})
	}

	ms := multiselect.New(gs, multiselect.Options{
		Title:        "Available Updates",
		PreselectAll: true,
	})
	return Model{ms: ms, items: items}
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
	var out []UpdateItem
	for _, id := range r.Selected {
		if idx, err := strconv.Atoi(id); err == nil && idx >= 0 && idx < len(inst.items) {
			out = append(out, inst.items[idx])
		}
	}
	return Result{Selected: out}
}

func sourceLabel(source string) string {
	switch source {
	case "apt":
		return "System Packages (apt)"
	case "brew":
		return "System Packages (brew)"
	case "brew-cask":
		return "Desktop Apps (brew cask)"
	case "mintupdate":
		return "System Packages (Mint)"
	case "flatpak":
		return "Flatpak"
	case "git":
		return "Git Repositories"
	case "runtime":
		return "Runtimes"
	case "npm":
		return "NPM Global Packages"
	case "ctdev":
		return "ctdev"
	case "cli":
		return "CLI Tools"
	case "docker":
		return "Docker (containers)"
	default:
		return source
	}
}

// Package picker is the interactive component install/uninstall screen. It is a
// thin adapter over tui/multiselect: it groups components by category, marks
// already-installed and unsupported components, and maps the selection back to
// component names.
package picker

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/ConnerTechnology/dotfiles/ctdev/component"
	"github.com/ConnerTechnology/dotfiles/ctdev/tui/multiselect"
)

type Mode int

const (
	ModeInstall Mode = iota
	ModeUninstall
)

// categoryOrder is the fixed top-to-bottom ordering of category sections.
var categoryOrder = []component.Category{
	component.CategoryCLI,
	component.CategoryDesktop,
	component.CategoryRuntime,
	component.CategorySecurity,
	component.CategoryInfra,
	component.CategorySystem,
}

type Model struct {
	ms *multiselect.Model
}

type Result struct {
	Selected []string
	Quit     bool
}

func New(components []component.Component, installed map[string]bool, os component.OS, mode Mode) Model {
	groups := component.GroupByCategory(components)

	installedCount := 0
	var gs []multiselect.Group
	for _, cat := range categoryOrder {
		comps, ok := groups[cat]
		if !ok || len(comps) == 0 {
			continue
		}
		var items []multiselect.Item
		for _, c := range comps {
			supported := c.SupportsOS(os)
			isInstalled := installed[c.Name]
			if isInstalled {
				installedCount++
			}

			// Bulk (select-all/invert) targets depend on mode: install offers the
			// not-yet-installed, uninstall offers the installed. Either way an
			// unsupported component is never a target.
			bulk := supported && (isInstalled == (mode == ModeUninstall))

			items = append(items, multiselect.Item{
				ID:         c.Name,
				Primary:    c.Name,
				Secondary:  c.Description,
				Note:       unsupportedNote(c, supported),
				Search:     strings.Join(c.Tags, " "),
				Selectable: supported,
				Bulk:       bulk,
				Marked:     isInstalled && mode == ModeInstall,
			})
		}
		gs = append(gs, multiselect.Group{Key: string(cat), Title: string(cat), Items: items})
	}

	title := "Select components to install"
	if mode == ModeUninstall {
		title = "Select components to uninstall"
	}
	ms := multiselect.New(gs, multiselect.Options{
		Title:        title,
		StatusSuffix: fmt.Sprintf("· %d already installed", installedCount),
	})
	return Model{ms: ms}
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
	return Result{Selected: r.Selected}
}

// unsupportedNote returns a "(linux only)" style trailing note for components
// the current OS can't install, or "" when supported.
func unsupportedNote(c component.Component, supported bool) string {
	if supported {
		return ""
	}
	osLabel := "other"
	for _, s := range c.SupportedOS {
		if s != component.OSAny {
			osLabel = string(s)
		}
	}
	return "(" + osLabel + " only)"
}

package settings

import "charm.land/bubbles/v2/key"

// keyMap doubles as the help.KeyMap so the footer help bar renders from it.
type keyMap struct {
	Up        key.Binding
	Down      key.Binding
	Home      key.Binding
	End       key.Binding
	Cycle     key.Binding
	Dec       key.Binding
	Inc       key.Binding
	Recommend key.Binding
	Revert    key.Binding
	Filter    key.Binding
	Apply     key.Binding
	Help      key.Binding
	Quit      key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:        key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:      key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Home:      key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		End:       key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		Cycle:     key.NewBinding(key.WithKeys("enter", "space"), key.WithHelp("enter", "change value")),
		Dec:       key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←", "slider down")),
		Inc:       key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→", "slider up")),
		Recommend: key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "set recommended")),
		Revert:    key.NewBinding(key.WithKeys("u"), key.WithHelp("u", "revert")),
		Filter:    key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Apply:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "apply changes")),
		Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Quit:      key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"), key.WithHelp("q", "quit (discard)")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Cycle, k.Recommend, k.Filter, k.Apply, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Home, k.End},
		{k.Cycle, k.Dec, k.Inc},
		{k.Recommend, k.Revert},
		{k.Filter, k.Apply, k.Help, k.Quit},
	}
}

package multiselect

import "charm.land/bubbles/v2/key"

// keyMap is the full set of bindings for the multi-select widget. It doubles as
// the help.KeyMap so the footer help bar is generated straight from it.
type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Home    key.Binding
	End     key.Binding
	Toggle  key.Binding
	Group   key.Binding
	All     key.Binding
	None    key.Binding
	Invert  key.Binding
	Filter  key.Binding
	Help    key.Binding
	Confirm key.Binding
	Quit    key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:      key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:    key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		Home:    key.NewBinding(key.WithKeys("g", "home"), key.WithHelp("g", "top")),
		End:     key.NewBinding(key.WithKeys("G", "end"), key.WithHelp("G", "bottom")),
		Toggle:  key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle")),
		Group:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "collapse group")),
		All:     key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "all")),
		None:    key.NewBinding(key.WithKeys("A", "n"), key.WithHelp("A", "none")),
		Invert:  key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "invert")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Help:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Confirm: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
		Quit:    key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp / FullHelp implement help.KeyMap.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Toggle, k.Filter, k.Confirm, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Home, k.End},
		{k.Toggle, k.Group},
		{k.All, k.None, k.Invert},
		{k.Filter, k.Confirm, k.Help, k.Quit},
	}
}

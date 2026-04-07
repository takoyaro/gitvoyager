package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up      key.Binding
	Down    key.Binding
	Enter   key.Binding
	Back    key.Binding
	Search  key.Binding
	Sort    key.Binding
	Open    key.Binding
	Clone   key.Binding
	Help    key.Binding
	Quit    key.Binding
	Tab     key.Binding
	Escape  key.Binding
	PageUp  key.Binding
	PageDn  key.Binding
	GoTop   key.Binding
	GoEnd   key.Binding
}

var keys = keyMap{
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("k/up", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("j/down", "down"),
	),
	Enter: key.NewBinding(
		key.WithKeys("enter", "l"),
		key.WithHelp("enter/l", "select"),
	),
	Back: key.NewBinding(
		key.WithKeys("h"),
		key.WithHelp("h", "back"),
	),
	Search: key.NewBinding(
		key.WithKeys("/"),
		key.WithHelp("/", "search"),
	),
	Sort: key.NewBinding(
		key.WithKeys("s"),
		key.WithHelp("s", "sort"),
	),
	Open: key.NewBinding(
		key.WithKeys("o"),
		key.WithHelp("o", "open"),
	),
	Clone: key.NewBinding(
		key.WithKeys("c"),
		key.WithHelp("c", "clone"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("?", "help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q"),
		key.WithHelp("q", "quit"),
	),
	Tab: key.NewBinding(
		key.WithKeys("tab"),
		key.WithHelp("tab", "switch pane"),
	),
	Escape: key.NewBinding(
		key.WithKeys("escape"),
		key.WithHelp("esc", "back"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+u"),
		key.WithHelp("pgup", "page up"),
	),
	PageDn: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+d"),
		key.WithHelp("pgdn", "page down"),
	),
	GoTop: key.NewBinding(
		key.WithKeys("home", "g"),
		key.WithHelp("g", "top"),
	),
	GoEnd: key.NewBinding(
		key.WithKeys("end", "G"),
		key.WithHelp("G", "bottom"),
	),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Search, k.Open, k.Clone, k.Sort, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Back},
		{k.Search, k.Sort, k.Tab},
		{k.Open, k.Clone},
		{k.PageUp, k.PageDn, k.GoTop, k.GoEnd},
		{k.Help, k.Quit},
	}
}

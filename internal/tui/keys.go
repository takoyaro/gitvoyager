package tui

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up            key.Binding
	Down          key.Binding
	Enter         key.Binding
	Back          key.Binding
	Search        key.Binding
	Sort          key.Binding
	Open          key.Binding
	Clone         key.Binding
	Watch         key.Binding
	Watchlist     key.Binding
	Filter        key.Binding
	AdvancedSearch key.Binding
	Help          key.Binding
	Quit          key.Binding
	Tab           key.Binding
	Escape        key.Binding
	PageUp        key.Binding
	PageDn        key.Binding
	GoTop         key.Binding
	GoEnd         key.Binding
	Peek          key.Binding
	Yank          key.Binding
	YankClone     key.Binding
	Exclude       key.Binding
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
	Watch: key.NewBinding(
		key.WithKeys("w"),
		key.WithHelp("w", "watch"),
	),
	Watchlist: key.NewBinding(
		key.WithKeys("W"),
		key.WithHelp("W", "watchlist"),
	),
	AdvancedSearch: key.NewBinding(
		key.WithKeys("a"),
		key.WithHelp("a", "advanced search"),
	),
	Filter: key.NewBinding(
		key.WithKeys("f"),
		key.WithHelp("f", "filter"),
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
	Peek: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "peek"),
	),
	Yank: key.NewBinding(
		key.WithKeys("y"),
		key.WithHelp("y", "yank url"),
	),
	YankClone: key.NewBinding(
		key.WithKeys("Y"),
		key.WithHelp("Y", "yank clone cmd"),
	),
	Exclude: key.NewBinding(
		key.WithKeys("x"),
		key.WithHelp("x", "exclude"),
	),
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Tab, k.Search, k.Filter, k.Open, k.Clone, k.Watch, k.Watchlist, k.Sort, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Enter, k.Back},
		{k.Search, k.AdvancedSearch, k.Sort, k.Filter, k.Tab},
		{k.Open, k.Clone, k.Watch, k.Watchlist, k.Peek, k.Yank, k.Exclude},
		{k.PageUp, k.PageDn, k.GoTop, k.GoEnd},
		{k.Help, k.Quit},
	}
}

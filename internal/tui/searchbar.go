package tui

import (
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

type searchBarModel struct {
	input  textinput.Model
	active bool
	width  int
	query  string // last submitted query
	sort   string
	count  int
}

func newSearchBar() searchBarModel {
	ti := textinput.New()
	ti.Placeholder = "Search repos (e.g. mcp server, language:go, topic:cli)"
	ti.SetWidth(60)
	ti.Prompt = "  Search: "

	return searchBarModel{
		input: ti,
		sort:  "stars",
	}
}

func (m *searchBarModel) SetWidth(w int) {
	m.width = w
	m.input.SetWidth(w - 12)
}

func (m *searchBarModel) Focus() {
	m.active = true
	m.input.Focus()
}

func (m *searchBarModel) Blur() {
	m.active = false
	m.input.Blur()
}

func (m *searchBarModel) Value() string {
	return m.input.Value()
}

func (m *searchBarModel) SetQuery(q string) {
	m.query = q
}

func (m *searchBarModel) SetSort(s string) {
	m.sort = s
}

func (m *searchBarModel) SetCount(n int) {
	m.count = n
}

func (m *searchBarModel) Reset() {
	m.input.SetValue("")
}

func (m *searchBarModel) View() string {
	left := m.input.View()

	right := ""
	if m.query != "" {
		info := styleSubtle.Render("[" + m.query + "]")
		sortInfo := styleAccent.Render(" sort:" + m.sort)
		right = info + sortInfo
	}

	// Pad to fill width
	return lipgloss.JoinHorizontal(lipgloss.Center, left, "  ", right)
}

package tui

import (
	"charm.land/bubbles/v2/help"
	"charm.land/lipgloss/v2"
)

type helpModel struct {
	help   help.Model
	active bool
	width  int
	height int
}

func newHelpModel() helpModel {
	h := help.New()
	h.ShortSeparator = "  "
	return helpModel{
		help: h,
	}
}

func (m *helpModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.help.SetWidth(w - 4)
}

func (m *helpModel) Toggle() {
	m.active = !m.active
}

func (m *helpModel) Show() {
	m.active = true
}

func (m *helpModel) Hide() {
	m.active = false
}

func (m *helpModel) IsActive() bool {
	return m.active
}

func (m *helpModel) View() string {
	if !m.active {
		return ""
	}

	content := m.help.FullHelpView(keys.FullHelp())

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccent).
		Padding(1, 2).
		Width(m.width - 10).
		Align(lipgloss.Center)

	title := styleHeader.Render("GitVoyager Help")

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, title, boxStyle.Render(content)),
	)
}

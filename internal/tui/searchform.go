package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/model"
)

const formSortFieldIdx = 5

type searchFormModel struct {
	fields   [5]textinput.Model
	labels   [5]string
	focusIdx int // 0–4 = text fields, 5 = sort row
	sortIdx  int
}

func newSearchForm() searchFormModel {
	labels := [5]string{
		"Query",
		"Language",
		"Stars",
		"Topics",
		"Created after",
	}
	placeholders := [5]string{
		"mcp server, language:go, ...",
		"go, python, rust, ...",
		"10..500  or  >100  or  <50",
		"cli, llm, mcp  (comma-separated)",
		"2026-01-01",
	}

	var fields [5]textinput.Model
	for i := range fields {
		f := textinput.New()
		f.Placeholder = placeholders[i]
		f.Prompt = ""
		fields[i] = f
	}

	return searchFormModel{
		fields: fields,
		labels: labels,
	}
}

// Focus activates the form, focusing the first field.
func (m *searchFormModel) Focus() tea.Cmd {
	for i := range m.fields {
		m.fields[i].Blur()
	}
	m.focusIdx = 0
	return m.fields[0].Focus()
}

// Blur deactivates all fields.
func (m *searchFormModel) Blur() {
	for i := range m.fields {
		m.fields[i].Blur()
	}
}

// Reset clears all fields and resets focus.
func (m *searchFormModel) Reset() {
	for i := range m.fields {
		m.fields[i].SetValue("")
	}
	m.sortIdx = 0
}

// OnSortField returns true when the sort row is focused.
func (m *searchFormModel) OnSortField() bool {
	return m.focusIdx == formSortFieldIdx
}

// NextField advances focus to the next field (wraps around).
func (m *searchFormModel) NextField() tea.Cmd {
	if m.focusIdx < len(m.fields) {
		m.fields[m.focusIdx].Blur()
	}
	m.focusIdx = (m.focusIdx + 1) % (formSortFieldIdx + 1)
	if m.focusIdx < len(m.fields) {
		return m.fields[m.focusIdx].Focus()
	}
	return nil
}

// PrevField moves focus to the previous field.
func (m *searchFormModel) PrevField() tea.Cmd {
	if m.focusIdx < len(m.fields) {
		m.fields[m.focusIdx].Blur()
	}
	total := formSortFieldIdx + 1
	m.focusIdx = (m.focusIdx - 1 + total) % total
	if m.focusIdx < len(m.fields) {
		return m.fields[m.focusIdx].Focus()
	}
	return nil
}

// UpdateActiveField forwards a key message to the currently focused textinput.
func (m *searchFormModel) UpdateActiveField(msg tea.Msg) tea.Cmd {
	if m.focusIdx >= len(m.fields) {
		return nil
	}
	var cmd tea.Cmd
	m.fields[m.focusIdx], cmd = m.fields[m.focusIdx].Update(msg)
	return cmd
}

// CycleSortNext advances to the next sort option.
func (m *searchFormModel) CycleSortNext() {
	m.sortIdx = (m.sortIdx + 1) % len(model.SortCycle)
}

// CycleSortPrev moves to the previous sort option.
func (m *searchFormModel) CycleSortPrev() {
	m.sortIdx = (m.sortIdx - 1 + len(model.SortCycle)) % len(model.SortCycle)
}

// IsEmpty returns true if no field has content.
func (m *searchFormModel) IsEmpty() bool {
	for _, f := range m.fields {
		if strings.TrimSpace(f.Value()) != "" {
			return false
		}
	}
	return true
}

// BuildParams converts the form values into a SearchParams.
func (m *searchFormModel) BuildParams() model.SearchParams {
	query := strings.TrimSpace(m.fields[0].Value())

	// Append created-after qualifier to the query string.
	if created := strings.TrimSpace(m.fields[4].Value()); created != "" {
		if !strings.Contains(query, "created:") {
			if query != "" {
				query += " "
			}
			query += "created:>" + created
		}
	}

	topicsStr := strings.TrimSpace(m.fields[3].Value())
	var topics []string
	for _, t := range strings.Split(topicsStr, ",") {
		if t = strings.TrimSpace(t); t != "" {
			topics = append(topics, t)
		}
	}

	return model.SearchParams{
		Query:    query,
		Language: strings.TrimSpace(m.fields[1].Value()),
		Stars:    strings.TrimSpace(m.fields[2].Value()),
		Topics:   topics,
		Sort:     model.SortCycle[m.sortIdx],
		Order:    "desc",
		Limit:    50,
	}
}

// View renders the form panel at the given width.
func (m *searchFormModel) View(width int) string {
	fieldW := width - 22
	if fieldW < 20 {
		fieldW = 20
	}

	var lines []string

	title := styleAccent.Render("  Advanced Search")
	hint := styleSubtle.Render("  tab/shift+tab: field  ←/→: sort  enter: search  esc: cancel")
	lines = append(lines, title)
	lines = append(lines, hint)
	lines = append(lines, "")

	for i := range m.fields {
		focused := m.focusIdx == i
		lStyle := styleSubtle
		if focused {
			lStyle = styleAccent
		}
		label := fmt.Sprintf("  %-15s", m.labels[i]+":")
		m.fields[i].SetWidth(fieldW)
		row := lStyle.Render(label) + m.fields[i].View()
		lines = append(lines, row)
	}

	// Sort row
	sortFocused := m.focusIdx == formSortFieldIdx
	lStyle := styleSubtle
	if sortFocused {
		lStyle = styleAccent
	}
	label := fmt.Sprintf("  %-15s", "Sort:")
	currentSort := string(model.SortCycle[m.sortIdx])
	var sortVal string
	if sortFocused {
		sortVal = styleListItemSelected.Render(" " + currentSort + " ") +
			styleSubtle.Render("  ←/→")
	} else {
		sortVal = styleAccent.Render(currentSort)
	}
	lines = append(lines, lStyle.Render(label)+sortVal)

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

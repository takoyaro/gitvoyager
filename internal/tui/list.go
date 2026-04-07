package tui

import (
	"fmt"
	"strings"

	"github.com/takoyaro/gitvoyager/internal/model"
)

type listModel struct {
	repos    []model.Repo
	filtered []int // indices into repos
	cursor   int
	offset   int // scroll offset for viewport
	height   int
	width    int
	filter   string
	seenSet  map[string]bool
}

func newListModel() listModel {
	return listModel{
		seenSet: make(map[string]bool),
	}
}

func (m *listModel) SetRepos(repos []model.Repo) {
	m.repos = repos
	m.applyFilter()
	m.cursor = 0
	m.offset = 0
}

func (m *listModel) SetSeen(seen map[string]bool) {
	m.seenSet = seen
}

func (m *listModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *listModel) Selected() *model.Repo {
	if len(m.filtered) == 0 {
		return nil
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	idx := m.filtered[m.cursor]
	return &m.repos[idx]
}

func (m *listModel) MoveUp() {
	if m.cursor > 0 {
		m.cursor--
		if m.cursor < m.offset {
			m.offset = m.cursor
		}
	}
}

func (m *listModel) MoveDown() {
	if m.cursor < len(m.filtered)-1 {
		m.cursor++
		if m.cursor >= m.offset+m.height {
			m.offset = m.cursor - m.height + 1
		}
	}
}

func (m *listModel) GoTop() {
	m.cursor = 0
	m.offset = 0
}

func (m *listModel) GoBottom() {
	m.cursor = len(m.filtered) - 1
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= m.height {
		m.offset = m.cursor - m.height + 1
	}
}

func (m *listModel) PageUp() {
	m.cursor -= m.height / 2
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
}

func (m *listModel) PageDown() {
	m.cursor += m.height / 2
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= m.offset+m.height {
		m.offset = m.cursor - m.height + 1
	}
}

func (m *listModel) SetFilter(f string) {
	m.filter = f
	m.applyFilter()
	m.cursor = 0
	m.offset = 0
}

func (m *listModel) applyFilter() {
	if m.filter == "" {
		m.filtered = make([]int, len(m.repos))
		for i := range m.repos {
			m.filtered[i] = i
		}
		return
	}

	f := strings.ToLower(m.filter)
	m.filtered = m.filtered[:0]
	for i, r := range m.repos {
		text := strings.ToLower(r.FullName + " " + r.Description + " " + r.Language)
		if strings.Contains(text, f) {
			m.filtered = append(m.filtered, i)
		}
	}
}

func (m *listModel) Len() int {
	return len(m.filtered)
}

func (m *listModel) View() string {
	if len(m.filtered) == 0 {
		return styleSubtle.Width(m.width).Render("  No results")
	}

	var b strings.Builder
	end := m.offset + m.height
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := m.offset; i < end; i++ {
		idx := m.filtered[i]
		repo := m.repos[idx]
		line := m.renderItem(repo, i == m.cursor)
		b.WriteString(line)
		if i < end-1 {
			b.WriteByte('\n')
		}
	}

	return b.String()
}

func (m *listModel) renderItem(r model.Repo, selected bool) string {
	stars := formatStars(r.Stars)
	lang := abbreviateLang(r.Language)

	// Star count
	starStr := styleStars.Render(fmt.Sprintf("★ %5s", stars))

	// Language dot
	langStr := styleLang.Foreground(langColor(r.Language)).Render(fmt.Sprintf("● %-3s", lang))

	// Repo name - truncate to fit
	nameWidth := m.width - 14 // stars(8) + lang(5) + padding(1)
	if nameWidth < 10 {
		nameWidth = 10
	}
	name := r.FullName
	if len(name) > nameWidth {
		name = name[:nameWidth-1] + "…"
	}

	// Seen indicator
	prefix := " "
	if m.seenSet[r.FullName] {
		prefix = styleSubtle.Render("·")
	}

	line := fmt.Sprintf("%s%s %s %s", prefix, starStr, langStr, name)

	if selected {
		return styleListItemSelected.Width(m.width).Render(line)
	}
	return styleListItem.Width(m.width).Render(line)
}

func formatStars(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func abbreviateLang(lang string) string {
	abbrevs := map[string]string{
		"Go":         "Go",
		"Python":     "Py",
		"Rust":       "Rs",
		"TypeScript": "TS",
		"JavaScript": "JS",
		"Java":       "Jv",
		"C++":        "C+",
		"C#":         "C#",
		"Ruby":       "Rb",
		"Shell":      "Sh",
		"Kotlin":     "Kt",
		"Swift":      "Sw",
		"Dart":       "Da",
		"Lua":        "Lu",
		"Zig":        "Zi",
		"Elixir":     "Ex",
		"Haskell":    "Hs",
		"OCaml":      "ML",
		"Scala":      "Sc",
		"Clojure":    "Cl",
	}
	if a, ok := abbrevs[lang]; ok {
		return a
	}
	if len(lang) >= 2 {
		return lang[:2]
	}
	return lang
}

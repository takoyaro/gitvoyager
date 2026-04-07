package tui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/model"
)

type listModel struct {
	repos    []model.Repo
	filtered []int // indices into repos
	cursor   int
	offset   int // scroll offset (in items, not rows)
	height   int // raw terminal rows available
	width    int
	filter   string
	seenSet  map[string]bool
	watchSet map[string]bool
	loading  bool
	focused  bool
}

func newListModel() listModel {
	return listModel{
		seenSet:  make(map[string]bool),
		watchSet: make(map[string]bool),
	}
}

// visibleCount returns how many items fit on screen (2 lines per item).
func (m *listModel) visibleCount() int {
	v := m.height / 2
	if v < 1 {
		v = 1
	}
	return v
}

func (m *listModel) SetRepos(repos []model.Repo) {
	m.repos = repos
	m.applyFilter()
	m.cursor = 0
	m.offset = 0
}

func (m *listModel) SetSeen(seen map[string]bool)       { m.seenSet = seen }
func (m *listModel) SetWatched(watched map[string]bool)  { m.watchSet = watched }
func (m *listModel) SetLoading(v bool)                   { m.loading = v }
func (m *listModel) SetFocused(v bool)                   { m.focused = v }

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
	vis := m.visibleCount()
	if m.cursor < len(m.filtered)-1 {
		m.cursor++
		if m.cursor >= m.offset+vis {
			m.offset = m.cursor - vis + 1
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
	vis := m.visibleCount()
	if m.cursor >= vis {
		m.offset = m.cursor - vis + 1
	}
}

func (m *listModel) PageUp() {
	vis := m.visibleCount()
	m.cursor -= vis / 2
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
}

func (m *listModel) PageDown() {
	vis := m.visibleCount()
	m.cursor += vis / 2
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= m.offset+vis {
		m.offset = m.cursor - vis + 1
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
	// Skeleton loading state
	if m.loading && len(m.filtered) == 0 {
		return m.renderSkeleton()
	}

	if len(m.filtered) == 0 {
		return styleSubtle.Width(m.width).Render("  No results")
	}

	vis := m.visibleCount()
	end := m.offset + vis
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	// Render list items
	itemW := m.width
	needScroll := len(m.filtered) > vis
	if needScroll {
		itemW = m.width - 1 // reserve 1 col for scrollbar
	}
	savedW := m.width
	m.width = itemW

	var lines []string
	for i := m.offset; i < end; i++ {
		idx := m.filtered[i]
		repo := m.repos[idx]
		line1, line2 := m.renderItem(repo, i == m.cursor)
		lines = append(lines, line1, line2)
	}

	m.width = savedW
	listContent := strings.Join(lines, "\n")

	if !needScroll {
		return listContent
	}

	// Render scrollbar
	totalLines := len(lines)
	scrollbar := m.renderScrollbar(totalLines)
	return lipgloss.JoinHorizontal(lipgloss.Top, listContent, scrollbar)
}

func (m *listModel) renderScrollbar(trackH int) string {
	total := len(m.filtered)
	vis := m.visibleCount()
	if total <= vis || trackH <= 0 {
		return ""
	}

	thumbH := trackH * vis / total
	if thumbH < 1 {
		thumbH = 1
	}
	thumbPos := 0
	if total > vis {
		thumbPos = trackH * m.offset / total
	}
	if thumbPos+thumbH > trackH {
		thumbPos = trackH - thumbH
	}

	var b strings.Builder
	for i := 0; i < trackH; i++ {
		if i >= thumbPos && i < thumbPos+thumbH {
			b.WriteString(styleScrollThumb.Render("┃"))
		} else {
			b.WriteString(styleScrollTrack.Render("│"))
		}
		if i < trackH-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *listModel) renderSkeleton() string {
	var b strings.Builder
	// Deterministic "random" block lengths for skeleton rows
	lengths := [][2]int{{9, 16}, {12, 10}, {7, 18}, {14, 8}, {10, 14}, {8, 12}, {11, 15}, {6, 19}}
	vis := m.visibleCount()
	if vis > len(lengths) {
		vis = len(lengths)
	}
	ghost := styleGhost
	for i := 0; i < vis; i++ {
		l := lengths[i]
		line1 := ghost.Render("  " + strings.Repeat("█", l[0]) + " " + strings.Repeat("█", l[1]))
		line2 := ghost.Render("       " + strings.Repeat("█", l[1]-3) + "   " + strings.Repeat("█", 5))
		b.WriteString(lipgloss.NewStyle().Width(m.width).Render(line1))
		b.WriteByte('\n')
		b.WriteString(lipgloss.NewStyle().Width(m.width).Render(line2))
		if i < vis-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func (m *listModel) renderItem(r model.Repo, selected bool) (string, string) {
	w := m.width
	lang := abbreviateLang(r.Language)

	// ── Line 1: [select bar] [lang dot] name ........... ★ stars delta ──

	// Star count + delta (right-aligned)
	starText := fmt.Sprintf("★ %s", formatStars(r.Stars))
	var deltaText string
	if r.StarDelta > 0 {
		deltaText = styleSuccess.Render(fmt.Sprintf(" ▲+%d", r.StarDelta))
	} else if r.StarDelta < 0 {
		deltaText = styleError.Render(fmt.Sprintf(" ▼%d", r.StarDelta))
	}
	rightSide := styleStars.Render(starText) + deltaText

	// Select bar + prefix
	selectBar := " "
	if selected {
		if m.focused {
			selectBar = lipgloss.NewStyle().Foreground(colorAccentViolet).Render("▌")
		} else {
			selectBar = lipgloss.NewStyle().Foreground(colorFgGhost).Render("▌")
		}
	}

	// Status indicator
	prefix := " "
	if m.watchSet[r.FullName] {
		prefix = lipgloss.NewStyle().Foreground(colorRedAlert).Render("♥")
	} else if m.seenSet[r.FullName] {
		prefix = styleMuted.Render("·")
	}

	// Language dot
	langDot := styleLang.Foreground(langColor(r.Language)).Render("● " + lang)

	// Name — bold for selected
	leftParts := selectBar + prefix + langDot + " "
	leftW := lipgloss.Width(leftParts)
	rightW := lipgloss.Width(rightSide)
	nameW := w - leftW - rightW - 1
	if nameW < 8 {
		nameW = 8
	}
	name := r.FullName
	if len(name) > nameW {
		name = name[:nameW-1] + "…"
	}
	nameStyle := lipgloss.NewStyle().Foreground(colorFgPrimary)
	if selected {
		nameStyle = nameStyle.Bold(true)
	}

	// Compose line 1 with right-aligned stars
	nameRendered := nameStyle.Render(name)
	gap1 := w - leftW - lipgloss.Width(nameRendered) - rightW
	if gap1 < 1 {
		gap1 = 1
	}
	line1 := leftParts + nameRendered + strings.Repeat(" ", gap1) + rightSide

	// ── Line 2: [pad] age chip   ◈ score ▓▓▓░░   ⑂ forks ──

	pad := "   " // align under name
	agePart := formatAgeChip(r.PushedAt)
	scorePart := renderScoreBar(r.DiscoveryScore, 10)
	forkPart := styleMuted.Render(fmt.Sprintf("⑂ %s", formatStars(r.Forks)))

	line2Left := pad + agePart + "  " + scorePart
	line2LeftW := lipgloss.Width(line2Left)
	forkW := lipgloss.Width(forkPart)
	gap2 := w - line2LeftW - forkW
	if gap2 < 1 {
		gap2 = 1
	}
	line2 := line2Left + strings.Repeat(" ", gap2) + forkPart

	// Apply background for selected rows
	if selected {
		bg := colorBgElevated
		if !m.focused {
			bg = colorBgSurface
		}
		bgStyle := lipgloss.NewStyle().Background(bg).Width(w)
		return bgStyle.Render(line1), bgStyle.Render(line2)
	}
	return lipgloss.NewStyle().Width(w).Render(line1),
		lipgloss.NewStyle().Width(w).Render(line2)
}

// formatAgeChip returns a colored age string based on how recently the repo was pushed.
func formatAgeChip(pushedAt time.Time) string {
	if pushedAt.IsZero() {
		return styleMuted.Render("—")
	}
	d := time.Since(pushedAt)
	var text string
	switch {
	case d < 24*time.Hour:
		text = "today"
	case d < 7*24*time.Hour:
		text = fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		text = fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	case d < 365*24*time.Hour:
		text = fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	default:
		text = fmt.Sprintf("%dy", int(d.Hours()/(24*365)))
	}

	switch {
	case d < 7*24*time.Hour:
		return stylePulse.Render(text)
	case d < 30*24*time.Hour:
		return styleSuccess.Render(text)
	case d < 365*24*time.Hour:
		return styleSubtle.Render(text)
	default:
		return styleMuted.Render(text)
	}
}

// renderScoreBar renders a gradient flame meter: ◆ 7.3 ▓▓▓▓▓▓▓░░░
// Colors flow from cyan (cool) → gold (warm) → pulse (hot).
func renderScoreBar(score float64, cells int) string {
	normalized := score / 15.0
	if normalized > 1.0 {
		normalized = 1.0
	}
	filled := int(normalized * float64(cells))
	if filled > cells {
		filled = cells
	}

	// Gradient flame bar
	flameColors := [3]color.Color{colorAccentCyan, colorGoldStar, colorAccentPulse}
	var bar strings.Builder
	for i := 0; i < cells; i++ {
		if i < filled {
			pos := float64(i) / float64(cells)
			var c color.Color
			switch {
			case pos < 0.4:
				c = flameColors[0]
			case pos < 0.7:
				c = flameColors[1]
			default:
				c = flameColors[2]
			}
			bar.WriteString(lipgloss.NewStyle().Foreground(c).Render("▓"))
		} else {
			bar.WriteString(styleGhost.Render("░"))
		}
	}

	// Icon intensity matches score
	var icon string
	var iconStyle lipgloss.Style
	switch {
	case normalized > 0.7:
		icon = "◆"
		iconStyle = stylePulse
	case normalized > 0.4:
		icon = "◈"
		iconStyle = styleCyan
	default:
		icon = "◇"
		iconStyle = styleMuted
	}

	return iconStyle.Render(fmt.Sprintf("%s %.1f", icon, score)) + " " + bar.String()
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

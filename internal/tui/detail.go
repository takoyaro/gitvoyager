package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/model"
)

type detailTab int

const (
	tabOverview detailTab = iota
	tabReadme
)

type detailModel struct {
	viewport viewport.Model
	repo     *model.Repo
	readme   string
	loading  bool
	width    int
	height   int
	tab      detailTab
	focused  bool
}

func (m *detailModel) SetFocused(v bool) { m.focused = v }

func newDetailModel() detailModel {
	vp := viewport.New()
	return detailModel{viewport: vp}
}

func (m *detailModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(h - m.headerHeight())
	if m.repo != nil {
		m.updateContent()
	}
}

func (m *detailModel) headerHeight() int {
	return 8 // border(2) + stats(1) + name(1) + meter+license(1) + topics(1) + tab bar(1) + gap(1)
}

func (m *detailModel) SetRepo(r *model.Repo) {
	m.repo = r
	m.readme = ""
	m.loading = true
	m.tab = tabOverview
	m.viewport.GotoTop()
	m.updateContent()
}

func (m *detailModel) UpdateRepo(r *model.Repo) {
	m.repo = r
	m.updateContent()
}

func (m *detailModel) SetReadme(content string) {
	m.readme = content
	m.loading = false
	m.updateContent()
}

func (m *detailModel) SetLoading(v bool) {
	m.loading = v
	m.updateContent()
}

func (m *detailModel) SetTab(t detailTab) {
	m.tab = t
	m.viewport.GotoTop()
	m.updateContent()
}

func (m *detailModel) updateContent() {
	if m.repo == nil {
		m.viewport.SetContent(styleSubtle.Render("  Select a repo to view details"))
		return
	}

	switch m.tab {
	case tabOverview:
		m.viewport.SetContent(m.renderOverview())
	case tabReadme:
		m.viewport.SetContent(m.renderReadmeContent())
	}
}

func (m *detailModel) renderOverview() string {
	r := m.repo
	var lines []string

	// Star count + delta
	starLine := styleStars.Render(fmt.Sprintf("★ %s", formatStars(r.Stars)))
	if r.StarDelta > 0 {
		starLine += styleSuccess.Render(fmt.Sprintf("  ▲+%d since first seen", r.StarDelta))
	} else if r.StarDelta < 0 {
		starLine += styleError.Render(fmt.Sprintf("  ▼%d since first seen", r.StarDelta))
	}
	lines = append(lines, "  "+starLine)

	// Forks + issues
	forkIssue := styleMuted.Render(fmt.Sprintf("  ⑂ %s forks  ◉ %d open issues", formatStars(r.Forks), r.OpenIssues))
	lines = append(lines, forkIssue)

	// Enriched stats
	if r.Enriched {
		var eParts []string
		if r.WatcherCount > 0 {
			eParts = append(eParts, fmt.Sprintf("👁 %s watchers", formatStars(r.WatcherCount)))
		}
		if r.CommitCount > 0 {
			eParts = append(eParts, fmt.Sprintf("⚡ %d recent commits", r.CommitCount))
		}
		if len(eParts) > 0 {
			lines = append(lines, styleMuted.Render("  "+strings.Join(eParts, "  ")))
		}
	}

	lines = append(lines, "")

	// Dates
	created := r.CreatedAt.Format("2006-01-02")
	pushed := formatAgeChip(r.PushedAt)
	lines = append(lines, styleMuted.Render(fmt.Sprintf("  Created %s  ·  Updated %s", created, pushed)))

	// Score
	if r.DiscoveryScore > 0 {
		bar := renderScoreBar(r.DiscoveryScore, 10)
		lines = append(lines, "  "+bar)
	}

	lines = append(lines, "")

	// Description
	if r.Description != "" {
		desc := r.Description
		maxW := m.width - 4
		if len(desc) > maxW {
			desc = desc[:maxW-3] + "..."
		}
		lines = append(lines, stylePrimary.Render("  "+desc))
	}

	// Topics
	if len(r.Topics) > 0 {
		lines = append(lines, "")
		var tags []string
		for _, t := range r.Topics {
			tags = append(tags, styleTopicInline.Render("["+t+"]"))
		}
		lines = append(lines, "  "+strings.Join(tags, " "))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (m *detailModel) renderReadmeContent() string {
	if m.loading {
		return styleSubtle.Render("  Loading README...")
	}
	if m.readme != "" {
		return m.renderMarkdown(m.readme)
	}
	return styleSubtle.Render("  No README available")
}

func (m *detailModel) renderMarkdown(md string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(m.width-4),
	)
	if err != nil {
		return md
	}
	out, err := r.Render(md)
	if err != nil {
		return md
	}
	return out
}

func (m *detailModel) View() string {
	header := m.renderHeader()
	tabBar := m.renderTabBar()
	vpView := m.viewport.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, tabBar, vpView)
}

func (m *detailModel) renderHeader() string {
	if m.repo == nil {
		return strings.Repeat("\n", 5)
	}

	r := m.repo
	innerW := m.width - 4 // border padding

	// Row 1: language + stats
	langPart := ""
	if r.Language != "" {
		langPart = styleLang.Foreground(langColor(r.Language)).Render("● " + r.Language)
	}
	statParts := styleStars.Render(fmt.Sprintf("★ %s", formatStars(r.Stars)))
	statParts += styleMuted.Render(fmt.Sprintf("  ⑂ %s", formatStars(r.Forks)))
	row1Gap := innerW - lipgloss.Width(langPart) - lipgloss.Width(statParts)
	if row1Gap < 1 {
		row1Gap = 1
	}
	row1 := langPart + strings.Repeat(" ", row1Gap) + statParts

	// Row 2: owner/name
	owner := lipgloss.NewStyle().Italic(true).Foreground(colorFgMuted).Render(r.Owner + "/")
	name := lipgloss.NewStyle().Bold(true).Foreground(colorFgPrimary).Render(r.Name)
	row2 := owner + name

	// Row 3: star meter + score + license
	meter := renderStarMeter(r.StarPercentile)
	scorePart := styleCyan.Render(fmt.Sprintf("◈ score %.0f", r.DiscoveryScore))
	licPart := ""
	if r.License != "" {
		licPart = styleMuted.Render("◎ " + r.License)
	}
	row3Parts := []string{meter, scorePart}
	if licPart != "" {
		row3Parts = append(row3Parts, licPart)
	}
	row3 := strings.Join(row3Parts, "   ")

	// Row 4: topics
	row4 := ""
	if len(r.Topics) > 0 {
		var tags []string
		for _, t := range r.Topics {
			tags = append(tags, styleTopicInline.Render("["+t+"]"))
		}
		row4 = strings.Join(tags, " ")
		if lipgloss.Width(row4) > innerW {
			// Truncate topics to fit
			row4 = ""
			for _, t := range r.Topics {
				tag := styleTopicInline.Render("[" + t + "]")
				if lipgloss.Width(row4)+lipgloss.Width(tag)+1 > innerW {
					row4 += styleMuted.Render(" …")
					break
				}
				if row4 != "" {
					row4 += " "
				}
				row4 += tag
			}
		}
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, row1, row2, row3, row4)

	borderColor := colorFgGhost
	if m.focused {
		borderColor = colorAccentViolet
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(borderColor).
		Padding(0, 1).
		Width(m.width - 2).
		Render(inner)

	return box
}

func (m *detailModel) renderTabBar() string {
	tabs := []struct {
		label string
		tab   detailTab
	}{
		{"Overview", tabOverview},
		{"README", tabReadme},
	}

	activeStyle, inactiveStyle := styleTabActiveDimmed, styleTabInactiveDimmed
	if m.focused {
		activeStyle, inactiveStyle = styleTabActive, styleTabInactive
	}

	var parts []string
	for _, t := range tabs {
		if t.tab == m.tab {
			parts = append(parts, activeStyle.Render(" "+t.label+" "))
		} else {
			parts = append(parts, inactiveStyle.Render(" "+t.label+" "))
		}
	}

	// Tab indicator: shows 1/2 key hints when focused
	hint := ""
	if m.focused {
		hint = styleMuted.Render("  1/2")
	}

	return " " + strings.Join(parts, " ") + hint
}

// renderStarMeter returns a ★★★★★☆☆☆☆☆ string for the given percentile (0–10).
func renderStarMeter(percentile int) string {
	if percentile < 0 {
		percentile = 0
	}
	if percentile > 10 {
		percentile = 10
	}
	filled := styleStarFilled.Render(strings.Repeat("★", percentile))
	empty := styleStarEmpty.Render(strings.Repeat("☆", 10-percentile))
	return filled + empty
}

// formatRelativeTime returns a human-friendly relative time string.
func formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return fmt.Sprintf("%dd ago", days)
	}
}

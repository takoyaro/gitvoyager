package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/model"
)

type detailModel struct {
	viewport viewport.Model
	repo     *model.Repo
	readme   string
	loading  bool
	width    int
	height   int
}

func newDetailModel() detailModel {
	vp := viewport.New()
	return detailModel{
		viewport: vp,
	}
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
	return 6 // title + stats + enriched stats + topics + description + separator
}

func (m *detailModel) SetRepo(r *model.Repo) {
	m.repo = r
	m.readme = ""
	m.loading = true
	m.viewport.GotoTop()
	m.updateContent()
}

// UpdateRepo refreshes the repo pointer (e.g. after enrichment) without
// resetting the README or scroll position.
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

func (m *detailModel) updateContent() {
	if m.repo == nil {
		m.viewport.SetContent(styleSubtle.Render("  Select a repo to view details"))
		return
	}

	var content strings.Builder

	if m.loading {
		content.WriteString(styleSubtle.Render("  Loading README..."))
	} else if m.readme != "" {
		rendered := m.renderMarkdown(m.readme)
		content.WriteString(rendered)
	} else {
		content.WriteString(styleSubtle.Render("  No README available"))
	}

	m.viewport.SetContent(content.String())
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
	vpView := m.viewport.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, vpView)
}

func (m *detailModel) renderHeader() string {
	if m.repo == nil {
		return strings.Repeat("\n", m.headerHeight()-1)
	}

	r := m.repo

	// Title
	title := styleDetailTitle.Render(r.FullName)

	// Stats line
	stars := styleStars.Render(fmt.Sprintf("★ %s", formatStars(r.Stars)))
	forks := styleSubtle.Render(fmt.Sprintf("⑂ %s", formatStars(r.Forks)))
	issues := styleSubtle.Render(fmt.Sprintf("◉ %d", r.OpenIssues))
	lang := ""
	if r.Language != "" {
		lang = styleLang.Foreground(langColor(r.Language)).Render(" ● " + r.Language)
	}
	lic := ""
	if r.License != "" {
		lic = styleSubtle.Render(" [" + r.License + "]")
	}
	stats := styleDetailMeta.Render(
		fmt.Sprintf("%s  %s  %s%s%s", stars, forks, issues, lang, lic),
	)

	// Enriched stats (watchers, commits, score)
	enrichedStats := ""
	if r.Enriched {
		parts := []string{}
		if r.WatcherCount > 0 {
			parts = append(parts, fmt.Sprintf("👁 %s watchers", formatStars(r.WatcherCount)))
		}
		if r.CommitCount > 0 {
			parts = append(parts, fmt.Sprintf("⚡ %d recent commits", r.CommitCount))
		}
		if r.DiscoveryScore > 0 {
			parts = append(parts, fmt.Sprintf("◈ score %.1f", r.DiscoveryScore))
		}
		if len(parts) > 0 {
			enrichedStats = styleDetailMeta.Render(strings.Join(parts, "  "))
		}
	}
	if enrichedStats == "" {
		enrichedStats = " " // keep layout stable
	}

	// Topics
	topics := ""
	if len(r.Topics) > 0 {
		tags := make([]string, len(r.Topics))
		for i, t := range r.Topics {
			tags[i] = styleTopic.Render("[" + t + "]")
		}
		topics = styleDetailMeta.Render(strings.Join(tags, " "))
	}

	// Description
	desc := ""
	if r.Description != "" {
		d := r.Description
		if len(d) > m.width-4 {
			d = d[:m.width-7] + "..."
		}
		desc = styleDetailMeta.Render(d)
	}

	sep := styleSubtle.Render(strings.Repeat("─", m.width-2))

	return lipgloss.JoinVertical(lipgloss.Left, title, stats, enrichedStats, topics, desc, sep)
}

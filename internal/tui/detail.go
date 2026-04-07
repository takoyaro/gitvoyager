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

type detailModel struct {
	viewport viewport.Model
	repo     *model.Repo
	readme   string
	loading  bool
	width    int
	height   int
	focused  bool

	// Claude AI content (set by app, read by view)
	aiSummary    string
	aiAnalysis   string
	summarizing  bool
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
	return 7 // border(2) + stats(1) + name(1) + meter+license(1) + topics(1) + gap(1)
}

func (m *detailModel) SetRepo(r *model.Repo) {
	m.repo = r
	m.readme = ""
	m.loading = true
	m.aiSummary = ""
	m.aiAnalysis = ""
	m.summarizing = false
	m.viewport.GotoTop()
	m.updateContent()
}

func (m *detailModel) SetAISummary(summary string) {
	m.aiSummary = summary
	m.summarizing = false
	m.updateContent()
}

func (m *detailModel) SetAIAnalysis(analysis string) {
	m.aiAnalysis = analysis
	m.updateContent()
}

func (m *detailModel) SetSummarizing(v bool) {
	m.summarizing = v
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

func (m *detailModel) updateContent() {
	if m.repo == nil {
		m.viewport.SetContent(styleSubtle.Render("  Select a repo to view details"))
		return
	}
	m.viewport.SetContent(m.renderCombinedView())
}

func (m *detailModel) renderCombinedView() string {
	r := m.repo
	var sections []string

	// -- Preamble: metadata not already in header --
	var meta []string

	// Description
	if r.Description != "" {
		desc := r.Description
		maxW := m.width - 4
		if len(desc) > maxW {
			desc = desc[:maxW-3] + "..."
		}
		meta = append(meta, stylePrimary.Render("  "+desc))
		meta = append(meta, "")
	}

	// Star delta (header shows count, not delta)
	if r.StarDelta > 0 {
		meta = append(meta, "  "+styleSuccess.Render(fmt.Sprintf("▲ +%d stars since first seen", r.StarDelta)))
	} else if r.StarDelta < 0 {
		meta = append(meta, "  "+styleError.Render(fmt.Sprintf("▼ %d stars since first seen", r.StarDelta)))
	}

	// Open issues
	if r.OpenIssues > 0 {
		meta = append(meta, styleMuted.Render(fmt.Sprintf("  ◉ %d open issues", r.OpenIssues)))
	}

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
			meta = append(meta, styleMuted.Render("  "+strings.Join(eParts, "  ")))
		}
	}

	// Dates
	created := r.CreatedAt.Format("2006-01-02")
	pushed := formatAgeChip(r.PushedAt)
	meta = append(meta, styleMuted.Render(fmt.Sprintf("  Created %s  ·  Updated %s", created, pushed)))

	// Score bar
	if r.DiscoveryScore > 0 {
		bar := renderScoreBar(r.DiscoveryScore, 10)
		meta = append(meta, "  "+bar)
	}

	if len(meta) > 0 {
		sections = append(sections, lipgloss.JoinVertical(lipgloss.Left, meta...))
	}

	// -- AI Summary --
	if m.aiSummary != "" {
		aiLabel := styleCyan.Render("  ◎ AI Summary")
		aiText := stylePrimary.Render("  " + m.aiSummary)
		sections = append(sections, "", aiLabel, aiText)
	} else if m.summarizing {
		sections = append(sections, "", styleCyan.Render("  ◎ Summarizing..."))
	}

	// -- AI Analysis (Why Trending) --
	if m.aiAnalysis != "" {
		aiLabel := stylePulse.Render("  ⚡ Why Trending")
		aiText := stylePrimary.Render("  " + m.aiAnalysis)
		sections = append(sections, "", aiLabel, aiText)
	}

	// -- Separator --
	sepW := m.width - 6
	if sepW < 4 {
		sepW = 4
	}
	sections = append(sections, styleMuted.Render("  "+strings.Repeat("─", sepW)))

	// -- README --
	sections = append(sections, m.renderReadmeContent())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
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
	vpView := m.viewport.View()
	return lipgloss.JoinVertical(lipgloss.Left, header, vpView)
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

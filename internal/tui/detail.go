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

	// Animation state
	summarizeDots    int  // 0-2: cycles ".", "..", "..."
	focusPulseActive bool // true during focus transition pulse
}

func (m *detailModel) SetFocused(v bool) { m.focused = v }

// contentWidth returns the max width for text content, capped for readability.
func (m *detailModel) contentWidth() int {
	return min(m.width, 120)
}

func newDetailModel() detailModel {
	vp := viewport.New()
	return detailModel{viewport: vp}
}

func (m *detailModel) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.SetWidth(w)
	m.viewport.SetHeight(max(1, h-m.headerHeight()))
	if m.repo != nil {
		m.updateContent()
	}
}

func (m *detailModel) headerHeight() int {
	if m.height < 20 {
		return 4 // compact: border(2) + name+stats(1) + gap(1)
	}
	return 5 // full: border(2) + name+lang+stats(1) + meter+license(1) + gap(1), topics optional
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
		empty := lipgloss.JoinVertical(lipgloss.Center,
			"",
			styleCyan.Render("◈")+lipgloss.NewStyle().Bold(true).Foreground(colorFgPrimary).Render("  Select a repo"),
			"",
			styleMuted.Render("j/k to navigate the list"),
			styleMuted.Render("enter or l to view details"),
			styleMuted.Render("space to quick peek"),
		)
		m.viewport.SetContent(lipgloss.Place(m.width, max(1, m.height-m.headerHeight()), lipgloss.Center, lipgloss.Center, empty))
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
		maxW := m.contentWidth() - 4
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
		dots := strings.Repeat(".", m.summarizeDots+1)
		sections = append(sections, "", styleCyan.Render("  ◎ Summarizing"+dots))
	}

	// -- AI Analysis (Why Trending) --
	if m.aiAnalysis != "" {
		aiLabel := stylePulse.Render("  ⚡ Why Trending")
		aiText := stylePrimary.Render("  " + m.aiAnalysis)
		sections = append(sections, "", aiLabel, aiText)
	}

	// -- Separator --
	sepW := m.contentWidth() - 6
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
		return styleCyan.Render("  ◎ ") + stylePrimary.Render("Loading README...")
	}
	if m.readme != "" {
		return m.renderMarkdown(m.readme)
	}
	return styleMuted.Render("  ──") + "\n\n" +
		styleCyan.Render("  ◇") + stylePrimary.Render(" No README available") + "\n\n" +
		styleMuted.Render("  This repo has no README file.\n") +
		styleMuted.Render("  Press ") + styleAccent.Render("o") + styleMuted.Render(" to view on GitHub.")
}

func (m *detailModel) renderMarkdown(md string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath("dark"),
		glamour.WithWordWrap(m.contentWidth()-4),
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
		if m.height < 20 {
			return strings.Repeat("\n", 2)
		}
		return strings.Repeat("\n", 5)
	}

	r := m.repo
	innerW := m.width - 4 // border padding

	borderColor := colorFgGhost
	if m.focused {
		if m.focusPulseActive {
			borderColor = colorAccentPulse // brighter flash during pulse
		} else {
			borderColor = colorAccentCyan
		}
	}

	if m.height < 20 {
		// Compact header: single row with name + stats
		owner := lipgloss.NewStyle().Italic(true).Foreground(colorFgMuted).Render(r.Owner + "/")
		name := GradientText(r.Name, "#22D3EE", "#818CF8")
		namePart := owner + name

		statParts := styleStars.Render(fmt.Sprintf("★ %s", formatStars(r.Stars)))
		if r.Language != "" {
			statParts = styleLang.Foreground(langColor(r.Language)).Render("● "+r.Language) + "  " + statParts
		}

		gap := innerW - lipgloss.Width(namePart) - lipgloss.Width(statParts)
		if gap < 1 {
			gap = 1
		}
		row := namePart + strings.Repeat(" ", gap) + statParts

		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(borderColor).
			Padding(0, 1).
			Width(m.width - 2).
			Render(row)
		return box
	}

	// Full header: 3 content rows (merged lang+name+stats, meter+license, topics)
	// Row 1: [● lang] owner/name ........... ★ stars  ⑂ forks
	langPart := ""
	if r.Language != "" {
		langPart = styleLang.Foreground(langColor(r.Language)).Render("● "+r.Language) + " "
	}
	owner := lipgloss.NewStyle().Italic(true).Foreground(colorFgMuted).Render(r.Owner + "/")
	name := GradientText(r.Name, "#22D3EE", "#818CF8")
	namePart := langPart + owner + name

	statParts := styleStars.Render(fmt.Sprintf("★ %s", formatStars(r.Stars)))
	statParts += styleMuted.Render(fmt.Sprintf("  ⑂ %s", formatStars(r.Forks)))
	row1Gap := innerW - lipgloss.Width(namePart) - lipgloss.Width(statParts)
	if row1Gap < 1 {
		row1Gap = 1
	}
	row1 := namePart + strings.Repeat(" ", row1Gap) + statParts

	// Row 2: star meter + score + license
	meter := renderStarMeter(r.StarPercentile)
	scorePart := styleCyan.Render(fmt.Sprintf("◈ score %.0f", r.DiscoveryScore))
	licPart := ""
	if r.License != "" {
		licPart = styleMuted.Render("◎ " + r.License)
	}
	row2Parts := []string{meter, scorePart}
	if licPart != "" {
		row2Parts = append(row2Parts, licPart)
	}
	row2 := strings.Join(row2Parts, "   ")

	// Row 3: topics (only if present)
	rows := []string{row1, row2}
	if len(r.Topics) > 0 {
		var tags []string
		for _, t := range r.Topics {
			tags = append(tags, styleTopicInline.Render("["+t+"]"))
		}
		topicRow := strings.Join(tags, " ")
		if lipgloss.Width(topicRow) > innerW {
			topicRow = ""
			for _, t := range r.Topics {
				tag := styleTopicInline.Render("[" + t + "]")
				if lipgloss.Width(topicRow)+lipgloss.Width(tag)+1 > innerW {
					topicRow += styleMuted.Render(" …")
					break
				}
				if topicRow != "" {
					topicRow += " "
				}
				topicRow += tag
			}
		}
		rows = append(rows, topicRow)
	}

	inner := lipgloss.JoinVertical(lipgloss.Left, rows...)

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

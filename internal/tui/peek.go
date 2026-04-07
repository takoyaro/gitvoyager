package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/model"
)

type peekModel struct {
	active bool
	repo   *model.Repo
	width  int
	height int
}

func (m *peekModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *peekModel) Show(r *model.Repo) {
	m.repo = r
	m.active = true
}

func (m *peekModel) Hide() {
	m.active = false
}

func (m *peekModel) IsActive() bool {
	return m.active
}

func (m *peekModel) View() string {
	if m.repo == nil {
		return ""
	}

	r := m.repo
	innerW := m.width * 55 / 100
	if innerW < 40 {
		innerW = 40
	}
	if innerW > m.width-4 {
		innerW = m.width - 4
	}

	var lines []string

	// Title
	owner := lipgloss.NewStyle().Italic(true).Foreground(colorFgMuted).Render(r.Owner + "/")
	name := lipgloss.NewStyle().Bold(true).Foreground(colorFgPrimary).Render(r.Name)
	lines = append(lines, owner+name)
	lines = append(lines, "")

	// Stats
	starLine := styleStars.Render(fmt.Sprintf("★ %s", formatStars(r.Stars)))
	if r.StarDelta > 0 {
		starLine += styleSuccess.Render(fmt.Sprintf("  ▲+%d", r.StarDelta))
	} else if r.StarDelta < 0 {
		starLine += styleError.Render(fmt.Sprintf("  ▼%d", r.StarDelta))
	}
	lines = append(lines, starLine)

	forkLine := styleMuted.Render(fmt.Sprintf("⑂ %s forks  ◉ %d issues", formatStars(r.Forks), r.OpenIssues))
	lines = append(lines, forkLine)

	// Language + license
	metaParts := []string{}
	if r.Language != "" {
		metaParts = append(metaParts, styleLang.Foreground(langColor(r.Language)).Render("● "+r.Language))
	}
	if r.License != "" {
		metaParts = append(metaParts, styleMuted.Render("◎ "+r.License))
	}
	if len(metaParts) > 0 {
		lines = append(lines, strings.Join(metaParts, "  "))
	}

	// Age
	pushed := formatAgeChip(r.PushedAt)
	lines = append(lines, styleMuted.Render("Updated ")+pushed)

	// Star meter + score
	meter := renderStarMeter(r.StarPercentile)
	score := styleCyan.Render(fmt.Sprintf("  ◈ %.1f", r.DiscoveryScore))
	lines = append(lines, meter+score)

	lines = append(lines, "")

	// Description
	if r.Description != "" {
		desc := r.Description
		if len(desc) > innerW-2 {
			desc = desc[:innerW-5] + "..."
		}
		lines = append(lines, stylePrimary.Render(desc))
	}

	// Topics
	if len(r.Topics) > 0 {
		lines = append(lines, "")
		var tags []string
		for _, t := range r.Topics {
			tags = append(tags, styleTopicInline.Render("["+t+"]"))
		}
		topicLine := strings.Join(tags, " ")
		lines = append(lines, topicLine)
	}

	// Hint
	lines = append(lines, "")
	lines = append(lines, styleMuted.Render("space/q/esc: close  enter: full detail  w: watch  o: open"))

	inner := lipgloss.JoinVertical(lipgloss.Left, lines...)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentViolet).
		Padding(1, 2).
		Width(innerW).
		Render(inner)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

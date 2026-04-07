package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/model"
)

type compareModel struct {
	left   *model.Repo
	right  *model.Repo
	active bool
	width  int
	height int
}

func (m *compareModel) SetSize(w, h int) {
	m.width = w
	m.height = h
}

func (m *compareModel) MarkLeft(r *model.Repo) {
	m.left = r
}

func (m *compareModel) Show(r *model.Repo) {
	m.right = r
	m.active = true
}

func (m *compareModel) Hide() {
	m.active = false
	m.left = nil
	m.right = nil
}

func (m *compareModel) IsActive() bool {
	return m.active
}

func (m *compareModel) HasLeft() bool {
	return m.left != nil
}

func (m *compareModel) View() string {
	if m.left == nil || m.right == nil {
		return ""
	}

	innerW := m.width - 6
	if innerW < 60 {
		innerW = 60
	}
	if innerW > m.width-4 {
		innerW = m.width - 4
	}
	colW := (innerW - 5) / 2 // 5 = divider + padding

	leftCol := m.renderRepoColumn(m.left, colW)
	rightCol := m.renderRepoColumn(m.right, colW)

	divider := lipgloss.NewStyle().
		Foreground(colorFgGhost).
		Render(strings.Repeat("│\n", strings.Count(leftCol, "\n")+1))

	content := lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(colW).Render(leftCol),
		"  "+divider+"  ",
		lipgloss.NewStyle().Width(colW).Render(rightCol),
	)

	title := stylePulse.Render("  ◎ Side-by-Side Comparison")
	hint := styleMuted.Render("  esc/q: close  C: clear")

	inner := lipgloss.JoinVertical(lipgloss.Left, title, "", content, "", hint)

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorAccentViolet).
		Padding(1, 2).
		Width(innerW).
		Render(inner)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m *compareModel) renderRepoColumn(r *model.Repo, w int) string {
	var lines []string

	// Name
	owner := lipgloss.NewStyle().Italic(true).Foreground(colorFgMuted).Render(r.Owner + "/")
	name := lipgloss.NewStyle().Bold(true).Foreground(colorFgPrimary).Render(r.Name)
	lines = append(lines, owner+name)
	lines = append(lines, "")

	// Stats
	lines = append(lines, styleStars.Render(fmt.Sprintf("★ %s", formatStars(r.Stars)))+
		styleMuted.Render(fmt.Sprintf("  ⑂ %s", formatStars(r.Forks))))

	// Language + License
	if r.Language != "" {
		lines = append(lines, styleLang.Foreground(langColor(r.Language)).Render("● "+r.Language))
	}
	if r.License != "" {
		lines = append(lines, styleMuted.Render("◎ "+r.License))
	}

	// Age
	lines = append(lines, styleMuted.Render("Updated ")+formatAgeChip(r.PushedAt))
	lines = append(lines, styleMuted.Render("Created ")+styleMuted.Render(r.CreatedAt.Format("2006-01-02")))

	// Score
	lines = append(lines, renderScoreBar(r.DiscoveryScore, 8))

	// Issues
	if r.OpenIssues > 0 {
		lines = append(lines, styleMuted.Render(fmt.Sprintf("◉ %d issues", r.OpenIssues)))
	}

	// Description
	if r.Description != "" {
		desc := r.Description
		if len(desc) > w-2 {
			desc = desc[:w-5] + "..."
		}
		lines = append(lines, "", stylePrimary.Render(desc))
	}

	// Topics
	if len(r.Topics) > 0 {
		var tags []string
		totalW := 0
		for _, t := range r.Topics {
			tag := "[" + t + "]"
			if totalW+len(tag)+1 > w-2 {
				tags = append(tags, styleMuted.Render("…"))
				break
			}
			tags = append(tags, styleTopicInline.Render(tag))
			totalW += len(tag) + 1
		}
		lines = append(lines, strings.Join(tags, " "))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

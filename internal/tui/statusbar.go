package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/github"
)

type statusBarModel struct {
	width     int
	message   string
	isError   bool
	rateLimit github.RateLimit
	repoCount int
	filtered  int
}

func newStatusBar() statusBarModel {
	return statusBarModel{}
}

func (m *statusBarModel) SetWidth(w int) {
	m.width = w
}

func (m *statusBarModel) SetMessage(msg string, isError bool) {
	m.message = msg
	m.isError = isError
}

func (m *statusBarModel) ClearMessage() {
	m.message = ""
	m.isError = false
}

func (m *statusBarModel) SetRateLimit(rl github.RateLimit) {
	m.rateLimit = rl
}

func (m *statusBarModel) SetCounts(total, filtered int) {
	m.repoCount = total
	m.filtered = filtered
}

func (m *statusBarModel) View() string {
	// Left side: message or hints
	left := ""
	if m.message != "" {
		if m.isError {
			left = styleError.Render(" " + m.message)
		} else {
			left = styleSuccess.Render(" " + m.message)
		}
	} else {
		left = styleSubtle.Render(" j/k:nav  /:search  s:sort  o:open  c:clone  ?:help")
	}

	// Right side: rate limit + count
	right := ""
	parts := []string{}

	if m.repoCount > 0 {
		if m.filtered < m.repoCount {
			parts = append(parts, fmt.Sprintf("%d/%d repos", m.filtered, m.repoCount))
		} else {
			parts = append(parts, fmt.Sprintf("%d repos", m.repoCount))
		}
	}

	if m.rateLimit.SearchLimit > 0 {
		rlColor := colorSuccess
		remaining := m.rateLimit.SearchRemaining
		if remaining <= 5 {
			rlColor = colorError
		} else if remaining <= 15 {
			rlColor = colorStar
		}
		rlStyle := lipgloss.NewStyle().Foreground(rlColor)
		resetIn := time.Until(m.rateLimit.SearchReset).Round(time.Second)
		if resetIn < 0 {
			resetIn = 0
		}
		parts = append(parts, rlStyle.Render(
			fmt.Sprintf("API: %d/%d (reset %s)", remaining, m.rateLimit.SearchLimit, resetIn),
		))
	}

	if len(parts) > 0 {
		right = strings.Join(parts, "  ")
	}

	// Pad between left and right
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if gap < 1 {
		gap = 1
	}

	return styleStatusBar.Width(m.width).Render(
		left + strings.Repeat(" ", gap) + right,
	)
}

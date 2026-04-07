package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/takoyaro/gitvoyager/internal/github"
)

type statusBarModel struct {
	width      int
	message    string
	isError    bool
	rateLimit  github.RateLimit
	repoCount  int
	filtered   int
	loading    bool
	spinFrame  int
	focusLabel string
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func newStatusBar() statusBarModel {
	return statusBarModel{}
}

func (m *statusBarModel) SetWidth(w int)                      { m.width = w }
func (m *statusBarModel) SetMessage(msg string, isError bool)  { m.message = msg; m.isError = isError }
func (m *statusBarModel) ClearMessage()                        { m.message = ""; m.isError = false }
func (m *statusBarModel) SetRateLimit(rl github.RateLimit)     { m.rateLimit = rl }
func (m *statusBarModel) SetLoading(v bool)                    { m.loading = v }
func (m *statusBarModel) SetFocusLabel(label string)           { m.focusLabel = label }

func (m *statusBarModel) SetCounts(total, filtered int) {
	m.repoCount = total
	m.filtered = filtered
}

func (m *statusBarModel) Tick() {
	m.spinFrame = (m.spinFrame + 1) % len(spinnerFrames)
}

func (m *statusBarModel) View() string {
	sep := styleMuted.Render(" │ ")

	// ── Zone 1: state dot + message ──
	var zone1 string
	if m.message != "" {
		if m.isError {
			zone1 = lipgloss.NewStyle().Foreground(colorRedAlert).Render("● ") +
				styleError.Render(m.message)
		} else if m.loading {
			zone1 = styleCyan.Render(spinnerFrames[m.spinFrame]+" ") +
				styleCyan.Render(m.message)
		} else {
			zone1 = lipgloss.NewStyle().Foreground(colorGreenGrow).Render("● ") +
				styleSuccess.Render(m.message)
		}
	} else if m.loading {
		zone1 = styleCyan.Render(spinnerFrames[m.spinFrame]+" ") +
			styleCyan.Render("searching…")
	} else {
		zone1 = lipgloss.NewStyle().Foreground(colorGreenGrow).Render("● ") +
			styleMuted.Render("j/k:nav  tab:pane  /:search  s:sort  o:open  c:clone  ?:help")
	}

	// ── Zone 2: repo count ──
	var zone2 string
	if m.repoCount > 0 {
		icon := stylePulse.Render("⚡ ")
		if m.filtered < m.repoCount {
			zone2 = icon + stylePrimary.Render(fmt.Sprintf("%d/%d", m.filtered, m.repoCount))
		} else {
			zone2 = icon + stylePrimary.Render(fmt.Sprintf("%d", m.repoCount))
		}
	}

	// ── Zone 3: rate limit bar ──
	var zone3 string
	if m.rateLimit.SearchLimit > 0 {
		remaining := m.rateLimit.SearchRemaining
		limit := m.rateLimit.SearchLimit

		// 10-cell bar
		filled := 10
		if limit > 0 {
			filled = remaining * 10 / limit
		}
		if filled > 10 {
			filled = 10
		}

		barColor := colorGreenGrow
		if remaining <= 5 {
			barColor = colorRedAlert
		} else if remaining <= 15 {
			barColor = colorGoldStar
		}

		bar := lipgloss.NewStyle().Foreground(barColor).Render(strings.Repeat("█", filled)) +
			styleGhost.Render(strings.Repeat("░", 10-filled))

		resetIn := time.Until(m.rateLimit.SearchReset).Round(time.Second)
		if resetIn < 0 {
			resetIn = 0
		}

		zone3 = styleMuted.Render("API ") + bar +
			styleMuted.Render(fmt.Sprintf(" %d/%d ", remaining, limit)) +
			styleMuted.Render(fmt.Sprintf("resets %s", formatDuration(resetIn)))
	}

	// ── Zone 0: focus breadcrumb ──
	var zone0 string
	if m.focusLabel != "" {
		zone0 = styleFocusPill.Render(m.focusLabel)
	}

	// ── Compose bar ──
	var parts []string
	if zone0 != "" {
		parts = append(parts, zone0)
	}
	parts = append(parts, zone1)
	if zone2 != "" {
		parts = append(parts, zone2)
	}
	if zone3 != "" {
		parts = append(parts, zone3)
	}

	left := strings.Join(parts, sep)
	leftW := lipgloss.Width(left)

	gap := m.width - leftW - 1
	if gap < 0 {
		gap = 0
	}

	return lipgloss.NewStyle().
		PaddingLeft(1).
		Width(m.width).
		Render(left + strings.Repeat(" ", gap))
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "now"
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

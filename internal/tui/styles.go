package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Colors -- lipgloss v2 Color() returns color.Color
var (
	colorAccent    = lipgloss.Color("#7C3AED") // purple
	colorSubtle    = lipgloss.Color("#6B7280") // gray
	colorStar      = lipgloss.Color("#EAB308") // gold
	colorError     = lipgloss.Color("#EF4444") // red
	colorSuccess   = lipgloss.Color("#22C55E") // green
	colorBorder    = lipgloss.Color("#374151") // dark gray
	colorHighlight = lipgloss.Color("#1E1B4B") // deep purple bg
	colorMuted     = lipgloss.Color("#4B5563") // dimmed

	// Language colors matching GitHub conventions
	colorGo         = lipgloss.Color("#00ADD8")
	colorPython     = lipgloss.Color("#3572A5")
	colorRust       = lipgloss.Color("#DEA584")
	colorTypeScript = lipgloss.Color("#3178C6")
	colorJavaScript = lipgloss.Color("#F1E05A")
	colorJava       = lipgloss.Color("#B07219")
	colorCpp        = lipgloss.Color("#F34B7D")
	colorRuby       = lipgloss.Color("#701516")
	colorShell      = lipgloss.Color("#89E051")
)

var langColors = map[string]color.Color{
	"Go":         colorGo,
	"Python":     colorPython,
	"Rust":       colorRust,
	"TypeScript": colorTypeScript,
	"JavaScript": colorJavaScript,
	"Java":       colorJava,
	"C++":        colorCpp,
	"Ruby":       colorRuby,
	"Shell":      colorShell,
}

func langColor(lang string) color.Color {
	if c, ok := langColors[lang]; ok {
		return c
	}
	return colorSubtle
}

// Layout styles
var (
	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	styleListItem = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1)

	styleListItemSelected = lipgloss.NewStyle().
				PaddingLeft(1).
				PaddingRight(1).
				Background(colorHighlight).
				Bold(true)

	styleStars = lipgloss.NewStyle().
			Foreground(colorStar)

	styleLang = lipgloss.NewStyle()

	styleSubtle = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleError = lipgloss.NewStyle().
			Foreground(colorError)

	styleSuccess = lipgloss.NewStyle().
			Foreground(colorSuccess)

	styleAccent = lipgloss.NewStyle().
			Foreground(colorAccent)

	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorSubtle).
			PaddingLeft(1).
			PaddingRight(1)

	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent).
			PaddingLeft(1)

	styleDetailTitle = lipgloss.NewStyle().
				Bold(true).
				PaddingLeft(1)

	styleDetailMeta = lipgloss.NewStyle().
			Foreground(colorSubtle).
			PaddingLeft(1)

	styleTopic = lipgloss.NewStyle().
			Foreground(colorAccent).
			PaddingLeft(0).
			PaddingRight(0)

	styleSearchPrompt = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)
)

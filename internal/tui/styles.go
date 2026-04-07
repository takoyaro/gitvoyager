package tui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// ── Base palette ───────────────────────────────────────────────────────
// Kanagawa / Tokyo Night inspired, dual-accent (violet + cyan)
var (
	// Backgrounds
	colorBgVoid    = lipgloss.Color("#0D0F17") // deepest, blue-tinted
	colorBgSurface = lipgloss.Color("#111827") // panels
	colorBgElevated = lipgloss.Color("#1A1F2E") // selected, active

	// Foreground
	colorFgPrimary   = lipgloss.Color("#E2E8F0") // main text
	colorFgSecondary = lipgloss.Color("#94A3B8") // metadata
	colorFgMuted     = lipgloss.Color("#4B5563") // timestamps, hints
	colorFgGhost     = lipgloss.Color("#2D3748") // structure, skeleton

	// Accents
	colorAccentViolet = lipgloss.Color("#818CF8") // primary: focus, selected
	colorAccentCyan   = lipgloss.Color("#22D3EE") // secondary: data, links
	colorAccentPulse  = lipgloss.Color("#C084FC") // trending, hot

	// Semantic
	colorGoldStar   = lipgloss.Color("#F59E0B") // star icon
	colorGreenGrow  = lipgloss.Color("#34D399") // positive delta, ready
	colorRedAlert   = lipgloss.Color("#F87171") // negative delta, error

	// Border
	colorBorder = lipgloss.Color("#1E293B") // subtle panel borders

	// Language colors (GitHub conventions)
	colorGo         = lipgloss.Color("#00ADD8")
	colorPython     = lipgloss.Color("#3B82F6")
	colorRust       = lipgloss.Color("#F97316")
	colorTypeScript = lipgloss.Color("#3178C6")
	colorJavaScript = lipgloss.Color("#EAB308")
	colorJava       = lipgloss.Color("#007396")
	colorCpp        = lipgloss.Color("#00589D")
	colorRuby       = lipgloss.Color("#CC342D")
	colorShell      = lipgloss.Color("#4EAA25")
	colorSwift      = lipgloss.Color("#FA7343")
	colorKotlin     = lipgloss.Color("#7F52FF")
	colorZig        = lipgloss.Color("#F7A41D")
	colorElixir     = lipgloss.Color("#6E4A7E")
	colorHaskell    = lipgloss.Color("#5D4F85")
	colorClojure    = lipgloss.Color("#5881D8")
	colorCSharp     = lipgloss.Color("#68217A")
	colorDart       = lipgloss.Color("#00B4AB")
	colorLua        = lipgloss.Color("#000080")
	colorScala      = lipgloss.Color("#DC322F")
	colorNix        = lipgloss.Color("#5277C3")
)

var langColors = map[string]color.Color{
	"Go":         colorGo,
	"Python":     colorPython,
	"Rust":       colorRust,
	"TypeScript": colorTypeScript,
	"JavaScript": colorJavaScript,
	"Java":       colorJava,
	"C++":        colorCpp,
	"C#":         colorCSharp,
	"Ruby":       colorRuby,
	"Shell":      colorShell,
	"Swift":      colorSwift,
	"Kotlin":     colorKotlin,
	"Zig":        colorZig,
	"Elixir":     colorElixir,
	"Haskell":    colorHaskell,
	"Clojure":    colorClojure,
	"Dart":       colorDart,
	"Lua":        colorLua,
	"Scala":      colorScala,
	"Nix":        colorNix,
}

func langColor(lang string) color.Color {
	if c, ok := langColors[lang]; ok {
		return c
	}
	return colorFgMuted
}

// ── Backward-compatible aliases ───────────────────────────────────────
// (used throughout the codebase — point them at the new palette)
var (
	colorAccent    = colorAccentViolet
	colorSubtle    = colorFgSecondary
	colorStar      = colorGoldStar
	colorError     = colorRedAlert
	colorSuccess   = colorGreenGrow
	colorHighlight = colorBgElevated
	colorMuted     = colorFgMuted
)

// ── Layout styles ─────────────────────────────────────────────────────

var (
	// List items — default row
	styleListItem = lipgloss.NewStyle().
			PaddingLeft(1).
			PaddingRight(1)

	// List items — selected: left accent bar + elevated bg
	styleListItemSelected = lipgloss.NewStyle().
				PaddingRight(1).
				Background(colorBgElevated).
				Bold(true)

	// Stars
	styleStars = lipgloss.NewStyle().
			Foreground(colorGoldStar)

	// Language dot
	styleLang = lipgloss.NewStyle()

	// Subtle / secondary text
	styleSubtle = lipgloss.NewStyle().
			Foreground(colorFgSecondary)

	// Error messages
	styleError = lipgloss.NewStyle().
			Foreground(colorRedAlert)

	// Success messages
	styleSuccess = lipgloss.NewStyle().
			Foreground(colorGreenGrow)

	// Primary accent
	styleAccent = lipgloss.NewStyle().
			Foreground(colorAccentViolet)

	// Cyan accent — for data, counts
	styleCyan = lipgloss.NewStyle().
			Foreground(colorAccentCyan)

	// Pulse accent — for trending, hot
	stylePulse = lipgloss.NewStyle().
			Foreground(colorAccentPulse)

	// Ghost — barely-visible structure
	styleGhost = lipgloss.NewStyle().
			Foreground(colorFgGhost)

	// Muted — timestamps, dim text
	styleMuted = lipgloss.NewStyle().
			Foreground(colorFgMuted)

	// Primary text
	stylePrimary = lipgloss.NewStyle().
			Foreground(colorFgPrimary)

	// Status bar
	styleStatusBar = lipgloss.NewStyle().
			Foreground(colorFgSecondary).
			PaddingLeft(1).
			PaddingRight(1)

	// Header / titles
	styleHeader = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccentViolet).
			PaddingLeft(1)

	// Detail pane title
	styleDetailTitle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorFgPrimary).
				PaddingLeft(1)

	// Detail pane metadata
	styleDetailMeta = lipgloss.NewStyle().
			Foreground(colorFgSecondary).
			PaddingLeft(1)

	// Topic pills
	styleTopic = lipgloss.NewStyle().
			Foreground(colorAccentPulse).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorAccentViolet).
			PaddingLeft(1).PaddingRight(1)

	// Topic inline (compact, no border — for list/overview)
	styleTopicInline = lipgloss.NewStyle().
				Foreground(colorAccentViolet)

	// Search prompt title
	styleSearchPrompt = lipgloss.NewStyle().
				Foreground(colorAccentViolet).
				Bold(true)

	// Border style
	styleBorder = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder)

	// Score bar chars
	styleScoreFilled = lipgloss.NewStyle().Foreground(colorAccentViolet)
	styleScoreEmpty  = lipgloss.NewStyle().Foreground(colorFgGhost)

	// Star meter
	styleStarFilled = lipgloss.NewStyle().Foreground(colorGoldStar)
	styleStarEmpty  = lipgloss.NewStyle().Foreground(colorFgGhost)

	// Focus breadcrumb pills (status bar)
	styleFocusPill = lipgloss.NewStyle().
			Foreground(colorFgPrimary).
			Background(colorAccentViolet).
			Bold(true).
			PaddingLeft(1).PaddingRight(1)

	styleFocusPillDim = lipgloss.NewStyle().
				Foreground(colorFgMuted).
				Background(colorBgElevated).
				PaddingLeft(1).PaddingRight(1)

	// Panel header titles
	stylePanelTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccentViolet)

	stylePanelTitleDim = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorFgGhost)

	// Scroll indicator
	styleScrollThumb = lipgloss.NewStyle().Foreground(colorAccentViolet)
	styleScrollTrack = lipgloss.NewStyle().Foreground(colorFgGhost)
)

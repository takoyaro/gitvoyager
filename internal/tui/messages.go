package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/model"
)

// Search
type searchResultsMsg struct {
	Repos []model.Repo
	Query string
}

type searchErrorMsg struct{ Err error }

// Detail enrichment
type repoDetailMsg struct {
	Repo model.Repo
}

// README
type readmeMsg struct {
	FullName string
	Content  string
}

type readmeErrorMsg struct{ Err error }

// Debounce
type detailDebounceMsg struct {
	Tag int
}

// Clone
type cloneFinishedMsg struct {
	FullName string
	Err      error
}

// Rate limit
type rateLimitMsg struct {
	RateLimit github.RateLimit
}

// Status messages
type statusMsg struct {
	Text    string
	IsError bool
}

type clearStatusMsg struct{}

// Init
type ghAuthCheckedMsg struct{ Err error }

// Search history
type searchHistoryMsg struct {
	Recent     []searchHistoryItem
	Bookmarked []searchHistoryItem
}

type searchHistoryItem struct {
	ID          int64
	Query       string
	SortField   string
	ResultCount int
	Bookmarked  bool
	SearchedAt  string // relative time string
}

// Watchlist
type watchlistLoadedMsg struct {
	WatchSet map[string]bool
}

type watchToggledMsg struct {
	FullName string
	Watched  bool
}

// Watchlist screen
type watchlistReposMsg struct {
	Repos []model.Repo
}

type watchlistRefreshedMsg struct {
	Stats []model.Repo // partial repos with updated star/fork/issue counts
}

// Star velocity
type starDeltasLoadedMsg struct {
	FirstSeenStars map[string]int
}

// Spinner / rate limit tick
type spinnerTickMsg struct{}
type rateLimitTickMsg struct{}

// Return visit
type returnVisitMsg struct {
	Repos []model.Repo
}

// Session quit
type quitTimerMsg struct{}

// Timer for status auto-clear
func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

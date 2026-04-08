package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/takoyaro/gitvoyager/internal/github"
	"github.com/takoyaro/gitvoyager/internal/model"
	"github.com/takoyaro/gitvoyager/internal/store"
	"github.com/takoyaro/gitvoyager/internal/taste"
)

// Search
type searchResultsMsg struct {
	Repos      []model.Repo
	Query      string
	KnownRepos map[string]bool // repos that existed in DB before this search
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
	Acceleration   map[string]float64
}

// Spinner / rate limit tick
type spinnerTickMsg struct{}
type rateLimitTickMsg struct{}

// Peek cascade tick
type peekRevealTickMsg struct{}

// First-launch animation tick
type firstLaunchTickMsg struct{}

// UI micro-animation tick (focus pulse, watch pulse, compare reveal)
type uiAnimTickMsg struct{}

// Skeleton-to-content crossfade holdoff
type shimmerHoldMsg struct{}

// Return visit
type returnVisitMsg struct {
	Repos []model.Repo
}

// Taste profile
type tasteProfileMsg struct {
	Profile taste.Profile
}

type surprisePickMsg struct {
	Repo *model.Repo
}

// Local intelligence
type localScanMsg struct {
	ProjectCount int
	DepCount     int
	Err          error
}

// Claude AI
type claudeSummaryMsg struct {
	FullName string
	Summary  string
	Err      error
}

type claudeNLSearchMsg struct {
	Params  model.SearchParams
	Display string // human-readable version of the generated query
	Err     error
}

type claudeAnalysisMsg struct {
	FullName string
	Analysis string
	Err      error
}

type starredReposSyncedMsg struct {
	Count int
	Err   error
}

type topicDriftMsg struct {
	Topics []string
	Err    error
}

// Exclusions
type exclusionsLoadedMsg struct {
	Set *store.ExclusionSet
}

type exclusionUpdatedMsg struct {
	Set   *store.ExclusionSet
	Kind  string
	Value string
	Added bool // true = added, false = removed
}

// README analysis (runs off main thread)
type readmeAnalyzedMsg struct {
	FullName string
	Score    float64
}

// Batch intrinsic probe results
type batchIntrinsicMsg struct {
	Signals map[string]*model.IntrinsicSignals
	Topics  map[string][]string
	Err     error
}

// Topic heat
type topicHeatDelayMsg struct{} // fires after startup delay before sampling
type topicHeatSampledMsg struct {
	HeatMap map[string]float64 // topic → acceleration ratio
	Err     error
}

// Async local scan for "like" search
type likeSearchReadyMsg struct {
	Query string
	Path  string
}

// Session quit
type quitTimerMsg struct{}

// Timer for status auto-clear
func clearStatusAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

package model

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type Repo struct {
	// Identity
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Owner    string `json:"owner"`
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`

	// From gh search (always available)
	Description string    `json:"description"`
	Language    string    `json:"language"`
	Stars       int       `json:"stars"`
	Forks       int       `json:"forks"`
	OpenIssues  int       `json:"open_issues,omitempty"`
	License     string    `json:"license,omitempty"`
	IsArchived  bool      `json:"is_archived,omitempty"`
	IsFork      bool      `json:"is_fork,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	PushedAt    time.Time `json:"pushed_at,omitempty"`

	// From GraphQL enrichment (lazy)
	Topics        []string `json:"topics,omitempty"`
	WatcherCount  int      `json:"watcher_count,omitempty"`
	CommitCount   int      `json:"commit_count,omitempty"`
	ReadmeContent string   `json:"-"`
	Enriched      bool     `json:"-"`

	// Computed scores (populated after fetch)
	UnderdogScore   float64 `json:"underdog_score,omitempty"`
	DiscoveryScore  float64 `json:"discovery_score,omitempty"`
	FreshnessScore  float64 `json:"freshness_score,omitempty"`
	StarPercentile  int     `json:"star_percentile,omitempty"`

	// Local tracking
	DiscoveredAt time.Time  `json:"discovered_at"`
	SeenAt       *time.Time `json:"seen_at,omitempty"`
	Watchlisted  bool       `json:"watchlisted,omitempty"`
	StarDelta    int        `json:"star_delta,omitempty"`
}

type SortField string

const (
	SortStars     SortField = "stars"
	SortUpdated   SortField = "updated"
	SortForks     SortField = "forks"
	SortBestMatch SortField = "best-match"
	SortScore     SortField = "score"
)

var SortCycle = []SortField{SortStars, SortUpdated, SortForks, SortBestMatch, SortScore}

// PostFilter names a two-phase post-fetch scoring/filtering strategy.
type PostFilter string

const (
	PostFilterNone        PostFilter = ""
	PostFilterFreshSignal PostFilter = "fresh-signal"
)

type SearchParams struct {
	Query           string
	Topics          []string
	Language        string
	Stars           string // ">100", "10..500"
	Sort            SortField
	Order           string // "asc", "desc"
	Limit           int
	GoodFirstIssues string // ">=5"
	Created         string // date filter, e.g. ">2026-01-01"
	MaxPushedAge    time.Duration  // post-fetch filter: drop repos not pushed within this duration
	PostFilter      PostFilter     // two-phase scoring/filtering applied after fetch

	// Global exclusions (injected from config)
	ExcludeTopics   []string // appended as -topic:x qualifiers
	ExcludeOwners   []string // appended as -user:x qualifiers
	ExcludeKeywords []string // post-fetch: drop repos matching name/description
}

func DefaultSearchParams() SearchParams {
	return SearchParams{
		Sort:  SortStars,
		Order: "desc",
		Limit: 30,
	}
}

// Preset is a named discovery query for the search prompt screen.
type Preset struct {
	Key         string
	Name        string
	Description string
	Params      SearchParams
}

// GetPresets returns the built-in discovery presets with current-date ranges.
func GetPresets() []Preset {
	now := time.Now()
	monthAgo := now.AddDate(0, -1, 0).Format("2006-01-02")
	weekAgo := now.AddDate(0, 0, -7).Format("2006-01-02")
	twoWeeksAgo := now.AddDate(0, 0, -14).Format("2006-01-02")
	threeDaysAgo := now.AddDate(0, 0, -3).Format("2006-01-02")
	threeMonthsAgo := now.AddDate(0, -3, 0).Format("2006-01-02")
	return []Preset{
		{
			Key:         "1",
			Name:        "Trending",
			Description: "Recently created repos gaining stars fast",
			Params: SearchParams{
				Query:        fmt.Sprintf("created:>%s stars:>50", monthAgo),
				Sort:         SortStars,
				Order:        "desc",
				Limit:        50,
				MaxPushedAge: 60 * 24 * time.Hour, // 60 days
			},
		},
		{
			Key:         "2",
			Name:        "Underdogs",
			Description: "Low stars, high activity – diamonds in the rough",
			Params: SearchParams{
				Query:        fmt.Sprintf("stars:5..200 pushed:>%s forks:>=3", weekAgo),
				Sort:         SortForks,
				Order:        "desc",
				Limit:        50,
				MaxPushedAge: 14 * 24 * time.Hour, // 2 weeks
			},
		},
		{
			Key:         "3",
			Name:        "Fresh Signal",
			Description: "Brand new repos already showing signs of life",
			Params: SearchParams{
				Query:        fmt.Sprintf("stars:1..50 created:>%s pushed:>%s fork:false", twoWeeksAgo, threeDaysAgo),
				Sort:         SortUpdated,
				Order:        "desc",
				Limit:        80,
				MaxPushedAge: 7 * 24 * time.Hour, // 1 week
				PostFilter:   PostFilterFreshSignal,
			},
		},
		{
			Key:         "4",
			Name:        "Hidden Gems",
			Description: "High forks, low stars – actively used but undiscovered",
			Params: SearchParams{
				Query:        fmt.Sprintf("stars:1..50 forks:>=10 pushed:>%s", monthAgo),
				Sort:         SortForks,
				Order:        "desc",
				Limit:        50,
				MaxPushedAge: 60 * 24 * time.Hour, // 2 months
			},
		},
		{
			Key:         "5",
			Name:        "Rising Stars",
			Description: "Fastest star velocity this week",
			Params: SearchParams{
				Query:        fmt.Sprintf("stars:>10 pushed:>%s created:>%s", weekAgo, threeMonthsAgo),
				Sort:         SortStars,
				Order:        "desc",
				Limit:        100,
				MaxPushedAge: 14 * 24 * time.Hour, // 2 weeks
			},
		},
	}
}

// ComputeScores computes UnderdogScore, DiscoveryScore, and StarPercentile for each repo in place.
func ComputeScores(repos []Repo) {
	now := time.Now()
	for i := range repos {
		r := &repos[i]
		ageDays := now.Sub(r.CreatedAt).Hours() / 24
		if ageDays < 1 {
			ageDays = 1
		}
		recencyDays := now.Sub(r.PushedAt).Hours() / 24
		recencyBonus := 1.0 / (recencyDays/7.0 + 1.0)

		r.UnderdogScore = float64(r.Forks*3+r.OpenIssues*2) / float64(r.Stars+1)
		r.DiscoveryScore = (math.Log(float64(r.Stars)+1)*1.0+
			math.Log(float64(r.Forks)+1)*2.0+
			math.Log(float64(r.OpenIssues)+1)*1.5+
			recencyBonus*3.0) / math.Log(ageDays+1)
	}
	computeStarPercentiles(repos)
}

// computeStarPercentiles assigns 0–10 percentile rank based on star count within the set.
func computeStarPercentiles(repos []Repo) {
	n := len(repos)
	if n == 0 {
		return
	}
	// Find max stars
	maxStars := 0
	for _, r := range repos {
		if r.Stars > maxStars {
			maxStars = r.Stars
		}
	}
	if maxStars == 0 {
		return
	}
	for i := range repos {
		pct := float64(repos[i].Stars) / float64(maxStars)
		repos[i].StarPercentile = int(pct * 10)
		if repos[i].StarPercentile > 10 {
			repos[i].StarPercentile = 10
		}
	}
}

// SortByScore sorts repos by DiscoveryScore descending, in place.
func SortByScore(repos []Repo) {
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].DiscoveryScore > repos[j].DiscoveryScore
	})
}

// ApplyFreshSignalFilter scores repos on quality/freshness signals, sorts by
// that score, and drops anything below a minimum quality threshold.
// This is the two-phase filter for the "Fresh Signal" preset.
func ApplyFreshSignalFilter(repos []Repo) []Repo {
	now := time.Now()
	for i := range repos {
		repos[i].FreshnessScore = freshnessScore(&repos[i], now)
	}
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].FreshnessScore > repos[j].FreshnessScore
	})
	// Drop repos that lack minimum quality signals.
	const threshold = 4.0
	filtered := repos[:0]
	for _, r := range repos {
		if r.FreshnessScore >= threshold {
			filtered = append(filtered, r)
		}
	}
	return filtered
}

// freshnessScore produces a quality-weighted signal for brand-new repos.
// Higher = more interesting. Rewards: description, license, language, early
// traction (stars/forks/issues), very recent pushes, and youth.
func freshnessScore(r *Repo, now time.Time) float64 {
	score := 0.0

	// Quality signals — is this a real project?
	if r.Description != "" {
		score += 2.0
	}
	if r.License != "" {
		score += 1.5
	}
	if r.Language != "" {
		score += 1.0
	}
	if !r.IsFork {
		score += 1.0
	}
	if !r.IsArchived {
		score += 0.5
	}

	// Early traction (capped so a single signal can't dominate)
	if r.Stars > 0 {
		score += math.Min(math.Log(float64(r.Stars)+1)*1.5, 5.0)
	}
	if r.Forks > 0 {
		score += math.Min(math.Log(float64(r.Forks)+1)*2.0, 4.0)
	}
	if r.OpenIssues > 0 {
		score += math.Min(math.Log(float64(r.OpenIssues)+1)*1.0, 2.0)
	}

	// Push recency — strong bonus for very recent pushes, decays over days
	hoursSincePush := now.Sub(r.PushedAt).Hours()
	if hoursSincePush < 1 {
		hoursSincePush = 1
	}
	score += 3.0 / (hoursSincePush/24.0 + 1.0)

	// Youth bonus — newer repos are more interesting in this context
	daysSinceCreated := now.Sub(r.CreatedAt).Hours() / 24
	if daysSinceCreated < 1 {
		daysSinceCreated = 1
	}
	score += 2.0 / (daysSinceCreated/7.0 + 1.0)

	return score
}

package model

import (
	"fmt"
	"math"
	"sort"
	"time"
)

type Repo struct {
	// Identity
	ID       int64
	FullName string // "owner/name"
	Owner    string
	Name     string
	URL      string

	// From gh search (always available)
	Description string
	Language    string
	Stars       int
	Forks       int
	OpenIssues  int
	License     string
	IsArchived  bool
	IsFork      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	PushedAt    time.Time

	// From GraphQL enrichment (lazy)
	Topics        []string
	WatcherCount  int
	CommitCount   int // recent commits (~last month)
	ReadmeContent string
	Enriched      bool

	// Computed scores (populated after fetch)
	UnderdogScore   float64 // forks+issues relative to stars
	DiscoveryScore  float64 // multi-signal, age-normalised
	StarPercentile  int     // 0–10, percentile rank within result set

	// Local tracking
	DiscoveredAt time.Time
	SeenAt       *time.Time
	Watchlisted  bool
	StarDelta    int // stars gained since first seen; negative = lost stars
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
	MaxPushedAge    time.Duration // post-fetch filter: drop repos not pushed within this duration
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
			Name:        "Help Wanted",
			Description: "Welcoming repos actively seeking contributors",
			Params: SearchParams{
				Query:           fmt.Sprintf("stars:10..500 pushed:>%s", weekAgo),
				GoodFirstIssues: ">=5",
				Sort:            SortUpdated,
				Order:           "desc",
				Limit:           50,
				MaxPushedAge:    14 * 24 * time.Hour, // 2 weeks
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

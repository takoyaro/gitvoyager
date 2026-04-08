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
	Topics           []string `json:"topics,omitempty"`
	WatcherCount     int      `json:"watcher_count,omitempty"`
	CommitCount      int      `json:"commit_count,omitempty"`
	ContributorCount int      `json:"contributor_count,omitempty"`
	OpenPRCount      int      `json:"open_pr_count,omitempty"`
	LatestReleaseTag string   `json:"latest_release_tag,omitempty"`
	LatestReleaseAt  time.Time `json:"latest_release_at,omitempty"`
	ReadmeContent    string   `json:"-"`
	Enriched         bool     `json:"-"`

	// Intrinsic quality signals (from GraphQL object probes)
	Intrinsic *IntrinsicSignals `json:"intrinsic,omitempty"`

	// Computed scores (populated after fetch)
	UnderdogScore   float64 `json:"underdog_score,omitempty"`
	DiscoveryScore  float64 `json:"discovery_score,omitempty"`
	FreshnessScore  float64 `json:"freshness_score,omitempty"`
	IntrinsicScore  float64 `json:"intrinsic_score,omitempty"`
	ReadmeScore     float64 `json:"readme_score,omitempty"`
	TopicHeatBoost  float64 `json:"topic_heat_boost,omitempty"`
	StarPercentile  int     `json:"star_percentile,omitempty"`

	// Local tracking
	DiscoveredAt time.Time  `json:"discovered_at"`
	SeenAt       *time.Time `json:"seen_at,omitempty"`
	Watchlisted  bool       `json:"watchlisted,omitempty"`
	StarDelta    int        `json:"star_delta,omitempty"`

	// Derived discovery signals
	NewDiscovery bool    `json:"new_discovery,omitempty"`
	Sleeper      bool    `json:"sleeper,omitempty"`
	StarAccel    float64 `json:"star_accel,omitempty"` // velocity_7d / velocity_lifetime; >1.0 = accelerating
}

// IntrinsicSignals holds repo structure probes from GraphQL object() lookups.
type IntrinsicSignals struct {
	ReadmeByteSize     int      `json:"readme_byte_size"`
	HasLicense         bool     `json:"has_license"`
	CIWorkflowCount    int      `json:"ci_workflow_count"`
	CIWorkflowNames    []string `json:"ci_workflow_names,omitempty"`
	HasClaudeMd        bool     `json:"has_claude_md"`
	HasContributing    bool     `json:"has_contributing"`
	HasPackageManifest bool     `json:"has_package_manifest"`
	RootDirCount       int      `json:"root_dir_count"`
	RootDirNames       []string `json:"root_dir_names,omitempty"`
}

type SortField string

const (
	SortStars     SortField = "stars"
	SortUpdated   SortField = "updated"
	SortForks     SortField = "forks"
	SortBestMatch SortField = "best-match"
	SortScore     SortField = "score"
	SortDiscovery SortField = "discovery"
)

var SortCycle = []SortField{SortDiscovery, SortStars, SortUpdated, SortForks, SortBestMatch, SortScore}

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

	// Extended search qualifiers
	License          []string // --license=mit, --license=apache-2.0
	Size             string   // --size=">100" or "<5000"
	Match            []string // --match=name, --match=readme
	NumberTopics     string   // --number-topics=">=3"
	HelpWantedIssues string   // --help-wanted-issues=">=5"
	Updated          string   // --updated=">2026-01-01"
	IncludeForks     string   // --include-forks=false|true|only
	Owner            []string // --owner=X (positive filter)

	// Global exclusions (injected from config)
	ExcludeTopics   []string // appended as -topic:x qualifiers
	ExcludeOwners   []string // appended as -user:x qualifiers
	ExcludeKeywords []string // post-fetch: drop repos matching name/description
}

func DefaultSearchParams() SearchParams {
	return SearchParams{
		Sort:  SortDiscovery,
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
		// ── Discovery-first: cards [1]-[4] ──
		{
			Key:         "1",
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
			Key:         "2",
			Name:        "Zero-Day Gems",
			Description: "Brand new repos with craft, any star count",
			Params: SearchParams{
				Query:        fmt.Sprintf("created:>%s pushed:>%s fork:false", weekAgo, threeDaysAgo),
				Sort:         SortUpdated,
				Order:        "desc",
				Limit:        80,
				MaxPushedAge: 14 * 24 * time.Hour,
				PostFilter:   PostFilterFreshSignal,
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
		// ── Extended expeditions: pills [5]-[9] ──
		{
			Key:         "5",
			Name:        "Underdog Craft",
			Description: "Low stars, high quality signals",
			Params: SearchParams{
				Query:        fmt.Sprintf("stars:0..100 pushed:>%s fork:false", weekAgo),
				Sort:         SortUpdated,
				Order:        "desc",
				Limit:        80,
				MaxPushedAge: 14 * 24 * time.Hour,
				PostFilter:   PostFilterFreshSignal,
			},
		},
		{
			Key:         "6",
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
		{
			Key:         "7",
			Name:        "Agent-Ready",
			Description: "Repos with MCP, agent tooling, or Claude integration",
			Params: SearchParams{
				Query:        fmt.Sprintf("topic:mcp OR topic:ai-agent OR topic:claude-code pushed:>%s", weekAgo),
				Sort:         SortUpdated,
				Order:        "desc",
				Limit:        50,
				MaxPushedAge: 30 * 24 * time.Hour,
			},
		},
		{
			Key:         "8",
			Name:        "Contributor Magnets",
			Description: "Active projects welcoming contributors",
			Params: SearchParams{
				Query:            fmt.Sprintf("pushed:>%s", weekAgo),
				GoodFirstIssues:  ">=5",
				HelpWantedIssues: ">=5",
				Sort:             SortUpdated,
				Order:            "desc",
				Limit:            50,
				MaxPushedAge:     30 * 24 * time.Hour,
			},
		},
		{
			Key:         "9",
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
	}
}

// GetHotSpacePreset returns a dynamic preset using the hottest topics.
func GetHotSpacePreset(hotTopics []string) Preset {
	twoWeeksAgo := time.Now().AddDate(0, 0, -14).Format("2006-01-02")
	return Preset{
		Key:         "0",
		Name:        "Hot Space",
		Description: "Repos in the fastest-growing problem spaces",
		Params: SearchParams{
			Topics:  hotTopics,
			Created: fmt.Sprintf(">%s", twoWeeksAgo),
			Sort:    SortStars,
			Order:   "desc",
			Limit:   60,
		},
	}
}

// ApplyTopicHeatBoosts sets TopicHeatBoost on each repo based on the topic heat map.
// The boost is the maximum acceleration ratio across the repo's topics.
// Must be called BEFORE ComputeScores so the boost is available for scoring.
func ApplyTopicHeatBoosts(repos []Repo, heatMap map[string]float64) {
	for i := range repos {
		r := &repos[i]
		best := 0.0
		for _, t := range r.Topics {
			if accel, ok := heatMap[t]; ok && accel > best {
				best = accel
			}
		}
		// Also check language as a pseudo-topic.
		if r.Language != "" {
			if accel, ok := heatMap[r.Language]; ok && accel > best {
				best = accel
			}
		}
		if best > 1.0 {
			r.TopicHeatBoost = best
		}
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

		// Default discovery score (social metrics only — used for unenriched repos).
		r.DiscoveryScore = (math.Log(float64(r.Stars)+1)*1.0+
			math.Log(float64(r.Forks)+1)*2.0+
			math.Log(float64(r.OpenIssues)+1)*1.5+
			recencyBonus*3.0) / math.Log(ageDays+1)

		// If intrinsic quality signals are available, blend them in.
		if r.IntrinsicScore > 0 {
			social := math.Log(float64(r.Stars)+1)*0.5 + math.Log(float64(r.Forks)+1)*1.0
			activity := recencyBonus * 3.0
			r.DiscoveryScore = (r.IntrinsicScore*4.0 + r.TopicHeatBoost*3.0 + activity*2.0 + social*1.0) / math.Log(ageDays+1)
		}

		// Sleeper: actively maintained but unnoticed
		r.Sleeper = recencyDays < 14 && r.Stars < 200 && ageDays > 180
	}
	computeStarPercentiles(repos)
}

// ComputeIntrinsicScore computes a 0-10 quality score from structure probes and README analysis.
// Call this after enrichment populates Intrinsic signals on a repo.
func ComputeIntrinsicScore(r *Repo) {
	if r.Intrinsic == nil {
		return
	}
	s := r.Intrinsic
	score := 0.0

	// README size tiers (max 4).
	switch {
	case s.ReadmeByteSize > 10240:
		score += 4.0
	case s.ReadmeByteSize > 5120:
		score += 3.0
	case s.ReadmeByteSize > 2048:
		score += 2.0
	case s.ReadmeByteSize > 500:
		score += 1.0
	}

	if s.HasLicense {
		score += 1.5
	}
	if s.CIWorkflowCount > 0 {
		score += 1.5
	}
	if s.HasContributing {
		score += 0.5
	}
	if s.HasClaudeMd {
		score += 0.5
	}
	if s.HasPackageManifest {
		score += 1.0
	}
	// Root structure: dirs in 3-30 range indicates organized project.
	if s.RootDirCount >= 3 && s.RootDirCount <= 30 {
		score += 1.0
	}

	if score > 10.0 {
		score = 10.0
	}
	r.IntrinsicScore = score

	// If README was analyzed via goldmark, blend with ReadmeScore.
	if r.ReadmeScore > 0 {
		r.IntrinsicScore = r.IntrinsicScore*0.5 + r.ReadmeScore*0.5
	}
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

// SortByDiscovery implements the default "smart" sort:
//
//	Tier 0: Unseen new discoveries, sleepers first, then by DiscoveryScore
//	Tier 1: Known unseen repos, by acceleration then DiscoveryScore
//	Tier 2: Previously seen repos, by acceleration then DiscoveryScore
func SortByDiscovery(repos []Repo, seenSet map[string]bool) {
	sort.SliceStable(repos, func(i, j int) bool {
		ri, rj := &repos[i], &repos[j]

		tierI := repoTier(ri, seenSet)
		tierJ := repoTier(rj, seenSet)
		if tierI != tierJ {
			return tierI < tierJ
		}

		// Within tier 0: sleepers float to very top
		if tierI == 0 && ri.Sleeper != rj.Sleeper {
			return ri.Sleeper
		}

		// Within tiers 1 and 2: accelerating repos first
		if tierI >= 1 {
			accelI := ri.StarAccel > 1.0
			accelJ := rj.StarAccel > 1.0
			if accelI != accelJ {
				return accelI
			}
			if accelI && accelJ && ri.StarAccel != rj.StarAccel {
				return ri.StarAccel > rj.StarAccel
			}
		}

		return ri.DiscoveryScore > rj.DiscoveryScore
	})
}

func repoTier(r *Repo, seenSet map[string]bool) int {
	if seenSet != nil && seenSet[r.FullName] {
		return 2 // seen
	}
	if r.NewDiscovery {
		return 0 // unseen + new to DB
	}
	return 1 // unseen + already in DB
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

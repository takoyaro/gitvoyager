package model

import "time"

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
	ReadmeContent string
	Enriched      bool

	// Local tracking
	DiscoveredAt time.Time
	SeenAt       *time.Time
}

type SortField string

const (
	SortStars     SortField = "stars"
	SortUpdated   SortField = "updated"
	SortForks     SortField = "forks"
	SortBestMatch SortField = "best-match"
)

var SortCycle = []SortField{SortStars, SortUpdated, SortForks, SortBestMatch}

type SearchParams struct {
	Query    string
	Topics   []string
	Language string
	Stars    string // ">100", "10..500"
	Sort     SortField
	Order    string // "asc", "desc"
	Limit    int
}

func DefaultSearchParams() SearchParams {
	return SearchParams{
		Sort:  SortStars,
		Order: "desc",
		Limit: 30,
	}
}

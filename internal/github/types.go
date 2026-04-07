package github

import "time"

// searchResult maps to the JSON output of `gh search repos --json ...`
type searchResult struct {
	FullName         string    `json:"fullName"`
	Description      string    `json:"description"`
	Language         string    `json:"language"`
	StargazersCount  int       `json:"stargazersCount"`
	ForksCount       int       `json:"forksCount"`
	OpenIssuesCount  int       `json:"openIssuesCount"`
	License          *license  `json:"license"`
	IsArchived       bool      `json:"isArchived"`
	IsFork           bool      `json:"isFork"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	PushedAt         time.Time `json:"pushedAt"`
	URL              string    `json:"url"`
	Owner            owner     `json:"owner"`
	Name             string    `json:"name"`
	DefaultBranch    string    `json:"defaultBranch"`
	HomepageURL      string    `json:"homepageUrl"`
	HasIssuesEnabled bool      `json:"hasIssuesEnabled"`
}

type license struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type owner struct {
	Login string `json:"login"`
}

// graphQLResponse wraps the batch enrichment response
type graphQLResponse struct {
	Data map[string]graphQLRepo `json:"data"`
}

type graphQLRepo struct {
	Description       string          `json:"description"`
	RepositoryTopics  topicConnection `json:"repositoryTopics"`
	MentionableUsers  countConnection `json:"mentionableUsers"`
	Watchers          countConnection `json:"watchers"`
	PullRequests      countConnection `json:"pullRequests"`
	DefaultBranchRef  *branchRef      `json:"defaultBranchRef"`
	LatestRelease     *release        `json:"latestRelease"`
}

type topicConnection struct {
	Nodes []topicNode `json:"nodes"`
}

type topicNode struct {
	Topic struct {
		Name string `json:"name"`
	} `json:"topic"`
}

type countConnection struct {
	TotalCount int `json:"totalCount"`
}

type branchRef struct {
	Target struct {
		History countConnection `json:"history"`
	} `json:"target"`
}

type release struct {
	TagName     string    `json:"tagName"`
	PublishedAt time.Time `json:"publishedAt"`
}

// RateLimit holds GitHub API rate limit state
type RateLimit struct {
	SearchRemaining int
	SearchLimit     int
	SearchReset     time.Time
	CoreRemaining   int
	CoreLimit       int
}

type rateLimitResponse struct {
	Resources struct {
		Search struct {
			Limit     int   `json:"limit"`
			Remaining int   `json:"remaining"`
			Reset     int64 `json:"reset"`
		} `json:"search"`
		Core struct {
			Limit     int `json:"limit"`
			Remaining int `json:"remaining"`
		} `json:"core"`
	} `json:"resources"`
}

package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

const nlSearchCacheTTL = 1 * time.Hour

type searchTranslation struct {
	Query          string   `json:"query"`
	Language       string   `json:"language"`
	Stars          string   `json:"stars"`
	Topics         []string `json:"topics"`
	Sort           string   `json:"sort"`
	Created        string   `json:"created"`
	GoodFirstIssue string   `json:"good_first_issues"`
}

// TranslateToSearch converts a natural language description into GitHub search params.
func (c *Client) TranslateToSearch(ctx context.Context, naturalQuery string) (model.SearchParams, string, error) {
	if !c.Available() {
		return model.SearchParams{}, "", fmt.Errorf("claude not available")
	}

	cacheKey := "claude:nlsearch:" + naturalQuery
	if cached, ok := c.getCached(cacheKey); ok {
		params, raw, err := parseSearchTranslation(cached)
		if err == nil {
			return params, raw, nil
		}
	}

	today := time.Now().Format("2006-01-02")
	prompt := fmt.Sprintf(
		`You are a search query translator for GitVoyager, a GitHub repo discovery tool focused on finding underdogs, hidden gems, and emerging projects.

Today's date: %s

Convert the user's natural language request into a JSON object for "gh search repos". Available fields (omit any that are empty/unused):

- "query": string — The main search text. You can embed GitHub search qualifiers directly here:
    pushed:>YYYY-MM-DD     (recently active)
    fork:false             (exclude forks)
    archived:false         (exclude archived)
    forks:>=N or forks:N..M
    is:public
    license:mit / license:apache-2.0
    in:name / in:description / in:readme
    org:ORGNAME / user:USERNAME
  Multiple qualifiers can be combined: "machine learning fork:false pushed:>2026-03-01"
- "language": string — Programming language filter (e.g. "go", "python", "rust", "typescript")
- "stars": string — Star count range. Formats: ">100", "10..500", ">=50", "<1000"
- "topics": array of strings — Topic/tag filters (e.g. ["cli", "tui"])
- "sort": string — One of: "stars", "updated", "forks", "best-match"
- "created": string — Repo creation date filter (e.g. ">2026-01-01", "2025-06-01..2026-01-01")
- "good_first_issues": string — Minimum good-first-issue count (e.g. ">=5", ">0")

Guidelines:
- For discovery/underdog queries, prefer low star ranges (e.g. "5..200") with activity signals (pushed:, forks:>=)
- For trending queries, use recent created: dates with stars:>50
- For "new" or "fresh" repos, use created:>DATE from the last 2-4 weeks
- For "active" or "maintained" repos, add pushed:>DATE (last 1-2 weeks) in the query field
- Always add fork:false and archived:false to the query unless the user specifically wants forks/archived repos
- Keep the query field focused — use structured fields (language, stars, topics, sort) for things that have dedicated fields
- If the user mentions a time frame like "this month" or "last week", compute the actual date from today

Examples:

User: "find me new Go CLI tools"
{"query":"cli tool fork:false archived:false created:>2026-03-08 pushed:>2026-03-24","language":"go","stars":">=1","topics":["cli"],"sort":"updated"}

User: "underrated Python ML libraries"
{"query":"machine learning fork:false archived:false pushed:>2026-03-24","language":"python","stars":"5..300","sort":"forks"}

User: "mcp server"
{"query":"mcp server fork:false archived:false pushed:>2026-03-01","stars":">=1","sort":"stars"}

User: "popular rust projects for beginners"
{"query":"fork:false archived:false","language":"rust","stars":">500","sort":"stars","good_first_issues":">=5"}

User wants: %s

Return ONLY the JSON object, no explanation or markdown.`, today, naturalQuery)

	result, err := c.run(ctx, prompt)
	if err != nil {
		return model.SearchParams{}, "", err
	}

	c.setCache(cacheKey, result, nlSearchCacheTTL)

	params, raw, err := parseSearchTranslation(result)
	return params, raw, err
}

func parseSearchTranslation(raw string) (model.SearchParams, string, error) {
	// Extract JSON from response (Claude might wrap it in markdown code blocks)
	jsonStr := extractJSON(raw)

	var t searchTranslation
	if err := json.Unmarshal([]byte(jsonStr), &t); err != nil {
		return model.SearchParams{}, raw, fmt.Errorf("parse response: %w", err)
	}

	params := model.DefaultSearchParams()
	params.Query = t.Query
	params.Language = t.Language
	params.Stars = t.Stars
	params.Topics = t.Topics
	params.Created = t.Created
	params.GoodFirstIssues = t.GoodFirstIssue
	if t.Sort != "" {
		params.Sort = model.SortField(t.Sort)
	}

	// Build a human-readable representation of the query
	display := t.Query
	if t.Language != "" {
		display += " language:" + t.Language
	}
	if t.Stars != "" {
		display += " stars:" + t.Stars
	}
	if t.Created != "" {
		display += " created:" + t.Created
	}

	return params, display, nil
}

// extractJSON strips markdown code fences and returns the JSON content.
func extractJSON(s string) string {
	// Try to find JSON object boundaries
	start := -1
	for i, c := range s {
		if c == '{' {
			start = i
			break
		}
	}
	if start == -1 {
		return s
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}

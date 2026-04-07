package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

var searchJSONFields = strings.Join([]string{
	"fullName", "description", "language", "stargazersCount",
	"forksCount", "openIssuesCount", "license", "isArchived",
	"isFork", "createdAt", "updatedAt", "pushedAt", "url",
	"owner", "name", "defaultBranch",
}, ",")

func (c *Client) SearchRepos(ctx context.Context, params model.SearchParams) ([]model.Repo, error) {
	args := []string{"search", "repos"}

	// Build the query string, appending exclusion qualifiers.
	query := params.Query
	for _, t := range params.ExcludeTopics {
		query += " -topic:" + t
	}
	for _, o := range params.ExcludeOwners {
		query += " -user:" + o
	}
	if query != "" {
		args = append(args, query)
	}

	for _, topic := range params.Topics {
		args = append(args, "--topic="+topic)
	}
	if params.Language != "" {
		args = append(args, "--language="+params.Language)
	}
	if params.Stars != "" {
		args = append(args, "--stars="+params.Stars)
	}
	if params.Sort != "" && params.Sort != model.SortScore {
		args = append(args, "--sort="+string(params.Sort))
	}
	if params.GoodFirstIssues != "" {
		args = append(args, "--good-first-issues="+params.GoodFirstIssues)
	}
	if params.Created != "" {
		args = append(args, "--created="+params.Created)
	}
	if params.Order != "" {
		args = append(args, "--order="+params.Order)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 30
	}
	args = append(args, fmt.Sprintf("-L%d", limit))
	args = append(args, "--json", searchJSONFields)

	out, err := c.run(ctx, args...)
	if err != nil {
		return nil, err
	}

	var results []searchResult
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parse search results: %w", err)
	}

	repos := make([]model.Repo, len(results))
	now := time.Now()
	for i, r := range results {
		lic := ""
		if r.License != nil {
			lic = r.License.Key
		}
		repos[i] = model.Repo{
			FullName:     r.FullName,
			Owner:        r.Owner.Login,
			Name:         r.Name,
			URL:          r.URL,
			Description:  r.Description,
			Language:     r.Language,
			Stars:        r.StargazersCount,
			Forks:        r.ForksCount,
			OpenIssues:   r.OpenIssuesCount,
			License:      lic,
			IsArchived:   r.IsArchived,
			IsFork:       r.IsFork,
			CreatedAt:    r.CreatedAt,
			UpdatedAt:    r.UpdatedAt,
			PushedAt:     r.PushedAt,
			DiscoveredAt: now,
		}
	}

	// Post-fetch filter: drop repos matching excluded keywords in name/description.
	if len(params.ExcludeKeywords) > 0 {
		filtered := repos[:0]
		for _, r := range repos {
			haystack := strings.ToLower(r.FullName + " " + r.Description)
			excluded := false
			for _, kw := range params.ExcludeKeywords {
				if strings.Contains(haystack, strings.ToLower(kw)) {
					excluded = true
					break
				}
			}
			if !excluded {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	}

	// Post-fetch filter: drop repos that haven't been pushed within MaxPushedAge.
	// GitHub search doesn't always honor the pushed: qualifier reliably.
	if params.MaxPushedAge > 0 {
		cutoff := now.Add(-params.MaxPushedAge)
		filtered := repos[:0]
		for _, r := range repos {
			if !r.PushedAt.IsZero() && r.PushedAt.After(cutoff) {
				filtered = append(filtered, r)
			}
		}
		repos = filtered
	}

	// Two-phase post-filter: score and re-rank results based on quality signals.
	if params.PostFilter == model.PostFilterFreshSignal {
		repos = model.ApplyFreshSignalFilter(repos)
	}

	return repos, nil
}

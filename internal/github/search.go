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

	if params.Query != "" {
		args = append(args, params.Query)
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

	return repos, nil
}

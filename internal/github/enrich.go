package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

const repoGraphQLFragment = `
	description
	repositoryTopics(first: 20) {
		nodes { topic { name } }
	}
	mentionableUsers { totalCount }
	watchers { totalCount }
	pullRequests(states: OPEN) { totalCount }
	latestRelease { tagName publishedAt }
	defaultBranchRef {
		target {
			... on Commit {
				history(since: "%s") { totalCount }
			}
		}
	}
`

func (c *Client) EnrichRepo(ctx context.Context, repo model.Repo) (model.Repo, error) {
	since := repo.UpdatedAt.AddDate(0, -1, 0).Format("2006-01-02T15:04:05Z")

	query := fmt.Sprintf(`{
		repository(owner: %q, name: %q) {
			%s
		}
	}`, repo.Owner, repo.Name, fmt.Sprintf(repoGraphQLFragment, since))

	out, err := c.run(ctx, "api", "graphql", "-f", "query="+query)
	if err != nil {
		return repo, err
	}

	var resp struct {
		Data struct {
			Repository graphQLRepo `json:"repository"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return repo, fmt.Errorf("parse enrichment: %w", err)
	}

	r := resp.Data.Repository
	topics := make([]string, len(r.RepositoryTopics.Nodes))
	for i, n := range r.RepositoryTopics.Nodes {
		topics[i] = n.Topic.Name
	}

	repo.Topics = topics
	repo.WatcherCount = r.Watchers.TotalCount
	if r.DefaultBranchRef != nil {
		repo.CommitCount = r.DefaultBranchRef.Target.History.TotalCount
	}
	repo.Enriched = true

	return repo, nil
}

// FetchRepoStats fetches current star/fork/issue counts for a batch of repos
// using a single GraphQL request (up to 20 at a time). Used for watchlist refresh.
func (c *Client) FetchRepoStats(ctx context.Context, fullNames []string) ([]model.Repo, error) {
	if len(fullNames) == 0 {
		return nil, nil
	}
	const batchSize = 20
	var all []model.Repo
	for i := 0; i < len(fullNames); i += batchSize {
		end := i + batchSize
		if end > len(fullNames) {
			end = len(fullNames)
		}
		batch, err := c.fetchRepoStatsBatch(ctx, fullNames[i:end], i)
		if err != nil {
			return all, err
		}
		all = append(all, batch...)
	}
	return all, nil
}

func (c *Client) fetchRepoStatsBatch(ctx context.Context, fullNames []string, offset int) ([]model.Repo, error) {
	type statsData struct {
		StargazerCount int `json:"stargazerCount"`
		ForkCount      int `json:"forkCount"`
		Issues         struct {
			TotalCount int `json:"totalCount"`
		} `json:"issues"`
		PushedAt string `json:"pushedAt"`
	}

	var parts []string
	for idx, fullName := range fullNames {
		halves := strings.SplitN(fullName, "/", 2)
		if len(halves) != 2 {
			continue
		}
		alias := fmt.Sprintf("r%d", offset+idx)
		parts = append(parts, fmt.Sprintf(`
			%s: repository(owner: %q, name: %q) {
				stargazerCount
				forkCount
				issues(states: OPEN) { totalCount }
				pushedAt
			}`, alias, halves[0], halves[1]))
	}

	query := "{ " + strings.Join(parts, "\n") + " }"
	out, err := c.run(ctx, "api", "graphql", "-f", "query="+query)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("parse repo stats: %w", err)
	}

	repos := make([]model.Repo, 0, len(fullNames))
	for idx, fullName := range fullNames {
		alias := fmt.Sprintf("r%d", offset+idx)
		raw, ok := resp.Data[alias]
		if !ok || string(raw) == "null" {
			continue
		}
		var s statsData
		if err := json.Unmarshal(raw, &s); err != nil {
			continue
		}
		halves := strings.SplitN(fullName, "/", 2)
		r := model.Repo{
			FullName:   fullName,
			Stars:      s.StargazerCount,
			Forks:      s.ForkCount,
			OpenIssues: s.Issues.TotalCount,
		}
		if len(halves) == 2 {
			r.Owner = halves[0]
			r.Name = halves[1]
		}
		if s.PushedAt != "" {
			r.PushedAt, _ = time.Parse(time.RFC3339, s.PushedAt)
		}
		repos = append(repos, r)
	}
	return repos, nil
}

func (c *Client) FetchReadme(ctx context.Context, owner, name string) (string, error) {
	out, err := c.run(ctx,
		"api",
		"-H", "Accept: application/vnd.github.raw+json",
		fmt.Sprintf("repos/%s/%s/readme", owner, name),
	)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

package github

import (
	"context"
	"encoding/json"
	"fmt"

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

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
	readmeFile: object(expression: "HEAD:README.md") { ... on Blob { byteSize } }
	readmeLower: object(expression: "HEAD:readme.md") { ... on Blob { byteSize } }
	licenseFile: object(expression: "HEAD:LICENSE") { ... on Blob { byteSize } }
	licenseMd: object(expression: "HEAD:LICENSE.md") { ... on Blob { byteSize } }
	ciDir: object(expression: "HEAD:.github/workflows") {
		... on Tree { entries { name } }
	}
	claudeMd: object(expression: "HEAD:CLAUDE.md") { ... on Blob { byteSize } }
	contributingMd: object(expression: "HEAD:CONTRIBUTING.md") { ... on Blob { byteSize } }
	gomod: object(expression: "HEAD:go.mod") { ... on Blob { byteSize } }
	packageJson: object(expression: "HEAD:package.json") { ... on Blob { byteSize } }
	cargoToml: object(expression: "HEAD:Cargo.toml") { ... on Blob { byteSize } }
	pyprojectToml: object(expression: "HEAD:pyproject.toml") { ... on Blob { byteSize } }
	rootTree: object(expression: "HEAD:") {
		... on Tree { entries { name type } }
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
	repo.ContributorCount = r.MentionableUsers.TotalCount
	repo.OpenPRCount = r.PullRequests.TotalCount
	if r.DefaultBranchRef != nil {
		repo.CommitCount = r.DefaultBranchRef.Target.History.TotalCount
	}
	if r.LatestRelease != nil {
		repo.LatestReleaseTag = r.LatestRelease.TagName
		repo.LatestReleaseAt = r.LatestRelease.PublishedAt
	}

	// Populate intrinsic signals from structure probes.
	repo.Intrinsic = buildIntrinsicSignals(&r)
	repo.Enriched = true

	return repo, nil
}

// buildIntrinsicSignals extracts structure signals from GraphQL object probes.
func buildIntrinsicSignals(r *graphQLRepo) *model.IntrinsicSignals {
	s := &model.IntrinsicSignals{}

	// README size (check both casing variants).
	if r.ReadmeFile != nil {
		s.ReadmeByteSize = r.ReadmeFile.ByteSize
	} else if r.ReadmeLower != nil {
		s.ReadmeByteSize = r.ReadmeLower.ByteSize
	}

	// License presence.
	s.HasLicense = r.LicenseFile != nil || r.LicenseMd != nil

	// CI workflows.
	if r.CIDir != nil {
		for _, e := range r.CIDir.Entries {
			if strings.HasSuffix(e.Name, ".yml") || strings.HasSuffix(e.Name, ".yaml") {
				s.CIWorkflowCount++
				s.CIWorkflowNames = append(s.CIWorkflowNames, e.Name)
			}
		}
	}

	// Agent-native signals.
	s.HasClaudeMd = r.ClaudeMd != nil

	// Contributing guide.
	s.HasContributing = r.ContribMd != nil

	// Package manifest (any language).
	s.HasPackageManifest = r.GoMod != nil || r.PackageJSON != nil ||
		r.CargoToml != nil || r.PyprojectToml != nil

	// Root tree structure.
	if r.RootTree != nil {
		for _, e := range r.RootTree.Entries {
			if e.Type == "tree" {
				s.RootDirCount++
				s.RootDirNames = append(s.RootDirNames, e.Name)
			}
		}
	}

	return s
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

// BatchIntrinsicProbeWithTopics runs structure probes on a batch of repos via
// aliased GraphQL queries (15 per request). Returns intrinsic signals and topics
// for each repo. Used to compute quality grades for search results list view.
func (c *Client) BatchIntrinsicProbeWithTopics(ctx context.Context, repos []model.Repo) (map[string]*model.IntrinsicSignals, map[string][]string, error) {
	if len(repos) == 0 {
		return nil, nil, nil
	}
	signals := make(map[string]*model.IntrinsicSignals)
	topics := make(map[string][]string)
	const batchSize = 15

	for i := 0; i < len(repos); i += batchSize {
		end := i + batchSize
		if end > len(repos) {
			end = len(repos)
		}
		batch := repos[i:end]

		var parts []string
		for idx, r := range batch {
			if r.Owner == "" || r.Name == "" {
				continue
			}
			alias := fmt.Sprintf("r%d", idx)
			parts = append(parts, fmt.Sprintf(`
				%s: repository(owner: %q, name: %q) {
					repositoryTopics(first: 20) { nodes { topic { name } } }
					readmeFile: object(expression: "HEAD:README.md") { ... on Blob { byteSize } }
					readmeLower: object(expression: "HEAD:readme.md") { ... on Blob { byteSize } }
					licenseFile: object(expression: "HEAD:LICENSE") { ... on Blob { byteSize } }
					licenseMd: object(expression: "HEAD:LICENSE.md") { ... on Blob { byteSize } }
					ciDir: object(expression: "HEAD:.github/workflows") {
						... on Tree { entries { name } }
					}
					claudeMd: object(expression: "HEAD:CLAUDE.md") { ... on Blob { byteSize } }
					contributingMd: object(expression: "HEAD:CONTRIBUTING.md") { ... on Blob { byteSize } }
					gomod: object(expression: "HEAD:go.mod") { ... on Blob { byteSize } }
					packageJson: object(expression: "HEAD:package.json") { ... on Blob { byteSize } }
					cargoToml: object(expression: "HEAD:Cargo.toml") { ... on Blob { byteSize } }
					pyprojectToml: object(expression: "HEAD:pyproject.toml") { ... on Blob { byteSize } }
					rootTree: object(expression: "HEAD:") {
						... on Tree { entries { name type } }
					}
				}`, alias, r.Owner, r.Name))
		}

		if len(parts) == 0 {
			continue
		}

		query := "{ " + strings.Join(parts, "\n") + " }"
		out, err := c.run(ctx, "api", "graphql", "-f", "query="+query)
		if err != nil {
			continue
		}

		var resp struct {
			Data map[string]json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(out, &resp); err != nil {
			continue
		}

		for idx, r := range batch {
			alias := fmt.Sprintf("r%d", idx)
			raw, ok := resp.Data[alias]
			if !ok || string(raw) == "null" {
				continue
			}
			var gr graphQLRepo
			if err := json.Unmarshal(raw, &gr); err != nil {
				continue
			}
			signals[r.FullName] = buildIntrinsicSignals(&gr)
			if len(gr.RepositoryTopics.Nodes) > 0 {
				t := make([]string, len(gr.RepositoryTopics.Nodes))
				for ti, n := range gr.RepositoryTopics.Nodes {
					t[ti] = n.Topic.Name
				}
				topics[r.FullName] = t
			}
		}
	}
	return signals, topics, nil
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

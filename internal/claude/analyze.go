package claude

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

const (
	analysisCacheTTL  = 3 * 24 * time.Hour  // 3 days
	topicDriftCacheTTL = 24 * time.Hour      // 1 day
)

// WhyTrending analyzes a repo's stats and README snippet to explain
// why it might be gaining traction.
func (c *Client) WhyTrending(ctx context.Context, repo model.Repo, readmeSnippet string) (string, error) {
	if !c.Available() {
		return "", fmt.Errorf("claude not available")
	}

	cacheKey := fmt.Sprintf("claude:trending:%s", repo.FullName)
	if cached, ok := c.getCached(cacheKey); ok {
		return cached, nil
	}

	snippet := readmeSnippet
	if len(snippet) > 2000 {
		snippet = snippet[:2000] + "\n[truncated]"
	}

	prompt := fmt.Sprintf(
		`Briefly explain why this GitHub repo might be trending or gaining attention. Use 2-3 sentences. Consider its stats, age, and description. Do not use markdown.

Repository: %s
Language: %s
Stars: %d, Forks: %d, Open Issues: %d
Created: %s
Description: %s

README excerpt:
%s`,
		repo.FullName,
		repo.Language,
		repo.Stars, repo.Forks, repo.OpenIssues,
		repo.CreatedAt.Format("2006-01-02"),
		repo.Description,
		snippet,
	)

	result, err := c.run(ctx, prompt)
	if err != nil {
		return "", err
	}

	c.setCache(cacheKey, result, analysisCacheTTL)
	return result, nil
}

// SuggestAdjacentTopics analyzes a search query and result topics to suggest
// related topics the user might also find interesting.
func (c *Client) SuggestAdjacentTopics(ctx context.Context, searchQuery string, resultTopics []string) ([]string, error) {
	if !c.Available() {
		return nil, fmt.Errorf("claude not available")
	}

	// Deduplicate and limit topics
	seen := make(map[string]bool)
	var uniqueTopics []string
	for _, t := range resultTopics {
		if !seen[t] && t != "" {
			seen[t] = true
			uniqueTopics = append(uniqueTopics, t)
		}
		if len(uniqueTopics) >= 20 {
			break
		}
	}

	if len(uniqueTopics) == 0 {
		return nil, nil
	}

	cacheKey := "claude:topicdrift:" + searchQuery
	if cached, ok := c.getCached(cacheKey); ok {
		return parseTopicList(cached), nil
	}

	topicStr := ""
	for i, t := range uniqueTopics {
		if i > 0 {
			topicStr += ", "
		}
		topicStr += t
	}

	prompt := fmt.Sprintf(
		`Given a GitHub search for "%s" that returned repos with these topics: %s

Suggest 3-5 adjacent/related topics the user might also find interesting. Return ONLY a comma-separated list of topic names, nothing else. Be specific and practical.`, searchQuery, topicStr)

	result, err := c.run(ctx, prompt)
	if err != nil {
		return nil, err
	}

	c.setCache(cacheKey, result, topicDriftCacheTTL)
	return parseTopicList(result), nil
}

func parseTopicList(raw string) []string {
	var topics []string
	for _, part := range strings.Split(raw, ",") {
		t := strings.TrimSpace(part)
		t = strings.Trim(t, "\"'`")
		if t != "" {
			topics = append(topics, t)
		}
	}
	return topics
}

package claude

import (
	"context"
	"fmt"
	"time"
)

const (
	summaryCacheTTL    = 7 * 24 * time.Hour // 7 days
	summaryMaxReadmeLen = 4000               // truncate README for Haiku speed
)

// SummarizeReadme asks Claude to distill a README into 2-3 sentences.
// Results are cached by repo full name with a 7-day TTL.
func (c *Client) SummarizeReadme(ctx context.Context, repoFullName, readmeContent string) (string, error) {
	if !c.Available() {
		return "", fmt.Errorf("claude not available")
	}

	cacheKey := "claude:summary:" + repoFullName
	if cached, ok := c.getCached(cacheKey); ok {
		return cached, nil
	}

	// Truncate README for speed and cost
	readme := readmeContent
	if len(readme) > summaryMaxReadmeLen {
		readme = readme[:summaryMaxReadmeLen] + "\n\n[truncated]"
	}

	prompt := fmt.Sprintf(
		`Summarize this GitHub README in 2-3 concise sentences. Focus on: what the project does, who should use it, and what makes it notable. Do not use markdown formatting. Do not start with "This".

Repository: %s

README:
%s`, repoFullName, readme)

	result, err := c.run(ctx, prompt)
	if err != nil {
		return "", err
	}

	c.setCache(cacheKey, result, summaryCacheTTL)
	return result, nil
}

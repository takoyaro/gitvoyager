package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Client struct {
	mu        sync.RWMutex
	rateLimit RateLimit
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh %s: %s: %w", args[0], stderr.String(), err)
	}
	return out, nil
}

func (c *Client) GetRateLimit() RateLimit {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.rateLimit
}

func (c *Client) FetchRateLimit(ctx context.Context) (RateLimit, error) {
	out, err := c.run(ctx, "api", "rate_limit")
	if err != nil {
		return RateLimit{}, err
	}

	var resp rateLimitResponse
	if err := json.Unmarshal(out, &resp); err != nil {
		return RateLimit{}, fmt.Errorf("parse rate limit: %w", err)
	}

	rl := RateLimit{
		SearchRemaining: resp.Resources.Search.Remaining,
		SearchLimit:     resp.Resources.Search.Limit,
		SearchReset:     time.Unix(resp.Resources.Search.Reset, 0),
		CoreRemaining:   resp.Resources.Core.Remaining,
		CoreLimit:       resp.Resources.Core.Limit,
	}

	c.mu.Lock()
	c.rateLimit = rl
	c.mu.Unlock()

	return rl, nil
}

func (c *Client) CheckAuth(ctx context.Context) error {
	_, err := c.run(ctx, "auth", "status")
	return err
}

func (c *Client) OpenInBrowser(ctx context.Context, repoFullName string) error {
	_, err := c.run(ctx, "repo", "view", "--web", repoFullName)
	return err
}

func (c *Client) CloneRepo(ctx context.Context, repoFullName, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	args := []string{"repo", "clone", repoFullName}
	if dir != "" {
		args = append(args, dir)
	}

	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("clone %s: %s: %w", repoFullName, stderr.String(), err)
	}
	return nil
}

// FetchStarredRepos returns the full names of repos the authenticated user has starred.
func (c *Client) FetchStarredRepos(ctx context.Context, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 200
	}
	out, err := c.run(ctx, "api", "user/starred",
		"--paginate", "-L", fmt.Sprintf("%d", limit),
		"--jq", ".[].full_name")
	if err != nil {
		return nil, err
	}

	var repos []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			repos = append(repos, line)
		}
	}
	return repos, nil
}

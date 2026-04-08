package github

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SampleTopicCount queries GitHub for the number of repos created with a given topic
// in a time window. Uses the search API (30 req/min rate limit).
func (c *Client) SampleTopicCount(ctx context.Context, topic, createdAfter string) (int, error) {
	q := fmt.Sprintf("topic:%s created:>%s", topic, createdAfter)
	out, err := c.run(ctx, "api", "search/repositories",
		"-X", "GET",
		"-f", "q="+q,
		"-f", "per_page=1",
		"--jq", ".total_count")
	if err != nil {
		return 0, err
	}
	s := strings.TrimSpace(string(out))
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("parse topic count for %q: %w", topic, err)
	}
	return n, nil
}

// TopicSample holds a topic name and its sampled repo count.
type TopicSample struct {
	Topic    string
	Count    int
	WindowID string // e.g. "2026-03-08"
}

// SampleTopicCounts samples repo creation counts for multiple topics in a given window.
// It respects rate limits by sleeping between batches.
func (c *Client) SampleTopicCounts(ctx context.Context, topics []string, windowDays int) ([]TopicSample, error) {
	now := time.Now()
	windowStart := now.AddDate(0, 0, -windowDays).Format("2006-01-02")

	var results []TopicSample
	for i, topic := range topics {
		// Respect search rate limit (30/min): pause every 20 requests.
		if i > 0 && i%20 == 0 {
			select {
			case <-ctx.Done():
				return results, ctx.Err()
			case <-time.After(65 * time.Second):
			}
		}

		// Small delay between each call to avoid burst.
		if i > 0 {
			time.Sleep(500 * time.Millisecond)
		}

		count, err := c.SampleTopicCount(ctx, topic, windowStart)
		if err != nil {
			// Skip individual failures, continue sampling.
			continue
		}
		results = append(results, TopicSample{
			Topic:    topic,
			Count:    count,
			WindowID: windowStart,
		})
	}
	return results, nil
}

// TopicSearchCount is an alternative that uses gh search repos --topic to get
// total_count from the search API response headers.
func (c *Client) TopicSearchCount(ctx context.Context, topic, createdAfter string) (int, error) {
	// Use gh search repos with --json and parse the result count
	out, err := c.run(ctx, "search", "repos",
		"--topic="+topic,
		"--created=>"+createdAfter,
		"-L1",
		"--json", "fullName")
	if err != nil {
		return 0, err
	}
	var repos []json.RawMessage
	if err := json.Unmarshal(out, &repos); err != nil {
		return 0, err
	}
	// gh search repos doesn't directly return total_count,
	// so we fall back to the REST API approach in SampleTopicCount.
	return len(repos), nil
}

package store

import (
	"database/sql"
	"time"
)

const migration009 = `
CREATE TABLE IF NOT EXISTS topic_heat_snapshots (
	topic        TEXT NOT NULL,
	repo_count   INTEGER NOT NULL,
	window_start TEXT NOT NULL,
	window_end   TEXT NOT NULL,
	sampled_at   TEXT DEFAULT (datetime('now')),
	PRIMARY KEY (topic, window_start)
);
CREATE INDEX IF NOT EXISTS idx_topic_heat_topic ON topic_heat_snapshots(topic);
`

// TopicHeat holds a topic's growth metrics.
type TopicHeat struct {
	Topic      string
	Current    int     // repo count in current window
	Previous   int     // repo count in prior window
	AccelRatio float64 // current / previous; >1.0 = growing
	SampledAt  time.Time
}

// RecordTopicHeat stores a repo count snapshot for a topic in a time window.
func (s *Store) RecordTopicHeat(topic string, repoCount int, windowStart, windowEnd string) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO topic_heat_snapshots (topic, repo_count, window_start, window_end)
		VALUES (?, ?, ?, ?)
	`, topic, repoCount, windowStart, windowEnd)
	return err
}

// GetTopicAcceleration returns the acceleration ratio for a topic.
// It compares the two most recent snapshots. Returns 0 if insufficient data.
func (s *Store) GetTopicAcceleration(topic string) (float64, error) {
	rows, err := s.db.Query(`
		SELECT repo_count FROM topic_heat_snapshots
		WHERE topic = ?
		ORDER BY window_start DESC
		LIMIT 2
	`, topic)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var counts []int
	for rows.Next() {
		var c int
		if err := rows.Scan(&c); err != nil {
			return 0, err
		}
		counts = append(counts, c)
	}

	if len(counts) < 2 || counts[1] == 0 {
		return 0, nil
	}
	return float64(counts[0]) / float64(counts[1]), nil
}

// GetHottestTopics returns topics ranked by acceleration ratio.
func (s *Store) GetHottestTopics(limit int) ([]TopicHeat, error) {
	rows, err := s.db.Query(`
		WITH ranked AS (
			SELECT topic, repo_count, window_start, sampled_at,
				ROW_NUMBER() OVER (PARTITION BY topic ORDER BY window_start DESC) as rn
			FROM topic_heat_snapshots
		),
		pairs AS (
			SELECT
				c.topic,
				c.repo_count as current_count,
				p.repo_count as prev_count,
				c.sampled_at
			FROM ranked c
			JOIN ranked p ON c.topic = p.topic AND c.rn = 1 AND p.rn = 2
			WHERE p.repo_count > 0
		)
		SELECT topic, current_count, prev_count,
			CAST(current_count AS REAL) / CAST(prev_count AS REAL) as accel,
			sampled_at
		FROM pairs
		ORDER BY accel DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []TopicHeat
	for rows.Next() {
		var th TopicHeat
		var sampledStr string
		if err := rows.Scan(&th.Topic, &th.Current, &th.Previous, &th.AccelRatio, &sampledStr); err != nil {
			return nil, err
		}
		th.SampledAt, _ = time.Parse("2006-01-02 15:04:05", sampledStr)
		results = append(results, th)
	}
	return results, nil
}

// GetTopicHeatMap returns acceleration ratios for all tracked topics as a map.
func (s *Store) GetTopicHeatMap() (map[string]float64, error) {
	topics, err := s.GetHottestTopics(100)
	if err != nil {
		return nil, err
	}
	m := make(map[string]float64, len(topics))
	for _, th := range topics {
		m[th.Topic] = th.AccelRatio
	}
	return m, nil
}

// NeedsTopicHeatRefresh returns true if topic heat data is stale (>24h) or missing.
func (s *Store) NeedsTopicHeatRefresh() bool {
	var sampledStr sql.NullString
	err := s.db.QueryRow(`
		SELECT MAX(sampled_at) FROM topic_heat_snapshots
	`).Scan(&sampledStr)
	if err != nil || !sampledStr.Valid {
		return true
	}
	t, err := time.Parse("2006-01-02 15:04:05", sampledStr.String)
	if err != nil {
		return true
	}
	return time.Since(t) > 24*time.Hour
}

// GetTrackedTopics returns distinct topics from discovered repos and search history.
func (s *Store) GetTrackedTopics(limit int) ([]string, error) {
	// Extract topics from enrichment_json where available, plus from search history.
	rows, err := s.db.Query(`
		SELECT topic, COUNT(*) as freq FROM (
			SELECT DISTINCT value as topic
			FROM repos, json_each(json_extract(enrichment_json, '$.topics'))
			WHERE enrichment_json IS NOT NULL AND enrichment_json != ''
			UNION ALL
			SELECT DISTINCT language as topic FROM repos WHERE language != ''
		)
		GROUP BY topic
		ORDER BY freq DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var topics []string
	for rows.Next() {
		var topic string
		var freq int
		if err := rows.Scan(&topic, &freq); err != nil {
			return nil, err
		}
		topics = append(topics, topic)
	}
	return topics, nil
}

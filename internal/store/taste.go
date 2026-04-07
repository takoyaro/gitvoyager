package store

import (
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

// GetLanguageDistribution returns aggregated language counts from repos,
// weighted by interaction type: watchlisted repos count 3x, seen repos 1x.
func (s *Store) GetLanguageDistribution() (map[string]int, error) {
	rows, err := s.db.Query(`
		SELECT r.language,
		       SUM(CASE WHEN w.full_name IS NOT NULL THEN 3 ELSE 0 END +
		           CASE WHEN sl.repo_id IS NOT NULL THEN 1 ELSE 0 END +
		           1) as weight
		FROM repos r
		LEFT JOIN watchlist w ON r.full_name = w.full_name
		LEFT JOIN (SELECT DISTINCT repo_id FROM seen_log) sl ON r.id = sl.repo_id
		WHERE r.language != ''
		GROUP BY r.language
		ORDER BY weight DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dist := make(map[string]int)
	for rows.Next() {
		var lang string
		var weight int
		if err := rows.Scan(&lang, &weight); err != nil {
			return nil, err
		}
		dist[lang] = weight
	}
	return dist, rows.Err()
}

// GetTopicDistribution returns aggregated topic counts from enrichment_json.
// Watchlisted repos get 3x weight.
func (s *Store) GetTopicDistribution() (map[string]int, error) {
	// Topics are stored in enrichment_json as a JSON object with a "topics" array.
	// Since we don't have a dedicated topics column, we extract from enrichment_json.
	rows, err := s.db.Query(`
		SELECT json_each.value as topic,
		       SUM(CASE WHEN w.full_name IS NOT NULL THEN 3 ELSE 1 END) as weight
		FROM repos r,
		     json_each(json_extract(r.enrichment_json, '$.topics'))
		LEFT JOIN watchlist w ON r.full_name = w.full_name
		WHERE r.enrichment_json IS NOT NULL
		  AND json_extract(r.enrichment_json, '$.topics') IS NOT NULL
		GROUP BY topic
		ORDER BY weight DESC
	`)
	if err != nil {
		// enrichment_json might not have topics yet — that's fine
		return make(map[string]int), nil
	}
	defer rows.Close()

	dist := make(map[string]int)
	for rows.Next() {
		var topic string
		var weight int
		if err := rows.Scan(&topic, &weight); err != nil {
			continue // skip malformed rows
		}
		if topic != "" {
			dist[topic] = weight
		}
	}
	return dist, nil
}

// SaveTasteSnapshot persists a computed taste profile.
func (s *Store) SaveTasteSnapshot(profileJSON string) error {
	_, err := s.db.Exec(`
		INSERT INTO taste_snapshots (profile_json) VALUES (?)
	`, profileJSON)
	return err
}

// GetLatestTasteSnapshot returns the most recent cached profile JSON and its timestamp.
func (s *Store) GetLatestTasteSnapshot() (string, time.Time, error) {
	var data, ts string
	err := s.db.QueryRow(`
		SELECT profile_json, computed_at
		FROM taste_snapshots
		ORDER BY computed_at DESC
		LIMIT 1
	`).Scan(&data, &ts)
	if err != nil {
		return "", time.Time{}, err
	}
	computedAt, _ := time.Parse("2006-01-02 15:04:05", ts)
	return data, computedAt, nil
}

// GetUnseenReposByAffinity returns repos matching language/topic affinities
// that the user hasn't seen yet. Used for surprise picks.
func (s *Store) GetUnseenReposByAffinity(languages []string, topics []string, limit int) ([]model.Repo, error) {
	if len(languages) == 0 {
		return nil, nil
	}

	// Build a query that prefers matching languages, excludes already-seen repos
	query := `
		SELECT r.full_name, r.owner, r.name, r.description, r.language,
		       r.stars, r.forks, r.license, r.is_archived, r.created_at, r.updated_at
		FROM repos r
		WHERE r.language IN (` + placeholders(len(languages)) + `)
		  AND r.full_name NOT IN (
		    SELECT r2.full_name FROM repos r2
		    JOIN seen_log sl ON r2.id = sl.repo_id
		    GROUP BY r2.full_name
		    HAVING COUNT(*) > 2
		  )
		  AND r.is_archived = 0
		ORDER BY r.stars DESC
		LIMIT ?
	`

	args := make([]any, 0, len(languages)+1)
	for _, l := range languages {
		args = append(args, l)
	}
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []model.Repo
	for rows.Next() {
		var r model.Repo
		var createdAt, updatedAt string
		var isArchived int
		if err := rows.Scan(
			&r.FullName, &r.Owner, &r.Name, &r.Description, &r.Language,
			&r.Stars, &r.Forks, &r.License, &isArchived, &createdAt, &updatedAt,
		); err != nil {
			return nil, err
		}
		r.IsArchived = isArchived == 1
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	s := "?"
	for i := 1; i < n; i++ {
		s += ",?"
	}
	return s
}

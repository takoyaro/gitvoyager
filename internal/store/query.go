package store

import (
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

// Stats holds aggregate counts across all tables.
type Stats struct {
	ReposDiscovered    int `json:"repos_discovered"`
	ReposWatched       int `json:"repos_watched"`
	ReposSeen          int `json:"repos_seen"`
	SearchesTotal      int `json:"searches_total"`
	SearchesBookmarked int `json:"searches_bookmarked"`
	LocalProjects      int `json:"local_projects"`
	LocalDependencies  int `json:"local_dependencies"`
	StarSnapshots      int `json:"star_snapshots"`
}

// GetStats returns aggregate counts from all tables.
func (s *Store) GetStats() (*Stats, error) {
	var st Stats
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM repos`).Scan(&st.ReposDiscovered)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM watchlist`).Scan(&st.ReposWatched)
	_ = s.db.QueryRow(`SELECT COUNT(DISTINCT repo_id) FROM seen_log`).Scan(&st.ReposSeen)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM search_history`).Scan(&st.SearchesTotal)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM search_history WHERE bookmarked = 1`).Scan(&st.SearchesBookmarked)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM local_projects`).Scan(&st.LocalProjects)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM local_dependencies`).Scan(&st.LocalDependencies)
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM star_snapshots`).Scan(&st.StarSnapshots)
	return &st, nil
}

// GetRepos returns discovered repos, optionally filtered by language, sorted by stars descending.
func (s *Store) GetRepos(limit int, language string) ([]model.Repo, error) {
	if limit <= 0 {
		limit = 100
	}

	query := `
		SELECT r.full_name, r.owner, r.name, r.description, r.language,
		       r.stars, r.forks, r.license, r.is_archived, r.created_at, r.updated_at,
		       r.discovered_at, COALESCE(r.first_seen_stars, r.stars),
		       CASE WHEN w.full_name IS NOT NULL THEN 1 ELSE 0 END
		FROM repos r
		LEFT JOIN watchlist w ON r.full_name = w.full_name
	`

	var args []any
	if language != "" {
		query += ` WHERE r.language = ?`
		args = append(args, language)
	}
	query += ` ORDER BY r.stars DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []model.Repo
	for rows.Next() {
		var r model.Repo
		var createdAt, updatedAt, discoveredAt string
		var isArchived, firstSeenStars, watched int
		if err := rows.Scan(
			&r.FullName, &r.Owner, &r.Name, &r.Description, &r.Language,
			&r.Stars, &r.Forks, &r.License, &isArchived, &createdAt, &updatedAt,
			&discoveredAt, &firstSeenStars, &watched,
		); err != nil {
			return nil, err
		}
		r.IsArchived = isArchived == 1
		r.Watchlisted = watched == 1
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		r.DiscoveredAt = parseDBTime(discoveredAt)
		r.StarDelta = r.Stars - firstSeenStars
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// parseDBTime tries RFC3339 then SQLite's datetime() format.
func parseDBTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, _ = time.Parse("2006-01-02 15:04:05", s)
	}
	return t
}

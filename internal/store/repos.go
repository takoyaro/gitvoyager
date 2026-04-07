package store

import (
	"strings"
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

func (s *Store) UpsertRepo(r model.Repo) error {
	_, err := s.db.Exec(`
		INSERT INTO repos (full_name, owner, name, description, language, stars, forks, license, is_archived, created_at, updated_at, first_seen_stars)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(full_name) DO UPDATE SET
			description    = excluded.description,
			language       = excluded.language,
			stars          = excluded.stars,
			forks          = excluded.forks,
			license        = excluded.license,
			is_archived    = excluded.is_archived,
			updated_at     = excluded.updated_at
	`, r.FullName, r.Owner, r.Name, r.Description, r.Language, r.Stars, r.Forks,
		r.License, r.IsArchived, r.CreatedAt.Format(time.RFC3339), r.UpdatedAt.Format(time.RFC3339), r.Stars)
	return err
}

func (s *Store) UpsertRepos(repos []model.Repo) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO repos (full_name, owner, name, description, language, stars, forks, license, is_archived, created_at, updated_at, first_seen_stars)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(full_name) DO UPDATE SET
			description    = excluded.description,
			language       = excluded.language,
			stars          = excluded.stars,
			forks          = excluded.forks,
			license        = excluded.license,
			is_archived    = excluded.is_archived,
			updated_at     = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range repos {
		_, err := stmt.Exec(r.FullName, r.Owner, r.Name, r.Description, r.Language,
			r.Stars, r.Forks, r.License, r.IsArchived,
			r.CreatedAt.Format(time.RFC3339), r.UpdatedAt.Format(time.RFC3339), r.Stars)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) MarkSeen(repoFullName string) error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO seen_log (repo_id, seen_at)
		SELECT id, datetime('now') FROM repos WHERE full_name = ?
	`, repoFullName)
	return err
}

// ToggleWatch adds or removes a repo from the watchlist.
// Returns true if the repo is now watched, false if unwatched.
func (s *Store) ToggleWatch(fullName string) (bool, error) {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM watchlist WHERE full_name = ?`, fullName).Scan(&count)
	if count > 0 {
		_, err := s.db.Exec(`DELETE FROM watchlist WHERE full_name = ?`, fullName)
		return false, err
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO watchlist (full_name) VALUES (?)`, fullName)
	return err == nil, err
}

// GetWatchlist returns the set of watched repo full names.
func (s *Store) GetWatchlist() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT full_name FROM watchlist`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	watched := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		watched[name] = true
	}
	return watched, rows.Err()
}

func (s *Store) GetSeenRepos() (map[string]time.Time, error) {
	rows, err := s.db.Query(`
		SELECT r.full_name, MAX(s.seen_at)
		FROM seen_log s JOIN repos r ON s.repo_id = r.id
		GROUP BY r.full_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]time.Time)
	for rows.Next() {
		var name, ts string
		if err := rows.Scan(&name, &ts); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, ts)
		seen[name] = t
	}
	return seen, rows.Err()
}

// GetFirstSeenStars returns the first-recorded star count for each given repo.
// Used to compute star velocity deltas.
func (s *Store) GetFirstSeenStars(fullNames []string) (map[string]int, error) {
	if len(fullNames) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(fullNames))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, len(fullNames))
	for i, n := range fullNames {
		args[i] = n
	}

	rows, err := s.db.Query(
		`SELECT full_name, COALESCE(first_seen_stars, stars) FROM repos WHERE full_name IN (`+placeholders+`)`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int, len(fullNames))
	for rows.Next() {
		var name string
		var first int
		if err := rows.Scan(&name, &first); err != nil {
			return nil, err
		}
		result[name] = first
	}
	return result, rows.Err()
}

// GetWatchlistRepos returns full repo data for all watched repos, with StarDelta pre-computed.
func (s *Store) GetWatchlistRepos() ([]model.Repo, error) {
	rows, err := s.db.Query(`
		SELECT r.full_name, r.owner, r.name, r.description, r.language,
		       r.stars, r.forks, r.license, r.is_archived, r.created_at, r.updated_at,
		       COALESCE(r.first_seen_stars, r.stars)
		FROM repos r
		JOIN watchlist w ON r.full_name = w.full_name
		ORDER BY w.watched_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []model.Repo
	for rows.Next() {
		var r model.Repo
		var createdAt, updatedAt string
		var isArchived, firstSeenStars int
		if err := rows.Scan(
			&r.FullName, &r.Owner, &r.Name, &r.Description, &r.Language,
			&r.Stars, &r.Forks, &r.License, &isArchived, &createdAt, &updatedAt,
			&firstSeenStars,
		); err != nil {
			return nil, err
		}
		r.IsArchived = isArchived == 1
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		r.Watchlisted = true
		r.StarDelta = r.Stars - firstSeenStars
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

// GetWatchlistLastViewed returns when the user last viewed the watchlist.
func (s *Store) GetWatchlistLastViewed() (time.Time, error) {
	var ts string
	err := s.db.QueryRow(`
		SELECT last_viewed_at FROM watchlist
		WHERE last_viewed_at != ''
		ORDER BY last_viewed_at DESC
		LIMIT 1
	`).Scan(&ts)
	if err != nil || ts == "" {
		return time.Time{}, err
	}
	t, _ := time.Parse(time.RFC3339, ts)
	return t, nil
}

// UpdateWatchlistViewedAt stamps the current time on all watchlist entries.
func (s *Store) UpdateWatchlistViewedAt() error {
	_, err := s.db.Exec(`
		UPDATE watchlist SET last_viewed_at = datetime('now')
	`)
	return err
}

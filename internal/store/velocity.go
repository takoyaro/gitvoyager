package store

import (
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

// RecordStarSnapshot records a point-in-time star count for a repo.
// Called during repo upserts to build a velocity curve over time.
func (s *Store) RecordStarSnapshot(fullName string, stars int) error {
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO star_snapshots (full_name, stars)
		VALUES (?, ?)
	`, fullName, stars)
	return err
}

// RecordStarSnapshots records snapshots for multiple repos in a transaction.
func (s *Store) RecordStarSnapshots(repos []model.Repo) error {
	if len(repos) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO star_snapshots (full_name, stars) VALUES (?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range repos {
		if _, err := stmt.Exec(r.FullName, r.Stars); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetStarVelocity returns the average stars gained per day over a window.
func (s *Store) GetStarVelocity(fullName string, windowDays int) (float64, error) {
	var oldestStars, newestStars int
	var oldestTime, newestTime string

	cutoff := time.Now().AddDate(0, 0, -windowDays).Format(time.RFC3339)

	// Get oldest snapshot in window
	err := s.db.QueryRow(`
		SELECT stars, recorded_at FROM star_snapshots
		WHERE full_name = ? AND recorded_at >= ?
		ORDER BY recorded_at ASC LIMIT 1
	`, fullName, cutoff).Scan(&oldestStars, &oldestTime)
	if err != nil {
		return 0, err
	}

	// Get newest snapshot
	err = s.db.QueryRow(`
		SELECT stars, recorded_at FROM star_snapshots
		WHERE full_name = ?
		ORDER BY recorded_at DESC LIMIT 1
	`, fullName).Scan(&newestStars, &newestTime)
	if err != nil {
		return 0, err
	}

	oldest, _ := time.Parse(time.RFC3339, oldestTime)
	newest, _ := time.Parse(time.RFC3339, newestTime)

	daysDiff := newest.Sub(oldest).Hours() / 24
	if daysDiff < 0.5 {
		daysDiff = 0.5 // minimum half a day to avoid division spikes
	}

	return float64(newestStars-oldestStars) / daysDiff, nil
}

// GetFastestRising returns repos with the highest star velocity from snapshots.
func (s *Store) GetFastestRising(limit int) ([]model.Repo, error) {
	if limit <= 0 {
		limit = 30
	}

	cutoff := time.Now().AddDate(0, 0, -7).Format(time.RFC3339) // 7-day window

	rows, err := s.db.Query(`
		WITH velocity AS (
			SELECT
				s.full_name,
				MAX(s.stars) - MIN(s.stars) as star_gain,
				(julianday(MAX(s.recorded_at)) - julianday(MIN(s.recorded_at))) as days_span
			FROM star_snapshots s
			WHERE s.recorded_at >= ?
			GROUP BY s.full_name
			HAVING days_span > 0.1 AND star_gain > 0
		)
		SELECT r.full_name, r.owner, r.name, r.description, r.language,
		       r.stars, r.forks, r.license, r.is_archived, r.created_at, r.updated_at,
		       COALESCE(r.first_seen_stars, r.stars),
		       v.star_gain, v.days_span
		FROM velocity v
		JOIN repos r ON r.full_name = v.full_name
		ORDER BY (v.star_gain / v.days_span) DESC
		LIMIT ?
	`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var repos []model.Repo
	for rows.Next() {
		var r model.Repo
		var createdAt, updatedAt string
		var isArchived, firstSeenStars int
		var starGain int
		var daysSpan float64
		if err := rows.Scan(
			&r.FullName, &r.Owner, &r.Name, &r.Description, &r.Language,
			&r.Stars, &r.Forks, &r.License, &isArchived, &createdAt, &updatedAt,
			&firstSeenStars, &starGain, &daysSpan,
		); err != nil {
			return nil, err
		}
		r.IsArchived = isArchived == 1
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		r.StarDelta = r.Stars - firstSeenStars
		repos = append(repos, r)
	}
	return repos, rows.Err()
}

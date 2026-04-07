package store

import (
	"time"

	"github.com/takoyaro/gitvoyager/internal/model"
)

func (s *Store) UpsertRepo(r model.Repo) error {
	_, err := s.db.Exec(`
		INSERT INTO repos (full_name, owner, name, description, language, stars, forks, license, is_archived, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(full_name) DO UPDATE SET
			description = excluded.description,
			language = excluded.language,
			stars = excluded.stars,
			forks = excluded.forks,
			license = excluded.license,
			is_archived = excluded.is_archived,
			updated_at = excluded.updated_at
	`, r.FullName, r.Owner, r.Name, r.Description, r.Language, r.Stars, r.Forks,
		r.License, r.IsArchived, r.CreatedAt.Format(time.RFC3339), r.UpdatedAt.Format(time.RFC3339))
	return err
}

func (s *Store) UpsertRepos(repos []model.Repo) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO repos (full_name, owner, name, description, language, stars, forks, license, is_archived, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(full_name) DO UPDATE SET
			description = excluded.description,
			language = excluded.language,
			stars = excluded.stars,
			forks = excluded.forks,
			license = excluded.license,
			is_archived = excluded.is_archived,
			updated_at = excluded.updated_at
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range repos {
		_, err := stmt.Exec(r.FullName, r.Owner, r.Name, r.Description, r.Language,
			r.Stars, r.Forks, r.License, r.IsArchived,
			r.CreatedAt.Format(time.RFC3339), r.UpdatedAt.Format(time.RFC3339))
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

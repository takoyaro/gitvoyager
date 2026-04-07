package store

import (
	"time"

	"github.com/takoyaro/gitvoyager/internal/local"
)

// SaveProjectFingerprint persists a scanned project and its dependencies.
func (s *Store) SaveProjectFingerprint(fp local.ProjectFingerprint) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Upsert project
	res, err := tx.Exec(`
		INSERT INTO local_projects (path, name, language, scanned_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			name = excluded.name,
			language = excluded.language,
			scanned_at = excluded.scanned_at
	`, fp.Path, fp.Name, fp.Language, fp.ScannedAt.Format(time.RFC3339))
	if err != nil {
		return err
	}

	// Get the project ID
	var projectID int64
	if id, err := res.LastInsertId(); err == nil && id > 0 {
		projectID = id
	} else {
		err := tx.QueryRow(`SELECT id FROM local_projects WHERE path = ?`, fp.Path).Scan(&projectID)
		if err != nil {
			return err
		}
	}

	// Clear old dependencies
	if _, err := tx.Exec(`DELETE FROM local_dependencies WHERE project_id = ?`, projectID); err != nil {
		return err
	}

	// Insert dependencies
	if len(fp.Dependencies) > 0 {
		stmt, err := tx.Prepare(`
			INSERT OR IGNORE INTO local_dependencies (project_id, name, version, source, github_repo)
			VALUES (?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, dep := range fp.Dependencies {
			if _, err := stmt.Exec(projectID, dep.Name, dep.Version, dep.Source, dep.RepoName); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}

// GetLocalDependencies returns all dependencies from scanned projects.
func (s *Store) GetLocalDependencies() ([]local.Dependency, error) {
	rows, err := s.db.Query(`
		SELECT d.name, d.version, d.source, d.github_repo
		FROM local_dependencies d
		ORDER BY d.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []local.Dependency
	for rows.Next() {
		var d local.Dependency
		if err := rows.Scan(&d.Name, &d.Version, &d.Source, &d.RepoName); err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	return deps, rows.Err()
}

// GetProjectLanguages returns language counts from scanned local projects.
func (s *Store) GetProjectLanguages() (map[string]int, error) {
	rows, err := s.db.Query(`
		SELECT language, COUNT(*) as cnt
		FROM local_projects
		WHERE language != ''
		GROUP BY language
		ORDER BY cnt DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	dist := make(map[string]int)
	for rows.Next() {
		var lang string
		var cnt int
		if err := rows.Scan(&lang, &cnt); err != nil {
			return nil, err
		}
		dist[lang] = cnt
	}
	return dist, rows.Err()
}

// GetLocalProjectCount returns the number of scanned projects.
func (s *Store) GetLocalProjectCount() int {
	var count int
	_ = s.db.QueryRow(`SELECT COUNT(*) FROM local_projects`).Scan(&count)
	return count
}

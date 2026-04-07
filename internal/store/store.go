package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)



type Store struct {
	db *sql.DB
}

func New(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(migration001); err != nil {
		return err
	}
	if _, err := s.db.Exec(migration002); err != nil {
		return err
	}
	if err := s.migration003(); err != nil {
		return err
	}
	if _, err := s.db.Exec(migration004); err != nil {
		return err
	}
	if _, err := s.db.Exec(migration005); err != nil {
		return err
	}
	if err := s.migration006(); err != nil {
		return err
	}
	_, err := s.db.Exec(migration007)
	return err
}

const migration007 = `
CREATE TABLE IF NOT EXISTS star_snapshots (
	full_name   TEXT NOT NULL,
	stars       INTEGER NOT NULL,
	recorded_at TEXT DEFAULT (datetime('now')),
	PRIMARY KEY (full_name, recorded_at)
);
CREATE INDEX IF NOT EXISTS idx_star_snapshots_time ON star_snapshots(recorded_at DESC);
`

const migration005 = `
CREATE TABLE IF NOT EXISTS local_projects (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	path        TEXT NOT NULL UNIQUE,
	name        TEXT DEFAULT '',
	language    TEXT DEFAULT '',
	scanned_at  TEXT DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS local_dependencies (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id   INTEGER NOT NULL REFERENCES local_projects(id),
	name         TEXT NOT NULL,
	version      TEXT DEFAULT '',
	source       TEXT DEFAULT '',
	github_repo  TEXT DEFAULT '',
	UNIQUE(project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_local_deps_repo ON local_dependencies(github_repo) WHERE github_repo != '';
`

const migration004 = `
CREATE TABLE IF NOT EXISTS taste_snapshots (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	profile_json TEXT NOT NULL,
	computed_at  TEXT DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_taste_snapshots_time ON taste_snapshots(computed_at DESC);
`

// migration003 adds first_seen_stars for tracking star velocity.
// Uses addColumnIfNotExists so it's safe to run multiple times.
func (s *Store) migration003() error {
	if err := s.addColumnIfNotExists("repos", "first_seen_stars", "INTEGER DEFAULT 0"); err != nil {
		return err
	}
	// Seed existing rows: treat their current star count as the baseline.
	_, err := s.db.Exec(`UPDATE repos SET first_seen_stars = stars WHERE first_seen_stars = 0 AND stars > 0`)
	return err
}

func (s *Store) migration006() error {
	return s.addColumnIfNotExists("watchlist", "last_viewed_at", "TEXT DEFAULT ''")
}

func (s *Store) addColumnIfNotExists(table, column, def string) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	_, err := s.db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, def))
	return err
}

const migration002 = `
CREATE TABLE IF NOT EXISTS watchlist (
	full_name  TEXT PRIMARY KEY,
	watched_at TEXT DEFAULT (datetime('now'))
);
`

const migration001 = `
CREATE TABLE IF NOT EXISTS repos (
	id              INTEGER PRIMARY KEY,
	full_name       TEXT NOT NULL UNIQUE,
	owner           TEXT NOT NULL,
	name            TEXT NOT NULL,
	description     TEXT DEFAULT '',
	language        TEXT DEFAULT '',
	stars           INTEGER DEFAULT 0,
	forks           INTEGER DEFAULT 0,
	license         TEXT DEFAULT '',
	is_archived     INTEGER DEFAULT 0,
	created_at      TEXT NOT NULL,
	updated_at      TEXT NOT NULL,
	discovered_at   TEXT DEFAULT (datetime('now')),
	enrichment_json TEXT
);

CREATE TABLE IF NOT EXISTS seen_log (
	repo_id  INTEGER NOT NULL REFERENCES repos(id),
	seen_at  TEXT DEFAULT (datetime('now')),
	PRIMARY KEY (repo_id, seen_at)
);

CREATE TABLE IF NOT EXISTS api_cache (
	cache_key   TEXT PRIMARY KEY,
	response    BLOB NOT NULL,
	etag        TEXT DEFAULT '',
	cached_at   TEXT DEFAULT (datetime('now')),
	expires_at  TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS search_history (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	query       TEXT NOT NULL,
	sort_field  TEXT DEFAULT 'stars',
	language    TEXT DEFAULT '',
	result_count INTEGER DEFAULT 0,
	searched_at TEXT DEFAULT (datetime('now')),
	bookmarked  INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_search_history_time ON search_history(searched_at DESC);
CREATE INDEX IF NOT EXISTS idx_search_history_bookmarked ON search_history(bookmarked) WHERE bookmarked = 1;
`

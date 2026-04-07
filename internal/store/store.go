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
	_, err := s.db.Exec(migration002)
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

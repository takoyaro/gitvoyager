package store

import "strings"

const migration008 = `
CREATE TABLE IF NOT EXISTS exclusions (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	kind       TEXT NOT NULL CHECK(kind IN ('keyword','topic','owner')),
	value      TEXT NOT NULL,
	created_at TEXT DEFAULT (datetime('now')),
	UNIQUE(kind, value)
);
CREATE INDEX IF NOT EXISTS idx_exclusions_kind ON exclusions(kind);
`

// ExclusionSet holds all active exclusions grouped by kind.
type ExclusionSet struct {
	Keywords []string
	Topics   []string
	Owners   []string
}

// Count returns the total number of active exclusions.
func (e *ExclusionSet) Count() int {
	if e == nil {
		return 0
	}
	return len(e.Keywords) + len(e.Topics) + len(e.Owners)
}

// AddExclusion inserts an exclusion. Duplicates are silently ignored.
func (s *Store) AddExclusion(kind, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	_, err := s.db.Exec(`INSERT OR IGNORE INTO exclusions (kind, value) VALUES (?, ?)`, kind, value)
	return err
}

// RemoveExclusion deletes an exclusion by kind and value.
func (s *Store) RemoveExclusion(kind, value string) error {
	_, err := s.db.Exec(`DELETE FROM exclusions WHERE kind = ? AND value = ?`, kind, value)
	return err
}

// GetAllExclusions returns all exclusions grouped by kind.
func (s *Store) GetAllExclusions() (*ExclusionSet, error) {
	rows, err := s.db.Query(`SELECT kind, value FROM exclusions ORDER BY kind, value`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	set := &ExclusionSet{}
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return nil, err
		}
		switch kind {
		case "keyword":
			set.Keywords = append(set.Keywords, value)
		case "topic":
			set.Topics = append(set.Topics, value)
		case "owner":
			set.Owners = append(set.Owners, value)
		}
	}
	return set, rows.Err()
}

// ExclusionCount returns the total number of stored exclusions.
func (s *Store) ExclusionCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM exclusions`).Scan(&n)
	return n, err
}

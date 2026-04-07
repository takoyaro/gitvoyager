package store

import "time"

type SavedSearch struct {
	ID          int64     `json:"id"`
	Query       string    `json:"query"`
	SortField   string    `json:"sort_field"`
	Language    string    `json:"language,omitempty"`
	ResultCount int       `json:"result_count"`
	SearchedAt  time.Time `json:"searched_at"`
	Bookmarked  bool      `json:"bookmarked,omitempty"`
}

func (s *Store) SaveSearch(query, sortField, language string, resultCount int) error {
	_, err := s.db.Exec(`
		INSERT INTO search_history (query, sort_field, language, result_count)
		VALUES (?, ?, ?, ?)
	`, query, sortField, language, resultCount)
	return err
}

func (s *Store) RecentSearches(limit int) ([]SavedSearch, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.Query(`
		SELECT id, query, sort_field, language, result_count, searched_at, bookmarked
		FROM search_history
		ORDER BY searched_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearches(rows)
}

func (s *Store) BookmarkedSearches() ([]SavedSearch, error) {
	rows, err := s.db.Query(`
		SELECT id, query, sort_field, language, result_count, searched_at, bookmarked
		FROM search_history
		WHERE bookmarked = 1
		ORDER BY searched_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearches(rows)
}

func (s *Store) ToggleSearchBookmark(id int64) error {
	_, err := s.db.Exec(`
		UPDATE search_history SET bookmarked = CASE WHEN bookmarked = 1 THEN 0 ELSE 1 END
		WHERE id = ?
	`, id)
	return err
}

func (s *Store) DeleteSearch(id int64) error {
	_, err := s.db.Exec(`DELETE FROM search_history WHERE id = ?`, id)
	return err
}

func scanSearches(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]SavedSearch, error) {
	var searches []SavedSearch
	for rows.Next() {
		var ss SavedSearch
		var ts string
		var bm int
		if err := rows.Scan(&ss.ID, &ss.Query, &ss.SortField, &ss.Language, &ss.ResultCount, &ts, &bm); err != nil {
			return nil, err
		}
		ss.SearchedAt, _ = time.Parse("2006-01-02 15:04:05", ts)
		ss.Bookmarked = bm == 1
		searches = append(searches, ss)
	}
	return searches, rows.Err()
}

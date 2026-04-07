package store

import "time"

func (s *Store) GetCached(key string) ([]byte, bool) {
	var data []byte
	err := s.db.QueryRow(`
		SELECT response FROM api_cache
		WHERE cache_key = ? AND expires_at > datetime('now')
	`, key).Scan(&data)
	if err != nil {
		return nil, false
	}
	return data, true
}

func (s *Store) SetCached(key string, response []byte, ttl time.Duration, etag string) error {
	expiresAt := time.Now().Add(ttl).UTC().Format(time.RFC3339)
	_, err := s.db.Exec(`
		INSERT INTO api_cache (cache_key, response, etag, expires_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			response = excluded.response,
			etag = excluded.etag,
			cached_at = datetime('now'),
			expires_at = excluded.expires_at
	`, key, response, etag, expiresAt)
	return err
}

func (s *Store) GetETag(key string) string {
	var etag string
	s.db.QueryRow(`SELECT etag FROM api_cache WHERE cache_key = ?`, key).Scan(&etag)
	return etag
}

func (s *Store) PruneExpiredCache() error {
	_, err := s.db.Exec(`DELETE FROM api_cache WHERE expires_at < datetime('now')`)
	return err
}

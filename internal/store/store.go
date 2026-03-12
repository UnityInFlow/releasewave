// Package store provides SQLite-based persistence for ReleaseWave.
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps a SQLite database for ReleaseWave persistence.
type Store struct {
	db *sql.DB
}

// Release is a stored release record.
type Release struct {
	Service      string    `json:"service"`
	Tag          string    `json:"tag"`
	Platform     string    `json:"platform"`
	URL          string    `json:"url"`
	PublishedAt  time.Time `json:"published_at"`
	DiscoveredAt time.Time `json:"discovered_at"`
}

// ToolCall is a stored tool call record.
type ToolCall struct {
	Tool       string        `json:"tool"`
	Args       string        `json:"args"`
	Status     string        `json:"status"`
	DurationMs int64         `json:"duration_ms"`
	CalledAt   time.Time     `json:"called_at"`
	Duration   time.Duration `json:"-"`
}

// New opens or creates a SQLite database at path and runs migrations.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return s, nil
}

func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS releases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			service TEXT NOT NULL,
			tag TEXT NOT NULL,
			platform TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL DEFAULT '',
			published_at DATETIME,
			discovered_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(service, tag)
		)`,
		`CREATE TABLE IF NOT EXISTS tool_calls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tool TEXT NOT NULL,
			args TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'ok',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			called_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS kv_store (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT '',
			updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_releases_service ON releases(service)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_tool ON tool_calls(tool)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_calls_called_at ON tool_calls(called_at)`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			return fmt.Errorf("exec %q: %w", m[:40], err)
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// RecordRelease inserts or ignores a release record.
func (s *Store) RecordRelease(r Release) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO releases (service, tag, platform, url, published_at, discovered_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.Service, r.Tag, r.Platform, r.URL, r.PublishedAt, r.DiscoveredAt,
	)
	return err
}

// GetHistory returns release history for a service, newest first.
func (s *Store) GetHistory(service string, limit int) ([]Release, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(
		`SELECT service, tag, platform, url, published_at, discovered_at
		 FROM releases WHERE service = ? ORDER BY discovered_at DESC LIMIT ?`,
		service, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var releases []Release
	for rows.Next() {
		var r Release
		if err := rows.Scan(&r.Service, &r.Tag, &r.Platform, &r.URL, &r.PublishedAt, &r.DiscoveredAt); err != nil {
			return nil, err
		}
		releases = append(releases, r)
	}
	return releases, rows.Err()
}

// LogToolCall records a tool invocation.
func (s *Store) LogToolCall(tc ToolCall) error {
	_, err := s.db.Exec(
		`INSERT INTO tool_calls (tool, args, status, duration_ms, called_at) VALUES (?, ?, ?, ?, ?)`,
		tc.Tool, tc.Args, tc.Status, tc.DurationMs, tc.CalledAt,
	)
	return err
}

// GetKV retrieves a value from the key-value store.
func (s *Store) GetKV(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM kv_store WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetKV stores a value in the key-value store (upsert).
func (s *Store) SetKV(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO kv_store (key, value, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	return err
}

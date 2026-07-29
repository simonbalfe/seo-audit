package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	DefaultSnapshotRetention = 100
	DefaultRankRunRetention  = 100
)

type CacheEntry struct {
	Key             string
	Provider        string
	DatasetGroup    string
	Target          string
	Location        string
	Language        string
	RowLimit        int
	ResponseJSON    []byte
	FetchedAt       time.Time
	ExpiresAt       time.Time
	ProviderCostUSD float64
}

type Snapshot struct {
	Provider              string
	DatasetGroup          string
	Target                string
	Location              string
	Language              string
	RowLimit              int
	RetrievedAt           time.Time
	CacheHit              bool
	ProviderCostUSD       float64
	CachedProviderCostUSD float64
	ResultJSON            []byte
}

type ReportSnapshot struct {
	ID          int64
	Kind        string
	Target      string
	Source      string
	RetrievedAt time.Time
	RecordedAt  time.Time
	ResultJSON  []byte
}

type ProviderSnapshot struct {
	ID           int64
	Provider     string
	DatasetGroup string
	Target       string
	Location     string
	Language     string
	RetrievedAt  time.Time
	RecordedAt   time.Time
	ResultJSON   []byte
}

type ProviderStore interface {
	LoadCache(context.Context, string, time.Time) (CacheEntry, bool, error)
	SaveCache(context.Context, CacheEntry) error
	SaveSnapshot(context.Context, Snapshot) (int64, error)
}

type SQLiteStore struct {
	db                *sql.DB
	path              string
	snapshotRetention int
}

func DefaultPath() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("SEOAUDIT_DB_PATH")); configured != "" {
		return configured, nil
	}
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user config directory: %w", err)
	}
	return filepath.Join(configDir, "seoaudit", "seoaudit.db"), nil
}

func OpenSQLite(path string, snapshotRetention int) (*SQLiteStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("SQLite database path is empty")
	}
	if snapshotRetention <= 0 {
		snapshotRetention = DefaultSnapshotRetention
	}
	if path != ":memory:" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve SQLite database path: %w", err)
		}
		path = absolute
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("create SQLite database directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	store := &SQLiteStore{
		db:                db,
		path:              path,
		snapshotRetention: snapshotRetention,
	}
	if err := store.initialize(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	if path != ":memory:" {
		if err := os.Chmod(path, 0o600); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("secure SQLite database permissions: %w", err)
		}
	}
	return store, nil
}

func (s *SQLiteStore) initialize(ctx context.Context) error {
	statements := []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS provider_cache (
			cache_key TEXT PRIMARY KEY,
			provider TEXT NOT NULL,
			dataset_group TEXT NOT NULL,
			target TEXT NOT NULL,
			location TEXT NOT NULL,
			language TEXT NOT NULL,
			row_limit INTEGER NOT NULL,
			response_json BLOB NOT NULL,
			fetched_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			provider_cost_usd REAL NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS provider_cache_expiry_idx
			ON provider_cache (expires_at)`,
		`CREATE TABLE IF NOT EXISTS provider_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL,
			dataset_group TEXT NOT NULL,
			target TEXT NOT NULL,
			location TEXT NOT NULL,
			language TEXT NOT NULL,
			row_limit INTEGER NOT NULL,
			retrieved_at TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			cache_hit INTEGER NOT NULL,
			provider_cost_usd REAL NOT NULL,
			cached_provider_cost_usd REAL NOT NULL,
			result_json BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS provider_snapshots_lookup_idx
			ON provider_snapshots (provider, dataset_group, target, recorded_at DESC)`,
		`CREATE TABLE IF NOT EXISTS report_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			kind TEXT NOT NULL,
			target TEXT NOT NULL,
			source TEXT NOT NULL,
			retrieved_at TEXT NOT NULL,
			recorded_at TEXT NOT NULL,
			result_json BLOB NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS report_snapshots_lookup_idx
			ON report_snapshots (kind, target, recorded_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS rank_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target TEXT NOT NULL,
			location TEXT NOT NULL,
			language TEXT NOT NULL,
			devices TEXT NOT NULL,
			serp_depth INTEGER NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (target, location, language)
		)`,
		`CREATE TABLE IF NOT EXISTS rank_keywords (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			config_id INTEGER NOT NULL REFERENCES rank_configs(id) ON DELETE CASCADE,
			keyword TEXT NOT NULL,
			normalized_keyword TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE (config_id, normalized_keyword)
		)`,
		`CREATE INDEX IF NOT EXISTS rank_keywords_config_idx
			ON rank_keywords (config_id, normalized_keyword)`,
		`CREATE TABLE IF NOT EXISTS rank_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			config_id INTEGER NOT NULL REFERENCES rank_configs(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			status TEXT NOT NULL,
			requested_tasks INTEGER NOT NULL,
			successful_tasks INTEGER NOT NULL,
			live_calls INTEGER NOT NULL,
			cost_usd REAL NOT NULL,
			started_at TEXT NOT NULL,
			completed_at TEXT,
			error_message TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS rank_runs_config_started_idx
			ON rank_runs (config_id, started_at DESC, id DESC)`,
		`CREATE TABLE IF NOT EXISTS rank_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL REFERENCES rank_runs(id) ON DELETE CASCADE,
			keyword_id INTEGER NOT NULL,
			keyword TEXT NOT NULL,
			device TEXT NOT NULL,
			position INTEGER,
			ranking_url TEXT NOT NULL,
			serp_features_json TEXT NOT NULL,
			checked_at TEXT NOT NULL,
			UNIQUE (run_id, keyword_id, device)
		)`,
		`CREATE INDEX IF NOT EXISTS rank_results_keyword_device_idx
			ON rank_results (keyword_id, device, checked_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize SQLite database: %w", err)
		}
	}
	if _, err := s.db.ExecContext(
		ctx,
		`DELETE FROM provider_cache WHERE expires_at <= ?`,
		formatTime(time.Now().UTC()),
	); err != nil {
		return fmt.Errorf("remove expired provider cache entries: %w", err)
	}
	return nil
}

func (s *SQLiteStore) LoadCache(ctx context.Context, key string, now time.Time) (CacheEntry, bool, error) {
	var entry CacheEntry
	var fetchedAt string
	var expiresAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT cache_key, provider, dataset_group, target, location, language,
			row_limit, response_json, fetched_at, expires_at, provider_cost_usd
		FROM provider_cache
		WHERE cache_key = ? AND expires_at > ?`,
		key,
		formatTime(now.UTC()),
	).Scan(
		&entry.Key,
		&entry.Provider,
		&entry.DatasetGroup,
		&entry.Target,
		&entry.Location,
		&entry.Language,
		&entry.RowLimit,
		&entry.ResponseJSON,
		&fetchedAt,
		&expiresAt,
		&entry.ProviderCostUSD,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return CacheEntry{}, false, nil
	}
	if err != nil {
		return CacheEntry{}, false, fmt.Errorf("load provider cache: %w", err)
	}
	entry.FetchedAt, err = time.Parse(time.RFC3339Nano, fetchedAt)
	if err != nil {
		return CacheEntry{}, false, fmt.Errorf("parse provider cache fetched time: %w", err)
	}
	entry.ExpiresAt, err = time.Parse(time.RFC3339Nano, expiresAt)
	if err != nil {
		return CacheEntry{}, false, fmt.Errorf("parse provider cache expiry: %w", err)
	}
	return entry, true, nil
}

func (s *SQLiteStore) SaveCache(ctx context.Context, entry CacheEntry) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO provider_cache (
			cache_key, provider, dataset_group, target, location, language,
			row_limit, response_json, fetched_at, expires_at, provider_cost_usd, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(cache_key) DO UPDATE SET
			response_json = excluded.response_json,
			fetched_at = excluded.fetched_at,
			expires_at = excluded.expires_at,
			provider_cost_usd = excluded.provider_cost_usd,
			updated_at = excluded.updated_at`,
		entry.Key,
		entry.Provider,
		entry.DatasetGroup,
		entry.Target,
		entry.Location,
		entry.Language,
		entry.RowLimit,
		entry.ResponseJSON,
		formatTime(entry.FetchedAt.UTC()),
		formatTime(entry.ExpiresAt.UTC()),
		entry.ProviderCostUSD,
		formatTime(time.Now().UTC()),
	)
	if err != nil {
		return fmt.Errorf("save provider cache: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveSnapshot(ctx context.Context, snapshot Snapshot) (int64, error) {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO provider_snapshots (
			provider, dataset_group, target, location, language, row_limit,
			retrieved_at, recorded_at, cache_hit, provider_cost_usd,
			cached_provider_cost_usd, result_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.Provider,
		snapshot.DatasetGroup,
		snapshot.Target,
		snapshot.Location,
		snapshot.Language,
		snapshot.RowLimit,
		formatTime(snapshot.RetrievedAt.UTC()),
		formatTime(time.Now().UTC()),
		snapshot.CacheHit,
		snapshot.ProviderCostUSD,
		snapshot.CachedProviderCostUSD,
		snapshot.ResultJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("save provider snapshot: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read provider snapshot id: %w", err)
	}
	if _, err := s.db.ExecContext(
		ctx,
		`DELETE FROM provider_snapshots
		WHERE id IN (
			SELECT id
			FROM provider_snapshots
			WHERE provider = ? AND dataset_group = ? AND target = ?
			ORDER BY recorded_at DESC, id DESC
			LIMIT -1 OFFSET ?
		)`,
		snapshot.Provider,
		snapshot.DatasetGroup,
		snapshot.Target,
		s.snapshotRetention,
	); err != nil {
		return 0, fmt.Errorf("prune provider snapshots: %w", err)
	}
	return id, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) Path() string {
	return s.path
}

func formatTime(value time.Time) string {
	return value.Format(time.RFC3339Nano)
}

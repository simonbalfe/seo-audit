package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type classificationCache struct {
	database *sql.DB
}

func openClassificationCache(ctx context.Context, databasePath string) (*classificationCache, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create classification cache directory: %w", err)
	}
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open classification cache: %w", err)
	}
	database.SetMaxOpenConns(1)
	for _, statement := range []string{
		`PRAGMA busy_timeout = 5000`,
		`CREATE TABLE IF NOT EXISTS visibility_page_research (
			url TEXT NOT NULL,
			model TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			page_type TEXT NOT NULL,
			reason TEXT NOT NULL,
			keyword_seeds TEXT NOT NULL,
			classified_at TEXT NOT NULL,
			PRIMARY KEY (url, model)
		)`,
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			closeErr := database.Close()
			return nil, errors.Join(fmt.Errorf("initialize classification cache: %w", err), closeErr)
		}
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		closeErr := database.Close()
		return nil, errors.Join(fmt.Errorf("secure classification cache: %w", err), closeErr)
	}
	return &classificationCache{database: database}, nil
}

func (c *classificationCache) load(ctx context.Context, page classificationInput) (classification, bool, error) {
	fingerprint, err := classificationFingerprint(page)
	if err != nil {
		return classification{}, false, err
	}
	result := classification{ID: page.ID}
	var keywordSeeds string
	err = c.database.QueryRowContext(ctx, `
		SELECT page_type, reason, keyword_seeds
		FROM visibility_page_research
		WHERE url = ? AND model = ? AND fingerprint = ?
	`, page.URL, openRouterModel, fingerprint).Scan(&result.Type, &result.Reason, &keywordSeeds)
	if errors.Is(err, sql.ErrNoRows) {
		return classification{}, false, nil
	}
	if err != nil {
		return classification{}, false, fmt.Errorf("read classification cache: %w", err)
	}
	if err := json.Unmarshal([]byte(keywordSeeds), &result.KeywordSeeds); err != nil {
		return classification{}, false, fmt.Errorf("decode cached keyword seeds: %w", err)
	}
	return result, true, nil
}

func (c *classificationCache) save(ctx context.Context, inputs map[int]classificationInput, classifications []classification) error {
	if len(classifications) == 0 {
		return nil
	}
	transaction, err := c.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin classification cache update: %w", err)
	}
	rollback := func(cause error) error {
		return errors.Join(cause, transaction.Rollback())
	}
	classifiedAt := time.Now().UTC().Format(time.RFC3339Nano)
	for _, result := range classifications {
		page, exists := inputs[result.ID]
		if !exists {
			continue
		}
		fingerprint, err := classificationFingerprint(page)
		if err != nil {
			return rollback(err)
		}
		keywordSeeds, err := json.Marshal(result.KeywordSeeds)
		if err != nil {
			return rollback(fmt.Errorf("encode keyword seeds: %w", err))
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO visibility_page_research (url, model, fingerprint, page_type, reason, keyword_seeds, classified_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (url, model) DO UPDATE SET
				fingerprint = excluded.fingerprint,
				page_type = excluded.page_type,
				reason = excluded.reason,
				keyword_seeds = excluded.keyword_seeds,
				classified_at = excluded.classified_at
		`, page.URL, openRouterModel, fingerprint, result.Type, result.Reason, string(keywordSeeds), classifiedAt); err != nil {
			return rollback(fmt.Errorf("write classification cache: %w", err))
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit classification cache: %w", err)
	}
	return nil
}

func (c *classificationCache) close() error {
	if err := c.database.Close(); err != nil {
		return fmt.Errorf("close classification cache: %w", err)
	}
	return nil
}

func classificationFingerprint(page classificationInput) (string, error) {
	page.ID = 0
	encoded, err := json.Marshal(struct {
		Version string              `json:"version"`
		Page    classificationInput `json:"page"`
	}{classificationPromptVersion, page})
	if err != nil {
		return "", fmt.Errorf("fingerprint classification metadata: %w", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

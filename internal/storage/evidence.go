package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/ranktracking"
)

func (s *SQLiteStore) SaveReportSnapshot(ctx context.Context, snapshot ReportSnapshot) (int64, error) {
	snapshot.Kind = strings.ToLower(strings.TrimSpace(snapshot.Kind))
	snapshot.Target = strings.ToLower(strings.TrimSpace(snapshot.Target))
	snapshot.Source = strings.TrimSpace(snapshot.Source)
	if snapshot.Kind == "" || snapshot.Target == "" || snapshot.Source == "" {
		return 0, errors.New("report snapshot kind, target, and source are required")
	}
	if len(snapshot.ResultJSON) == 0 {
		return 0, errors.New("report snapshot JSON is empty")
	}
	if snapshot.RetrievedAt.IsZero() {
		snapshot.RetrievedAt = time.Now().UTC()
	}
	recordedAt := time.Now().UTC()
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO report_snapshots (
			kind, target, source, retrieved_at, recorded_at, result_json
		) VALUES (?, ?, ?, ?, ?, ?)`,
		snapshot.Kind,
		snapshot.Target,
		snapshot.Source,
		formatTime(snapshot.RetrievedAt.UTC()),
		formatTime(recordedAt),
		snapshot.ResultJSON,
	)
	if err != nil {
		return 0, fmt.Errorf("save report snapshot: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read report snapshot id: %w", err)
	}
	if _, err := s.db.ExecContext(
		ctx,
		`DELETE FROM report_snapshots
		WHERE id IN (
			SELECT id
			FROM report_snapshots
			WHERE kind = ? AND target = ?
			ORDER BY recorded_at DESC, id DESC
			LIMIT -1 OFFSET ?
		)`,
		snapshot.Kind,
		snapshot.Target,
		s.snapshotRetention,
	); err != nil {
		return 0, fmt.Errorf("prune report snapshots: %w", err)
	}
	return id, nil
}

func (s *SQLiteStore) LatestReportSnapshot(ctx context.Context, kind, target string) (ReportSnapshot, bool, error) {
	var snapshot ReportSnapshot
	var retrievedAt string
	var recordedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, kind, target, source, retrieved_at, recorded_at, result_json
		FROM report_snapshots
		WHERE kind = ? AND target = ?
		ORDER BY recorded_at DESC, id DESC
		LIMIT 1`,
		strings.ToLower(strings.TrimSpace(kind)),
		strings.ToLower(strings.TrimSpace(target)),
	).Scan(
		&snapshot.ID,
		&snapshot.Kind,
		&snapshot.Target,
		&snapshot.Source,
		&retrievedAt,
		&recordedAt,
		&snapshot.ResultJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ReportSnapshot{}, false, nil
	}
	if err != nil {
		return ReportSnapshot{}, false, fmt.Errorf("load latest report snapshot: %w", err)
	}
	snapshot.RetrievedAt, err = time.Parse(time.RFC3339Nano, retrievedAt)
	if err != nil {
		return ReportSnapshot{}, false, fmt.Errorf("parse report snapshot retrieval time: %w", err)
	}
	snapshot.RecordedAt, err = time.Parse(time.RFC3339Nano, recordedAt)
	if err != nil {
		return ReportSnapshot{}, false, fmt.Errorf("parse report snapshot recording time: %w", err)
	}
	return snapshot, true, nil
}

func (s *SQLiteStore) LatestProviderSnapshot(ctx context.Context, datasetGroup, target string) (ProviderSnapshot, bool, error) {
	var snapshot ProviderSnapshot
	var retrievedAt string
	var recordedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, provider, dataset_group, target, location, language,
			retrieved_at, recorded_at, result_json
		FROM provider_snapshots
		WHERE dataset_group = ? AND target = ?
		ORDER BY recorded_at DESC, id DESC
		LIMIT 1`,
		strings.ToLower(strings.TrimSpace(datasetGroup)),
		strings.ToLower(strings.TrimSpace(target)),
	).Scan(
		&snapshot.ID,
		&snapshot.Provider,
		&snapshot.DatasetGroup,
		&snapshot.Target,
		&snapshot.Location,
		&snapshot.Language,
		&retrievedAt,
		&recordedAt,
		&snapshot.ResultJSON,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ProviderSnapshot{}, false, nil
	}
	if err != nil {
		return ProviderSnapshot{}, false, fmt.Errorf("load latest provider snapshot: %w", err)
	}
	snapshot.RetrievedAt, err = time.Parse(time.RFC3339Nano, retrievedAt)
	if err != nil {
		return ProviderSnapshot{}, false, fmt.Errorf("parse provider snapshot retrieval time: %w", err)
	}
	snapshot.RecordedAt, err = time.Parse(time.RFC3339Nano, recordedAt)
	if err != nil {
		return ProviderSnapshot{}, false, fmt.Errorf("parse provider snapshot recording time: %w", err)
	}
	return snapshot, true, nil
}

func (s *SQLiteStore) ListEvidenceTargets(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT target FROM report_snapshots
		UNION
		SELECT target FROM provider_snapshots
		UNION
		SELECT target FROM rank_configs
		ORDER BY target`,
	)
	if err != nil {
		return nil, fmt.Errorf("list dashboard targets: %w", err)
	}
	defer rows.Close()
	targets := make([]string, 0)
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			return nil, fmt.Errorf("scan dashboard target: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard targets: %w", err)
	}
	sort.Strings(targets)
	return targets, nil
}

func (s *SQLiteStore) ListRankConfigsByTarget(ctx context.Context, target string) ([]ranktracking.Config, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, target, location, language, devices, serp_depth, created_at, updated_at
		FROM rank_configs
		WHERE target = ?
		ORDER BY location, language`,
		strings.ToLower(strings.TrimSpace(target)),
	)
	if err != nil {
		return nil, fmt.Errorf("list rank tracking configs: %w", err)
	}
	defer rows.Close()
	configs := make([]ranktracking.Config, 0)
	for rows.Next() {
		var config ranktracking.Config
		var createdAt string
		var updatedAt string
		if err := rows.Scan(
			&config.ID,
			&config.Target,
			&config.Location,
			&config.Language,
			&config.Devices,
			&config.SERPDepth,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan rank tracking config: %w", err)
		}
		config.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse rank config creation time: %w", err)
		}
		config.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse rank config update time: %w", err)
		}
		configs = append(configs, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rank tracking configs: %w", err)
	}
	return configs, nil
}

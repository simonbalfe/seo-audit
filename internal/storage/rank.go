package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/ranktracking"
)

func (s *SQLiteStore) UpsertRankConfig(ctx context.Context, config ranktracking.Config) (ranktracking.Config, error) {
	normalized, err := ranktracking.NormalizeConfig(config)
	if err != nil {
		return ranktracking.Config{}, err
	}
	now := formatTime(time.Now().UTC())
	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO rank_configs (
			target, location, language, devices, serp_depth, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target, location, language) DO UPDATE SET
			devices = excluded.devices,
			serp_depth = excluded.serp_depth,
			updated_at = excluded.updated_at`,
		normalized.Target,
		normalized.Location,
		normalized.Language,
		normalized.Devices,
		normalized.SERPDepth,
		now,
		now,
	)
	if err != nil {
		return ranktracking.Config{}, fmt.Errorf("save rank tracking config: %w", err)
	}
	return s.GetRankConfig(ctx, normalized.Target, normalized.Location, normalized.Language)
}

func (s *SQLiteStore) GetRankConfig(ctx context.Context, target, location, language string) (ranktracking.Config, error) {
	normalized, err := ranktracking.NormalizeConfig(ranktracking.Config{
		Target:    target,
		Location:  location,
		Language:  language,
		Devices:   ranktracking.DefaultDevice,
		SERPDepth: ranktracking.DefaultDepth,
	})
	if err != nil {
		return ranktracking.Config{}, err
	}
	var config ranktracking.Config
	var createdAt string
	var updatedAt string
	err = s.db.QueryRowContext(
		ctx,
		`SELECT id, target, location, language, devices, serp_depth, created_at, updated_at
		FROM rank_configs
		WHERE target = ? AND location = ? AND language = ?`,
		normalized.Target,
		normalized.Location,
		normalized.Language,
	).Scan(
		&config.ID,
		&config.Target,
		&config.Location,
		&config.Language,
		&config.Devices,
		&config.SERPDepth,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ranktracking.Config{}, ranktracking.ErrTrackerNotFound
	}
	if err != nil {
		return ranktracking.Config{}, fmt.Errorf("load rank tracking config: %w", err)
	}
	config.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ranktracking.Config{}, fmt.Errorf("parse rank config creation time: %w", err)
	}
	config.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ranktracking.Config{}, fmt.Errorf("parse rank config update time: %w", err)
	}
	return config, nil
}

func (s *SQLiteStore) AddRankKeywords(ctx context.Context, configID int64, values []string, limit int) (int, error) {
	keywords, err := ranktracking.NormalizeKeywords(values)
	if err != nil {
		return 0, err
	}
	if limit <= 0 {
		limit = ranktracking.MaxKeywords
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin rank keyword update: %w", err)
	}
	defer tx.Rollback()

	existingRows, err := tx.QueryContext(
		ctx,
		`SELECT normalized_keyword FROM rank_keywords WHERE config_id = ?`,
		configID,
	)
	if err != nil {
		return 0, fmt.Errorf("load tracked keywords: %w", err)
	}
	existing := make(map[string]bool)
	for existingRows.Next() {
		var keyword string
		if err := existingRows.Scan(&keyword); err != nil {
			existingRows.Close()
			return 0, fmt.Errorf("scan tracked keyword: %w", err)
		}
		existing[keyword] = true
	}
	if err := existingRows.Close(); err != nil {
		return 0, fmt.Errorf("close tracked keyword rows: %w", err)
	}
	if err := existingRows.Err(); err != nil {
		return 0, fmt.Errorf("iterate tracked keywords: %w", err)
	}

	newCount := 0
	for _, keyword := range keywords {
		if !existing[strings.ToLower(keyword)] {
			newCount++
		}
	}
	if len(existing)+newCount > limit {
		return 0, fmt.Errorf("rank tracker supports at most %d keywords; this would create %d", limit, len(existing)+newCount)
	}

	added := 0
	now := formatTime(time.Now().UTC())
	for _, keyword := range keywords {
		result, err := tx.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO rank_keywords (
				config_id, keyword, normalized_keyword, created_at
			) VALUES (?, ?, ?, ?)`,
			configID,
			keyword,
			strings.ToLower(keyword),
			now,
		)
		if err != nil {
			return 0, fmt.Errorf("add tracked keyword: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count added tracked keywords: %w", err)
		}
		added += int(affected)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tracked keywords: %w", err)
	}
	return added, nil
}

func (s *SQLiteStore) RemoveRankKeywords(ctx context.Context, configID int64, values []string) (int, error) {
	keywords, err := ranktracking.NormalizeKeywords(values)
	if err != nil {
		return 0, err
	}
	placeholders := make([]string, len(keywords))
	args := make([]any, 0, len(keywords)+1)
	args = append(args, configID)
	for index, keyword := range keywords {
		placeholders[index] = "?"
		args = append(args, strings.ToLower(keyword))
	}
	result, err := s.db.ExecContext(
		ctx,
		`DELETE FROM rank_keywords
		WHERE config_id = ? AND normalized_keyword IN (`+strings.Join(placeholders, ",")+`)`,
		args...,
	)
	if err != nil {
		return 0, fmt.Errorf("remove tracked keywords: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count removed tracked keywords: %w", err)
	}
	return int(affected), nil
}

func (s *SQLiteStore) ListRankKeywords(ctx context.Context, configID int64) ([]ranktracking.Keyword, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, config_id, keyword, created_at
		FROM rank_keywords
		WHERE config_id = ?
		ORDER BY normalized_keyword`,
		configID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tracked keywords: %w", err)
	}
	defer rows.Close()
	keywords := make([]ranktracking.Keyword, 0)
	for rows.Next() {
		var keyword ranktracking.Keyword
		var createdAt string
		if err := rows.Scan(&keyword.ID, &keyword.ConfigID, &keyword.Keyword, &createdAt); err != nil {
			return nil, fmt.Errorf("scan tracked keyword: %w", err)
		}
		keyword.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse tracked keyword creation time: %w", err)
		}
		keywords = append(keywords, keyword)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tracked keywords: %w", err)
	}
	return keywords, nil
}

func (s *SQLiteStore) StartRankRun(ctx context.Context, configID int64, provider string, requestedTasks int) (ranktracking.Run, error) {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO rank_runs (
			config_id, provider, status, requested_tasks, successful_tasks,
			live_calls, cost_usd, started_at, error_message
		) VALUES (?, ?, 'running', ?, 0, 0, 0, ?, '')`,
		configID,
		provider,
		requestedTasks,
		formatTime(now),
	)
	if err != nil {
		return ranktracking.Run{}, fmt.Errorf("start rank check: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return ranktracking.Run{}, fmt.Errorf("read rank check id: %w", err)
	}
	return ranktracking.Run{
		ID:             id,
		ConfigID:       configID,
		Provider:       provider,
		Status:         "running",
		RequestedTasks: requestedTasks,
		StartedAt:      now,
	}, nil
}

func (s *SQLiteStore) FinishRankRun(ctx context.Context, run ranktracking.Run, results []ranktracking.Result) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rank check completion: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	for _, result := range results {
		featuresJSON, err := json.Marshal(result.SERPFeatures)
		if err != nil {
			return fmt.Errorf("encode rank result features: %w", err)
		}
		var position any
		if result.Position != nil {
			position = *result.Position
		}
		_, err = tx.ExecContext(
			ctx,
			`INSERT INTO rank_results (
				run_id, keyword_id, keyword, device, position,
				ranking_url, serp_features_json, checked_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(run_id, keyword_id, device) DO UPDATE SET
				position = excluded.position,
				ranking_url = excluded.ranking_url,
				serp_features_json = excluded.serp_features_json,
				checked_at = excluded.checked_at`,
			run.ID,
			result.KeywordID,
			result.Keyword,
			result.Device,
			position,
			result.RankingURL,
			string(featuresJSON),
			formatTime(now),
		)
		if err != nil {
			return fmt.Errorf("save rank result: %w", err)
		}
	}
	status := "completed"
	if run.SuccessfulTasks < run.RequestedTasks {
		status = "partial"
	}
	_, err = tx.ExecContext(
		ctx,
		`UPDATE rank_runs
		SET status = ?, successful_tasks = ?, live_calls = ?, cost_usd = ?,
			completed_at = ?, error_message = ?
		WHERE id = ?`,
		status,
		run.SuccessfulTasks,
		run.LiveCalls,
		run.CostUSD,
		formatTime(now),
		run.ErrorMessage,
		run.ID,
	)
	if err != nil {
		return fmt.Errorf("complete rank check: %w", err)
	}
	_, err = tx.ExecContext(
		ctx,
		`DELETE FROM rank_runs
		WHERE id IN (
			SELECT id FROM rank_runs
			WHERE config_id = ?
			ORDER BY started_at DESC, id DESC
			LIMIT -1 OFFSET ?
		)`,
		run.ConfigID,
		DefaultRankRunRetention,
	)
	if err != nil {
		return fmt.Errorf("prune rank check history: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rank check: %w", err)
	}
	return nil
}

func (s *SQLiteStore) FailRankRun(ctx context.Context, run ranktracking.Run) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE rank_runs
		SET status = 'failed', successful_tasks = ?, live_calls = ?, cost_usd = ?,
			completed_at = ?, error_message = ?
		WHERE id = ?`,
		run.SuccessfulTasks,
		run.LiveCalls,
		run.CostUSD,
		formatTime(now),
		run.ErrorMessage,
		run.ID,
	)
	if err != nil {
		return fmt.Errorf("fail rank check: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetRankReport(ctx context.Context, configID int64) (ranktracking.Report, error) {
	config, err := s.rankConfigByID(ctx, configID)
	if err != nil {
		return ranktracking.Report{}, err
	}
	keywords, err := s.ListRankKeywords(ctx, configID)
	if err != nil {
		return ranktracking.Report{}, err
	}
	runs, err := s.latestComparableRankRuns(ctx, configID, 2)
	if err != nil {
		return ranktracking.Report{}, err
	}
	report := ranktracking.Report{
		Config:   config,
		Keywords: keywords,
		Summary: ranktracking.Summary{
			TrackedKeywords: len(keywords),
			TrackedTasks:    len(keywords) * len(ranktracking.Devices(config.Devices)),
		},
	}
	if len(runs) == 0 {
		report.Rows = emptyRankRows(keywords, config.Devices)
		report.Summary.NotChecked = len(report.Rows)
		return report, nil
	}
	report.LatestRun = &runs[0]
	if len(runs) > 1 {
		previousID := runs[1].ID
		report.PreviousRunID = &previousID
	}

	current, err := s.rankResultsForRun(ctx, runs[0].ID)
	if err != nil {
		return ranktracking.Report{}, err
	}
	previous := make(map[string]storedRankResult)
	if len(runs) > 1 {
		previous, err = s.rankResultsForRun(ctx, runs[1].ID)
		if err != nil {
			return ranktracking.Report{}, err
		}
	}
	for _, keyword := range keywords {
		for _, device := range ranktracking.Devices(config.Devices) {
			key := rankResultKey(keyword.ID, device)
			currentResult, observed := current[key]
			previousResult, previousObserved := previous[key]
			row := ranktracking.Row{
				Keyword:          keyword.Keyword,
				Device:           device,
				Observed:         observed,
				Position:         currentResult.Position,
				PreviousPosition: previousResult.Position,
				PreviousObserved: previousObserved,
				RankingURL:       currentResult.RankingURL,
				SERPFeatures:     currentResult.SERPFeatures,
				Change: ranktracking.ClassifyChange(
					currentResult.Position,
					previousResult.Position,
					observed,
					previousObserved,
				),
			}
			report.Rows = append(report.Rows, row)
			updateRankSummary(&report.Summary, row)
		}
	}
	sortRankRows(report.Rows)
	return report, nil
}

func (s *SQLiteStore) rankConfigByID(ctx context.Context, configID int64) (ranktracking.Config, error) {
	var config ranktracking.Config
	var createdAt string
	var updatedAt string
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, target, location, language, devices, serp_depth, created_at, updated_at
		FROM rank_configs WHERE id = ?`,
		configID,
	).Scan(
		&config.ID,
		&config.Target,
		&config.Location,
		&config.Language,
		&config.Devices,
		&config.SERPDepth,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ranktracking.Config{}, ranktracking.ErrTrackerNotFound
	}
	if err != nil {
		return ranktracking.Config{}, fmt.Errorf("load rank tracking config: %w", err)
	}
	config.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return ranktracking.Config{}, fmt.Errorf("parse rank config creation time: %w", err)
	}
	config.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return ranktracking.Config{}, fmt.Errorf("parse rank config update time: %w", err)
	}
	return config, nil
}

func (s *SQLiteStore) latestComparableRankRuns(ctx context.Context, configID int64, limit int) ([]ranktracking.Run, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT id, config_id, provider, status, requested_tasks, successful_tasks,
			live_calls, cost_usd, started_at, completed_at, error_message
		FROM rank_runs
		WHERE config_id = ? AND status IN ('completed', 'partial')
		ORDER BY started_at DESC, id DESC
		LIMIT ?`,
		configID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("load rank check history: %w", err)
	}
	defer rows.Close()
	runs := make([]ranktracking.Run, 0, limit)
	for rows.Next() {
		run, err := scanRankRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rank check history: %w", err)
	}
	return runs, nil
}

type rowScanner interface {
	Scan(...any) error
}

func scanRankRun(scanner rowScanner) (ranktracking.Run, error) {
	var run ranktracking.Run
	var startedAt string
	var completedAt sql.NullString
	err := scanner.Scan(
		&run.ID,
		&run.ConfigID,
		&run.Provider,
		&run.Status,
		&run.RequestedTasks,
		&run.SuccessfulTasks,
		&run.LiveCalls,
		&run.CostUSD,
		&startedAt,
		&completedAt,
		&run.ErrorMessage,
	)
	if err != nil {
		return ranktracking.Run{}, fmt.Errorf("scan rank check: %w", err)
	}
	run.StartedAt, err = time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return ranktracking.Run{}, fmt.Errorf("parse rank check start time: %w", err)
	}
	if completedAt.Valid {
		value, err := time.Parse(time.RFC3339Nano, completedAt.String)
		if err != nil {
			return ranktracking.Run{}, fmt.Errorf("parse rank check completion time: %w", err)
		}
		run.CompletedAt = &value
	}
	return run, nil
}

type storedRankResult struct {
	Position     *int
	RankingURL   string
	SERPFeatures []string
}

func (s *SQLiteStore) rankResultsForRun(ctx context.Context, runID int64) (map[string]storedRankResult, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT keyword_id, device, position, ranking_url, serp_features_json
		FROM rank_results WHERE run_id = ?`,
		runID,
	)
	if err != nil {
		return nil, fmt.Errorf("load rank results: %w", err)
	}
	defer rows.Close()
	results := make(map[string]storedRankResult)
	for rows.Next() {
		var keywordID int64
		var device string
		var position sql.NullInt64
		var rankingURL string
		var featuresJSON string
		if err := rows.Scan(&keywordID, &device, &position, &rankingURL, &featuresJSON); err != nil {
			return nil, fmt.Errorf("scan rank result: %w", err)
		}
		var positionValue *int
		if position.Valid {
			value := int(position.Int64)
			positionValue = &value
		}
		var features []string
		if err := json.Unmarshal([]byte(featuresJSON), &features); err != nil {
			return nil, fmt.Errorf("decode rank result features: %w", err)
		}
		results[rankResultKey(keywordID, device)] = storedRankResult{
			Position:     positionValue,
			RankingURL:   rankingURL,
			SERPFeatures: features,
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rank results: %w", err)
	}
	return results, nil
}

func emptyRankRows(keywords []ranktracking.Keyword, devices string) []ranktracking.Row {
	rows := make([]ranktracking.Row, 0, len(keywords)*len(ranktracking.Devices(devices)))
	for _, keyword := range keywords {
		for _, device := range ranktracking.Devices(devices) {
			rows = append(rows, ranktracking.Row{
				Keyword: keyword.Keyword,
				Device:  device,
				Change:  "not-checked",
			})
		}
	}
	return rows
}

func rankResultKey(keywordID int64, device string) string {
	return fmt.Sprintf("%d:%s", keywordID, device)
}

func updateRankSummary(summary *ranktracking.Summary, row ranktracking.Row) {
	if !row.Observed {
		summary.NotChecked++
	} else {
		summary.Checked++
		if row.Position == nil {
			summary.NotRanking++
		} else {
			summary.Ranking++
			if *row.Position <= 3 {
				summary.Top3++
			}
			if *row.Position <= 10 {
				summary.Top10++
			}
		}
	}
	switch row.Change {
	case "improved":
		summary.Improved++
	case "declined":
		summary.Declined++
	case "new":
		summary.New++
	case "lost":
		summary.Lost++
	case "stable":
		summary.Stable++
	case "uncompared":
		summary.Uncompared++
	}
}

func sortRankRows(rows []ranktracking.Row) {
	order := map[string]int{
		"lost":        0,
		"declined":    1,
		"improved":    2,
		"new":         3,
		"stable":      4,
		"uncompared":  5,
		"not-checked": 6,
	}
	sort.Slice(rows, func(i, j int) bool {
		if order[rows[i].Change] != order[rows[j].Change] {
			return order[rows[i].Change] < order[rows[j].Change]
		}
		if rows[i].Position != nil && rows[j].Position != nil && *rows[i].Position != *rows[j].Position {
			return *rows[i].Position < *rows[j].Position
		}
		if rows[i].Keyword != rows[j].Keyword {
			return strings.ToLower(rows[i].Keyword) < strings.ToLower(rows[j].Keyword)
		}
		return rows[i].Device < rows[j].Device
	})
}

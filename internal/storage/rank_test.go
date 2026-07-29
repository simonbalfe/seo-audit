package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/simonbalfe/seo-audit/internal/ranktracking"
)

func TestRankTrackingHistoryAndComparisons(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "rank.db"), 10)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	update, err := ranktracking.Add(ctx, store, ranktracking.Config{
		Target:    "example.com",
		Location:  "United Kingdom",
		Language:  "en",
		Devices:   "desktop",
		SERPDepth: 100,
	}, []string{"alpha", "beta", "ALPHA"})
	if err != nil {
		t.Fatalf("add keywords: %v", err)
	}
	if update.Added != 2 || update.TotalKeywords != 2 {
		t.Fatalf("unexpected keyword update: %#v", update)
	}
	alpha := update.Keywords[0]
	beta := update.Keywords[1]

	first, err := store.StartRankRun(ctx, update.Config.ID, ranktracking.ProviderDataForSEO, 2)
	if err != nil {
		t.Fatalf("start first run: %v", err)
	}
	first.SuccessfulTasks = 2
	first.LiveCalls = 2
	first.CostUSD = 0.004
	if err := store.FinishRankRun(ctx, first, []ranktracking.Result{
		{KeywordID: alpha.ID, Keyword: alpha.Keyword, Device: "desktop", Position: rankInt(8), RankingURL: "https://example.com/alpha"},
		{KeywordID: beta.ID, Keyword: beta.Keyword, Device: "desktop"},
	}); err != nil {
		t.Fatalf("finish first run: %v", err)
	}

	second, err := store.StartRankRun(ctx, update.Config.ID, ranktracking.ProviderDataForSEO, 2)
	if err != nil {
		t.Fatalf("start second run: %v", err)
	}
	second.SuccessfulTasks = 2
	second.LiveCalls = 2
	second.CostUSD = 0.004
	if err := store.FinishRankRun(ctx, second, []ranktracking.Result{
		{KeywordID: alpha.ID, Keyword: alpha.Keyword, Device: "desktop", Position: rankInt(5), RankingURL: "https://example.com/alpha", SERPFeatures: []string{"organic", "people_also_ask"}},
		{KeywordID: beta.ID, Keyword: beta.Keyword, Device: "desktop", Position: rankInt(25), RankingURL: "https://example.com/beta"},
	}); err != nil {
		t.Fatalf("finish second run: %v", err)
	}

	report, err := store.GetRankReport(ctx, update.Config.ID)
	if err != nil {
		t.Fatalf("get report: %v", err)
	}
	if report.LatestRun == nil || report.LatestRun.ID != second.ID || report.PreviousRunID == nil || *report.PreviousRunID != first.ID {
		t.Fatalf("unexpected run comparison: %#v", report)
	}
	if report.Summary.Ranking != 2 || report.Summary.Top10 != 1 || report.Summary.Improved != 1 || report.Summary.New != 1 {
		t.Fatalf("unexpected comparison summary: %#v", report.Summary)
	}
	if len(report.Rows[0].SERPFeatures) != 2 {
		t.Fatalf("SERP features were not preserved: %#v", report.Rows)
	}

	third, err := store.StartRankRun(ctx, update.Config.ID, ranktracking.ProviderDataForSEO, 2)
	if err != nil {
		t.Fatalf("start third run: %v", err)
	}
	third.SuccessfulTasks = 1
	third.LiveCalls = 2
	third.CostUSD = 0.004
	third.ErrorMessage = "beta failed"
	if err := store.FinishRankRun(ctx, third, []ranktracking.Result{
		{KeywordID: alpha.ID, Keyword: alpha.Keyword, Device: "desktop"},
	}); err != nil {
		t.Fatalf("finish third run: %v", err)
	}

	report, err = store.GetRankReport(ctx, update.Config.ID)
	if err != nil {
		t.Fatalf("get partial report: %v", err)
	}
	if report.LatestRun == nil || report.LatestRun.Status != "partial" || report.Summary.Lost != 1 || report.Summary.NotChecked != 1 {
		t.Fatalf("unexpected partial summary: %#v", report)
	}

	removed, err := ranktracking.Remove(ctx, store, "example.com", "United Kingdom", "en", []string{"beta"})
	if err != nil {
		t.Fatalf("remove keyword: %v", err)
	}
	if removed.Removed != 1 || removed.TotalKeywords != 1 {
		t.Fatalf("unexpected removal: %#v", removed)
	}
	var historicalBetaRows int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM rank_results WHERE keyword_id = ?`, beta.ID).Scan(&historicalBetaRows); err != nil {
		t.Fatalf("count historical beta rows: %v", err)
	}
	if historicalBetaRows != 2 {
		t.Fatalf("historical beta rows = %d, want 2", historicalBetaRows)
	}
}

func TestRankKeywordLimit(t *testing.T) {
	ctx := context.Background()
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "limit.db"), 10)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	config, err := store.UpsertRankConfig(ctx, ranktracking.Config{Target: "example.com"})
	if err != nil {
		t.Fatalf("add config: %v", err)
	}
	if _, err := store.AddRankKeywords(ctx, config.ID, []string{"one", "two", "three"}, 2); err == nil {
		t.Fatal("expected keyword limit error")
	}
}

func rankInt(value int) *int {
	return &value
}

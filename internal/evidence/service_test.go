package evidence

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/simonbalfe/seo-audit/internal/dataforseo"
	"github.com/simonbalfe/seo-audit/internal/ranktracking"
	"github.com/simonbalfe/seo-audit/internal/storage"
)

func TestServiceJoinsSavedSiteEvidence(t *testing.T) {
	store, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "dashboard.db"), 10)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	retrievedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	auditJSON, err := json.Marshal(audit.SiteReport{
		StartURL: "https://example.com/",
		Summary:  audit.Summary{Pages: 2, Indexable: 2},
		Pages: []audit.PageReport{{
			URL:        "https://example.com/",
			StatusCode: 200,
			Indexable:  true,
			Title:      "Example",
		}},
	})
	if err != nil {
		t.Fatalf("encode audit: %v", err)
	}
	if _, err := store.SaveReportSnapshot(ctx, storage.ReportSnapshot{
		Kind:        "audit",
		Target:      "example.com",
		Source:      "public crawl",
		RetrievedAt: retrievedAt,
		ResultJSON:  auditJSON,
	}); err != nil {
		t.Fatalf("save audit: %v", err)
	}
	searchJSON, err := json.Marshal(dataforseo.Report{
		Available:    true,
		Source:       "DataForSEO",
		DatasetGroup: "search",
		Target:       "example.com",
		RetrievedAt:  retrievedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("encode search: %v", err)
	}
	if _, err := store.SaveSnapshot(ctx, storage.Snapshot{
		Provider:     "DataForSEO",
		DatasetGroup: "search",
		Target:       "example.com",
		RetrievedAt:  retrievedAt.Add(time.Hour),
		ResultJSON:   searchJSON,
	}); err != nil {
		t.Fatalf("save search: %v", err)
	}
	config, err := store.UpsertRankConfig(ctx, ranktracking.Config{
		Target:    "example.com",
		Location:  ranktracking.DefaultLocation,
		Language:  ranktracking.DefaultLanguage,
		Devices:   ranktracking.DefaultDevice,
		SERPDepth: ranktracking.DefaultDepth,
	})
	if err != nil {
		t.Fatalf("save rank config: %v", err)
	}
	if _, err := store.AddRankKeywords(ctx, config.ID, []string{"example keyword"}, 100); err != nil {
		t.Fatalf("save rank keyword: %v", err)
	}
	service := NewService(store)
	response, err := service.GetSite(ctx, "https://www.example.com")
	if err != nil {
		t.Fatalf("load site: %v", err)
	}
	if response.Target != "example.com" || response.Audit == nil || response.Search == nil || len(response.Rankings) != 1 {
		t.Fatalf("unexpected response: %#v", response)
	}
	if response.Audit.Summary.Pages != 2 || len(response.Audit.Pages) != 1 {
		t.Fatalf("unexpected audit: %#v", response.Audit)
	}
	if response.LastUpdated.Before(retrievedAt.Add(time.Hour)) {
		t.Fatalf("last updated = %s", response.LastUpdated)
	}
	sites, err := service.ListSites(ctx)
	if err != nil || len(sites.Sites) != 1 || !sites.Sites[0].HasAudit || !sites.Sites[0].HasSearch {
		t.Fatalf("unexpected sites: %#v err=%v", sites, err)
	}
}

package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/simonbalfe/seo-audit/internal/ranktracking"
)

func TestEvidenceSnapshotsAndTargets(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "evidence.db"), 2)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	retrievedAt := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	for index := range 3 {
		_, err := store.SaveReportSnapshot(ctx, ReportSnapshot{
			Kind:        "audit",
			Target:      "example.com",
			Source:      "public crawl",
			RetrievedAt: retrievedAt.Add(time.Duration(index) * time.Hour),
			ResultJSON:  []byte(`{"summary":{"pages":1}}`),
		})
		if err != nil {
			t.Fatalf("save report snapshot: %v", err)
		}
	}
	latest, found, err := store.LatestReportSnapshot(ctx, "audit", "example.com")
	if err != nil || !found {
		t.Fatalf("load report snapshot: found=%t err=%v", found, err)
	}
	if !latest.RetrievedAt.Equal(retrievedAt.Add(2 * time.Hour)) {
		t.Fatalf("latest retrieval time = %s", latest.RetrievedAt)
	}
	var retained int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM report_snapshots`).Scan(&retained); err != nil {
		t.Fatalf("count report snapshots: %v", err)
	}
	if retained != 2 {
		t.Fatalf("retained snapshots = %d, want 2", retained)
	}
	if _, err := store.SaveSnapshot(ctx, Snapshot{
		Provider:     "DataForSEO",
		DatasetGroup: "backlinks",
		Target:       "links.example",
		RetrievedAt:  retrievedAt,
		ResultJSON:   []byte(`{"available":true}`),
	}); err != nil {
		t.Fatalf("save provider snapshot: %v", err)
	}
	if _, err := store.UpsertRankConfig(ctx, ranktracking.Config{
		Target:    "ranks.example",
		Location:  ranktracking.DefaultLocation,
		Language:  ranktracking.DefaultLanguage,
		Devices:   ranktracking.DefaultDevice,
		SERPDepth: ranktracking.DefaultDepth,
	}); err != nil {
		t.Fatalf("save rank config: %v", err)
	}
	targets, err := store.ListEvidenceTargets(ctx)
	if err != nil {
		t.Fatalf("list targets: %v", err)
	}
	if len(targets) != 3 || targets[0] != "example.com" || targets[1] != "links.example" || targets[2] != "ranks.example" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
	provider, found, err := store.LatestProviderSnapshot(ctx, "backlinks", "links.example")
	if err != nil || !found || provider.DatasetGroup != "backlinks" {
		t.Fatalf("unexpected provider snapshot: found=%t snapshot=%#v err=%v", found, provider, err)
	}
	configs, err := store.ListRankConfigsByTarget(ctx, "ranks.example")
	if err != nil || len(configs) != 1 {
		t.Fatalf("unexpected configs: %#v err=%v", configs, err)
	}
}

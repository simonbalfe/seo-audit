package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSQLiteCacheRoundTripAndExpiry(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "cache.db"), 10)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	entry := CacheEntry{
		Key:             "cache-key",
		Provider:        "DataForSEO",
		DatasetGroup:    "search",
		Target:          "example.com",
		Location:        "United Kingdom",
		Language:        "en",
		RowLimit:        25,
		ResponseJSON:    []byte(`{"available":true}`),
		FetchedAt:       now,
		ExpiresAt:       now.Add(6 * time.Hour),
		ProviderCostUSD: 0.04,
	}
	if err := store.SaveCache(context.Background(), entry); err != nil {
		t.Fatalf("save cache: %v", err)
	}

	loaded, found, err := store.LoadCache(context.Background(), entry.Key, now.Add(time.Hour))
	if err != nil {
		t.Fatalf("load cache: %v", err)
	}
	if !found || string(loaded.ResponseJSON) != string(entry.ResponseJSON) || loaded.ProviderCostUSD != entry.ProviderCostUSD {
		t.Fatalf("unexpected cache entry: found=%t entry=%#v", found, loaded)
	}

	_, found, err = store.LoadCache(context.Background(), entry.Key, now.Add(7*time.Hour))
	if err != nil {
		t.Fatalf("load expired cache: %v", err)
	}
	if found {
		t.Fatal("expired cache entry was returned")
	}
}

func TestSQLiteSnapshotRetention(t *testing.T) {
	store, err := OpenSQLite(filepath.Join(t.TempDir(), "snapshots.db"), 2)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	for index := 0; index < 3; index++ {
		_, err := store.SaveSnapshot(context.Background(), Snapshot{
			Provider:     "DataForSEO",
			DatasetGroup: "backlinks",
			Target:       "example.com",
			RowLimit:     25,
			RetrievedAt:  time.Date(2026, 7, 28, 12, index, 0, 0, time.UTC),
			ResultJSON:   []byte(`{"available":true}`),
		})
		if err != nil {
			t.Fatalf("save snapshot %d: %v", index, err)
		}
	}

	var count int
	err = store.db.QueryRow(
		`SELECT COUNT(*) FROM provider_snapshots
		WHERE provider = ? AND dataset_group = ? AND target = ?`,
		"DataForSEO",
		"backlinks",
		"example.com",
	).Scan(&count)
	if err != nil {
		t.Fatalf("count snapshots: %v", err)
	}
	if count != 2 {
		t.Fatalf("snapshot count = %d, want 2", count)
	}
}

package dataforseo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/simonbalfe/seo-audit/internal/storage"
)

func TestNewClientReadsEnvironmentCredentials(t *testing.T) {
	t.Setenv("DATAFORSEO_USERNAME", "api-login")
	t.Setenv("DATAFORSEO_PASSWORD", "api-password")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if client.Username != "api-login" || client.Password != "api-password" {
		t.Fatalf("unexpected credentials: username=%q password=%q", client.Username, client.Password)
	}
}

func TestNewClientRequiresEnvironmentCredentials(t *testing.T) {
	t.Setenv("DATAFORSEO_USERNAME", "")
	t.Setenv("DATAFORSEO_PASSWORD", "")

	_, err := NewClient()
	if err == nil || !strings.Contains(err.Error(), "DATAFORSEO_USERNAME and DATAFORSEO_PASSWORD") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearchAndBacklinksCollectTheirDatasets(t *testing.T) {
	results := map[string]any{
		"/v3/dataforseo_labs/google/domain_rank_overview/live": []any{
			map[string]any{"items": []any{map[string]any{"metrics": map[string]any{"organic": map[string]any{
				"count": 12, "etv": 34.5, "estimated_paid_traffic_cost": 90.2,
				"pos_1": 1, "pos_2_3": 2, "pos_4_10": 3, "pos_11_20": 4, "pos_21_30": 2,
			}}}}},
		},
		"/v3/dataforseo_labs/google/ranked_keywords/live": []any{
			map[string]any{"items": []any{map[string]any{
				"keyword_data": map[string]any{
					"keyword":            "seo audit",
					"keyword_info":       map[string]any{"search_volume": 1000, "cpc": 4.2},
					"keyword_properties": map[string]any{"keyword_difficulty": 31},
					"search_intent_info": map[string]any{"main_intent": "commercial"},
				},
				"ranked_serp_element": map[string]any{
					"last_updated_time": "2026-07-28",
					"serp_item": map[string]any{
						"rank_absolute": 8,
						"url":           "https://example.com/audit",
						"etv":           20.5,
						"rank_changes":  map[string]any{"previous_rank_absolute": 11},
					},
				},
			}}},
		},
		"/v3/dataforseo_labs/google/keywords_for_site/live": []any{
			map[string]any{"items": []any{map[string]any{
				"keyword":            "website seo checker",
				"keyword_info":       map[string]any{"search_volume": 500, "cpc": 2.5, "competition": 0.4, "competition_level": "MEDIUM"},
				"keyword_properties": map[string]any{"keyword_difficulty": 22},
				"search_intent_info": map[string]any{"main_intent": "commercial"},
			}}},
		},
		"/v3/dataforseo_labs/google/competitors_domain/live": []any{
			map[string]any{"items": []any{
				map[string]any{
					"domain":              "example.com",
					"intersections":       12,
					"avg_position":        8,
					"full_domain_metrics": map[string]any{"organic": map[string]any{"count": 12, "etv": 34.5}},
				},
				map[string]any{
					"domain":              "competitor.example",
					"intersections":       7,
					"avg_position":        12.5,
					"full_domain_metrics": map[string]any{"organic": map[string]any{"count": 900, "etv": 1200.5}},
				},
			}},
		},
		"/v3/backlinks/summary/live": []any{
			map[string]any{
				"rank": 51, "backlinks": 80, "backlinks_spam_score": 3, "referring_domains": 10,
				"referring_main_domains": 9, "referring_pages": 30, "referring_pages_nofollow": 5,
				"referring_ips": 8, "broken_backlinks": 2, "broken_pages": 1, "crawled_pages": 200,
				"info": map[string]any{"target_spam_score": 1},
			},
		},
		"/v3/backlinks/referring_domains/live": []any{
			map[string]any{"items": []any{map[string]any{
				"domain": "source.example", "rank": 70, "backlinks": 4, "referring_pages": 3,
				"referring_pages_nofollow": 1, "backlinks_spam_score": 0,
			}}},
		},
		"/v3/backlinks/backlinks/live": []any{
			map[string]any{"items": []any{map[string]any{
				"url_from": "https://source.example/page", "domain_from": "source.example",
				"url_to": "https://example.com/", "anchor": "example", "dofollow": true,
				"rank": 88, "domain_from_rank": 70, "page_from_status_code": 200, "url_to_status_code": 200,
			}}},
		},
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		username, password, ok := request.BasicAuth()
		if !ok || username != "user" || password != "pass" {
			t.Errorf("unexpected authorization")
		}
		var payload []map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if len(payload) != 1 || payload[0]["target"] != "example.com" {
			t.Errorf("unexpected payload: %#v", payload)
		}
		result, exists := results[request.URL.Path]
		if !exists {
			http.Error(writer, "unknown endpoint", http.StatusNotFound)
			return
		}
		response := map[string]any{
			"status_code": 20000,
			"cost":        0.01,
			"tasks": []any{map[string]any{
				"status_code": 20000,
				"cost":        0.01,
				"result":      result,
			}},
		}
		writer.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(writer).Encode(response); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClientWithCredentials("user", "pass")
	client.BaseURL = server.URL + "/v3"
	searchReport := client.Search(context.Background(), Options{
		Target:   "example.com",
		Location: "United Kingdom",
		Language: "en",
		Limit:    10,
	})

	if requests.Load() != SearchDatasetCount || searchReport.SuccessfulCalls != SearchDatasetCount || !searchReport.Available {
		t.Fatalf("unexpected search call summary: requests=%d success=%d available=%t", requests.Load(), searchReport.SuccessfulCalls, searchReport.Available)
	}
	if searchReport.CostUSD < 0.039 || searchReport.CostUSD > 0.041 {
		t.Fatalf("unexpected search cost: %f", searchReport.CostUSD)
	}
	if searchReport.DatasetGroup != "search" || searchReport.RequestedDatasets != SearchDatasetCount {
		t.Fatalf("unexpected search metadata: %#v", searchReport)
	}
	if searchReport.OrganicVisibility.Keywords != 12 || len(searchReport.RankedKeywords) != 1 || searchReport.RankedKeywords[0].Position != 8 {
		t.Fatalf("unexpected organic data: %#v %#v", searchReport.OrganicVisibility, searchReport.RankedKeywords)
	}
	if len(searchReport.KeywordIdeas) != 1 || searchReport.KeywordIdeas[0].Keyword != "website seo checker" {
		t.Fatalf("unexpected keyword ideas: %#v", searchReport.KeywordIdeas)
	}
	if len(searchReport.Competitors) != 1 || searchReport.Competitors[0].Domain != "competitor.example" {
		t.Fatalf("unexpected competitors: %#v", searchReport.Competitors)
	}
	if searchReport.BacklinkSummary.Backlinks != 0 || len(searchReport.ReferringDomains) != 0 || len(searchReport.TopBacklinks) != 0 {
		t.Fatalf("search report contains backlink data: %#v", searchReport)
	}

	backlinkReport := client.Backlinks(context.Background(), Options{Target: "example.com", Limit: 10})
	if requests.Load() != SearchDatasetCount+BacklinkDatasetCount || backlinkReport.SuccessfulCalls != BacklinkDatasetCount || !backlinkReport.Available {
		t.Fatalf("unexpected backlink call summary: requests=%d success=%d available=%t", requests.Load(), backlinkReport.SuccessfulCalls, backlinkReport.Available)
	}
	if backlinkReport.CostUSD < 0.029 || backlinkReport.CostUSD > 0.031 {
		t.Fatalf("unexpected backlink cost: %f", backlinkReport.CostUSD)
	}
	if backlinkReport.DatasetGroup != "backlinks" || backlinkReport.RequestedDatasets != BacklinkDatasetCount {
		t.Fatalf("unexpected backlink metadata: %#v", backlinkReport)
	}
	if backlinkReport.BacklinkSummary.DataForSEORank != 51 || len(backlinkReport.ReferringDomains) != 1 || len(backlinkReport.TopBacklinks) != 1 {
		t.Fatalf("unexpected backlink data: %#v %#v %#v", backlinkReport.BacklinkSummary, backlinkReport.ReferringDomains, backlinkReport.TopBacklinks)
	}
	if backlinkReport.OrganicVisibility.Keywords != 0 || len(backlinkReport.RankedKeywords) != 0 || len(backlinkReport.KeywordIdeas) != 0 {
		t.Fatalf("backlink report contains search data: %#v", backlinkReport)
	}
}

func TestSearchPreservesDatasetErrors(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		status := 20000
		message := "Ok."
		result := []any{}
		if strings.Contains(request.URL.Path, "keywords_for_site") {
			status = 40501
			message = "Invalid Field"
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"status_code": 20000,
			"tasks": []any{map[string]any{
				"status_code":    status,
				"status_message": message,
				"cost":           0.01,
				"result":         result,
			}},
		})
	}))
	defer server.Close()

	client := NewClientWithCredentials("user", "pass")
	client.BaseURL = server.URL + "/v3"
	store, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "partial.db"), 10)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	client.Store = store
	report := client.Search(context.Background(), Options{Target: "example.com"})

	if report.SuccessfulCalls != 3 || len(report.Errors) != 1 {
		t.Fatalf("unexpected partial report: success=%d errors=%#v", report.SuccessfulCalls, report.Errors)
	}
	if report.Errors[0].Dataset != "keyword-ideas" || !strings.Contains(report.Errors[0].Message, "40501") {
		t.Fatalf("unexpected dataset error: %#v", report.Errors[0])
	}
	if report.Cache.Stored || report.SnapshotID == 0 {
		t.Fatalf("partial report cache evidence = %#v", report)
	}

	second := client.Search(context.Background(), Options{Target: "example.com"})
	if second.Cache.Hit || requests.Load() != SearchDatasetCount*2 {
		t.Fatalf("partial report was cached: requests=%d report=%#v", requests.Load(), second)
	}
}

func TestSearchCachesCompleteReportsAndStoresSnapshots(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"status_code": 20000,
			"tasks": []any{map[string]any{
				"status_code": 20000,
				"cost":        0.01,
				"result":      []any{},
			}},
		})
	}))
	defer server.Close()

	store, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "provider.db"), 10)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	client := NewClientWithCredentials("user", "pass")
	client.BaseURL = server.URL
	client.Store = store
	options := Options{
		Target:   "Example.COM",
		Location: "United Kingdom",
		Language: "EN",
		Limit:    10,
		CacheTTL: time.Hour,
	}

	first := client.Search(context.Background(), options)
	if requests.Load() != SearchDatasetCount {
		t.Fatalf("first request count = %d, want %d", requests.Load(), SearchDatasetCount)
	}
	if first.Cache.Hit || !first.Cache.Stored || first.LiveCalls != SearchDatasetCount || first.SnapshotID == 0 {
		t.Fatalf("unexpected first cache evidence: %#v", first)
	}
	if first.CostUSD < 0.039 || first.CostUSD > 0.041 {
		t.Fatalf("first current provider cost = %f", first.CostUSD)
	}

	second := client.Search(context.Background(), options)
	if requests.Load() != SearchDatasetCount {
		t.Fatalf("cached request made new provider calls: %d", requests.Load())
	}
	if !second.Cache.Hit || second.LiveCalls != 0 || second.CostUSD != 0 || second.SnapshotID == 0 {
		t.Fatalf("unexpected cached report: %#v", second)
	}
	if second.Cache.CachedProviderCostUSD < 0.039 || second.Cache.CachedProviderCostUSD > 0.041 {
		t.Fatalf("cached original provider cost = %f", second.Cache.CachedProviderCostUSD)
	}
	if second.SnapshotID == first.SnapshotID {
		t.Fatalf("cache hit reused snapshot id %d", second.SnapshotID)
	}

	changedLimit := options
	changedLimit.Limit = 11
	third := client.Search(context.Background(), changedLimit)
	if requests.Load() != SearchDatasetCount*2 || third.Cache.Hit {
		t.Fatalf("changed inputs did not miss cache: requests=%d report=%#v", requests.Load(), third)
	}

	refreshed := client.Search(context.Background(), Options{
		Target:   options.Target,
		Location: options.Location,
		Language: options.Language,
		Limit:    options.Limit,
		CacheTTL: options.CacheTTL,
		Refresh:  true,
	})
	if requests.Load() != SearchDatasetCount*3 || refreshed.Cache.Hit {
		t.Fatalf("refresh did not bypass cache: requests=%d report=%#v", requests.Load(), refreshed)
	}
}

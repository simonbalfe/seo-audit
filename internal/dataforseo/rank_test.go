package dataforseo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/simonbalfe/seo-audit/internal/ranktracking"
)

func TestCheckRanksParsesPositionsAndNoResults(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path != "/v3/serp/google/organic/live/advanced" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		var payload []map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if len(payload) != 1 {
			t.Errorf("payload tasks = %d, want 1", len(payload))
		}
		for _, task := range payload {
			if task["location_name"] != "United Kingdom" || task["language_code"] != "en" || task["depth"] != float64(50) {
				t.Errorf("unexpected task settings: %#v", task)
			}
			targets, ok := task["stop_crawl_on_match"].([]any)
			if !ok || len(targets) != 1 {
				t.Errorf("missing stop target: %#v", task)
			}
		}
		tag, _ := payload[0]["tag"].(string)
		responseTask := map[string]any{
			"status_code": 20000,
			"cost":        0.002,
			"data":        map[string]any{"tag": tag},
			"result": []any{map[string]any{"items": []any{
				map[string]any{"type": "people_also_ask"},
				map[string]any{
					"type": "organic", "rank_absolute": 5,
					"domain": "www.example.com", "url": "https://example.com/alpha",
				},
			}}},
		}
		if tag == "2:mobile" {
			responseTask = map[string]any{
				"status_code":    40501,
				"status_message": "No Search Results.",
				"cost":           0.002,
				"data":           map[string]any{"tag": tag},
				"result":         nil,
			}
		}
		if tag == "3:desktop" {
			responseTask = map[string]any{
				"status_code":    40501,
				"status_message": "Invalid Field: 'keyword'.",
				"cost":           0.002,
				"data":           map[string]any{"tag": tag},
				"result":         nil,
			}
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"status_code": 20000,
			"tasks":       []any{responseTask},
		})
	}))
	defer server.Close()

	client := NewClientWithCredentials("user", "pass")
	client.BaseURL = server.URL + "/v3"
	report, err := client.CheckRanks(context.Background(), ranktracking.CheckOptions{
		Target:   "example.com",
		Location: "United Kingdom",
		Language: "en",
		Depth:    50,
		Tasks: []ranktracking.CheckTask{
			{KeywordID: 1, Keyword: "alpha", Device: "desktop"},
			{KeywordID: 2, Keyword: "beta", Device: "mobile"},
			{KeywordID: 3, Keyword: "gamma", Device: "desktop"},
		},
	})
	if err != nil {
		t.Fatalf("check ranks: %v", err)
	}
	if requests.Load() != 3 || report.LiveCalls != 3 || report.SuccessfulTasks != 2 || len(report.Errors) != 1 {
		t.Fatalf("unexpected provider report: requests=%d report=%#v", requests.Load(), report)
	}
	if report.CostUSD < 0.0059 || report.CostUSD > 0.0061 {
		t.Fatalf("cost = %f", report.CostUSD)
	}
	if len(report.Results) != 2 || report.Results[0].Position == nil || *report.Results[0].Position != 5 {
		t.Fatalf("unexpected results: %#v", report.Results)
	}
	if report.Results[0].RankingURL != "https://example.com/alpha" || len(report.Results[0].SERPFeatures) != 2 {
		t.Fatalf("unexpected alpha evidence: %#v", report.Results[0])
	}
	if report.Results[1].Position != nil {
		t.Fatalf("no-results task has position: %#v", report.Results[1])
	}
}

package gsc

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientUsesAccessTokenEnvironment(t *testing.T) {
	t.Setenv("GSC_ACCESS_TOKEN", "access-token")

	client, err := NewClient()
	if err != nil {
		t.Fatalf("create client: %v", err)
	}
	if client.AccessToken != "access-token" {
		t.Fatalf("unexpected access token: %q", client.AccessToken)
	}
}

func TestNewClientRequiresCredentials(t *testing.T) {
	t.Setenv("GSC_ACCESS_TOKEN", "")
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
	t.Setenv("HOME", t.TempDir())

	_, err := NewClient()
	if err == nil || !strings.Contains(err.Error(), "Search Console is not authenticated") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCollectsQueryPageMetricsAndOpportunities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("unexpected authorization: %q", request.Header.Get("Authorization"))
		}
		if !strings.HasSuffix(request.URL.Path, "/sites/sc-domain:example.com/searchAnalytics/query") {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		var payload struct {
			StartDate  string   `json:"startDate"`
			EndDate    string   `json:"endDate"`
			Dimensions []string `json:"dimensions"`
			RowLimit   int      `json:"rowLimit"`
			DataState  string   `json:"dataState"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload.StartDate != "2026-06-30" || payload.EndDate != "2026-07-27" || payload.RowLimit != 25 || payload.DataState != "final" {
			t.Errorf("unexpected payload: %#v", payload)
		}
		if len(payload.Dimensions) != 2 || payload.Dimensions[0] != "query" || payload.Dimensions[1] != "page" {
			t.Errorf("unexpected dimensions: %#v", payload.Dimensions)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"responseAggregationType": "byPage",
			"rows": []any{
				map[string]any{"keys": []string{"commercial query", "https://example.com/a"}, "clicks": 5, "impressions": 100, "ctr": 0.05, "position": 8},
				map[string]any{"keys": []string{"commercial query", "https://example.com/b"}, "clicks": 2, "impressions": 50, "ctr": 0.04, "position": 25},
				map[string]any{"keys": []string{"another query", "https://example.com/c"}, "clicks": 6, "impressions": 50, "ctr": 0.12, "position": 15},
			},
		})
	}))
	defer server.Close()

	client := NewClientWithToken("access-token")
	client.BaseURL = server.URL
	client.Now = func() time.Time {
		return time.Date(2026, time.July, 28, 12, 0, 0, 0, time.UTC)
	}
	report, err := client.QueryPerformance(context.Background(), Options{
		SiteURL: "sc-domain:example.com",
		Days:    28,
		Limit:   25,
	})
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if !report.Available || report.Summary.Rows != 3 {
		t.Fatalf("unexpected report summary: %#v", report.Summary)
	}
	if report.Summary.ReturnedClicks != 13 || report.Summary.ReturnedImpressions != 200 {
		t.Fatalf("unexpected returned totals: %#v", report.Summary)
	}
	if math.Abs(report.Summary.ReturnedCTR-0.065) > 0.0001 || math.Abs(report.Summary.WeightedPosition-14) > 0.0001 {
		t.Fatalf("unexpected derived metrics: %#v", report.Summary)
	}
	if len(report.StrikingDistance) != 2 || report.StrikingDistance[0].Query != "commercial query" {
		t.Fatalf("unexpected striking-distance rows: %#v", report.StrikingDistance)
	}
	if len(report.QueryOverlaps) != 1 || len(report.QueryOverlaps[0].Pages) != 2 {
		t.Fatalf("unexpected query overlaps: %#v", report.QueryOverlaps)
	}
}

func TestAuditExchangesRefreshToken(t *testing.T) {
	var tokenCalls int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/token" {
			tokenCalls++
			if err := request.ParseForm(); err != nil {
				t.Fatalf("parse token form: %v", err)
			}
			if request.Form.Get("refresh_token") != "refresh" {
				t.Errorf("unexpected refresh token")
			}
			_ = json.NewEncoder(writer).Encode(map[string]any{"access_token": "fresh-token"})
			return
		}
		if request.Header.Get("Authorization") != "Bearer fresh-token" {
			t.Errorf("unexpected authorization: %q", request.Header.Get("Authorization"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"rows": []any{}})
	}))
	defer server.Close()

	client := newClient()
	client.BaseURL = server.URL
	client.TokenURL = server.URL + "/token"
	client.Credentials = credentials{
		ClientID:     "client",
		ClientSecret: "secret",
		RefreshToken: "refresh",
	}
	if _, err := client.QueryPerformance(context.Background(), Options{SiteURL: "sc-domain:example.com"}); err != nil {
		t.Fatalf("audit: %v", err)
	}
	if tokenCalls != 1 || client.AccessToken != "fresh-token" {
		t.Fatalf("unexpected token state: calls=%d token=%q", tokenCalls, client.AccessToken)
	}
}

func TestAuditRejectsUnboundedOptions(t *testing.T) {
	client := NewClientWithToken("access-token")

	if _, err := client.QueryPerformance(context.Background(), Options{SiteURL: "sc-domain:example.com", Days: maxDays + 1}); err == nil {
		t.Fatal("expected lookback validation error")
	}
	if _, err := client.QueryPerformance(context.Background(), Options{SiteURL: "sc-domain:example.com", Limit: maxRows + 1}); err == nil {
		t.Fatal("expected row-limit validation error")
	}
}

func TestAnalyzeTreatsTrailingSlashVariantsAsOnePage(t *testing.T) {
	report := Report{QueryPages: []QueryPageMetric{
		{Query: "same query", Page: "https://example.com/page", Impressions: 10},
		{Query: "same query", Page: "https://example.com/page/", Impressions: 20},
	}}

	analyze(&report)

	if len(report.QueryOverlaps) != 0 {
		t.Fatalf("unexpected overlap: %#v", report.QueryOverlaps)
	}
}

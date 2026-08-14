package audit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simonbalfe/seo-audit/internal/report"
)

func TestOpenRouterClassifiesOnlySuppliedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(body)
		if strings.Contains(string(encoded), "private page body") {
			t.Fatal("request contains page body")
		}
		if body["model"] != openRouterModel {
			t.Errorf("model = %v", body["model"])
		}
		provider := body["provider"].(map[string]any)
		if provider["require_parameters"] != true {
			t.Errorf("provider = %#v", provider)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"choices":[{"message":{"content":"{\"pages\":[{\"id\":3,\"type\":\"service\",\"reason\":\"service title and booking action\",\"keyword_seeds\":[\"invisalign treatment\",\"clear braces\"]}]}"}}],"usage":{"prompt_tokens":120,"completion_tokens":20,"cost":0.00002}}`))
	}))
	defer server.Close()

	classifier := pageClassifier{HTTP: server.Client(), Endpoint: server.URL, APIKey: "test-key"}
	classified, usage, err := classifier.classify(t.Context(), []classificationInput{{
		ID: 3, URL: "https://example.com/invisalign", Title: "Invisalign", HasBooking: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(classified) != 1 || classified[0].Type != "service" || len(classified[0].KeywordSeeds) != 2 || usage.PromptTokens != 120 || usage.Cost != 0.00002 {
		t.Fatalf("classification = %#v, usage = %#v", classified, usage)
	}
}

func TestPageClassificationCacheReusesUnchangedMetadata(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "classifications.sqlite")
	page := report.PageReport{
		URL:       "https://example.com/invisalign",
		FinalURL:  "https://example.com/invisalign",
		Title:     "Invisalign London",
		H1:        []string{"Invisalign London"},
		Indexable: true,
	}
	input := classificationPage(0, page)
	cache, err := openClassificationCache(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := cache.save(t.Context(), map[int]classificationInput{0: input}, []classification{{ID: 0, Type: "service-location", Reason: "service and named location", KeywordSeeds: []string{"invisalign london"}}}); err != nil {
		t.Fatal(err)
	}
	changed := input
	changed.Title = "Changed title"
	if _, found, err := cache.load(t.Context(), changed); err != nil || found {
		t.Fatalf("changed metadata cache lookup = found %t, error %v", found, err)
	}
	if err := cache.close(); err != nil {
		t.Fatal(err)
	}

	site := SiteReport{Pages: []report.PageReport{page}}
	classifyPages(t.Context(), &site, Options{ClassificationCachePath: databasePath})
	if site.Pages[0].PageType != "service-location" || site.Pages[0].PageTypeSource != "openrouter-cache" {
		t.Fatalf("cached page = %#v", site.Pages[0])
	}
	if site.PageClassification.CacheHits != 1 || site.PageClassification.Requests != 0 {
		t.Fatalf("classification report = %#v", site.PageClassification)
	}
	if !site.Pages[0].PriorityPage || len(site.Pages[0].KeywordSeeds) != 1 || site.Pages[0].KeywordSeeds[0] != "invisalign london" {
		t.Fatalf("priority page = %#v", site.Pages[0])
	}
}

func TestClassificationFingerprintIgnoresSchemaDiscoveryOrder(t *testing.T) {
	forward := report.PageReport{URL: "https://example.com/service", SchemaTypes: []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K"}}
	reverse := report.PageReport{URL: "https://example.com/service", SchemaTypes: []string{"K", "J", "I", "H", "G", "F", "E", "D", "C", "B", "A"}}
	first, err := classificationFingerprint(classificationPage(0, forward))
	if err != nil {
		t.Fatal(err)
	}
	second, err := classificationFingerprint(classificationPage(0, reverse))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("schema-order fingerprints differ: %s != %s", first, second)
	}
}

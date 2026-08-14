package dataforseo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/simonbalfe/seo-audit/internal/report"
)

func TestMapsAnchorsResultsToTargetBusiness(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/serp/google/maps/live/advanced" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		var payload []map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if got := payload[0]["location_coordinate"]; got != "51.5001000,-0.1202000,15z" {
			t.Fatalf("location_coordinate = %q", got)
		}
		if got := payload[0]["search_places"]; got != false {
			t.Fatalf("search_places = %v", got)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"cost":0.002,"result":[{"items":[{"type":"maps_search","rank_group":1,"title":"Nearby Dental","place_id":"competitor","rating":{"value":4.9,"votes_count":300}},{"type":"maps_search","rank_group":2,"title":"Target Dental","place_id":"target","category":"Dentist","rating":{"value":4.8,"votes_count":200}}]}]}]}`))
	}))
	defer server.Close()

	client := NewClientWithCredentials("user", "password")
	client.BaseURL = server.URL
	snapshot, cost, live, err := client.maps(context.Background(), "dentist near me", Options{
		Language:        "en",
		TargetPlaceID:   "target",
		TargetLatitude:  51.5001,
		TargetLongitude: -0.1202,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !live || cost != 0.002 || snapshot.TargetPosition != 2 || len(snapshot.Results) != 2 {
		t.Fatalf("live=%v cost=%f snapshot=%#v", live, cost, snapshot)
	}
	if !snapshot.Results[1].IsTarget || snapshot.Results[0].ReviewCount != 300 {
		t.Fatalf("results = %#v", snapshot.Results)
	}
}

func TestSiteKeywordsRequestsWholeSite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/keywords_data/google_ads/keywords_for_site/live" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		var payload []map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload[0]["target"] != "example.com" || payload[0]["target_type"] != "site" || payload[0]["location_coordinate"] != "51.5001000,-0.1202000" {
			t.Fatalf("payload = %#v", payload[0])
		}
		if _, exists := payload[0]["location_name"]; exists {
			t.Fatalf("payload should use coordinates: %#v", payload[0])
		}
		_, _ = writer.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"cost":0.075,"result":[{"keyword":"local service","search_volume":100,"cpc":4.2}]}]}`))
	}))
	defer server.Close()

	client := NewClientWithCredentials("user", "password")
	client.BaseURL = server.URL
	items, cost, live, err := client.siteKeywords(context.Background(), Options{Target: "example.com", Location: "London", Language: "en", TargetLatitude: 51.5001, TargetLongitude: -0.1202})
	if err != nil || !live || cost != 0.075 || len(items) != 1 || items[0].SearchVolume != 100 {
		t.Fatalf("items=%#v cost=%f live=%v err=%v", items, cost, live, err)
	}
}

func TestRankedKeywordsUsePlaceCountry(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var payload []map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if got := payload[0]["location_name"]; got != "United Kingdom" {
			t.Fatalf("location_name = %q, want United Kingdom", got)
		}
		_, _ = writer.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"result":[{"items":[]}]}]}`))
	}))
	defer server.Close()

	client := NewClientWithCredentials("user", "password")
	client.BaseURL = server.URL
	if _, _, _, err := client.ranked(context.Background(), Options{Target: "example.com", Location: "London", TargetCountry: "United Kingdom", Language: "en"}); err != nil {
		t.Fatal(err)
	}
}

func TestExistingRankingsAreKeptAndPrioritizedForLocalChecks(t *testing.T) {
	items := make([]rankedItem, 3)
	items[0].KeywordData.Keyword = "Dental Implants"
	items[0].KeywordData.KeywordInfo.SearchVolume = 500
	items[0].RankedSERPElement.SERPItem.RankAbsolute = 55
	items[0].RankedSERPElement.SERPItem.URL = "https://example.com/"
	items[1].KeywordData.Keyword = "How Dental Implants Work"
	items[1].KeywordData.KeywordInfo.SearchVolume = 900
	items[1].RankedSERPElement.SERPItem.RankAbsolute = 8
	items[1].RankedSERPElement.SERPItem.URL = "https://example.com/guide/"
	items[2].KeywordData.Keyword = "Dentist Near Me"
	items[2].KeywordData.KeywordInfo.SearchVolume = 1000
	items[2].RankedSERPElement.SERPItem.RankAbsolute = 3
	items[2].RankedSERPElement.SERPItem.URL = "https://example.com/"

	rankings := existingRankings(items)
	if len(rankings) != 3 || rankings[0].Keyword != "dentist near me" || rankings[2].Position != 55 {
		t.Fatalf("existingRankings() = %#v", rankings)
	}
	candidates := rankedCandidates(items)
	if len(candidates) != 2 || candidates[0].keyword != "dental implants" || candidates[0].position != 55 {
		t.Fatalf("rankedCandidates() = %#v", candidates)
	}
	shortlist := shortlistCandidates(append(candidates, candidate{keyword: "new implant idea", source: "openrouter", searchVolume: 5000}), 2)
	if len(shortlist) != 2 || shortlist[0].source != "current-ranking" || shortlist[1].source != "current-ranking" {
		t.Fatalf("shortlistCandidates() = %#v", shortlist)
	}
	newIdeas := removeExistingCandidates([]candidate{{keyword: "dental implants"}, {keyword: "new implant idea"}}, rankings)
	if len(newIdeas) != 1 || newIdeas[0].keyword != "new implant idea" {
		t.Fatalf("removeExistingCandidates() = %#v", newIdeas)
	}
}

func TestBacklinksReturnsOneDomainSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/backlinks/summary/live" {
			t.Fatalf("path = %q", request.URL.Path)
		}
		var payload []map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if got := payload[0]["target"]; got != "example.com" {
			t.Fatalf("target = %q", got)
		}
		if got := payload[0]["include_subdomains"]; got != true {
			t.Fatalf("include_subdomains = %v", got)
		}
		_, _ = writer.Write([]byte(`{"status_code":20000,"tasks":[{"status_code":20000,"cost":0.024,"result":[{"backlinks":358,"referring_domains":186,"referring_domains_nofollow":123,"referring_pages":338,"referring_ips":93,"rank":170,"backlinks_spam_score":24,"broken_backlinks":8,"broken_pages":5,"referring_links_countries":{"GB":101,"US":23},"info":{"target_spam_score":3}}]}]}`))
	}))
	defer server.Close()

	client := NewClientWithCredentials("user", "password")
	client.BaseURL = server.URL
	got := client.Backlinks(context.Background(), "https://www.Example.com/path")
	if got.Error != "" || got.LiveCalls != 1 || got.CostUSD != 0.024 {
		t.Fatalf("backlink request = %#v", got)
	}
	if got.Backlinks != 358 || got.ReferringDomains != 186 || got.BrokenBacklinks != 8 || got.Countries["GB"] != 101 {
		t.Fatalf("backlink summary = %#v", got)
	}
}

func TestSiteCandidatesKeepCommercialNonBrandKeywords(t *testing.T) {
	items := []volumeItem{
		{Keyword: "dentist london", SearchVolume: 1000, CPC: 8},
		{Keyword: "teeth whitening", SearchVolume: 500, CPC: 4},
		{Keyword: "Whites Dental", SearchVolume: 100, CPC: 2},
		{Keyword: "how to brush teeth", SearchVolume: 2000, CPC: 1},
		{Keyword: "dental history", SearchVolume: 100, CPC: 0},
		{Keyword: "dental surgerys", SearchVolume: 8100, CPC: 4.6},
	}
	pages := []report.PageReport{{URL: "https://whitesdental.co.uk/services", Indexable: true, Title: "Dentist and teeth whitening"}}
	got := siteCandidates(items, pages, Options{Target: "whitesdental.co.uk", TargetName: "Whites Dental Waterloo", TargetCategory: "Dentist", Location: "London,England,United Kingdom"})
	if len(got) != 2 || got[0].keyword != "dentist london" || got[1].keyword != "teeth whitening" {
		t.Fatalf("siteCandidates() = %#v", got)
	}
	if got[0].source != "site-discovery" || candidateImportance(got[0]) != 5 {
		t.Fatalf("first candidate = %#v", got[0])
	}
}

func TestShortlistPrefersProviderRelevanceForDiscoveredKeywords(t *testing.T) {
	got := shortlistCandidates([]candidate{
		{keyword: "high volume phrase", source: "site-discovery", relevance: 20, searchVolume: 10000},
		{keyword: "closest business phrase", source: "site-discovery", relevance: 1, searchVolume: 100},
	}, 1)
	if len(got) != 1 || got[0].keyword != "closest business phrase" {
		t.Fatalf("shortlistCandidates() = %#v", got)
	}
}

func TestOpenRouterKeywordMapsToPriorityCommercialPage(t *testing.T) {
	pages := []report.PageReport{
		{
			FinalURL:     "https://example.com/dental-implants/",
			Indexable:    true,
			PriorityPage: true,
			PageType:     "service",
			Title:        "Dental Implants",
			KeywordSeeds: []string{"dental implants london"},
		},
		{
			FinalURL:     "https://example.com/blog/dental-implant-guide/",
			Indexable:    true,
			PriorityPage: false,
			PageType:     "blog",
			Title:        "Dental Implant Guide",
		},
	}
	got := assignCandidatePages(generatedCandidates([]string{"dental implants london"}, []volumeItem{{Keyword: "dental implants london", SearchVolume: 500, CPC: 8}}), pages)
	if len(got) != 1 || got[0].url != "" || got[0].targetURL != pages[0].FinalURL || got[0].source != "openrouter" {
		t.Fatalf("mapped candidate = %#v", got)
	}
	opportunity := opportunity(got[0], &pages[0], nil)
	if opportunity.URL != "" || opportunity.TargetURL != pages[0].FinalURL || len(opportunity.Actions) == 0 || !strings.Contains(opportunity.Actions[0], "matched priority page") {
		t.Fatalf("opportunity = %#v", opportunity)
	}
}

func TestGeoGridAndOpportunitySummariseAreaVisibility(t *testing.T) {
	points := geoGrid(51.5, -0.12, 2)
	if len(points) != 9 || points[4].Latitude != 51.5 || points[4].Longitude != -0.12 {
		t.Fatalf("geoGrid() = %#v", points)
	}
	for index := range points {
		points[index].Position = index + 1
		points[index].Status = "ranked"
	}
	snapshot := report.MapsVisibility{TargetPosition: 2, GridPoints: points}
	summarizeGrid(&snapshot)
	if snapshot.TopThreeCoverage != 33.3 || snapshot.FoundCoverage != 100 || snapshot.MedianPosition != 5 {
		t.Fatalf("grid summary = %#v", snapshot)
	}
	got := opportunity(candidate{keyword: "dentist london", source: "site-discovery", searchVolume: 1000}, nil, &snapshot)
	if got.Priority != "high" || got.Status != "weak-organic-and-maps" || got.MapsTopThreeCoverage != 33.3 {
		t.Fatalf("opportunity() = %#v", got)
	}
}

func TestGridCandidatesPreferExistingNonBrandAndDistinctPages(t *testing.T) {
	current := []candidate{
		{keyword: "addison place dentist", searchVolume: 100, url: "https://addisonplace.co.uk/", source: "current-ranking"},
		{keyword: "dentist w11 4rj", searchVolume: 90, url: "https://addisonplace.co.uk/", source: "current-ranking"},
		{keyword: "dentist holland park", searchVolume: 80, url: "https://addisonplace.co.uk/", source: "current-ranking"},
	}
	opportunities := []candidate{
		{keyword: "dentist near me", searchVolume: 1000, targetURL: "https://addisonplace.co.uk/"},
		{keyword: "dental implants london", searchVolume: 500, targetURL: "https://addisonplace.co.uk/services/implants/"},
		{keyword: "children's dentist london", searchVolume: 300, targetURL: "https://addisonplace.co.uk/services/children/"},
		{keyword: "braces london", searchVolume: 200, targetURL: "https://addisonplace.co.uk/services/braces/"},
		{keyword: "facial aesthetics london", searchVolume: 100, targetURL: "https://addisonplace.co.uk/services/facial-aesthetics/"},
		{keyword: "emergency dentist london", searchVolume: 0, targetURL: "https://addisonplace.co.uk/services/emergency/"},
	}
	got := selectGridCandidates(current, opportunities, Options{Target: "addisonplace.co.uk", TargetName: "Addison Place Dental Practice"}, 5)
	want := []string{"dentist holland park", "dental implants london", "children's dentist london", "braces london", "facial aesthetics london"}
	if len(got) != len(want) {
		t.Fatalf("selectGridCandidates() = %#v", got)
	}
	for index := range want {
		if got[index].keyword != want[index] {
			t.Fatalf("keyword %d = %q, want %q", index, got[index].keyword, want[index])
		}
	}
}

func TestGridSummaryExcludesFailedPoints(t *testing.T) {
	points := []report.GeoRankPoint{
		{Position: 1, Status: "ranked"},
		{Position: 5, Status: "ranked"},
		{Status: "not_found"},
	}
	for range 6 {
		points = append(points, report.GeoRankPoint{Status: "error", Error: "provider error"})
	}
	snapshot := report.MapsVisibility{GridPoints: points}
	summarizeGrid(&snapshot)
	if snapshot.GridCheckedPoints != 3 || snapshot.GridFailedPoints != 6 || snapshot.TopThreeCoverage != 33.3 || snapshot.FoundCoverage != 66.7 || snapshot.MedianPosition != 5 {
		t.Fatalf("grid summary = %#v", snapshot)
	}
}

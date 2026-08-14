package cli

import (
	"bytes"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/simonbalfe/seo-audit/internal/report"
)

func TestPrintReportShowsPublicAuditOverview(t *testing.T) {
	var output bytes.Buffer
	printReport(&output, report.SiteReport{
		StartURL: "https://example.com/",
		PageClassification: report.PageClassificationReport{
			AIClassified:  1,
			PriorityPages: 1,
			Counts:        map[string]int{"home": 1, "service": 2},
		},
		Pages: []report.PageReport{{
			FinalURL:       "https://example.com/service/",
			PageType:       "service",
			PriorityPage:   true,
			TargetKeywords: []string{"local service"},
			PhoneLinks:     []string{"tel:123"},
			Findings:       []report.Finding{{Priority: "high"}},
		}},
		Summary: report.Summary{
			Pages:         3,
			Indexable:     2,
			NonIndexable:  1,
			Failures:      1,
			Warnings:      2,
			InternalLinks: 8,
			ExternalLinks: 2,
			SitemapURLs:   3,
		},
	})

	for _, expected := range []string{"SEO audit: https://example.com/", "Crawled: 3 URLs", "Actions: 1 failures, 2 warnings", "Priority pages: 1 commercial candidates; 1 matched", "PRIORITY PAGE AUDIT", "local service", "1 high, 0 medium, 0 low", "call only"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestAuditCommandOnlyExposesPublicAuditFlags(t *testing.T) {
	var output bytes.Buffer
	err := execute(t.Context(), []string{"audit", "--help"}, &output, &output)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("audit help error = %v, want flag.ErrHelp", err)
	}
	for _, name := range []string{"gsc", "dataforseo", "save", "db", "cache-ttl", "refresh", "verbose", "market", "language", "market-checks", "grid-radius-km", "business", "city", "state", "place-id"} {
		if strings.Contains(output.String(), "-"+name) {
			t.Fatalf("audit command unexpectedly exposes --%s", name)
		}
	}
	for _, name := range []string{"debug", "output", "dashboard", "keyword", "limit", "timeout", "steps", "website"} {
		if !strings.Contains(output.String(), "-"+name) {
			t.Fatalf("audit command does not expose --%s", name)
		}
	}
	if strings.Contains(output.String(), "-ai-pages") {
		t.Fatal("audit command unexpectedly exposes --ai-pages")
	}
	for _, expected := range []string{"usage: seoaudit audit <place-id>", "Runs: resolve Place ID and website", "Crawl: 50 pages", "5 MiB pages", "Provider calls are paid"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("audit help does not explain %q:\n%s", expected, output.String())
		}
	}
}

func TestDashboardModeRejectsURL(t *testing.T) {
	err := execute(t.Context(), []string{"audit", "place-1", "--dashboard"}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "does not accept a Place ID") {
		t.Fatalf("dashboard error = %v", err)
	}
}

func TestAuditFlagsAcceptDocumentedArgumentOrder(t *testing.T) {
	for _, args := range [][]string{
		{"place-1", "--limit", "10"},
		{"--limit", "10", "place-1"},
	} {
		flags := flag.NewFlagSet("test", flag.ContinueOnError)
		limit := flags.Int("limit", 50, "")
		positionals, err := parseCommandFlags(flags, args)
		if err != nil {
			t.Fatalf("parse %v: %v", args, err)
		}
		if len(positionals) != 1 || positionals[0] != "place-1" || *limit != 10 {
			t.Fatalf("parse %v = positionals %v, limit %d", args, positionals, *limit)
		}
	}
}

func TestPrintReportShowsMarketOpportunities(t *testing.T) {
	var output bytes.Buffer
	printReport(&output, report.SiteReport{
		Market: report.MarketReport{
			Enabled:                  true,
			Location:                 "Croydon, England, United Kingdom",
			LiveCalls:                3,
			CostUSD:                  0.014,
			GridKeywords:             []string{"dentist near me"},
			ExistingRankingsLocation: "United Kingdom",
			ExistingRankings: []report.ExistingRanking{{
				Keyword:      "dentist near me",
				Position:     7,
				URL:          "https://example.com/",
				SearchVolume: 1000,
			}},
			CurrentVisibility: []report.Opportunity{{
				Keyword:         "dentist near me",
				CountryPosition: 7,
				Position:        4,
				MapsChecked:     true,
				MapsPosition:    2,
				SearchVolume:    1000,
				URL:             "https://example.com/",
			}},
			Opportunities: []report.Opportunity{{
				Keyword:      "dental implants croydon",
				Source:       "site-discovery",
				Priority:     "high",
				Status:       "weak-organic",
				Evidence:     "organic not found in top 100",
				SearchVolume: 90,
				CPC:          23.71,
				Actions:      []string{"Create or improve the matching page."},
			}},
			CurrentMaps: []report.MapsVisibility{{
				Keyword:           "dentist near me",
				CenterLatitude:    51.372,
				CenterLongitude:   -0.101,
				Zoom:              15,
				TargetPosition:    2,
				GridPoints:        []report.GeoRankPoint{{Position: 2, Status: "ranked"}},
				GridCheckedPoints: 1,
				Results:           []report.LocalSearchResult{{Position: 1, Name: "Nearby Dental", Rating: 4.9, ReviewCount: 300}},
			}},
		},
	})
	for _, expected := range []string{"Local visibility: Croydon", "1 grid keywords", "Maps grid keywords: dentist near me", "Existing organic rankings (United Kingdom): 1 found", "CURRENT LOCAL SEARCH AND MAPS SNAPSHOT", "dentist near me", "1/1", "NEW KEYWORD OPPORTUNITIES", "dental implants croydon", "23.71", "weak-organic", "Current ranking Maps snapshot centered on target GBP", "#1 Nearby Dental (4.9/300)"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestPrintReportShowsBacklinkSummary(t *testing.T) {
	var output bytes.Buffer
	printReport(&output, report.SiteReport{Backlinks: report.BacklinkReport{
		Enabled:            true,
		Backlinks:          358,
		ReferringDomains:   186,
		ReferringPages:     338,
		Rank:               170,
		BacklinksSpamScore: 24,
		BrokenBacklinks:    8,
		BrokenPages:        5,
		LiveCalls:          1,
		CostUSD:            0.024,
		Countries:          map[string]int{"US": 23, "GB": 101},
	}})
	for _, expected := range []string{"358 links from 186 domains", "spam score 24", "8 across 5 target pages", "GB 101, US 23", "$0.024000"} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, output.String())
		}
	}
}

func TestAuditOptionsRejectInvalidCrawlBounds(t *testing.T) {
	for _, opts := range []auditOptions{
		{limit: 0, timeout: 30 * time.Second},
		{limit: 50, timeout: 121 * time.Second},
	} {
		if err := opts.validate(); err == nil {
			t.Fatalf("validate %#v: expected error", opts)
		}
	}
}

func TestAuditOptionsRejectUnknownSteps(t *testing.T) {
	opts := auditOptions{limit: 50, timeout: 30 * time.Second, steps: "technical"}
	if err := opts.validate(); err == nil || !strings.Contains(err.Error(), "all, website") {
		t.Fatalf("validate steps error = %v", err)
	}
}

func TestAuditOptionsAcceptDashboardWorkflows(t *testing.T) {
	for _, steps := range []string{"all", "website", "performance", "visibility", "backlinks", "profile"} {
		if err := (auditOptions{limit: 50, timeout: 30 * time.Second, steps: steps}).validate(); err != nil {
			t.Errorf("validate steps %q: %v", steps, err)
		}
	}
}

func TestWebsiteForPlaceRequiresPublicWebsite(t *testing.T) {
	if _, err := websiteForPlace("place-1", report.GBPAuditReport{}, ""); err == nil || !strings.Contains(err.Error(), "no public website") {
		t.Fatalf("websiteForPlace missing website error = %v", err)
	}
	got, err := websiteForPlace("place-1", report.GBPAuditReport{Website: " https://example.com "}, "")
	if err != nil || got != "https://example.com" {
		t.Fatalf("websiteForPlace = %q, %v", got, err)
	}
	got, err = websiteForPlace("place-1", report.GBPAuditReport{}, "https://override.example/")
	if err != nil || got != "https://override.example/" {
		t.Fatalf("websiteForPlace override = %q, %v", got, err)
	}
	if _, err := websiteForPlace("place-1", report.GBPAuditReport{}, "example.com"); err == nil {
		t.Fatal("websiteForPlace accepted a relative override")
	}
}

func TestSaveJSONPersistsCompleteReport(t *testing.T) {
	target := filepath.Join(t.TempDir(), "output", "audit.json")
	if err := saveJSON(target, report.SiteReport{StartURL: "https://example.com/"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"start_url": "https://example.com/"`) {
		t.Fatalf("saved report = %s", data)
	}
}

func TestAutomaticOutputPathIsSortableAndIdentifiesHost(t *testing.T) {
	got := automaticOutputPath("https://www.Example.com/path", time.Date(2026, time.August, 5, 14, 30, 12, 345000000, time.FixedZone("BST", 3600)))
	want := filepath.Join("output", "20260805T133012.345Z-example.com.json")
	if got != want {
		t.Fatalf("automaticOutputPath() = %q, want %q", got, want)
	}
}

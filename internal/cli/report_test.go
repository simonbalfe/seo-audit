package cli

import (
	"bytes"
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simonbalfe/seo-audit/internal/api"
	"github.com/simonbalfe/seo-audit/internal/evidence"
	"github.com/simonbalfe/seo-audit/internal/storage"
	"github.com/spf13/cobra"
)

func TestPrintOpportunityReportIncludesSearchConsoleData(t *testing.T) {
	var output bytes.Buffer
	printOpportunityReport(&output, opportunityReport{
		Target: "example.com",
		SearchConsole: &gscReport{
			Available: true,
			SiteURL:   "sc-domain:example.com",
			StartDate: "2026-07-01",
			EndDate:   "2026-07-28",
			Summary: gscSummary{
				Rows:                1,
				ReturnedClicks:      10,
				ReturnedImpressions: 100,
				ReturnedCTR:         0.1,
				WeightedPosition:    8,
			},
			StrikingDistance: []gscQueryPageMetric{{
				Query:       "seo audit",
				Page:        "https://example.com/audit",
				Clicks:      10,
				Impressions: 100,
				CTR:         0.1,
				Position:    8,
			}},
		},
	})

	rendered := output.String()
	for _, expected := range []string{
		"Search Console: sc-domain:example.com",
		"GSC returned dataset: 10 clicks, 100 impressions",
		"Search Console opportunities",
		"seo audit",
	} {
		if !strings.Contains(rendered, expected) {
			t.Fatalf("output does not contain %q:\n%s", expected, rendered)
		}
	}
}

func TestAuditCommandDoesNotExposeAccountDataFlags(t *testing.T) {
	command := newAuditCommand(&clientOptions{})
	for _, name := range []string{"gsc", "gsc-site", "dataforseo", "location", "data-limit", "db", "cache-ttl", "refresh"} {
		if command.Flags().Lookup(name) != nil {
			t.Fatalf("audit command unexpectedly exposes --%s", name)
		}
	}
}

func TestPaidCommandsExposeProviderRequestFlags(t *testing.T) {
	options := &clientOptions{}
	for _, command := range []*cobra.Command{newOpportunitiesCommand(options), newBacklinksCommand(options)} {
		for _, name := range []string{"cache-ttl", "refresh"} {
			if command.Flags().Lookup(name) == nil {
				t.Fatalf("%s command does not expose --%s", command.Name(), name)
			}
		}
		if command.Flags().Lookup("db") != nil {
			t.Fatalf("%s command must not access the API database directly", command.Name())
		}
	}
}

func TestOpportunitiesRequiresExplicitSource(t *testing.T) {
	command := newOpportunitiesCommand(&clientOptions{})
	command.SetArgs([]string{"https://example.com"})
	err := command.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "--gsc or --dataforseo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBacklinksRequiresExplicitPaidSource(t *testing.T) {
	command := newBacklinksCommand(&clientOptions{})
	command.SetArgs([]string{"https://example.com"})
	err := command.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "--dataforseo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRankingsCheckRequiresExplicitPaidSource(t *testing.T) {
	command := newRankingsCheckCommand(&clientOptions{})
	command.SetArgs([]string{"https://example.com"})
	err := command.ExecuteContext(context.Background())
	if err == nil || !strings.Contains(err.Error(), "--dataforseo") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRankingsAddAndReportProxyThroughAPI(t *testing.T) {
	database := filepath.Join(t.TempDir(), "rankings.db")
	store, err := storage.OpenSQLite(database, 10)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer store.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	jobs := api.NewJobManager(ctx, 1, 10)
	server := httptest.NewServer(api.NewHandler(
		api.NewService(store, jobs, 1, 10),
		evidence.NewService(store),
	))
	defer server.Close()
	client := &clientOptions{apiURL: server.URL}

	add := newRankingsAddCommand(client)
	var addOutput bytes.Buffer
	add.SetOut(&addOutput)
	add.SetArgs([]string{
		"https://example.com",
		"seo audit",
		"technical seo",
		"--device", "both",
		"--depth", "50",
	})
	if err := add.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("add rankings: %v", err)
	}
	if !strings.Contains(addOutput.String(), "tracking 2 total") {
		t.Fatalf("unexpected add output:\n%s", addOutput.String())
	}

	addMore := newRankingsAddCommand(client)
	addMore.SetArgs([]string{"https://example.com", "content decay"})
	if err := addMore.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("add another ranking: %v", err)
	}

	report := newRankingsReportCommand(client)
	var reportOutput bytes.Buffer
	report.SetOut(&reportOutput)
	report.SetArgs([]string{"https://example.com"})
	if err := report.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("report rankings: %v", err)
	}
	for _, expected := range []string{"both, depth 50", "Tracked: 3 keywords", "no rank checks stored yet"} {
		if !strings.Contains(reportOutput.String(), expected) {
			t.Fatalf("report output does not contain %q:\n%s", expected, reportOutput.String())
		}
	}
}

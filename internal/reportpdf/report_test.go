package reportpdf

import (
	"os"
	"strings"
	"testing"

	"github.com/simonbalfe/seo-audit/internal/report"
)

func TestRender(t *testing.T) {
	points := make([]report.GeoRankPoint, 9)
	for index := range points {
		points[index] = report.GeoRankPoint{Position: index + 1, Status: "ok"}
	}
	data := report.SiteReport{
		StartURL: "https://example-dental.co.uk",
		GBP: &report.GBPAuditReport{
			Name: "Example Dental Clinic", Category: "Dentist", Address: "10 High Street, Birmingham",
		},
		Market: report.MarketReport{
			Location:          "Birmingham, England, United Kingdom",
			ExistingRankings:  []report.ExistingRanking{{Keyword: "dentist birmingham", Position: 8}},
			CurrentVisibility: []report.Opportunity{{Keyword: "dentist birmingham", Position: 8, MapsPosition: 5, SearchVolume: 2400}},
			CurrentMaps:       []report.MapsVisibility{{Keyword: "dentist birmingham", TargetPosition: 5, TopThreeCoverage: 33, MedianPosition: 6, GridPoints: points}},
			Opportunities:     []report.Opportunity{{Keyword: "emergency dentist birmingham", Position: 19, MapsPosition: 8, SearchVolume: 1300, Actions: []string{"Create a focused service page and strengthen the listing."}}},
		},
	}
	output, err := Render(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(output), "%PDF-") || len(output) < 1000 {
		t.Fatal("invalid PDF output")
	}
	if destination := os.Getenv("SEO_AUDIT_TEST_PDF"); destination != "" {
		if err := os.WriteFile(destination, output, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

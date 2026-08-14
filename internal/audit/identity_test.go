package audit

import (
	"testing"

	"github.com/simonbalfe/seo-audit/internal/report"
)

func TestVerifyBusinessIdentity(t *testing.T) {
	tests := []struct {
		name       string
		site       report.SiteReport
		profile    report.GBPAuditReport
		wantStatus string
		findings   int
	}{
		{
			name:       "website domain",
			site:       report.SiteReport{StartURL: "https://example.com/"},
			profile:    report.GBPAuditReport{Website: "https://www.example.com/location"},
			wantStatus: "matched",
		},
		{
			name:       "redirected subdomain",
			site:       report.SiteReport{StartURL: "https://example.com/", Pages: []report.PageReport{{FinalURL: "https://locations.example.com/shop"}}},
			profile:    report.GBPAuditReport{Website: "https://locations.example.com/shop"},
			wantStatus: "matched",
		},
		{
			name:       "different website",
			site:       report.SiteReport{StartURL: "https://example.com/"},
			profile:    report.GBPAuditReport{Website: "https://another.example/"},
			wantStatus: "mismatch",
			findings:   1,
		},
		{
			name:       "phone fallback",
			site:       report.SiteReport{StartURL: "https://example.com/", Pages: []report.PageReport{{PhoneLinks: []string{"020 7946 0123"}}}},
			profile:    report.GBPAuditReport{Phone: "+44 20 7946 0123"},
			wantStatus: "matched",
		},
		{
			name:       "different phone",
			site:       report.SiteReport{StartURL: "https://example.com/", Pages: []report.PageReport{{PhoneLinks: []string{"020 7946 0123"}}}},
			profile:    report.GBPAuditReport{Phone: "+44 20 7000 0000"},
			wantStatus: "mismatch",
			findings:   1,
		},
		{
			name:       "insufficient evidence",
			site:       report.SiteReport{StartURL: "https://example.com/"},
			profile:    report.GBPAuditReport{},
			wantStatus: "unverified",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := VerifyBusinessIdentity(test.site, test.profile)
			if got.IdentityStatus != test.wantStatus {
				t.Fatalf("identity status = %q, want %q", got.IdentityStatus, test.wantStatus)
			}
			if len(got.Findings) != test.findings {
				t.Fatalf("findings = %d, want %d", len(got.Findings), test.findings)
			}
		})
	}
}

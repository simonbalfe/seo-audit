package audit

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/simonbalfe/seo-audit/internal/report"
)

// VerifyBusinessIdentity compares a public GBP with the crawled website.
func VerifyBusinessIdentity(site report.SiteReport, profile report.GBPAuditReport) report.GBPAuditReport {
	profileHost := websiteHost(profile.Website)
	if profile.Website != "" {
		if profileHost == "" {
			profile.IdentityStatus = "unverified"
			profile.IdentityEvidence = "the GBP website URL could not be compared"
			return profile
		}
		for _, auditedHost := range auditedWebsiteHosts(site) {
			if relatedHost(profileHost, auditedHost) {
				profile.IdentityStatus = "matched"
				profile.IdentityEvidence = fmt.Sprintf("GBP website host %s matches audited host %s", profileHost, auditedHost)
				return profile
			}
		}
		return identityMismatch(profile, fmt.Sprintf("GBP website host %s differs from audited host %s", profileHost, websiteHost(site.StartURL)))
	}

	if profile.Phone != "" {
		publicPhones := 0
		for _, page := range site.Pages {
			for _, phone := range page.PhoneLinks {
				if len(phoneDigits(phone)) < 7 {
					continue
				}
				publicPhones++
				if samePhone(profile.Phone, phone) {
					profile.IdentityStatus = "matched"
					profile.IdentityEvidence = fmt.Sprintf("GBP phone %s matches a public website phone", profile.Phone)
					return profile
				}
			}
		}
		if publicPhones > 0 {
			return identityMismatch(profile, fmt.Sprintf("GBP phone %s differs from %d public website phone numbers", profile.Phone, publicPhones))
		}
	}

	profile.IdentityStatus = "unverified"
	profile.IdentityEvidence = "GBP has no comparable website domain and no matching public website phone"
	return profile
}

func identityMismatch(profile report.GBPAuditReport, evidence string) report.GBPAuditReport {
	profile.IdentityStatus = "mismatch"
	profile.IdentityEvidence = evidence
	profile.Findings = append(profile.Findings, report.GBPFinding{
		Priority: "high",
		Check:    "Business identity mismatch",
		Evidence: evidence,
		Fix:      "Confirm the selected Place ID and link the profile to the audited business website.",
	})
	return profile
}

func auditedWebsiteHosts(site report.SiteReport) []string {
	hosts := make([]string, 0, len(site.Pages)+1)
	add := func(rawURL string) {
		host := websiteHost(rawURL)
		if host == "" {
			return
		}
		for _, existing := range hosts {
			if existing == host {
				return
			}
		}
		hosts = append(hosts, host)
	}
	add(site.StartURL)
	for _, page := range site.Pages {
		add(page.FinalURL)
	}
	return hosts
}

func websiteHost(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return strings.TrimPrefix(strings.TrimSuffix(strings.ToLower(parsed.Hostname()), "."), "www.")
}

func relatedHost(left, right string) bool {
	return left == right || strings.HasSuffix(left, "."+right) || strings.HasSuffix(right, "."+left)
}

func samePhone(left, right string) bool {
	left = phoneDigits(left)
	right = phoneDigits(right)
	if len(left) < 7 || len(right) < 7 {
		return false
	}
	if left == right {
		return true
	}
	const suffixLength = 9
	return len(left) >= suffixLength && len(right) >= suffixLength && left[len(left)-suffixLength:] == right[len(right)-suffixLength:]
}

func phoneDigits(value string) string {
	var result strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			result.WriteRune(char)
		}
	}
	return result.String()
}

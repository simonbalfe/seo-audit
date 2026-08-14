package dataforseo

import (
	"context"
	"time"

	"github.com/simonbalfe/seo-audit/internal/report"
)

type backlinkSummary struct {
	Backlinks                int            `json:"backlinks"`
	ReferringDomains         int            `json:"referring_domains"`
	ReferringDomainsNofollow int            `json:"referring_domains_nofollow"`
	ReferringPages           int            `json:"referring_pages"`
	ReferringIPs             int            `json:"referring_ips"`
	Rank                     int            `json:"rank"`
	BacklinksSpamScore       int            `json:"backlinks_spam_score"`
	BrokenBacklinks          int            `json:"broken_backlinks"`
	BrokenPages              int            `json:"broken_pages"`
	Countries                map[string]int `json:"referring_links_countries"`
	Info                     struct {
		TargetSpamScore int `json:"target_spam_score"`
	} `json:"info"`
}

// Backlinks returns one live DataForSEO backlink summary for a target domain.
func (c *Client) Backlinks(ctx context.Context, target string) report.BacklinkReport {
	target = normalizeDomain(target)
	result := report.BacklinkReport{
		Enabled:     true,
		Source:      "DataForSEO",
		Target:      target,
		RetrievedAt: time.Now().UTC().Format(time.RFC3339),
	}
	var summaries []backlinkSummary
	cost, live, err := c.postTask(ctx, "/backlinks/summary/live", map[string]any{
		"target":              target,
		"include_subdomains":  true,
		"internal_list_limit": 10,
	}, &summaries)
	result.CostUSD = cost
	if live {
		result.LiveCalls = 1
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if len(summaries) == 0 {
		result.Error = "DataForSEO returned no backlink summary"
		return result
	}
	summary := summaries[0]
	result.Backlinks = summary.Backlinks
	result.ReferringDomains = summary.ReferringDomains
	result.ReferringDomainsNofollow = summary.ReferringDomainsNofollow
	result.ReferringPages = summary.ReferringPages
	result.ReferringIPs = summary.ReferringIPs
	result.Rank = summary.Rank
	result.BacklinksSpamScore = summary.BacklinksSpamScore
	result.TargetSpamScore = summary.Info.TargetSpamScore
	result.BrokenBacklinks = summary.BrokenBacklinks
	result.BrokenPages = summary.BrokenPages
	result.Countries = summary.Countries
	return result
}

package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/simonbalfe/seo-audit/internal/report"
)

type issueGroup struct {
	priority string
	category string
	check    string
	fix      string
	items    []report.Finding
}

func printReport(output io.Writer, report report.SiteReport) {
	fmt.Fprintf(output, "SEO audit: %s\n", report.StartURL)
	fmt.Fprintf(output, "Crawled: %d URLs (%d indexable, %d non-indexable) in %.1fs\n",
		report.Summary.Pages,
		report.Summary.Indexable,
		report.Summary.NonIndexable,
		float64(report.Duration)/1000,
	)
	fmt.Fprintf(output, "Discovered: %d internal links, %d external links, %d sitemap URLs\n",
		report.Summary.InternalLinks,
		report.Summary.ExternalLinks,
		report.Summary.SitemapURLs,
	)
	fmt.Fprintf(output, "Actions: %d failures, %d warnings\n", report.Summary.Failures, report.Summary.Warnings)
	printPriorityPageAudit(output, report)
	printPerformanceSummary(output, report)
	if report.LimitReached {
		fmt.Fprintln(output, "Warning: crawl limit reached; rerun with a higher --limit for complete coverage.")
	}
	fmt.Fprintln(output)
	printFindings(output, report.Findings)
	printMarketReport(output, report.Market)
	printBacklinkReport(output, report.Backlinks)
	if report.GBP != nil {
		fmt.Fprintln(output)
		printGBPAuditReport(output, *report.GBP)
	}
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Use --json to also print the complete saved report.")
}

func printBacklinkReport(output io.Writer, report report.BacklinkReport) {
	if !report.Enabled {
		return
	}
	if report.Error != "" {
		fmt.Fprintf(output, "\nBacklinks: unavailable (%s)\n", report.Error)
		return
	}
	fmt.Fprintf(
		output,
		"\nBacklinks: %d links from %d domains | %d referring pages | DataForSEO rank %d | spam score %d\n",
		report.Backlinks,
		report.ReferringDomains,
		report.ReferringPages,
		report.Rank,
		report.BacklinksSpamScore,
	)
	fmt.Fprintf(output, "Broken backlinks: %d across %d target pages | %d live call | $%.6f provider cost\n", report.BrokenBacklinks, report.BrokenPages, report.LiveCalls, report.CostUSD)
	countries := topBacklinkCountries(report.Countries, 5)
	if len(countries) > 0 {
		fmt.Fprintf(output, "Leading backlink countries: %s\n", strings.Join(countries, ", "))
	}
}

func topBacklinkCountries(countries map[string]int, limit int) []string {
	type countryCount struct {
		country string
		count   int
	}
	items := make([]countryCount, 0, len(countries))
	for country, count := range countries {
		if country != "" && count > 0 {
			items = append(items, countryCount{country: country, count: count})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].country < items[j].country
	})
	if len(items) > limit {
		items = items[:limit]
	}
	result := make([]string, len(items))
	for index, item := range items {
		result[index] = fmt.Sprintf("%s %d", item.country, item.count)
	}
	return result
}

func printPriorityPageAudit(output io.Writer, site report.SiteReport) {
	classification := site.PageClassification
	if classification.Model == "" && classification.PriorityPages == 0 && len(classification.Errors) == 0 {
		return
	}
	pages := make([]report.PageReport, 0, classification.PriorityPages)
	for _, page := range site.Pages {
		if page.PriorityPage && len(page.TargetKeywords) > 0 {
			pages = append(pages, page)
		}
	}
	fmt.Fprintf(output, "Priority pages: %d commercial candidates; %d matched to validated keywords (%d classified, %d cached, %d unknown)\n", classification.PriorityPages, len(pages), classification.AIClassified, classification.CacheHits, classification.Unknown)
	if classification.Model != "" {
		fmt.Fprintf(output, "OpenRouter visibility research: %s | %d requests | %d input tokens | %d output tokens | $%.6f\n", classification.Model, classification.Requests, classification.PromptTokens, classification.CompletionTokens, classification.CostUSD)
	}
	for _, message := range classification.Errors {
		fmt.Fprintf(output, "Visibility research warning: %s\n", message)
	}
	sort.Slice(pages, func(i, j int) bool {
		if len(pages[i].TargetKeywords) != len(pages[j].TargetKeywords) {
			return len(pages[i].TargetKeywords) > len(pages[j].TargetKeywords)
		}
		return priorityPageURL(pages[i]) < priorityPageURL(pages[j])
	})
	if len(pages) == 0 {
		return
	}
	fmt.Fprintln(output, "\nPRIORITY PAGE AUDIT")
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "TYPE\tPAGE\tVALIDATED KEYWORDS\tISSUES\tCONVERSION")
	for _, page := range pages {
		keywords := "not shortlisted"
		if len(page.TargetKeywords) > 0 {
			keywords = strings.Join(page.TargetKeywords, ", ")
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", page.PageType, priorityPageURL(page), keywords, priorityPageIssues(page.Findings), priorityPageConversion(page))
	}
	_ = writer.Flush()
}

func priorityPageURL(page report.PageReport) string {
	if page.FinalURL != "" {
		return page.FinalURL
	}
	return page.URL
}

func priorityPageIssues(findings []report.Finding) string {
	counts := map[string]int{}
	for _, finding := range findings {
		counts[finding.Priority]++
	}
	return fmt.Sprintf("%d high, %d medium, %d low", counts["high"], counts["medium"], counts["low"])
}

func priorityPageConversion(page report.PageReport) string {
	switch {
	case len(page.PhoneLinks) > 0 && len(page.BookingLinks) > 0:
		return "call + booking"
	case len(page.PhoneLinks) > 0:
		return "call only"
	case len(page.BookingLinks) > 0:
		return "booking only"
	default:
		return "none"
	}
}

func printMarketReport(output io.Writer, report report.MarketReport) {
	if !report.Enabled {
		return
	}
	fmt.Fprintf(output, "\nLocal visibility: %s | %d existing rankings | %d current checks | %d new ideas | %d opportunity checks | %d grid keywords | %d live calls | $%.6f provider cost\n", report.Location, len(report.ExistingRankings), len(report.CurrentVisibility), report.KeywordIdeas, len(report.Opportunities), len(report.GridKeywords), report.LiveCalls, report.CostUSD)
	for _, item := range report.Errors {
		fmt.Fprintf(output, "Market warning: %s: %s\n", item.Operation, item.Message)
	}
	if len(report.GridKeywords) > 0 {
		fmt.Fprintf(output, "Maps grid keywords: %s\n", strings.Join(report.GridKeywords, ", "))
	}
	printExistingRankings(output, report.ExistingRankingsLocation, report.ExistingRankings)
	printCurrentVisibility(output, report.CurrentVisibility, report.CurrentMaps)
	printMapsVisibility(output, "Current ranking Maps snapshot", report.CurrentMaps)
	if len(report.Opportunities) == 0 {
		return
	}
	fmt.Fprintln(output, "\nNEW KEYWORD OPPORTUNITIES")
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "PRIORITY\tKEYWORD\tSOURCE\tVOLUME\tCPC\tVISIBILITY\tGAP\tNEXT ACTION")
	for _, item := range report.Opportunities {
		action := ""
		if len(item.Actions) > 0 {
			action = item.Actions[0]
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%d\t%.2f\t%s\t%s\t%s\n", strings.ToUpper(item.Priority), item.Keyword, item.Source, item.SearchVolume, item.CPC, item.Evidence, item.Status, oneLine(action))
	}
	_ = writer.Flush()
	printMapsVisibility(output, "Opportunity keyword Maps checks", report.OpportunityMaps)
}

func printExistingRankings(output io.Writer, location string, rankings []report.ExistingRanking) {
	if len(rankings) == 0 {
		return
	}
	fmt.Fprintf(output, "\nExisting organic rankings (%s): %d found\n", location, len(rankings))
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "POSITION\tKEYWORD\tVOLUME\tRANKING PAGE")
	limit := min(len(rankings), 10)
	for _, item := range rankings[:limit] {
		fmt.Fprintf(writer, "%d\t%s\t%d\t%s\n", item.Position, item.Keyword, item.SearchVolume, item.URL)
	}
	_ = writer.Flush()
	if len(rankings) > limit {
		fmt.Fprintf(output, "%d more existing rankings are saved in JSON.\n", len(rankings)-limit)
	}
}

func printCurrentVisibility(output io.Writer, visibility []report.Opportunity, snapshots []report.MapsVisibility) {
	if len(visibility) == 0 {
		return
	}
	fmt.Fprintln(output, "\nCURRENT LOCAL SEARCH AND MAPS SNAPSHOT")
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "UK POSITION\tKEYWORD\tVOLUME\tLOCAL SEARCH\tMAPS\tGRID TOP 3\tRANKING PAGE")
	for _, item := range visibility {
		organic := "not found in top 100"
		if item.Position > 0 {
			organic = fmt.Sprintf("#%d", item.Position)
		}
		maps := "not checked"
		if item.MapsChecked {
			maps = "not found in top 20"
			if item.MapsPosition > 0 {
				maps = fmt.Sprintf("#%d", item.MapsPosition)
			}
		}
		grid := "not run"
		for _, snapshot := range snapshots {
			if snapshot.Keyword == item.Keyword && len(snapshot.GridPoints) > 0 {
				grid = fmt.Sprintf("%.1f%%", snapshot.TopThreeCoverage)
				break
			}
		}
		fmt.Fprintf(writer, "%d\t%s\t%d\t%s\t%s\t%s\t%s\n", item.CountryPosition, item.Keyword, item.SearchVolume, organic, maps, grid, item.URL)
	}
	_ = writer.Flush()
}

func printMapsVisibility(output io.Writer, heading string, snapshots []report.MapsVisibility) {
	if len(snapshots) == 0 {
		return
	}
	first := snapshots[0]
	fmt.Fprintf(output, "\n%s centered on target GBP: %.5f, %.5f (%dz)\n", heading, first.CenterLatitude, first.CenterLongitude, first.Zoom)
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "KEYWORD\tTARGET POSITION\tGRID POINTS\tGRID TOP 3\tGRID FOUND\tMEDIAN\tLEADING NEARBY COMPETITORS")
	for _, snapshot := range snapshots {
		position := "not found in top 20"
		if snapshot.TargetPosition > 0 {
			position = fmt.Sprintf("%d", snapshot.TargetPosition)
		}
		competitors := make([]string, 0, 3)
		for _, item := range snapshot.Results {
			if item.IsTarget {
				continue
			}
			competitors = append(competitors, fmt.Sprintf("#%d %s (%.1f/%d)", item.Position, item.Name, item.Rating, item.ReviewCount))
			if len(competitors) == 3 {
				break
			}
		}
		gridTopThree := "not run"
		gridFound := "not run"
		median := "not run"
		gridPoints := "not run"
		if len(snapshot.GridPoints) > 0 {
			gridPoints = fmt.Sprintf("%d/%d", snapshot.GridCheckedPoints, len(snapshot.GridPoints))
			gridTopThree = fmt.Sprintf("%.1f%%", snapshot.TopThreeCoverage)
			gridFound = fmt.Sprintf("%.1f%%", snapshot.FoundCoverage)
			median = "not found"
			if snapshot.MedianPosition > 0 {
				median = fmt.Sprintf("%d", snapshot.MedianPosition)
			}
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", snapshot.Keyword, position, gridPoints, gridTopThree, gridFound, median, strings.Join(competitors, "; "))
	}
	_ = writer.Flush()
}

func printGBPAuditReport(output io.Writer, report report.GBPAuditReport) {
	fmt.Fprintf(output, "GBP audit: %s\n", report.Name)
	fmt.Fprintf(output, "Category: %s\nAddress: %s\nPhone: %s\nWebsite: %s\n", report.Category, report.Address, report.Phone, report.Website)
	if report.IdentityStatus != "" {
		fmt.Fprintf(output, "Website identity: %s (%s)\n", report.IdentityStatus, report.IdentityEvidence)
	}
	fmt.Fprintf(output, "Status: %s | Rating: %.1f (%d reviews) | Photos: %d\n", report.BusinessStatus, report.Rating, report.ReviewCount, report.PhotoCount)
	if len(report.Hours) > 0 {
		fmt.Fprintf(output, "Hours: %s\n", strings.Join(report.Hours, "; "))
	}
	if report.GoogleMapsURL != "" {
		fmt.Fprintf(output, "Google Maps: %s\n", report.GoogleMapsURL)
	}
	if len(report.Findings) == 0 {
		fmt.Fprintln(output, "No deterministic public-listing issues found.")
		return
	}
	fmt.Fprintln(output, "\nPRIORITY\tISSUE\tEVIDENCE\tFIX")
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	for _, finding := range report.Findings {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", strings.ToUpper(finding.Priority), finding.Check, oneLine(finding.Evidence), oneLine(finding.Fix))
	}
	_ = writer.Flush()
}

func printPerformanceSummary(output io.Writer, report report.SiteReport) {
	if report.Performance.Available {
		fmt.Fprintf(
			output,
			"Performance: %d %s pages (worst LCP %.1fs, CLS %.3f, TBT %.0fms, TTFB %.0fms)\n",
			report.Performance.Summary.Pages,
			report.Performance.Profile,
			report.Performance.Summary.WorstLCP/1000,
			report.Performance.Summary.WorstCLS,
			report.Performance.Summary.WorstTBT,
			report.Performance.Summary.WorstTTFB,
		)
		return
	}
	if report.Performance.Profile != "" || len(report.Performance.Errors) > 0 {
		fmt.Fprintf(output, "Performance: unavailable (%s)\n", strings.Join(report.Performance.Errors, "; "))
	}
}

func printFindings(output io.Writer, findings []report.Finding) {
	groups := groupFindings(findings)
	if len(groups) == 0 {
		fmt.Fprintln(output, "No deterministic issues found in the public crawl.")
		return
	}
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "PRIORITY\tAREA\tISSUE\tOCCURRENCES\tEXAMPLE\tFIX")
	for _, group := range groups {
		example := ""
		if len(group.items) > 0 {
			example = strings.TrimSpace(group.items[0].URL + " " + group.items[0].Evidence)
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%d\t%s\t%s\n",
			strings.ToUpper(group.priority),
			group.category,
			group.check,
			len(group.items),
			oneLine(example),
			oneLine(group.fix),
		)
	}
	_ = writer.Flush()
}

func groupFindings(findings []report.Finding) []issueGroup {
	grouped := map[string]*issueGroup{}
	for _, finding := range findings {
		key := finding.Priority + "\x00" + finding.Category + "\x00" + finding.Check + "\x00" + finding.Fix
		if grouped[key] == nil {
			grouped[key] = &issueGroup{
				priority: finding.Priority,
				category: finding.Category,
				check:    finding.Check,
				fix:      finding.Fix,
			}
		}
		grouped[key].items = append(grouped[key].items, finding)
	}
	result := make([]issueGroup, 0, len(grouped))
	for _, group := range grouped {
		result = append(result, *group)
	}
	priority := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(result, func(i, j int) bool {
		if priority[result[i].priority] != priority[result[j].priority] {
			return priority[result[i].priority] < priority[result[j].priority]
		}
		if result[i].category != result[j].category {
			return result[i].category < result[j].category
		}
		return result[i].check < result[j].check
	})
	return result
}

func printJSON(output io.Writer, value any) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func saveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode audit report: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create audit output directory: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("save audit report: %w", err)
	}
	return nil
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

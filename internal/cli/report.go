package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
)

const terminalResultLimit = 10

func printReport(output io.Writer, report auditReport) {
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
	printPerformanceSummary(output, report)
	if report.LimitReached {
		fmt.Fprintln(output, "Warning: crawl limit reached; rerun with a higher --limit for complete coverage.")
	}
	fmt.Fprintln(output)
	printFindings(output, report.Findings)
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Use --json for every affected URL and the complete crawl dataset.")
}

func printOpportunityReport(output io.Writer, report opportunityReport) {
	fmt.Fprintf(output, "Search opportunities: %s\n", report.Target)
	if report.SearchConsole != nil {
		printGSCSummary(output, *report.SearchConsole)
	}
	if report.SearchData != nil {
		printSearchSummary(output, *report.SearchData)
	}
	if report.SearchConsole != nil && report.SearchConsole.Available {
		printGSCData(output, *report.SearchConsole)
	}
	if report.SearchData != nil && report.SearchData.Available {
		printOpportunityData(output, *report.SearchData)
	}
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Use --json for every returned row and complete provider evidence.")
}

func printBacklinkReport(output io.Writer, report providerReport) {
	fmt.Fprintf(output, "Backlink analysis: %s\n", report.Target)
	printBacklinkSummary(output, report)
	if report.Available {
		printBacklinkData(output, report)
	}
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Use --json for every returned backlink and complete provider evidence.")
}

func printGSCSummary(output io.Writer, report gscReport) {
	fmt.Fprintf(
		output,
		"Search Console: %s, %s to %s, %d returned query/page rows\n",
		report.SiteURL,
		report.StartDate,
		report.EndDate,
		report.Summary.Rows,
	)
	fmt.Fprintf(
		output,
		"GSC returned dataset: %.0f clicks, %.0f impressions, %.2f%% CTR, %.1f weighted position\n",
		report.Summary.ReturnedClicks,
		report.Summary.ReturnedImpressions,
		report.Summary.ReturnedCTR*100,
		report.Summary.WeightedPosition,
	)
}

func printPerformanceSummary(output io.Writer, report auditReport) {
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

func printSearchSummary(output io.Writer, report providerReport) {
	fmt.Fprintf(
		output,
		"Search data: DataForSEO %s/%s, %d of %d datasets, %d live calls, current provider cost $%.6f\n",
		report.Location,
		report.Language,
		report.SuccessfulCalls,
		report.RequestedDatasets,
		report.LiveCalls,
		report.CostUSD,
	)
	printProviderStorageSummary(output, report)
	if report.Available {
		visibility := report.OrganicVisibility
		fmt.Fprintf(
			output,
			"Visibility: %d ranking keywords, %.2f estimated monthly visits, %d top-10 rankings\n",
			visibility.Keywords,
			visibility.EstimatedTraffic,
			visibility.Position1+visibility.Positions2To3+visibility.Positions4To10,
		)
	}
	for _, datasetErr := range report.Errors {
		fmt.Fprintf(output, "DataForSEO warning: %s: %s\n", datasetErr.Dataset, datasetErr.Message)
	}
}

func printBacklinkSummary(output io.Writer, report providerReport) {
	fmt.Fprintf(
		output,
		"Backlink data: DataForSEO, %d of %d datasets, %d live calls, current provider cost $%.6f\n",
		report.SuccessfulCalls,
		report.RequestedDatasets,
		report.LiveCalls,
		report.CostUSD,
	)
	printProviderStorageSummary(output, report)
	if report.Available {
		links := report.BacklinkSummary
		fmt.Fprintf(
			output,
			"Authority signals: DataForSEO rank %d, %d backlinks from %d referring domains, target spam score %d\n",
			links.DataForSEORank,
			links.Backlinks,
			links.ReferringDomains,
			links.TargetSpamScore,
		)
	}
	for _, datasetErr := range report.Errors {
		fmt.Fprintf(output, "DataForSEO warning: %s: %s\n", datasetErr.Dataset, datasetErr.Message)
	}
}

func printProviderStorageSummary(output io.Writer, report providerReport) {
	if report.Cache.Hit {
		expiry := ""
		if report.Cache.ExpiresAt != nil {
			expiry = ", expires " + report.Cache.ExpiresAt.Format("2006-01-02 15:04 MST")
		}
		fmt.Fprintf(
			output,
			"Cache: hit, source retrieved %s%s, original provider cost $%.6f\n",
			report.RetrievedAt.Format("2006-01-02 15:04 MST"),
			expiry,
			report.Cache.CachedProviderCostUSD,
		)
	} else if report.Cache.Stored && report.Cache.ExpiresAt != nil {
		fmt.Fprintf(output, "Cache: stored until %s\n", report.Cache.ExpiresAt.Format("2006-01-02 15:04 MST"))
	}
	if report.SnapshotID > 0 {
		fmt.Fprintf(output, "Snapshot: SQLite row %d\n", report.SnapshotID)
	}
	for _, storageErr := range report.StorageErrors {
		fmt.Fprintf(output, "Storage warning: %s\n", storageErr)
	}
}

func printFindings(output io.Writer, findings []auditFinding) {
	groups := groupFindings(findings)
	if len(groups) == 0 {
		fmt.Fprintln(output, "No deterministic issues found in the public crawl.")
		return
	}
	writer := newTableWriter(output)
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

func printOpportunityData(output io.Writer, report providerReport) {
	if len(report.RankedKeywords) > 0 {
		writer := startTable(output, "Top ranking keywords", "POS\tKEYWORD\tVOLUME\tDIFFICULTY\tINTENT\tURL")
		for _, item := range first(report.RankedKeywords, terminalResultLimit) {
			fmt.Fprintf(writer, "%d\t%s\t%d\t%s\t%s\t%s\n",
				item.Position,
				oneLine(item.Keyword),
				item.SearchVolume,
				optionalNumber(item.Difficulty),
				item.Intent,
				item.URL,
			)
		}
		_ = writer.Flush()
	}
	if len(report.KeywordIdeas) > 0 {
		writer := startTable(output, "Keyword research", "KEYWORD\tVOLUME\tDIFFICULTY\tCPC\tINTENT")
		for _, item := range first(report.KeywordIdeas, terminalResultLimit) {
			fmt.Fprintf(writer, "%s\t%d\t%s\t%s\t%s\n",
				oneLine(item.Keyword),
				item.SearchVolume,
				optionalNumber(item.Difficulty),
				optionalMoney(item.CPC),
				item.Intent,
			)
		}
		_ = writer.Flush()
	}
	if len(report.Competitors) > 0 {
		writer := startTable(output, "Organic competitors", "DOMAIN\tOVERLAP\tORGANIC KEYWORDS\tEST. TRAFFIC")
		for _, item := range first(report.Competitors, terminalResultLimit) {
			fmt.Fprintf(writer, "%s\t%d\t%d\t%.2f\n",
				item.Domain,
				item.KeywordOverlap,
				item.OrganicKeywords,
				item.EstimatedTraffic,
			)
		}
		_ = writer.Flush()
	}
}

func printBacklinkData(output io.Writer, report providerReport) {
	if len(report.ReferringDomains) > 0 {
		writer := startTable(output, "Top referring domains", "DOMAIN\tDFS RANK\tBACKLINKS\tNOFOLLOW PAGES")
		for _, item := range first(report.ReferringDomains, terminalResultLimit) {
			fmt.Fprintf(writer, "%s\t%d\t%d\t%d\n",
				item.Domain,
				item.DataForSEORank,
				item.Backlinks,
				item.NofollowReferringPages,
			)
		}
		_ = writer.Flush()
	}
	if len(report.TopBacklinks) > 0 {
		writer := startTable(output, "Top backlinks", "SOURCE\tRANK\tFOLLOW\tTARGET")
		for _, item := range first(report.TopBacklinks, terminalResultLimit) {
			follow := "nofollow"
			if item.Dofollow {
				follow = "dofollow"
			}
			fmt.Fprintf(writer, "%s\t%d\t%s\t%s\n",
				item.SourceURL,
				item.LinkRank,
				follow,
				item.TargetURL,
			)
		}
		_ = writer.Flush()
	}
}

func printGSCData(output io.Writer, report gscReport) {
	if len(report.StrikingDistance) > 0 {
		writer := startTable(output, "Search Console opportunities", "POS\tQUERY\tIMPRESSIONS\tCLICKS\tCTR\tPAGE")
		for _, item := range first(report.StrikingDistance, terminalResultLimit) {
			fmt.Fprintf(writer, "%.1f\t%s\t%.0f\t%.0f\t%.2f%%\t%s\n",
				item.Position,
				oneLine(item.Query),
				item.Impressions,
				item.Clicks,
				item.CTR*100,
				item.Page,
			)
		}
		_ = writer.Flush()
	}
	if len(report.QueryOverlaps) > 0 {
		writer := startTable(output, "Observed query overlap", "QUERY\tPAGES\tIMPRESSIONS\tEXAMPLES")
		for _, item := range first(report.QueryOverlaps, terminalResultLimit) {
			fmt.Fprintf(writer, "%s\t%d\t%.0f\t%s\n",
				oneLine(item.Query),
				len(item.Pages),
				item.Impressions,
				strings.Join(first(item.Pages, 2), ", "),
			)
		}
		_ = writer.Flush()
	}
}

func startTable(output io.Writer, title, header string) *tabwriter.Writer {
	fmt.Fprintln(output)
	fmt.Fprintln(output, title)
	writer := newTableWriter(output)
	fmt.Fprintln(writer, header)
	return writer
}

func newTableWriter(output io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
}

func groupFindings(findings []auditFinding) []issueGroup {
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

func printRawJSON(output io.Writer, data []byte) error {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, data, "", "  "); err != nil {
		return fmt.Errorf("format API response: %w", err)
	}
	formatted.WriteByte('\n')
	_, err := output.Write(formatted.Bytes())
	return err
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func first[T any](items []T, limit int) []T {
	if len(items) <= limit {
		return items
	}
	return items[:limit]
}

func optionalNumber(value *float64) string {
	if value == nil {
		return "-"
	}
	return strconv.FormatFloat(*value, 'f', 0, 64)
}

func optionalMoney(value *float64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("$%.2f", *value)
}

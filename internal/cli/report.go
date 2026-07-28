package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/simonbalfe/seo-audit/internal/dataforseo"
)

const terminalResultLimit = 10

func printReport(output io.Writer, report audit.SiteReport, searchData *dataforseo.Report) {
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
	if searchData != nil {
		printSearchSummary(output, *searchData)
	}
	fmt.Fprintln(output)
	printFindings(output, report.Findings)
	if searchData != nil && searchData.Available {
		printSearchData(output, *searchData)
	}
	fmt.Fprintln(output)
	fmt.Fprintln(output, "Use --json for every affected URL and the complete crawl dataset.")
}

func printPerformanceSummary(output io.Writer, report audit.SiteReport) {
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

func printSearchSummary(output io.Writer, report dataforseo.Report) {
	fmt.Fprintf(
		output,
		"Search data: DataForSEO %s/%s, %d of %d datasets, provider cost $%.6f\n",
		report.Location,
		report.Language,
		report.SuccessfulCalls,
		dataforseo.DatasetCount,
		report.CostUSD,
	)
	if report.Available {
		visibility := report.OrganicVisibility
		links := report.BacklinkSummary
		fmt.Fprintf(
			output,
			"Visibility: %d ranking keywords, %.2f estimated monthly visits, %d top-10 rankings\n",
			visibility.Keywords,
			visibility.EstimatedTraffic,
			visibility.Position1+visibility.Positions2To3+visibility.Positions4To10,
		)
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

func printFindings(output io.Writer, findings []audit.Finding) {
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

func printSearchData(output io.Writer, report dataforseo.Report) {
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

func groupFindings(findings []audit.Finding) []issueGroup {
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

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func domainTarget(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("cannot determine domain from %q", rawURL)
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www."), nil
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

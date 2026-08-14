package audit

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/audit/crawl"
	"github.com/simonbalfe/seo-audit/internal/audit/performance"
	"github.com/simonbalfe/seo-audit/internal/dataforseo"
	"github.com/simonbalfe/seo-audit/internal/report"
)

// Run performs one public SEO audit.
func Run(ctx context.Context, rawURL string, timeout time.Duration, options Options) (SiteReport, error) {
	started := time.Now()
	result, err := crawl.NewClient(timeout).Audit(ctx, rawURL, crawl.Options{
		Limit:         options.Limit,
		CheckExternal: options.CheckExternal,
		Progress:      options.Progress,
	})
	if err != nil {
		return SiteReport{}, err
	}
	if options.Market != nil {
		classifyPages(ctx, &result, options)
	}
	if options.CheckPerformance {
		result.Performance = performance.Inspect(ctx, result, performance.Options{
			Progress: options.Progress,
		})
		for _, measured := range result.Performance.Pages {
			result.Findings = append(result.Findings, measured.Findings...)
		}
	}
	var dataClient *dataforseo.Client
	var dataErr error
	if options.Market != nil || options.CheckBacklinks {
		dataClient, dataErr = dataforseo.NewClient()
	}
	if options.Market != nil {
		if dataErr != nil {
			result.Market = report.MarketReport{
				Enabled:   true,
				Source:    "DataForSEO",
				Target:    rawURL,
				Location:  options.Market.Location,
				Language:  options.Market.Language,
				MaxChecks: options.Market.MaxChecks,
				Errors:    []report.MarketError{{Operation: "authentication", Message: dataErr.Error()}},
			}
		} else {
			result.Market = dataClient.Scan(ctx, result.Pages, dataforseo.Options{
				Target:          rawURL,
				Location:        options.Market.Location,
				Language:        options.Market.Language,
				MaxChecks:       options.Market.MaxChecks,
				Keywords:        options.Market.Keywords,
				TargetName:      options.Market.TargetName,
				TargetCategory:  options.Market.TargetCategory,
				TargetCountry:   options.Market.TargetCountry,
				TargetPlaceID:   options.Market.TargetPlaceID,
				TargetLatitude:  options.Market.TargetLatitude,
				TargetLongitude: options.Market.TargetLongitude,
				GridRadiusKM:    options.Market.GridRadiusKM,
				AIKeywords:      priorityKeywordSeeds(result.Pages),
				Progress: func(message string) {
					emitProgress(options, "visibility", "%s", message)
				},
			})
			assignTargetKeywords(&result)
		}
	}
	if options.CheckBacklinks {
		result.Backlinks = backlinkSummary(ctx, rawURL, dataClient, dataErr, options.Progress)
	}
	crawl.Finalize(&result)
	result.Duration = time.Since(started).Milliseconds()
	emitProgress(options, "done", "Completed in %.1fs: %d pages, %d failures, %d warnings", float64(result.Duration)/1000, result.Summary.Pages, result.Summary.Failures, result.Summary.Warnings)
	return result, nil
}

// Backlinks runs only the live backlink summary stage.
func Backlinks(ctx context.Context, rawURL string, progress func(ProgressEvent)) report.BacklinkReport {
	client, err := dataforseo.NewClient()
	return backlinkSummary(ctx, rawURL, client, err, progress)
}

func backlinkSummary(ctx context.Context, rawURL string, client *dataforseo.Client, clientErr error, progress func(ProgressEvent)) report.BacklinkReport {
	options := Options{Progress: progress}
	emitProgress(options, "backlinks", "Fetching one DataForSEO backlink summary")
	result := report.BacklinkReport{Enabled: true, Source: "DataForSEO", Target: rawURL}
	if clientErr != nil {
		result.Error = clientErr.Error()
	} else {
		result = client.Backlinks(ctx, rawURL)
	}
	emitProgress(options, "backlinks", "Completed backlink summary at provider cost $%.6f", result.CostUSD)
	return result
}

func priorityKeywordSeeds(pages []report.PageReport) []string {
	result := make([]string, 0)
	seen := map[string]bool{}
	for _, page := range pages {
		if !page.PriorityPage {
			continue
		}
		for _, keyword := range page.KeywordSeeds {
			if !seen[keyword] {
				seen[keyword] = true
				result = append(result, keyword)
			}
		}
	}
	return result
}

func assignTargetKeywords(site *SiteReport) {
	pages := map[string]*report.PageReport{}
	for index := range site.Pages {
		page := &site.Pages[index]
		page.TargetKeywords = nil
		if !page.PriorityPage {
			continue
		}
		pages[comparablePageURL(page.URL)] = page
		pages[comparablePageURL(page.FinalURL)] = page
	}
	for _, opportunity := range site.Market.Opportunities {
		targetURL := opportunity.TargetURL
		if targetURL == "" {
			targetURL = opportunity.URL
		}
		page := pages[comparablePageURL(targetURL)]
		if page == nil {
			continue
		}
		found := false
		for _, keyword := range page.TargetKeywords {
			found = found || keyword == opportunity.Keyword
		}
		if !found {
			page.TargetKeywords = append(page.TargetKeywords, opportunity.Keyword)
		}
	}
	for index := range site.Pages {
		sort.Strings(site.Pages[index].TargetKeywords)
	}
}

func comparablePageURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String()
}

func emitProgress(options Options, stage, format string, values ...any) {
	if options.Progress == nil {
		return
	}
	options.Progress(ProgressEvent{
		Stage:   stage,
		Message: fmt.Sprintf(format, values...),
	})
}

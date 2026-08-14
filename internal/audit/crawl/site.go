package crawl

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const pageWorkerCount = 16

type crawlItem struct {
	URL   string
	Depth int
}

type crawlResult struct {
	item crawlItem
	page PageReport
	err  error
}

func (c *Client) Audit(ctx context.Context, rawURL string, options Options) (SiteReport, error) {
	started := time.Now()
	start, err := normalizeStartURL(rawURL)
	if err != nil {
		return SiteReport{}, err
	}
	if options.Limit <= 0 {
		options.Limit = 50
	}
	startURL := normalizeCrawlURL(start)
	emitProgress(options, "setup", "Starting %s with a %d-page limit and %d crawl workers (external checks: %t)", startURL, options.Limit, pageWorkerCount, options.CheckExternal)
	emitProgress(options, "robots", "Fetching %s", (&url.URL{Scheme: start.Scheme, Host: start.Host, Path: "/robots.txt"}).String())
	robots, robotsErr := c.InspectRobots(ctx, startURL)
	if robotsErr != nil {
		robots = RobotsReport{URL: (&url.URL{Scheme: start.Scheme, Host: start.Host, Path: "/robots.txt"}).String()}
		emitProgress(options, "robots", "Could not fetch robots.txt: %v", robotsErr)
	} else {
		emitProgress(options, "robots", "HTTP %d; evaluated %d crawler agents; found %d sitemap declarations", robots.StatusCode, len(robots.Agents), len(robots.Sitemaps))
	}
	emitProgress(options, "sitemaps", "Discovering and parsing XML sitemaps")
	sitemaps, _ := c.InspectSitemaps(ctx, startURL)
	emitProgress(options, "sitemaps", "Parsed %d sitemap files with %d URLs and %d errors", len(sitemaps.Sources), len(sitemaps.URLs), len(sitemaps.Errors))
	for _, sitemapErr := range sitemaps.Errors {
		emitProgress(options, "sitemaps", "Error: %s", sitemapErr)
	}
	sitemapSet := map[string]bool{}
	for _, item := range sitemaps.URLs {
		parsed, parseErr := url.Parse(item)
		if parseErr == nil && sameHost(start, parsed) {
			sitemapSet[normalizeCrawlURL(parsed)] = true
		}
	}
	groups, _ := parseRobots(robots.Body)
	report := SiteReport{StartURL: startURL, Robots: robots, Sitemaps: sitemaps}
	queue := []crawlItem{{URL: startURL, Depth: 0}}
	queued := map[string]bool{startURL: true}
	seen := map[string]bool{}

	crawlBatch := func(items []crawlItem) []crawlResult {
		results := make([]crawlResult, len(items))
		var workers sync.WaitGroup
		for index, item := range items {
			workers.Add(1)
			go func() {
				defer workers.Done()
				page, inspectErr := c.InspectPage(ctx, item.URL)
				results[index] = crawlResult{item: item, page: page, err: inspectErr}
			}()
		}
		workers.Wait()
		return results
	}

	applyResult := func(result crawlResult) {
		item := result.item
		page := result.page
		if result.err != nil {
			report.Findings = append(report.Findings, newFinding(
				"response",
				"Fetch failed",
				Fail,
				"high",
				item.URL,
				result.err.Error(),
				"Make the URL publicly reachable and return a valid response.",
			))
			emitProgress(options, "crawl", "Fetch failed for %s: %v", item.URL, result.err)
			return
		}
		page.Depth = item.Depth
		queued[page.FinalURL] = true
		seen[page.FinalURL] = true
		page.InSitemap = sitemapSet[page.URL] || sitemapSet[page.FinalURL]
		page.RobotsAllowed, _ = evaluateRobots(groups, "Googlebot", pathOf(page.URL))
		if !page.RobotsAllowed {
			page.Indexable = false
			page.Indexability = "blocked by robots.txt"
			page.Findings = append(page.Findings, newFinding(
				"crawlability",
				"Blocked by robots.txt",
				Fail,
				"high",
				page.URL,
				pathOf(page.URL),
				"Allow Googlebot to crawl the page if it should appear in search.",
			))
		}
		report.Pages = append(report.Pages, page)
		report.Findings = append(report.Findings, page.Findings...)
		indexability := "indexable"
		if !page.Indexable {
			indexability = "non-indexable: " + page.Indexability
		}
		contentSource := "raw HTML"
		if page.Rendered {
			contentSource = "browser-rendered HTML"
		}
		emitProgress(
			options,
			"crawl",
			"Fetched page %d: HTTP %d, %d words, %d internal links, %s, %s, %dms: %s",
			len(report.Pages),
			page.StatusCode,
			page.WordCount,
			len(page.InternalLinks),
			indexability,
			contentSource,
			page.Duration,
			page.FinalURL,
		)
		for _, link := range page.InternalLinks {
			parsed, parseErr := url.Parse(link)
			if parseErr != nil || !sameHost(start, parsed) {
				continue
			}
			normalized := normalizeCrawlURL(parsed)
			if !queued[normalized] {
				queued[normalized] = true
				queue = append(queue, crawlItem{URL: normalized, Depth: item.Depth + 1})
			}
		}
	}

	crawlQueued := func() {
		for len(queue) > 0 && len(report.Pages) < options.Limit {
			remaining := options.Limit - len(report.Pages)
			batch := make([]crawlItem, 0, min(pageWorkerCount, remaining))
			for len(queue) > 0 && len(batch) < pageWorkerCount && len(batch) < remaining {
				item := queue[0]
				queue = queue[1:]
				if seen[item.URL] {
					continue
				}
				seen[item.URL] = true
				batch = append(batch, item)
			}
			for index, item := range batch {
				source := fmt.Sprintf("depth %d", item.Depth)
				if item.Depth < 0 {
					source = "sitemap-only"
				}
				emitProgress(options, "crawl", "Fetching page %d (%s, %d queued): %s", len(report.Pages)+index+1, source, len(queue), item.URL)
			}
			for _, result := range crawlBatch(batch) {
				applyResult(result)
			}
		}
	}
	crawlQueued()
	sitemapURLs := make([]string, 0, len(sitemapSet))
	for sitemapURL := range sitemapSet {
		sitemapURLs = append(sitemapURLs, sitemapURL)
	}
	sort.Strings(sitemapURLs)
	for _, sitemapURL := range sitemapURLs {
		if seen[sitemapURL] || queued[sitemapURL] {
			continue
		}
		queued[sitemapURL] = true
		queue = append(queue, crawlItem{URL: sitemapURL, Depth: -1})
	}
	crawlQueued()
	if len(report.Pages) == 0 {
		for _, finding := range report.Findings {
			if finding.Check == "Fetch failed" {
				return SiteReport{}, fmt.Errorf("could not fetch start URL: %s", finding.Evidence)
			}
		}
		return SiteReport{}, errors.New("could not fetch any public URLs")
	}
	report.LimitReached = len(queue) > 0 || hasUnseenSitemapURL(sitemapSet, seen)
	emitProgress(options, "crawl", "Crawl complete: %d pages fetched; limit reached: %t", len(report.Pages), report.LimitReached)
	emitProgress(options, "analysis", "Analyzing architecture, links, canonicals, duplicates, hreflang, and URL format")
	analyzeSite(&report)
	emitProgress(options, "analysis", "Site analysis produced %d findings before resource checks", len(report.Findings))
	report.Resources = c.checkResources(ctx, report, options)
	emitProgress(options, "analysis", "Analyzing %d resource responses", len(report.Resources))
	analyzeResources(&report)
	Finalize(&report)
	report.Duration = time.Since(started).Milliseconds()
	return report, nil
}

func Finalize(report *SiteReport) {
	report.Summary = summarizeSite(*report)
	sort.Slice(report.Pages, func(i, j int) bool { return report.Pages[i].URL < report.Pages[j].URL })
	sortFindings(report.Findings)
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

func analyzeSite(report *SiteReport) {
	pageByURL := map[string]*PageReport{}
	inlinks := map[string]int{}
	sitemapURLs := map[string]bool{}
	for _, sitemapURL := range report.Sitemaps.URLs {
		sitemapURLs[normalizeComparableURL(sitemapURL)] = true
	}
	for index := range report.Pages {
		page := &report.Pages[index]
		pageByURL[normalizeComparableURL(page.URL)] = page
		pageByURL[normalizeComparableURL(page.FinalURL)] = page
		for _, link := range page.InternalLinks {
			inlinks[normalizeComparableURL(link)]++
		}
	}
	for index := range report.Pages {
		page := &report.Pages[index]
		page.Inlinks = inlinks[normalizeComparableURL(page.URL)]
		isHTML := isHTMLContent(page.ContentType)
		if page.Depth < 0 && page.InSitemap && page.Indexable && isHTML {
			addSiteFinding(report, "architecture", "Sitemap-only page", Warn, "medium", page.URL, "not found through internal links", "Add a relevant internal link or remove the URL from the sitemap.")
		}
		if page.Depth > 3 && page.Indexable && isHTML {
			addSiteFinding(report, "architecture", "Deep page", Warn, "low", page.URL, fmt.Sprintf("crawl depth %d", page.Depth), "Link important pages closer to the homepage.")
		}
		if page.Depth > 0 && page.Inlinks == 0 && page.Indexable && isHTML {
			addSiteFinding(report, "architecture", "No internal inlinks", Warn, "medium", page.URL, "zero discovered internal links", "Link to the page from a relevant indexable page.")
		}
		if page.InSitemap {
			if page.StatusCode < 200 || page.StatusCode >= 300 {
				addSiteFinding(report, "sitemap", "Non-success URL in sitemap", Fail, "high", page.URL, fmt.Sprintf("HTTP %d", page.StatusCode), "Keep only successful canonical URLs in XML sitemaps.")
			}
			if !page.Indexable {
				addSiteFinding(report, "sitemap", "Non-indexable URL in sitemap", Fail, "high", page.URL, page.Indexability, "Keep only canonical indexable URLs in XML sitemaps.")
			}
			if len(page.RedirectChain) > 0 && sitemapURLs[normalizeComparableURL(page.URL)] {
				addSiteFinding(report, "sitemap", "Redirecting URL in sitemap", Fail, "high", page.URL, strings.Join(page.RedirectChain, " -> "), "Replace it with the final canonical URL.")
			}
		}
		if page.Indexable && page.Canonical != "" {
			target := pageByURL[normalizeComparableURL(page.Canonical)]
			if target != nil && (!target.Indexable || target.StatusCode < 200 || target.StatusCode >= 300) {
				addSiteFinding(report, "canonical", "Canonical points to non-indexable URL", Fail, "high", page.URL, page.Canonical, "Point the canonical to a successful indexable URL.")
			}
		}
		analyzeURL(report, *page)
	}
	analyzeLinks(report, pageByURL)
	analyzeDuplicates(report)
	analyzeHreflang(report, pageByURL)
	if report.Robots.StatusCode == 0 || report.Robots.StatusCode == http.StatusTooManyRequests || report.Robots.StatusCode >= 500 {
		addSiteFinding(report, "crawlability", "Robots.txt unavailable", Warn, "medium", report.Robots.URL, fmt.Sprintf("HTTP %d", report.Robots.StatusCode), "Return a valid robots.txt or an intentional 404 without blocking crawlers.")
	}
	for _, sitemapErr := range report.Sitemaps.Errors {
		if strings.Contains(sitemapErr, "/sitemap.xml returned 404") && len(report.Sitemaps.Sources) == 1 {
			addSiteFinding(report, "sitemap", "No XML sitemap found", Warn, "medium", "", sitemapErr, "Add an XML sitemap when it would help search engines discover canonical URLs.")
			continue
		}
		addSiteFinding(report, "sitemap", "Sitemap error", Fail, "high", "", sitemapErr, "Return valid XML sitemap files.")
	}
}

func analyzeLinks(report *SiteReport, pages map[string]*PageReport) {
	for _, source := range report.Pages {
		reported := map[string]bool{}
		for _, link := range source.Links {
			if !link.Internal {
				continue
			}
			target := pages[normalizeComparableURL(link.URL)]
			if target == nil {
				continue
			}
			if target.StatusCode >= 400 || target.StatusCode == 0 {
				addLinkFinding(report, reported, source.URL, link.URL, "Broken internal link", Fail, "high", fmt.Sprintf("%s returned %d", link.URL, target.StatusCode), "Update or remove the broken link.")
			} else if len(target.RedirectChain) > 0 {
				addLinkFinding(report, reported, source.URL, link.URL, "Internal link redirects", Warn, "medium", link.URL, "Link directly to the final canonical URL.")
			}
			if link.NoFollow {
				addLinkFinding(report, reported, source.URL, link.URL, "Nofollow internal link", Warn, "low", link.URL, "Remove nofollow unless there is a specific reason to prevent normal crawling.")
			}
			if strings.TrimSpace(link.Text) == "" {
				addLinkFinding(report, reported, source.URL, link.URL, "Empty internal anchor", Warn, "low", link.URL, "Give linked images alt text or add descriptive anchor text.")
			}
		}
	}
}

func addLinkFinding(report *SiteReport, reported map[string]bool, source, target, check string, status Status, priority, evidence, fix string) {
	key := check + "\x00" + normalizeComparableURL(target)
	if reported[key] {
		return
	}
	reported[key] = true
	addSiteFinding(report, "links", check, status, priority, source, evidence, fix)
}

func analyzeDuplicates(report *SiteReport) {
	analyzeDuplicateField(report, "Duplicate title", "on-page", func(page PageReport) string { return strings.ToLower(cleanText(page.Title)) })
	analyzeDuplicateField(report, "Duplicate meta description", "on-page", func(page PageReport) string { return strings.ToLower(cleanText(page.Description)) })
	analyzeDuplicateField(report, "Duplicate H1", "headings", func(page PageReport) string {
		if len(page.H1) == 0 {
			return ""
		}
		return strings.ToLower(cleanText(page.H1[0]))
	})
	analyzeDuplicateField(report, "Exact duplicate content", "content", func(page PageReport) string {
		if page.WordCount < 50 {
			return ""
		}
		return page.ContentHash
	})
	for left := 0; left < len(report.Pages); left++ {
		if !report.Pages[left].Indexable || report.Pages[left].WordCount < 100 {
			continue
		}
		for right := left + 1; right < len(report.Pages); right++ {
			if !report.Pages[right].Indexable || report.Pages[right].WordCount < 100 || report.Pages[left].ContentHash == report.Pages[right].ContentHash {
				continue
			}
			similarity := jaccard(report.Pages[left].TextTokens, report.Pages[right].TextTokens)
			if similarity >= 0.9 {
				addSiteFinding(report, "content", "Near-duplicate content", Warn, "medium", report.Pages[left].URL, fmt.Sprintf("%.0f%% similar to %s", similarity*100, report.Pages[right].URL), "Consolidate the pages or make their purpose and content genuinely distinct.")
			}
		}
	}
}

func analyzeDuplicateField(report *SiteReport, check, category string, field func(PageReport) string) {
	groups := map[string][]string{}
	for _, page := range report.Pages {
		if !page.Indexable {
			continue
		}
		value := field(page)
		if value != "" {
			groups[value] = append(groups[value], page.URL)
		}
	}
	for _, urls := range groups {
		if len(urls) > 1 {
			for _, duplicateURL := range urls {
				addSiteFinding(report, category, check, Warn, "medium", duplicateURL, strings.Join(urls, ", "), "Make each indexable page distinct or consolidate duplicates.")
			}
		}
	}
}

func analyzeHreflang(report *SiteReport, pages map[string]*PageReport) {
	for _, page := range report.Pages {
		for _, alternate := range page.Hreflang {
			target := pages[normalizeComparableURL(alternate.URL)]
			if target == nil {
				addSiteFinding(report, "hreflang", "Hreflang target not crawled", Warn, "medium", page.URL, alternate.Language+": "+alternate.URL, "Use a successful canonical URL and ensure it is crawlable.")
				continue
			}
			if target.StatusCode != 200 || !target.Indexable {
				addSiteFinding(report, "hreflang", "Invalid hreflang target", Fail, "high", page.URL, alternate.Language+": "+alternate.URL, "Point hreflang to a successful indexable canonical URL.")
			}
			if !hasReturnHreflang(*target, page.URL) {
				addSiteFinding(report, "hreflang", "Missing hreflang return link", Fail, "high", page.URL, alternate.URL, "Add a reciprocal hreflang reference.")
			}
		}
	}
}

func analyzeURL(report *SiteReport, page PageReport) {
	if !isHTMLContent(page.ContentType) {
		return
	}
	parsed, err := url.Parse(page.URL)
	if err != nil {
		return
	}
	if parsed.RawQuery != "" {
		addSiteFinding(report, "url", "URL parameters", Warn, "low", page.URL, parsed.RawQuery, "Confirm parameter URLs are intentional and controlled.")
	}
	if parsed.Path != strings.ToLower(parsed.Path) {
		addSiteFinding(report, "url", "Uppercase URL", Warn, "low", page.URL, parsed.Path, "Use a consistent lowercase URL format.")
	}
	if strings.Contains(parsed.Path, "_") {
		addSiteFinding(report, "url", "Underscore in URL", Warn, "low", page.URL, parsed.Path, "Prefer readable hyphen-separated words for new URLs.")
	}
	if len(page.URL) > 150 {
		addSiteFinding(report, "url", "Long URL", Warn, "low", page.URL, fmt.Sprintf("%d characters", len(page.URL)), "Shorten the URL where this can be done safely.")
	}
}

func summarizeSite(report SiteReport) Summary {
	summary := Summary{Pages: len(report.Pages), SitemapURLs: len(report.Sitemaps.URLs)}
	internalLinks := map[string]bool{}
	externalLinks := map[string]bool{}
	for _, page := range report.Pages {
		if page.Indexable {
			summary.Indexable++
		} else {
			summary.NonIndexable++
		}
		for _, link := range page.InternalLinks {
			internalLinks[link] = true
		}
		for _, link := range page.ExternalLinks {
			externalLinks[link] = true
		}
	}
	for _, finding := range report.Findings {
		if finding.Status == Fail {
			summary.Failures++
		}
		if finding.Status == Warn {
			summary.Warnings++
		}
		if finding.Check == "Broken internal link" {
			summary.BrokenInternal++
		}
		if finding.Check == "Internal link redirects" {
			summary.RedirectedInternal++
		}
	}
	summary.InternalLinks = len(internalLinks)
	summary.ExternalLinks = len(externalLinks)
	return summary
}

func addSiteFinding(report *SiteReport, category, check string, status Status, priority, target, evidence, fix string) {
	report.Findings = append(report.Findings, newFinding(category, check, status, priority, target, evidence, fix))
}

func pathOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.EscapedPath() == "" {
		return "/"
	}
	return parsed.EscapedPath()
}

func hasUnseenSitemapURL(sitemap, seen map[string]bool) bool {
	for item := range sitemap {
		if !seen[item] {
			return true
		}
	}
	return false
}

func hasReturnHreflang(page PageReport, target string) bool {
	for _, alternate := range page.Hreflang {
		if normalizeComparableURL(alternate.URL) == normalizeComparableURL(target) {
			return true
		}
	}
	return false
}

func jaccard(left, right []string) float64 {
	leftSet := map[string]bool{}
	rightSet := map[string]bool{}
	for _, token := range left {
		leftSet[token] = true
	}
	for _, token := range right {
		rightSet[token] = true
	}
	if len(leftSet) == 0 || len(rightSet) == 0 {
		return 0
	}
	intersection := 0
	union := map[string]bool{}
	for token := range leftSet {
		union[token] = true
		if rightSet[token] {
			intersection++
		}
	}
	for token := range rightSet {
		union[token] = true
	}
	return float64(intersection) / float64(len(union))
}

func sortFindings(findings []Finding) {
	weight := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(findings, func(i, j int) bool {
		if weight[findings[i].Priority] != weight[findings[j].Priority] {
			return weight[findings[i].Priority] < weight[findings[j].Priority]
		}
		if findings[i].Check != findings[j].Check {
			return findings[i].Check < findings[j].Check
		}
		return findings[i].URL < findings[j].URL
	})
}

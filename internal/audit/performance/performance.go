package performance

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/simonbalfe/seo-audit/internal/audit/browser"
	"github.com/simonbalfe/seo-audit/internal/report"
)

const (
	performanceProfile   = "simulated mobile lab"
	performancePageLimit = 6
)

type performanceMetrics struct {
	FCPMilliseconds                float64 `json:"fcp"`
	LCPMilliseconds                float64 `json:"lcp"`
	CLS                            float64 `json:"cls"`
	TBTMilliseconds                float64 `json:"tbt"`
	TTFBMilliseconds               float64 `json:"ttfb"`
	DOMContentLoadedMilliseconds   float64 `json:"domContentLoaded"`
	LoadMilliseconds               float64 `json:"load"`
	DOMNodes                       int     `json:"domNodes"`
	ImagesMissingDimensions        int     `json:"imagesMissingDimensions"`
	OffscreenImagesWithoutLazyLoad int     `json:"offscreenImagesWithoutLazyLoad"`
}

type performanceResource struct {
	URL  string
	Type network.ResourceType
}

type performanceResourceTotals struct {
	Requests           int
	TransferBytes      int64
	JavaScriptBytes    int64
	CSSBytes           int64
	ImageBytes         int64
	ThirdPartyRequests int
}

type performanceResourceCollector struct {
	mu        sync.Mutex
	resources map[network.RequestID]performanceResource
	totals    performanceResourceTotals
	origin    *url.URL
}

type performanceSection struct {
	Name  string
	Count int
	URL   string
	Depth int
}

type Finding = report.Finding
type PageReport = report.PageReport
type PerformancePageReport = report.PerformancePageReport
type PerformanceSummary = report.PerformanceSummary
type PerformanceReport = report.PerformanceReport
type SiteReport = report.SiteReport

const Warn = report.Warn

type Options struct {
	Progress func(report.ProgressEvent)
}

const performanceObserverScript = `
(() => {
  const state = {
    lcp: 0,
    cls: 0,
    clsSessionValue: 0,
    clsSessionStart: 0,
    clsLastShift: 0,
    longTasks: []
  };
  window.__seoAuditPerformance = state;
  try {
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        state.lcp = Math.max(state.lcp, entry.startTime || entry.renderTime || entry.loadTime || 0);
      }
    }).observe({ type: 'largest-contentful-paint', buffered: true });
  } catch {}
  try {
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        if (entry.hadRecentInput) continue;
        if (
          state.clsSessionStart > 0 &&
          entry.startTime - state.clsLastShift < 1000 &&
          entry.startTime - state.clsSessionStart < 5000
        ) {
          state.clsSessionValue += entry.value;
        } else {
          state.clsSessionValue = entry.value;
          state.clsSessionStart = entry.startTime;
        }
        state.clsLastShift = entry.startTime;
        state.cls = Math.max(state.cls, state.clsSessionValue);
      }
    }).observe({ type: 'layout-shift', buffered: true });
  } catch {}
  try {
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        state.longTasks.push({ start: entry.startTime, duration: entry.duration });
      }
    }).observe({ type: 'longtask', buffered: true });
  } catch {}
})();
`

const performanceEvaluationScript = `
(() => {
  const state = window.__seoAuditPerformance || { lcp: 0, cls: 0, longTasks: [] };
  const navigation = performance.getEntriesByType('navigation')[0] || {};
  const paint = performance.getEntriesByName('first-contentful-paint')[0];
  const fcp = paint ? paint.startTime : 0;
  const tbt = (state.longTasks || []).reduce((total, task) => {
    if (task.start < fcp) return total;
    return total + Math.max(0, task.duration - 50);
  }, 0);
  const images = Array.from(document.images);
  return {
    fcp,
    lcp: state.lcp || 0,
    cls: state.cls || 0,
    tbt,
    ttfb: navigation.responseStart || 0,
    domContentLoaded: navigation.domContentLoadedEventEnd || 0,
    load: navigation.loadEventEnd || 0,
    domNodes: document.querySelectorAll('*').length,
    imagesMissingDimensions: images.filter((image) => !image.hasAttribute('width') || !image.hasAttribute('height')).length,
    offscreenImagesWithoutLazyLoad: images.filter((image) => {
      const rect = image.getBoundingClientRect();
      return rect.top > window.innerHeight && image.loading !== 'lazy';
    }).length
  };
})();
`

func Inspect(ctx context.Context, report SiteReport, options Options) PerformanceReport {
	result := PerformanceReport{Profile: performanceProfile}
	binary := browser.Binary()
	if binary == "" {
		result.Errors = append(result.Errors, "Chrome or Chromium not found")
		return result
	}
	targets := selectPerformancePages(report.Pages, performancePageLimit)
	if len(targets) == 0 {
		result.Errors = append(result.Errors, "no indexable HTML pages available for performance testing")
		return result
	}
	emitProgress(options, "Selected %d representative pages for %s testing", len(targets), performanceProfile)

	allocatorOptions := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocatorOptions = append(allocatorOptions, chromedp.ExecPath(binary))
	allocatorContext, allocatorCancel := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	defer allocatorCancel()
	browserContext, browserCancel := chromedp.NewContext(allocatorContext)
	defer browserCancel()
	if err := chromedp.Run(browserContext); err != nil {
		result.Errors = append(result.Errors, err.Error())
		return result
	}

	for index, target := range targets {
		emitProgress(options, "Testing %d/%d: %s", index+1, len(targets), target)
		tabContext, tabCancel := chromedp.NewContext(browserContext)
		pageContext, pageCancel := context.WithTimeout(tabContext, 45*time.Second)
		measured, err := inspectPerformancePage(pageContext, target)
		pageCancel()
		tabCancel()
		if err != nil {
			message := fmt.Sprintf("%s: %v", target, err)
			result.Errors = append(result.Errors, message)
			emitProgress(options, "Could not test %s: %v", target, err)
			continue
		}
		measured.Findings = performanceFindings(measured)
		result.Pages = append(result.Pages, measured)
		emitProgress(
			options,
			"LCP %.0fms, CLS %.3f, TBT %.0fms, TTFB %.0fms, %d requests, %.1fMB: %s",
			measured.LCPMilliseconds,
			measured.CLS,
			measured.TBTMilliseconds,
			measured.TTFBMilliseconds,
			measured.Requests,
			float64(measured.TransferBytes)/(1024*1024),
			target,
		)
	}
	result.Available = len(result.Pages) > 0
	result.Summary = summarizePerformance(result)
	return result
}

func inspectPerformancePage(ctx context.Context, target string) (PerformancePageReport, error) {
	started := time.Now()
	parsed, err := url.Parse(target)
	if err != nil {
		return PerformancePageReport{}, err
	}
	collector := &performanceResourceCollector{
		resources: map[network.RequestID]performanceResource{},
		origin:    parsed,
	}
	chromedp.ListenTarget(ctx, collector.handle)
	var metrics performanceMetrics
	err = chromedp.Run(
		ctx,
		network.Enable(),
		network.SetCacheDisabled(true),
		network.SetBypassServiceWorker(true),
		emulation.SetDeviceMetricsOverride(390, 844, 3, true),
		emulation.SetTouchEmulationEnabled(true),
		emulation.SetCPUThrottlingRate(4),
		network.EmulateNetworkConditions(false, 150, 200*1024, 90*1024),
		chromedp.ActionFunc(func(ctx context.Context) error {
			_, scriptErr := page.AddScriptToEvaluateOnNewDocument(performanceObserverScript).Do(ctx)
			return scriptErr
		}),
		chromedp.Navigate(target),
		chromedp.WaitReady("body", chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		chromedp.Evaluate(performanceEvaluationScript, &metrics),
	)
	if err != nil {
		return PerformancePageReport{}, err
	}
	totals := collector.snapshot()
	return PerformancePageReport{
		URL:                            target,
		Profile:                        performanceProfile,
		FCPMilliseconds:                roundMetric(metrics.FCPMilliseconds, 1),
		LCPMilliseconds:                roundMetric(metrics.LCPMilliseconds, 1),
		CLS:                            roundMetric(metrics.CLS, 4),
		TBTMilliseconds:                roundMetric(metrics.TBTMilliseconds, 1),
		TTFBMilliseconds:               roundMetric(metrics.TTFBMilliseconds, 1),
		DOMContentLoadedMilliseconds:   roundMetric(metrics.DOMContentLoadedMilliseconds, 1),
		LoadMilliseconds:               roundMetric(metrics.LoadMilliseconds, 1),
		Requests:                       totals.Requests,
		TransferBytes:                  totals.TransferBytes,
		JavaScriptBytes:                totals.JavaScriptBytes,
		CSSBytes:                       totals.CSSBytes,
		ImageBytes:                     totals.ImageBytes,
		ThirdPartyRequests:             totals.ThirdPartyRequests,
		DOMNodes:                       metrics.DOMNodes,
		ImagesMissingDimensions:        metrics.ImagesMissingDimensions,
		OffscreenImagesWithoutLazyLoad: metrics.OffscreenImagesWithoutLazyLoad,
		Duration:                       time.Since(started).Milliseconds(),
	}, nil
}

func (collector *performanceResourceCollector) handle(event any) {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	switch typed := event.(type) {
	case *network.EventResponseReceived:
		collector.resources[typed.RequestID] = performanceResource{
			URL:  typed.Response.URL,
			Type: typed.Type,
		}
	case *network.EventLoadingFinished:
		resource, found := collector.resources[typed.RequestID]
		if !found {
			return
		}
		size := int64(math.Round(typed.EncodedDataLength))
		collector.totals.Requests++
		collector.totals.TransferBytes += size
		switch resource.Type {
		case network.ResourceTypeScript:
			collector.totals.JavaScriptBytes += size
		case network.ResourceTypeStylesheet:
			collector.totals.CSSBytes += size
		case network.ResourceTypeImage:
			collector.totals.ImageBytes += size
		}
		parsed, err := url.Parse(resource.URL)
		if err == nil && !sameHost(collector.origin, parsed) {
			collector.totals.ThirdPartyRequests++
		}
		delete(collector.resources, typed.RequestID)
	}
}

func (collector *performanceResourceCollector) snapshot() performanceResourceTotals {
	collector.mu.Lock()
	defer collector.mu.Unlock()
	return collector.totals
}

func selectPerformancePages(pages []PageReport, limit int) []string {
	if limit <= 0 {
		return nil
	}
	sections := map[string]*performanceSection{}
	root := ""
	for _, candidate := range pages {
		if !candidate.Indexable || candidate.StatusCode < 200 || candidate.StatusCode >= 300 || !isHTMLContent(candidate.ContentType) {
			continue
		}
		parsed, err := url.Parse(candidate.FinalURL)
		if err != nil {
			continue
		}
		parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
		if len(parts) == 1 && parts[0] == "" {
			root = candidate.FinalURL
			continue
		}
		sectionName := parts[0]
		section := sections[sectionName]
		candidateDepth := candidate.Depth
		if candidateDepth < 0 {
			candidateDepth = math.MaxInt
		}
		if section == nil {
			section = &performanceSection{Name: sectionName, URL: candidate.FinalURL, Depth: candidateDepth}
			sections[sectionName] = section
		}
		section.Count++
		if candidateDepth < section.Depth || candidateDepth == section.Depth && candidate.FinalURL < section.URL {
			section.URL = candidate.FinalURL
			section.Depth = candidateDepth
		}
	}
	ordered := make([]performanceSection, 0, len(sections))
	for _, section := range sections {
		ordered = append(ordered, *section)
	}
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].Count != ordered[right].Count {
			return ordered[left].Count > ordered[right].Count
		}
		return ordered[left].Name < ordered[right].Name
	})
	selected := make([]string, 0, limit)
	if root != "" {
		selected = append(selected, root)
	}
	for _, section := range ordered {
		if len(selected) >= limit {
			break
		}
		selected = append(selected, section.URL)
	}
	return selected
}

func performanceFindings(page PerformancePageReport) []Finding {
	var findings []Finding
	add := func(check, priority, evidence, fix string) {
		findings = append(findings, newFinding("performance", check, Warn, priority, page.URL, evidence, fix))
	}
	if page.LCPMilliseconds > 2500 {
		priority := "medium"
		if page.LCPMilliseconds > 4000 {
			priority = "high"
		}
		add("Slow lab LCP", priority, fmt.Sprintf("%.0f ms under %s", page.LCPMilliseconds, page.Profile), "Reduce server delay, render-blocking work, and time needed to load and paint the largest visible element.")
	}
	if page.CLS > 0.1 {
		priority := "medium"
		if page.CLS > 0.25 {
			priority = "high"
		}
		add("High lab CLS", priority, fmt.Sprintf("%.3f under %s", page.CLS, page.Profile), "Reserve space for images and dynamic elements, and avoid inserting content above existing content.")
	}
	if page.TBTMilliseconds > 200 {
		priority := "medium"
		if page.TBTMilliseconds > 600 {
			priority = "high"
		}
		add("High lab TBT", priority, fmt.Sprintf("%.0f ms under %s", page.TBTMilliseconds, page.Profile), "Reduce JavaScript execution and split long main-thread tasks.")
	}
	if page.TTFBMilliseconds > 800 {
		add("Slow lab TTFB", "medium", fmt.Sprintf("%.0f ms under %s", page.TTFBMilliseconds, page.Profile), "Improve server response time, caching, redirects, and edge delivery.")
	}
	if page.TransferBytes > 2500*1024 {
		add("Heavy page transfer", "medium", fmt.Sprintf("%.1f MB across %d requests", float64(page.TransferBytes)/(1024*1024), page.Requests), "Compress and remove unnecessary scripts, styles, images, fonts, and third-party resources.")
	}
	if page.JavaScriptBytes > 500*1024 {
		add("Large JavaScript transfer", "low", fmt.Sprintf("%.0f KB", float64(page.JavaScriptBytes)/1024), "Remove unused JavaScript, split bundles, and defer code that is not needed for initial rendering.")
	}
	if page.Requests > 100 {
		add("Excessive page requests", "low", fmt.Sprintf("%d requests", page.Requests), "Remove unnecessary resources and consolidate requests where it improves loading.")
	}
	if page.DOMNodes > 1500 {
		add("Large rendered DOM", "low", fmt.Sprintf("%d elements", page.DOMNodes), "Simplify deeply nested or repeated markup and render nonessential interface elements only when needed.")
	}
	if page.ImagesMissingDimensions > 0 {
		add("Rendered images missing dimensions", "low", countLabel(page.ImagesMissingDimensions, "image"), "Set explicit width and height or a stable aspect ratio so image loading does not shift content.")
	}
	if page.OffscreenImagesWithoutLazyLoad > 0 {
		add("Offscreen images not lazy-loaded", "low", countLabel(page.OffscreenImagesWithoutLazyLoad, "image"), "Lazy-load below-the-fold images while loading the likely LCP image eagerly.")
	}
	return findings
}

func summarizePerformance(report PerformanceReport) PerformanceSummary {
	summary := PerformanceSummary{
		Pages:  len(report.Pages),
		Errors: len(report.Errors),
	}
	for _, page := range report.Pages {
		summary.WorstLCP = math.Max(summary.WorstLCP, page.LCPMilliseconds)
		summary.WorstCLS = math.Max(summary.WorstCLS, page.CLS)
		summary.WorstTBT = math.Max(summary.WorstTBT, page.TBTMilliseconds)
		summary.WorstTTFB = math.Max(summary.WorstTTFB, page.TTFBMilliseconds)
		if page.TransferBytes > summary.MaxTransfer {
			summary.MaxTransfer = page.TransferBytes
		}
	}
	return summary
}

func roundMetric(value float64, places int) float64 {
	scale := math.Pow10(places)
	return math.Round(value*scale) / scale
}

func countLabel(count int, singular string) string {
	label := singular
	if count != 1 {
		label += "s"
	}
	return fmt.Sprintf("%d %s", count, label)
}

func emitProgress(options Options, format string, values ...any) {
	if options.Progress == nil {
		return
	}
	options.Progress(report.ProgressEvent{
		Stage:   "performance",
		Message: fmt.Sprintf(format, values...),
	})
}

func newFinding(category, check string, status report.Status, priority, target, evidence, fix string) Finding {
	return Finding{
		Category: category,
		Check:    check,
		Status:   status,
		Priority: priority,
		URL:      target,
		Evidence: evidence,
		Fix:      fix,
	}
}

func isHTMLContent(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "html")
}

func sameHost(left, right *url.URL) bool {
	return strings.TrimPrefix(strings.ToLower(left.Hostname()), "www.") == strings.TrimPrefix(strings.ToLower(right.Hostname()), "www.")
}

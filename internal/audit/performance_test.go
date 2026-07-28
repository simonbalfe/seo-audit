package audit

import (
	"net/http"
	"testing"
)

func TestSelectPerformancePagesUsesRootAndLargestSections(t *testing.T) {
	pages := []PageReport{
		performanceTestPage("https://example.com/", 0),
		performanceTestPage("https://example.com/blog/", 1),
		performanceTestPage("https://example.com/blog/one/", 2),
		performanceTestPage("https://example.com/blog/two/", 2),
		performanceTestPage("https://example.com/product/", 1),
		performanceTestPage("https://example.com/product/feature/", 2),
		performanceTestPage("https://example.com/about/", 1),
	}

	selected := selectPerformancePages(pages, 3)
	expected := []string{
		"https://example.com/",
		"https://example.com/blog/",
		"https://example.com/product/",
	}
	if len(selected) != len(expected) {
		t.Fatalf("expected %d pages, got %#v", len(expected), selected)
	}
	for index := range expected {
		if selected[index] != expected[index] {
			t.Fatalf("expected page %d to be %s, got %s", index, expected[index], selected[index])
		}
	}
}

func TestSelectPerformancePagesSkipsNonIndexableAndNonHTML(t *testing.T) {
	noindex := performanceTestPage("https://example.com/private/", 1)
	noindex.Indexable = false
	asset := performanceTestPage("https://example.com/file.pdf", 1)
	asset.ContentType = "application/pdf"

	selected := selectPerformancePages([]PageReport{
		performanceTestPage("https://example.com/", 0),
		noindex,
		asset,
	}, 6)
	if len(selected) != 1 || selected[0] != "https://example.com/" {
		t.Fatalf("expected only the indexable HTML root, got %#v", selected)
	}
}

func TestPerformanceFindingsReportMeasuredProblems(t *testing.T) {
	report := PerformancePageReport{
		URL:                            "https://example.com/",
		Profile:                        performanceProfile,
		LCPMilliseconds:                4200,
		CLS:                            0.3,
		TBTMilliseconds:                700,
		TTFBMilliseconds:               900,
		Requests:                       120,
		TransferBytes:                  3 * 1024 * 1024,
		JavaScriptBytes:                700 * 1024,
		DOMNodes:                       1800,
		ImagesMissingDimensions:        2,
		OffscreenImagesWithoutLazyLoad: 3,
	}

	findings := performanceFindings(report)
	for _, check := range []string{
		"Slow lab LCP",
		"High lab CLS",
		"High lab TBT",
		"Slow lab TTFB",
		"Heavy page transfer",
		"Large JavaScript transfer",
		"Excessive page requests",
		"Large rendered DOM",
		"Rendered images missing dimensions",
		"Offscreen images not lazy-loaded",
	} {
		assertFinding(t, findings, check)
	}
}

func performanceTestPage(target string, depth int) PageReport {
	return PageReport{
		URL:         target,
		FinalURL:    target,
		StatusCode:  http.StatusOK,
		ContentType: "text/html",
		Depth:       depth,
		Indexable:   true,
	}
}

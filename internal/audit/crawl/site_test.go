package crawl

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAuditFindsCrossPageIssues(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/robots.txt":
			_, _ = fmt.Fprintf(writer, "User-agent: *\nAllow: /\nSitemap: %s/sitemap.xml", server.URL)
		case "/sitemap.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprintf(writer, `<urlset><url><loc>%s/</loc></url><url><loc>%s/orphan</loc></url><url><loc>%s/noindex</loc></url></urlset>`, server.URL, server.URL, server.URL)
		case "/":
			writeHTML(writer, "Home", `<h1>Home</h1><a href="/about">About</a><a href="/missing">Missing</a><a href="/missing">Missing again</a><a href="/cdn-cgi/l/email-protection#test">Email</a>`)
		case "/about":
			writeHTML(writer, "About", `<h1>About</h1>`)
		case "/orphan":
			writeHTML(writer, "Orphan", `<h1>Orphan</h1>`)
		case "/noindex":
			_, _ = writer.Write([]byte(`<html lang="en"><head><title>Hidden page</title><meta name="robots" content="noindex"><meta name="viewport" content="width=device-width"><link rel="canonical" href="` + server.URL + `/noindex"></head><body><main><h1>Hidden</h1></main></body></html>`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client := NewClient(2 * time.Second)
	client.Render = false
	report, err := client.Audit(context.Background(), server.URL, Options{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Pages != 5 {
		t.Fatalf("expected 5 pages, got %d", report.Summary.Pages)
	}
	assertFinding(t, report.Findings, "Broken internal link")
	if count := countFindings(report.Findings, "Broken internal link"); count != 1 {
		t.Fatalf("broken internal link findings = %d, want 1", count)
	}
	assertFinding(t, report.Findings, "Sitemap-only page")
	assertFinding(t, report.Findings, "Non-indexable URL in sitemap")
	if report.LimitReached {
		t.Fatal("did not expect crawl limit to be reached")
	}
}

func TestAuditFollowsApexToWWWRedirectWithoutDuplicateHomepage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/robots.txt":
			_, _ = writer.Write([]byte("User-agent: *\nAllow: /"))
		case "/sitemap.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = writer.Write([]byte("<urlset><url><loc>http://www.example.test/</loc></url></urlset>"))
		case "/":
			if request.Host == "example.test" {
				http.Redirect(writer, request, "http://www.example.test/", http.StatusMovedPermanently)
				return
			}
			writeHTML(writer, "Home", `<h1>Home</h1><a href="/">Home</a><a href="/about">About</a>`)
		case "/about":
			writeHTML(writer, "About", "<h1>About</h1>")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
		},
	}
	defer transport.CloseIdleConnections()
	client := NewClient(2 * time.Second)
	client.HTTP.Transport = transport
	client.Render = false
	report, err := client.Audit(t.Context(), "http://example.test", Options{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Pages != 2 {
		t.Fatalf("pages = %d, want 2", report.Summary.Pages)
	}
	if report.Pages[0].FinalURL != "http://www.example.test/" {
		t.Fatalf("homepage final URL = %q, want www host", report.Pages[0].FinalURL)
	}
	if count := countFindings(report.Findings, "Redirecting URL in sitemap"); count != 0 {
		t.Fatalf("redirecting sitemap findings = %d, want 0", count)
	}
}

func TestAuditReturnsErrorWhenStartURLCannotBeFetched(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {}))
	target := server.URL
	server.Close()

	client := NewClient(500 * time.Millisecond)
	client.Render = false
	if _, err := client.Audit(context.Background(), target, Options{Limit: 5}); err == nil {
		t.Fatal("expected an error for an unreachable start URL")
	}
}

func TestAuditReportsProgress(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/robots.txt":
			_, _ = writer.Write([]byte("User-agent: *\nAllow: /"))
		case "/sitemap.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = writer.Write([]byte("<urlset></urlset>"))
		case "/":
			writeHTML(writer, "Progress test page", "<h1>Progress test page</h1>")
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	var events []ProgressEvent
	client := NewClient(2 * time.Second)
	client.Render = false
	_, err := client.Audit(context.Background(), server.URL, Options{
		Limit: 5,
		Progress: func(event ProgressEvent) {
			events = append(events, event)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, stage := range []string{"setup", "robots", "sitemaps", "crawl", "analysis", "resources"} {
		if !hasProgressStage(events, stage) {
			t.Fatalf("missing progress stage %q in %#v", stage, events)
		}
	}
}

func TestAuditFetchesPagesConcurrently(t *testing.T) {
	var active atomic.Int32
	var peak atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/robots.txt":
			_, _ = writer.Write([]byte("User-agent: *\nAllow: /"))
		case "/sitemap.xml":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = writer.Write([]byte("<urlset></urlset>"))
		case "/":
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(`<html><body><main><h1>Home</h1>`))
			for index := range pageWorkerCount {
				_, _ = fmt.Fprintf(writer, `<a href="/page-%d">Page</a>`, index)
			}
			_, _ = writer.Write([]byte(`</main></body></html>`))
		default:
			current := active.Add(1)
			for {
				previous := peak.Load()
				if current <= previous || peak.CompareAndSwap(previous, current) {
					break
				}
			}
			defer active.Add(-1)
			time.Sleep(50 * time.Millisecond)
			writeHTML(writer, "Concurrent page", "<h1>Concurrent page</h1>")
		}
	}))
	defer server.Close()

	client := NewClient(2 * time.Second)
	client.Render = false
	report, err := client.Audit(t.Context(), server.URL, Options{Limit: pageWorkerCount + 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Pages != pageWorkerCount+1 {
		t.Fatalf("pages = %d, want %d", report.Summary.Pages, pageWorkerCount+1)
	}
	if peak.Load() < 2 {
		t.Fatalf("peak concurrent page requests = %d, want at least 2", peak.Load())
	}
}

func TestAnalyzeURLIgnoresNonHTMLAssets(t *testing.T) {
	report := SiteReport{}
	analyzeURL(&report, PageReport{
		URL:         "https://example.com/skills/example/SKILL.md",
		ContentType: "text/markdown",
	})
	if len(report.Findings) != 0 {
		t.Fatalf("expected no URL findings for non-HTML asset, got %#v", report.Findings)
	}
}

func hasProgressStage(events []ProgressEvent, stage string) bool {
	for _, event := range events {
		if event.Stage == stage {
			return true
		}
	}
	return false
}

func writeHTML(writer http.ResponseWriter, title, body string) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(writer, `<html lang="en"><head><title>%s</title><meta name="description" content="A sufficiently descriptive summary for this useful page and its purpose."><meta name="viewport" content="width=device-width"><link rel="canonical" href=""></head><body><main>%s</main></body></html>`, title, body)
}

func assertFinding(t *testing.T, findings []Finding, check string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Check == check {
			return
		}
	}
	t.Fatalf("missing finding %q", check)
}

func countFindings(findings []Finding, check string) int {
	count := 0
	for _, finding := range findings {
		if finding.Check == check {
			count++
		}
	}
	return count
}

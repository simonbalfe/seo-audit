package audit

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
			writeHTML(writer, "Home", `<h1>Home</h1><a href="/about">About</a><a href="/missing">Missing</a>`)
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
	assertFinding(t, report.Findings, "Sitemap-only page")
	assertFinding(t, report.Findings, "Non-indexable URL in sitemap")
	if report.LimitReached {
		t.Fatal("did not expect crawl limit to be reached")
	}
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

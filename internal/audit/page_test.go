package audit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInspectPageExtractsPublicSignals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(`<!doctype html>
<html lang="en">
<head>
<title>Useful page</title>
<meta name="description" content="A useful description">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="canonical" href="/page">
<meta name="author" content="Sam Writer">
<meta property="article:published_time" content="2026-07-28">
<script type="application/ld+json">{"@context":"https://schema.org","@type":"BlogPosting"}</script>
</head>
<body><main><article><h1>Useful heading</h1><p>This useful paragraph explains the page clearly for readers.</p><a href="/next">Next</a><img src="/image.jpg" alt=""></article></main></body>
</html>`))
	}))
	defer server.Close()

	client := NewClient(2 * time.Second)
	client.Render = false
	report, err := client.InspectPage(context.Background(), server.URL+"/page")
	if err != nil {
		t.Fatal(err)
	}
	if report.Title != "Useful page" {
		t.Fatalf("unexpected title %q", report.Title)
	}
	if len(report.H1) != 1 || report.H1[0] != "Useful heading" {
		t.Fatalf("unexpected H1 values %#v", report.H1)
	}
	if report.Canonical != server.URL+"/page" {
		t.Fatalf("unexpected canonical %q", report.Canonical)
	}
	if report.StructuredData != 1 || report.InvalidStructured != 0 {
		t.Fatalf("unexpected structured data counts %d/%d", report.StructuredData, report.InvalidStructured)
	}
	if !report.HasArticle || report.Author != "Sam Writer" || report.PublishedDate != "2026-07-28" {
		t.Fatalf("unexpected article signals %#v", report)
	}
	if report.ParagraphCount != 1 || report.FirstParagraph == "" {
		t.Fatalf("unexpected paragraph signals %#v", report)
	}
	if len(report.InternalLinks) != 1 || report.InternalLinks[0] != server.URL+"/next" {
		t.Fatalf("unexpected links %#v", report.InternalLinks)
	}
}

func TestPageFindingsReportsContentReviewSignals(t *testing.T) {
	page := PageReport{
		URL:              "https://example.com/blog/guide",
		FinalURL:         "https://example.com/blog/guide",
		StatusCode:       http.StatusOK,
		ContentType:      "text/html",
		Indexable:        true,
		Title:            "Technical SEO checklist",
		Titles:           []string{"Technical SEO checklist"},
		Description:      "A sufficiently descriptive summary for a useful guide and its purpose.",
		Descriptions:     []string{"A sufficiently descriptive summary for a useful guide and its purpose."},
		H1:               []string{"Content marketing guide"},
		Canonicals:       []string{"https://example.com/blog/guide"},
		Canonical:        "https://example.com/blog/guide",
		Language:         "en",
		HasViewport:      true,
		HasMain:          true,
		HasArticle:       true,
		WordCount:        800,
		LongestParagraph: 180,
	}

	findings := pageFindings(page)
	for _, check := range []string{
		"Long page has no subheadings",
		"Very long paragraph",
		"Article author not evident",
		"Article date not evident",
		"Long article has no external sources",
	} {
		assertFinding(t, findings, check)
	}
}

func TestInspectRobotsEvaluatesAgents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("User-agent: GPTBot\nDisallow: /\n\nUser-agent: *\nAllow: /\nSitemap: " + "https://example.com/sitemap.xml"))
	}))
	defer server.Close()

	client := NewClient(2 * time.Second)
	client.Render = false
	report, err := client.InspectRobots(context.Background(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	access := map[string]bool{}
	for _, item := range report.Agents {
		access[item.Agent] = item.Allowed
	}
	if access["GPTBot"] {
		t.Fatal("expected GPTBot to be blocked")
	}
	if !access["Googlebot"] {
		t.Fatal("expected Googlebot to be allowed")
	}
}

func TestPageFindingsKeepsIntentionalNoindexOutOfActions(t *testing.T) {
	page := PageReport{
		URL:         "https://example.com/private",
		FinalURL:    "https://example.com/private",
		StatusCode:  http.StatusOK,
		ContentType: "text/html",
		Title:       "Private reference page",
		Titles:      []string{"Private reference page"},
		Description: "A sufficiently descriptive summary for a deliberately excluded reference page.",
		Descriptions: []string{
			"A sufficiently descriptive summary for a deliberately excluded reference page.",
		},
		H1:          []string{"Private reference"},
		Canonicals:  []string{"https://example.com/private"},
		Canonical:   "https://example.com/private",
		Robots:      "noindex, follow",
		Language:    "en",
		HasViewport: true,
		HasMain:     true,
	}

	for _, finding := range pageFindings(page) {
		if finding.Check == "Noindex directive" {
			t.Fatal("intentional noindex should remain indexability evidence, not an action")
		}
	}
}

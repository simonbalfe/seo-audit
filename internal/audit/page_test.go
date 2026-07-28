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
<script type="application/ld+json">{"@context":"https://schema.org"}</script>
</head>
<body><main><h1>Useful heading</h1><a href="/next">Next</a><img src="/image.jpg" alt=""></main></body>
</html>`))
	}))
	defer server.Close()

	report, err := NewClient(2*time.Second).InspectPage(context.Background(), server.URL+"/page")
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
	if len(report.InternalLinks) != 1 || report.InternalLinks[0] != server.URL+"/next" {
		t.Fatalf("unexpected links %#v", report.InternalLinks)
	}
}

func TestInspectRobotsEvaluatesAgents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte("User-agent: GPTBot\nDisallow: /\n\nUser-agent: *\nAllow: /\nSitemap: " + "https://example.com/sitemap.xml"))
	}))
	defer server.Close()

	report, err := NewClient(2*time.Second).InspectRobots(context.Background(), server.URL)
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

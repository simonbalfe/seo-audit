package crawl

import (
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type sitemapDocument struct {
	URLs     []sitemapLocation `xml:"url"`
	Sitemaps []sitemapLocation `xml:"sitemap"`
}

type sitemapLocation struct {
	Location string `xml:"loc"`
}

func (c *Client) InspectSitemaps(ctx context.Context, rawURL string) (SitemapReport, error) {
	target, err := normalizeStartURL(rawURL)
	if err != nil {
		return SitemapReport{}, err
	}
	robots, _ := c.InspectRobots(ctx, target.String())
	candidates := append([]string{}, robots.Sitemaps...)
	if len(candidates) == 0 {
		candidates = append(candidates, (&url.URL{Scheme: target.Scheme, Host: target.Host, Path: "/sitemap.xml"}).String())
	}
	report := SitemapReport{}
	queue := candidates
	seen := map[string]bool{}
	for len(queue) > 0 && len(seen) < 25 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] {
			continue
		}
		seen[current] = true
		report.Sources = append(report.Sources, current)
		document, fetchErr := c.fetchSitemap(ctx, current)
		if fetchErr != nil {
			report.Errors = append(report.Errors, fetchErr.Error())
			continue
		}
		for _, item := range document.URLs {
			if strings.TrimSpace(item.Location) != "" {
				report.URLs = appendUnique(report.URLs, strings.TrimSpace(item.Location))
			}
		}
		for _, item := range document.Sitemaps {
			if strings.TrimSpace(item.Location) != "" {
				queue = append(queue, strings.TrimSpace(item.Location))
			}
		}
	}
	return report, nil
}

func (c *Client) fetchSitemap(ctx context.Context, target string) (sitemapDocument, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return sitemapDocument{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	response, err := c.HTTP.Do(req)
	if err != nil {
		return sitemapDocument{}, fmt.Errorf("%s: %w", target, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sitemapDocument{}, fmt.Errorf("%s returned %d", target, response.StatusCode)
	}
	var reader io.Reader = response.Body
	if strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "gzip") || strings.HasSuffix(strings.ToLower(target), ".gz") {
		compressed, gzipErr := gzip.NewReader(response.Body)
		if gzipErr != nil {
			return sitemapDocument{}, fmt.Errorf("%s: %w", target, gzipErr)
		}
		defer compressed.Close()
		reader = compressed
	}
	var document sitemapDocument
	if err := xml.NewDecoder(io.LimitReader(reader, 10<<20)).Decode(&document); err != nil {
		return sitemapDocument{}, fmt.Errorf("%s: %w", target, err)
	}
	return document, nil
}

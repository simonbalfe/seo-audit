package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

const maxPageBytes = 5 << 20

func (c *Client) InspectPage(ctx context.Context, rawURL string) (PageReport, error) {
	target, err := normalizeStartURL(rawURL)
	if err != nil {
		return PageReport{}, err
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return PageReport{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	response, err := c.HTTP.Do(req)
	if err != nil {
		return PageReport{}, err
	}
	defer response.Body.Close()

	report := PageReport{
		URL:         target.String(),
		FinalURL:    response.Request.URL.String(),
		StatusCode:  response.StatusCode,
		ContentType: response.Header.Get("Content-Type"),
		Duration:    time.Since(started).Milliseconds(),
	}
	if !strings.Contains(strings.ToLower(report.ContentType), "html") {
		report.Findings = append(report.Findings, Finding{
			Category: "technical",
			Check:    "HTML response",
			Status:   Warn,
			URL:      report.URL,
			Evidence: valueOr(report.ContentType, "content type was not supplied"),
			Fix:      "Audit HTML pages and keep non-HTML assets out of the page crawl.",
		})
		return report, nil
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxPageBytes+1))
	if err != nil {
		return PageReport{}, err
	}
	if len(body) > maxPageBytes {
		return PageReport{}, fmt.Errorf("page exceeds %d bytes", maxPageBytes)
	}
	document, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return PageReport{}, err
	}
	extractPage(document, response.Request.URL, &report)
	report.Findings = pageFindings(report)
	return report, nil
}

func extractPage(document *html.Node, base *url.URL, report *PageReport) {
	var text strings.Builder
	var walk func(*html.Node, bool)
	walk = func(node *html.Node, hidden bool) {
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			hidden = hidden || tag == "script" || tag == "style" || tag == "noscript" || tag == "svg"
			switch tag {
			case "html":
				report.Language = attribute(node, "lang")
			case "title":
				report.Title = strings.TrimSpace(nodeText(node))
			case "h1":
				value := strings.TrimSpace(nodeText(node))
				if value != "" {
					report.H1 = append(report.H1, value)
				}
			case "main":
				report.HasMain = true
			case "meta":
				name := strings.ToLower(attribute(node, "name"))
				switch name {
				case "description":
					report.Description = strings.TrimSpace(attribute(node, "content"))
				case "robots":
					report.Robots = strings.TrimSpace(attribute(node, "content"))
				case "viewport":
					report.HasViewport = true
				}
			case "link":
				if hasToken(attribute(node, "rel"), "canonical") {
					report.Canonical = resolveURL(base, attribute(node, "href"))
				}
			case "a":
				link := resolveURL(base, attribute(node, "href"))
				if link != "" {
					parsed, err := url.Parse(link)
					if err == nil && sameHost(base, parsed) {
						report.InternalLinks = appendUnique(report.InternalLinks, normalizeCrawlURL(parsed))
					} else if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") {
						report.ExternalLinks = appendUnique(report.ExternalLinks, link)
					}
				}
			case "img":
				report.ImageCount++
				if _, exists := attributeValue(node, "alt"); !exists {
					report.ImagesMissingAlt++
				}
			case "script":
				if strings.EqualFold(attribute(node, "type"), "application/ld+json") {
					report.StructuredData++
					var value any
					if json.Unmarshal([]byte(nodeText(node)), &value) != nil {
						report.InvalidStructured++
					}
				}
			}
		}
		if node.Type == html.TextNode && !hidden {
			value := strings.TrimSpace(node.Data)
			if value != "" {
				text.WriteString(value)
				text.WriteByte(' ')
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child, hidden)
		}
	}
	walk(document, false)
	report.WordCount = countWords(text.String())
}

func pageFindings(page PageReport) []Finding {
	findings := make([]Finding, 0)
	add := func(category, check string, status Status, evidence, fix string) {
		findings = append(findings, Finding{Category: category, Check: check, Status: status, URL: page.URL, Evidence: evidence, Fix: fix})
	}
	if page.StatusCode >= 200 && page.StatusCode < 300 {
		add("technical", "HTTP status", Pass, fmt.Sprintf("%d", page.StatusCode), "")
	} else {
		add("technical", "HTTP status", Fail, fmt.Sprintf("%d", page.StatusCode), "Return a successful response for indexable pages.")
	}
	if strings.HasPrefix(page.FinalURL, "https://") {
		add("technical", "HTTPS", Pass, page.FinalURL, "")
	} else {
		add("technical", "HTTPS", Fail, page.FinalURL, "Serve the page over HTTPS and redirect HTTP URLs.")
	}
	if page.Title == "" {
		add("on-page", "Page title", Fail, "missing", "Add a unique title that describes the page.")
	} else {
		add("on-page", "Page title", Pass, page.Title, "")
	}
	if page.Description == "" {
		add("on-page", "Meta description", Warn, "missing", "Add a useful description that explains why someone should visit.")
	} else {
		add("on-page", "Meta description", Pass, page.Description, "")
	}
	switch len(page.H1) {
	case 0:
		add("on-page", "Main heading", Fail, "no H1 found", "Add one clear main heading.")
	case 1:
		add("on-page", "Main heading", Pass, page.H1[0], "")
	default:
		add("on-page", "Main heading", Warn, fmt.Sprintf("%d H1 headings", len(page.H1)), "Use one clear main heading unless multiple H1 elements are genuinely required.")
	}
	if page.Canonical == "" {
		add("indexing", "Canonical", Warn, "missing", "Add a self-referencing canonical to important standalone pages.")
	} else {
		add("indexing", "Canonical", Pass, page.Canonical, "")
	}
	if strings.Contains(strings.ToLower(page.Robots), "noindex") {
		add("indexing", "Index directive", Fail, page.Robots, "Remove noindex if this page should appear in search.")
	} else {
		add("indexing", "Index directive", Pass, valueOr(page.Robots, "no noindex directive"), "")
	}
	if page.Language == "" {
		add("accessibility", "Page language", Warn, "missing", "Set the html lang attribute.")
	} else {
		add("accessibility", "Page language", Pass, page.Language, "")
	}
	if page.HasViewport {
		add("mobile", "Viewport", Pass, "present", "")
	} else {
		add("mobile", "Viewport", Fail, "missing", "Add a responsive viewport meta tag.")
	}
	if page.ImagesMissingAlt > 0 {
		add("content", "Image alternatives", Warn, fmt.Sprintf("%d of %d images have no alt attribute", page.ImagesMissingAlt, page.ImageCount), "Add descriptive alt text or an empty alt attribute to decorative images.")
	} else {
		add("content", "Image alternatives", Pass, fmt.Sprintf("%d images checked", page.ImageCount), "")
	}
	if page.InvalidStructured > 0 {
		add("schema", "Structured data syntax", Fail, fmt.Sprintf("%d invalid JSON-LD blocks", page.InvalidStructured), "Fix invalid JSON-LD before checking eligibility.")
	} else if page.StructuredData > 0 {
		add("schema", "Structured data syntax", Pass, fmt.Sprintf("%d valid JSON-LD blocks", page.StructuredData), "")
	} else {
		add("schema", "Structured data presence", Warn, "none found in rendered HTML", "Add relevant structured data when it accurately represents visible content.")
	}
	if page.HasMain {
		add("geo", "Main content landmark", Pass, "main element present", "")
	} else {
		add("geo", "Main content landmark", Warn, "main element missing", "Wrap the primary content in a semantic main element.")
	}
	return findings
}

func normalizeStartURL(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("URL is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return nil, errors.New("valid public URL is required")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("URL must use HTTP or HTTPS")
	}
	parsed.Fragment = ""
	return parsed, nil
}

func normalizeCrawlURL(value *url.URL) string {
	copy := *value
	copy.Fragment = ""
	copy.RawQuery = ""
	if copy.Path == "" {
		copy.Path = "/"
	}
	return copy.String()
}

func resolveURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "tel:") || strings.HasPrefix(raw, "javascript:") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return base.ResolveReference(parsed).String()
}

func sameHost(left, right *url.URL) bool {
	return strings.EqualFold(left.Hostname(), right.Hostname())
}

func attribute(node *html.Node, key string) string {
	value, _ := attributeValue(node, key)
	return value
}

func attributeValue(node *html.Node, key string) (string, bool) {
	for _, item := range node.Attr {
		if strings.EqualFold(item.Key, key) {
			return item.Val, true
		}
	}
	return "", false
}

func hasToken(value, token string) bool {
	for _, part := range strings.Fields(strings.ToLower(value)) {
		if part == token {
			return true
		}
	}
	return false
}

func nodeText(node *html.Node) string {
	var result strings.Builder
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.TextNode {
			result.WriteString(current.Data)
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return result.String()
}

func countWords(value string) int {
	return len(strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}))
}

func appendUnique(values []string, candidate string) []string {
	if candidate == "" {
		return values
	}
	for _, value := range values {
		if value == candidate {
			return values
		}
	}
	return append(values, candidate)
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

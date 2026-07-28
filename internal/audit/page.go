package audit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/net/html"
)

const maxPageBytes = 5 << 20

var whitespacePattern = regexp.MustCompile(`\s+`)

func (c *Client) InspectPage(ctx context.Context, rawURL string) (PageReport, error) {
	target, err := normalizeStartURL(rawURL)
	if err != nil {
		return PageReport{}, err
	}
	redirects := []string{}
	httpClient := *c.HTTP
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		redirects = append(redirects, req.URL.String())
		if len(via) >= 10 {
			return http.ErrUseLastResponse
		}
		return nil
	}
	started := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return PageReport{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	response, err := httpClient.Do(req)
	if err != nil {
		return PageReport{}, err
	}
	defer response.Body.Close()

	report := PageReport{
		URL:           normalizeCrawlURL(target),
		FinalURL:      normalizeCrawlURL(response.Request.URL),
		RedirectChain: redirects,
		StatusCode:    response.StatusCode,
		ContentType:   response.Header.Get("Content-Type"),
		XRobots:       response.Header.Get("X-Robots-Tag"),
		Duration:      time.Since(started).Milliseconds(),
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxPageBytes+1))
	if err != nil {
		return PageReport{}, err
	}
	report.SizeBytes = len(body)
	if len(body) > maxPageBytes {
		return PageReport{}, fmt.Errorf("page exceeds %d bytes", maxPageBytes)
	}
	if !strings.Contains(strings.ToLower(report.ContentType), "html") {
		report.Indexable = response.StatusCode >= 200 && response.StatusCode < 300 && !containsDirective(report.XRobots, "noindex")
		if !report.Indexable {
			report.Indexability = "non-HTML response is not indexable"
		}
		report.Findings = pageFindings(report)
		return report, nil
	}
	document, err := html.Parse(strings.NewReader(string(body)))
	if err != nil {
		return PageReport{}, err
	}
	extractPage(document, response.Request.URL, &report)
	if c.Render && report.WordCount < 50 {
		if renderedHTML, renderErr := renderHTML(ctx, report.FinalURL); renderErr == nil {
			renderedDocument, parseErr := html.Parse(strings.NewReader(renderedHTML))
			if parseErr == nil {
				renderedReport := PageReport{
					URL:           report.URL,
					FinalURL:      report.FinalURL,
					RedirectChain: report.RedirectChain,
					StatusCode:    report.StatusCode,
					ContentType:   report.ContentType,
					SizeBytes:     report.SizeBytes,
					XRobots:       report.XRobots,
					Duration:      report.Duration,
					Rendered:      true,
				}
				extractPage(renderedDocument, response.Request.URL, &renderedReport)
				report = renderedReport
			}
		}
	}
	for _, canonical := range httpCanonicals(response.Header.Values("Link"), response.Request.URL) {
		report.Canonicals = appendUnique(report.Canonicals, canonical)
	}
	if len(report.Canonicals) > 0 {
		report.Canonical = report.Canonicals[0]
	}
	report.Indexable, report.Indexability = pageIndexability(report)
	report.Findings = pageFindings(report)
	return report, nil
}

func extractPage(document *html.Node, base *url.URL, report *PageReport) {
	var text strings.Builder
	var walk func(*html.Node, bool)
	walk = func(node *html.Node, hidden bool) {
		if node.Type == html.ElementNode {
			tag := strings.ToLower(node.Data)
			hidden = hidden || tag == "script" || tag == "style" || tag == "noscript" || tag == "svg" || tag == "template"
			switch tag {
			case "html":
				report.Language = strings.TrimSpace(attribute(node, "lang"))
			case "head":
			case "body":
			case "title":
				value := cleanText(nodeText(node))
				report.Titles = append(report.Titles, value)
				if report.Title == "" {
					report.Title = value
				}
			case "h1", "h2", "h3", "h4", "h5", "h6":
				level := int(tag[1] - '0')
				report.HeadingLevels = append(report.HeadingLevels, level)
				value := cleanText(nodeText(node))
				if tag == "h1" && value != "" {
					report.H1 = append(report.H1, value)
				}
				if tag == "h2" && value != "" {
					report.H2 = append(report.H2, value)
				}
			case "main":
				report.HasMain = true
			case "meta":
				extractMeta(node, report)
			case "link":
				extractLinkElement(node, base, report)
			case "a":
				extractAnchor(node, base, report)
			case "img":
				extractImage(node, base, report)
			case "script":
				if source := resolveURL(base, attribute(node, "src")); source != "" {
					report.Resources = appendUnique(report.Resources, source)
				}
				if strings.EqualFold(attribute(node, "type"), "application/ld+json") {
					extractJSONLD(nodeText(node), report)
				}
			case "source":
				if source := resolveURL(base, attribute(node, "src")); source != "" {
					report.Resources = appendUnique(report.Resources, source)
				}
			case "iframe", "video", "audio":
				if source := resolveURL(base, attribute(node, "src")); source != "" {
					report.Resources = appendUnique(report.Resources, source)
				}
			}
		}
		if node.Type == html.TextNode && !hidden {
			value := cleanText(node.Data)
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
	normalized := strings.ToLower(cleanText(text.String()))
	report.TextTokens = tokenize(normalized)
	report.WordCount = len(report.TextTokens)
	sum := sha256.Sum256([]byte(normalized))
	report.ContentHash = hex.EncodeToString(sum[:])
}

func extractMeta(node *html.Node, report *PageReport) {
	name := strings.ToLower(strings.TrimSpace(attribute(node, "name")))
	property := strings.ToLower(strings.TrimSpace(attribute(node, "property")))
	content := cleanText(attribute(node, "content"))
	switch name {
	case "description":
		report.Descriptions = append(report.Descriptions, content)
		if report.Description == "" {
			report.Description = content
		}
	case "robots":
		report.Robots = content
	case "viewport":
		report.HasViewport = true
	case "twitter:card":
		report.TwitterCard = content
	}
	if property == "og:title" {
		report.OpenGraphTitle = content
	}
	if property == "og:description" {
		report.OpenGraphDesc = content
	}
	if strings.EqualFold(attribute(node, "charset"), "utf-8") || attribute(node, "charset") != "" {
		report.HasCharset = true
	}
	if strings.EqualFold(attribute(node, "http-equiv"), "content-type") {
		report.HasCharset = true
	}
}

func extractLinkElement(node *html.Node, base *url.URL, report *PageReport) {
	rel := strings.ToLower(attribute(node, "rel"))
	href := resolveURL(base, attribute(node, "href"))
	if href == "" {
		return
	}
	if hasToken(rel, "canonical") {
		report.Canonicals = appendUnique(report.Canonicals, href)
	}
	if hasToken(rel, "alternate") && attribute(node, "hreflang") != "" {
		report.Hreflang = append(report.Hreflang, Alternate{Language: strings.ToLower(attribute(node, "hreflang")), URL: href})
	}
	if hasToken(rel, "stylesheet") || hasToken(rel, "preload") || hasToken(rel, "modulepreload") {
		report.Resources = appendUnique(report.Resources, href)
	}
}

func extractAnchor(node *html.Node, base *url.URL, report *PageReport) {
	resolved := resolveURL(base, attribute(node, "href"))
	if resolved == "" {
		return
	}
	parsed, err := url.Parse(resolved)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return
	}
	internal := sameHost(base, parsed)
	normalized := resolved
	if internal {
		normalized = normalizeCrawlURL(parsed)
		report.InternalLinks = appendUnique(report.InternalLinks, normalized)
	} else {
		report.ExternalLinks = appendUnique(report.ExternalLinks, normalized)
	}
	report.Links = append(report.Links, Link{
		URL:      normalized,
		Text:     anchorText(node),
		Internal: internal,
		NoFollow: hasToken(attribute(node, "rel"), "nofollow"),
	})
}

func extractImage(node *html.Node, base *url.URL, report *PageReport) {
	source := resolveURL(base, attribute(node, "src"))
	alt, hasAlt := attributeValue(node, "alt")
	image := Image{
		URL:    source,
		Alt:    cleanText(alt),
		HasAlt: hasAlt,
		Lazy:   strings.EqualFold(attribute(node, "loading"), "lazy"),
		Srcset: attribute(node, "srcset") != "",
		Width:  attribute(node, "width"),
		Height: attribute(node, "height"),
	}
	report.Images = append(report.Images, image)
	report.ImageCount++
	if !hasAlt {
		report.ImagesMissingAlt++
	} else if strings.TrimSpace(alt) == "" {
		report.ImagesEmptyAlt++
	}
	if source != "" {
		report.Resources = appendUnique(report.Resources, source)
	}
}

func extractJSONLD(raw string, report *PageReport) {
	report.StructuredData++
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		report.InvalidStructured++
		return
	}
	collectSchemaTypes(value, &report.SchemaTypes)
}

func collectSchemaTypes(value any, types *[]string) {
	switch item := value.(type) {
	case map[string]any:
		if rawType, exists := item["@type"]; exists {
			switch typed := rawType.(type) {
			case string:
				*types = appendUnique(*types, typed)
			case []any:
				for _, entry := range typed {
					if name, ok := entry.(string); ok {
						*types = appendUnique(*types, name)
					}
				}
			}
		}
		for _, child := range item {
			collectSchemaTypes(child, types)
		}
	case []any:
		for _, child := range item {
			collectSchemaTypes(child, types)
		}
	}
}

func pageIndexability(page PageReport) (bool, string) {
	if page.StatusCode < 200 || page.StatusCode >= 300 {
		return false, fmt.Sprintf("HTTP %d", page.StatusCode)
	}
	if !strings.Contains(strings.ToLower(page.ContentType), "html") {
		return false, "non-HTML response"
	}
	if containsDirective(page.Robots, "noindex") || containsDirective(page.XRobots, "noindex") {
		return false, "noindex directive"
	}
	if page.Canonical != "" && normalizeComparableURL(page.Canonical) != normalizeComparableURL(page.FinalURL) {
		return false, "canonicalised to another URL"
	}
	return true, ""
}

func pageFindings(page PageReport) []Finding {
	findings := make([]Finding, 0)
	add := func(category, check string, status Status, priority, evidence, fix string) {
		if status != Pass {
			findings = append(findings, Finding{Category: category, Check: check, Status: status, Priority: priority, URL: page.URL, Evidence: evidence, Fix: fix})
		}
	}
	if page.StatusCode < 200 || page.StatusCode >= 300 {
		add("response", "Non-success response", Fail, "high", fmt.Sprintf("HTTP %d", page.StatusCode), "Return a successful response or link directly to the final valid URL.")
	}
	if len(page.RedirectChain) > 1 {
		add("response", "Redirect chain", Warn, "medium", strings.Join(append([]string{page.URL}, page.RedirectChain...), " -> "), "Replace multi-hop redirects with one direct redirect.")
	}
	if strings.Contains(strings.ToLower(page.ContentType), "html") {
		if page.Title == "" {
			add("on-page", "Missing title", Fail, "high", "no title element", "Add one unique, descriptive title in the document head.")
		} else if len(page.Titles) > 1 {
			add("on-page", "Multiple titles", Warn, "medium", fmt.Sprintf("%d title elements", len(page.Titles)), "Keep one title element in the document head.")
		} else if len([]rune(page.Title)) > 60 {
			add("on-page", "Long title", Warn, "low", fmt.Sprintf("%d characters: %s", len([]rune(page.Title)), page.Title), "Shorten the title if its important wording may be truncated.")
		} else if len([]rune(page.Title)) < 20 {
			add("on-page", "Short title", Warn, "low", fmt.Sprintf("%d characters: %s", len([]rune(page.Title)), page.Title), "Make the title more descriptive without padding it.")
		}
		if page.Description == "" {
			add("on-page", "Missing meta description", Warn, "medium", "no meta description", "Add a useful summary for search-result snippets.")
		} else if len(page.Descriptions) > 1 {
			add("on-page", "Multiple meta descriptions", Warn, "medium", fmt.Sprintf("%d descriptions", len(page.Descriptions)), "Keep one meta description.")
		} else if len([]rune(page.Description)) > 160 {
			add("on-page", "Long meta description", Warn, "low", fmt.Sprintf("%d characters", len([]rune(page.Description))), "Put the most useful information first and remove unnecessary wording.")
		} else if len([]rune(page.Description)) < 70 {
			add("on-page", "Short meta description", Warn, "low", fmt.Sprintf("%d characters", len([]rune(page.Description))), "Use the available snippet to explain the page clearly.")
		}
		switch len(page.H1) {
		case 0:
			add("headings", "Missing H1", Fail, "high", "no H1 found", "Add one clear main heading.")
		case 1:
		default:
			add("headings", "Multiple H1s", Warn, "medium", fmt.Sprintf("%d H1 elements", len(page.H1)), "Use one clear main heading unless the document structure genuinely requires more.")
		}
		if skippedHeadingLevel(page.HeadingLevels) {
			add("headings", "Skipped heading level", Warn, "low", fmt.Sprintf("levels: %v", page.HeadingLevels), "Keep heading levels in a logical hierarchy.")
		}
		if len(page.Canonicals) == 0 {
			add("indexing", "Missing canonical", Warn, "medium", "no HTML or HTTP canonical", "Add one absolute canonical for important indexable pages.")
		} else if len(page.Canonicals) > 1 {
			add("indexing", "Multiple canonicals", Fail, "high", strings.Join(page.Canonicals, ", "), "Declare one consistent canonical.")
		}
		if containsDirective(page.Robots, "noindex") || containsDirective(page.XRobots, "noindex") {
			add("indexing", "Noindex directive", Warn, "high", strings.TrimSpace(page.Robots+" "+page.XRobots), "Confirm the page should be excluded; otherwise remove noindex.")
		}
		if containsDirective(page.Robots, "nofollow") || containsDirective(page.XRobots, "nofollow") {
			add("indexing", "Nofollow directive", Warn, "medium", strings.TrimSpace(page.Robots+" "+page.XRobots), "Remove nofollow if crawlers should follow links on this page.")
		}
		if !page.HasViewport {
			add("mobile", "Missing viewport", Fail, "high", "viewport meta tag missing", "Add a responsive viewport meta tag.")
		}
		if page.Language == "" {
			add("accessibility", "Missing page language", Warn, "low", "html lang missing", "Set the html lang attribute.")
		}
		if page.ImagesMissingAlt > 0 {
			add("images", "Images missing alt attribute", Warn, "medium", fmt.Sprintf("%d of %d images", page.ImagesMissingAlt, page.ImageCount), "Add descriptive alt text or an empty alt attribute for decorative images.")
		}
		if page.InvalidStructured > 0 {
			add("schema", "Invalid JSON-LD", Fail, "high", fmt.Sprintf("%d invalid blocks", page.InvalidStructured), "Fix JSON syntax, then validate rich-result eligibility separately.")
		}
		if !page.HasMain {
			add("semantics", "Missing main landmark", Warn, "low", "main element missing", "Wrap the primary content in a semantic main element.")
		}
		if page.WordCount < 50 && page.Indexable {
			add("content", "Very little visible text", Warn, "medium", fmt.Sprintf("%d words", page.WordCount), "Review whether the page provides enough unique value for its purpose.")
		}
		for _, resource := range append(append([]string{}, page.Resources...), page.ExternalLinks...) {
			if strings.HasPrefix(page.FinalURL, "https://") && strings.HasPrefix(resource, "http://") {
				add("security", "Mixed HTTP resource", Fail, "high", resource, "Load every resource and link target over HTTPS.")
				break
			}
		}
	}
	return findings
}

func skippedHeadingLevel(levels []int) bool {
	last := 0
	for _, level := range levels {
		if last > 0 && level > last+1 {
			return true
		}
		last = level
	}
	return false
}

func httpCanonicals(headers []string, base *url.URL) []string {
	var result []string
	for _, header := range headers {
		for _, part := range strings.Split(header, ",") {
			if !strings.Contains(strings.ToLower(part), `rel="canonical"`) && !strings.Contains(strings.ToLower(part), "rel=canonical") {
				continue
			}
			start := strings.Index(part, "<")
			end := strings.Index(part, ">")
			if start >= 0 && end > start {
				result = appendUnique(result, resolveURL(base, part[start+1:end]))
			}
		}
	}
	return result
}

func containsDirective(value, directive string) bool {
	for _, part := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	}) {
		if part == directive {
			return true
		}
	}
	return false
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

func normalizeComparableURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Fragment = ""
	parsed.RawQuery = ""
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}
	return parsed.String()
}

func resolveURL(base *url.URL, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "mailto:") || strings.HasPrefix(raw, "tel:") || strings.HasPrefix(raw, "javascript:") || strings.HasPrefix(raw, "data:") {
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

func anchorText(node *html.Node) string {
	if label := cleanText(attribute(node, "aria-label")); label != "" {
		return label
	}
	if text := cleanText(nodeText(node)); text != "" {
		return text
	}
	var alternatives []string
	var walk func(*html.Node)
	walk = func(current *html.Node) {
		if current.Type == html.ElementNode && strings.EqualFold(current.Data, "img") {
			if alt := cleanText(attribute(current, "alt")); alt != "" {
				alternatives = append(alternatives, alt)
			}
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return strings.Join(alternatives, " ")
}

func cleanText(value string) string {
	return strings.TrimSpace(whitespacePattern.ReplaceAllString(value, " "))
}

func tokenize(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
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

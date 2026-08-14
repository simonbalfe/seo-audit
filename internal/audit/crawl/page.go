package crawl

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
	if !isHTMLContent(report.ContentType) {
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
		if renderedHTML, renderErr := c.render(ctx, report.FinalURL); renderErr == nil {
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
			case "article":
				report.HasArticle = true
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
			case "p":
				value := cleanText(nodeText(node))
				words := len(tokenize(value))
				if words > 0 {
					report.ParagraphCount++
					if words > report.LongestParagraph {
						report.LongestParagraph = words
					}
					if report.FirstParagraph == "" && words >= 5 {
						report.FirstParagraph = value
					}
				}
			case "time":
				extractTime(node, report)
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
			if report.Author == "" && (hasToken(attribute(node, "rel"), "author") || strings.EqualFold(attribute(node, "itemprop"), "author")) {
				report.Author = cleanText(nodeText(node))
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
	case "author":
		report.Author = content
	}
	if property == "article:published_time" || property == "og:published_time" {
		report.PublishedDate = content
	}
	if property == "article:modified_time" || property == "og:updated_time" {
		report.ModifiedDate = content
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
	raw := strings.TrimSpace(attribute(node, "href"))
	if strings.HasPrefix(strings.ToLower(raw), "tel:") {
		report.PhoneLinks = appendUnique(report.PhoneLinks, strings.TrimSpace(raw[4:]))
		return
	}
	resolved := resolveURL(base, raw)
	if resolved == "" {
		return
	}
	parsed, err := url.Parse(resolved)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return
	}
	if parsed.Path == "/cdn-cgi/l/email-protection" {
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
	link := Link{
		URL:      normalized,
		Text:     anchorText(node),
		Internal: internal,
		NoFollow: hasToken(attribute(node, "rel"), "nofollow"),
	}
	report.Links = append(report.Links, link)
	if isBookingLink(link) {
		report.BookingLinks = append(report.BookingLinks, link)
	}
}

func isBookingLink(link Link) bool {
	value := strings.ToLower(link.Text + " " + link.URL)
	for _, signal := range []string{"book", "appointment", "consultation", "schedule", "calendly"} {
		if strings.Contains(value, signal) {
			return true
		}
	}
	return false
}

func extractImage(node *html.Node, base *url.URL, report *PageReport) {
	source := resolveURL(base, attribute(node, "src"))
	alt, hasAlt := attributeValue(node, "alt")
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
	collectContentMetadata(value, report)
}

func collectContentMetadata(value any, report *PageReport) {
	switch item := value.(type) {
	case map[string]any:
		if schemaType, ok := item["@type"].(string); ok && isArticleSchema(schemaType) {
			report.HasArticle = true
		}
		if report.Author == "" {
			report.Author = schemaName(item["author"])
		}
		if report.PublishedDate == "" {
			report.PublishedDate = schemaText(item["datePublished"])
		}
		if report.ModifiedDate == "" {
			report.ModifiedDate = schemaText(item["dateModified"])
		}
		for _, child := range item {
			collectContentMetadata(child, report)
		}
	case []any:
		for _, child := range item {
			collectContentMetadata(child, report)
		}
	}
}

func schemaName(value any) string {
	switch item := value.(type) {
	case string:
		return cleanText(item)
	case map[string]any:
		return schemaText(item["name"])
	case []any:
		for _, candidate := range item {
			if name := schemaName(candidate); name != "" {
				return name
			}
		}
	}
	return ""
}

func schemaText(value any) string {
	text, _ := value.(string)
	return cleanText(text)
}

func extractTime(node *html.Node, report *PageReport) {
	value := cleanText(attribute(node, "datetime"))
	if value == "" {
		value = cleanText(nodeText(node))
	}
	if value == "" {
		return
	}
	signal := strings.ToLower(strings.Join([]string{
		attribute(node, "itemprop"),
		attribute(node, "class"),
		attribute(node, "data-testid"),
	}, " "))
	if strings.Contains(signal, "modified") || strings.Contains(signal, "updated") {
		if report.ModifiedDate == "" {
			report.ModifiedDate = value
		}
		return
	}
	if strings.Contains(signal, "published") || hasAncestor(node, "article") {
		if report.PublishedDate == "" {
			report.PublishedDate = value
		}
	}
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
	if !isHTMLContent(page.ContentType) {
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

func isArticleSchema(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "article", "blogposting", "newsarticle":
		return true
	default:
		return false
	}
}

func hasAncestor(node *html.Node, tag string) bool {
	for parent := node.Parent; parent != nil; parent = parent.Parent {
		if parent.Type == html.ElementNode && strings.EqualFold(parent.Data, tag) {
			return true
		}
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
	return strings.TrimPrefix(strings.ToLower(left.Hostname()), "www.") == strings.TrimPrefix(strings.ToLower(right.Hostname()), "www.")
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
	return strings.Join(strings.Fields(value), " ")
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

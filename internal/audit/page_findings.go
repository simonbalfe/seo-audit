package audit

import (
	"fmt"
	"net/url"
	"strings"
)

func pageFindings(page PageReport) []Finding {
	findings := make([]Finding, 0)
	add := func(category, check string, status Status, priority, evidence, fix string) {
		findings = append(findings, newFinding(category, check, status, priority, page.URL, evidence, fix))
	}
	if page.StatusCode < 200 || page.StatusCode >= 300 {
		add("response", "Non-success response", Fail, "high", fmt.Sprintf("HTTP %d", page.StatusCode), "Return a successful response or link directly to the final valid URL.")
	}
	if len(page.RedirectChain) > 1 {
		add("response", "Redirect chain", Warn, "medium", strings.Join(append([]string{page.URL}, page.RedirectChain...), " -> "), "Replace multi-hop redirects with one direct redirect.")
	}
	if !isHTMLContent(page.ContentType) {
		return findings
	}
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
	if page.Indexable && page.WordCount >= 500 && len(page.H2) == 0 {
		add("content-review", "Long page has no subheadings", Warn, "low", fmt.Sprintf("%d words and no H2 headings", page.WordCount), "Break long content into descriptive sections that help readers scan and understand it.")
	}
	if page.Indexable && page.LongestParagraph > 150 {
		add("content-review", "Very long paragraph", Warn, "low", fmt.Sprintf("longest paragraph is %d words", page.LongestParagraph), "Split the paragraph where the subject changes so the content is easier to scan.")
	}
	if page.Indexable && isArticlePage(page) {
		if page.Author == "" {
			add("content-review", "Article author not evident", Warn, "low", "no public author signal found", "Show a real author or responsible organisation when authorship helps readers assess the content.")
		}
		if page.PublishedDate == "" && page.ModifiedDate == "" {
			add("content-review", "Article date not evident", Warn, "low", "no published or updated date found", "Show a published or updated date when freshness matters to the topic.")
		}
		if page.WordCount >= 500 && len(page.ExternalLinks) == 0 {
			add("content-review", "Long article has no external sources", Warn, "low", fmt.Sprintf("%d words and no external links", page.WordCount), "Review factual claims and link to useful primary sources where external evidence is appropriate.")
		}
	}
	for _, resource := range append(append([]string{}, page.Resources...), page.ExternalLinks...) {
		if strings.HasPrefix(page.FinalURL, "https://") && strings.HasPrefix(resource, "http://") {
			add("security", "Mixed HTTP resource", Fail, "high", resource, "Load every resource and link target over HTTPS.")
			break
		}
	}
	return findings
}

func isArticlePage(page PageReport) bool {
	for _, schemaType := range page.SchemaTypes {
		if isArticleSchema(schemaType) {
			return true
		}
	}
	parsed, err := url.Parse(page.FinalURL)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	return len(parts) >= 2 && strings.EqualFold(parts[0], "blog")
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

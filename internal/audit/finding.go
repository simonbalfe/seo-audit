package audit

import "strings"

func newFinding(category, check string, status Status, priority, target, evidence, fix string) Finding {
	return Finding{
		Category: category,
		Check:    check,
		Status:   status,
		Priority: priority,
		URL:      target,
		Evidence: evidence,
		Fix:      fix,
	}
}

func isHTMLContent(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "html")
}

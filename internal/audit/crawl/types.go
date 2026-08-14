package crawl

import "github.com/simonbalfe/seo-audit/internal/report"

type Status = report.Status

const (
	Warn = report.Warn
	Fail = report.Fail
)

type Finding = report.Finding
type Link = report.Link
type Alternate = report.Alternate
type PageReport = report.PageReport
type AgentAccess = report.AgentAccess
type RobotsReport = report.RobotsReport
type SitemapReport = report.SitemapReport
type ResourceReport = report.ResourceReport
type Summary = report.Summary
type SiteReport = report.SiteReport
type ProgressEvent = report.ProgressEvent

type Options struct {
	Limit         int
	CheckExternal bool
	Progress      func(ProgressEvent)
}

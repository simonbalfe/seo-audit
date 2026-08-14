package audit

import "github.com/simonbalfe/seo-audit/internal/report"

type SiteReport = report.SiteReport
type ProgressEvent = report.ProgressEvent

type Options struct {
	Limit                   int
	CheckExternal           bool
	CheckPerformance        bool
	CheckBacklinks          bool
	ClassificationCachePath string
	Market                  *MarketOptions
	Progress                func(ProgressEvent)
}

type MarketOptions struct {
	Location        string
	Language        string
	MaxChecks       int
	Keywords        []string
	TargetName      string
	TargetCategory  string
	TargetCountry   string
	TargetPlaceID   string
	TargetLatitude  float64
	TargetLongitude float64
	GridRadiusKM    float64
}

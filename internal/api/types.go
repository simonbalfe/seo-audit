package api

import (
	"github.com/simonbalfe/seo-audit/internal/dataforseo"
	"github.com/simonbalfe/seo-audit/internal/gsc"
	"github.com/simonbalfe/seo-audit/internal/protocol"
	"github.com/simonbalfe/seo-audit/internal/ranktracking"
)

const (
	DefaultAPIURL           = protocol.DefaultAPIURL
	DefaultAuditPageLimit   = protocol.DefaultAuditPageLimit
	MaxAuditPageLimit       = protocol.MaxAuditPageLimit
	DefaultRequestTimeout   = protocol.DefaultRequestTimeout
	MaxRequestTimeout       = protocol.MaxRequestTimeout
	DefaultProviderLimit    = protocol.DefaultProviderLimit
	MaxProviderLimit        = protocol.MaxProviderLimit
	DefaultGSCDays          = protocol.DefaultGSCDays
	DefaultGSCLimit         = protocol.DefaultGSCLimit
	MaxGSCDays              = protocol.MaxGSCDays
	MaxGSCLimit             = protocol.MaxGSCLimit
	DefaultJobWorkers       = protocol.DefaultJobWorkers
	DefaultJobRetention     = protocol.DefaultJobRetention
	DefaultProviderCacheTTL = protocol.DefaultProviderCacheTTL
)

type AuditRequest = protocol.AuditRequest
type GSCRequest = protocol.GSCRequest
type DataForSEORequest = protocol.DataForSEORequest
type OpportunityRequest = protocol.OpportunityRequest
type BacklinkRequest = protocol.BacklinkRequest
type RankTrackerRequest = protocol.RankTrackerRequest
type RankTrackerPatchRequest = protocol.RankTrackerPatchRequest
type RankKeywordPatchRequest = protocol.RankKeywordPatchRequest
type RankCheckRequest = protocol.RankCheckRequest
type CapabilitiesResponse = protocol.CapabilitiesResponse
type APICapabilities = protocol.APICapabilities
type AuditCapabilities = protocol.AuditCapabilities
type ProviderCapabilities = protocol.ProviderCapabilities
type ProviderCapability = protocol.ProviderCapability
type RankingCapabilities = protocol.RankingCapabilities

type OpportunityReport struct {
	Target        string             `json:"target"`
	SearchConsole *gsc.Report        `json:"search_console,omitempty"`
	SearchData    *dataforseo.Report `json:"search_data,omitempty"`
}

type RankTrackersResponse struct {
	Trackers []ranktracking.Report `json:"trackers"`
}

package protocol

import "time"

const (
	DefaultAPIURL           = "http://127.0.0.1:8787"
	DefaultAuditPageLimit   = 500
	MaxAuditPageLimit       = 5000
	DefaultRequestTimeout   = 30
	MaxRequestTimeout       = 120
	DefaultProviderLimit    = 25
	MaxProviderLimit        = 100
	DefaultGSCDays          = 28
	DefaultGSCLimit         = 250
	MaxGSCDays              = 480
	MaxGSCLimit             = 25000
	DefaultJobWorkers       = 4
	DefaultJobRetention     = 100
	DefaultRankLocation     = "United Kingdom"
	DefaultRankLanguage     = "en"
	DefaultProviderCacheTTL = 6 * time.Hour
)

type AuditRequest struct {
	URL                   string `json:"url"`
	PageLimit             int    `json:"page_limit,omitempty"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds,omitempty"`
	CheckExternal         *bool  `json:"check_external,omitempty"`
	CheckPerformance      *bool  `json:"check_performance,omitempty"`
	Save                  bool   `json:"save"`
}

type GSCRequest struct {
	SiteURL  string `json:"site_url,omitempty"`
	Days     int    `json:"days,omitempty"`
	RowLimit int    `json:"row_limit,omitempty"`
	Save     bool   `json:"save"`
}

type DataForSEORequest struct {
	Location        string `json:"location,omitempty"`
	Language        string `json:"language,omitempty"`
	RowLimit        int    `json:"row_limit,omitempty"`
	CacheTTLSeconds int64  `json:"cache_ttl_seconds,omitempty"`
	Refresh         bool   `json:"refresh"`
}

type OpportunityRequest struct {
	URL        string            `json:"url"`
	Sources    []string          `json:"sources"`
	GSC        GSCRequest        `json:"gsc,omitempty"`
	DataForSEO DataForSEORequest `json:"dataforseo,omitempty"`
}

type BacklinkRequest struct {
	URL             string `json:"url"`
	Source          string `json:"source"`
	RowLimit        int    `json:"row_limit,omitempty"`
	CacheTTLSeconds int64  `json:"cache_ttl_seconds,omitempty"`
	Refresh         bool   `json:"refresh"`
}

type RankTrackerRequest struct {
	URL       string   `json:"url"`
	Location  string   `json:"location,omitempty"`
	Language  string   `json:"language,omitempty"`
	Devices   string   `json:"devices,omitempty"`
	SERPDepth int      `json:"serp_depth,omitempty"`
	Keywords  []string `json:"keywords"`
}

type RankTrackerPatchRequest struct {
	Devices   string `json:"devices,omitempty"`
	SERPDepth int    `json:"serp_depth,omitempty"`
}

type RankKeywordPatchRequest struct {
	Add    []string `json:"add,omitempty"`
	Remove []string `json:"remove,omitempty"`
}

type RankCheckRequest struct {
	Source string `json:"source"`
}

type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobSucceeded JobStatus = "succeeded"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

type JobEvent struct {
	Sequence int64     `json:"sequence"`
	Time     time.Time `json:"time"`
	Stage    string    `json:"stage"`
	Message  string    `json:"message"`
}

type Job struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Status      JobStatus  `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	Events      []JobEvent `json:"events,omitempty"`
	StatusURL   string     `json:"status_url"`
	EventsURL   string     `json:"events_url"`
	ResultURL   string     `json:"result_url"`
}

type JobEvents struct {
	Events    []JobEvent `json:"events"`
	NextAfter int64      `json:"next_after"`
}

type CapabilitiesResponse struct {
	API       APICapabilities      `json:"api"`
	Audit     AuditCapabilities    `json:"audit"`
	Providers ProviderCapabilities `json:"providers"`
	Rankings  RankingCapabilities  `json:"rankings"`
}

type APICapabilities struct {
	Version      string `json:"version"`
	JobWorkers   int    `json:"job_workers"`
	JobRetention int    `json:"job_retention"`
}

type AuditCapabilities struct {
	DefaultPageLimit         int `json:"default_page_limit"`
	MaxPageLimit             int `json:"max_page_limit"`
	DefaultTimeoutSeconds    int `json:"default_timeout_seconds"`
	MaxRequestTimeoutSeconds int `json:"max_request_timeout_seconds"`
}

type ProviderCapabilities struct {
	DataForSEO ProviderCapability `json:"dataforseo"`
	GSC        ProviderCapability `json:"gsc"`
}

type ProviderCapability struct {
	Configured bool `json:"configured"`
}

type RankingCapabilities struct {
	MaxKeywords int `json:"max_keywords"`
	MaxDepth    int `json:"max_depth"`
}

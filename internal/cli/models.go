package cli

import "time"

type auditReport struct {
	StartURL     string                 `json:"start_url"`
	Duration     int64                  `json:"duration_ms"`
	LimitReached bool                   `json:"limit_reached"`
	Summary      auditSummary           `json:"summary"`
	Performance  auditPerformanceReport `json:"performance"`
	Findings     []auditFinding         `json:"findings"`
}

type auditSummary struct {
	Pages         int `json:"pages"`
	Indexable     int `json:"indexable"`
	NonIndexable  int `json:"non_indexable"`
	Failures      int `json:"failures"`
	Warnings      int `json:"warnings"`
	InternalLinks int `json:"internal_links"`
	ExternalLinks int `json:"external_links"`
	SitemapURLs   int `json:"sitemap_urls"`
}

type auditFinding struct {
	Category string `json:"category"`
	Check    string `json:"check"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
	URL      string `json:"url,omitempty"`
	Evidence string `json:"evidence"`
	Fix      string `json:"fix,omitempty"`
}

type auditPerformanceReport struct {
	Available bool                    `json:"available"`
	Profile   string                  `json:"profile"`
	Errors    []string                `json:"errors,omitempty"`
	Summary   auditPerformanceSummary `json:"summary"`
}

type auditPerformanceSummary struct {
	Pages     int     `json:"pages"`
	WorstLCP  float64 `json:"worst_lcp_ms"`
	WorstCLS  float64 `json:"worst_cls"`
	WorstTBT  float64 `json:"worst_tbt_ms"`
	WorstTTFB float64 `json:"worst_ttfb_ms"`
}

type opportunityReport struct {
	Target        string          `json:"target"`
	SearchConsole *gscReport      `json:"search_console,omitempty"`
	SearchData    *providerReport `json:"search_data,omitempty"`
}

type gscReport struct {
	Available        bool                 `json:"available"`
	SiteURL          string               `json:"site_url"`
	StartDate        string               `json:"start_date"`
	EndDate          string               `json:"end_date"`
	Summary          gscSummary           `json:"summary"`
	StrikingDistance []gscQueryPageMetric `json:"striking_distance"`
	QueryOverlaps    []gscQueryOverlap    `json:"query_overlaps"`
}

type gscSummary struct {
	Rows                int     `json:"rows"`
	ReturnedClicks      float64 `json:"returned_clicks"`
	ReturnedImpressions float64 `json:"returned_impressions"`
	ReturnedCTR         float64 `json:"returned_ctr"`
	WeightedPosition    float64 `json:"weighted_position"`
}

type gscQueryPageMetric struct {
	Query       string  `json:"query"`
	Page        string  `json:"page"`
	Clicks      float64 `json:"clicks"`
	Impressions float64 `json:"impressions"`
	CTR         float64 `json:"ctr"`
	Position    float64 `json:"position"`
}

type gscQueryOverlap struct {
	Query       string   `json:"query"`
	Pages       []string `json:"pages"`
	Impressions float64  `json:"impressions"`
}

type providerReport struct {
	Available         bool                   `json:"available"`
	Target            string                 `json:"target"`
	Location          string                 `json:"location,omitempty"`
	Language          string                 `json:"language,omitempty"`
	RetrievedAt       time.Time              `json:"retrieved_at"`
	CostUSD           float64                `json:"cost_usd"`
	RequestedDatasets int                    `json:"requested_datasets"`
	SuccessfulCalls   int                    `json:"successful_calls"`
	LiveCalls         int                    `json:"live_calls"`
	Cache             providerCacheInfo      `json:"cache"`
	SnapshotID        int64                  `json:"snapshot_id,omitempty"`
	OrganicVisibility organicMetrics         `json:"organic_visibility,omitempty"`
	RankedKeywords    []rankedKeyword        `json:"ranked_keywords,omitempty"`
	KeywordIdeas      []keywordIdea          `json:"keyword_ideas,omitempty"`
	Competitors       []competitor           `json:"competitors,omitempty"`
	BacklinkSummary   backlinkSummary        `json:"backlink_summary,omitempty"`
	ReferringDomains  []referringDomain      `json:"referring_domains,omitempty"`
	TopBacklinks      []backlink             `json:"top_backlinks,omitempty"`
	Errors            []providerDatasetError `json:"errors,omitempty"`
	StorageErrors     []string               `json:"storage_errors,omitempty"`
}

type providerCacheInfo struct {
	Hit                   bool       `json:"hit"`
	Stored                bool       `json:"stored"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	CachedProviderCostUSD float64    `json:"cached_provider_cost_usd,omitempty"`
}

type providerDatasetError struct {
	Dataset string `json:"dataset"`
	Message string `json:"message"`
}

type organicMetrics struct {
	Keywords         int     `json:"keywords"`
	EstimatedTraffic float64 `json:"estimated_traffic"`
	Position1        int     `json:"position_1"`
	Positions2To3    int     `json:"positions_2_to_3"`
	Positions4To10   int     `json:"positions_4_to_10"`
}

type rankedKeyword struct {
	Keyword      string   `json:"keyword"`
	Position     int      `json:"position"`
	URL          string   `json:"url"`
	SearchVolume int      `json:"search_volume"`
	Difficulty   *float64 `json:"difficulty,omitempty"`
	Intent       string   `json:"intent,omitempty"`
}

type keywordIdea struct {
	Keyword      string   `json:"keyword"`
	SearchVolume int      `json:"search_volume"`
	Difficulty   *float64 `json:"difficulty,omitempty"`
	CPC          *float64 `json:"cpc,omitempty"`
	Intent       string   `json:"intent,omitempty"`
}

type competitor struct {
	Domain           string  `json:"domain"`
	KeywordOverlap   int     `json:"keyword_overlap"`
	OrganicKeywords  int     `json:"organic_keywords"`
	EstimatedTraffic float64 `json:"estimated_traffic"`
}

type backlinkSummary struct {
	DataForSEORank   int `json:"dataforseo_rank"`
	TargetSpamScore  int `json:"target_spam_score"`
	Backlinks        int `json:"backlinks"`
	ReferringDomains int `json:"referring_domains"`
}

type referringDomain struct {
	Domain                 string `json:"domain"`
	DataForSEORank         int    `json:"dataforseo_rank"`
	Backlinks              int    `json:"backlinks"`
	NofollowReferringPages int    `json:"nofollow_referring_pages"`
}

type backlink struct {
	SourceURL string `json:"source_url"`
	TargetURL string `json:"target_url"`
	Dofollow  bool   `json:"dofollow"`
	LinkRank  int    `json:"link_rank"`
}

type rankConfig struct {
	ID        int64     `json:"id"`
	Target    string    `json:"target"`
	Location  string    `json:"location"`
	Language  string    `json:"language"`
	Devices   string    `json:"devices"`
	SERPDepth int       `json:"serp_depth"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type rankKeyword struct {
	ID        int64     `json:"id"`
	ConfigID  int64     `json:"config_id"`
	Keyword   string    `json:"keyword"`
	CreatedAt time.Time `json:"created_at"`
}

type rankKeywordUpdate struct {
	Config        rankConfig    `json:"config"`
	Added         int           `json:"added,omitempty"`
	Removed       int           `json:"removed,omitempty"`
	TotalKeywords int           `json:"total_keywords"`
	Keywords      []rankKeyword `json:"keywords"`
}

type rankRun struct {
	ID              int64      `json:"id"`
	Status          string     `json:"status"`
	RequestedTasks  int        `json:"requested_tasks"`
	SuccessfulTasks int        `json:"successful_tasks"`
	LiveCalls       int        `json:"live_calls"`
	CostUSD         float64    `json:"cost_usd"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	ErrorMessage    string     `json:"error_message,omitempty"`
}

type rankSummary struct {
	TrackedKeywords int `json:"tracked_keywords"`
	Ranking         int `json:"ranking"`
	NotRanking      int `json:"not_ranking"`
	Top3            int `json:"top_3"`
	Top10           int `json:"top_10"`
	Improved        int `json:"improved"`
	Declined        int `json:"declined"`
	New             int `json:"new"`
	Lost            int `json:"lost"`
}

type rankRow struct {
	Keyword          string `json:"keyword"`
	Device           string `json:"device"`
	Observed         bool   `json:"observed"`
	Position         *int   `json:"position,omitempty"`
	PreviousPosition *int   `json:"previous_position,omitempty"`
	PreviousObserved bool   `json:"previous_observed"`
	RankingURL       string `json:"ranking_url,omitempty"`
	Change           string `json:"change"`
}

type rankReport struct {
	Config        rankConfig    `json:"config"`
	Keywords      []rankKeyword `json:"keywords"`
	LatestRun     *rankRun      `json:"latest_run,omitempty"`
	PreviousRunID *int64        `json:"previous_run_id,omitempty"`
	Summary       rankSummary   `json:"summary"`
	Rows          []rankRow     `json:"rows"`
}

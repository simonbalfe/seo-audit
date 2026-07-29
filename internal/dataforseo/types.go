package dataforseo

import "time"

type Options struct {
	Target   string
	Location string
	Language string
	Limit    int
	CacheTTL time.Duration
	Refresh  bool
	Progress func(dataset, message string)
}

type DatasetError struct {
	Dataset string `json:"dataset"`
	Message string `json:"message"`
}

type CacheInfo struct {
	Enabled               bool       `json:"enabled"`
	Hit                   bool       `json:"hit"`
	Stored                bool       `json:"stored"`
	TTLSeconds            int64      `json:"ttl_seconds,omitempty"`
	ExpiresAt             *time.Time `json:"expires_at,omitempty"`
	CachedProviderCostUSD float64    `json:"cached_provider_cost_usd,omitempty"`
}

type OrganicMetrics struct {
	Keywords             int     `json:"keywords"`
	EstimatedTraffic     float64 `json:"estimated_traffic"`
	EstimatedTrafficCost float64 `json:"estimated_traffic_cost"`
	Position1            int     `json:"position_1"`
	Positions2To3        int     `json:"positions_2_to_3"`
	Positions4To10       int     `json:"positions_4_to_10"`
	Positions11To20      int     `json:"positions_11_to_20"`
	Positions21To100     int     `json:"positions_21_to_100"`
	New                  int     `json:"new"`
	Up                   int     `json:"up"`
	Down                 int     `json:"down"`
	Lost                 int     `json:"lost"`
}

type RankedKeyword struct {
	Keyword          string   `json:"keyword"`
	Position         int      `json:"position"`
	PreviousPosition *int     `json:"previous_position,omitempty"`
	URL              string   `json:"url"`
	SearchVolume     int      `json:"search_volume"`
	Difficulty       *float64 `json:"difficulty,omitempty"`
	CPC              *float64 `json:"cpc,omitempty"`
	Intent           string   `json:"intent,omitempty"`
	EstimatedTraffic float64  `json:"estimated_traffic"`
	LastUpdated      string   `json:"last_updated,omitempty"`
}

type KeywordIdea struct {
	Keyword          string   `json:"keyword"`
	SearchVolume     int      `json:"search_volume"`
	Difficulty       *float64 `json:"difficulty,omitempty"`
	CPC              *float64 `json:"cpc,omitempty"`
	Competition      *float64 `json:"competition,omitempty"`
	CompetitionLevel string   `json:"competition_level,omitempty"`
	Intent           string   `json:"intent,omitempty"`
	LastUpdated      string   `json:"last_updated,omitempty"`
}

type Competitor struct {
	Domain           string  `json:"domain"`
	KeywordOverlap   int     `json:"keyword_overlap"`
	AveragePosition  float64 `json:"average_position"`
	OrganicKeywords  int     `json:"organic_keywords"`
	EstimatedTraffic float64 `json:"estimated_traffic"`
}

type BacklinkSummary struct {
	DataForSEORank          int    `json:"dataforseo_rank"`
	TargetSpamScore         int    `json:"target_spam_score"`
	Backlinks               int    `json:"backlinks"`
	BacklinksSpamScore      int    `json:"backlinks_spam_score"`
	ReferringDomains        int    `json:"referring_domains"`
	ReferringMainDomains    int    `json:"referring_main_domains"`
	ReferringPages          int    `json:"referring_pages"`
	NofollowReferringPages  int    `json:"nofollow_referring_pages"`
	ReferringIPs            int    `json:"referring_ips"`
	BrokenBacklinks         int    `json:"broken_backlinks"`
	BrokenPages             int    `json:"broken_pages"`
	FirstSeen               string `json:"first_seen,omitempty"`
	CrawledPagesInLinkIndex int    `json:"crawled_pages_in_link_index"`
}

type ReferringDomain struct {
	Domain                 string `json:"domain"`
	DataForSEORank         int    `json:"dataforseo_rank"`
	Backlinks              int    `json:"backlinks"`
	BacklinksSpamScore     int    `json:"backlinks_spam_score"`
	ReferringPages         int    `json:"referring_pages"`
	NofollowReferringPages int    `json:"nofollow_referring_pages"`
	FirstSeen              string `json:"first_seen,omitempty"`
}

type Backlink struct {
	SourceURL          string `json:"source_url"`
	SourceDomain       string `json:"source_domain"`
	TargetURL          string `json:"target_url"`
	Anchor             string `json:"anchor,omitempty"`
	Dofollow           bool   `json:"dofollow"`
	LinkRank           int    `json:"link_rank"`
	SourceDomainRank   int    `json:"source_domain_rank"`
	BacklinkSpamScore  int    `json:"backlink_spam_score"`
	SourcePageStatus   int    `json:"source_page_status"`
	TargetPageStatus   int    `json:"target_page_status"`
	SourcePageTitle    string `json:"source_page_title,omitempty"`
	SourcePageLanguage string `json:"source_page_language,omitempty"`
	SemanticLocation   string `json:"semantic_location,omitempty"`
	FirstSeen          string `json:"first_seen,omitempty"`
	LastSeen           string `json:"last_seen,omitempty"`
	New                bool   `json:"new"`
	Lost               bool   `json:"lost"`
	Broken             bool   `json:"broken"`
}

type Report struct {
	Enabled           bool              `json:"enabled"`
	Available         bool              `json:"available"`
	Source            string            `json:"source"`
	DatasetGroup      string            `json:"dataset_group"`
	Target            string            `json:"target"`
	Location          string            `json:"location,omitempty"`
	Language          string            `json:"language,omitempty"`
	RetrievedAt       time.Time         `json:"retrieved_at"`
	CostUSD           float64           `json:"cost_usd"`
	RequestedDatasets int               `json:"requested_datasets"`
	SuccessfulCalls   int               `json:"successful_calls"`
	LiveCalls         int               `json:"live_calls"`
	Cache             CacheInfo         `json:"cache"`
	SnapshotID        int64             `json:"snapshot_id,omitempty"`
	OrganicVisibility OrganicMetrics    `json:"organic_visibility,omitempty"`
	RankedKeywords    []RankedKeyword   `json:"ranked_keywords,omitempty"`
	KeywordIdeas      []KeywordIdea     `json:"keyword_ideas,omitempty"`
	Competitors       []Competitor      `json:"competitors,omitempty"`
	BacklinkSummary   BacklinkSummary   `json:"backlink_summary,omitempty"`
	ReferringDomains  []ReferringDomain `json:"referring_domains,omitempty"`
	TopBacklinks      []Backlink        `json:"top_backlinks,omitempty"`
	Errors            []DatasetError    `json:"errors,omitempty"`
	StorageErrors     []string          `json:"storage_errors,omitempty"`
}

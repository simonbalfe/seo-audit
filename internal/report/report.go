package report

type Status string

const (
	Warn Status = "warn"
	Fail Status = "fail"
)

type Finding struct {
	Category string `json:"category"`
	Check    string `json:"check"`
	Status   Status `json:"status"`
	Priority string `json:"priority"`
	URL      string `json:"url,omitempty"`
	Evidence string `json:"evidence"`
	Fix      string `json:"fix,omitempty"`
}

type Link struct {
	URL      string `json:"url"`
	Text     string `json:"text,omitempty"`
	Internal bool   `json:"internal"`
	NoFollow bool   `json:"nofollow"`
}

type Alternate struct {
	Language string `json:"language"`
	URL      string `json:"url"`
}

type PageReport struct {
	URL               string      `json:"url"`
	FinalURL          string      `json:"final_url"`
	PageType          string      `json:"page_type"`
	PageTypeSource    string      `json:"page_type_source,omitempty"`
	PageTypeReason    string      `json:"page_type_reason,omitempty"`
	PriorityPage      bool        `json:"priority_page"`
	KeywordSeeds      []string    `json:"keyword_seeds,omitempty"`
	TargetKeywords    []string    `json:"target_keywords,omitempty"`
	RedirectChain     []string    `json:"redirect_chain,omitempty"`
	StatusCode        int         `json:"status_code"`
	ContentType       string      `json:"content_type"`
	SizeBytes         int         `json:"size_bytes"`
	Depth             int         `json:"depth"`
	Inlinks           int         `json:"inlinks"`
	InSitemap         bool        `json:"in_sitemap"`
	RobotsAllowed     bool        `json:"robots_allowed"`
	Indexable         bool        `json:"indexable"`
	Indexability      string      `json:"indexability,omitempty"`
	Title             string      `json:"title,omitempty"`
	Titles            []string    `json:"titles,omitempty"`
	Description       string      `json:"description,omitempty"`
	Descriptions      []string    `json:"descriptions,omitempty"`
	H1                []string    `json:"h1,omitempty"`
	H2                []string    `json:"h2,omitempty"`
	HeadingLevels     []int       `json:"heading_levels,omitempty"`
	FirstParagraph    string      `json:"first_paragraph,omitempty"`
	ParagraphCount    int         `json:"paragraph_count"`
	LongestParagraph  int         `json:"longest_paragraph_words"`
	HasArticle        bool        `json:"has_article"`
	Author            string      `json:"author,omitempty"`
	PublishedDate     string      `json:"published_date,omitempty"`
	ModifiedDate      string      `json:"modified_date,omitempty"`
	Canonical         string      `json:"canonical,omitempty"`
	Canonicals        []string    `json:"canonicals,omitempty"`
	Robots            string      `json:"robots,omitempty"`
	XRobots           string      `json:"x_robots,omitempty"`
	Language          string      `json:"language,omitempty"`
	Hreflang          []Alternate `json:"hreflang,omitempty"`
	WordCount         int         `json:"word_count"`
	ImageCount        int         `json:"image_count"`
	ImagesMissingAlt  int         `json:"images_missing_alt"`
	ImagesEmptyAlt    int         `json:"images_empty_alt"`
	StructuredData    int         `json:"structured_data_blocks"`
	InvalidStructured int         `json:"invalid_structured_data_blocks"`
	SchemaTypes       []string    `json:"schema_types,omitempty"`
	InternalLinks     []string    `json:"internal_links,omitempty"`
	ExternalLinks     []string    `json:"external_links,omitempty"`
	Links             []Link      `json:"links,omitempty"`
	PhoneLinks        []string    `json:"phone_links,omitempty"`
	BookingLinks      []Link      `json:"booking_links,omitempty"`
	Resources         []string    `json:"resources,omitempty"`
	HasViewport       bool        `json:"has_viewport"`
	HasMain           bool        `json:"has_main"`
	ContentHash       string      `json:"content_hash,omitempty"`
	Rendered          bool        `json:"rendered"`
	Duration          int64       `json:"duration_ms"`
	Findings          []Finding   `json:"findings"`
	TextTokens        []string    `json:"-"`
}

type PageClassificationReport struct {
	Model            string         `json:"model,omitempty"`
	AIClassified     int            `json:"ai_classified"`
	CacheHits        int            `json:"cache_hits"`
	Unknown          int            `json:"unknown"`
	PriorityPages    int            `json:"priority_pages"`
	Requests         int            `json:"requests"`
	PromptTokens     int            `json:"prompt_tokens"`
	CompletionTokens int            `json:"completion_tokens"`
	CostUSD          float64        `json:"cost_usd"`
	Counts           map[string]int `json:"counts"`
	Errors           []string       `json:"errors,omitempty"`
}

type MarketError struct {
	Operation string `json:"operation"`
	Message   string `json:"message"`
}

type Opportunity struct {
	Keyword              string   `json:"keyword"`
	Source               string   `json:"source"`
	Status               string   `json:"status"`
	Priority             string   `json:"priority"`
	Evidence             string   `json:"evidence"`
	URL                  string   `json:"url,omitempty"`
	TargetURL            string   `json:"target_url,omitempty"`
	PageTitle            string   `json:"page_title,omitempty"`
	CountryPosition      int      `json:"country_position,omitempty"`
	Position             int      `json:"position,omitempty"`
	MapsPosition         int      `json:"maps_position,omitempty"`
	MapsChecked          bool     `json:"maps_checked"`
	MapsTopThreeCoverage float64  `json:"maps_top_three_coverage_percent,omitempty"`
	SearchVolume         int      `json:"search_volume,omitempty"`
	CPC                  float64  `json:"cpc,omitempty"`
	Importance           int      `json:"importance"`
	Effort               int      `json:"effort"`
	PriorityRatio        float64  `json:"priority_ratio"`
	HasBooking           bool     `json:"has_booking_link"`
	HasPhone             bool     `json:"has_phone_link"`
	Actions              []string `json:"actions"`
}

// ExistingRanking records a keyword the audited domain already ranks for.
type ExistingRanking struct {
	Keyword      string  `json:"keyword"`
	Position     int     `json:"position"`
	URL          string  `json:"url"`
	SearchVolume int     `json:"search_volume,omitempty"`
	CPC          float64 `json:"cpc,omitempty"`
}

type LocalSearchResult struct {
	Position    int     `json:"position"`
	PlaceID     string  `json:"place_id,omitempty"`
	Name        string  `json:"name"`
	Category    string  `json:"category,omitempty"`
	Address     string  `json:"address,omitempty"`
	Domain      string  `json:"domain,omitempty"`
	URL         string  `json:"url,omitempty"`
	Rating      float64 `json:"rating,omitempty"`
	ReviewCount int     `json:"review_count,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	IsTarget    bool    `json:"is_target"`
}

type GeoRankPoint struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Position  int     `json:"position,omitempty"`
	Status    string  `json:"status"`
	Error     string  `json:"error,omitempty"`
}

type MapsVisibility struct {
	Keyword           string              `json:"keyword"`
	Device            string              `json:"device"`
	CenterLatitude    float64             `json:"center_latitude"`
	CenterLongitude   float64             `json:"center_longitude"`
	Zoom              int                 `json:"zoom"`
	TargetPlaceID     string              `json:"target_place_id"`
	TargetPosition    int                 `json:"target_position,omitempty"`
	Results           []LocalSearchResult `json:"results"`
	GridRadiusKM      float64             `json:"grid_radius_km,omitempty"`
	GridPoints        []GeoRankPoint      `json:"grid_points,omitempty"`
	GridCheckedPoints int                 `json:"grid_checked_points,omitempty"`
	GridFailedPoints  int                 `json:"grid_failed_points,omitempty"`
	TopThreeCoverage  float64             `json:"top_three_coverage_percent,omitempty"`
	FoundCoverage     float64             `json:"found_coverage_percent,omitempty"`
	MedianPosition    int                 `json:"median_position,omitempty"`
}

type MarketReport struct {
	Enabled                  bool              `json:"enabled"`
	Source                   string            `json:"source,omitempty"`
	Target                   string            `json:"target,omitempty"`
	Location                 string            `json:"location,omitempty"`
	Language                 string            `json:"language,omitempty"`
	RetrievedAt              string            `json:"retrieved_at,omitempty"`
	LiveCalls                int               `json:"live_calls"`
	CostUSD                  float64           `json:"cost_usd"`
	MaxChecks                int               `json:"max_checks"`
	KeywordIdeas             int               `json:"keyword_ideas"`
	ExistingRankingsLocation string            `json:"existing_rankings_location,omitempty"`
	ExistingRankings         []ExistingRanking `json:"existing_rankings,omitempty"`
	CurrentVisibility        []Opportunity     `json:"current_visibility,omitempty"`
	CurrentMaps              []MapsVisibility  `json:"current_maps_visibility,omitempty"`
	Opportunities            []Opportunity     `json:"opportunities"`
	OpportunityMaps          []MapsVisibility  `json:"opportunity_maps_visibility,omitempty"`
	GridKeywords             []string          `json:"grid_keywords,omitempty"`
	Errors                   []MarketError     `json:"errors,omitempty"`
}

// BacklinkReport records one live DataForSEO domain summary.
type BacklinkReport struct {
	Enabled                  bool           `json:"enabled"`
	Source                   string         `json:"source,omitempty"`
	Target                   string         `json:"target,omitempty"`
	RetrievedAt              string         `json:"retrieved_at,omitempty"`
	LiveCalls                int            `json:"live_calls"`
	CostUSD                  float64        `json:"cost_usd"`
	Backlinks                int            `json:"backlinks"`
	ReferringDomains         int            `json:"referring_domains"`
	ReferringDomainsNofollow int            `json:"referring_domains_nofollow"`
	ReferringPages           int            `json:"referring_pages"`
	ReferringIPs             int            `json:"referring_ips"`
	Rank                     int            `json:"dataforseo_rank"`
	BacklinksSpamScore       int            `json:"backlinks_spam_score"`
	TargetSpamScore          int            `json:"target_spam_score"`
	BrokenBacklinks          int            `json:"broken_backlinks"`
	BrokenPages              int            `json:"broken_pages"`
	Countries                map[string]int `json:"referring_link_countries,omitempty"`
	Error                    string         `json:"error,omitempty"`
}

type AgentAccess struct {
	Agent   string `json:"agent"`
	Allowed bool   `json:"allowed"`
	Rule    string `json:"rule"`
}

type RobotsReport struct {
	URL        string        `json:"url"`
	StatusCode int           `json:"status_code"`
	Sitemaps   []string      `json:"sitemaps,omitempty"`
	Agents     []AgentAccess `json:"agents"`
	Body       string        `json:"body,omitempty"`
}

type SitemapReport struct {
	Sources []string `json:"sources"`
	URLs    []string `json:"urls"`
	Errors  []string `json:"errors,omitempty"`
}

type ResourceReport struct {
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	ContentType string `json:"content_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	Error       string `json:"error,omitempty"`
}

type PerformancePageReport struct {
	URL                            string    `json:"url"`
	Profile                        string    `json:"profile"`
	FCPMilliseconds                float64   `json:"fcp_ms"`
	LCPMilliseconds                float64   `json:"lcp_ms"`
	CLS                            float64   `json:"cls"`
	TBTMilliseconds                float64   `json:"tbt_ms"`
	TTFBMilliseconds               float64   `json:"ttfb_ms"`
	DOMContentLoadedMilliseconds   float64   `json:"dom_content_loaded_ms"`
	LoadMilliseconds               float64   `json:"load_ms"`
	Requests                       int       `json:"requests"`
	TransferBytes                  int64     `json:"transfer_bytes"`
	JavaScriptBytes                int64     `json:"javascript_bytes"`
	CSSBytes                       int64     `json:"css_bytes"`
	ImageBytes                     int64     `json:"image_bytes"`
	ThirdPartyRequests             int       `json:"third_party_requests"`
	DOMNodes                       int       `json:"dom_nodes"`
	ImagesMissingDimensions        int       `json:"images_missing_dimensions"`
	OffscreenImagesWithoutLazyLoad int       `json:"offscreen_images_without_lazy_load"`
	Duration                       int64     `json:"duration_ms"`
	Findings                       []Finding `json:"findings"`
}

type PerformanceSummary struct {
	Pages       int     `json:"pages"`
	Errors      int     `json:"errors"`
	WorstLCP    float64 `json:"worst_lcp_ms"`
	WorstCLS    float64 `json:"worst_cls"`
	WorstTBT    float64 `json:"worst_tbt_ms"`
	WorstTTFB   float64 `json:"worst_ttfb_ms"`
	MaxTransfer int64   `json:"max_transfer_bytes"`
}

type PerformanceReport struct {
	Available bool                    `json:"available"`
	Profile   string                  `json:"profile"`
	Pages     []PerformancePageReport `json:"pages"`
	Errors    []string                `json:"errors,omitempty"`
	Summary   PerformanceSummary      `json:"summary"`
}

type Summary struct {
	Pages              int `json:"pages"`
	Indexable          int `json:"indexable"`
	NonIndexable       int `json:"non_indexable"`
	Failures           int `json:"failures"`
	Warnings           int `json:"warnings"`
	InternalLinks      int `json:"internal_links"`
	ExternalLinks      int `json:"external_links"`
	BrokenInternal     int `json:"broken_internal_links"`
	RedirectedInternal int `json:"redirected_internal_links"`
	SitemapURLs        int `json:"sitemap_urls"`
}

type SiteReport struct {
	StartURL           string                   `json:"start_url"`
	Duration           int64                    `json:"duration_ms"`
	LimitReached       bool                     `json:"limit_reached"`
	Summary            Summary                  `json:"summary"`
	Robots             RobotsReport             `json:"robots"`
	Sitemaps           SitemapReport            `json:"sitemaps"`
	Pages              []PageReport             `json:"pages"`
	PageClassification PageClassificationReport `json:"page_classification"`
	Resources          []ResourceReport         `json:"resources,omitempty"`
	Performance        PerformanceReport        `json:"performance"`
	Market             MarketReport             `json:"market"`
	Backlinks          BacklinkReport           `json:"backlinks"`
	GBP                *GBPAuditReport          `json:"gbp,omitempty"`
	Findings           []Finding                `json:"findings"`
}

type GBPAuditReport struct {
	Query            string       `json:"query"`
	PlaceID          string       `json:"place_id"`
	Name             string       `json:"name"`
	IdentityStatus   string       `json:"identity_status,omitempty"`
	IdentityEvidence string       `json:"identity_evidence,omitempty"`
	Category         string       `json:"category,omitempty"`
	Address          string       `json:"address,omitempty"`
	Market           string       `json:"market,omitempty"`
	Country          string       `json:"country,omitempty"`
	Phone            string       `json:"phone,omitempty"`
	Website          string       `json:"website,omitempty"`
	GoogleMapsURL    string       `json:"google_maps_url,omitempty"`
	BusinessStatus   string       `json:"business_status,omitempty"`
	Rating           float64      `json:"rating,omitempty"`
	ReviewCount      int          `json:"review_count,omitempty"`
	PhotoCount       int          `json:"photo_count,omitempty"`
	Latitude         float64      `json:"latitude,omitempty"`
	Longitude        float64      `json:"longitude,omitempty"`
	Hours            []string     `json:"hours,omitempty"`
	Findings         []GBPFinding `json:"findings"`
}

type GBPFinding struct {
	Priority string `json:"priority"`
	Check    string `json:"check"`
	Evidence string `json:"evidence"`
	Fix      string `json:"fix"`
}

type ProgressEvent struct {
	Stage   string
	Message string
}

package audit

import (
	"net/http"
	"time"
)

type Status string

const (
	Pass Status = "pass"
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

type Image struct {
	URL    string `json:"url"`
	Alt    string `json:"alt,omitempty"`
	HasAlt bool   `json:"has_alt"`
	Lazy   bool   `json:"lazy"`
	Srcset bool   `json:"srcset"`
	Width  string `json:"width,omitempty"`
	Height string `json:"height,omitempty"`
}

type Alternate struct {
	Language string `json:"language"`
	URL      string `json:"url"`
}

type PageReport struct {
	URL               string      `json:"url"`
	FinalURL          string      `json:"final_url"`
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
	Images            []Image     `json:"images,omitempty"`
	StructuredData    int         `json:"structured_data_blocks"`
	InvalidStructured int         `json:"invalid_structured_data_blocks"`
	SchemaTypes       []string    `json:"schema_types,omitempty"`
	InternalLinks     []string    `json:"internal_links,omitempty"`
	ExternalLinks     []string    `json:"external_links,omitempty"`
	Links             []Link      `json:"links,omitempty"`
	Resources         []string    `json:"resources,omitempty"`
	HasViewport       bool        `json:"has_viewport"`
	HasMain           bool        `json:"has_main"`
	HasCharset        bool        `json:"has_charset"`
	OpenGraphTitle    string      `json:"open_graph_title,omitempty"`
	OpenGraphDesc     string      `json:"open_graph_description,omitempty"`
	TwitterCard       string      `json:"twitter_card,omitempty"`
	ContentHash       string      `json:"content_hash,omitempty"`
	Rendered          bool        `json:"rendered"`
	Duration          int64       `json:"duration_ms"`
	Findings          []Finding   `json:"findings"`
	TextTokens        []string    `json:"-"`
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
	StartURL     string           `json:"start_url"`
	Duration     int64            `json:"duration_ms"`
	LimitReached bool             `json:"limit_reached"`
	Summary      Summary          `json:"summary"`
	Robots       RobotsReport     `json:"robots"`
	Sitemaps     SitemapReport    `json:"sitemaps"`
	Pages        []PageReport     `json:"pages"`
	Resources    []ResourceReport `json:"resources,omitempty"`
	Findings     []Finding        `json:"findings"`
}

type ProgressEvent struct {
	Stage   string
	Message string
}

type Options struct {
	Limit         int
	CheckExternal bool
	Progress      func(ProgressEvent)
}

type Client struct {
	HTTP      *http.Client
	UserAgent string
	Render    bool
}

func NewClient(timeout time.Duration) *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		},
		UserAgent: "seo-audit/0.1 (+https://github.com/simonbalfe/seo-audit)",
		Render:    true,
	}
}

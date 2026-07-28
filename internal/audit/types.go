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
	URL      string `json:"url,omitempty"`
	Evidence string `json:"evidence"`
	Fix      string `json:"fix,omitempty"`
}

type PageReport struct {
	URL               string    `json:"url"`
	FinalURL          string    `json:"final_url"`
	StatusCode        int       `json:"status_code"`
	ContentType       string    `json:"content_type"`
	Title             string    `json:"title,omitempty"`
	Description       string    `json:"description,omitempty"`
	H1                []string  `json:"h1,omitempty"`
	Canonical         string    `json:"canonical,omitempty"`
	Robots            string    `json:"robots,omitempty"`
	Language          string    `json:"language,omitempty"`
	WordCount         int       `json:"word_count"`
	ImageCount        int       `json:"image_count"`
	ImagesMissingAlt  int       `json:"images_missing_alt"`
	StructuredData    int       `json:"structured_data_blocks"`
	InvalidStructured int       `json:"invalid_structured_data_blocks"`
	InternalLinks     []string  `json:"internal_links,omitempty"`
	ExternalLinks     []string  `json:"external_links,omitempty"`
	HasViewport       bool      `json:"has_viewport"`
	HasMain           bool      `json:"has_main"`
	Duration          int64     `json:"duration_ms"`
	Findings          []Finding `json:"findings"`
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

type Client struct {
	HTTP      *http.Client
	UserAgent string
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
	}
}

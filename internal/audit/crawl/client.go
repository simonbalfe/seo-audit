package crawl

import (
	"context"
	"net/http"
	"time"
)

const renderWorkerCount = 4

type Client struct {
	HTTP        *http.Client
	UserAgent   string
	Render      bool
	renderSlots chan struct{}
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
		UserAgent:   "seo-audit/0.1 (+https://github.com/simonbalfe/seo-audit)",
		Render:      true,
		renderSlots: make(chan struct{}, renderWorkerCount),
	}
}

func (c *Client) render(ctx context.Context, target string) (string, error) {
	select {
	case c.renderSlots <- struct{}{}:
		defer func() { <-c.renderSlots }()
		return renderHTML(ctx, target)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

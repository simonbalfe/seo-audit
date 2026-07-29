package apiclient

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/protocol"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type Problem struct {
	Type   string            `json:"type"`
	Title  string            `json:"title"`
	Status int               `json:"status"`
	Detail string            `json:"detail"`
	Code   string            `json:"code"`
	Fields map[string]string `json:"fields,omitempty"`
}

func (p Problem) Error() string {
	if p.Detail != "" {
		return p.Detail
	}
	return p.Title
}

func New(baseURL string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(os.Getenv("SEOAUDIT_API_URL")), "/")
	}
	if baseURL == "" {
		baseURL = protocol.DefaultAPIURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("invalid SEO Audit API URL %q", baseURL)
	}
	return &Client{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (c *Client) Submit(ctx context.Context, path string, input any) (protocol.Job, error) {
	var job protocol.Job
	request, err := c.newJSONRequest(ctx, http.MethodPost, path, input)
	if err != nil {
		return protocol.Job{}, err
	}
	request.Header.Set("Idempotency-Key", requestID())
	if err := c.do(request, &job); err != nil {
		return protocol.Job{}, err
	}
	return job, nil
}

func (c *Client) SubmitAndWait(
	ctx context.Context,
	path string,
	input any,
	progress func(protocol.JobEvent),
) ([]byte, error) {
	job, err := c.Submit(ctx, path, input)
	if err != nil {
		return nil, err
	}
	return c.Wait(ctx, job, progress)
}

func (c *Client) Wait(
	ctx context.Context,
	job protocol.Job,
	progress func(protocol.JobEvent),
) ([]byte, error) {
	after := int64(0)
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		var current protocol.Job
		path := job.StatusURL
		if after > 0 {
			path += "?after=" + fmt.Sprintf("%d", after)
		}
		if err := c.Get(ctx, path, &current); err != nil {
			if ctx.Err() != nil {
				_ = c.Cancel(context.Background(), job.ID)
				return nil, ctx.Err()
			}
			return nil, err
		}
		for _, event := range current.Events {
			if event.Sequence > after {
				after = event.Sequence
			}
			if progress != nil {
				progress(event)
			}
		}
		switch current.Status {
		case protocol.JobSucceeded:
			return c.GetRaw(ctx, current.ResultURL)
		case protocol.JobFailed:
			if current.Error == "" {
				current.Error = "API job failed"
			}
			return nil, errors.New(current.Error)
		case protocol.JobCancelled:
			return nil, errors.New("API job was cancelled")
		}
		select {
		case <-ctx.Done():
			_ = c.Cancel(context.Background(), job.ID)
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *Client) Get(ctx context.Context, path string, output any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve(path), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	return c.do(request, output)
}

func (c *Client) GetRaw(ctx context.Context, path string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.resolve(path), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, c.connectionError(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 128<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, decodeProblem(response.StatusCode, data)
	}
	return data, nil
}

func (c *Client) Post(ctx context.Context, path string, input, output any) error {
	request, err := c.newJSONRequest(ctx, http.MethodPost, path, input)
	if err != nil {
		return err
	}
	return c.do(request, output)
}

func (c *Client) Patch(ctx context.Context, path string, input, output any) error {
	request, err := c.newJSONRequest(ctx, http.MethodPatch, path, input)
	if err != nil {
		return err
	}
	return c.do(request, output)
}

func (c *Client) Cancel(ctx context.Context, id string) error {
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		c.resolve("/api/v1/jobs/"+url.PathEscape(id)),
		nil,
	)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	var job protocol.Job
	return c.do(request, &job)
}

func (c *Client) newJSONRequest(
	ctx context.Context,
	method,
	path string,
	input any,
) (*http.Request, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, c.resolve(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	return request, nil
}

func (c *Client) do(request *http.Request, output any) error {
	response, err := c.HTTP.Do(request)
	if err != nil {
		return c.connectionError(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 128<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return decodeProblem(response.StatusCode, data)
	}
	if output == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode API response: %w", err)
	}
	return nil
}

func (c *Client) resolve(path string) string {
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}
	return c.BaseURL + "/" + strings.TrimLeft(path, "/")
}

func (c *Client) connectionError(err error) error {
	return fmt.Errorf(
		"SEO Audit API is unavailable at %s: %w; start it with `seoaudit-api`",
		c.BaseURL,
		err,
	)
}

func decodeProblem(status int, data []byte) error {
	var value Problem
	if err := json.Unmarshal(data, &value); err == nil && value.Detail != "" {
		return value
	}
	detail := strings.TrimSpace(string(data))
	if detail == "" {
		detail = http.StatusText(status)
	}
	return Problem{
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
		Code:   "http_error",
	}
}

func requestID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(value)
}

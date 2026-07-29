package gsc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	defaultBaseURL  = "https://searchconsole.googleapis.com/webmasters/v3"
	defaultTokenURL = "https://oauth2.googleapis.com/token"
	maxRows         = 25000
	maxDays         = 480
)

type credentials struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RefreshToken string `json:"refresh_token"`
	TokenURI     string `json:"token_uri"`
}

type Client struct {
	HTTP        *http.Client
	BaseURL     string
	TokenURL    string
	AccessToken string
	Credentials credentials
	Now         func() time.Time
}

type searchAnalyticsResponse struct {
	Rows []struct {
		Keys        []string `json:"keys"`
		Clicks      float64  `json:"clicks"`
		Impressions float64  `json:"impressions"`
		CTR         float64  `json:"ctr"`
		Position    float64  `json:"position"`
	} `json:"rows"`
	ResponseAggregationType string `json:"responseAggregationType"`
}

func NewClient() (*Client, error) {
	client := newClient()
	if token := strings.TrimSpace(os.Getenv("GSC_ACCESS_TOKEN")); token != "" {
		client.AccessToken = token
		return client, nil
	}
	data, paths, err := readCredentials()
	if err != nil {
		return nil, fmt.Errorf("Search Console is not authenticated; set GSC_ACCESS_TOKEN or provide OAuth credentials at one of %v", paths)
	}
	if err := json.Unmarshal(data, &client.Credentials); err != nil {
		return nil, fmt.Errorf("parse Google OAuth credentials: %w", err)
	}
	if client.Credentials.ClientID == "" || client.Credentials.ClientSecret == "" || client.Credentials.RefreshToken == "" {
		return nil, errors.New("Google OAuth credentials must contain client_id, client_secret, and refresh_token")
	}
	if client.Credentials.TokenURI != "" {
		client.TokenURL = client.Credentials.TokenURI
	}
	return client, nil
}

func NewClientWithToken(token string) *Client {
	client := newClient()
	client.AccessToken = token
	return client
}

func newClient() *Client {
	return &Client{
		HTTP:     &http.Client{Timeout: 60 * time.Second},
		BaseURL:  defaultBaseURL,
		TokenURL: defaultTokenURL,
		Now:      time.Now,
	}
}

func (c *Client) QueryPerformance(ctx context.Context, options Options) (Report, error) {
	if options.SiteURL == "" {
		return Report{}, errors.New("Search Console property is required")
	}
	if options.Days <= 0 {
		options.Days = 28
	}
	if options.Days > maxDays {
		return Report{}, fmt.Errorf("Search Console lookback cannot exceed %d days", maxDays)
	}
	if options.Limit <= 0 {
		options.Limit = 250
	}
	if options.Limit > maxRows {
		return Report{}, fmt.Errorf("Search Console row limit cannot exceed %d", maxRows)
	}

	now := c.Now().UTC()
	end := now.AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -(options.Days - 1))
	report := Report{
		Enabled:     true,
		Source:      "Google Search Console",
		SiteURL:     options.SiteURL,
		StartDate:   start.Format(time.DateOnly),
		EndDate:     end.Format(time.DateOnly),
		RetrievedAt: now,
		RowLimit:    options.Limit,
	}
	if options.Progress != nil {
		options.Progress(fmt.Sprintf("Requesting up to %d query/page rows for %s from %s to %s", options.Limit, options.SiteURL, report.StartDate, report.EndDate))
	}

	payload := map[string]any{
		"startDate":  report.StartDate,
		"endDate":    report.EndDate,
		"dimensions": []string{"query", "page"},
		"rowLimit":   options.Limit,
		"dataState":  "final",
	}
	var response searchAnalyticsResponse
	target := fmt.Sprintf("%s/sites/%s/searchAnalytics/query", strings.TrimRight(c.BaseURL, "/"), url.PathEscape(options.SiteURL))
	if err := c.call(ctx, http.MethodPost, target, payload, &response); err != nil {
		return Report{}, err
	}
	report.Available = true
	report.AggregationType = response.ResponseAggregationType
	for _, row := range response.Rows {
		if len(row.Keys) < 2 {
			continue
		}
		report.QueryPages = append(report.QueryPages, QueryPageMetric{
			Query:       row.Keys[0],
			Page:        row.Keys[1],
			Clicks:      row.Clicks,
			Impressions: row.Impressions,
			CTR:         row.CTR,
			Position:    row.Position,
		})
	}
	analyze(&report)
	if options.Progress != nil {
		options.Progress(fmt.Sprintf("Received %d query/page rows with %.0f clicks and %.0f impressions in the returned dataset", report.Summary.Rows, report.Summary.ReturnedClicks, report.Summary.ReturnedImpressions))
	}
	return report, nil
}

func analyze(report *Report) {
	var weightedPosition float64
	overlaps := map[string]*QueryOverlap{}
	overlapPages := map[string]map[string]string{}
	for _, row := range report.QueryPages {
		report.Summary.ReturnedClicks += row.Clicks
		report.Summary.ReturnedImpressions += row.Impressions
		weightedPosition += row.Position * row.Impressions
		if row.Position >= 4 && row.Position <= 20 && row.Impressions > 0 {
			report.StrikingDistance = append(report.StrikingDistance, row)
		}
		key := strings.ToLower(strings.TrimSpace(row.Query))
		if key == "" || row.Page == "" {
			continue
		}
		if overlaps[key] == nil {
			overlaps[key] = &QueryOverlap{Query: row.Query}
			overlapPages[key] = map[string]string{}
		}
		overlaps[key].Clicks += row.Clicks
		overlaps[key].Impressions += row.Impressions
		overlapPages[key][comparablePageURL(row.Page)] = row.Page
	}
	report.Summary.Rows = len(report.QueryPages)
	if report.Summary.ReturnedImpressions > 0 {
		report.Summary.ReturnedCTR = report.Summary.ReturnedClicks / report.Summary.ReturnedImpressions
		report.Summary.WeightedPosition = weightedPosition / report.Summary.ReturnedImpressions
	}
	sort.Slice(report.StrikingDistance, func(i, j int) bool {
		if report.StrikingDistance[i].Impressions != report.StrikingDistance[j].Impressions {
			return report.StrikingDistance[i].Impressions > report.StrikingDistance[j].Impressions
		}
		return report.StrikingDistance[i].Clicks > report.StrikingDistance[j].Clicks
	})
	for key, overlap := range overlaps {
		if len(overlapPages[key]) < 2 {
			continue
		}
		for _, page := range overlapPages[key] {
			overlap.Pages = append(overlap.Pages, page)
		}
		sort.Strings(overlap.Pages)
		report.QueryOverlaps = append(report.QueryOverlaps, *overlap)
	}
	sort.Slice(report.QueryOverlaps, func(i, j int) bool {
		if report.QueryOverlaps[i].Impressions != report.QueryOverlaps[j].Impressions {
			return report.QueryOverlaps[i].Impressions > report.QueryOverlaps[j].Impressions
		}
		return report.QueryOverlaps[i].Query < report.QueryOverlaps[j].Query
	})
}

func comparablePageURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.Fragment = ""
	parsed.Host = strings.ToLower(parsed.Host)
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	if parsed.Path != "/" {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}
	return parsed.String()
}

func (c *Client) call(ctx context.Context, method, target string, payload any, output any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Search Console API returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	if err := json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("decode Search Console response: %w", err)
	}
	return nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	if c.AccessToken != "" {
		return c.AccessToken, nil
	}
	form := url.Values{
		"client_id":     {c.Credentials.ClientID},
		"client_secret": {c.Credentials.ClientSecret},
		"refresh_token": {c.Credentials.RefreshToken},
		"grant_type":    {"refresh_token"},
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Google OAuth token exchange returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(data)))
	}
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &token); err != nil {
		return "", fmt.Errorf("decode Google OAuth token: %w", err)
	}
	if token.AccessToken == "" {
		return "", errors.New("Google OAuth token response did not contain an access token")
	}
	c.AccessToken = token.AccessToken
	return c.AccessToken, nil
}

func readCredentials() ([]byte, []string, error) {
	paths := credentialPaths()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err == nil {
			return data, paths, nil
		}
	}
	return nil, paths, errors.New("no Google OAuth credentials found")
}

func credentialPaths() []string {
	var paths []string
	if path := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); path != "" {
		paths = append(paths, path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return paths
	}
	paths = append(paths,
		filepath.Join(home, ".config", "google-cli", "credentials.json"),
		filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"),
	)
	return paths
}

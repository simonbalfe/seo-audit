package dataforseo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.dataforseo.com/v3"
	DatasetCount   = 7
)

type Client struct {
	HTTP     *http.Client
	BaseURL  string
	Username string
	Password string
}

type apiEnvelope struct {
	StatusCode    int       `json:"status_code"`
	StatusMessage string    `json:"status_message"`
	Cost          float64   `json:"cost"`
	Tasks         []apiTask `json:"tasks"`
}

type apiTask struct {
	StatusCode    int             `json:"status_code"`
	StatusMessage string          `json:"status_message"`
	Cost          float64         `json:"cost"`
	Result        json.RawMessage `json:"result"`
}

type datasetResult struct {
	name  string
	cost  float64
	err   error
	apply func(*Report)
}

type datasetJob func(context.Context, Options) datasetResult

func NewClient() (*Client, error) {
	username := os.Getenv("DATAFORSEO_USERNAME")
	password := os.Getenv("DATAFORSEO_PASSWORD")
	if username == "" || password == "" {
		return nil, errors.New("DataForSEO is not authenticated; set DATAFORSEO_USERNAME and DATAFORSEO_PASSWORD")
	}
	return NewClientWithCredentials(username, password), nil
}

func NewClientWithCredentials(username, password string) *Client {
	return &Client{
		HTTP:     &http.Client{Timeout: 90 * time.Second},
		BaseURL:  defaultBaseURL,
		Username: username,
		Password: password,
	}
}

func (c *Client) Audit(ctx context.Context, options Options) Report {
	options = normalizeOptions(options)
	report := Report{
		Enabled:     true,
		Source:      "DataForSEO",
		Target:      options.Target,
		Location:    options.Location,
		Language:    options.Language,
		RetrievedAt: time.Now().UTC(),
	}
	jobs := []datasetJob{
		c.domainOverview,
		c.rankedKeywords,
		c.keywordIdeas,
		c.competitors,
		c.backlinkSummary,
		c.referringDomains,
		c.topBacklinks,
	}
	if options.Progress != nil {
		options.Progress("setup", fmt.Sprintf("Requesting %d external datasets for %s in %s (%s)", len(jobs), options.Target, options.Location, options.Language))
	}
	results := make(chan datasetResult, len(jobs))
	for _, job := range jobs {
		go func() {
			results <- job(ctx, options)
		}()
	}
	for range jobs {
		result := <-results
		report.CostUSD += result.cost
		if result.err != nil {
			report.Errors = append(report.Errors, DatasetError{Dataset: result.name, Message: result.err.Error()})
			if options.Progress != nil {
				options.Progress(result.name, "Failed: "+result.err.Error())
			}
			continue
		}
		result.apply(&report)
		report.SuccessfulCalls++
		if options.Progress != nil {
			options.Progress(result.name, fmt.Sprintf("Received successfully; provider cost $%.6f", result.cost))
		}
	}
	report.Available = report.SuccessfulCalls > 0
	report.KeywordIdeas = removeRankedKeywords(report.KeywordIdeas, report.RankedKeywords)
	return report
}

func normalizeOptions(options Options) Options {
	if options.Location == "" {
		options.Location = "United Kingdom"
	}
	if options.Language == "" {
		options.Language = "en"
	}
	if options.Limit <= 0 {
		options.Limit = 25
	}
	if options.Limit > 100 {
		options.Limit = 100
	}
	return options
}

func (c *Client) domainOverview(ctx context.Context, options Options) datasetResult {
	payload := localePayload(options)
	payload["target"] = options.Target
	results, cost, err := postTask[[]domainOverviewResult](ctx, c, "/dataforseo_labs/google/domain_rank_overview/live", payload)
	result := datasetResult{name: "domain-overview", cost: cost, err: err}
	if err == nil {
		result.apply = func(report *Report) {
			if len(results) == 0 || len(results[0].Items) == 0 {
				return
			}
			report.OrganicVisibility = organicMetrics(results[0].Items[0].Metrics.Organic)
		}
	}
	return result
}

func (c *Client) rankedKeywords(ctx context.Context, options Options) datasetResult {
	payload := localePayload(options)
	payload["target"] = options.Target
	payload["limit"] = options.Limit
	payload["order_by"] = []string{"keyword_data.keyword_info.search_volume,desc"}
	results, cost, err := postTask[[]itemsResult[rankedKeywordItem]](ctx, c, "/dataforseo_labs/google/ranked_keywords/live", payload)
	result := datasetResult{name: "ranked-keywords", cost: cost, err: err}
	if err == nil {
		result.apply = func(report *Report) {
			if len(results) == 0 {
				return
			}
			report.RankedKeywords = make([]RankedKeyword, 0, len(results[0].Items))
			for _, item := range results[0].Items {
				keyword := item.KeywordData
				serp := item.RankedSERPElement.SERPItem
				report.RankedKeywords = append(report.RankedKeywords, RankedKeyword{
					Keyword:          keyword.Keyword,
					Position:         roundedInt(serp.RankAbsolute),
					PreviousPosition: serp.RankChanges.PreviousRankAbsolute,
					URL:              serp.URL,
					SearchVolume:     roundedInt(keyword.KeywordInfo.SearchVolume),
					Difficulty:       keyword.KeywordProperties.KeywordDifficulty,
					CPC:              keyword.KeywordInfo.CPC,
					Intent:           keyword.SearchIntentInfo.MainIntent,
					EstimatedTraffic: serp.ETV,
					LastUpdated:      item.RankedSERPElement.LastUpdatedTime,
				})
			}
		}
	}
	return result
}

func (c *Client) keywordIdeas(ctx context.Context, options Options) datasetResult {
	payload := localePayload(options)
	payload["target"] = options.Target
	payload["limit"] = options.Limit
	payload["include_subdomains"] = true
	payload["filters"] = []any{"keyword_info.search_volume", ">", 0}
	results, cost, err := postTask[[]itemsResult[keywordData]](ctx, c, "/dataforseo_labs/google/keywords_for_site/live", payload)
	result := datasetResult{name: "keyword-ideas", cost: cost, err: err}
	if err == nil {
		result.apply = func(report *Report) {
			if len(results) == 0 {
				return
			}
			report.KeywordIdeas = make([]KeywordIdea, 0, len(results[0].Items))
			for _, keyword := range results[0].Items {
				report.KeywordIdeas = append(report.KeywordIdeas, KeywordIdea{
					Keyword:          keyword.Keyword,
					SearchVolume:     roundedInt(keyword.KeywordInfo.SearchVolume),
					Difficulty:       keyword.KeywordProperties.KeywordDifficulty,
					CPC:              keyword.KeywordInfo.CPC,
					Competition:      keyword.KeywordInfo.Competition,
					CompetitionLevel: keyword.KeywordInfo.CompetitionLevel,
					Intent:           keyword.SearchIntentInfo.MainIntent,
					LastUpdated:      keyword.KeywordInfo.LastUpdatedTime,
				})
			}
		}
	}
	return result
}

func (c *Client) competitors(ctx context.Context, options Options) datasetResult {
	payload := localePayload(options)
	payload["target"] = options.Target
	payload["limit"] = options.Limit
	results, cost, err := postTask[[]itemsResult[competitorItem]](ctx, c, "/dataforseo_labs/google/competitors_domain/live", payload)
	result := datasetResult{name: "competitors", cost: cost, err: err}
	if err == nil {
		result.apply = func(report *Report) {
			if len(results) == 0 {
				return
			}
			report.Competitors = make([]Competitor, 0, len(results[0].Items))
			for _, item := range results[0].Items {
				if strings.EqualFold(strings.TrimPrefix(item.Domain, "www."), strings.TrimPrefix(options.Target, "www.")) {
					continue
				}
				report.Competitors = append(report.Competitors, Competitor{
					Domain:           item.Domain,
					KeywordOverlap:   roundedInt(item.Intersections),
					AveragePosition:  item.AveragePosition,
					OrganicKeywords:  roundedInt(item.FullDomainMetrics.Organic.Count),
					EstimatedTraffic: item.FullDomainMetrics.Organic.ETV,
				})
			}
		}
	}
	return result
}

func (c *Client) backlinkSummary(ctx context.Context, options Options) datasetResult {
	payload := map[string]any{
		"target":             options.Target,
		"include_subdomains": true,
	}
	results, cost, err := postTask[[]backlinkSummaryItem](ctx, c, "/backlinks/summary/live", payload)
	result := datasetResult{name: "backlink-summary", cost: cost, err: err}
	if err == nil {
		result.apply = func(report *Report) {
			if len(results) == 0 {
				return
			}
			item := results[0]
			report.BacklinkSummary = BacklinkSummary{
				DataForSEORank:          item.Rank,
				TargetSpamScore:         item.Info.TargetSpamScore,
				Backlinks:               item.Backlinks,
				BacklinksSpamScore:      item.BacklinksSpamScore,
				ReferringDomains:        item.ReferringDomains,
				ReferringMainDomains:    item.ReferringMainDomains,
				ReferringPages:          item.ReferringPages,
				NofollowReferringPages:  item.ReferringPagesNofollow,
				ReferringIPs:            item.ReferringIPs,
				BrokenBacklinks:         item.BrokenBacklinks,
				BrokenPages:             item.BrokenPages,
				FirstSeen:               item.FirstSeen,
				CrawledPagesInLinkIndex: item.CrawledPages,
			}
		}
	}
	return result
}

func (c *Client) referringDomains(ctx context.Context, options Options) datasetResult {
	payload := map[string]any{
		"target":             options.Target,
		"include_subdomains": true,
		"limit":              options.Limit,
		"order_by":           []string{"rank,desc"},
	}
	results, cost, err := postTask[[]itemsResult[referringDomainItem]](ctx, c, "/backlinks/referring_domains/live", payload)
	result := datasetResult{name: "referring-domains", cost: cost, err: err}
	if err == nil {
		result.apply = func(report *Report) {
			if len(results) == 0 {
				return
			}
			report.ReferringDomains = make([]ReferringDomain, 0, len(results[0].Items))
			for _, item := range results[0].Items {
				report.ReferringDomains = append(report.ReferringDomains, ReferringDomain{
					Domain:                 item.Domain,
					DataForSEORank:         item.Rank,
					Backlinks:              item.Backlinks,
					BacklinksSpamScore:     item.BacklinksSpamScore,
					ReferringPages:         item.ReferringPages,
					NofollowReferringPages: item.ReferringPagesNofollow,
					FirstSeen:              item.FirstSeen,
				})
			}
		}
	}
	return result
}

func (c *Client) topBacklinks(ctx context.Context, options Options) datasetResult {
	payload := map[string]any{
		"target":                     options.Target,
		"include_subdomains":         true,
		"exclude_internal_backlinks": true,
		"backlinks_status_type":      "live",
		"limit":                      options.Limit,
		"order_by":                   []string{"rank,desc"},
	}
	results, cost, err := postTask[[]itemsResult[backlinkItem]](ctx, c, "/backlinks/backlinks/live", payload)
	result := datasetResult{name: "top-backlinks", cost: cost, err: err}
	if err == nil {
		result.apply = func(report *Report) {
			if len(results) == 0 {
				return
			}
			report.TopBacklinks = make([]Backlink, 0, len(results[0].Items))
			for _, item := range results[0].Items {
				report.TopBacklinks = append(report.TopBacklinks, Backlink{
					SourceURL:          item.URLFrom,
					SourceDomain:       item.DomainFrom,
					TargetURL:          item.URLTo,
					Anchor:             item.Anchor,
					Dofollow:           item.Dofollow,
					LinkRank:           item.Rank,
					SourceDomainRank:   item.DomainFromRank,
					BacklinkSpamScore:  item.BacklinkSpamScore,
					SourcePageStatus:   item.PageFromStatusCode,
					TargetPageStatus:   item.URLToStatusCode,
					SourcePageTitle:    item.PageFromTitle,
					SourcePageLanguage: item.PageFromLanguage,
					SemanticLocation:   item.SemanticLocation,
					FirstSeen:          item.FirstSeen,
					LastSeen:           item.LastSeen,
					New:                item.IsNew,
					Lost:               item.IsLost,
					Broken:             item.IsBroken,
				})
			}
		}
	}
	return result
}

func postTask[T any](ctx context.Context, client *Client, path string, payload map[string]any) (T, float64, error) {
	var zero T
	body, err := json.Marshal([]map[string]any{payload})
	if err != nil {
		return zero, 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(client.BaseURL, "/")+"/"+strings.TrimLeft(path, "/"), bytes.NewReader(body))
	if err != nil {
		return zero, 0, err
	}
	request.SetBasicAuth(client.Username, client.Password)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.HTTP.Do(request)
	if err != nil {
		return zero, 0, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return zero, 0, err
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return zero, 0, fmt.Errorf("decode response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return zero, envelope.Cost, fmt.Errorf("HTTP %d: %s", response.StatusCode, envelope.StatusMessage)
	}
	if envelope.StatusCode != 20000 {
		return zero, envelope.Cost, fmt.Errorf("DataForSEO %d: %s", envelope.StatusCode, envelope.StatusMessage)
	}
	if len(envelope.Tasks) == 0 {
		return zero, envelope.Cost, errors.New("DataForSEO returned no task")
	}
	task := envelope.Tasks[0]
	cost := task.Cost
	if cost == 0 {
		cost = envelope.Cost
	}
	if task.StatusCode != 20000 {
		return zero, cost, fmt.Errorf("DataForSEO %d: %s", task.StatusCode, task.StatusMessage)
	}
	var result T
	if err := json.Unmarshal(task.Result, &result); err != nil {
		return zero, cost, fmt.Errorf("decode task result: %w", err)
	}
	return result, cost, nil
}

func localePayload(options Options) map[string]any {
	return map[string]any{
		"location_name": options.Location,
		"language_code": options.Language,
	}
}

func roundedInt(value float64) int {
	return int(math.Round(value))
}

func organicMetrics(metrics organicMetricItem) OrganicMetrics {
	return OrganicMetrics{
		Keywords:             roundedInt(metrics.Count),
		EstimatedTraffic:     metrics.ETV,
		EstimatedTrafficCost: metrics.EstimatedPaidTrafficCost,
		Position1:            roundedInt(metrics.Position1),
		Positions2To3:        roundedInt(metrics.Positions2To3),
		Positions4To10:       roundedInt(metrics.Positions4To10),
		Positions11To20:      roundedInt(metrics.Positions11To20),
		Positions21To100: roundedInt(
			metrics.Positions21To30 +
				metrics.Positions31To40 +
				metrics.Positions41To50 +
				metrics.Positions51To60 +
				metrics.Positions61To70 +
				metrics.Positions71To80 +
				metrics.Positions81To90 +
				metrics.Positions91To100,
		),
		New:  roundedInt(metrics.New),
		Up:   roundedInt(metrics.Up),
		Down: roundedInt(metrics.Down),
		Lost: roundedInt(metrics.Lost),
	}
}

func removeRankedKeywords(ideas []KeywordIdea, ranked []RankedKeyword) []KeywordIdea {
	existing := make(map[string]bool, len(ranked))
	for _, keyword := range ranked {
		existing[strings.ToLower(strings.TrimSpace(keyword.Keyword))] = true
	}
	filtered := make([]KeywordIdea, 0, len(ideas))
	for _, idea := range ideas {
		if !existing[strings.ToLower(strings.TrimSpace(idea.Keyword))] {
			filtered = append(filtered, idea)
		}
	}
	return filtered
}

package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/simonbalfe/seo-audit/internal/report"
)

const (
	openRouterEndpoint          = "https://openrouter.ai/api/v1/chat/completions"
	openRouterModel             = "google/gemini-2.5-flash-lite"
	classificationPrompt        = "Route public website pages for local visibility research. Use exactly one type: home, service, location, service-location, product-category, blog, team-about, contact-legal-utility, unknown. Service-location means one page targets both a service and a named geographic area. For home, service, location, service-location, and product-category pages, return up to three concise non-branded commercial phrases that real customers would type and that the supplied metadata clearly supports. Prefer established service or category terms. Do not use verbs such as find, vague umbrella phrases such as services or treatments, or invented locations. For other page types return an empty keyword_seeds array. Do not assess SEO quality, invent services, or create pass/fail findings. Return every input id once with a short evidence-based reason."
	classificationPromptVersion = "2"
	classificationBatchSize     = 100
	classificationWorkers       = 4
)

var pageTypes = []string{
	"home",
	"service",
	"location",
	"service-location",
	"product-category",
	"blog",
	"team-about",
	"contact-legal-utility",
	"unknown",
}

type pageClassifier struct {
	HTTP     *http.Client
	Endpoint string
	APIKey   string
}

type classificationInput struct {
	ID          int      `json:"id"`
	URL         string   `json:"url"`
	Title       string   `json:"title,omitempty"`
	H1          []string `json:"h1,omitempty"`
	SchemaTypes []string `json:"schema_types,omitempty"`
	HasBooking  bool     `json:"has_booking"`
	HasPhone    bool     `json:"has_phone"`
}

type classification struct {
	ID           int      `json:"id"`
	Type         string   `json:"type"`
	Reason       string   `json:"reason"`
	KeywordSeeds []string `json:"keyword_seeds"`
}

type classificationUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	Cost             float64 `json:"cost"`
}

type classificationResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage classificationUsage `json:"usage"`
}

type classificationBatchResult struct {
	Pages []classification `json:"pages"`
	Usage classificationUsage
	Err   error
}

func classifyPages(ctx context.Context, site *SiteReport, options Options) {
	result := report.PageClassificationReport{Counts: make(map[string]int)}
	unknown := make([]classificationInput, 0, len(site.Pages))
	for index := range site.Pages {
		page := &site.Pages[index]
		page.PageType = "unknown"
		page.PageTypeSource = ""
		page.PageTypeReason = ""
		unknown = append(unknown, classificationPage(index, *page))
	}
	if len(unknown) > 0 {
		result.Model = openRouterModel
		var cache *classificationCache
		if options.ClassificationCachePath != "" {
			var err error
			cache, err = openClassificationCache(ctx, options.ClassificationCachePath)
			if err != nil {
				result.Errors = append(result.Errors, err.Error())
			} else {
				emitProgress(options, "visibility", "Using cached page research from %s", options.ClassificationCachePath)
				misses := make([]classificationInput, 0, len(unknown))
				for index, input := range unknown {
					cached, found, err := cache.load(ctx, input)
					if err != nil {
						result.Errors = append(result.Errors, err.Error())
						misses = append(misses, unknown[index:]...)
						break
					}
					if !found || !validPageType(cached.Type) {
						misses = append(misses, input)
						continue
					}
					site.Pages[cached.ID].PageType = cached.Type
					site.Pages[cached.ID].PageTypeSource = "openrouter-cache"
					site.Pages[cached.ID].PageTypeReason = cached.Reason
					site.Pages[cached.ID].KeywordSeeds = cached.KeywordSeeds
					result.AIClassified++
					result.CacheHits++
				}
				unknown = misses
				emitProgress(options, "visibility", "Reused %d cached pages; %d pages need OpenRouter research", result.CacheHits, len(unknown))
			}
		}
		classifier := pageClassifier{
			HTTP:     &http.Client{Timeout: 90 * time.Second},
			Endpoint: openRouterEndpoint,
			APIKey:   strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		}
		if len(unknown) > 0 {
			emitProgress(options, "visibility", "Identifying commercial pages and keyword seeds for %d pages with OpenRouter", len(unknown))
			applyAIClassifications(ctx, &classifier, cache, site.Pages, unknown, &result)
		}
		if cache != nil {
			if err := cache.close(); err != nil {
				result.Errors = append(result.Errors, err.Error())
			}
		}
	}
	for index := range site.Pages {
		page := &site.Pages[index]
		page.PriorityPage = page.Indexable && commercialPageType(page.PageType)
		if !page.PriorityPage {
			page.KeywordSeeds = nil
		} else {
			page.KeywordSeeds = normalizeKeywordSeeds(page.KeywordSeeds)
		}
		result.Counts[page.PageType]++
		if page.PriorityPage {
			result.PriorityPages++
		}
		if page.PageType == "unknown" {
			result.Unknown++
		}
	}
	sort.Strings(result.Errors)
	site.PageClassification = result
}

func classificationPage(index int, page report.PageReport) classificationInput {
	target := page.FinalURL
	if target == "" {
		target = page.URL
	}
	schemaTypes := append([]string(nil), page.SchemaTypes...)
	sort.Strings(schemaTypes)
	schemaTypes = truncateStrings(schemaTypes, 10, 80)
	return classificationInput{
		ID:          index,
		URL:         target,
		Title:       truncate(page.Title, 240),
		H1:          truncateStrings(page.H1, 3, 240),
		SchemaTypes: schemaTypes,
		HasBooking:  len(page.BookingLinks) > 0,
		HasPhone:    len(page.PhoneLinks) > 0,
	}
}

func applyAIClassifications(ctx context.Context, classifier *pageClassifier, cache *classificationCache, pages []report.PageReport, unknown []classificationInput, report *report.PageClassificationReport) {
	batches := make([][]classificationInput, 0, (len(unknown)+classificationBatchSize-1)/classificationBatchSize)
	for start := 0; start < len(unknown); start += classificationBatchSize {
		end := min(start+classificationBatchSize, len(unknown))
		batches = append(batches, unknown[start:end])
	}
	report.Requests = len(batches)
	results := make(chan classificationBatchResult, len(batches))
	semaphore := make(chan struct{}, classificationWorkers)
	var workers sync.WaitGroup
	for _, batch := range batches {
		workers.Add(1)
		go func() {
			defer workers.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			classified, usage, err := classifier.classify(ctx, batch)
			results <- classificationBatchResult{Pages: classified, Usage: usage, Err: err}
		}()
	}
	workers.Wait()
	close(results)
	inputs := make(map[int]classificationInput, len(unknown))
	for _, input := range unknown {
		inputs[input.ID] = input
	}
	fresh := make([]classification, 0, len(unknown))
	for batch := range results {
		report.PromptTokens += batch.Usage.PromptTokens
		report.CompletionTokens += batch.Usage.CompletionTokens
		report.CostUSD += batch.Usage.Cost
		if batch.Err != nil {
			report.Errors = append(report.Errors, batch.Err.Error())
			continue
		}
		for _, classified := range batch.Pages {
			if classified.ID < 0 || classified.ID >= len(pages) || !validPageType(classified.Type) || pages[classified.ID].PageType != "unknown" {
				continue
			}
			classified.Reason = truncate(strings.Join(strings.Fields(classified.Reason), " "), 160)
			pages[classified.ID].PageType = classified.Type
			pages[classified.ID].PageTypeSource = "openrouter"
			pages[classified.ID].PageTypeReason = classified.Reason
			pages[classified.ID].KeywordSeeds = normalizeKeywordSeeds(classified.KeywordSeeds)
			report.AIClassified++
			fresh = append(fresh, classified)
		}
	}
	if cache != nil {
		if err := cache.save(ctx, inputs, fresh); err != nil {
			report.Errors = append(report.Errors, err.Error())
		}
	}
}

func validPageType(value string) bool {
	for _, pageType := range pageTypes {
		if value == pageType {
			return true
		}
	}
	return false
}

func commercialPageType(value string) bool {
	switch value {
	case "home", "service", "location", "service-location", "product-category":
		return true
	default:
		return false
	}
}

func normalizeKeywordSeeds(values []string) []string {
	result := make([]string, 0, min(len(values), 3))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.Join(strings.Fields(value), " "))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, truncate(value, 80))
		if len(result) == 3 {
			break
		}
	}
	return result
}

func (c *pageClassifier) classify(ctx context.Context, pages []classificationInput) ([]classification, classificationUsage, error) {
	if c.APIKey == "" {
		return nil, classificationUsage{}, errors.New("OpenRouter is not authenticated; set OPENROUTER_API_KEY")
	}
	pageJSON, err := json.Marshal(pages)
	if err != nil {
		return nil, classificationUsage{}, fmt.Errorf("encode page metadata: %w", err)
	}
	requestBody := map[string]any{
		"model": openRouterModel,
		"messages": []map[string]string{
			{"role": "system", "content": classificationPrompt},
			{"role": "user", "content": string(pageJSON)},
		},
		"temperature": 0,
		"max_tokens":  5000,
		"provider":    map[string]bool{"require_parameters": true},
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "page_classifications",
				"strict": true,
				"schema": map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"pages": map[string]any{
							"type": "array",
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"properties": map[string]any{
									"id":     map[string]any{"type": "integer"},
									"type":   map[string]any{"type": "string", "enum": pageTypes},
									"reason": map[string]any{"type": "string"},
									"keyword_seeds": map[string]any{
										"type":     "array",
										"maxItems": 3,
										"items":    map[string]any{"type": "string"},
									},
								},
								"required": []string{"id", "type", "reason", "keyword_seeds"},
							},
						},
					},
					"required": []string{"pages"},
				},
			},
		},
	}
	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, classificationUsage{}, fmt.Errorf("encode OpenRouter request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, classificationUsage{}, fmt.Errorf("create OpenRouter request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.APIKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Title", "SEO Audit")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, classificationUsage{}, fmt.Errorf("classify pages with OpenRouter: %w", err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, classificationUsage{}, fmt.Errorf("read OpenRouter response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, classificationUsage{}, fmt.Errorf("OpenRouter returned %s: %s", response.Status, truncate(strings.TrimSpace(string(responseBody)), 500))
	}
	var completion classificationResponse
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return nil, classificationUsage{}, fmt.Errorf("decode OpenRouter response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, completion.Usage, errors.New("OpenRouter returned no classification")
	}
	var result struct {
		Pages []classification `json:"pages"`
	}
	if err := json.Unmarshal([]byte(completion.Choices[0].Message.Content), &result); err != nil {
		return nil, completion.Usage, fmt.Errorf("decode OpenRouter classification: %w", err)
	}
	return result.Pages, completion.Usage, nil
}

func truncate(value string, limit int) string {
	characters := []rune(value)
	if len(characters) <= limit {
		return value
	}
	return string(characters[:limit])
}

func truncateStrings(values []string, count, length int) []string {
	if len(values) > count {
		values = values[:count]
	}
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = truncate(value, length)
	}
	return result
}

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
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/simonbalfe/seo-audit/internal/report"
)

const (
	defaultBaseURL   = "https://api.dataforseo.com/v3"
	gridKeywordLimit = 5
)

// Options bounds one local market scan.
type Options struct {
	Target          string
	Location        string
	Language        string
	MaxChecks       int
	Keywords        []string
	AIKeywords      []string
	TargetName      string
	TargetCategory  string
	TargetCountry   string
	TargetPlaceID   string
	TargetLatitude  float64
	TargetLongitude float64
	GridRadiusKM    float64
	Progress        func(string)
}

// Client calls the synchronous DataForSEO API.
type Client struct {
	HTTP     *http.Client
	BaseURL  string
	Username string
	Password string
	limiter  chan struct{}
}

type apiEnvelope struct {
	StatusCode    int       `json:"status_code"`
	StatusMessage string    `json:"status_message"`
	Tasks         []apiTask `json:"tasks"`
}

type apiTask struct {
	StatusCode    int             `json:"status_code"`
	StatusMessage string          `json:"status_message"`
	Cost          float64         `json:"cost"`
	Result        json.RawMessage `json:"result"`
}

type rankedResult struct {
	Items []rankedItem `json:"items"`
}

type rankedItem struct {
	KeywordData struct {
		Keyword     string `json:"keyword"`
		KeywordInfo struct {
			SearchVolume int     `json:"search_volume"`
			CPC          float64 `json:"cpc"`
		} `json:"keyword_info"`
	} `json:"keyword_data"`
	RankedSERPElement struct {
		SERPItem struct {
			RankAbsolute int    `json:"rank_absolute"`
			URL          string `json:"url"`
		} `json:"serp_item"`
	} `json:"ranked_serp_element"`
}

type volumeItem struct {
	Keyword      string  `json:"keyword"`
	SearchVolume int     `json:"search_volume"`
	CPC          float64 `json:"cpc"`
}

type serpResult struct {
	Items []serpItem `json:"items"`
}

type serpItem struct {
	Type         string `json:"type"`
	RankAbsolute int    `json:"rank_absolute"`
	Domain       string `json:"domain"`
	Title        string `json:"title"`
	URL          string `json:"url"`
}

type mapsResult struct {
	Items []mapsItem `json:"items"`
}

type mapsItem struct {
	Type         string  `json:"type"`
	RankGroup    int     `json:"rank_group"`
	RankAbsolute int     `json:"rank_absolute"`
	Domain       string  `json:"domain"`
	Title        string  `json:"title"`
	URL          string  `json:"url"`
	Address      string  `json:"address"`
	PlaceID      string  `json:"place_id"`
	Category     string  `json:"category"`
	Latitude     float64 `json:"latitude"`
	Longitude    float64 `json:"longitude"`
	Rating       *struct {
		Value      float64 `json:"value"`
		VotesCount int     `json:"votes_count"`
	} `json:"rating"`
}

type candidate struct {
	keyword      string
	url          string
	targetURL    string
	position     int
	searchVolume int
	cpc          float64
	explicit     bool
	source       string
	relevance    int
}

// NewClient loads DataForSEO API credentials from the environment.
func NewClient() (*Client, error) {
	username := strings.TrimSpace(os.Getenv("DATAFORSEO_USERNAME"))
	password := strings.TrimSpace(os.Getenv("DATAFORSEO_PASSWORD"))
	if username == "" || password == "" {
		return nil, errors.New("DataForSEO is not authenticated; set DATAFORSEO_USERNAME and DATAFORSEO_PASSWORD")
	}
	return NewClientWithCredentials(username, password), nil
}

// NewClientWithCredentials constructs a DataForSEO client.
func NewClientWithCredentials(username, password string) *Client {
	return &Client{
		HTTP:     &http.Client{Timeout: 90 * time.Second},
		BaseURL:  defaultBaseURL,
		Username: username,
		Password: password,
		limiter:  make(chan struct{}, 20),
	}
}

// Scan finds and verifies a bounded shortlist of local organic opportunities.
func (c *Client) Scan(ctx context.Context, pages []report.PageReport, options Options) report.MarketReport {
	options = normalizeOptions(options)
	result := report.MarketReport{
		Enabled:           true,
		Source:            "DataForSEO",
		Target:            options.Target,
		Location:          options.Location,
		Language:          options.Language,
		RetrievedAt:       time.Now().UTC().Format(time.RFC3339),
		MaxChecks:         options.MaxChecks,
		CurrentVisibility: make([]report.Opportunity, 0),
		CurrentMaps:       make([]report.MapsVisibility, 0),
		Opportunities:     make([]report.Opportunity, 0),
		OpportunityMaps:   make([]report.MapsVisibility, 0),
	}

	progress(options, "Pulling the domain's existing organic rankings")
	ranked, cost, live, err := c.ranked(ctx, options)
	if live {
		result.LiveCalls++
	}
	result.CostUSD += cost
	if err != nil {
		result.Errors = append(result.Errors, report.MarketError{Operation: "ranked-keywords", Message: err.Error()})
	}
	result.ExistingRankingsLocation = options.TargetCountry
	if result.ExistingRankingsLocation == "" {
		result.ExistingRankingsLocation = countryLocation(options.Location)
	}
	result.ExistingRankings = existingRankings(ranked)
	mapsEnabled := options.TargetPlaceID != "" && (options.TargetLatitude != 0 || options.TargetLongitude != 0)
	if !mapsEnabled {
		result.Errors = append(result.Errors, report.MarketError{Operation: "maps", Message: "exact Maps visibility requires public coordinates from the Place ID"})
	}
	pageByURL := indexPages(pages)
	currentShortlist := shortlistCandidates(assignCandidatePages(rankedCandidates(ranked), pages), options.MaxChecks)
	if len(currentShortlist) > 0 {
		progress(options, fmt.Sprintf("Checking %d existing ranking queries in local organic Search and Maps", len(currentShortlist)))
	}
	checkOptions := options
	checkOptions.GridRadiusKM = 0
	for _, check := range c.checkCandidates(ctx, currentShortlist, pageByURL, mapsEnabled, checkOptions) {
		result.LiveCalls += check.liveCalls
		result.CostUSD += check.cost
		result.Errors = append(result.Errors, check.errors...)
		result.CurrentVisibility = append(result.CurrentVisibility, check.opportunity)
		if check.maps != nil {
			result.CurrentMaps = append(result.CurrentMaps, *check.maps)
		}
	}

	progress(options, "Discovering new local commercial keyword opportunities")
	ideas, ideaCost, ideaLive, ideaErr := c.siteKeywords(ctx, options)
	if ideaLive {
		result.LiveCalls++
	}
	result.CostUSD += ideaCost
	if ideaErr != nil {
		result.Errors = append(result.Errors, report.MarketError{Operation: "keyword-discovery", Message: ideaErr.Error()})
	}
	candidates := siteCandidates(ideas, pages, options)

	explicit := normalizeKeywords(options.Keywords)
	aiKeywords := normalizeKeywords(options.AIKeywords)
	volumeKeywords := normalizeKeywords(append(append([]string{}, explicit...), aiKeywords...))
	if len(volumeKeywords) > 0 {
		progress(options, fmt.Sprintf("Validating demand for %d supplied and OpenRouter keyword seeds", len(volumeKeywords)))
		volumes, volumeCost, volumeLive, volumeErr := c.volumes(ctx, volumeKeywords, options)
		if volumeLive {
			result.LiveCalls++
		}
		result.CostUSD += volumeCost
		if volumeErr != nil {
			result.Errors = append(result.Errors, report.MarketError{Operation: "search-volume", Message: volumeErr.Error()})
		}
		candidates = append(candidates, generatedCandidates(aiKeywords, volumes)...)
		candidates = append(candidates, explicitCandidates(explicit, volumes)...)
	}

	candidates = removeExistingCandidates(assignCandidatePages(candidates, pages), result.ExistingRankings)
	result.KeywordIdeas = uniqueCandidateCount(candidates)
	shortlist := shortlistCandidates(candidates, options.MaxChecks)
	if len(shortlist) > 0 {
		progress(options, fmt.Sprintf("Checking %d new opportunity queries in local organic Search and Maps", len(shortlist)))
	}
	for _, check := range c.checkCandidates(ctx, shortlist, pageByURL, mapsEnabled, checkOptions) {
		result.LiveCalls += check.liveCalls
		result.CostUSD += check.cost
		result.Errors = append(result.Errors, check.errors...)
		result.Opportunities = append(result.Opportunities, check.opportunity)
		if check.maps != nil {
			result.OpportunityMaps = append(result.OpportunityMaps, *check.maps)
		}
	}
	if mapsEnabled && options.GridRadiusKM > 0 {
		selected := selectGridCandidates(currentShortlist, shortlist, options, gridKeywordLimit)
		mapsByKeyword := make(map[string]*report.MapsVisibility, len(result.CurrentMaps)+len(result.OpportunityMaps))
		for index := range result.CurrentMaps {
			mapsByKeyword[result.CurrentMaps[index].Keyword] = &result.CurrentMaps[index]
		}
		for index := range result.OpportunityMaps {
			mapsByKeyword[result.OpportunityMaps[index].Keyword] = &result.OpportunityMaps[index]
		}
		gridCandidates := make([]candidate, 0, len(selected))
		for _, item := range selected {
			if mapsByKeyword[item.keyword] != nil {
				gridCandidates = append(gridCandidates, item)
				result.GridKeywords = append(result.GridKeywords, item.keyword)
			}
		}
		if len(gridCandidates) > 0 {
			progress(options, fmt.Sprintf("Running 3x3 Maps grids for %d selected commercial keywords", len(gridCandidates)))
		}
		for _, check := range c.checkGrids(ctx, gridCandidates, mapsByKeyword, options) {
			result.LiveCalls += check.liveCalls
			result.CostUSD += check.cost
			result.Errors = append(result.Errors, check.errors...)
			updateGridResult(result.CurrentVisibility, result.CurrentMaps, check.item, pageByURL, check.points, options.GridRadiusKM)
			updateGridResult(result.Opportunities, result.OpportunityMaps, check.item, pageByURL, check.points, options.GridRadiusKM)
		}
	}

	sort.Slice(result.CurrentVisibility, func(i, j int) bool {
		return result.CurrentVisibility[i].CountryPosition < result.CurrentVisibility[j].CountryPosition
	})
	sort.Slice(result.Opportunities, func(i, j int) bool {
		left := result.Opportunities[i]
		right := result.Opportunities[j]
		priorities := map[string]int{"high": 0, "medium": 1, "low": 2}
		if priorities[left.Priority] != priorities[right.Priority] {
			return priorities[left.Priority] < priorities[right.Priority]
		}
		if left.PriorityRatio != right.PriorityRatio {
			return left.PriorityRatio > right.PriorityRatio
		}
		if left.SearchVolume != right.SearchVolume {
			return left.SearchVolume > right.SearchVolume
		}
		return left.Keyword < right.Keyword
	})
	progress(options, fmt.Sprintf("Completed %d current-visibility and %d opportunity checks at provider cost $%.6f", len(result.CurrentVisibility), len(result.Opportunities), result.CostUSD))
	return result
}

func (c *Client) checkCandidates(ctx context.Context, shortlist []candidate, pages map[string]*report.PageReport, mapsEnabled bool, options Options) []keywordCheck {
	checks := make(chan keywordCheck, len(shortlist))
	var wait sync.WaitGroup
	for _, item := range shortlist {
		wait.Add(1)
		go func(item candidate) {
			defer wait.Done()
			checks <- c.checkKeyword(ctx, item, pages, mapsEnabled, options)
		}(item)
	}
	wait.Wait()
	close(checks)
	result := make([]keywordCheck, 0, len(shortlist))
	for check := range checks {
		result = append(result, check)
	}
	return result
}

type gridCheck struct {
	item      candidate
	points    []report.GeoRankPoint
	errors    []report.MarketError
	liveCalls int
	cost      float64
}

func (c *Client) checkGrids(ctx context.Context, items []candidate, snapshots map[string]*report.MapsVisibility, options Options) []gridCheck {
	checks := make(chan gridCheck, len(items))
	var wait sync.WaitGroup
	for _, item := range items {
		wait.Add(1)
		go func(item candidate) {
			defer wait.Done()
			points, cost, liveCalls, gridErrors := c.grid(ctx, item.keyword, *snapshots[item.keyword], options)
			check := gridCheck{item: item, points: points, liveCalls: liveCalls, cost: cost}
			if len(gridErrors) > 0 {
				check.errors = append(check.errors, report.MarketError{
					Operation: "maps-grid:" + item.keyword,
					Message:   fmt.Sprintf("%d of 8 non-center grid points failed; first error: %v", len(gridErrors), gridErrors[0]),
				})
			}
			checks <- check
		}(item)
	}
	wait.Wait()
	close(checks)
	result := make([]gridCheck, 0, len(items))
	for check := range checks {
		result = append(result, check)
	}
	return result
}

func updateGridResult(opportunities []report.Opportunity, snapshots []report.MapsVisibility, item candidate, pages map[string]*report.PageReport, points []report.GeoRankPoint, radiusKM float64) {
	var snapshot *report.MapsVisibility
	for index := range snapshots {
		if snapshots[index].Keyword == item.keyword {
			snapshot = &snapshots[index]
			break
		}
	}
	if snapshot == nil {
		return
	}
	snapshot.GridRadiusKM = radiusKM
	snapshot.GridPoints = points
	summarizeGrid(snapshot)
	for index := range opportunities {
		if opportunities[index].Keyword != item.keyword {
			continue
		}
		countryPosition := opportunities[index].CountryPosition
		item.position = opportunities[index].Position
		item.url = opportunities[index].URL
		page := pages[comparableURL(item.targetURL)]
		if page == nil {
			page = pages[comparableURL(item.url)]
		}
		opportunities[index] = opportunity(item, page, snapshot)
		opportunities[index].CountryPosition = countryPosition
		return
	}
}

type keywordCheck struct {
	opportunity report.Opportunity
	maps        *report.MapsVisibility
	errors      []report.MarketError
	liveCalls   int
	cost        float64
}

func (c *Client) checkKeyword(ctx context.Context, item candidate, pages map[string]*report.PageReport, mapsEnabled bool, options Options) keywordCheck {
	result := keywordCheck{}
	countryPosition := 0
	if item.source == "current-ranking" {
		countryPosition = item.position
	}
	match, cost, live, err := c.serp(ctx, item.keyword, item.position > 10 || item.position == 0, options)
	result.cost += cost
	if live {
		result.liveCalls++
	}
	if err != nil {
		result.errors = append(result.errors, report.MarketError{Operation: "serp:" + item.keyword, Message: err.Error()})
	} else if match.URL == "" {
		item.position = 0
		item.url = ""
	} else {
		item.url = match.URL
		item.position = match.RankAbsolute
	}
	if mapsEnabled {
		snapshot, mapsCost, mapsLive, mapsErr := c.maps(ctx, item.keyword, options)
		result.cost += mapsCost
		if mapsLive {
			result.liveCalls++
		}
		if mapsErr != nil {
			result.errors = append(result.errors, report.MarketError{Operation: "maps:" + item.keyword, Message: mapsErr.Error()})
		} else {
			result.maps = &snapshot
		}
	}
	page := pages[comparableURL(item.targetURL)]
	if page == nil {
		page = pages[comparableURL(item.url)]
	}
	result.opportunity = opportunity(item, page, result.maps)
	result.opportunity.CountryPosition = countryPosition
	return result
}

func (c *Client) siteKeywords(ctx context.Context, options Options) ([]volumeItem, float64, bool, error) {
	payload := map[string]any{
		"target":        options.Target,
		"target_type":   "site",
		"language_code": options.Language,
		"sort_by":       "relevance",
	}
	setLocalLocation(payload, options)
	var result []volumeItem
	cost, live, err := c.postTask(ctx, "/keywords_data/google_ads/keywords_for_site/live", payload, &result)
	return result, cost, live, err
}

func (c *Client) maps(ctx context.Context, keyword string, options Options) (report.MapsVisibility, float64, bool, error) {
	return c.mapsAt(ctx, keyword, options.TargetLatitude, options.TargetLongitude, options)
}

func (c *Client) mapsAt(ctx context.Context, keyword string, latitude, longitude float64, options Options) (report.MapsVisibility, float64, bool, error) {
	const zoom = 15
	payload := map[string]any{
		"keyword":             keyword,
		"location_coordinate": fmt.Sprintf("%.7f,%.7f,%dz", latitude, longitude, zoom),
		"language_code":       options.Language,
		"device":              "mobile",
		"depth":               20,
		"search_this_area":    true,
		"search_places":       false,
	}
	var result []mapsResult
	cost, live, err := c.postTask(ctx, "/serp/google/maps/live/advanced", payload, &result)
	snapshot := report.MapsVisibility{
		Keyword:         keyword,
		Device:          "mobile",
		CenterLatitude:  latitude,
		CenterLongitude: longitude,
		Zoom:            zoom,
		TargetPlaceID:   options.TargetPlaceID,
		Results:         make([]report.LocalSearchResult, 0),
	}
	if err != nil || len(result) == 0 {
		return snapshot, cost, live, err
	}
	for _, item := range result[0].Items {
		if item.Type != "maps_search" {
			continue
		}
		position := item.RankGroup
		if position == 0 {
			position = item.RankAbsolute
		}
		local := report.LocalSearchResult{
			Position:  position,
			PlaceID:   item.PlaceID,
			Name:      item.Title,
			Category:  item.Category,
			Address:   item.Address,
			Domain:    item.Domain,
			URL:       item.URL,
			Latitude:  item.Latitude,
			Longitude: item.Longitude,
			IsTarget:  item.PlaceID == options.TargetPlaceID,
		}
		if item.Rating != nil {
			local.Rating = item.Rating.Value
			local.ReviewCount = item.Rating.VotesCount
		}
		if local.IsTarget {
			snapshot.TargetPosition = position
		}
		snapshot.Results = append(snapshot.Results, local)
	}
	return snapshot, cost, live, nil
}

type gridCall struct {
	index int
	point report.GeoRankPoint
	cost  float64
	live  bool
	err   error
}

func (c *Client) grid(ctx context.Context, keyword string, center report.MapsVisibility, options Options) ([]report.GeoRankPoint, float64, int, []error) {
	points := geoGrid(options.TargetLatitude, options.TargetLongitude, options.GridRadiusKM)
	points[4].Position = center.TargetPosition
	points[4].Status = "not_found"
	if center.TargetPosition > 0 {
		points[4].Status = "ranked"
	}
	results := make(chan gridCall, len(points)-1)
	var wait sync.WaitGroup
	for index := range points {
		if index == 4 {
			continue
		}
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			point := points[index]
			snapshot, cost, live, err := c.mapsAt(ctx, keyword, point.Latitude, point.Longitude, options)
			point.Position = snapshot.TargetPosition
			point.Status = "not_found"
			if snapshot.TargetPosition > 0 {
				point.Status = "ranked"
			}
			if err != nil {
				point.Status = "error"
				point.Error = err.Error()
			}
			results <- gridCall{index: index, point: point, cost: cost, live: live, err: err}
		}(index)
	}
	wait.Wait()
	close(results)
	var cost float64
	liveCalls := 0
	errors := make([]error, 0)
	for result := range results {
		points[result.index] = result.point
		cost += result.cost
		if result.live {
			liveCalls++
		}
		if result.err != nil {
			errors = append(errors, result.err)
		}
	}
	return points, cost, liveCalls, errors
}

func geoGrid(latitude, longitude, radiusKM float64) []report.GeoRankPoint {
	latitudeStep := radiusKM / 111.32
	longitudeStep := radiusKM / (111.32 * math.Cos(latitude*math.Pi/180))
	points := make([]report.GeoRankPoint, 0, 9)
	for row := -1; row <= 1; row++ {
		for column := -1; column <= 1; column++ {
			points = append(points, report.GeoRankPoint{
				Latitude:  latitude + float64(row)*latitudeStep,
				Longitude: longitude + float64(column)*longitudeStep,
			})
		}
	}
	return points
}

func summarizeGrid(snapshot *report.MapsVisibility) {
	positions := make([]int, 0, len(snapshot.GridPoints))
	topThree := 0
	found := 0
	snapshot.GridCheckedPoints = 0
	snapshot.GridFailedPoints = 0
	for _, point := range snapshot.GridPoints {
		if point.Status == "error" {
			snapshot.GridFailedPoints++
			continue
		}
		snapshot.GridCheckedPoints++
		position := point.Position
		if position > 0 {
			found++
			if position <= 3 {
				topThree++
			}
		} else {
			position = 21
		}
		positions = append(positions, position)
	}
	if len(positions) == 0 {
		return
	}
	sort.Ints(positions)
	snapshot.TopThreeCoverage = math.Round(float64(topThree)*1000/float64(snapshot.GridCheckedPoints)) / 10
	snapshot.FoundCoverage = math.Round(float64(found)*1000/float64(snapshot.GridCheckedPoints)) / 10
	snapshot.MedianPosition = positions[len(positions)/2]
	if snapshot.MedianPosition == 21 {
		snapshot.MedianPosition = 0
	}
}

func (c *Client) ranked(ctx context.Context, options Options) ([]rankedItem, float64, bool, error) {
	location := options.TargetCountry
	if location == "" {
		location = countryLocation(options.Location)
	}
	payload := map[string]any{
		"target":        options.Target,
		"location_name": location,
		"language_code": options.Language,
		"limit":         500,
		"order_by":      []string{"keyword_data.keyword_info.search_volume,desc"},
	}
	var result []rankedResult
	cost, live, err := c.postTask(ctx, "/dataforseo_labs/google/ranked_keywords/live", payload, &result)
	if err != nil || len(result) == 0 {
		return nil, cost, live, err
	}
	return result[0].Items, cost, live, nil
}

func countryLocation(value string) string {
	parts := strings.Split(value, ",")
	return strings.TrimSpace(parts[len(parts)-1])
}

func (c *Client) volumes(ctx context.Context, keywords []string, options Options) ([]volumeItem, float64, bool, error) {
	payload := map[string]any{
		"keywords":      keywords,
		"language_code": options.Language,
	}
	setLocalLocation(payload, options)
	var result []volumeItem
	cost, live, err := c.postTask(ctx, "/keywords_data/google_ads/search_volume/live", payload, &result)
	return result, cost, live, err
}

func (c *Client) serp(ctx context.Context, keyword string, deep bool, options Options) (serpItem, float64, bool, error) {
	depth := 10
	if deep {
		depth = 100
	}
	payload := map[string]any{
		"keyword":       keyword,
		"language_code": options.Language,
		"depth":         depth,
	}
	setLocalLocation(payload, options)
	var result []serpResult
	cost, live, err := c.postTask(ctx, "/serp/google/organic/live/advanced", payload, &result)
	if err != nil || len(result) == 0 {
		return serpItem{}, cost, live, err
	}
	target := normalizeDomain(options.Target)
	for _, item := range result[0].Items {
		if item.Type == "organic" && normalizeDomain(item.Domain) == target {
			return item, cost, live, nil
		}
	}
	return serpItem{}, cost, live, nil
}

func setLocalLocation(payload map[string]any, options Options) {
	if options.TargetLatitude != 0 || options.TargetLongitude != 0 {
		payload["location_coordinate"] = fmt.Sprintf("%.7f,%.7f", options.TargetLatitude, options.TargetLongitude)
		return
	}
	payload["location_name"] = options.Location
}

func (c *Client) postTask(ctx context.Context, path string, payload any, destination any) (float64, bool, error) {
	body, err := json.Marshal([]any{payload})
	if err != nil {
		return 0, false, err
	}
	select {
	case c.limiter <- struct{}{}:
		defer func() { <-c.limiter }()
	case <-ctx.Done():
		return 0, false, ctx.Err()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, false, err
	}
	request.SetBasicAuth(c.Username, c.Password)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return 0, true, err
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return 0, true, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, true, fmt.Errorf("DataForSEO HTTP %d", response.StatusCode)
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(data, &envelope); err != nil {
		return 0, true, fmt.Errorf("decode DataForSEO response: %w", err)
	}
	if envelope.StatusCode != 20000 {
		return 0, true, fmt.Errorf("DataForSEO %d: %s", envelope.StatusCode, envelope.StatusMessage)
	}
	if len(envelope.Tasks) == 0 {
		return 0, true, errors.New("DataForSEO returned no task")
	}
	task := envelope.Tasks[0]
	if task.StatusCode != 20000 {
		return task.Cost, true, fmt.Errorf("DataForSEO %d: %s", task.StatusCode, task.StatusMessage)
	}
	if err := json.Unmarshal(task.Result, destination); err != nil {
		return task.Cost, true, fmt.Errorf("decode DataForSEO task result: %w", err)
	}
	return task.Cost, true, nil
}

func rankedCandidates(items []rankedItem) []candidate {
	result := make([]candidate, 0)
	for _, item := range items {
		keyword := strings.ToLower(strings.TrimSpace(item.KeywordData.Keyword))
		position := item.RankedSERPElement.SERPItem.RankAbsolute
		if keyword == "" || position < 1 || item.KeywordData.KeywordInfo.SearchVolume < 1 {
			continue
		}
		if informationalKeyword(keyword) {
			continue
		}
		result = append(result, candidate{
			keyword:      keyword,
			url:          item.RankedSERPElement.SERPItem.URL,
			position:     position,
			searchVolume: item.KeywordData.KeywordInfo.SearchVolume,
			cpc:          item.KeywordData.KeywordInfo.CPC,
			source:       "current-ranking",
		})
	}
	return result
}

func existingRankings(items []rankedItem) []report.ExistingRanking {
	result := make([]report.ExistingRanking, 0, len(items))
	for _, item := range items {
		keyword := strings.ToLower(strings.TrimSpace(item.KeywordData.Keyword))
		position := item.RankedSERPElement.SERPItem.RankAbsolute
		pageURL := strings.TrimSpace(item.RankedSERPElement.SERPItem.URL)
		if keyword == "" || position < 1 || pageURL == "" {
			continue
		}
		result = append(result, report.ExistingRanking{
			Keyword:      keyword,
			Position:     position,
			URL:          pageURL,
			SearchVolume: item.KeywordData.KeywordInfo.SearchVolume,
			CPC:          item.KeywordData.KeywordInfo.CPC,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Position != result[j].Position {
			return result[i].Position < result[j].Position
		}
		if result[i].SearchVolume != result[j].SearchVolume {
			return result[i].SearchVolume > result[j].SearchVolume
		}
		return result[i].Keyword < result[j].Keyword
	})
	return result
}

func siteCandidates(items []volumeItem, pages []report.PageReport, options Options) []candidate {
	locality := strings.ToLower(strings.TrimSpace(strings.Split(options.Location, ",")[0]))
	brand := compact(options.TargetName)
	domainBrand := compact(strings.Split(options.Target, ".")[0])
	contextTerms := keywordContext(pages, options)
	result := make([]candidate, 0, len(items))
	for index, item := range items {
		keyword := strings.ToLower(strings.Join(strings.Fields(item.Keyword), " "))
		compactKeyword := compact(keyword)
		if keyword == "" || item.SearchVolume < 1 || informationalKeyword(keyword) || likelyMisspelledPlural(keyword) {
			continue
		}
		if (brand != "" && strings.Contains(compactKeyword, brand)) || (len(domainBrand) >= 4 && strings.Contains(compactKeyword, domainBrand)) {
			continue
		}
		if item.CPC <= 0 && !strings.Contains(keyword, locality) && !strings.Contains(keyword, "near me") {
			continue
		}
		if !matchesKeywordContext(keyword, contextTerms) {
			continue
		}
		result = append(result, candidate{
			keyword:      keyword,
			searchVolume: item.SearchVolume,
			cpc:          item.CPC,
			source:       "site-discovery",
			relevance:    index + 1,
		})
	}
	return result
}

func keywordContext(pages []report.PageReport, options Options) map[string]bool {
	terms := map[string]bool{}
	add := func(value string) {
		for _, term := range strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
			return character < 'a' || character > 'z'
		}) {
			term = normalizeTerm(term)
			if len(term) >= 4 && !keywordStopword(term) {
				terms[term] = true
			}
		}
	}
	add(options.TargetCategory)
	for _, page := range pages {
		if !page.Indexable || excludedKeywordPageType(page.PageType) || excludedKeywordPage(page.URL) {
			continue
		}
		add(page.URL)
		add(page.Title)
		add(strings.Join(page.H1, " "))
	}
	for _, locationPart := range strings.Split(options.Location, ",") {
		for _, term := range strings.Fields(strings.ToLower(locationPart)) {
			delete(terms, normalizeTerm(term))
		}
	}
	return terms
}

func assignCandidatePages(items []candidate, pages []report.PageReport) []candidate {
	for index := range items {
		if items[index].targetURL != "" {
			continue
		}
		bestScore := 0
		bestURL := ""
		for _, page := range pages {
			if !page.PriorityPage || !page.Indexable {
				continue
			}
			score := candidatePageScore(items[index].keyword, page)
			pageURL := page.FinalURL
			if pageURL == "" {
				pageURL = page.URL
			}
			if score > bestScore || score == bestScore && score > 0 && pageURL < bestURL {
				bestScore = score
				bestURL = pageURL
			}
		}
		if bestScore > 0 {
			items[index].targetURL = bestURL
		}
	}
	return items
}

func candidatePageScore(keyword string, page report.PageReport) int {
	pageTerms := map[string]bool{}
	values := []string{page.URL, page.Title}
	values = append(values, page.H1...)
	values = append(values, page.KeywordSeeds...)
	for _, value := range values {
		for _, term := range keywordTerms(value) {
			pageTerms[term] = true
		}
	}
	score := 0
	for _, term := range keywordTerms(keyword) {
		if pageTerms[term] {
			score++
		}
	}
	return score
}

func keywordTerms(value string) []string {
	result := make([]string, 0)
	seen := map[string]bool{}
	for _, term := range strings.FieldsFunc(strings.ToLower(value), func(character rune) bool {
		return character < 'a' || character > 'z'
	}) {
		term = normalizeTerm(term)
		if len(term) < 4 || keywordStopword(term) || seen[term] {
			continue
		}
		seen[term] = true
		result = append(result, term)
	}
	return result
}

func excludedKeywordPageType(pageType string) bool {
	switch pageType {
	case "blog", "team-about", "contact-legal-utility":
		return true
	default:
		return false
	}
}

func matchesKeywordContext(keyword string, contextTerms map[string]bool) bool {
	for _, term := range strings.FieldsFunc(strings.ToLower(keyword), func(character rune) bool {
		return character < 'a' || character > 'z'
	}) {
		if contextTerms[normalizeTerm(term)] {
			return true
		}
	}
	return false
}

func normalizeTerm(term string) string {
	if len(term) > 5 {
		term = strings.TrimSuffix(term, "s")
	}
	return term
}

func excludedKeywordPage(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	pagePath := strings.ToLower(parsed.Path)
	for _, excluded := range []string{"/blog/", "/news/", "/privacy", "/terms", "/cookie", "/about", "/team", "/contact"} {
		if strings.Contains(pagePath, excluded) {
			return true
		}
	}
	return false
}

func keywordStopword(term string) bool {
	switch term {
	case "about", "also", "best", "book", "business", "contact", "from", "home", "more", "near", "page", "private", "service", "site", "that", "their", "this", "with", "your":
		return true
	default:
		return false
	}
}

func explicitCandidates(keywords []string, volumes []volumeItem) []candidate {
	byKeyword := make(map[string]volumeItem, len(volumes))
	for _, item := range volumes {
		byKeyword[strings.ToLower(strings.TrimSpace(item.Keyword))] = item
	}
	result := make([]candidate, 0, len(keywords))
	for _, keyword := range keywords {
		volume := byKeyword[keyword]
		result = append(result, candidate{
			keyword:      keyword,
			searchVolume: volume.SearchVolume,
			cpc:          volume.CPC,
			explicit:     true,
			source:       "supplied",
		})
	}
	return result
}

func generatedCandidates(keywords []string, volumes []volumeItem) []candidate {
	byKeyword := make(map[string]volumeItem, len(volumes))
	for _, item := range volumes {
		byKeyword[strings.ToLower(strings.TrimSpace(item.Keyword))] = item
	}
	result := make([]candidate, 0, len(keywords))
	for index, keyword := range keywords {
		volume := byKeyword[keyword]
		result = append(result, candidate{
			keyword:      keyword,
			searchVolume: volume.SearchVolume,
			cpc:          volume.CPC,
			source:       "openrouter",
			relevance:    index + 1,
		})
	}
	return result
}

func shortlistCandidates(items []candidate, limit int) []candidate {
	byKeyword := make(map[string]candidate, len(items))
	for _, item := range items {
		current, found := byKeyword[item.keyword]
		if !found {
			byKeyword[item.keyword] = item
			continue
		}
		if item.searchVolume > current.searchVolume {
			current.searchVolume = item.searchVolume
		}
		if item.cpc > current.cpc {
			current.cpc = item.cpc
		}
		if item.position > 0 {
			current.position = item.position
			current.url = item.url
			if current.source == "" {
				current.source = item.source
			}
		}
		if item.explicit {
			current.explicit = true
			current.source = item.source
		}
		if current.targetURL == "" {
			current.targetURL = item.targetURL
		}
		byKeyword[item.keyword] = current
	}
	byPage := map[string]candidate{}
	for _, item := range byKeyword {
		key := comparableURL(item.targetURL)
		if key == "" {
			key = comparableURL(item.url)
		}
		if item.explicit || item.source == "current-ranking" || key == "" {
			key = "keyword:" + item.keyword
		}
		current, found := byPage[key]
		if !found || item.explicit || candidateSourcePriority(item.source) < candidateSourcePriority(current.source) || item.source == current.source && candidateRatio(item) > candidateRatio(current) {
			byPage[key] = item
		}
	}
	result := make([]candidate, 0, len(byPage))
	for _, item := range byPage {
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].explicit != result[j].explicit {
			return result[i].explicit
		}
		if candidateSourcePriority(result[i].source) != candidateSourcePriority(result[j].source) {
			return candidateSourcePriority(result[i].source) < candidateSourcePriority(result[j].source)
		}
		if result[i].source == "current-ranking" && result[i].position != result[j].position {
			return result[i].position < result[j].position
		}
		if (result[i].source == "openrouter" || result[i].source == "site-discovery") && result[i].relevance != result[j].relevance {
			return result[i].relevance < result[j].relevance
		}
		if candidateImportance(result[i]) != candidateImportance(result[j]) {
			return candidateImportance(result[i]) > candidateImportance(result[j])
		}
		if result[i].searchVolume != result[j].searchVolume {
			return result[i].searchVolume > result[j].searchVolume
		}
		return result[i].keyword < result[j].keyword
	})
	if len(result) > limit {
		result = result[:limit]
	}
	return result
}

func selectGridCandidates(current, opportunities []candidate, options Options, limit int) []candidate {
	result := make([]candidate, 0, limit)
	seenPages := map[string]bool{}
	for _, items := range [][]candidate{current, opportunities} {
		for _, item := range items {
			if len(result) == limit {
				return result
			}
			if item.searchVolume < 1 || informationalKeyword(item.keyword) || brandedKeyword(item.keyword, options) || likelyLocationCode(item.keyword) {
				continue
			}
			page := comparableURL(item.targetURL)
			if page == "" {
				page = comparableURL(item.url)
			}
			if page == "" {
				page = "keyword:" + item.keyword
			}
			if seenPages[page] {
				continue
			}
			seenPages[page] = true
			result = append(result, item)
		}
	}
	return result
}

func brandedKeyword(keyword string, options Options) bool {
	keyword = compact(keyword)
	if len(keyword) < 5 {
		return false
	}
	for _, value := range []string{options.TargetName, strings.Split(options.Target, ".")[0]} {
		brand := compact(value)
		if len(brand) >= 5 && (strings.Contains(keyword, brand) || strings.Contains(brand, keyword)) {
			return true
		}
	}
	return false
}

func likelyLocationCode(keyword string) bool {
	fields := strings.Fields(keyword)
	if len(fields) > 3 {
		return false
	}
	for _, field := range fields {
		hasLetter := false
		hasDigit := false
		for _, character := range field {
			hasLetter = hasLetter || character >= 'a' && character <= 'z'
			hasDigit = hasDigit || character >= '0' && character <= '9'
		}
		if hasLetter && hasDigit {
			return true
		}
	}
	return false
}

func candidateSourcePriority(source string) int {
	switch source {
	case "current-ranking":
		return 0
	case "supplied":
		return 1
	case "openrouter":
		return 2
	default:
		return 3
	}
}

func removeExistingCandidates(items []candidate, rankings []report.ExistingRanking) []candidate {
	existing := make(map[string]bool, len(rankings))
	for _, item := range rankings {
		existing[item.Keyword] = true
	}
	result := make([]candidate, 0, len(items))
	for _, item := range items {
		if !existing[item.keyword] {
			result = append(result, item)
		}
	}
	return result
}

func uniqueCandidateCount(items []candidate) int {
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[item.keyword] = true
	}
	return len(seen)
}

func opportunity(item candidate, page *report.PageReport, maps *report.MapsVisibility) report.Opportunity {
	itemImportance := candidateImportance(item)
	itemEffort := effort(item.position)
	result := report.Opportunity{
		Keyword:       item.keyword,
		Source:        item.source,
		URL:           item.url,
		TargetURL:     item.targetURL,
		Position:      item.position,
		SearchVolume:  item.searchVolume,
		CPC:           item.cpc,
		Importance:    itemImportance,
		Effort:        itemEffort,
		PriorityRatio: float64(itemImportance) / float64(itemEffort),
	}
	if maps != nil {
		result.MapsChecked = true
		result.MapsPosition = maps.TargetPosition
		result.MapsTopThreeCoverage = maps.TopThreeCoverage
	}
	if page != nil {
		result.PageTitle = page.Title
		result.HasBooking = len(page.BookingLinks) > 0
		result.HasPhone = len(page.PhoneLinks) > 0
	}
	organicWeak := result.Position == 0 || result.Position > 10
	mapsWeak := result.MapsChecked && (result.MapsPosition == 0 || result.MapsPosition > 3 || len(maps.GridPoints) > 0 && result.MapsTopThreeCoverage < 50)
	switch {
	case organicWeak && mapsWeak:
		result.Status = "weak-organic-and-maps"
		result.Priority = gapPriority(result.Importance)
	case organicWeak:
		result.Status = "weak-organic"
		result.Priority = gapPriority(result.Importance)
	case mapsWeak:
		result.Status = "weak-maps"
		result.Priority = gapPriority(result.Importance)
	default:
		result.Status = "visible"
		result.Priority = "low"
	}
	result.Evidence = "organic not found in top 100"
	if result.Position > 0 {
		result.Evidence = fmt.Sprintf("organic #%d", result.Position)
	}
	if result.MapsChecked {
		mapsEvidence := "Maps not found in top 20"
		if result.MapsPosition > 0 {
			mapsEvidence = fmt.Sprintf("Maps #%d", result.MapsPosition)
		}
		result.Evidence += "; " + mapsEvidence
		if len(maps.GridPoints) > 0 {
			result.Evidence += fmt.Sprintf("; Maps top-three coverage %.1f%% across %d grid points", maps.TopThreeCoverage, len(maps.GridPoints))
		}
	}
	if organicWeak {
		if result.TargetURL != "" {
			result.Actions = append(result.Actions, "Improve the matched priority page's title, H1, opening copy, proof, and internal links for this search intent.")
		} else if result.URL == "" {
			result.Actions = append(result.Actions, "Create or assign one relevant service/location page, then align its title, H1, opening copy, and internal links with this search intent.")
		} else {
			result.Actions = append(result.Actions, "Improve the existing ranking page's title, H1, opening copy, proof, and internal links for this search intent.")
		}
	}
	if mapsWeak {
		result.Actions = append(result.Actions, "Review the accurate GBP category and services, landing-page relevance, reviews, and local proof against the profiles ranking above it.")
	}
	if !organicWeak && !mapsWeak {
		result.Actions = append(result.Actions, "Protect the ranking page and GBP coverage; no visibility gap was found at this location.")
	}
	if page != nil && !result.HasBooking {
		result.Actions = append(result.Actions, "Add a clear booking or consultation action on the page.")
	}
	if page != nil && !result.HasPhone {
		result.Actions = append(result.Actions, "Add a visible call action for high-intent local visitors.")
	}
	return result
}

func gapPriority(importance int) string {
	if importance >= 4 {
		return "high"
	}
	return "medium"
}

func indexPages(pages []report.PageReport) map[string]*report.PageReport {
	result := make(map[string]*report.PageReport, len(pages))
	for index := range pages {
		page := &pages[index]
		result[comparableURL(page.URL)] = page
		result[comparableURL(page.FinalURL)] = page
	}
	return result
}

func normalizeOptions(options Options) Options {
	options.Target = normalizeDomain(options.Target)
	options.Location = strings.TrimSpace(options.Location)
	options.Language = strings.ToLower(strings.TrimSpace(options.Language))
	if options.Language == "" {
		options.Language = "en"
	}
	if options.MaxChecks < 1 {
		options.MaxChecks = 8
	}
	if options.MaxChecks > 10 {
		options.MaxChecks = 10
	}
	return options
}

func normalizeKeywords(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.Join(strings.Fields(value), " "))
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func normalizeDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		value = parsed.Hostname()
	}
	return strings.TrimPrefix(value, "www.")
}

func comparableURL(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return parsed.String()
}

func informationalKeyword(keyword string) bool {
	for _, prefix := range []string{"what ", "how ", "why ", "when ", "can ", "does ", "is ", "are ", "guide ", "meaning "} {
		if strings.HasPrefix(keyword, prefix) {
			return true
		}
	}
	return false
}

func likelyMisspelledPlural(keyword string) bool {
	for _, word := range strings.Fields(keyword) {
		if len(word) < 4 || !strings.HasSuffix(word, "ys") {
			continue
		}
		beforeY := word[len(word)-3]
		if !strings.ContainsRune("aeiou", rune(beforeY)) {
			return true
		}
	}
	return false
}

func importance(cpc float64, searchVolume int) int {
	switch {
	case cpc >= 15 || searchVolume >= 1000:
		return 5
	case cpc >= 5 || searchVolume >= 100:
		return 4
	default:
		return 3
	}
}

func candidateImportance(item candidate) int {
	return importance(item.cpc, item.searchVolume)
}

func candidateRatio(item candidate) float64 {
	return float64(candidateImportance(item)) / float64(effort(item.position))
}

func compact(value string) string {
	var result strings.Builder
	for _, character := range strings.ToLower(value) {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func effort(position int) int {
	switch {
	case position <= 0:
		return 5
	case position >= 1 && position <= 10:
		return 1
	case position <= 25:
		return 2
	case position <= 40:
		return 3
	case position <= 70:
		return 4
	default:
		return 5
	}
}

func progress(options Options, message string) {
	if options.Progress != nil {
		options.Progress(message)
	}
}

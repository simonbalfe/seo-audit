package dataforseo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/ranktracking"
)

const rankLiveBatchSize = 1

type rankEnvelope struct {
	StatusCode    int        `json:"status_code"`
	StatusMessage string     `json:"status_message"`
	Cost          float64    `json:"cost"`
	Tasks         []rankTask `json:"tasks"`
}

type rankTask struct {
	StatusCode    int             `json:"status_code"`
	StatusMessage string          `json:"status_message"`
	Cost          float64         `json:"cost"`
	Data          rankTaskData    `json:"data"`
	Result        json.RawMessage `json:"result"`
}

type rankTaskData struct {
	Tag string `json:"tag"`
}

type rankSERPResult struct {
	Items []rankSERPItem `json:"items"`
}

type rankSERPItem struct {
	Type         string   `json:"type"`
	RankGroup    *float64 `json:"rank_group"`
	RankAbsolute *float64 `json:"rank_absolute"`
	Domain       string   `json:"domain"`
	URL          string   `json:"url"`
}

func (c *Client) CheckRanks(ctx context.Context, options ranktracking.CheckOptions) (ranktracking.ProviderReport, error) {
	report := ranktracking.ProviderReport{
		Source:         ranktracking.ProviderDataForSEO,
		RetrievedAt:    time.Now().UTC(),
		RequestedTasks: len(options.Tasks),
	}
	for start := 0; start < len(options.Tasks); start += rankLiveBatchSize {
		end := min(start+rankLiveBatchSize, len(options.Tasks))
		batch := options.Tasks[start:end]
		if options.Progress != nil {
			options.Progress(fmt.Sprintf("Checking tasks %d-%d of %d", start+1, end, len(options.Tasks)))
		}
		results, taskErrors, cost, err := c.checkRankBatch(ctx, options, batch)
		report.LiveCalls += len(batch)
		report.CostUSD += cost
		report.Results = append(report.Results, results...)
		report.SuccessfulTasks += len(results)
		report.Errors = append(report.Errors, taskErrors...)
		if err != nil {
			return report, err
		}
	}
	sort.Slice(report.Results, func(i, j int) bool {
		if report.Results[i].Keyword != report.Results[j].Keyword {
			return strings.ToLower(report.Results[i].Keyword) < strings.ToLower(report.Results[j].Keyword)
		}
		return report.Results[i].Device < report.Results[j].Device
	})
	return report, nil
}

func (c *Client) checkRankBatch(
	ctx context.Context,
	options ranktracking.CheckOptions,
	tasks []ranktracking.CheckTask,
) ([]ranktracking.Result, []string, float64, error) {
	payload := make([]map[string]any, 0, len(tasks))
	taskByTag := make(map[string]ranktracking.CheckTask, len(tasks))
	for _, task := range tasks {
		tag := strconv.FormatInt(task.KeywordID, 10) + ":" + task.Device
		taskByTag[tag] = task
		payload = append(payload, map[string]any{
			"keyword":       task.Keyword,
			"location_name": options.Location,
			"language_code": options.Language,
			"device":        task.Device,
			"os":            rankOperatingSystem(task.Device),
			"depth":         options.Depth,
			"tag":           tag,
			"stop_crawl_on_match": []map[string]string{{
				"match_value": options.Target,
				"match_type":  "with_subdomains",
			}},
			"find_targets_in": []string{"organic"},
		})
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("encode rank check request: %w", err)
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(c.BaseURL, "/")+"/serp/google/organic/live/advanced",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("create rank check request: %w", err)
	}
	request.SetBasicAuth(c.Username, c.Password)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("request rank check: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return nil, nil, 0, fmt.Errorf("read rank check response: %w", err)
	}
	var envelope rankEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, nil, 0, fmt.Errorf("decode rank check response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil, envelope.Cost, fmt.Errorf("rank check HTTP %d: %s", response.StatusCode, envelope.StatusMessage)
	}
	if envelope.StatusCode != 20000 {
		return nil, nil, envelope.Cost, fmt.Errorf("DataForSEO %d: %s", envelope.StatusCode, envelope.StatusMessage)
	}

	results := make([]ranktracking.Result, 0, len(tasks))
	taskErrors := make([]string, 0)
	taskCost := 0.0
	for index, responseTask := range envelope.Tasks {
		taskCost += responseTask.Cost
		task, found := taskByTag[responseTask.Data.Tag]
		if !found && index < len(tasks) {
			task = tasks[index]
			found = true
		}
		if !found {
			taskErrors = append(taskErrors, "DataForSEO returned a rank task without a recognized tag")
			continue
		}
		if responseTask.StatusCode != 20000 {
			if strings.Contains(strings.ToLower(responseTask.StatusMessage), "no search results") {
				results = append(results, emptyRankResult(task))
				continue
			}
			taskErrors = append(taskErrors, fmt.Sprintf("%s (%s): DataForSEO %d: %s", task.Keyword, task.Device, responseTask.StatusCode, responseTask.StatusMessage))
			continue
		}
		result, err := parseRankTask(task, options.Target, responseTask.Result)
		if err != nil {
			taskErrors = append(taskErrors, fmt.Sprintf("%s (%s): %s", task.Keyword, task.Device, err))
			continue
		}
		results = append(results, result)
	}
	if len(envelope.Tasks) < len(tasks) {
		taskErrors = append(taskErrors, fmt.Sprintf("DataForSEO returned %d of %d rank tasks", len(envelope.Tasks), len(tasks)))
	}
	if taskCost == 0 {
		taskCost = envelope.Cost
	}
	return results, taskErrors, taskCost, nil
}

func parseRankTask(task ranktracking.CheckTask, target string, raw json.RawMessage) (ranktracking.Result, error) {
	var response []rankSERPResult
	if len(raw) == 0 || string(raw) == "null" {
		return emptyRankResult(task), nil
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return ranktracking.Result{}, fmt.Errorf("decode SERP result: %w", err)
	}
	if len(response) == 0 {
		return emptyRankResult(task), nil
	}
	result := emptyRankResult(task)
	features := make(map[string]bool)
	normalizedTarget := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(target)), "www.")
	for _, item := range response[0].Items {
		if item.Type != "" {
			features[item.Type] = true
		}
		if result.Position != nil || item.Type != "organic" {
			continue
		}
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(item.Domain)), "www.")
		if domain != normalizedTarget && !strings.HasSuffix(domain, "."+normalizedTarget) {
			continue
		}
		position := item.RankAbsolute
		if position == nil {
			position = item.RankGroup
		}
		if position != nil {
			value := roundedInt(*position)
			result.Position = &value
		}
		result.RankingURL = item.URL
	}
	for feature := range features {
		result.SERPFeatures = append(result.SERPFeatures, feature)
	}
	sort.Strings(result.SERPFeatures)
	return result, nil
}

func emptyRankResult(task ranktracking.CheckTask) ranktracking.Result {
	return ranktracking.Result{
		KeywordID: task.KeywordID,
		Keyword:   task.Keyword,
		Device:    task.Device,
	}
}

func rankOperatingSystem(device string) string {
	if device == "mobile" {
		return "android"
	}
	return "windows"
}

package crawl

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
)

const resourceWorkerCount = 24

func (c *Client) checkResources(ctx context.Context, report SiteReport, options Options) []ResourceReport {
	start, _ := url.Parse(report.StartURL)
	unique := map[string]bool{}
	for _, page := range report.Pages {
		for _, resource := range page.Resources {
			unique[resource] = true
		}
		if options.CheckExternal {
			for _, link := range page.ExternalLinks {
				unique[link] = true
			}
		}
	}
	targets := make([]string, 0, len(unique))
	for target := range unique {
		parsed, err := url.Parse(target)
		if err == nil && (options.CheckExternal || sameHost(start, parsed)) {
			targets = append(targets, target)
		}
	}
	sort.Strings(targets)
	emitProgress(options, "resources", "Checking %d linked resources with %d workers", len(targets), resourceWorkerCount)

	jobs := make(chan string)
	results := make(chan ResourceReport)
	var workers sync.WaitGroup
	for range resourceWorkerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for target := range jobs {
				results <- c.checkResource(ctx, target)
			}
		}()
	}
	go func() {
		for _, target := range targets {
			jobs <- target
		}
		close(jobs)
		workers.Wait()
		close(results)
	}()

	output := make([]ResourceReport, 0, len(targets))
	for result := range results {
		output = append(output, result)
		detail := fmt.Sprintf("HTTP %d", result.StatusCode)
		if result.Error != "" {
			detail = "error: " + result.Error
		}
		emitProgress(options, "resources", "Checked %d/%d: %s; %s", len(output), len(targets), result.URL, detail)
	}
	sort.Slice(output, func(i, j int) bool { return output[i].URL < output[j].URL })
	return output
}

func (c *Client) checkResource(ctx context.Context, target string) ResourceReport {
	result := ResourceReport{URL: target}
	request := func(method string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, method, target, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", c.UserAgent)
		if method == http.MethodGet {
			req.Header.Set("Range", "bytes=0-0")
		}
		return c.HTTP.Do(req)
	}
	response, err := request(http.MethodHead)
	if err == nil && response.StatusCode >= 400 {
		response.Body.Close()
		response, err = request(http.MethodGet)
	}
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer response.Body.Close()
	result.StatusCode = response.StatusCode
	result.ContentType = response.Header.Get("Content-Type")
	result.SizeBytes = response.ContentLength
	return result
}

func analyzeResources(report *SiteReport) {
	start, _ := url.Parse(report.StartURL)
	for _, resource := range report.Resources {
		parsed, _ := url.Parse(resource.URL)
		internal := parsed != nil && sameHost(start, parsed)
		if resource.StatusCode >= 400 {
			priority := "low"
			status := Warn
			if internal {
				priority = "high"
				status = Fail
			}
			addSiteFinding(report, "resources", "Broken resource or external link", status, priority, resource.URL, fmt.Sprintf("HTTP %d", resource.StatusCode), "Update or remove the failing URL.")
		}
		if internal && strings.HasPrefix(resource.ContentType, "image/") && resource.SizeBytes > 500*1024 {
			addSiteFinding(report, "images", "Large image", Warn, "medium", resource.URL, fmt.Sprintf("%d KB", resource.SizeBytes/1024), "Compress and resize the image, then serve a modern format where practical.")
		}
	}
}

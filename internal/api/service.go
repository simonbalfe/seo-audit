package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/simonbalfe/seo-audit/internal/dataforseo"
	"github.com/simonbalfe/seo-audit/internal/gsc"
	"github.com/simonbalfe/seo-audit/internal/ranktracking"
	"github.com/simonbalfe/seo-audit/internal/storage"
)

type ValidationError struct {
	Message string
	Fields  map[string]string
}

func (e *ValidationError) Error() string {
	return e.Message
}

type Service struct {
	store        *storage.SQLiteStore
	jobs         *JobManager
	jobWorkers   int
	jobRetention int
}

func NewService(store *storage.SQLiteStore, jobs *JobManager, jobWorkers, jobRetention int) *Service {
	return &Service{
		store:        store,
		jobs:         jobs,
		jobWorkers:   jobWorkers,
		jobRetention: jobRetention,
	}
}

func (s *Service) Capabilities() CapabilitiesResponse {
	return CapabilitiesResponse{
		API: APICapabilities{
			Version:      "v1",
			JobWorkers:   s.jobWorkers,
			JobRetention: s.jobRetention,
		},
		Audit: AuditCapabilities{
			DefaultPageLimit:         DefaultAuditPageLimit,
			MaxPageLimit:             MaxAuditPageLimit,
			DefaultTimeoutSeconds:    DefaultRequestTimeout,
			MaxRequestTimeoutSeconds: MaxRequestTimeout,
		},
		Providers: ProviderCapabilities{
			DataForSEO: ProviderCapability{Configured: dataForSEOConfigured()},
			GSC:        ProviderCapability{Configured: gscConfigured()},
		},
		Rankings: RankingCapabilities{
			MaxKeywords: ranktracking.MaxKeywords,
			MaxDepth:    ranktracking.DefaultDepth,
		},
	}
}

func (s *Service) SubmitAudit(request AuditRequest, idempotencyKey string) (Job, bool, error) {
	if err := normalizeAuditRequest(&request); err != nil {
		return Job{}, false, err
	}
	target, err := normalizeDomainTarget(request.URL)
	if err != nil {
		return Job{}, false, err
	}
	return s.jobs.Submit("audit", idempotencyKey, func(ctx context.Context, emit func(string, string)) (any, error) {
		report, err := audit.NewClient(time.Duration(request.RequestTimeoutSeconds)*time.Second).Audit(
			ctx,
			request.URL,
			audit.Options{
				Limit:            request.PageLimit,
				CheckExternal:    boolValue(request.CheckExternal, true),
				CheckPerformance: boolValue(request.CheckPerformance, true),
				Progress: func(event audit.ProgressEvent) {
					emit(event.Stage, event.Message)
				},
			},
		)
		if err != nil {
			return nil, err
		}
		if request.Save {
			id, err := saveReportSnapshot(ctx, s.store, "audit", target, "public crawl", time.Now().UTC(), report)
			if err != nil {
				return nil, err
			}
			emit("storage", fmt.Sprintf("Saved audit snapshot %d", id))
		}
		return report, nil
	})
}

func (s *Service) SubmitOpportunities(request OpportunityRequest, idempotencyKey string) (Job, bool, error) {
	target, err := normalizeDomainTarget(request.URL)
	if err != nil {
		return Job{}, false, err
	}
	if err := normalizeOpportunityRequest(&request); err != nil {
		return Job{}, false, err
	}
	return s.jobs.Submit("opportunities", idempotencyKey, func(ctx context.Context, emit func(string, string)) (any, error) {
		report := OpportunityReport{Target: target}
		if slices.Contains(request.Sources, "gsc") {
			client, err := gsc.NewClient()
			if err != nil {
				return nil, err
			}
			siteURL := strings.TrimSpace(request.GSC.SiteURL)
			if siteURL == "" {
				siteURL = strings.TrimSpace(os.Getenv("GSC_SITE_URL"))
			}
			if siteURL == "" {
				siteURL = "sc-domain:" + target
			}
			searchConsole, err := client.QueryPerformance(ctx, gsc.Options{
				SiteURL: siteURL,
				Days:    request.GSC.Days,
				Limit:   request.GSC.RowLimit,
				Progress: func(message string) {
					emit("gsc", message)
				},
			})
			if err != nil {
				return nil, err
			}
			report.SearchConsole = &searchConsole
			if request.GSC.Save {
				id, err := saveReportSnapshot(
					ctx,
					s.store,
					"gsc",
					target,
					searchConsole.Source,
					searchConsole.RetrievedAt,
					searchConsole,
				)
				if err != nil {
					return nil, err
				}
				emit("storage", fmt.Sprintf("Saved Search Console snapshot %d", id))
			}
		}
		if slices.Contains(request.Sources, "dataforseo") {
			client, err := dataforseo.NewClient()
			if err != nil {
				return nil, err
			}
			client.Store = s.store
			searchData := client.Search(ctx, dataforseo.Options{
				Target:   target,
				Location: request.DataForSEO.Location,
				Language: request.DataForSEO.Language,
				Limit:    request.DataForSEO.RowLimit,
				CacheTTL: time.Duration(request.DataForSEO.CacheTTLSeconds) * time.Second,
				Refresh:  request.DataForSEO.Refresh,
				Progress: func(dataset, message string) {
					emit("dataforseo:"+dataset, message)
				},
			})
			report.SearchData = &searchData
		}
		return report, nil
	})
}

func (s *Service) SubmitBacklinks(request BacklinkRequest, idempotencyKey string) (Job, bool, error) {
	target, err := normalizeDomainTarget(request.URL)
	if err != nil {
		return Job{}, false, err
	}
	if err := normalizeBacklinkRequest(&request); err != nil {
		return Job{}, false, err
	}
	return s.jobs.Submit("backlinks", idempotencyKey, func(ctx context.Context, emit func(string, string)) (any, error) {
		client, err := dataforseo.NewClient()
		if err != nil {
			return nil, err
		}
		client.Store = s.store
		report := client.Backlinks(ctx, dataforseo.Options{
			Target:   target,
			Limit:    request.RowLimit,
			CacheTTL: time.Duration(request.CacheTTLSeconds) * time.Second,
			Refresh:  request.Refresh,
			Progress: func(dataset, message string) {
				emit("dataforseo:"+dataset, message)
			},
		})
		return report, nil
	})
}

func (s *Service) UpsertRankTracker(ctx context.Context, request RankTrackerRequest) (ranktracking.KeywordUpdate, error) {
	target, err := normalizeDomainTarget(request.URL)
	if err != nil {
		return ranktracking.KeywordUpdate{}, err
	}
	config := ranktracking.Config{
		Target:    target,
		Location:  request.Location,
		Language:  request.Language,
		Devices:   request.Devices,
		SERPDepth: request.SERPDepth,
	}
	normalized, err := ranktracking.NormalizeConfig(config)
	if err != nil {
		return ranktracking.KeywordUpdate{}, validationError(err.Error(), "tracker", err.Error())
	}
	existing, err := s.store.GetRankConfig(ctx, normalized.Target, normalized.Location, normalized.Language)
	if err == nil {
		if request.Devices == "" {
			normalized.Devices = existing.Devices
		}
		if request.SERPDepth == 0 {
			normalized.SERPDepth = existing.SERPDepth
		}
	} else if !errors.Is(err, ranktracking.ErrTrackerNotFound) {
		return ranktracking.KeywordUpdate{}, err
	}
	update, err := ranktracking.Add(ctx, s.store, normalized, request.Keywords)
	if err != nil {
		return ranktracking.KeywordUpdate{}, validationError(err.Error(), "keywords", err.Error())
	}
	return update, nil
}

func (s *Service) ListRankTrackers(
	ctx context.Context,
	target,
	location,
	language string,
) (RankTrackersResponse, error) {
	var targets []string
	if strings.TrimSpace(target) != "" {
		normalized, err := normalizeDomainTarget(target)
		if err != nil {
			return RankTrackersResponse{}, err
		}
		targets = []string{normalized}
	} else {
		var err error
		targets, err = s.store.ListEvidenceTargets(ctx)
		if err != nil {
			return RankTrackersResponse{}, err
		}
	}
	response := RankTrackersResponse{Trackers: make([]ranktracking.Report, 0)}
	for _, currentTarget := range targets {
		configs, err := s.store.ListRankConfigsByTarget(ctx, currentTarget)
		if err != nil {
			return RankTrackersResponse{}, err
		}
		for _, config := range configs {
			if location != "" && config.Location != location {
				continue
			}
			if language != "" && !strings.EqualFold(config.Language, language) {
				continue
			}
			report, err := s.store.GetRankReport(ctx, config.ID)
			if err != nil {
				return RankTrackersResponse{}, err
			}
			response.Trackers = append(response.Trackers, report)
		}
	}
	return response, nil
}

func (s *Service) GetRankTracker(ctx context.Context, id int64) (ranktracking.Report, error) {
	return s.store.GetRankReport(ctx, id)
}

func (s *Service) PatchRankTracker(
	ctx context.Context,
	id int64,
	request RankTrackerPatchRequest,
) (ranktracking.Report, error) {
	report, err := s.store.GetRankReport(ctx, id)
	if err != nil {
		return ranktracking.Report{}, err
	}
	config := report.Config
	if request.Devices != "" {
		config.Devices = request.Devices
	}
	if request.SERPDepth != 0 {
		config.SERPDepth = request.SERPDepth
	}
	if request.Devices == "" && request.SERPDepth == 0 {
		return ranktracking.Report{}, validationError(
			"provide devices or serp_depth",
			"tracker",
			"provide at least one tracker setting",
		)
	}
	config, err = s.store.UpsertRankConfig(ctx, config)
	if err != nil {
		return ranktracking.Report{}, validationError(err.Error(), "tracker", err.Error())
	}
	return s.store.GetRankReport(ctx, config.ID)
}

func (s *Service) PatchRankKeywords(
	ctx context.Context,
	id int64,
	request RankKeywordPatchRequest,
) (ranktracking.KeywordUpdate, error) {
	report, err := s.store.GetRankReport(ctx, id)
	if err != nil {
		return ranktracking.KeywordUpdate{}, err
	}
	if len(request.Add) > 0 && len(request.Remove) > 0 {
		return ranktracking.KeywordUpdate{}, validationError(
			"add and remove cannot be combined",
			"keywords",
			"send one keyword operation per request",
		)
	}
	if len(request.Add) > 0 {
		update, err := ranktracking.Add(ctx, s.store, report.Config, request.Add)
		if err != nil {
			return ranktracking.KeywordUpdate{}, validationError(err.Error(), "keywords", err.Error())
		}
		return update, nil
	}
	if len(request.Remove) > 0 {
		update, err := ranktracking.Remove(
			ctx,
			s.store,
			report.Config.Target,
			report.Config.Location,
			report.Config.Language,
			request.Remove,
		)
		if err != nil {
			return ranktracking.KeywordUpdate{}, validationError(err.Error(), "keywords", err.Error())
		}
		return update, nil
	}
	return ranktracking.KeywordUpdate{}, validationError(
		"provide keywords to add or remove",
		"keywords",
		"at least one keyword is required",
	)
}

func (s *Service) SubmitRankCheck(
	ctx context.Context,
	id int64,
	request RankCheckRequest,
	idempotencyKey string,
) (Job, bool, error) {
	if strings.ToLower(strings.TrimSpace(request.Source)) != "dataforseo" {
		return Job{}, false, validationError(
			"rank checking requires the explicit DataForSEO source",
			"source",
			"must be dataforseo",
		)
	}
	report, err := s.store.GetRankReport(ctx, id)
	if err != nil {
		return Job{}, false, err
	}
	return s.jobs.Submit("rank-check", idempotencyKey, func(ctx context.Context, emit func(string, string)) (any, error) {
		client, err := dataforseo.NewClient()
		if err != nil {
			return nil, err
		}
		return ranktracking.Check(
			ctx,
			s.store,
			client,
			report.Config.Target,
			report.Config.Location,
			report.Config.Language,
			func(message string) {
				emit("dataforseo:rankings", message)
			},
		)
	})
}

func normalizeAuditRequest(request *AuditRequest) error {
	if _, err := normalizeDomainTarget(request.URL); err != nil {
		return err
	}
	if request.PageLimit == 0 {
		request.PageLimit = DefaultAuditPageLimit
	}
	if request.PageLimit < 1 || request.PageLimit > MaxAuditPageLimit {
		return validationError(
			fmt.Sprintf("page_limit must be from 1 to %d", MaxAuditPageLimit),
			"page_limit",
			"out of range",
		)
	}
	if request.RequestTimeoutSeconds == 0 {
		request.RequestTimeoutSeconds = DefaultRequestTimeout
	}
	if request.RequestTimeoutSeconds < 1 || request.RequestTimeoutSeconds > MaxRequestTimeout {
		return validationError(
			fmt.Sprintf("request_timeout_seconds must be from 1 to %d", MaxRequestTimeout),
			"request_timeout_seconds",
			"out of range",
		)
	}
	return nil
}

func normalizeOpportunityRequest(request *OpportunityRequest) error {
	seen := make(map[string]bool)
	for index, source := range request.Sources {
		source = strings.ToLower(strings.TrimSpace(source))
		if source != "gsc" && source != "dataforseo" {
			return validationError("unsupported opportunity source", "sources", source)
		}
		if !seen[source] {
			request.Sources[index] = source
			seen[source] = true
		}
	}
	if len(seen) == 0 {
		return validationError(
			"select at least one source",
			"sources",
			"include gsc or dataforseo",
		)
	}
	request.Sources = request.Sources[:0]
	for _, source := range []string{"gsc", "dataforseo"} {
		if seen[source] {
			request.Sources = append(request.Sources, source)
		}
	}
	if request.GSC.Days == 0 {
		request.GSC.Days = DefaultGSCDays
	}
	if request.GSC.RowLimit == 0 {
		request.GSC.RowLimit = DefaultGSCLimit
	}
	if request.GSC.Days < 1 || request.GSC.Days > MaxGSCDays {
		return validationError("gsc.days is out of range", "gsc.days", "must be from 1 to 480")
	}
	if request.GSC.RowLimit < 1 || request.GSC.RowLimit > MaxGSCLimit {
		return validationError("gsc.row_limit is out of range", "gsc.row_limit", "must be from 1 to 25000")
	}
	if slices.Contains(request.Sources, "dataforseo") {
		if err := normalizeDataForSEORequest(&request.DataForSEO); err != nil {
			return err
		}
	}
	return nil
}

func normalizeBacklinkRequest(request *BacklinkRequest) error {
	request.Source = strings.ToLower(strings.TrimSpace(request.Source))
	if request.Source != "dataforseo" {
		return validationError(
			"backlink analysis requires the explicit DataForSEO source",
			"source",
			"must be dataforseo",
		)
	}
	if request.RowLimit == 0 {
		request.RowLimit = DefaultProviderLimit
	}
	if request.RowLimit < 1 || request.RowLimit > MaxProviderLimit {
		return validationError("row_limit is out of range", "row_limit", "must be from 1 to 100")
	}
	if request.CacheTTLSeconds == 0 {
		request.CacheTTLSeconds = int64(DefaultProviderCacheTTL / time.Second)
	}
	if request.CacheTTLSeconds < 1 {
		return validationError("cache_ttl_seconds must be positive", "cache_ttl_seconds", "must be positive")
	}
	return nil
}

func normalizeDataForSEORequest(request *DataForSEORequest) error {
	if request.RowLimit == 0 {
		request.RowLimit = DefaultProviderLimit
	}
	if request.RowLimit < 1 || request.RowLimit > MaxProviderLimit {
		return validationError(
			"dataforseo.row_limit is out of range",
			"dataforseo.row_limit",
			"must be from 1 to 100",
		)
	}
	if request.CacheTTLSeconds == 0 {
		request.CacheTTLSeconds = int64(DefaultProviderCacheTTL / time.Second)
	}
	if request.CacheTTLSeconds < 1 {
		return validationError(
			"dataforseo.cache_ttl_seconds must be positive",
			"dataforseo.cache_ttl_seconds",
			"must be positive",
		)
	}
	if strings.TrimSpace(request.Location) == "" {
		request.Location = ranktracking.DefaultLocation
	}
	if strings.TrimSpace(request.Language) == "" {
		request.Language = ranktracking.DefaultLanguage
	}
	return nil
}

func normalizeDomainTarget(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", validationError("url must be an absolute HTTP or HTTPS URL", "url", "invalid URL")
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www."), nil
}

func validationError(message, field, detail string) error {
	return &ValidationError{
		Message: message,
		Fields:  map[string]string{field: detail},
	}
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func saveReportSnapshot(
	ctx context.Context,
	store *storage.SQLiteStore,
	kind,
	target,
	source string,
	retrievedAt time.Time,
	value any,
) (int64, error) {
	resultJSON, err := json.Marshal(value)
	if err != nil {
		return 0, fmt.Errorf("encode %s snapshot: %w", kind, err)
	}
	return store.SaveReportSnapshot(ctx, storage.ReportSnapshot{
		Kind:        kind,
		Target:      target,
		Source:      source,
		RetrievedAt: retrievedAt,
		ResultJSON:  resultJSON,
	})
}

func dataForSEOConfigured() bool {
	return strings.TrimSpace(os.Getenv("DATAFORSEO_USERNAME")) != "" &&
		strings.TrimSpace(os.Getenv("DATAFORSEO_PASSWORD")) != ""
}

func gscConfigured() bool {
	if strings.TrimSpace(os.Getenv("GSC_ACCESS_TOKEN")) != "" {
		return true
	}
	paths := make([]string, 0, 3)
	if configured := strings.TrimSpace(os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")); configured != "" {
		paths = append(paths, configured)
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(
			paths,
			filepath.Join(home, ".config", "google-cli", "credentials.json"),
			filepath.Join(home, ".config", "gcloud", "application_default_credentials.json"),
		)
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	return false
}

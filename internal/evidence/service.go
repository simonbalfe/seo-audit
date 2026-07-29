package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/simonbalfe/seo-audit/internal/dataforseo"
	"github.com/simonbalfe/seo-audit/internal/gsc"
	"github.com/simonbalfe/seo-audit/internal/ranktracking"
	"github.com/simonbalfe/seo-audit/internal/storage"
)

var ErrSiteNotFound = errors.New("site evidence not found")

type Store interface {
	ListEvidenceTargets(context.Context) ([]string, error)
	LatestReportSnapshot(context.Context, string, string) (storage.ReportSnapshot, bool, error)
	LatestProviderSnapshot(context.Context, string, string) (storage.ProviderSnapshot, bool, error)
	ListRankConfigsByTarget(context.Context, string) ([]ranktracking.Config, error)
	GetRankReport(context.Context, int64) (ranktracking.Report, error)
}

type Service struct {
	store Store
}

type SitesResponse struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Sites       []SiteSummary `json:"sites"`
}

type SiteSummary struct {
	Target          string    `json:"target"`
	LastUpdated     time.Time `json:"last_updated"`
	HasAudit        bool      `json:"has_audit"`
	HasGSC          bool      `json:"has_gsc"`
	HasSearch       bool      `json:"has_search"`
	HasBacklinks    bool      `json:"has_backlinks"`
	RankingTrackers int       `json:"ranking_trackers"`
}

type AuditSnapshot struct {
	SnapshotID   int64                   `json:"snapshot_id"`
	RetrievedAt  time.Time               `json:"retrieved_at"`
	StartURL     string                  `json:"start_url"`
	DurationMS   int64                   `json:"duration_ms"`
	LimitReached bool                    `json:"limit_reached"`
	Summary      audit.Summary           `json:"summary"`
	Performance  audit.PerformanceReport `json:"performance"`
	Findings     []audit.Finding         `json:"findings"`
	Pages        []AuditPage             `json:"pages"`
}

type AuditPage struct {
	URL          string `json:"url"`
	StatusCode   int    `json:"status_code"`
	Indexable    bool   `json:"indexable"`
	Indexability string `json:"indexability,omitempty"`
	Title        string `json:"title,omitempty"`
	WordCount    int    `json:"word_count"`
	Depth        int    `json:"depth"`
	Inlinks      int    `json:"inlinks"`
	Findings     int    `json:"findings"`
}

type SiteResponse struct {
	GeneratedAt time.Time             `json:"generated_at"`
	Target      string                `json:"target"`
	LastUpdated time.Time             `json:"last_updated"`
	Audit       *AuditSnapshot        `json:"audit,omitempty"`
	GSC         *gsc.Report           `json:"gsc,omitempty"`
	Search      *dataforseo.Report    `json:"search,omitempty"`
	Backlinks   *dataforseo.Report    `json:"backlinks,omitempty"`
	Rankings    []ranktracking.Report `json:"rankings"`
	Warnings    []string              `json:"warnings,omitempty"`
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) ListSites(ctx context.Context) (SitesResponse, error) {
	targets, err := s.store.ListEvidenceTargets(ctx)
	if err != nil {
		return SitesResponse{}, err
	}
	response := SitesResponse{
		GeneratedAt: time.Now().UTC(),
		Sites:       make([]SiteSummary, 0, len(targets)),
	}
	for _, target := range targets {
		site, err := s.GetSite(ctx, target)
		if err != nil {
			return SitesResponse{}, err
		}
		response.Sites = append(response.Sites, SiteSummary{
			Target:          site.Target,
			LastUpdated:     site.LastUpdated,
			HasAudit:        site.Audit != nil,
			HasGSC:          site.GSC != nil,
			HasSearch:       site.Search != nil,
			HasBacklinks:    site.Backlinks != nil,
			RankingTrackers: len(site.Rankings),
		})
	}
	return response, nil
}

func (s *Service) GetSite(ctx context.Context, rawTarget string) (SiteResponse, error) {
	target, err := normalizeTarget(rawTarget)
	if err != nil {
		return SiteResponse{}, ErrSiteNotFound
	}
	targets, err := s.store.ListEvidenceTargets(ctx)
	if err != nil {
		return SiteResponse{}, err
	}
	if !containsTarget(targets, target) {
		return SiteResponse{}, ErrSiteNotFound
	}
	response := SiteResponse{
		GeneratedAt: time.Now().UTC(),
		Target:      target,
		Rankings:    make([]ranktracking.Report, 0),
	}
	if err := s.loadAudit(ctx, target, &response); err != nil {
		return SiteResponse{}, err
	}
	if err := s.loadGSC(ctx, target, &response); err != nil {
		return SiteResponse{}, err
	}
	if err := s.loadProvider(ctx, target, "search", &response.Search, &response); err != nil {
		return SiteResponse{}, err
	}
	if err := s.loadProvider(ctx, target, "backlinks", &response.Backlinks, &response); err != nil {
		return SiteResponse{}, err
	}
	configs, err := s.store.ListRankConfigsByTarget(ctx, target)
	if err != nil {
		return SiteResponse{}, err
	}
	for _, config := range configs {
		report, err := s.store.GetRankReport(ctx, config.ID)
		if err != nil {
			return SiteResponse{}, err
		}
		response.Rankings = append(response.Rankings, report)
		response.LastUpdated = laterTime(response.LastUpdated, config.UpdatedAt)
		if report.LatestRun != nil {
			response.LastUpdated = laterTime(response.LastUpdated, report.LatestRun.StartedAt)
		}
	}
	sort.Slice(response.Rankings, func(i, j int) bool {
		if response.Rankings[i].Config.Location != response.Rankings[j].Config.Location {
			return response.Rankings[i].Config.Location < response.Rankings[j].Config.Location
		}
		return response.Rankings[i].Config.Language < response.Rankings[j].Config.Language
	})
	return response, nil
}

func (s *Service) loadAudit(ctx context.Context, target string, response *SiteResponse) error {
	snapshot, found, err := s.store.LatestReportSnapshot(ctx, "audit", target)
	if err != nil || !found {
		return err
	}
	var report audit.SiteReport
	if err := json.Unmarshal(snapshot.ResultJSON, &report); err != nil {
		response.Warnings = append(response.Warnings, "Could not decode the latest audit snapshot: "+err.Error())
		return nil
	}
	pages := make([]AuditPage, 0, len(report.Pages))
	for _, page := range report.Pages {
		pages = append(pages, AuditPage{
			URL:          page.URL,
			StatusCode:   page.StatusCode,
			Indexable:    page.Indexable,
			Indexability: page.Indexability,
			Title:        page.Title,
			WordCount:    page.WordCount,
			Depth:        page.Depth,
			Inlinks:      page.Inlinks,
			Findings:     len(page.Findings),
		})
	}
	response.Audit = &AuditSnapshot{
		SnapshotID:   snapshot.ID,
		RetrievedAt:  snapshot.RetrievedAt,
		StartURL:     report.StartURL,
		DurationMS:   report.Duration,
		LimitReached: report.LimitReached,
		Summary:      report.Summary,
		Performance:  report.Performance,
		Findings:     append([]audit.Finding{}, report.Findings...),
		Pages:        pages,
	}
	response.LastUpdated = laterTime(response.LastUpdated, snapshot.RetrievedAt)
	return nil
}

func (s *Service) loadGSC(ctx context.Context, target string, response *SiteResponse) error {
	snapshot, found, err := s.store.LatestReportSnapshot(ctx, "gsc", target)
	if err != nil || !found {
		return err
	}
	var report gsc.Report
	if err := json.Unmarshal(snapshot.ResultJSON, &report); err != nil {
		response.Warnings = append(response.Warnings, "Could not decode the latest Search Console snapshot: "+err.Error())
		return nil
	}
	report.QueryPages = append([]gsc.QueryPageMetric{}, report.QueryPages...)
	report.StrikingDistance = append([]gsc.QueryPageMetric{}, report.StrikingDistance...)
	report.QueryOverlaps = append([]gsc.QueryOverlap{}, report.QueryOverlaps...)
	response.GSC = &report
	response.LastUpdated = laterTime(response.LastUpdated, snapshot.RetrievedAt)
	return nil
}

func (s *Service) loadProvider(
	ctx context.Context,
	target,
	group string,
	destination **dataforseo.Report,
	response *SiteResponse,
) error {
	snapshot, found, err := s.store.LatestProviderSnapshot(ctx, group, target)
	if err != nil || !found {
		return err
	}
	var report dataforseo.Report
	if err := json.Unmarshal(snapshot.ResultJSON, &report); err != nil {
		response.Warnings = append(response.Warnings, fmt.Sprintf("Could not decode the latest %s snapshot: %s", group, err))
		return nil
	}
	*destination = &report
	response.LastUpdated = laterTime(response.LastUpdated, snapshot.RetrievedAt)
	return nil
}

func normalizeTarget(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("site target is empty")
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Hostname() == "" {
		return "", errors.New("site target is invalid")
	}
	return strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www."), nil
}

func containsTarget(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(value, target) {
			return true
		}
	}
	return false
}

func laterTime(current, candidate time.Time) time.Time {
	if candidate.After(current) {
		return candidate
	}
	return current
}

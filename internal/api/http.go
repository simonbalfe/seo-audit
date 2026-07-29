package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"github.com/simonbalfe/seo-audit/internal/evidence"
	"github.com/simonbalfe/seo-audit/internal/ranktracking"
)

const maxRequestBody = 1 << 20

type handler struct {
	service *Service
	loader  EvidenceLoader
}

type EvidenceLoader interface {
	ListSites(context.Context) (evidence.SitesResponse, error)
	GetSite(context.Context, string) (evidence.SiteResponse, error)
}

type problem struct {
	Type   string            `json:"type"`
	Title  string            `json:"title"`
	Status int               `json:"status"`
	Detail string            `json:"detail"`
	Code   string            `json:"code"`
	Fields map[string]string `json:"fields,omitempty"`
}

func NewHandler(
	service *Service,
	loader EvidenceLoader,
) http.Handler {
	value := &handler{service: service, loader: loader}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", value.health)
	mux.HandleFunc("GET /api/v1/capabilities", value.capabilities)
	mux.HandleFunc("POST /api/v1/audits", value.createAudit)
	mux.HandleFunc("POST /api/v1/opportunities", value.createOpportunities)
	mux.HandleFunc("POST /api/v1/backlinks", value.createBacklinks)
	mux.HandleFunc("GET /api/v1/jobs/{id}", value.getJob)
	mux.HandleFunc("DELETE /api/v1/jobs/{id}", value.cancelJob)
	mux.HandleFunc("GET /api/v1/jobs/{id}/events", value.getJobEvents)
	mux.HandleFunc("GET /api/v1/jobs/{id}/result", value.getJobResult)
	mux.HandleFunc("GET /api/v1/rank-trackers", value.listRankTrackers)
	mux.HandleFunc("POST /api/v1/rank-trackers", value.upsertRankTracker)
	mux.HandleFunc("GET /api/v1/rank-trackers/{id}", value.getRankTracker)
	mux.HandleFunc("PATCH /api/v1/rank-trackers/{id}", value.patchRankTracker)
	mux.HandleFunc("PATCH /api/v1/rank-trackers/{id}/keywords", value.patchRankKeywords)
	mux.HandleFunc("POST /api/v1/rank-trackers/{id}/checks", value.createRankCheck)
	mux.HandleFunc("GET /api/v1/sites", value.listSites)
	mux.HandleFunc("GET /api/v1/sites/{target}", value.getSite)
	mux.HandleFunc("/api/v1/", value.apiNotFound)
	return securityHeaders(cors(mux))
}

func (h *handler) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) capabilities(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, h.service.Capabilities())
}

func (h *handler) createAudit(writer http.ResponseWriter, request *http.Request) {
	var input AuditRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	job, replayed, err := h.service.SubmitAudit(input, request.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeAcceptedJob(writer, job, replayed)
}

func (h *handler) createOpportunities(writer http.ResponseWriter, request *http.Request) {
	var input OpportunityRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	job, replayed, err := h.service.SubmitOpportunities(input, request.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeAcceptedJob(writer, job, replayed)
}

func (h *handler) createBacklinks(writer http.ResponseWriter, request *http.Request) {
	var input BacklinkRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	job, replayed, err := h.service.SubmitBacklinks(input, request.Header.Get("Idempotency-Key"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeAcceptedJob(writer, job, replayed)
}

func (h *handler) getJob(writer http.ResponseWriter, request *http.Request) {
	after, ok := parseAfter(writer, request)
	if !ok {
		return
	}
	job, err := h.service.jobs.Get(request.PathValue("id"), after)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, job)
}

func (h *handler) getJobEvents(writer http.ResponseWriter, request *http.Request) {
	after, ok := parseAfter(writer, request)
	if !ok {
		return
	}
	events, err := h.service.jobs.Events(request.PathValue("id"), after)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, events)
}

func (h *handler) getJobResult(writer http.ResponseWriter, request *http.Request) {
	result, err := h.service.jobs.Result(request.PathValue("id"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (h *handler) cancelJob(writer http.ResponseWriter, request *http.Request) {
	job, err := h.service.jobs.Cancel(request.PathValue("id"))
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, job)
}

func (h *handler) listRankTrackers(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	response, err := h.service.ListRankTrackers(
		request.Context(),
		query.Get("target"),
		query.Get("location"),
		query.Get("language"),
	)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (h *handler) upsertRankTracker(writer http.ResponseWriter, request *http.Request) {
	var input RankTrackerRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	update, err := h.service.UpsertRankTracker(request.Context(), input)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, update)
}

func (h *handler) getRankTracker(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseTrackerID(writer, request)
	if !ok {
		return
	}
	report, err := h.service.GetRankTracker(request.Context(), id)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, report)
}

func (h *handler) patchRankTracker(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseTrackerID(writer, request)
	if !ok {
		return
	}
	var input RankTrackerPatchRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	report, err := h.service.PatchRankTracker(request.Context(), id, input)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, report)
}

func (h *handler) patchRankKeywords(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseTrackerID(writer, request)
	if !ok {
		return
	}
	var input RankKeywordPatchRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	update, err := h.service.PatchRankKeywords(request.Context(), id, input)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, update)
}

func (h *handler) createRankCheck(writer http.ResponseWriter, request *http.Request) {
	id, ok := parseTrackerID(writer, request)
	if !ok {
		return
	}
	var input RankCheckRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	job, replayed, err := h.service.SubmitRankCheck(
		request.Context(),
		id,
		input,
		request.Header.Get("Idempotency-Key"),
	)
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeAcceptedJob(writer, job, replayed)
}

func (h *handler) listSites(writer http.ResponseWriter, request *http.Request) {
	response, err := h.loader.ListSites(request.Context())
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (h *handler) getSite(writer http.ResponseWriter, request *http.Request) {
	response, err := h.loader.GetSite(request.Context(), request.PathValue("target"))
	if errors.Is(err, evidence.ErrSiteNotFound) {
		writeProblem(writer, http.StatusNotFound, "site_not_found", err.Error(), nil)
		return
	}
	if err != nil {
		writeServiceError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}

func (h *handler) apiNotFound(writer http.ResponseWriter, _ *http.Request) {
	writeProblem(writer, http.StatusNotFound, "not_found", "API route not found", nil)
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	contentType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		writeProblem(
			writer,
			http.StatusUnsupportedMediaType,
			"unsupported_media_type",
			"Content-Type must be application/json",
			nil,
		)
		return false
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maxRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "Invalid JSON request: "+err.Error(), nil)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(writer, http.StatusBadRequest, "invalid_json", "Request must contain one JSON value", nil)
		return false
	}
	return true
}

func parseTrackerID(writer http.ResponseWriter, request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeProblem(writer, http.StatusBadRequest, "invalid_tracker_id", "Tracker id must be a positive integer", nil)
		return 0, false
	}
	return id, true
}

func parseAfter(writer http.ResponseWriter, request *http.Request) (int64, bool) {
	value := strings.TrimSpace(request.URL.Query().Get("after"))
	if value == "" {
		return 0, true
	}
	after, err := strconv.ParseInt(value, 10, 64)
	if err != nil || after < 0 {
		writeProblem(writer, http.StatusBadRequest, "invalid_event_cursor", "after must be a non-negative integer", nil)
		return 0, false
	}
	return after, true
}

func writeAcceptedJob(writer http.ResponseWriter, job Job, replayed bool) {
	writer.Header().Set("Location", job.StatusURL)
	writer.Header().Set("Retry-After", "1")
	if replayed {
		writer.Header().Set("Idempotency-Replayed", "true")
	}
	writeJSON(writer, http.StatusAccepted, job)
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeServiceError(writer http.ResponseWriter, err error) {
	var validation *ValidationError
	switch {
	case errors.As(err, &validation):
		writeProblem(writer, http.StatusBadRequest, "validation_failed", validation.Message, validation.Fields)
	case errors.Is(err, ErrJobNotFound):
		writeProblem(writer, http.StatusNotFound, "job_not_found", err.Error(), nil)
	case errors.Is(err, ErrJobNotReady):
		writeProblem(writer, http.StatusConflict, "job_not_ready", err.Error(), nil)
	case errors.Is(err, ErrJobNoResult):
		writeProblem(writer, http.StatusConflict, "job_has_no_result", err.Error(), nil)
	case errors.Is(err, ErrJobCapacity):
		writeProblem(writer, http.StatusServiceUnavailable, "job_capacity_reached", err.Error(), nil)
	case errors.Is(err, ranktracking.ErrTrackerNotFound) ||
		strings.Contains(strings.ToLower(err.Error()), "rank tracker not found"):
		writeProblem(writer, http.StatusNotFound, "tracker_not_found", err.Error(), nil)
	default:
		writeProblem(writer, http.StatusInternalServerError, "internal_error", err.Error(), nil)
	}
}

func writeProblem(
	writer http.ResponseWriter,
	status int,
	code,
	detail string,
	fields map[string]string,
) {
	title := http.StatusText(status)
	writer.Header().Set("Content-Type", "application/problem+json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(problem{
		Type:   "https://seoaudit.local/problems/" + code,
		Title:  title,
		Status: status,
		Detail: detail,
		Code:   code,
		Fields: fields,
	})
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		origin := request.Header.Get("Origin")
		if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" {
			writer.Header().Set("Access-Control-Allow-Origin", origin)
			writer.Header().Set("Vary", "Origin")
			writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			writer.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Idempotency-Key")
		}
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set(
			"Content-Security-Policy",
			"default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'",
		)
		next.ServeHTTP(writer, request)
	})
}

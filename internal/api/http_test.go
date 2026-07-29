package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/simonbalfe/seo-audit/internal/api"
	"github.com/simonbalfe/seo-audit/internal/apiclient"
	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/simonbalfe/seo-audit/internal/evidence"
	"github.com/simonbalfe/seo-audit/internal/ranktracking"
	"github.com/simonbalfe/seo-audit/internal/storage"
)

func TestAPIHealthCapabilitiesAndValidation(t *testing.T) {
	server, _ := newTestAPI(t)
	response, err := http.Get(server.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("get health: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", response.StatusCode)
	}

	response, err = http.Get(server.URL + "/api/v1/capabilities")
	if err != nil {
		t.Fatalf("get capabilities: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("capabilities status = %d", response.StatusCode)
	}
	var capabilities api.CapabilitiesResponse
	if err := json.NewDecoder(response.Body).Decode(&capabilities); err != nil {
		t.Fatalf("decode capabilities: %v", err)
	}
	if capabilities.API.Version != "v1" || capabilities.Audit.MaxPageLimit != api.MaxAuditPageLimit {
		t.Fatalf("unexpected capabilities: %#v", capabilities)
	}

	request, err := http.NewRequest(
		http.MethodPost,
		server.URL+"/api/v1/audits",
		bytes.NewBufferString(`{"url":"https://example.com"}`),
	)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("post invalid content type: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("content type status = %d", response.StatusCode)
	}
	if response.Header.Get("Content-Type") != "application/problem+json; charset=utf-8" {
		t.Fatalf("problem content type = %q", response.Header.Get("Content-Type"))
	}
}

func TestAuditJobRunsThroughRESTAPIAndHonoursIdempotency(t *testing.T) {
	site := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/robots.txt":
			writer.Header().Set("Content-Type", "text/plain")
			_, _ = writer.Write([]byte("User-agent: *\nAllow: /\n"))
		case "/sitemap.xml":
			http.NotFound(writer, request)
		default:
			writer.Header().Set("Content-Type", "text/html")
			_, _ = writer.Write([]byte("<html><head><title>Example page</title></head><body><main><h1>Example page</h1><p>This is public audit evidence.</p></main></body></html>"))
		}
	}))
	defer site.Close()

	server, _ := newTestAPI(t)
	client, err := apiclient.New(server.URL)
	if err != nil {
		t.Fatalf("create API client: %v", err)
	}
	external := false
	performance := false
	input := api.AuditRequest{
		URL:              site.URL,
		PageLimit:        5,
		CheckExternal:    &external,
		CheckPerformance: &performance,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	requestBody, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("encode request: %v", err)
	}
	submit := func() api.Job {
		request, err := http.NewRequestWithContext(
			ctx,
			http.MethodPost,
			server.URL+"/api/v1/audits",
			bytes.NewReader(requestBody),
		)
		if err != nil {
			t.Fatalf("create submit request: %v", err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "same-audit")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("submit audit: %v", err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("submit status = %d", response.StatusCode)
		}
		var job api.Job
		if err := json.NewDecoder(response.Body).Decode(&job); err != nil {
			t.Fatalf("decode job: %v", err)
		}
		return job
	}
	first := submit()
	second := submit()
	if first.ID != second.ID {
		t.Fatalf("idempotent job ids differ: %s != %s", first.ID, second.ID)
	}
	data, err := client.Wait(ctx, first, nil)
	if err != nil {
		t.Fatalf("wait for audit: %v", err)
	}
	var report audit.SiteReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode audit report: %v", err)
	}
	if report.Summary.Pages != 1 || report.StartURL == "" {
		t.Fatalf("unexpected audit report: %#v", report.Summary)
	}
}

func TestRankTrackerRESTWorkflow(t *testing.T) {
	server, _ := newTestAPI(t)
	client, err := apiclient.New(server.URL)
	if err != nil {
		t.Fatalf("create API client: %v", err)
	}
	ctx := context.Background()
	var added ranktracking.KeywordUpdate
	if err := client.Post(ctx, "/api/v1/rank-trackers", api.RankTrackerRequest{
		URL:       "https://example.com",
		Devices:   "both",
		SERPDepth: 50,
		Keywords:  []string{"seo audit", "technical seo"},
	}, &added); err != nil {
		t.Fatalf("add rank tracker: %v", err)
	}
	if added.Added != 2 || added.Config.ID == 0 {
		t.Fatalf("unexpected tracker update: %#v", added)
	}

	var trackers api.RankTrackersResponse
	if err := client.Get(
		ctx,
		"/api/v1/rank-trackers?target=https%3A%2F%2Fexample.com&location=United+Kingdom&language=en",
		&trackers,
	); err != nil {
		t.Fatalf("list trackers: %v", err)
	}
	if len(trackers.Trackers) != 1 || trackers.Trackers[0].Summary.TrackedKeywords != 2 {
		t.Fatalf("unexpected trackers: %#v", trackers)
	}

	var removed ranktracking.KeywordUpdate
	if err := client.Patch(
		ctx,
		"/api/v1/rank-trackers/"+strconv.FormatInt(added.Config.ID, 10)+"/keywords",
		api.RankKeywordPatchRequest{Remove: []string{"technical seo"}},
		&removed,
	); err != nil {
		t.Fatalf("remove keyword: %v", err)
	}
	if removed.Removed != 1 || removed.TotalKeywords != 1 {
		t.Fatalf("unexpected removal: %#v", removed)
	}
}

func newTestAPI(t *testing.T) (*httptest.Server, *storage.SQLiteStore) {
	t.Helper()
	store, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "api.db"), 10)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	jobs := api.NewJobManager(ctx, 2, 20)
	server := httptest.NewServer(api.NewHandler(
		api.NewService(store, jobs, 2, 20),
		evidence.NewService(store),
	))
	t.Cleanup(server.Close)
	return server, store
}

package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/simonbalfe/seo-audit/internal/report"
)

func TestDashboardListsBusinessPages(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "audits.sqlite")
	site := report.SiteReport{
		StartURL: "https://example.com/",
		GBP:      &report.GBPAuditReport{PlaceID: "place-123", Name: "Example Dental"},
		Pages: []report.PageReport{{
			URL: "https://example.com/implants", FinalURL: "https://example.com/implants", PageType: "service",
			PageTypeSource: "openrouter", StatusCode: 200, Indexable: true, Title: "Implants", H1: []string{"Dental implants"}, WordCount: 800,
		}},
	}
	if err := Save(t.Context(), databasePath, site); err != nil {
		t.Fatal(err)
	}
	database, err := open(t.Context(), databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close dashboard database: %v", err)
		}
	})
	handler, err := newHandler(database, newAuditRunner(t.Context(), "seoaudit", t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}

	businessRequest := httptest.NewRequest(http.MethodGet, "/api/businesses", nil)
	businessResponse := httptest.NewRecorder()
	handler.ServeHTTP(businessResponse, businessRequest)
	var businesses []business
	if err := json.NewDecoder(businessResponse.Body).Decode(&businesses); err != nil {
		t.Fatal(err)
	}
	if businessResponse.Code != http.StatusOK || len(businesses) != 1 || businesses[0].PlaceID != "place-123" || businesses[0].PageCount != 1 {
		t.Fatalf("business response %d: %#v", businessResponse.Code, businesses)
	}

	pageRequest := httptest.NewRequest(http.MethodGet, "/api/businesses/1/pages", nil)
	pageRequest.SetPathValue("id", "1")
	pageResponse := httptest.NewRecorder()
	handler.ServeHTTP(pageResponse, pageRequest)
	var pages []page
	if err := json.NewDecoder(pageResponse.Body).Decode(&pages); err != nil {
		t.Fatal(err)
	}
	if pageResponse.Code != http.StatusOK || len(pages) != 1 || pages[0].PageType != "service" || pages[0].Title != "Implants" {
		t.Fatalf("page response %d: %#v", pageResponse.Code, pages)
	}
}

func TestAuditRequestValidationAndProgressStage(t *testing.T) {
	request, err := validAuditRequest(auditRequest{PlaceID: " place-123 ", Steps: "website"})
	if err != nil {
		t.Fatal(err)
	}
	if request.PlaceID != "place-123" || request.Limit != 50 || request.TimeoutSeconds != 30 {
		t.Fatalf("validated request = %#v", request)
	}
	if got := progressStage("[crawl] Fetching page 2"); got != "website" {
		t.Fatalf("progressStage() = %q, want website", got)
	}
	if _, err := validAuditRequest(auditRequest{PlaceID: "place-123", Steps: "unknown"}); err == nil {
		t.Fatal("validAuditRequest accepted unknown steps")
	}
}

func TestAuditRunnerCompletesCommand(t *testing.T) {
	runner := newAuditRunner(t.Context(), "true", t.TempDir())
	job, err := runner.Start(auditRequest{PlaceID: "place-123", Steps: "profile"})
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" {
		t.Fatalf("Start status = %q, want running", job.Status)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		job = runner.Current()
		if job.Status == "completed" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("runner status = %q, error = %q", job.Status, job.Error)
}

func TestIdleAuditJobUsesEmptyLogArray(t *testing.T) {
	job := newAuditRunner(t.Context(), "true", t.TempDir()).Current()
	data, err := json.Marshal(job)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"status":"idle","logs":[]}` {
		t.Fatalf("idle audit job = %s", data)
	}
}

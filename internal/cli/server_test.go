package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/simonbalfe/seo-audit/internal/report"
)

func TestAuditServerRunsPlaceIDRequest(t *testing.T) {
	handler := newAuditServer(func(_ context.Context, placeID string) (report.SiteReport, error) {
		if placeID != "place-123" {
			t.Fatalf("place ID = %q", placeID)
		}
		return report.SiteReport{GBP: &report.GBPAuditReport{PlaceID: placeID, Name: "Example Dental"}}, nil
	})
	request := httptest.NewRequest(http.MethodPost, "/api/audits", strings.NewReader(`{"placeId":" place-123 "}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var result report.SiteReport
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.GBP == nil || result.GBP.PlaceID != "place-123" {
		t.Fatalf("GBP = %#v", result.GBP)
	}
}

func TestAuditServerRejectsInvalidRequest(t *testing.T) {
	handler := newAuditServer(func(context.Context, string) (report.SiteReport, error) {
		return report.SiteReport{}, errors.New("must not run")
	})
	request := httptest.NewRequest(http.MethodPost, "/api/audits", strings.NewReader(`{"placeId":"","other":true}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

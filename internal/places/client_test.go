package places

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuditReportsMissingPublicListingFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/places/place-1" {
			http.NotFound(writer, request)
			return
		}
		if got := request.Header.Get("X-Goog-Api-Key"); got != "test-key" {
			t.Fatalf("API key = %q", got)
		}
		for _, field := range []string{"websiteUri", "location", "addressComponents"} {
			if !strings.Contains(request.Header.Get("X-Goog-FieldMask"), field) {
				t.Fatalf("place details field mask does not request %s", field)
			}
		}
		_, _ = writer.Write([]byte(`{"id":"place-1","displayName":{"text":"Test Beauty"},"businessStatus":"OPERATIONAL","addressComponents":[{"longText":"London","types":["postal_town"]},{"longText":"United Kingdom","types":["country","political"]}],"location":{"latitude":51.5001,"longitude":-0.1202}}`))
	}))
	defer server.Close()

	client := NewClientWithAPIKey("test-key")
	client.BaseURL = server.URL
	report, err := client.AuditPlace(context.Background(), "place-1")
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if report.Name != "Test Beauty" || report.PlaceID != "place-1" {
		t.Fatalf("report = %#v", report)
	}
	if report.Latitude != 51.5001 || report.Longitude != -0.1202 {
		t.Fatalf("coordinates = %f,%f", report.Latitude, report.Longitude)
	}
	if report.Country != "United Kingdom" {
		t.Fatalf("country = %q, want United Kingdom", report.Country)
	}
	if report.Market != "London, United Kingdom" {
		t.Fatalf("market = %q, want London, United Kingdom", report.Market)
	}
	if len(report.Findings) != 5 {
		t.Fatalf("findings = %d, want 5", len(report.Findings))
	}
}

func TestAuditReportsExplicitEmptyReviewsAndPhotos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`{"id":"place-1","displayName":{"text":"Test Beauty"},"businessStatus":"OPERATIONAL","userRatingCount":0,"photos":[]}`))
	}))
	defer server.Close()

	client := NewClientWithAPIKey("test-key")
	client.BaseURL = server.URL
	report, err := client.AuditPlace(context.Background(), "place-1")
	if err != nil {
		t.Fatalf("audit place: %v", err)
	}
	if len(report.Findings) != 7 {
		t.Fatalf("findings = %d, want 7", len(report.Findings))
	}
	if report.Findings[5].Check != "No reviews" || report.Findings[6].Check != "No photos" {
		t.Fatalf("findings = %#v", report.Findings)
	}
}

func TestAuditPlaceUsesExactPlaceDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/places/place-1" {
			t.Fatalf("path = %q, want place details", request.URL.Path)
		}
		_, _ = writer.Write([]byte(`{"id":"place-1","displayName":{"text":"Test Beauty"},"businessStatus":"OPERATIONAL"}`))
	}))
	defer server.Close()

	client := NewClientWithAPIKey("test-key")
	client.BaseURL = server.URL
	report, err := client.AuditPlace(context.Background(), "place-1")
	if err != nil {
		t.Fatalf("audit place: %v", err)
	}
	if report.PlaceID != "place-1" {
		t.Fatalf("place ID = %q, want place-1", report.PlaceID)
	}
}

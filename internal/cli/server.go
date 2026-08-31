package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/simonbalfe/seo-audit/internal/dataforseo"
	"github.com/simonbalfe/seo-audit/internal/places"
	"github.com/simonbalfe/seo-audit/internal/report"
)

const defaultListenAddress = "0.0.0.0:8090"

type auditRequest struct {
	PlaceID string `json:"placeId"`
}

type auditRunner func(context.Context, string) (report.SiteReport, error)

func runServer(ctx context.Context, args []string, errorOutput io.Writer) error {
	if len(args) != 0 {
		return errors.New("usage: seoaudit serve")
	}
	address := strings.TrimSpace(os.Getenv("SEO_AUDIT_LISTEN_ADDR"))
	if address == "" {
		address = defaultListenAddress
	}
	server := &http.Server{
		Addr:              address,
		Handler:           newAuditServer(runVisibilityAudit),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.ListenAndServe()
	}()
	fmt.Fprintf(errorOutput, "SEO Audit API listening on %s\n", address)
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown SEO Audit API: %w", err)
		}
		if err := <-serveErrors; !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func newAuditServer(run auditRunner) http.Handler {
	var running sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			writeAPIError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		writeAPIJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/audits", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			writeAPIError(writer, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var input auditRequest
		decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeAPIError(writer, http.StatusBadRequest, "invalid audit request")
			return
		}
		input.PlaceID = strings.TrimSpace(input.PlaceID)
		if input.PlaceID == "" {
			writeAPIError(writer, http.StatusBadRequest, "placeId is required")
			return
		}
		if !running.TryLock() {
			writeAPIError(writer, http.StatusConflict, "an audit is already running")
			return
		}
		defer running.Unlock()
		report, err := run(request.Context(), input.PlaceID)
		if err != nil {
			writeAPIError(writer, http.StatusBadGateway, err.Error())
			return
		}
		writeAPIJSON(writer, http.StatusOK, report)
	})
	return mux
}

func runVisibilityAudit(ctx context.Context, placeID string) (report.SiteReport, error) {
	started := time.Now()
	placesClient, err := places.NewClient()
	if err != nil {
		return report.SiteReport{}, err
	}
	profile, err := placesClient.AuditPlace(ctx, placeID)
	if err != nil {
		return report.SiteReport{}, err
	}
	if strings.TrimSpace(profile.Website) == "" {
		return report.SiteReport{}, errors.New("Google Place has no public website")
	}
	dataClient, err := dataforseo.NewClient()
	if err != nil {
		return report.SiteReport{}, err
	}
	market := dataClient.Scan(ctx, nil, dataforseo.Options{
		Target:          profile.Website,
		Location:        profile.Market,
		Language:        "en",
		MaxChecks:       5,
		TargetName:      profile.Name,
		TargetCategory:  profile.Category,
		TargetCountry:   profile.Country,
		TargetPlaceID:   profile.PlaceID,
		TargetLatitude:  profile.Latitude,
		TargetLongitude: profile.Longitude,
		GridRadiusKM:    2,
	})
	return report.SiteReport{
		StartURL: profile.Website,
		Duration: time.Since(started).Milliseconds(),
		Market:   market,
		GBP:      &profile,
	}, nil
}

func writeAPIJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeAPIError(writer http.ResponseWriter, status int, message string) {
	writeAPIJSON(writer, status, map[string]string{"error": message})
}

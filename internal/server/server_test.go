package server

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/simonbalfe/seo-audit/internal/storage"
)

func TestServerOnlyAcceptsLoopbackAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1:8787", "[::1]:8787", "localhost:8787"} {
		if err := ValidateAddress(address); err != nil {
			t.Errorf("loopback address %q rejected: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:8787", "192.168.1.5:8787", "example.com:8787"} {
		if err := ValidateAddress(address); err == nil {
			t.Errorf("non-loopback address %q accepted", address)
		}
	}
}

func TestHandlerKeepsAPIRoutesSeparateFromDashboardRoutes(t *testing.T) {
	store, err := storage.OpenSQLite(filepath.Join(t.TempDir(), "server.db"), 10)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer store.Close()
	static := fstest.MapFS{
		"index.html": {Data: []byte("<!doctype html><title>Dashboard</title>")},
	}
	handler := NewHandler(
		context.Background(),
		store,
		Config{Workers: 1, JobRetention: 10},
		fs.FS(static),
	)
	for _, test := range []struct {
		path        string
		status      int
		contentType string
	}{
		{path: "/api/v1/health", status: http.StatusOK, contentType: "application/json"},
		{path: "/api/v1/missing", status: http.StatusNotFound, contentType: "application/problem+json"},
		{path: "/", status: http.StatusOK, contentType: "text/html"},
		{path: "/rankings", status: http.StatusOK, contentType: "text/html"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s status = %d, want %d", test.path, response.Code, test.status)
		}
		if value := response.Header().Get("Content-Type"); !strings.Contains(value, test.contentType) {
			t.Errorf("%s content type = %q", test.path, value)
		}
	}
}

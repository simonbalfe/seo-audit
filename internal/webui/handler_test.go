package webui

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestHandlerServesDashboardRoutesAndAssets(t *testing.T) {
	static := fstest.MapFS{
		"index.html":     {Data: []byte("<!doctype html><title>Dashboard</title>")},
		"assets/app.css": {Data: []byte("body{}")},
	}
	handler := NewHandler(fs.FS(static))
	for _, test := range []struct {
		path        string
		status      int
		contentType string
	}{
		{path: "/", status: http.StatusOK, contentType: "text/html"},
		{path: "/rankings", status: http.StatusOK, contentType: "text/html"},
		{path: "/assets/app.css", status: http.StatusOK, contentType: "text/css"},
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

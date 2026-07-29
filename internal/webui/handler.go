package webui

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

type handler struct {
	static fs.FS
	files  http.Handler
}

func NewHandler(static fs.FS) http.Handler {
	value := &handler{
		static: static,
		files:  http.FileServer(http.FS(static)),
	}
	return securityHeaders(http.HandlerFunc(value.frontend))
}

func (h *handler) frontend(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cleaned := strings.TrimPrefix(path.Clean(request.URL.Path), "/")
	if cleaned == "." || cleaned == "" {
		cleaned = "index.html"
	}
	if _, err := fs.Stat(h.static, cleaned); err != nil {
		request.URL.Path = "/"
		h.files.ServeHTTP(writer, request)
		return
	}
	if cleaned == "index.html" {
		request.URL.Path = "/"
		h.files.ServeHTTP(writer, request)
		return
	}
	request.URL.Path = "/" + cleaned
	h.files.ServeHTTP(writer, request)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; img-src 'self' data:; connect-src 'self'")
		next.ServeHTTP(writer, request)
	})
}

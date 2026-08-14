// Package dashboard persists crawled pages and serves the embedded review UI.
package dashboard

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/internal/report"
	_ "modernc.org/sqlite"
)

//go:embed dist
var assets embed.FS

type business struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	PlaceID   string `json:"place_id"`
	SiteURL   string `json:"site_url"`
	PageCount int    `json:"page_count"`
	UpdatedAt string `json:"updated_at"`
}

type page struct {
	URL            string `json:"url"`
	FinalURL       string `json:"final_url"`
	PageType       string `json:"page_type"`
	PageTypeSource string `json:"page_type_source"`
	StatusCode     int    `json:"status_code"`
	Indexable      bool   `json:"indexable"`
	Title          string `json:"title"`
	H1             string `json:"h1"`
	Depth          int    `json:"depth"`
	Inlinks        int    `json:"inlinks"`
	WordCount      int    `json:"word_count"`
	Issues         int    `json:"issues"`
	UpdatedAt      string `json:"updated_at"`
}

// Save adds the latest crawled pages to their business entry.
func Save(ctx context.Context, databasePath string, site report.SiteReport) error {
	database, err := open(ctx, databasePath)
	if err != nil {
		return err
	}
	saveErr := save(ctx, database, site)
	return errors.Join(saveErr, database.Close())
}

// Serve starts the local dashboard and API.
func Serve(ctx context.Context, databasePath, address string) error {
	database, err := open(ctx, databasePath)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return errors.Join(fmt.Errorf("find dashboard executable: %w", err), database.Close())
	}
	workdir, err := os.Getwd()
	if err != nil {
		return errors.Join(fmt.Errorf("find dashboard working directory: %w", err), database.Close())
	}
	handler, err := newHandler(database, newAuditRunner(ctx, executable, workdir))
	if err != nil {
		return errors.Join(err, database.Close())
	}
	server := &http.Server{
		Addr:              address,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	serveErrors := make(chan error, 1)
	go func() {
		serveErrors <- server.ListenAndServe()
	}()
	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		return errors.Join(err, database.Close())
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		shutdownErr := server.Shutdown(shutdownContext)
		serveErr := <-serveErrors
		if errors.Is(serveErr, http.ErrServerClosed) {
			serveErr = nil
		}
		return errors.Join(shutdownErr, serveErr, database.Close())
	}
}

func open(ctx context.Context, databasePath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("create dashboard database directory: %w", err)
	}
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open dashboard database: %w", err)
	}
	database.SetMaxOpenConns(1)
	for _, statement := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS businesses (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			business_key TEXT NOT NULL UNIQUE,
			place_id TEXT NOT NULL,
			name TEXT NOT NULL,
			site_url TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS pages (
			business_id INTEGER NOT NULL REFERENCES businesses(id) ON DELETE CASCADE,
			url TEXT NOT NULL,
			final_url TEXT NOT NULL,
			page_type TEXT NOT NULL,
			page_type_source TEXT NOT NULL,
			status_code INTEGER NOT NULL,
			indexable INTEGER NOT NULL,
			title TEXT NOT NULL,
			h1 TEXT NOT NULL,
			depth INTEGER NOT NULL,
			inlinks INTEGER NOT NULL,
			word_count INTEGER NOT NULL,
			issues INTEGER NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (business_id, url)
		)`,
		`PRAGMA optimize`,
	} {
		if _, err := database.ExecContext(ctx, statement); err != nil {
			closeErr := database.Close()
			return nil, errors.Join(fmt.Errorf("initialize dashboard database: %w", err), closeErr)
		}
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		closeErr := database.Close()
		return nil, errors.Join(fmt.Errorf("secure dashboard database: %w", err), closeErr)
	}
	return database, nil
}

func save(ctx context.Context, database *sql.DB, site report.SiteReport) error {
	key, name, placeID := businessIdentity(site)
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	transaction, err := database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dashboard update: %w", err)
	}
	rollback := func(cause error) error {
		return errors.Join(cause, transaction.Rollback())
	}
	var businessID int64
	err = transaction.QueryRowContext(ctx, `
		INSERT INTO businesses (business_key, place_id, name, site_url, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (business_key) DO UPDATE SET
			place_id = excluded.place_id,
			name = excluded.name,
			site_url = excluded.site_url,
			updated_at = excluded.updated_at
		RETURNING id
	`, key, placeID, name, site.StartURL, updatedAt).Scan(&businessID)
	if err != nil {
		return rollback(fmt.Errorf("save business entry: %w", err))
	}
	for _, crawled := range site.Pages {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO pages (
				business_id, url, final_url, page_type, page_type_source, status_code,
				indexable, title, h1, depth, inlinks, word_count, issues, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (business_id, url) DO UPDATE SET
				final_url = excluded.final_url,
				page_type = excluded.page_type,
				page_type_source = excluded.page_type_source,
				status_code = excluded.status_code,
				indexable = excluded.indexable,
				title = excluded.title,
				h1 = excluded.h1,
				depth = excluded.depth,
				inlinks = excluded.inlinks,
				word_count = excluded.word_count,
				issues = excluded.issues,
				updated_at = excluded.updated_at
		`, businessID, crawled.URL, crawled.FinalURL, crawled.PageType, crawled.PageTypeSource, crawled.StatusCode, crawled.Indexable, crawled.Title, strings.Join(crawled.H1, " | "), crawled.Depth, crawled.Inlinks, crawled.WordCount, len(crawled.Findings), updatedAt); err != nil {
			return rollback(fmt.Errorf("save crawled page %s: %w", crawled.URL, err))
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit dashboard update: %w", err)
	}
	return nil
}

func businessIdentity(site report.SiteReport) (string, string, string) {
	if site.GBP != nil && site.GBP.PlaceID != "" {
		return "place:" + site.GBP.PlaceID, site.GBP.Name, site.GBP.PlaceID
	}
	host := site.StartURL
	if parsed, err := url.Parse(site.StartURL); err == nil && parsed.Hostname() != "" {
		host = strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	}
	return "site:" + host, host, ""
}

func newHandler(database *sql.DB, runner *auditRunner) (http.Handler, error) {
	static, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, fmt.Errorf("open embedded dashboard: %w", err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/businesses", func(writer http.ResponseWriter, request *http.Request) {
		listBusinesses(writer, request, database)
	})
	mux.HandleFunc("GET /api/businesses/{id}/pages", func(writer http.ResponseWriter, request *http.Request) {
		listPages(writer, request, database)
	})
	mux.HandleFunc("GET /api/audits/current", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, runner.Current())
	})
	mux.HandleFunc("POST /api/audits", func(writer http.ResponseWriter, request *http.Request) {
		startAudit(writer, request, runner)
	})
	mux.Handle("/", http.FileServer(http.FS(static)))
	return mux, nil
}

func startAudit(writer http.ResponseWriter, request *http.Request, runner *auditRunner) {
	request.Body = http.MaxBytesReader(writer, request.Body, 64<<10)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var input auditRequest
	if err := decoder.Decode(&input); err != nil {
		http.Error(writer, "invalid audit request", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		http.Error(writer, "invalid audit request", http.StatusBadRequest)
		return
	}
	job, err := runner.Start(input)
	if errors.Is(err, errAuditRunning) {
		http.Error(writer, err.Error(), http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(writer, err.Error(), http.StatusBadRequest)
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(http.StatusAccepted)
	writeJSON(writer, job)
}

func listBusinesses(writer http.ResponseWriter, request *http.Request, database *sql.DB) {
	rows, err := database.QueryContext(request.Context(), `
		SELECT businesses.id, businesses.name, businesses.place_id, businesses.site_url,
			COUNT(pages.url), businesses.updated_at
		FROM businesses
		LEFT JOIN pages ON pages.business_id = businesses.id
		GROUP BY businesses.id
		ORDER BY businesses.updated_at DESC
	`)
	if err != nil {
		http.Error(writer, "could not load businesses", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := make([]business, 0)
	for rows.Next() {
		var item business
		if err := rows.Scan(&item.ID, &item.Name, &item.PlaceID, &item.SiteURL, &item.PageCount, &item.UpdatedAt); err != nil {
			http.Error(writer, "could not read businesses", http.StatusInternalServerError)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(writer, "could not read businesses", http.StatusInternalServerError)
		return
	}
	writeJSON(writer, items)
}

func listPages(writer http.ResponseWriter, request *http.Request, database *sql.DB) {
	businessID, err := strconv.ParseInt(request.PathValue("id"), 10, 64)
	if err != nil || businessID < 1 {
		http.Error(writer, "invalid business id", http.StatusBadRequest)
		return
	}
	rows, err := database.QueryContext(request.Context(), `
		SELECT url, final_url, page_type, page_type_source, status_code, indexable,
			title, h1, depth, inlinks, word_count, issues, updated_at
		FROM pages
		WHERE business_id = ?
		ORDER BY page_type, url
	`, businessID)
	if err != nil {
		http.Error(writer, "could not load pages", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	items := make([]page, 0)
	for rows.Next() {
		var item page
		if err := rows.Scan(&item.URL, &item.FinalURL, &item.PageType, &item.PageTypeSource, &item.StatusCode, &item.Indexable, &item.Title, &item.H1, &item.Depth, &item.Inlinks, &item.WordCount, &item.Issues, &item.UpdatedAt); err != nil {
			http.Error(writer, "could not read pages", http.StatusInternalServerError)
			return
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		http.Error(writer, "could not read pages", http.StatusInternalServerError)
		return
	}
	writeJSON(writer, items)
}

func writeJSON(writer http.ResponseWriter, value any) {
	writer.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		return
	}
}

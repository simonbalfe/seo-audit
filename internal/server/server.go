package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"time"

	dashboardui "github.com/simonbalfe/seo-audit/dashboard"
	"github.com/simonbalfe/seo-audit/internal/api"
	"github.com/simonbalfe/seo-audit/internal/evidence"
	"github.com/simonbalfe/seo-audit/internal/protocol"
	"github.com/simonbalfe/seo-audit/internal/storage"
	"github.com/simonbalfe/seo-audit/internal/webui"
)

type Config struct {
	Database     string
	Listen       string
	Workers      int
	JobRetention int
}

func DefaultConfig() Config {
	return Config{
		Listen:       "127.0.0.1:8787",
		Workers:      protocol.DefaultJobWorkers,
		JobRetention: protocol.DefaultJobRetention,
	}
}

func Run(ctx context.Context, config Config, output io.Writer) error {
	config, err := normalizeConfig(config)
	if err != nil {
		return err
	}
	store, err := openSQLiteStore(config.Database)
	if err != nil {
		return err
	}
	defer store.Close()
	static, err := dashboardui.FileSystem()
	if err != nil {
		return fmt.Errorf("load dashboard assets: %w", err)
	}
	listener, err := net.Listen("tcp", config.Listen)
	if err != nil {
		return fmt.Errorf("start API listener: %w", err)
	}
	defer listener.Close()
	server := &http.Server{
		Handler:           NewHandler(ctx, store, config, static),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()
	if output == nil {
		output = io.Discard
	}
	fmt.Fprintf(output, "SEO Audit API: http://%s/api/v1\n", listener.Addr().String())
	fmt.Fprintf(output, "Dashboard: http://%s\n", listener.Addr().String())
	fmt.Fprintf(output, "Database: %s\n", store.Path())
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func NewHandler(
	ctx context.Context,
	store *storage.SQLiteStore,
	config Config,
	static fs.FS,
) http.Handler {
	jobs := api.NewJobManager(ctx, config.Workers, config.JobRetention)
	apiService := api.NewService(store, jobs, config.Workers, config.JobRetention)
	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewHandler(apiService, evidence.NewService(store)))
	mux.Handle("/", webui.NewHandler(static))
	return mux
}

func ValidateAddress(address string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return fmt.Errorf("invalid API listen address: %w", err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("API must listen on localhost or a loopback IP")
	}
	return nil
}

func normalizeConfig(config Config) (Config, error) {
	defaults := DefaultConfig()
	if strings.TrimSpace(config.Listen) == "" {
		config.Listen = defaults.Listen
	}
	if config.Workers == 0 {
		config.Workers = defaults.Workers
	}
	if config.JobRetention == 0 {
		config.JobRetention = defaults.JobRetention
	}
	if err := ValidateAddress(config.Listen); err != nil {
		return Config{}, err
	}
	if config.Workers < 1 || config.Workers > 32 {
		return Config{}, errors.New("API workers must be from 1 to 32")
	}
	if config.JobRetention < 1 || config.JobRetention > 10000 {
		return Config{}, errors.New("API job retention must be from 1 to 10000")
	}
	return config, nil
}

func openSQLiteStore(configuredPath string) (*storage.SQLiteStore, error) {
	path := strings.TrimSpace(configuredPath)
	if path == "" {
		var err error
		path, err = storage.DefaultPath()
		if err != nil {
			return nil, err
		}
	}
	return storage.OpenSQLite(path, storage.DefaultSnapshotRetention)
}

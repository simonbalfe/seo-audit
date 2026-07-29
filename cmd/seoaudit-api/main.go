package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/simonbalfe/seo-audit/internal/server"
)

func main() {
	config := server.DefaultConfig()
	flag.StringVar(&config.Database, "db", "", "API SQLite path; defaults to the user config directory")
	flag.StringVar(&config.Listen, "listen", config.Listen, "local API listen address")
	flag.IntVar(&config.Workers, "workers", config.Workers, "maximum concurrent API jobs")
	flag.IntVar(
		&config.JobRetention,
		"job-retention",
		config.JobRetention,
		"maximum in-memory queued, running, and completed API jobs",
	)
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := server.Run(ctx, config, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

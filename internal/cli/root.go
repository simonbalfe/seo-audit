package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/simonbalfe/seo-audit/internal/dataforseo"
	"github.com/spf13/cobra"
)

type options struct {
	json          bool
	verbose       bool
	timeout       time.Duration
	limit         int
	checkExternal bool
	performance   bool
	dataForSEO    bool
	location      string
	language      string
	dataLimit     int
}

type issueGroup struct {
	priority string
	category string
	check    string
	fix      string
	items    []audit.Finding
}

type completeReport struct {
	audit.SiteReport
	SearchData *dataforseo.Report `json:"search_data,omitempty"`
}

func Execute(ctx context.Context) error {
	root := &cobra.Command{
		Use:           "seoaudit",
		Short:         "Deep public website SEO auditor",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	root.AddCommand(newAuditCommand())
	return root.ExecuteContext(ctx)
}

func newAuditCommand() *cobra.Command {
	opts := &options{}
	command := &cobra.Command{
		Use:   "audit <url>",
		Short: "Crawl and audit a public website",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return opts.runAudit(command, args[0])
		},
	}
	command.Flags().BoolVar(&opts.json, "json", false, "print the complete machine-readable report")
	command.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "show crawl and analysis progress")
	command.Flags().DurationVar(&opts.timeout, "timeout", 30*time.Second, "timeout for each request")
	command.Flags().IntVar(&opts.limit, "limit", 500, "maximum pages to audit")
	command.Flags().BoolVar(&opts.checkExternal, "external", true, "check discovered external links")
	command.Flags().BoolVar(&opts.performance, "performance", true, "test representative pages with local Chrome")
	command.Flags().BoolVar(&opts.dataForSEO, "dataforseo", false, "add paid search, keyword, competitor, and backlink data")
	command.Flags().StringVar(&opts.location, "location", "United Kingdom", "DataForSEO search location")
	command.Flags().StringVar(&opts.language, "language", "en", "DataForSEO language code")
	command.Flags().IntVar(&opts.dataLimit, "data-limit", 25, "maximum rows per DataForSEO dataset")
	return command
}

func (opts options) runAudit(command *cobra.Command, rawURL string) error {
	var searchClient *dataforseo.Client
	if opts.dataForSEO {
		var err error
		searchClient, err = dataforseo.NewClient()
		if err != nil {
			return err
		}
	}

	var crawlProgress func(audit.ProgressEvent)
	if opts.verbose {
		crawlProgress = func(event audit.ProgressEvent) {
			fmt.Fprintf(command.ErrOrStderr(), "[%s] %s\n", event.Stage, event.Message)
		}
	}
	report, err := audit.NewClient(opts.timeout).Audit(command.Context(), rawURL, audit.Options{
		Limit:            opts.limit,
		CheckExternal:    opts.checkExternal,
		CheckPerformance: opts.performance,
		Progress:         crawlProgress,
	})
	if err != nil {
		return err
	}

	searchData, err := opts.loadSearchData(command, searchClient, report.StartURL)
	if err != nil {
		return err
	}
	output := command.OutOrStdout()
	if opts.json {
		return printJSON(output, completeReport{SiteReport: report, SearchData: searchData})
	}
	printReport(output, report, searchData)
	return nil
}

func (opts options) loadSearchData(command *cobra.Command, client *dataforseo.Client, startURL string) (*dataforseo.Report, error) {
	if client == nil {
		return nil, nil
	}
	target, err := domainTarget(startURL)
	if err != nil {
		return nil, err
	}
	var progress func(string, string)
	if opts.verbose {
		progress = func(dataset, message string) {
			fmt.Fprintf(command.ErrOrStderr(), "[dataforseo:%s] %s\n", dataset, message)
		}
	}
	report := client.Audit(command.Context(), dataforseo.Options{
		Target:   target,
		Location: opts.location,
		Language: opts.language,
		Limit:    opts.dataLimit,
		Progress: progress,
	})
	return &report, nil
}

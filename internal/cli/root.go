package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/simonbalfe/seo-audit/internal/apiclient"
	"github.com/simonbalfe/seo-audit/internal/protocol"
	"github.com/spf13/cobra"
)

type clientOptions struct {
	apiURL string
}

type auditOptions struct {
	client        *clientOptions
	json          bool
	verbose       bool
	save          bool
	timeout       time.Duration
	limit         int
	checkExternal bool
	performance   bool
}

type opportunityOptions struct {
	client     *clientOptions
	json       bool
	verbose    bool
	dataForSEO bool
	location   string
	language   string
	dataLimit  int
	gsc        bool
	gscSite    string
	gscDays    int
	gscLimit   int
	save       bool
	cacheTTL   time.Duration
	refresh    bool
}

type backlinkOptions struct {
	client     *clientOptions
	json       bool
	verbose    bool
	dataForSEO bool
	dataLimit  int
	cacheTTL   time.Duration
	refresh    bool
}

type issueGroup struct {
	priority string
	category string
	check    string
	fix      string
	items    []auditFinding
}

func Execute(ctx context.Context) error {
	client := &clientOptions{}
	root := &cobra.Command{
		Use:           "seoaudit",
		Short:         "Evidence-based website and search analysis",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	root.PersistentFlags().StringVar(
		&client.apiURL,
		"api-url",
		"",
		"SEO Audit API base URL; defaults to SEOAUDIT_API_URL or http://127.0.0.1:8787",
	)
	root.AddCommand(
		newAuditCommand(client),
		newOpportunitiesCommand(client),
		newBacklinksCommand(client),
		newRankingsCommand(client),
	)
	return root.ExecuteContext(ctx)
}

func newAuditCommand(client *clientOptions) *cobra.Command {
	opts := &auditOptions{client: client}
	command := &cobra.Command{
		Use:   "audit <url>",
		Short: "Request a public technical, on-page, and performance audit",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return opts.run(command, args[0])
		},
	}
	command.Flags().BoolVar(&opts.json, "json", false, "print the complete machine-readable report")
	command.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "stream API job progress")
	command.Flags().BoolVar(&opts.save, "save", false, "save the completed audit in the API database")
	command.Flags().DurationVar(&opts.timeout, "timeout", 30*time.Second, "timeout for each audited request")
	command.Flags().IntVar(&opts.limit, "limit", 500, "maximum pages to audit")
	command.Flags().BoolVar(&opts.checkExternal, "external", true, "check discovered external links")
	command.Flags().BoolVar(&opts.performance, "performance", true, "test representative pages with local Chrome")
	return command
}

func newOpportunitiesCommand(client *clientOptions) *cobra.Command {
	opts := &opportunityOptions{client: client}
	command := &cobra.Command{
		Use:   "opportunities <url>",
		Short: "Request search query, ranking, keyword, and competitor analysis",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return opts.run(command, args[0])
		},
	}
	command.Flags().BoolVar(&opts.json, "json", false, "print the complete machine-readable report")
	command.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "stream API job progress")
	command.Flags().BoolVar(&opts.gsc, "gsc", false, "use authenticated Google Search Console query and page data")
	command.Flags().StringVar(&opts.gscSite, "gsc-site", "", "Search Console property, such as sc-domain:example.com")
	command.Flags().IntVar(&opts.gscDays, "gsc-days", 28, "finalized Search Console lookback window in days")
	command.Flags().IntVar(&opts.gscLimit, "gsc-limit", 250, "maximum Search Console query/page rows")
	command.Flags().BoolVar(&opts.save, "save", false, "save Search Console data in the API database")
	command.Flags().BoolVar(&opts.dataForSEO, "dataforseo", false, "use paid DataForSEO rankings, keywords, and competitor data")
	command.Flags().StringVar(&opts.location, "location", "United Kingdom", "DataForSEO search location")
	command.Flags().StringVar(&opts.language, "language", "en", "DataForSEO language code")
	command.Flags().IntVar(&opts.dataLimit, "data-limit", 25, "maximum rows per DataForSEO dataset")
	addProviderFlags(command, &opts.cacheTTL, &opts.refresh)
	return command
}

func newBacklinksCommand(client *clientOptions) *cobra.Command {
	opts := &backlinkOptions{client: client}
	command := &cobra.Command{
		Use:   "backlinks <url>",
		Short: "Request backlink authority, referring-domain, and link analysis",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			return opts.run(command, args[0])
		},
	}
	command.Flags().BoolVar(&opts.json, "json", false, "print the complete machine-readable report")
	command.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "stream API job progress")
	command.Flags().BoolVar(&opts.dataForSEO, "dataforseo", false, "use the paid DataForSEO backlink index")
	command.Flags().IntVar(&opts.dataLimit, "data-limit", 25, "maximum referring domains and backlinks")
	addProviderFlags(command, &opts.cacheTTL, &opts.refresh)
	return command
}

func addProviderFlags(command *cobra.Command, cacheTTL *time.Duration, refresh *bool) {
	command.Flags().DurationVar(
		cacheTTL,
		"cache-ttl",
		protocol.DefaultProviderCacheTTL,
		"request reuse of complete paid provider results for this duration",
	)
	command.Flags().BoolVar(refresh, "refresh", false, "bypass cached provider data and request a fresh paid snapshot")
}

func (opts auditOptions) run(command *cobra.Command, rawURL string) error {
	external := opts.checkExternal
	performance := opts.performance
	request := protocol.AuditRequest{
		URL:                   rawURL,
		PageLimit:             opts.limit,
		RequestTimeoutSeconds: durationSeconds(opts.timeout),
		CheckExternal:         &external,
		CheckPerformance:      &performance,
		Save:                  opts.save,
	}
	data, err := opts.runJob(command, "/api/v1/audits", request)
	if err != nil {
		return err
	}
	var report auditReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("decode audit result: %w", err)
	}
	if opts.json {
		return printRawJSON(command.OutOrStdout(), data)
	}
	printReport(command.OutOrStdout(), report)
	return nil
}

func (opts opportunityOptions) run(command *cobra.Command, rawURL string) error {
	if !opts.gsc && !opts.dataForSEO {
		return errors.New("select at least one source with --gsc or --dataforseo")
	}
	sources := make([]string, 0, 2)
	if opts.gsc {
		sources = append(sources, "gsc")
	}
	if opts.dataForSEO {
		sources = append(sources, "dataforseo")
	}
	request := protocol.OpportunityRequest{
		URL:     rawURL,
		Sources: sources,
		GSC: protocol.GSCRequest{
			SiteURL:  opts.gscSite,
			Days:     opts.gscDays,
			RowLimit: opts.gscLimit,
			Save:     opts.save,
		},
		DataForSEO: protocol.DataForSEORequest{
			Location:        opts.location,
			Language:        opts.language,
			RowLimit:        opts.dataLimit,
			CacheTTLSeconds: int64(opts.cacheTTL / time.Second),
			Refresh:         opts.refresh,
		},
	}
	data, err := opts.runJob(command, "/api/v1/opportunities", request)
	if err != nil {
		return err
	}
	var report opportunityReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("decode opportunity result: %w", err)
	}
	if opts.json {
		return printRawJSON(command.OutOrStdout(), data)
	}
	printOpportunityReport(command.OutOrStdout(), report)
	return nil
}

func (opts backlinkOptions) run(command *cobra.Command, rawURL string) error {
	if !opts.dataForSEO {
		return errors.New("backlink analysis requires the paid DataForSEO source; rerun with --dataforseo")
	}
	request := protocol.BacklinkRequest{
		URL:             rawURL,
		Source:          "dataforseo",
		RowLimit:        opts.dataLimit,
		CacheTTLSeconds: int64(opts.cacheTTL / time.Second),
		Refresh:         opts.refresh,
	}
	data, err := opts.runJob(command, "/api/v1/backlinks", request)
	if err != nil {
		return err
	}
	var report providerReport
	if err := json.Unmarshal(data, &report); err != nil {
		return fmt.Errorf("decode backlink result: %w", err)
	}
	if opts.json {
		return printRawJSON(command.OutOrStdout(), data)
	}
	printBacklinkReport(command.OutOrStdout(), report)
	return nil
}

func (opts auditOptions) runJob(command *cobra.Command, path string, request any) ([]byte, error) {
	return runAPIJob(command, opts.client, opts.verbose, path, request)
}

func (opts opportunityOptions) runJob(command *cobra.Command, path string, request any) ([]byte, error) {
	return runAPIJob(command, opts.client, opts.verbose, path, request)
}

func (opts backlinkOptions) runJob(command *cobra.Command, path string, request any) ([]byte, error) {
	return runAPIJob(command, opts.client, opts.verbose, path, request)
}

func runAPIJob(
	command *cobra.Command,
	options *clientOptions,
	verbose bool,
	path string,
	request any,
) ([]byte, error) {
	client, err := newAPIClient(options)
	if err != nil {
		return nil, err
	}
	var progress func(protocol.JobEvent)
	if verbose {
		progress = func(event protocol.JobEvent) {
			fmt.Fprintf(command.ErrOrStderr(), "[%s] %s\n", event.Stage, event.Message)
		}
	}
	return client.SubmitAndWait(command.Context(), path, request, progress)
}

func newAPIClient(options *clientOptions) (*apiclient.Client, error) {
	if options == nil {
		options = &clientOptions{}
	}
	return apiclient.New(options.apiURL)
}

func durationSeconds(value time.Duration) int {
	if value <= 0 {
		return 0
	}
	seconds := int(value / time.Second)
	if value%time.Second != 0 {
		seconds++
	}
	return seconds
}

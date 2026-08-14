package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/simonbalfe/seo-audit/dashboard"
	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/simonbalfe/seo-audit/internal/places"
	"github.com/simonbalfe/seo-audit/internal/report"
)

type auditOptions struct {
	json          bool
	outputPath    string
	debug         bool
	timeout       time.Duration
	limit         int
	checkExternal bool
	performance   bool
	keywords      []string
	steps         string
	website       string
	dashboard     bool
}

func Execute(ctx context.Context) error {
	err := execute(ctx, os.Args[1:], os.Stdout, os.Stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func execute(ctx context.Context, args []string, output, errorOutput io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: seoaudit audit <place-id> [options]")
	}
	switch args[0] {
	case "audit":
		return runAudit(ctx, args[1:], output, errorOutput)
	case "help", "-h", "--help":
		return runAudit(ctx, []string{"--help"}, output, errorOutput)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func runAudit(ctx context.Context, args []string, output, errorOutput io.Writer) error {
	opts := auditOptions{timeout: 30 * time.Second, limit: 50, checkExternal: true, performance: true, steps: "all"}
	flags := flag.NewFlagSet("seoaudit audit", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	flags.BoolVar(&opts.json, "json", false, "print the complete machine-readable report")
	flags.StringVar(&opts.outputPath, "output", "", "Output: override the automatic output/<timestamp>-<host>.json path")
	flags.BoolVar(&opts.debug, "debug", false, "Logging: show each audit stage and fetched page")
	flags.DurationVar(&opts.timeout, "timeout", opts.timeout, "Crawl: timeout for each request")
	flags.IntVar(&opts.limit, "limit", opts.limit, "Crawl: maximum pages")
	flags.BoolVar(&opts.checkExternal, "external", opts.checkExternal, "Technical: check external links; use --external=false to skip")
	flags.BoolVar(&opts.performance, "performance", opts.performance, "Performance: test representative pages; use --performance=false to skip")
	flags.StringVar(&opts.steps, "steps", opts.steps, "Run: all, website, performance, visibility, backlinks, or profile")
	flags.StringVar(&opts.website, "website", "", "Website: canonical URL to audit when the GBP has none")
	flags.Func("keyword", "Keywords: local query to include in the five-query visibility shortlist; repeat for more queries", func(value string) error {
		opts.keywords = append(opts.keywords, value)
		return nil
	})
	flags.BoolVar(&opts.dashboard, "dashboard", false, "Dashboard: serve saved businesses and crawled pages at http://127.0.0.1:4173; no Place ID required")
	flags.Usage = func() {
		fmt.Fprintln(errorOutput, "usage: seoaudit audit <place-id> [options]")
		fmt.Fprintln(errorOutput, "       seoaudit audit --dashboard")
		fmt.Fprintln(errorOutput, "\nRuns: resolve Place ID and website; crawl technical and on-page SEO; check GBP, backlinks, keywords, organic and Maps ranks, competitors, and a 3x3 grid over 2 km for up to five selected commercial keywords.")
		fmt.Fprintln(errorOutput, "Crawl: 50 pages by default (1-5000), 30s per request (120s maximum), 16 page fetches, four Chrome renders, 24 resource checks, 5 MiB pages, 10 redirects, and six performance pages.")
		fmt.Fprintln(errorOutput, "Backlinks: one paid live DataForSEO domain-summary request including subdomains.")
		fmt.Fprintln(errorOutput, "Workflows: all, website, performance, visibility, backlinks, or profile. Each partial workflow still resolves the exact Place ID; website-dependent workflows also resolve its public website.")
		fmt.Fprintln(errorOutput, "Providers: GOOGLE_MAPS_API_KEY is always required. DataForSEO is required by all, visibility, and backlinks. OpenRouter is required by all and visibility. Provider calls are paid.")
		fmt.Fprintln(errorOutput)
		flags.PrintDefaults()
	}
	positionals, err := parseCommandFlags(flags, args)
	if err != nil {
		return err
	}
	if opts.dashboard {
		if len(positionals) != 0 {
			return errors.New("--dashboard does not accept a Place ID")
		}
		fmt.Fprintln(output, "SEO Audit dashboard: http://127.0.0.1:4173")
		return dashboard.Serve(ctx, filepath.Join("output", "audits.sqlite"), "127.0.0.1:4173")
	}
	if len(positionals) != 1 {
		return errors.New("usage: seoaudit audit <place-id> [options]")
	}
	return opts.run(ctx, output, errorOutput, positionals[0])
}

func parseCommandFlags(flags *flag.FlagSet, args []string) ([]string, error) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positional := args[0]
		if err := flags.Parse(args[1:]); err != nil {
			return nil, err
		}
		return append([]string{positional}, flags.Args()...), nil
	}
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	return flags.Args(), nil
}

func (opts auditOptions) run(ctx context.Context, output, errorOutput io.Writer, placeID string) error {
	if opts.steps == "" {
		opts.steps = "all"
	}
	if err := opts.validate(); err != nil {
		return err
	}
	if err := opts.loadProviderCredentials(); err != nil {
		return err
	}
	var progress func(audit.ProgressEvent)
	if opts.debug {
		progress = func(event audit.ProgressEvent) {
			fmt.Fprintf(errorOutput, "[%s] %s\n", event.Stage, event.Message)
		}
	}
	client, err := places.NewClient()
	if err != nil {
		return err
	}
	if opts.debug {
		fmt.Fprintln(errorOutput, "[places] Resolving the Place ID, public website, and business profile")
	}
	profile, err := client.AuditPlace(ctx, placeID)
	if err != nil {
		return err
	}
	rawURL := strings.TrimSpace(profile.Website)
	if opts.steps != "profile" {
		rawURL, err = websiteForPlace(placeID, profile, opts.website)
		if err != nil {
			return err
		}
	}
	if opts.debug {
		fmt.Fprintf(errorOutput, "[places] Target: %s at %.7f, %.7f; website: %s\n", profile.Name, profile.Latitude, profile.Longitude, profile.Website)
	}
	outputPath := opts.outputPath
	if outputPath == "" {
		outputPath = automaticOutputPath(rawURL, time.Now())
	}
	if opts.steps == "profile" {
		return opts.saveProfile(ctx, output, errorOutput, rawURL, outputPath, profile)
	}
	if opts.steps == "backlinks" {
		return opts.runBacklinks(ctx, output, errorOutput, rawURL, outputPath, profile, progress)
	}
	auditOptions := audit.Options{
		Limit:                   opts.limit,
		CheckExternal:           opts.checkExternal,
		CheckPerformance:        opts.steps == "performance" || (opts.steps == "all" && opts.performance),
		CheckBacklinks:          opts.steps == "all",
		ClassificationCachePath: filepath.Join(filepath.Dir(outputPath), "classifications.sqlite"),
		Progress:                progress,
	}
	if opts.steps == "all" || opts.steps == "visibility" {
		auditOptions.Market = &audit.MarketOptions{
			Location:        profile.Market,
			Language:        "en",
			MaxChecks:       5,
			Keywords:        opts.keywords,
			TargetName:      profile.Name,
			TargetCategory:  profile.Category,
			TargetCountry:   profile.Country,
			TargetPlaceID:   profile.PlaceID,
			TargetLatitude:  profile.Latitude,
			TargetLongitude: profile.Longitude,
			GridRadiusKM:    2,
		}
	}
	siteReport, err := audit.Run(ctx, rawURL, opts.timeout, auditOptions)
	if err != nil {
		return err
	}
	siteReport.GBP = &profile
	*siteReport.GBP = audit.VerifyBusinessIdentity(siteReport, *siteReport.GBP)
	if opts.debug {
		fmt.Fprintf(errorOutput, "[places] Audited %s\n", siteReport.GBP.Name)
	}
	if err := saveJSON(outputPath, siteReport); err != nil {
		return err
	}
	fmt.Fprintf(errorOutput, "Saved JSON report to %s\n", outputPath)
	if err := dashboard.Save(ctx, filepath.Join("output", "audits.sqlite"), siteReport); err != nil {
		return err
	}
	fmt.Fprintln(errorOutput, "Updated dashboard data in output/audits.sqlite")
	if opts.json {
		return printJSON(output, siteReport)
	}
	printReport(output, siteReport)
	return nil
}

func (opts auditOptions) saveProfile(ctx context.Context, output, errorOutput io.Writer, rawURL, outputPath string, profile report.GBPAuditReport) error {
	siteReport := report.SiteReport{StartURL: rawURL, GBP: &profile}
	if err := saveJSON(outputPath, siteReport); err != nil {
		return err
	}
	fmt.Fprintf(errorOutput, "Saved JSON report to %s\n", outputPath)
	if err := dashboard.Save(ctx, filepath.Join("output", "audits.sqlite"), siteReport); err != nil {
		return err
	}
	fmt.Fprintln(errorOutput, "Updated dashboard data in output/audits.sqlite")
	if opts.json {
		return printJSON(output, siteReport)
	}
	printGBPAuditReport(output, profile)
	return nil
}

func (opts auditOptions) runBacklinks(ctx context.Context, output, errorOutput io.Writer, rawURL, outputPath string, profile report.GBPAuditReport, progress func(audit.ProgressEvent)) error {
	backlinks := audit.Backlinks(ctx, rawURL, progress)
	if backlinks.Error != "" {
		return fmt.Errorf("backlink summary: %s", backlinks.Error)
	}
	siteReport := report.SiteReport{StartURL: rawURL, Backlinks: backlinks, GBP: &profile}
	if err := saveJSON(outputPath, siteReport); err != nil {
		return err
	}
	fmt.Fprintf(errorOutput, "Saved JSON report to %s\n", outputPath)
	if opts.json {
		return printJSON(output, siteReport)
	}
	fmt.Fprintf(output, "Backlink audit: %s\n", profile.Name)
	printBacklinkReport(output, backlinks)
	return nil
}

func websiteForPlace(placeID string, profile report.GBPAuditReport, override string) (string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		parsed, err := url.Parse(override)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
			return "", errors.New("--website must be an absolute http or https URL")
		}
		return override, nil
	}
	website := strings.TrimSpace(profile.Website)
	if website == "" {
		return "", fmt.Errorf("Google Place %q has no public website to audit", placeID)
	}
	return website, nil
}

func automaticOutputPath(rawURL string, now time.Time) string {
	host := "audit"
	parsed, err := url.Parse(rawURL)
	if err == nil && parsed.Hostname() == "" {
		parsed, err = url.Parse("https://" + rawURL)
	}
	if err == nil && parsed.Hostname() != "" {
		host = strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	}
	host = strings.ReplaceAll(host, ":", "_")
	return filepath.Join("output", now.UTC().Format("20060102T150405.000Z")+"-"+host+".json")
}

func (opts auditOptions) validate() error {
	switch opts.steps {
	case "", "all", "website", "performance", "visibility", "backlinks", "profile":
	default:
		return errors.New("--steps must be all, website, performance, visibility, backlinks, or profile")
	}
	if opts.limit < 1 || opts.limit > 5000 {
		return errors.New("--limit must be from 1 to 5000")
	}
	if opts.timeout <= 0 || opts.timeout > 120*time.Second {
		return errors.New("--timeout must be greater than zero and no more than 120s")
	}
	return nil
}

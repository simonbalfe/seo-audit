package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/spf13/cobra"
)

type options struct {
	json    bool
	all     bool
	timeout time.Duration
}

type auditReport struct {
	Page     audit.PageReport    `json:"page"`
	Robots   audit.RobotsReport  `json:"robots"`
	Sitemaps audit.SitemapReport `json:"sitemaps"`
}

func Execute(ctx context.Context) error {
	opts := &options{}
	root := &cobra.Command{
		Use:           "seoaudit",
		Short:         "Public website SEO auditor",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().BoolVar(&opts.json, "json", false, "print JSON")
	root.PersistentFlags().BoolVar(&opts.all, "all", false, "show passing checks as well as actions")
	root.PersistentFlags().DurationVar(&opts.timeout, "timeout", 20*time.Second, "request timeout")
	root.AddCommand(
		newAuditCommand(opts),
		newPageCommand(opts),
		newRobotsCommand(opts),
		newSitemapCommand(opts),
		newRoadmapCommand(),
	)
	return root.ExecuteContext(ctx)
}

func newAuditCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "audit <url>",
		Short: "Run every Stage 1 public check",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			client := audit.NewClient(opts.timeout)
			page, err := client.InspectPage(command.Context(), args[0])
			if err != nil {
				return err
			}
			robots, err := client.InspectRobots(command.Context(), args[0])
			if err != nil {
				return err
			}
			sitemaps, err := client.InspectSitemaps(command.Context(), args[0])
			if err != nil {
				return err
			}
			report := auditReport{Page: page, Robots: robots, Sitemaps: sitemaps}
			if opts.json {
				return printJSON(report)
			}
			printPage(page, opts.all)
			fmt.Println()
			printRobots(robots)
			fmt.Println()
			printSitemaps(sitemaps)
			return nil
		},
	}
}

func newPageCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "page <url>",
		Short: "Inspect one public HTML page",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			report, err := audit.NewClient(opts.timeout).InspectPage(command.Context(), args[0])
			if err != nil {
				return err
			}
			if opts.json {
				return printJSON(report)
			}
			printPage(report, opts.all)
			return nil
		},
	}
}

func newRobotsCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "robots <url>",
		Short: "Check public search and AI crawler access",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			report, err := audit.NewClient(opts.timeout).InspectRobots(command.Context(), args[0])
			if err != nil {
				return err
			}
			if opts.json {
				return printJSON(report)
			}
			printRobots(report)
			return nil
		},
	}
}

func newSitemapCommand(opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "sitemap <url>",
		Short: "Discover and read public XML sitemaps",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			report, err := audit.NewClient(opts.timeout).InspectSitemaps(command.Context(), args[0])
			if err != nil {
				return err
			}
			if opts.json {
				return printJSON(report)
			}
			printSitemaps(report)
			return nil
		},
	}
}

func newRoadmapCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "roadmap",
		Short: "Show implemented and planned audit stages",
		Run: func(command *cobra.Command, args []string) {
			writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(writer, "STAGE\tSTATUS\tSCOPE")
			fmt.Fprintln(writer, "1\tbuilt\tSingle page, robots.txt, sitemap discovery")
			fmt.Fprintln(writer, "2\tplanned\tBounded crawl, redirects, broken links, duplicate metadata")
			fmt.Fprintln(writer, "3\tplanned\tSite structure, internal links, content inventory")
			fmt.Fprintln(writer, "4\tplanned\tPublic GEO readability, entities, evidence, freshness")
			fmt.Fprintln(writer, "5\tplanned\tSaved baselines, change detection, reports")
			writer.Flush()
		},
	}
}

func printPage(report audit.PageReport, showAll bool) {
	pass, warnings, failures := findingCounts(report.Findings)
	fmt.Printf("Page: %s\n", report.URL)
	if report.FinalURL != report.URL {
		fmt.Printf("Final URL: %s\n", report.FinalURL)
	}
	fmt.Printf("Result: %d failures, %d warnings, %d passes\n", failures, warnings, pass)
	fmt.Printf("Title: %s\n", valueOr(report.Title, "missing"))
	fmt.Printf("Content: %d words, %d internal links, %d external links\n", report.WordCount, len(report.InternalLinks), len(report.ExternalLinks))
	fmt.Println()
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "STATUS\tAREA\tCHECK\tEVIDENCE\tACTION")
	for _, finding := range report.Findings {
		if !showAll && finding.Status == audit.Pass {
			continue
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
			strings.ToUpper(string(finding.Status)),
			finding.Category,
			finding.Check,
			oneLine(finding.Evidence),
			oneLine(finding.Fix),
		)
	}
	writer.Flush()
}

func printRobots(report audit.RobotsReport) {
	fmt.Printf("Robots: %s returned %d\n", report.URL, report.StatusCode)
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "AGENT\tACCESS\tMATCHED RULE")
	for _, item := range report.Agents {
		access := "blocked"
		if item.Allowed {
			access = "allowed"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\n", item.Agent, access, item.Rule)
	}
	writer.Flush()
}

func printSitemaps(report audit.SitemapReport) {
	fmt.Printf("Sitemaps: %d files, %d URLs\n", len(report.Sources), len(report.URLs))
	for _, source := range report.Sources {
		fmt.Println(" ", source)
	}
	for _, item := range report.Errors {
		fmt.Println(" warning:", item)
	}
}

func findingCounts(findings []audit.Finding) (int, int, int) {
	var pass, warnings, failures int
	for _, finding := range findings {
		switch finding.Status {
		case audit.Pass:
			pass++
		case audit.Warn:
			warnings++
		case audit.Fail:
			failures++
		}
	}
	return pass, warnings, failures
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func valueOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

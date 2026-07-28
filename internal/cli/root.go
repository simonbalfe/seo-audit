package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/simonbalfe/seo-audit/internal/audit"
	"github.com/spf13/cobra"
)

type options struct {
	json          bool
	verbose       bool
	timeout       time.Duration
	limit         int
	checkExternal bool
}

type issueGroup struct {
	Priority string
	Category string
	Check    string
	Fix      string
	Items    []audit.Finding
}

func Execute(ctx context.Context) error {
	opts := &options{}
	root := &cobra.Command{
		Use:           "seoaudit",
		Short:         "Deep public website SEO auditor",
		SilenceUsage:  true,
		SilenceErrors: true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}
	command := &cobra.Command{
		Use:   "audit <url>",
		Short: "Crawl and audit a public website",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			var progress func(audit.ProgressEvent)
			if opts.verbose {
				progress = func(event audit.ProgressEvent) {
					fmt.Fprintf(command.ErrOrStderr(), "[%s] %s\n", event.Stage, event.Message)
				}
			}
			report, err := audit.NewClient(opts.timeout).Audit(command.Context(), args[0], audit.Options{
				Limit:         opts.limit,
				CheckExternal: opts.checkExternal,
				Progress:      progress,
			})
			if err != nil {
				return err
			}
			if opts.json {
				return printJSON(report)
			}
			printReport(report)
			return nil
		},
	}
	command.Flags().BoolVar(&opts.json, "json", false, "print the complete machine-readable report")
	command.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "show crawl and analysis progress")
	command.Flags().DurationVar(&opts.timeout, "timeout", 30*time.Second, "timeout for each request")
	command.Flags().IntVar(&opts.limit, "limit", 500, "maximum pages to audit")
	command.Flags().BoolVar(&opts.checkExternal, "external", true, "check discovered external links")
	root.AddCommand(command)
	return root.ExecuteContext(ctx)
}

func printReport(report audit.SiteReport) {
	fmt.Printf("SEO audit: %s\n", report.StartURL)
	fmt.Printf("Crawled: %d URLs (%d indexable, %d non-indexable) in %.1fs\n",
		report.Summary.Pages,
		report.Summary.Indexable,
		report.Summary.NonIndexable,
		float64(report.Duration)/1000,
	)
	fmt.Printf("Discovered: %d internal links, %d external links, %d sitemap URLs\n",
		report.Summary.InternalLinks,
		report.Summary.ExternalLinks,
		report.Summary.SitemapURLs,
	)
	fmt.Printf("Actions: %d failures, %d warnings\n", report.Summary.Failures, report.Summary.Warnings)
	if report.LimitReached {
		fmt.Println("Warning: crawl limit reached; rerun with a higher --limit for complete coverage.")
	}
	fmt.Println()

	groups := groupFindings(report.Findings)
	if len(groups) == 0 {
		fmt.Println("No deterministic issues found in the public crawl.")
		return
	}
	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "PRIORITY\tAREA\tISSUE\tOCCURRENCES\tEXAMPLE\tFIX")
	for _, group := range groups {
		example := ""
		if len(group.Items) > 0 {
			example = group.Items[0].URL
			if group.Items[0].Evidence != "" {
				example = strings.TrimSpace(example + " " + group.Items[0].Evidence)
			}
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%d\t%s\t%s\n",
			strings.ToUpper(group.Priority),
			group.Category,
			group.Check,
			len(group.Items),
			oneLine(example),
			oneLine(group.Fix),
		)
	}
	writer.Flush()
	fmt.Println()
	fmt.Println("Use --json for every affected URL and the complete crawl dataset.")
}

func groupFindings(findings []audit.Finding) []issueGroup {
	grouped := map[string]*issueGroup{}
	for _, finding := range findings {
		key := finding.Priority + "\x00" + finding.Category + "\x00" + finding.Check + "\x00" + finding.Fix
		if grouped[key] == nil {
			grouped[key] = &issueGroup{
				Priority: finding.Priority,
				Category: finding.Category,
				Check:    finding.Check,
				Fix:      finding.Fix,
			}
		}
		grouped[key].Items = append(grouped[key].Items, finding)
	}
	result := make([]issueGroup, 0, len(grouped))
	for _, group := range grouped {
		result = append(result, *group)
	}
	priority := map[string]int{"high": 0, "medium": 1, "low": 2}
	sort.Slice(result, func(i, j int) bool {
		if priority[result[i].Priority] != priority[result[j].Priority] {
			return priority[result[i].Priority] < priority[result[j].Priority]
		}
		if result[i].Category != result[j].Category {
			return result[i].Category < result[j].Category
		}
		return result[i].Check < result[j].Check
	})
	return result
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

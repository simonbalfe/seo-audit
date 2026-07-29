package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"

	"github.com/simonbalfe/seo-audit/internal/protocol"
	"github.com/spf13/cobra"
)

type rankMarketOptions struct {
	client   *clientOptions
	json     bool
	location string
	language string
}

type rankAddOptions struct {
	rankMarketOptions
	device string
	depth  int
}

type rankCheckOptions struct {
	rankMarketOptions
	dataForSEO bool
	verbose    bool
}

func newRankingsCommand(client *clientOptions) *cobra.Command {
	command := &cobra.Command{
		Use:   "rankings",
		Short: "Manage API-backed Google keyword tracking",
	}
	command.AddCommand(
		newRankingsAddCommand(client),
		newRankingsRemoveCommand(client),
		newRankingsCheckCommand(client),
		newRankingsReportCommand(client),
	)
	return command
}

func newRankingsAddCommand(client *clientOptions) *cobra.Command {
	opts := &rankAddOptions{rankMarketOptions: rankMarketOptions{client: client}}
	command := &cobra.Command{
		Use:   "add <url> <keyword> [keyword...]",
		Short: "Add keywords to an API-backed rank tracker",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			apiClient, err := newAPIClient(opts.client)
			if err != nil {
				return err
			}
			var update rankKeywordUpdate
			err = apiClient.Post(command.Context(), "/api/v1/rank-trackers", protocol.RankTrackerRequest{
				URL:       args[0],
				Location:  opts.location,
				Language:  opts.language,
				Devices:   opts.device,
				SERPDepth: opts.depth,
				Keywords:  args[1:],
			}, &update)
			if err != nil {
				return err
			}
			if opts.json {
				return printJSON(command.OutOrStdout(), update)
			}
			printRankKeywordUpdate(command.OutOrStdout(), "Added", update.Added, update)
			return nil
		},
	}
	addRankMarketFlags(command, &opts.rankMarketOptions)
	command.Flags().StringVar(&opts.device, "device", "", "device to track: desktop, mobile, or both; preserves an existing setting")
	command.Flags().IntVar(&opts.depth, "depth", 0, "maximum organic position to inspect; preserves an existing setting")
	return command
}

func newRankingsRemoveCommand(client *clientOptions) *cobra.Command {
	opts := &rankMarketOptions{client: client}
	command := &cobra.Command{
		Use:   "remove <url> <keyword> [keyword...]",
		Short: "Remove keywords while preserving historical API results",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			apiClient, err := newAPIClient(opts.client)
			if err != nil {
				return err
			}
			tracker, _, err := resolveRankTracker(command, apiClient, args[0], opts.location, opts.language)
			if err != nil {
				return err
			}
			var update rankKeywordUpdate
			err = apiClient.Patch(
				command.Context(),
				fmt.Sprintf("/api/v1/rank-trackers/%d/keywords", tracker.Config.ID),
				protocol.RankKeywordPatchRequest{Remove: args[1:]},
				&update,
			)
			if err != nil {
				return err
			}
			if opts.json {
				return printJSON(command.OutOrStdout(), update)
			}
			printRankKeywordUpdate(command.OutOrStdout(), "Removed", update.Removed, update)
			return nil
		},
	}
	addRankMarketFlags(command, opts)
	return command
}

func newRankingsCheckCommand(client *clientOptions) *cobra.Command {
	opts := &rankCheckOptions{rankMarketOptions: rankMarketOptions{client: client}}
	command := &cobra.Command{
		Use:   "check <url>",
		Short: "Request an explicit paid rank check through the API",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if !opts.dataForSEO {
				return errors.New("rank checking requires the paid DataForSEO source; rerun with --dataforseo")
			}
			apiClient, err := newAPIClient(opts.client)
			if err != nil {
				return err
			}
			tracker, _, err := resolveRankTracker(command, apiClient, args[0], opts.location, opts.language)
			if err != nil {
				return err
			}
			data, err := runAPIJob(
				command,
				opts.client,
				opts.verbose,
				fmt.Sprintf("/api/v1/rank-trackers/%d/checks", tracker.Config.ID),
				protocol.RankCheckRequest{Source: "dataforseo"},
			)
			if err != nil {
				return err
			}
			var report rankReport
			if err := json.Unmarshal(data, &report); err != nil {
				return fmt.Errorf("decode rank check result: %w", err)
			}
			if opts.json {
				return printRawJSON(command.OutOrStdout(), data)
			}
			printRankReport(command.OutOrStdout(), report)
			return nil
		},
	}
	addRankMarketFlags(command, &opts.rankMarketOptions)
	command.Flags().BoolVar(&opts.dataForSEO, "dataforseo", false, "use paid DataForSEO live organic SERP checks")
	command.Flags().BoolVarP(&opts.verbose, "verbose", "v", false, "stream API job progress")
	return command
}

func newRankingsReportCommand(client *clientOptions) *cobra.Command {
	opts := &rankMarketOptions{client: client}
	command := &cobra.Command{
		Use:   "report <url>",
		Short: "Read the latest positions and changes from the API",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			apiClient, err := newAPIClient(opts.client)
			if err != nil {
				return err
			}
			report, raw, err := resolveRankTracker(command, apiClient, args[0], opts.location, opts.language)
			if err != nil {
				return err
			}
			if opts.json {
				return printRawJSON(command.OutOrStdout(), raw)
			}
			printRankReport(command.OutOrStdout(), report)
			return nil
		},
	}
	addRankMarketFlags(command, opts)
	return command
}

func addRankMarketFlags(command *cobra.Command, opts *rankMarketOptions) {
	command.Flags().BoolVar(&opts.json, "json", false, "print the complete machine-readable report")
	command.Flags().StringVar(&opts.location, "location", protocol.DefaultRankLocation, "DataForSEO search location")
	command.Flags().StringVar(&opts.language, "language", protocol.DefaultRankLanguage, "DataForSEO language code")
}

func resolveRankTracker(
	command *cobra.Command,
	client interface {
		Get(context.Context, string, any) error
	},
	rawURL,
	location,
	language string,
) (rankReport, json.RawMessage, error) {
	query := url.Values{
		"target":   {rawURL},
		"location": {location},
		"language": {language},
	}
	var response struct {
		Trackers []json.RawMessage `json:"trackers"`
	}
	if err := client.Get(command.Context(), "/api/v1/rank-trackers?"+query.Encode(), &response); err != nil {
		return rankReport{}, nil, err
	}
	if len(response.Trackers) == 0 {
		return rankReport{}, nil, errors.New("rank tracker not found; add keywords first")
	}
	var report rankReport
	if err := json.Unmarshal(response.Trackers[0], &report); err != nil {
		return rankReport{}, nil, fmt.Errorf("decode rank tracker: %w", err)
	}
	return report, response.Trackers[0], nil
}

func printRankKeywordUpdate(output io.Writer, action string, count int, update rankKeywordUpdate) {
	fmt.Fprintf(output, "Rank tracker: %s in %s (%s), %s, depth %d\n",
		update.Config.Target,
		update.Config.Location,
		update.Config.Language,
		update.Config.Devices,
		update.Config.SERPDepth,
	)
	fmt.Fprintf(output, "%s: %d keywords; tracking %d total\n", action, count, update.TotalKeywords)
	for _, keyword := range first(update.Keywords, terminalResultLimit) {
		fmt.Fprintf(output, "- %s\n", keyword.Keyword)
	}
	if len(update.Keywords) > terminalResultLimit {
		fmt.Fprintf(output, "... and %d more\n", len(update.Keywords)-terminalResultLimit)
	}
}

func printRankReport(output io.Writer, report rankReport) {
	fmt.Fprintf(output, "Rank tracking: %s in %s (%s), %s, depth %d\n",
		report.Config.Target,
		report.Config.Location,
		report.Config.Language,
		report.Config.Devices,
		report.Config.SERPDepth,
	)
	if report.LatestRun == nil {
		fmt.Fprintf(output, "Tracked: %d keywords; no rank checks stored yet\n", report.Summary.TrackedKeywords)
		fmt.Fprintln(output, "Run `seoaudit rankings check <url> --dataforseo` to create the first paid snapshot.")
		return
	}
	fmt.Fprintf(output, "Latest run: %d, %s, %d of %d tasks, %d live provider tasks, provider cost $%.6f\n",
		report.LatestRun.ID,
		report.LatestRun.Status,
		report.LatestRun.SuccessfulTasks,
		report.LatestRun.RequestedTasks,
		report.LatestRun.LiveCalls,
		report.LatestRun.CostUSD,
	)
	fmt.Fprintf(output, "Positions: %d ranking, %d not ranking, %d top 3, %d top 10\n",
		report.Summary.Ranking,
		report.Summary.NotRanking,
		report.Summary.Top3,
		report.Summary.Top10,
	)
	if report.PreviousRunID != nil {
		fmt.Fprintf(output, "Changes versus run %d: %d improved, %d declined, %d new, %d lost\n",
			*report.PreviousRunID,
			report.Summary.Improved,
			report.Summary.Declined,
			report.Summary.New,
			report.Summary.Lost,
		)
	}
	if report.LatestRun.ErrorMessage != "" {
		fmt.Fprintf(output, "Provider warning: %s\n", report.LatestRun.ErrorMessage)
	}
	writer := startTable(output, "Tracked positions", "POS\tPREVIOUS\tCHANGE\tDEVICE\tKEYWORD\tURL")
	for _, row := range first(report.Rows, terminalResultLimit) {
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\t%s\n",
			rankPosition(row.Position, row.Observed, report.Config.SERPDepth),
			rankPosition(row.PreviousPosition, row.PreviousObserved, report.Config.SERPDepth),
			row.Change,
			row.Device,
			oneLine(row.Keyword),
			row.RankingURL,
		)
	}
	_ = writer.Flush()
	if len(report.Rows) > terminalResultLimit {
		fmt.Fprintf(output, "Showing %d of %d keyword/device rows. Use --json for all results.\n", terminalResultLimit, len(report.Rows))
	}
}

func rankPosition(position *int, observed bool, depth int) string {
	if !observed {
		return "-"
	}
	if position == nil {
		return ">" + strconv.Itoa(depth)
	}
	return strconv.Itoa(*position)
}

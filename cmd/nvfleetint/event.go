// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Stores local flag values for event list
type eventListFlags struct {
	window    string
	start     string
	end       string
	node      string
	component string
}

// Stores local flag values for event buckets
type eventBucketsFlags struct {
	window     string
	start      string
	end        string
	node       string
	component  string
	maxBuckets int
}

// Stores data ready for event list rendering
type eventListOutput struct {
	Events    []nvfleetint.Event
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Creates the top-level event command group
func newEventCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Inspect events",
	}

	cmd.AddCommand(newEventListCmd())
	cmd.AddCommand(newEventBucketsCmd())
	rejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the event list command
func newEventListCmd() *cobra.Command {
	flags := eventListFlags{}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events",
		Args:  cobra.NoArgs,
		Long: `List events filtered by node, component, and a time range.

A time range is required: use --window for a relative range, or --start and
--end for an absolute range.`,
		Example: `  nvfleetint event list --window 24h
  nvfleetint event list --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z --component GPU
  nvfleetint event list --window 168h --node 1e9c0d2a-0000-4a1b-9c3d-000000000001`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEventList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	registerEventTimeFlags(cmd, &flags.window, &flags.start, &flags.end)
	cmd.Flags().StringVar(&flags.node, "node", "", "Filter by node UUID")
	cmd.Flags().StringVar(&flags.component, "component", "", "Filter by component")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the event buckets command
func newEventBucketsCmd() *cobra.Command {
	flags := eventBucketsFlags{maxBuckets: 100}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "buckets",
		Short: "Show time-bucketed event counts",
		Args:  cobra.NoArgs,
		Long: `Show time-bucketed event counts for histogram display, filtered by node,
component, and a time range.

A time range is required: use --window for a relative range, or --start and
--end for an absolute range.`,
		Example: `  nvfleetint event buckets --window 24h
  nvfleetint event buckets --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z --max-buckets 50`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEventBuckets(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	registerEventTimeFlags(cmd, &flags.window, &flags.start, &flags.end)
	cmd.Flags().StringVar(&flags.node, "node", "", "Filter by node UUID")
	cmd.Flags().StringVar(&flags.component, "component", "", "Filter by component")
	cmd.Flags().IntVar(&flags.maxBuckets, "max-buckets", flags.maxBuckets, fmt.Sprintf("Maximum number of buckets (1-%d)", nvfleetint.MaxEventBuckets))
	registerReadCommonFlags(cmd, common)

	return cmd
}

// Registers the shared relative/absolute time-range flags
func registerEventTimeFlags(cmd *cobra.Command, window, start, end *string) {
	cmd.Flags().StringVar(window, "window", "", "Relative time window; valid units: ns, us, µs, ms, s, m, h")
	cmd.Flags().StringVar(start, "start", "", "Absolute start time in RFC3339 format")
	cmd.Flags().StringVar(end, "end", "", "Absolute end time in RFC3339 format")
}

// Validates flags, calls the SDK, and writes output
func runEventList(cmd *cobra.Command, flags eventListFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if err := validateEventTimeFlags(flags.window, flags.start, flags.end); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	opts := nvfleetint.EventListOptions{
		NodeUUID:  strings.TrimSpace(flags.node),
		Component: strings.TrimSpace(flags.component),
		Window:    strings.TrimSpace(flags.window),
		StartTime: strings.TrimSpace(flags.start),
		EndTime:   strings.TrimSpace(flags.end),
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var events []nvfleetint.Event
		result, err := clihelpers.FetchAllRawPages("events", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListEvents(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			events = append(events, currentPage.Events...)
			hasMore := currentPage.HasMore
			return clihelpers.RawPage{
				RawJSON:  currentPage.RawJSON,
				Page:     currentPage.Page,
				PageSize: currentPage.PageSize,
				Total:    currentPage.Total,
				HasMore:  &hasMore,
			}, nil
		})
		if err != nil {
			return err
		}
		return writeEventListOutput(cmd.OutOrStdout(), common, eventListOutput{
			Events:    events,
			JSONValue: result,
		})
	}

	page, err := client.ListEvents(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeEventListOutput(cmd.OutOrStdout(), common, eventListOutput{
		Events:  page.Events,
		RawJSON: page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	})
}

// Validates flags, calls the SDK, and writes output
func runEventBuckets(cmd *cobra.Command, flags eventBucketsFlags, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}
	if err := validateEventTimeFlags(flags.window, flags.start, flags.end); err != nil {
		return err
	}
	if flags.maxBuckets < 1 || flags.maxBuckets > nvfleetint.MaxEventBuckets {
		return fmt.Errorf("--max-buckets must be between 1 and %d", nvfleetint.MaxEventBuckets)
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	opts := nvfleetint.EventBucketsOptions{
		NodeUUID:  strings.TrimSpace(flags.node),
		Component: strings.TrimSpace(flags.component),
		Window:    strings.TrimSpace(flags.window),
		StartTime: strings.TrimSpace(flags.start),
		EndTime:   strings.TrimSpace(flags.end),
	}
	// Only forward --max-buckets when the user set it; otherwise let the backend
	// apply its own default.
	if cmd.Flags().Changed("max-buckets") {
		maxBuckets := flags.maxBuckets
		opts.MaxBuckets = &maxBuckets
	}

	buckets, err := client.GetEventBuckets(cmd.Context(), opts)
	if err != nil {
		return err
	}

	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), buckets.RawJSON)
	}
	return writeEventBucketsTable(cmd.OutOrStdout(), buckets)
}

// Mirrors the SDK time-range rules at the CLI layer for early, friendly errors
func validateEventTimeFlags(window, start, end string) error {
	window = strings.TrimSpace(window)
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)

	hasWindow := window != ""
	hasStart := start != ""
	hasEnd := end != ""

	if !hasWindow && !hasStart && !hasEnd {
		return errors.New("a time range is required: use --window for a relative range, or --start and --end for an absolute range")
	}
	if hasWindow && (hasStart || hasEnd) {
		return errors.New("--window cannot be used with --start or --end")
	}
	if hasWindow {
		if _, err := reportWindowBackendValue(window); err != nil {
			return err
		}
		return nil
	}
	if !hasStart || !hasEnd {
		return errors.New("--start and --end must be used together")
	}
	if err := validateRFC3339Flag("--start", start); err != nil {
		return err
	}
	return validateRFC3339Flag("--end", end)
}

// Writes JSON or table output for event list results
func writeEventListOutput(w io.Writer, common resolvedCommonFlags, result eventListOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	// Columns mirror the event JSON fields one-to-one, in the SDK model's field
	// order, so the table is a faithful human-readable view of the same data the
	// JSON output carries.
	headers := []string{"EVENT ID", "NODE UUID", "COMPONENT", "NAME", "TYPE", "MESSAGE", "TIMESTAMP"}
	if err := clioutput.WriteTable(w, headers, eventRows(result.Events)); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// The MESSAGE column is free-text and can be arbitrarily long, so it is
// truncated with an ellipsis to keep the table readable; the full text (and the
// omitted createdAt / suggestedActions fields) is available via -o json.
const eventMessageColumnWidth = 60

// Converts events into table rows. Each column corresponds to an event JSON
// field. The verbose createdAt and suggestedActions fields are intentionally
// omitted from the table to keep it narrow; use -o json for the full payload.
func eventRows(events []nvfleetint.Event) [][]string {
	rows := make([][]string, 0, len(events))
	for _, event := range events {
		rows = append(rows, []string{
			clioutput.DisplayString(event.EventID),
			clioutput.DisplayString(event.NodeUUID),
			clioutput.DisplayString(event.Component),
			clioutput.DisplayString(event.Name),
			clioutput.DisplayString(event.Type),
			clioutput.Truncate(clioutput.DisplayString(event.Message), eventMessageColumnWidth),
			clioutput.DisplayString(event.Timestamp),
		})
	}
	return rows
}

// Renders event buckets as a table with a summary footer
func writeEventBucketsTable(w io.Writer, buckets nvfleetint.EventBuckets) error {
	if err := clioutput.WriteTable(w, []string{"START TIME", "END TIME", "COUNT", "FIRST EVENT TIME"}, eventBucketRows(buckets.Buckets)); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "Bucket Interval: %s  Total Buckets: %d\n", clioutput.DisplayString(buckets.BucketInterval), len(buckets.Buckets))
	return err
}

// Converts event buckets into table rows
func eventBucketRows(buckets []nvfleetint.EventBucket) [][]string {
	rows := make([][]string, 0, len(buckets))
	for _, bucket := range buckets {
		rows = append(rows, []string{
			clioutput.DisplayString(bucket.StartTime),
			clioutput.DisplayString(bucket.EndTime),
			clioutput.FormatOptionalInt(bucket.Count),
			clioutput.DisplayString(bucket.FirstEventTime),
		})
	}
	return rows
}

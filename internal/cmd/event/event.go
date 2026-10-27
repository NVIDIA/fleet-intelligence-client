// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"fmt"
	"io"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Shared flags keep list and bucket queries on the same event selection.
type eventFilterFlags struct {
	window                string
	start                 string
	end                   string
	node                  string
	component             string
	nameSearch            string
	dataSource            string
	nodeGroupIDs          string
	computeZoneIDs        string
	excludeNodeGroupIDs   string
	excludeComputeZoneIDs string
	eventNames            string
	severities            string
	components            string
	gpuTypes              string
}

type eventListFlags struct {
	eventFilterFlags
	sortBy string
	order  string
}

type eventBucketsFlags struct {
	eventFilterFlags
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
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "event",
		Short: "Inspect events",
	}

	cmd.AddCommand(newEventListCmd())
	cmd.AddCommand(newEventBucketsCmd())
	cmd.AddCommand(newEventOptionsCmd())
	cmdutil.RejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the event list command
func newEventListCmd() *cobra.Command {
	flags := eventListFlags{}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List events",
		Args:  cobra.NoArgs,
		Long: `List events filtered by source, node inventory, event fields, and a time range.
Use event options to discover filter values and sort fields.

A time range is required: use --window for a relative range, or --start and
--end for an absolute range.`,
		Example: `  nvfleetint event list --window 24h
  nvfleetint event list --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z --component GPU
  nvfleetint event list --window 168h --node 1e9c0d2a-0000-4a1b-9c3d-000000000001`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEventList(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	registerEventFilterFlags(cmd, &flags.eventFilterFlags)
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field; see event options (default: timestamp)")
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort direction: asc or desc (default: desc)")
	cmdutil.RegisterListFlags(cmd, common)

	return cmd
}

// Creates the event buckets command
func newEventBucketsCmd() *cobra.Command {
	flags := eventBucketsFlags{maxBuckets: 100}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "buckets",
		Short: "Show time-bucketed event counts",
		Args:  cobra.NoArgs,
		Long: `Show time-bucketed event counts for histogram display, filtered by source,
node inventory, event fields, and a time range.

A time range is required: use --window for a relative range, or --start and
--end for an absolute range.`,
		Example: `  nvfleetint event buckets --window 24h
  nvfleetint event buckets --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z --max-buckets 50`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEventBuckets(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	registerEventFilterFlags(cmd, &flags.eventFilterFlags)
	cmd.Flags().IntVar(&flags.maxBuckets, "max-buckets", flags.maxBuckets, fmt.Sprintf("Maximum number of buckets (1-%d)", nvfleetint.MaxEventBuckets))
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runEventList(cmd *cobra.Command, flags eventListFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateListFlags(common); err != nil {
		return err
	}
	if err := cmdutil.ValidateTimeRangeFlags(flags.window, flags.start, flags.end); err != nil {
		return err
	}

	opts, err := eventFilterOptions(flags.eventFilterFlags)
	if err != nil {
		return err
	}
	opts.SortBy = nvfleetint.EventSortBy(strings.TrimSpace(flags.sortBy))
	opts.SortOrder = nvfleetint.EventSortOrder(strings.TrimSpace(flags.order))
	if err := opts.Validate(); err != nil {
		return renderEventOptionError(err)
	}
	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}
	cmdutil.ApplyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.All {
		var events []nvfleetint.Event
		result, err := cmdutil.FetchAllPages("events",
			func(pageNumber int) (nvfleetint.EventsPage, error) {
				opts.Page = &pageNumber
				return client.ListEvents(cmd.Context(), opts)
			},
			func(page nvfleetint.EventsPage) { events = append(events, page.Events...) },
		)
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
func runEventBuckets(cmd *cobra.Command, flags eventBucketsFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}
	if err := cmdutil.ValidateTimeRangeFlags(flags.window, flags.start, flags.end); err != nil {
		return err
	}
	if flags.maxBuckets < 1 || flags.maxBuckets > nvfleetint.MaxEventBuckets {
		return fmt.Errorf("--max-buckets must be between 1 and %d", nvfleetint.MaxEventBuckets)
	}

	filters, err := eventFilterOptions(flags.eventFilterFlags)
	if err != nil {
		return err
	}
	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}
	opts := nvfleetint.EventBucketsOptions{
		NodeUUID:              filters.NodeUUID,
		Component:             filters.Component,
		Window:                filters.Window,
		StartTime:             filters.StartTime,
		EndTime:               filters.EndTime,
		NameSearch:            filters.NameSearch,
		DataSources:           filters.DataSources,
		NodeGroupIDs:          filters.NodeGroupIDs,
		ComputeZoneIDs:        filters.ComputeZoneIDs,
		ExcludeNodeGroupIDs:   filters.ExcludeNodeGroupIDs,
		ExcludeComputeZoneIDs: filters.ExcludeComputeZoneIDs,
		EventNames:            filters.EventNames,
		Severities:            filters.Severities,
		Components:            filters.Components,
		GPUTypes:              filters.GPUTypes,
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

	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), buckets.RawJSON)
	}
	return writeEventBucketsTable(cmd.OutOrStdout(), buckets)
}

// Writes JSON or table output for event list results
func writeEventListOutput(w io.Writer, common cmdutil.Resolved, result eventListOutput) error {
	if common.Output == clioutput.FormatJSON {
		return cmdutil.WritePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	// Keep the table compact; complete identity and inventory fields are in JSON.
	headers := []string{"EVENT ID", "NODE UUID", "DATA SOURCE", "COMPONENT", "NAME", "TYPE", "MESSAGE", "TIMESTAMP"}
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
			clioutput.DisplayString(event.DataSource),
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

func registerEventFilterFlags(cmd *cobra.Command, flags *eventFilterFlags) {
	cmdutil.RegisterTimeRangeFlags(cmd, &flags.window, &flags.start, &flags.end)
	cmd.Flags().StringVar(&flags.node, "node", "", "Filter by node UUID")
	cmd.Flags().StringVar(&flags.component, "component", "", "Filter by one component; cannot be combined with --components")
	cmd.Flags().StringVar(&flags.nameSearch, "name-search", "", "Search hostname, OOB node name, BMC MAC identifier, or node UUID")
	cmd.Flags().StringVar(&flags.nameSearch, "hostname", "", "Compatibility alias for --name-search")
	cmd.MarkFlagsMutuallyExclusive("name-search", "hostname")
	cmd.Flags().StringVar(&flags.dataSource, "data-source", "", "Comma-separated event sources: inband, oob (omitted: both)")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs")
	cmd.Flags().StringVar(&flags.excludeNodeGroupIDs, "exclude-nodegroup-ids", "", "Comma-separated node group IDs to exclude; cannot be combined with --nodegroup-ids")
	cmd.Flags().StringVar(&flags.excludeComputeZoneIDs, "exclude-compute-zone-ids", "", "Comma-separated compute zone IDs to exclude; cannot be combined with --compute-zone-ids")
	cmd.Flags().StringVar(&flags.eventNames, "event-names", "", "Comma-separated exact event names (Others matches names outside the catalog)")
	cmd.Flags().StringVar(&flags.severities, "severity", "", "Comma-separated exact severity values; see event options")
	cmd.Flags().StringVar(&flags.components, "components", "", "Comma-separated exact component identifiers; cannot be combined with --component")
	cmd.Flags().StringVar(&flags.gpuTypes, "gpu-type", "", "Comma-separated resolved node GPU types")
}

// Parse shared filters once so list and buckets select the same events.
func eventFilterOptions(flags eventFilterFlags) (nvfleetint.EventListOptions, error) {
	opts := nvfleetint.EventListOptions{
		NodeUUID:   strings.TrimSpace(flags.node),
		Component:  strings.TrimSpace(flags.component),
		Window:     strings.TrimSpace(flags.window),
		StartTime:  strings.TrimSpace(flags.start),
		EndTime:    strings.TrimSpace(flags.end),
		NameSearch: strings.TrimSpace(flags.nameSearch),
	}
	for _, filter := range []struct {
		name, raw string
		target    *[]string
	}{
		{"nodegroup-ids", flags.nodeGroupIDs, &opts.NodeGroupIDs},
		{"compute-zone-ids", flags.computeZoneIDs, &opts.ComputeZoneIDs},
		{"exclude-nodegroup-ids", flags.excludeNodeGroupIDs, &opts.ExcludeNodeGroupIDs},
		{"exclude-compute-zone-ids", flags.excludeComputeZoneIDs, &opts.ExcludeComputeZoneIDs},
		{"event-names", flags.eventNames, &opts.EventNames},
		{"severity", flags.severities, &opts.Severities},
		{"components", flags.components, &opts.Components},
		{"gpu-type", flags.gpuTypes, &opts.GPUTypes},
	} {
		values, err := cmdutil.ParseCommaList(filter.raw)
		if err != nil {
			return opts, fmt.Errorf("invalid --%s: %w", filter.name, err)
		}
		*filter.target = values
	}
	var err error
	opts.DataSources, err = cmdutil.ParseTypedList[nvfleetint.DataSource]("--data-source", flags.dataSource)
	if err != nil {
		return opts, err
	}
	return opts, renderEventOptionError(opts.Validate())
}

func renderEventOptionError(err error) error {
	return cmdutil.RenderOptionError(err, map[string]cmdutil.OptionFlagName{
		"dataSources": {Flag: "--data-source"},
		"sortBy":      {Flag: "--sort-by"},
		"sortOrder":   {Flag: "--order"},
	})
}

func newEventOptionsCmd() *cobra.Command {
	common := cmdutil.NewCommon()
	cmd := &cobra.Command{
		Use:   "options",
		Short: "Show event filter values and list sort fields",
		Args:  cobra.NoArgs,
		Long: `Show supported event filters and list sort fields. Options are catalog-based,
not scoped to a time range or inventory filters, and may have no matching events.
Others matches stored event names outside the catalog. GPU types reflect current
resolved node inventory, independently of event source.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			resolved := cmdutil.ResolveCommon(cmd, common)
			if err := cmdutil.ValidateReadFlags(resolved); err != nil {
				return err
			}
			client, err := cmdutil.New(resolved)
			if err != nil {
				return err
			}
			options, err := client.GetEventFilterOptions(cmd.Context())
			if err != nil {
				return err
			}
			if resolved.Output == clioutput.FormatJSON {
				return clioutput.WriteRawJSON(cmd.OutOrStdout(), options.RawJSON)
			}
			return writeEventOptionsTable(cmd.OutOrStdout(), options)
		},
	}
	cmdutil.RegisterReadFlags(cmd, common)
	return cmd
}

func writeEventOptionsTable(w io.Writer, options nvfleetint.EventFilterOptions) error {
	if _, err := fmt.Fprintf(w, "Filters for 'event list' and 'event buckets' (sorting applies to list only)\n%s\n", cmdutil.OptionsPreamble); err != nil {
		return err
	}
	components := make([][]string, 0, len(options.Components))
	for _, component := range options.Components {
		components = append(components, []string{component.Component, component.ComponentDisplayName})
	}
	for _, section := range []cmdutil.OptionSection{
		{Heading: "--event-names", Rows: cmdutil.ValueRows(options.EventNames)},
		{Heading: "--severity", Rows: cmdutil.ValueRows(options.Severities)},
		{Heading: "--components (or --component for one value)", Rows: components},
		{Heading: "--data-source", Rows: cmdutil.ValueRows(options.DataSources)},
		{Heading: "--gpu-type", Rows: cmdutil.ValueRows(options.GPUTypes)},
		{Heading: "--sort-by (event list only)", Rows: cmdutil.ValueRows(options.SortFields)},
		{Heading: "--order (event list only)", Rows: cmdutil.ValueRows([]string{"asc", "desc"})},
	} {
		if err := cmdutil.WriteOptionSection(w, section.Heading, section.Rows); err != nil {
			return err
		}
	}
	return nil
}

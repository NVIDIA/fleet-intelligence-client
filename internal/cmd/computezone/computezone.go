// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package computezone

import (
	"errors"
	"io"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Stores local flag values for computezone list
type computeZoneListFlags struct {
	view           string
	includeMetrics bool
	zoneIDs        string
}

// Stores data ready for computezone list rendering
type computeZoneListOutput struct {
	ComputeZones []nvfleetint.ComputeZone
	View         string
	JSONValue    any
	RawJSON      []byte
	Page         *clioutput.Pagination
}

// Creates the top-level computezone command group
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "computezone",
		Aliases: []string{"compute-zone"},
		Short:   "Inspect compute zones",
	}

	cmd.AddCommand(newComputeZoneListCmd())
	cmdutil.RejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the compute zone list command
func newComputeZoneListCmd() *cobra.Command {
	flags := computeZoneListFlags{
		view:           string(nvfleetint.ComputeZoneViewDetail),
		includeMetrics: true,
	}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List compute zones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runComputeZoneList(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "View mode: detail or basic")
	cmd.Flags().BoolVar(&flags.includeMetrics, "include-metrics", flags.includeMetrics, "Include metrics in detail view; use --include-metrics=false to omit")
	cmd.Flags().StringVar(&flags.zoneIDs, "zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmdutil.RegisterListFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runComputeZoneList(cmd *cobra.Command, flags computeZoneListFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateListFlags(common); err != nil {
		return err
	}
	opts, err := computeZoneListOptions(flags, cmd.Flags().Changed("include-metrics"))
	if err != nil {
		return err
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	cmdutil.ApplyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.All {
		var zones []nvfleetint.ComputeZone
		result, err := cmdutil.FetchAllPages("computezones",
			func(pageNumber int) (nvfleetint.ComputeZonesPage, error) {
				opts.Page = &pageNumber
				return client.ListComputeZones(cmd.Context(), opts)
			},
			func(page nvfleetint.ComputeZonesPage) { zones = append(zones, page.ComputeZones...) },
		)
		if err != nil {
			return err
		}
		return writeComputeZoneListOutput(cmd.OutOrStdout(), common, computeZoneListOutput{
			ComputeZones: zones,
			View:         flags.view,
			JSONValue:    result,
		})
	}

	page, err := client.ListComputeZones(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeComputeZoneListOutput(cmd.OutOrStdout(), common, computeZoneListOutput{
		ComputeZones: page.ComputeZones,
		View:         flags.view,
		RawJSON:      page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	})
}

// Names the flag carrying each compute zone list option, for rendering SDK
// validation errors against what the user typed.
var computeZoneListFlagNames = map[string]cmdutil.OptionFlagName{
	"view": {Flag: "view"},
}

// computeZoneListOptions reads every compute zone list flag exactly once and
// hands the result to the SDK to validate. includeMetricsSet reports whether
// --include-metrics was given at all, which is the difference between asking
// for the backend default and asking for false.
func computeZoneListOptions(flags computeZoneListFlags, includeMetricsSet bool) (nvfleetint.ListComputeZonesOptions, error) {
	opts := nvfleetint.ListComputeZonesOptions{
		View: nvfleetint.ComputeZoneView(flags.view),
	}
	if includeMetricsSet {
		opts.IncludeMetrics = &flags.includeMetrics
	}

	zoneIDs, err := cmdutil.ParseCommaList(flags.zoneIDs)
	if err != nil {
		return opts, err
	}
	opts.ZoneIDs = zoneIDs

	// The SDK enforces this too; naming the flag is the only reason it is
	// restated here, since the SDK sees a pointer rather than a flag.
	if opts.View == nvfleetint.ComputeZoneViewBasic && includeMetricsSet {
		return opts, errors.New("basic compute zone view is incompatible with --include-metrics")
	}
	if err := opts.Validate(); err != nil {
		return opts, cmdutil.RenderOptionError(err, computeZoneListFlagNames)
	}

	return opts, nil
}

// Writes JSON or table output for compute zone list results
func writeComputeZoneListOutput(w io.Writer, common cmdutil.Resolved, result computeZoneListOutput) error {
	if common.Output == clioutput.FormatJSON {
		return cmdutil.WritePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	if err := writeComputeZoneTable(w, result.View, result.ComputeZones); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Renders compute zones using the selected view columns
func writeComputeZoneTable(w io.Writer, view string, zones []nvfleetint.ComputeZone) error {
	if nvfleetint.ComputeZoneView(view) == nvfleetint.ComputeZoneViewBasic {
		return clioutput.WriteTable(w, []string{"ID", "NAME"}, basicComputeZoneRows(zones))
	}
	// "LOCATION" is the user-facing label for the backend geoLocation field.
	return clioutput.WriteTable(w, []string{"ID", "NAME", "TYPE", "LOCATION", "NODE COUNT"}, detailComputeZoneRows(zones))
}

// Converts compute zones into basic table rows
func basicComputeZoneRows(zones []nvfleetint.ComputeZone) [][]string {
	rows := make([][]string, 0, len(zones))
	for _, zone := range zones {
		rows = append(rows, []string{clioutput.DisplayString(zone.ID), clioutput.DisplayString(zone.Name)})
	}
	return rows
}

// Converts compute zones into detail table rows
func detailComputeZoneRows(zones []nvfleetint.ComputeZone) [][]string {
	rows := make([][]string, 0, len(zones))
	for _, zone := range zones {
		rows = append(rows, []string{
			clioutput.DisplayString(zone.ID),
			clioutput.DisplayString(zone.Name),
			clioutput.DisplayString(zone.Type),
			cmdutil.FormatGeoLocation(zone.GeoLocation),
			clioutput.FormatOptionalInt(zone.NodeCount),
		})
	}
	return rows
}

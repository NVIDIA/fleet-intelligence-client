// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Stores local flag values for computezone list
type computeZoneListFlags struct {
	view    string
	zoneIDs string
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
func newComputeZoneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "computezone",
		Aliases: []string{"compute-zone"},
		Short:   "Inspect compute zones",
	}

	cmd.AddCommand(newComputeZoneListCmd())

	return cmd
}

// Creates the compute zone list command
func newComputeZoneListCmd() *cobra.Command {
	flags := computeZoneListFlags{
		view: string(nvfleetint.ComputeZoneViewDetail),
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List compute zones",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runComputeZoneList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "View mode: detail or basic")
	cmd.Flags().StringVar(&flags.zoneIDs, "zone-ids", "", "Comma-separated compute zone IDs to filter")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runComputeZoneList(cmd *cobra.Command, flags computeZoneListFlags, common resolvedCommonFlags) error {
	if err := validateComputeZoneListFlags(flags, common); err != nil {
		return err
	}

	zoneIDs, err := clihelpers.ParseCommaList(flags.zoneIDs)
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	opts := nvfleetint.ListComputeZonesOptions{
		View:    nvfleetint.ComputeZoneView(flags.view),
		ZoneIDs: zoneIDs,
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var zones []nvfleetint.ComputeZone
		result, err := clihelpers.FetchAllRawPages("computezones", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListComputeZones(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			zones = append(zones, currentPage.ComputeZones...)
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

// Checks compute zone list flags
func validateComputeZoneListFlags(flags computeZoneListFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if !nvfleetint.ComputeZoneView(flags.view).Valid() {
		return fmt.Errorf("invalid view %q: expected basic or detail", flags.view)
	}
	return nil
}

// Writes JSON or table output for compute zone list results
func writeComputeZoneListOutput(w io.Writer, common resolvedCommonFlags, result computeZoneListOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
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
			clioutput.FormatGeoLocation(zone.GeoLocation),
			clioutput.FormatOptionalInt(zone.NodeCount),
		})
	}
	return rows
}

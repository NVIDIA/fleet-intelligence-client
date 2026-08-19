// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
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

// Stores local flag values for computezone update
type computeZoneUpdateFlags struct {
	zoneType     string
	contactEmail string
	contactPIC   string
	geoCity      string
	geoCountry   string
	geoRegion    string
	geoLatitude  string
	geoLongitude string
	yes          bool
	dryRun       bool
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
	cmd.AddCommand(newComputeZoneUpdateCmd())
	rejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the compute zone list command
func newComputeZoneListCmd() *cobra.Command {
	flags := computeZoneListFlags{
		view:           string(nvfleetint.ComputeZoneViewDetail),
		includeMetrics: true,
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List compute zones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runComputeZoneList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "View mode: detail or basic")
	cmd.Flags().BoolVar(&flags.includeMetrics, "include-metrics", flags.includeMetrics, "Include metrics in detail view; use --include-metrics=false to omit")
	cmd.Flags().StringVar(&flags.zoneIDs, "zone-ids", "", "Comma-separated compute zone IDs to filter")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the compute zone update command
func newComputeZoneUpdateCmd() *cobra.Command {
	flags := computeZoneUpdateFlags{}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update compute zone metadata",
		Args:  requireSingleArg("compute zone ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runComputeZoneUpdate(cmd, args[0], flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.zoneType, "type", "", `Compute zone type: datacenter or "cloud provider"`)
	cmd.Flags().StringVar(&flags.contactEmail, "contact-email", "", "Contact email; pass an empty value to clear")
	cmd.Flags().StringVar(&flags.contactPIC, "contact-pic", "", "Contact person in charge; pass an empty value to clear")
	cmd.Flags().StringVar(&flags.geoCity, "geo-city", "", "Location city; pass an empty value to clear")
	cmd.Flags().StringVar(&flags.geoCountry, "geo-country", "", "Location country; pass an empty value to clear")
	cmd.Flags().StringVar(&flags.geoRegion, "geo-region", "", "Location region; pass an empty value to clear")
	cmd.Flags().StringVar(&flags.geoLatitude, "geo-latitude", "", "Location latitude between -90 and 90; pass an empty value to clear")
	cmd.Flags().StringVar(&flags.geoLongitude, "geo-longitude", "", "Location longitude between -180 and 180; pass an empty value to clear")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Preview the request without sending it")
	registerReadCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runComputeZoneList(cmd *cobra.Command, flags computeZoneListFlags, common resolvedCommonFlags) error {
	if err := validateComputeZoneListFlags(flags, common); err != nil {
		return err
	}
	if nvfleetint.ComputeZoneView(flags.view) == nvfleetint.ComputeZoneViewBasic && cmd.Flags().Changed("include-metrics") {
		return errors.New("basic compute zone view is incompatible with --include-metrics")
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
	if cmd.Flags().Changed("include-metrics") {
		opts.IncludeMetrics = &flags.includeMetrics
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

// Validates flags, calls the SDK, and writes output
func runComputeZoneUpdate(cmd *cobra.Command, id string, flags computeZoneUpdateFlags, common resolvedCommonFlags) error {
	if err := validateComputeZoneUpdateFlags(cmd, flags, common); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}
	opts := computeZoneUpdateOptionsFromFlags(cmd, id, flags)

	if flags.dryRun {
		preview, err := client.PreviewUpdateComputeZone(cmd.Context(), opts)
		if err != nil {
			return err
		}
		return writeComputeZoneUpdatePreview(cmd.OutOrStdout(), common, preview)
	}

	if !flags.yes {
		if err := clihelpers.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), computeZoneUpdateSummary(id, opts)); err != nil {
			return err
		}
	}

	result, err := client.UpdateComputeZone(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeComputeZoneUpdateOutput(cmd.OutOrStdout(), common, result)
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

// Checks compute zone update flags
func validateComputeZoneUpdateFlags(cmd *cobra.Command, flags computeZoneUpdateFlags, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}
	if !hasComputeZoneUpdateFlag(cmd) {
		return errors.New("at least one update flag must be set")
	}
	if cmd.Flags().Changed("type") {
		zoneType := strings.TrimSpace(flags.zoneType)
		if zoneType == "" {
			return errors.New("--type cannot be empty")
		}
		if !nvfleetint.ComputeZoneType(zoneType).Valid() {
			return fmt.Errorf("invalid --type %q: expected datacenter or cloud provider", zoneType)
		}
	}
	// An empty coordinate clears the stored value, so only real values are checked.
	if cmd.Flags().Changed("geo-latitude") && strings.TrimSpace(flags.geoLatitude) != "" {
		if err := nvfleetint.ValidateLatitude(flags.geoLatitude); err != nil {
			return fmt.Errorf("--geo-latitude: %w", err)
		}
	}
	if cmd.Flags().Changed("geo-longitude") && strings.TrimSpace(flags.geoLongitude) != "" {
		if err := nvfleetint.ValidateLongitude(flags.geoLongitude); err != nil {
			return fmt.Errorf("--geo-longitude: %w", err)
		}
	}
	return nil
}

func hasComputeZoneUpdateFlag(cmd *cobra.Command) bool {
	for _, name := range []string{
		"type",
		"contact-email",
		"contact-pic",
		"geo-city",
		"geo-country",
		"geo-region",
		"geo-latitude",
		"geo-longitude",
	} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func computeZoneUpdateOptionsFromFlags(cmd *cobra.Command, id string, flags computeZoneUpdateFlags) nvfleetint.UpdateComputeZoneOptions {
	opts := nvfleetint.UpdateComputeZoneOptions{ID: id}
	if cmd.Flags().Changed("type") {
		value := strings.TrimSpace(flags.zoneType)
		opts.Type = &value
	}
	if cmd.Flags().Changed("contact-email") {
		value := strings.TrimSpace(flags.contactEmail)
		opts.ContactEmail = &value
	}
	if cmd.Flags().Changed("contact-pic") {
		value := strings.TrimSpace(flags.contactPIC)
		opts.ContactPIC = &value
	}
	if cmd.Flags().Changed("geo-city") {
		value := strings.TrimSpace(flags.geoCity)
		opts.GeoCity = &value
	}
	if cmd.Flags().Changed("geo-country") {
		value := strings.TrimSpace(flags.geoCountry)
		opts.GeoCountry = &value
	}
	if cmd.Flags().Changed("geo-region") {
		value := strings.TrimSpace(flags.geoRegion)
		opts.GeoRegion = &value
	}
	if cmd.Flags().Changed("geo-latitude") {
		value := strings.TrimSpace(flags.geoLatitude)
		opts.GeoLatitude = &value
	}
	if cmd.Flags().Changed("geo-longitude") {
		value := strings.TrimSpace(flags.geoLongitude)
		opts.GeoLongitude = &value
	}
	return opts
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

// Writes JSON or text output for a successful compute zone update
func writeComputeZoneUpdateOutput(w io.Writer, common resolvedCommonFlags, result nvfleetint.UpdateComputeZoneResult) error {
	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(w, result.RawJSON)
	}

	id := clioutput.DisplayString(result.ID)
	_, err := fmt.Fprintf(w, "Compute zone %q updated.\n", id)
	return err
}

// Writes the dry-run request preview
func writeComputeZoneUpdatePreview(w io.Writer, common resolvedCommonFlags, preview nvfleetint.RequestPreview) error {
	if common.output == clioutput.FormatJSON {
		return clioutput.WriteJSON(w, preview)
	}

	prettyBody := preview.Body
	var formatted bytes.Buffer
	if len(preview.Body) > 0 && json.Indent(&formatted, preview.Body, "", "  ") == nil {
		prettyBody = formatted.Bytes()
	}

	if _, err := fmt.Fprintf(w, "Dry run: no write request sent.\nMETHOD: %s\nURL: %s\nBODY:\n%s\n", preview.Method, preview.URL, prettyBody); err != nil {
		return err
	}
	return nil
}

func computeZoneUpdateSummary(id string, opts nvfleetint.UpdateComputeZoneOptions) string {
	var fields []string
	if opts.Type != nil {
		fields = append(fields, "type")
	}
	if opts.ContactEmail != nil {
		fields = append(fields, "contact email")
	}
	if opts.ContactPIC != nil {
		fields = append(fields, "contact PIC")
	}
	if opts.GeoCity != nil {
		fields = append(fields, "geo city")
	}
	if opts.GeoCountry != nil {
		fields = append(fields, "geo country")
	}
	if opts.GeoRegion != nil {
		fields = append(fields, "geo region")
	}
	if opts.GeoLatitude != nil {
		fields = append(fields, "geo latitude")
	}
	if opts.GeoLongitude != nil {
		fields = append(fields, "geo longitude")
	}

	return fmt.Sprintf("Update compute zone %q fields: %s.", id, strings.Join(fields, ", "))
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

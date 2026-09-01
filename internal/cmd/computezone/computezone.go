// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package computezone

import (
	"errors"
	"fmt"
	"io"
	"strings"

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
		Short:   "Inspect and update compute zones",
	}

	cmd.AddCommand(newComputeZoneListCmd())
	cmd.AddCommand(newComputeZoneUpdateCmd())
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
			cmdutil.FormatLocation(zone.Location),
			clioutput.FormatOptionalInt(zone.NodeCount),
		})
	}
	return rows
}

// Stores local flag values for computezone update. Every metadata value is a
// string so that an empty value can mean "clear this field"; whether the user
// typed a flag at all is read from the flag set.
type computeZoneUpdateFlags struct {
	zoneType  string
	email     string
	pic       string
	city      string
	country   string
	region    string
	latitude  string
	longitude string
	yes       bool
	dryRun    bool
}

// Creates the compute zone update command
func newComputeZoneUpdateCmd() *cobra.Command {
	flags := computeZoneUpdateFlags{}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "update <zone-id>",
		Short: "Update a compute zone's type, contact, and location",
		Args:  cmdutil.RequireSingleArg("compute zone ID"),
		Long: `Update the metadata stored for one compute zone.

Contact and location are structured values on the API, so each of their fields
has its own flag. A flag that is not given leaves the stored value alone, and a
flag given an empty value clears that field; the command reads the zone first
and merges the changes over what it already stores, so setting a contact never
clears a location.

Compute zone names are agent-managed and cannot be changed through the customer
API, so there is no --name.

At least one field is required, and the command confirms before writing unless
--yes is passed. Use --dry-run to print the method, URL, and body of the PUT
request that would be sent, without issuing it; it still reads the zone to
build the merge, but skips both the confirmation prompt and the write, and
exits 0.`,
		Example: `  nvfleetint computezone update cz-1 --type datacenter
  nvfleetint computezone update cz-1 --contact-email ops@example.com --contact-pic "Jane Doe"
  nvfleetint computezone update cz-1 --location-city Baltimore --location-latitude 39.0458 --location-longitude -76.6413
  nvfleetint computezone update cz-1 --location-region "" --yes
  nvfleetint computezone update cz-1 --type datacenter --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runComputeZoneUpdate(cmd, args[0], flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.zoneType, "type", "", `Compute zone type: "datacenter" or "cloud provider"`)
	cmd.Flags().StringVar(&flags.email, "contact-email", "", "Contact email address; empty clears it")
	cmd.Flags().StringVar(&flags.pic, "contact-pic", "", "Contact person in charge; empty clears it")
	cmd.Flags().StringVar(&flags.city, "location-city", "", "Location city; empty clears it")
	cmd.Flags().StringVar(&flags.country, "location-country", "", "Location country; empty clears it")
	cmd.Flags().StringVar(&flags.region, "location-region", "", "Location region; empty clears it")
	cmd.Flags().StringVar(&flags.latitude, "location-latitude", "", "Location latitude in decimal degrees; empty clears it")
	cmd.Flags().StringVar(&flags.longitude, "location-longitude", "", "Location longitude in decimal degrees; empty clears it")
	cmd.Flags().BoolVar(&flags.yes, "yes", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVar(&flags.dryRun, "dry-run", false, "Print the request that would be sent without issuing it")
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Names the flag carrying each compute zone update option, for rendering SDK
// validation errors against what the user typed.
var computeZoneUpdateFlagNames = map[string]cmdutil.OptionFlagName{
	"type":      {Flag: "type"},
	"latitude":  {Flag: "location-latitude"},
	"longitude": {Flag: "location-longitude"},
}

// Validates flags, confirms the write, calls the SDK, and writes output
func runComputeZoneUpdate(cmd *cobra.Command, zoneID string, flags computeZoneUpdateFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}

	zoneID = strings.TrimSpace(zoneID)
	if zoneID == "" {
		return errors.New("compute zone ID is required")
	}

	opts, changes := computeZoneUpdateOptions(cmd, flags)
	if len(changes) == 0 {
		return errors.New("no changes requested; specify at least one field to update")
	}
	if err := opts.Validate(); err != nil {
		return cmdutil.RenderOptionError(err, computeZoneUpdateFlagNames)
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	if flags.dryRun {
		preview, err := client.PreviewUpdateComputeZone(cmd.Context(), zoneID, opts)
		if err != nil {
			return err
		}
		return cmdutil.WriteRequestPreview(cmd.OutOrStdout(), common, preview)
	}

	// Confirm after the client is built so a bad profile fails before the
	// question rather than after it.
	if !flags.yes {
		if err := cmdutil.Confirm(cmd.InOrStdin(), cmd.ErrOrStderr(), computeZoneUpdateSummary(zoneID, changes)); err != nil {
			return err
		}
	}

	result, err := client.UpdateComputeZone(cmd.Context(), zoneID, opts)
	if err != nil {
		return err
	}

	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), result.RawJSON)
	}
	return writeComputeZoneUpdateTable(cmd.OutOrStdout(), result)
}

// Reads every update flag exactly once, returning the SDK options and a
// description of each requested change for the confirmation prompt. Only flags
// the user actually typed are carried, which is what separates clearing a field
// from leaving it alone.
func computeZoneUpdateOptions(cmd *cobra.Command, flags computeZoneUpdateFlags) (nvfleetint.UpdateComputeZoneOptions, []string) {
	opts := nvfleetint.UpdateComputeZoneOptions{}
	changes := make([]string, 0, 8)

	fields := []struct {
		flag   string
		label  string
		value  string
		target **string
	}{
		{"type", "type", flags.zoneType, &opts.Type},
		{"contact-email", "contact email", flags.email, &opts.ContactEmail},
		{"contact-pic", "contact person in charge", flags.pic, &opts.ContactPIC},
		{"location-city", "location city", flags.city, &opts.LocationCity},
		{"location-country", "location country", flags.country, &opts.LocationCountry},
		{"location-region", "location region", flags.region, &opts.LocationRegion},
		{"location-latitude", "location latitude", flags.latitude, &opts.LocationLatitude},
		{"location-longitude", "location longitude", flags.longitude, &opts.LocationLongitude},
	}

	for _, field := range fields {
		if !cmd.Flags().Changed(field.flag) {
			continue
		}
		value := field.value
		*field.target = &value
		if strings.TrimSpace(value) == "" {
			changes = append(changes, fmt.Sprintf("%s: cleared", field.label))
			continue
		}
		changes = append(changes, fmt.Sprintf("%s: %s", field.label, value))
	}

	return opts, changes
}

// Describes the pending write for the confirmation prompt
func computeZoneUpdateSummary(zoneID string, changes []string) string {
	return fmt.Sprintf("This updates compute zone %s:\n  %s", zoneID, strings.Join(changes, "\n  "))
}

// Renders the metadata the zone carries after the update
func writeComputeZoneUpdateTable(w io.Writer, result nvfleetint.ComputeZoneUpdate) error {
	rows := [][]string{
		{"ID", clioutput.DisplayString(result.ID)},
		{"TYPE", clioutput.DisplayString(result.Type)},
		{"CONTACT EMAIL", clioutput.DisplayString(result.ContactEmail)},
		{"CONTACT PIC", clioutput.DisplayString(result.ContactPIC)},
		{"CITY", clioutput.DisplayString(result.City)},
		{"COUNTRY", clioutput.DisplayString(result.Country)},
		{"REGION", clioutput.DisplayString(result.Region)},
		{"LATITUDE", clioutput.DisplayString(result.Latitude)},
		{"LONGITUDE", clioutput.DisplayString(result.Longitude)},
	}
	return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, rows)
}

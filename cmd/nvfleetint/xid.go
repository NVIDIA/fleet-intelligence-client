// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Lists the sort fields accepted by xid burst list
const xidBurstSortByList = "startTime, hostname, nodeGroup, computeZone, xidNumbers, jobDisruption, " +
	"jobDisruptionDueToPlatformIssue, category, subcategory, tenantAction, tenantInvestigation, " +
	"dcAdminAction, or dcAdminInvestigation"

// Stores local flag values for xid burst list
type xidBurstListFlags struct {
	window                     string
	start                      string
	end                        string
	node                       string
	nodeGroupIDs               string
	computeZoneIDs             string
	jobDisruption              bool
	platformDisruption         bool
	xidNumbers                 string
	hostname                   string
	categorySearch             string
	subcategorySearch          string
	tenantActionSearch         string
	tenantInvestigationSearch  string
	dcAdminActionSearch        string
	dcAdminInvestigationSearch string
	categories                 string
	subcategories              string
	tenantActions              string
	tenantInvestigations       string
	dcAdminActions             string
	dcAdminInvestigations      string
	sortBy                     string
	order                      string
}

// Stores data ready for xid burst list rendering
type xidBurstListOutput struct {
	Bursts    []nvfleetint.XIDBurst
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Creates the top-level xid command group
func newXIDCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "xid",
		Short: "Inspect XID diagnostics",
	}

	cmd.AddCommand(newXIDBurstCmd())
	rejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the xid burst command group
func newXIDBurstCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "burst",
		Short: "Inspect finalized XID bursts",
		Long: "Inspect finalized XID bursts: groups of XID errors observed together on one node.\n\n" +
			"Workflow: list → describe. List the bursts in a time range, then describe one for its " +
			"impacted devices, XID catalog details, and suggested actions.",
		Example: "  nvfleetint xid burst list --window 24h\n" +
			"  nvfleetint xid burst list --window 168h --job-disruption --sort-by startTime\n" +
			"  nvfleetint xid burst describe <burst-id>",
	}

	cmd.AddCommand(newXIDBurstListCmd())
	cmd.AddCommand(newXIDBurstDescribeCmd())
	rejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the xid burst list command
func newXIDBurstListCmd() *cobra.Command {
	flags := xidBurstListFlags{}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List finalized XID bursts",
		Args:  cobra.NoArgs,
		Long: `List finalized XID bursts filtered by node scope, XID numbers, classification,
and a time range.

A time range is required: use --window for a relative range, or --start and
--end for an absolute range. The range applies to each burst's start time.

Some filters and response fields are only available to cloud-provider/NCP
callers; for tenant callers the backend rejects them.`,
		Example: `  nvfleetint xid burst list --window 24h
  nvfleetint xid burst list --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z
  nvfleetint xid burst list --window 168h --xid-numbers 48,94 --job-disruption`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runXIDBurstList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	registerEventTimeFlags(cmd, &flags.window, &flags.start, &flags.end)
	cmd.Flags().StringVar(&flags.node, "node", "", "Filter by node UUID")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().BoolVar(&flags.jobDisruption, "job-disruption", false, "Filter by the public fatal-XID job-disruption value")
	cmd.Flags().BoolVar(&flags.platformDisruption, "platform-disruption", false, "Filter by analyzer platform-attributed disruption (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.xidNumbers, "xid-numbers", "", "Comma-separated XID numbers; a burst matches if it contains any of them")
	cmd.Flags().StringVar(&flags.hostname, "hostname", "", "Hostname partial match")
	cmd.Flags().StringVar(&flags.categorySearch, "category-search", "", "Category partial match (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.subcategorySearch, "subcategory-search", "", "Subcategory partial match (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.tenantActionSearch, "tenant-action-search", "", "Tenant immediate-action code partial match")
	cmd.Flags().StringVar(&flags.tenantInvestigationSearch, "tenant-investigation-search", "", "Tenant investigatory-action code partial match")
	cmd.Flags().StringVar(&flags.dcAdminActionSearch, "dc-admin-action-search", "", "DC-admin immediate-action code partial match (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.dcAdminInvestigationSearch, "dc-admin-investigation-search", "", "DC-admin investigatory-action code partial match (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.categories, "categories", "", "Comma-separated exact category values to filter (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.subcategories, "subcategories", "", "Comma-separated exact subcategory values to filter (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.tenantActions, "tenant-actions", "", "Comma-separated exact tenant immediate-action codes to filter")
	cmd.Flags().StringVar(&flags.tenantInvestigations, "tenant-investigations", "", "Comma-separated exact tenant investigatory-action codes to filter")
	cmd.Flags().StringVar(&flags.dcAdminActions, "dc-admin-actions", "", "Comma-separated exact DC-admin immediate-action codes to filter (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.dcAdminInvestigations, "dc-admin-investigations", "", "Comma-separated exact DC-admin investigatory-action codes to filter (cloud-provider/NCP only)")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field: "+xidBurstSortByList)
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order: asc or desc")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the xid burst describe command
func newXIDBurstDescribeCmd() *cobra.Command {
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "describe <burstID>",
		Short: "Describe a finalized XID burst",
		Args:  requireSingleArg("burst ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runXIDBurstDescribe(cmd, args[0], resolveCommonFlags(cmd, common))
		},
	}

	registerReadCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runXIDBurstList(cmd *cobra.Command, flags xidBurstListFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if err := validateEventTimeFlags(flags.window, flags.start, flags.end); err != nil {
		return err
	}
	if flags.sortBy != "" && !nvfleetint.XIDBurstSortBy(strings.TrimSpace(flags.sortBy)).Valid() {
		return fmt.Errorf("invalid sort-by %q: expected %s", flags.sortBy, xidBurstSortByList)
	}
	if flags.order != "" && !nvfleetint.XIDBurstSortOrder(strings.TrimSpace(flags.order)).Valid() {
		return fmt.Errorf("invalid order %q: expected asc or desc", flags.order)
	}

	opts := nvfleetint.ListXIDBurstsOptions{
		Window:                     strings.TrimSpace(flags.window),
		StartTime:                  strings.TrimSpace(flags.start),
		EndTime:                    strings.TrimSpace(flags.end),
		NodeUUID:                   strings.TrimSpace(flags.node),
		HostnameSearch:             strings.TrimSpace(flags.hostname),
		CategorySearch:             strings.TrimSpace(flags.categorySearch),
		SubcategorySearch:          strings.TrimSpace(flags.subcategorySearch),
		TenantActionSearch:         strings.TrimSpace(flags.tenantActionSearch),
		TenantInvestigationSearch:  strings.TrimSpace(flags.tenantInvestigationSearch),
		DCAdminActionSearch:        strings.TrimSpace(flags.dcAdminActionSearch),
		DCAdminInvestigationSearch: strings.TrimSpace(flags.dcAdminInvestigationSearch),
		SortBy:                     nvfleetint.XIDBurstSortBy(strings.TrimSpace(flags.sortBy)),
		SortOrder:                  nvfleetint.XIDBurstSortOrder(strings.TrimSpace(flags.order)),
	}

	// Boolean filters are tri-state: omitted means "either", so they are only
	// forwarded when the user set them.
	if cmd.Flags().Changed("job-disruption") {
		opts.JobDisruption = &flags.jobDisruption
	}
	if cmd.Flags().Changed("platform-disruption") {
		opts.JobDisruptionDueToPlatformIssue = &flags.platformDisruption
	}

	xidNumbers, err := parseXIDNumberList(flags.xidNumbers)
	if err != nil {
		return err
	}
	opts.XIDNumbers = xidNumbers

	for _, filter := range []struct {
		name   string
		raw    string
		assign func([]string)
	}{
		{name: "nodegroup-ids", raw: flags.nodeGroupIDs, assign: func(v []string) { opts.NodeGroupIDs = v }},
		{name: "compute-zone-ids", raw: flags.computeZoneIDs, assign: func(v []string) { opts.ComputeZoneIDs = v }},
		{name: "categories", raw: flags.categories, assign: func(v []string) { opts.Categories = v }},
		{name: "subcategories", raw: flags.subcategories, assign: func(v []string) { opts.Subcategories = v }},
		{name: "tenant-actions", raw: flags.tenantActions, assign: func(v []string) { opts.TenantActions = v }},
		{name: "tenant-investigations", raw: flags.tenantInvestigations, assign: func(v []string) { opts.TenantInvestigations = v }},
		{name: "dc-admin-actions", raw: flags.dcAdminActions, assign: func(v []string) { opts.DCAdminActions = v }},
		{name: "dc-admin-investigations", raw: flags.dcAdminInvestigations, assign: func(v []string) { opts.DCAdminInvestigations = v }},
	} {
		values, err := clihelpers.ParseCommaList(filter.raw)
		if err != nil {
			return fmt.Errorf("invalid --%s: %w", filter.name, err)
		}
		filter.assign(values)
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var bursts []nvfleetint.XIDBurst
		result, err := clihelpers.FetchAllRawPages("items", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListXIDBursts(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			bursts = append(bursts, currentPage.Bursts...)
			hasMore := xidBurstPageHasMore(currentPage)
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
		return writeXIDBurstListOutput(cmd.OutOrStdout(), common, xidBurstListOutput{
			Bursts:    bursts,
			JSONValue: result,
		})
	}

	page, err := client.ListXIDBursts(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeXIDBurstListOutput(cmd.OutOrStdout(), common, xidBurstListOutput{
		Bursts:  page.Bursts,
		RawJSON: page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	})
}

// Validates args, calls the SDK, and writes output
func runXIDBurstDescribe(cmd *cobra.Command, burstID string, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	burst, err := client.DescribeXIDBurst(cmd.Context(), strings.TrimSpace(burstID))
	if err != nil {
		return err
	}
	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), burst.RawJSON)
	}
	return writeXIDBurstDescribeTable(cmd.OutOrStdout(), burst)
}

// Converts comma-separated XID numbers into API values
func parseXIDNumberList(raw string) ([]int, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid --xid-numbers: %w", err)
	}
	if len(values) == 0 {
		return nil, nil
	}

	numbers := make([]int, 0, len(values))
	for _, value := range values {
		number, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid xid-numbers %q: expected an integer", value)
		}
		if number < 0 {
			return nil, fmt.Errorf("invalid xid-numbers %q: expected a non-negative integer", value)
		}
		numbers = append(numbers, number)
	}

	return numbers, nil
}

// Reports whether an XID burst list response has another page. The endpoint
// reports no hasMore flag, so it is derived from the page counters.
func xidBurstPageHasMore(page nvfleetint.XIDBurstsPage) bool {
	if page.Page < 0 || page.PageSize <= 0 || page.Total <= 0 {
		return false
	}
	// Page is 0-indexed, so the first (page+1) pages have been seen so far.
	return (page.Page+1)*page.PageSize < page.Total
}

// Writes JSON or table output for xid burst list results
func writeXIDBurstListOutput(w io.Writer, common resolvedCommonFlags, result xidBurstListOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	headers := []string{
		"BURST ID", "HOSTNAME", "XIDS", "XID COUNT", "START TIME", "DURATION (S)",
		"JOB DISRUPTION", "NODE GROUP", "COMPUTE ZONE",
	}
	if err := clioutput.WriteTable(w, headers, xidBurstRows(result.Bursts)); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Converts XID bursts into table rows. Category, subcategory, device IDs, and
// suggested actions are omitted to keep the table narrow; use describe or
// -o json for the full payload.
func xidBurstRows(bursts []nvfleetint.XIDBurst) [][]string {
	rows := make([][]string, 0, len(bursts))
	for _, burst := range bursts {
		rows = append(rows, []string{
			clioutput.DisplayString(burst.BurstID),
			clioutput.DisplayString(burst.Hostname),
			formatXIDNumbers(burst.XIDNumbers),
			clioutput.FormatOptionalInt(burst.XIDCount),
			clioutput.DisplayString(burst.StartTime),
			clioutput.FormatOptionalInt(burst.BurstDurationSeconds),
			clioutput.FormatOptionalBool(burst.JobDisruption),
			clioutput.DisplayString(burst.NodeGroup),
			clioutput.DisplayString(burst.ComputeZone),
		})
	}
	return rows
}

// Renders XID burst detail fields as a table
func writeXIDBurstDescribeTable(w io.Writer, burst nvfleetint.XIDBurstDetails) error {
	return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, xidBurstDescribeRows(burst))
}

// Converts XID burst details into describe table rows
func xidBurstDescribeRows(burst nvfleetint.XIDBurstDetails) [][]string {
	rows := [][]string{
		{"BURST ID", clioutput.DisplayString(burst.BurstID)},
		{"NODE UUID", clioutput.DisplayString(burst.NodeUUID)},
		{"HOSTNAME", clioutput.DisplayString(burst.Hostname)},
		{"NODE GROUP", clioutput.FormatNameAndID(burst.NodeGroup, burst.NodeGroupID)},
		{"COMPUTE ZONE", clioutput.FormatNameAndID(burst.ComputeZone, burst.ComputeZoneID)},
		{"START TIME", clioutput.DisplayString(burst.StartTime)},
		{"END TIME", clioutput.DisplayString(burst.EndTime)},
		{"DURATION (S)", clioutput.FormatOptionalInt(burst.BurstDurationSeconds)},
		{"XID COUNT", clioutput.FormatOptionalInt(burst.XIDCount)},
		{"XIDS", formatXIDNumbers(burst.XIDNumbers)},
		{"JOB DISRUPTION", clioutput.FormatOptionalBool(burst.JobDisruption)},
		{"PLATFORM JOB DISRUPTION", clioutput.FormatOptionalBool(burst.JobDisruptionDueToPlatformIssue)},
		{"CATEGORY", clioutput.DisplayString(burst.Category)},
		{"SUBCATEGORY", clioutput.DisplayString(burst.Subcategory)},
		{"STICKY XIDS SUPPRESSED", clioutput.FormatOptionalInt(burst.StickyXIDsSuppressed)},
	}

	for _, xid := range burst.XIDNumbers {
		detail := strings.TrimSpace(strings.Join(nonEmptyValues(xid.Mnemonic, xid.Description), ": "))
		if detail == "" {
			continue
		}
		rows = append(rows, []string{"XID " + clioutput.FormatOptionalInt(xid.XIDNumber), detail})
	}
	rows = append(rows, xidBurstDeviceRows(burst.DeviceIDs)...)
	rows = append(rows, xidBurstActionRows(burst.SuggestedActions)...)

	return rows
}

// Converts the impacted-device map into describe table rows, sorted by device
// ID so repeated runs render identically
func xidBurstDeviceRows(devices map[string][]int) [][]string {
	if len(devices) == 0 {
		return nil
	}
	deviceIDs := make([]string, 0, len(devices))
	for deviceID := range devices {
		deviceIDs = append(deviceIDs, deviceID)
	}
	sort.Strings(deviceIDs)

	rows := make([][]string, 0, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		rows = append(rows, []string{"DEVICE " + deviceID, formatIntList(devices[deviceID])})
	}
	return rows
}

// Converts suggested actions into describe table rows
func xidBurstActionRows(actions []nvfleetint.SuggestedAction) [][]string {
	rows := make([][]string, 0, len(actions))
	for _, action := range actions {
		label := strings.TrimSpace(strings.Join(nonEmptyValues(action.Persona, action.Type), " "))
		if label == "" {
			label = "SUGGESTED ACTION"
		}
		rows = append(rows, []string{
			strings.ToUpper(label),
			clioutput.DisplayString(strings.Join(nonEmptyValues(action.Code, action.Action), ": ")),
		})
	}
	return rows
}

// Formats burst XID numbers as a comma-separated list
func formatXIDNumbers(xids []nvfleetint.XIDBurstXID) string {
	values := make([]string, 0, len(xids))
	for _, xid := range xids {
		if xid.XIDNumber == nil {
			continue
		}
		values = append(values, strconv.Itoa(*xid.XIDNumber))
	}
	return clioutput.FormatStringList(values)
}

// Formats integers as a comma-separated list
func formatIntList(values []int) string {
	formatted := make([]string, 0, len(values))
	for _, value := range values {
		formatted = append(formatted, strconv.Itoa(value))
	}
	return clioutput.FormatStringList(formatted)
}

// Returns the non-blank values in order
func nonEmptyValues(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

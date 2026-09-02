// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Lists the sort fields accepted by xidburst list
const xidBurstSortByList = "startTime, burstDurationSeconds, hostname, nodeUuid, nodeGroup, computeZone, " +
	"xidNumbers, xidCount, jobDisruption, jobDisruptionDueToPlatformIssue, category, subcategory, " +
	"tenantAction, tenantInvestigation, dcAdminAction, or dcAdminInvestigation"

// Stores local flag values for xidburst list
type xidBurstListFlags struct {
	window                     string
	start                      string
	end                        string
	node                       string
	nodeGroupIDs               string
	computeZoneIDs             string
	excludeNodeGroupIDs        string
	excludeComputeZoneIDs      string
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

// Stores data ready for xidburst list rendering
type xidBurstListOutput struct {
	Bursts    []nvfleetint.XIDBurst
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Creates the top-level xidburst command group
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "xidburst",
		Short: "Inspect finalized XID bursts",
		Long: "Inspect finalized XID bursts: groups of XID errors observed together on one node.\n\n" +
			"Workflow: list → describe. List the bursts in a time range, then describe one for its " +
			"impacted devices, XID catalog details, and suggested actions.",
		Example: "  nvfleetint xidburst list --window 24h\n" +
			"  nvfleetint xidburst list --window 168h --job-disruption --sort-by startTime\n" +
			"  nvfleetint xidburst describe <burst-id>\n" +
			"  nvfleetint xidburst options",
	}

	cmd.AddCommand(newXIDBurstListCmd())
	cmd.AddCommand(newXIDBurstDescribeCmd())
	cmd.AddCommand(newXIDBurstOptionsCmd())
	cmdutil.RejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the xidburst list command
func newXIDBurstListCmd() *cobra.Command {
	flags := xidBurstListFlags{}
	common := cmdutil.NewCommon()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List finalized XID bursts",
		Args:  cobra.NoArgs,
		Long: `List finalized XID bursts filtered by node scope, XID numbers, classification,
and a time range.

A time range is required: use --window for a relative range, or --start and
--end for an absolute range. The range applies to each burst's start time.

Some filters and response fields are only available to cloud-provider/NCP
callers; for tenant callers the backend rejects them.

` + cmdutil.OptionsHelpNote("nvfleetint xidburst options",
			"--xid-numbers", "--categories", "--subcategories", "--tenant-actions",
			"--tenant-investigations", "--dc-admin-actions", "--dc-admin-investigations",
			"--job-disruption", "--platform-disruption"),
		Example: `  nvfleetint xidburst list --window 24h
  nvfleetint xidburst list --start 2026-05-01T00:00:00Z --end 2026-05-08T00:00:00Z
  nvfleetint xidburst list --window 168h --xid-numbers 48,94 --job-disruption`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runXIDBurstList(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmdutil.RegisterTimeRangeFlags(cmd, &flags.window, &flags.start, &flags.end)
	cmd.Flags().StringVar(&flags.node, "node", "", "Filter by node UUID")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.excludeNodeGroupIDs, "exclude-nodegroup-ids", "",
		"Comma-separated node group IDs to exclude; cannot be combined with --nodegroup-ids")
	cmd.Flags().StringVar(&flags.excludeComputeZoneIDs, "exclude-compute-zone-ids", "",
		"Comma-separated compute zone IDs to exclude; cannot be combined with --compute-zone-ids")
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
	cmdutil.RegisterListFlags(cmd, common)

	return cmd
}

// Creates the xidburst describe command
func newXIDBurstDescribeCmd() *cobra.Command {
	common := cmdutil.NewCommon()
	cmd := &cobra.Command{
		Use:   "describe <burstID>",
		Short: "Describe a finalized XID burst",
		Args:  cmdutil.RequireSingleArg("burst ID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runXIDBurstDescribe(cmd, args[0], cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runXIDBurstList(cmd *cobra.Command, flags xidBurstListFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateListFlags(common); err != nil {
		return err
	}
	if err := cmdutil.ValidateTimeRangeFlags(flags.window, flags.start, flags.end); err != nil {
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
		{name: "exclude-nodegroup-ids", raw: flags.excludeNodeGroupIDs, assign: func(v []string) { opts.ExcludeNodeGroupIDs = v }},
		{name: "exclude-compute-zone-ids", raw: flags.excludeComputeZoneIDs, assign: func(v []string) { opts.ExcludeComputeZoneIDs = v }},
		{name: "categories", raw: flags.categories, assign: func(v []string) { opts.Categories = v }},
		{name: "subcategories", raw: flags.subcategories, assign: func(v []string) { opts.Subcategories = v }},
		{name: "tenant-actions", raw: flags.tenantActions, assign: func(v []string) { opts.TenantActions = v }},
		{name: "tenant-investigations", raw: flags.tenantInvestigations, assign: func(v []string) { opts.TenantInvestigations = v }},
		{name: "dc-admin-actions", raw: flags.dcAdminActions, assign: func(v []string) { opts.DCAdminActions = v }},
		{name: "dc-admin-investigations", raw: flags.dcAdminInvestigations, assign: func(v []string) { opts.DCAdminInvestigations = v }},
	} {
		values, err := cmdutil.ParseCommaList(filter.raw)
		if err != nil {
			return fmt.Errorf("invalid --%s: %w", filter.name, err)
		}
		filter.assign(values)
	}

	if len(opts.NodeGroupIDs) > 0 && len(opts.ExcludeNodeGroupIDs) > 0 {
		return errors.New("--nodegroup-ids cannot be used with --exclude-nodegroup-ids")
	}
	if len(opts.ComputeZoneIDs) > 0 && len(opts.ExcludeComputeZoneIDs) > 0 {
		return errors.New("--compute-zone-ids cannot be used with --exclude-compute-zone-ids")
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}
	cmdutil.ApplyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.All {
		var bursts []nvfleetint.XIDBurst
		result, err := cmdutil.FetchAllPages("items",
			func(pageNumber int) (nvfleetint.XIDBurstsPage, error) {
				opts.Page = &pageNumber
				return client.ListXIDBursts(cmd.Context(), opts)
			},
			func(page nvfleetint.XIDBurstsPage) { bursts = append(bursts, page.Bursts...) },
		)
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
func runXIDBurstDescribe(cmd *cobra.Command, burstID string, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}

	burst, err := client.DescribeXIDBurst(cmd.Context(), strings.TrimSpace(burstID))
	if err != nil {
		return err
	}
	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), burst.RawJSON)
	}
	return writeXIDBurstDescribeTable(cmd.OutOrStdout(), burst)
}

// Converts comma-separated XID numbers into API values
func parseXIDNumberList(raw string) ([]int, error) {
	values, err := cmdutil.ParseCommaList(raw)
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
// Writes JSON or table output for xidburst list results
func writeXIDBurstListOutput(w io.Writer, common cmdutil.Resolved, result xidBurstListOutput) error {
	if common.Output == clioutput.FormatJSON {
		return cmdutil.WritePaginatedListJSON(w, result.RawJSON, result.JSONValue)
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

// The command whose flags the XID burst options describe.
const xidBurstOptionsConsumer = "'xidburst list'"

// Stores local flag values for the XID burst options command
type xidBurstOptionsFlags struct {
	window                string
	start                 string
	end                   string
	nodeGroupIDs          string
	computeZoneIDs        string
	excludeNodeGroupIDs   string
	excludeComputeZoneIDs string
}

// Creates the xid burst options command
func newXIDBurstOptionsCmd() *cobra.Command {
	flags := xidBurstOptionsFlags{}
	common := cmdutil.NewCommon()
	cmd := &cobra.Command{
		Use:   "options",
		Short: "List available XID burst filters",
		Args:  cobra.NoArgs,
		Long: `List the filter values available for 'xidburst list'.

Every filter is optional. Supplying a time range or an inventory scope narrows
the values to those present in that slice of the fleet, so each remaining option
is one that returns results. Unlike 'xidburst list', a time range is not
required. Column filters are not applied, and no counts are returned.`,
		Example: `  nvfleetint xidburst options
  nvfleetint xidburst options --window 24h
  nvfleetint xidburst options --window 168h --compute-zone-ids cz-1`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runXIDBurstOptions(cmd, flags, cmdutil.ResolveCommon(cmd, common))
		},
	}

	cmdutil.RegisterTimeRangeFlags(cmd, &flags.window, &flags.start, &flags.end)
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to scope the options to")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to scope the options to")
	cmd.Flags().StringVar(&flags.excludeNodeGroupIDs, "exclude-nodegroup-ids", "",
		"Comma-separated node group IDs to exclude; cannot be combined with --nodegroup-ids")
	cmd.Flags().StringVar(&flags.excludeComputeZoneIDs, "exclude-compute-zone-ids", "",
		"Comma-separated compute zone IDs to exclude; cannot be combined with --compute-zone-ids")
	cmdutil.RegisterReadFlags(cmd, common)

	return cmd
}

// Gets and renders the filter values available for XID burst queries.
func runXIDBurstOptions(cmd *cobra.Command, flags xidBurstOptionsFlags, common cmdutil.Resolved) error {
	if err := cmdutil.ValidateReadFlags(common); err != nil {
		return err
	}
	// The range narrows the options rather than selecting them, so unlike
	// 'xidburst list' omitting it entirely is valid.
	if err := cmdutil.ValidateOptionalTimeRangeFlags(flags.window, flags.start, flags.end); err != nil {
		return err
	}

	scope := nvfleetint.XIDBurstFilterOptionsScope{
		Window:    strings.TrimSpace(flags.window),
		StartTime: strings.TrimSpace(flags.start),
		EndTime:   strings.TrimSpace(flags.end),
	}
	for _, filter := range []struct {
		name   string
		raw    string
		assign func([]string)
	}{
		{name: "nodegroup-ids", raw: flags.nodeGroupIDs, assign: func(v []string) { scope.NodeGroupIDs = v }},
		{name: "compute-zone-ids", raw: flags.computeZoneIDs, assign: func(v []string) { scope.ComputeZoneIDs = v }},
		{name: "exclude-nodegroup-ids", raw: flags.excludeNodeGroupIDs, assign: func(v []string) { scope.ExcludeNodeGroupIDs = v }},
		{name: "exclude-compute-zone-ids", raw: flags.excludeComputeZoneIDs, assign: func(v []string) { scope.ExcludeComputeZoneIDs = v }},
	} {
		values, err := cmdutil.ParseCommaList(filter.raw)
		if err != nil {
			return fmt.Errorf("invalid --%s: %w", filter.name, err)
		}
		filter.assign(values)
	}

	client, err := cmdutil.New(common)
	if err != nil {
		return err
	}
	options, err := client.GetXIDBurstFilterOptions(cmd.Context(), scope)
	if err != nil {
		return err
	}
	if common.Output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), options.RawJSON)
	}
	return writeXIDBurstOptionsTable(cmd.OutOrStdout(), options)
}

// Writes XID burst filter values grouped by the flag that accepts them. The XID
// endpoint returns per-field value lists rather than the shared filters/sorting
// envelope, and publishes no sorting metadata, so this renders filters only.
func writeXIDBurstOptionsTable(w io.Writer, options nvfleetint.XIDBurstFilterOptions) error {
	if _, err := fmt.Fprintf(w, "Filters for %s\n%s\n", xidBurstOptionsConsumer, cmdutil.OptionsPreamble); err != nil {
		return err
	}

	sections := []cmdutil.OptionSection{
		{Heading: "--xid-numbers", Rows: xidNumberRows(options.XIDNumbers)},
		{Heading: "--categories", Rows: cmdutil.ValueRows(options.Categories)},
		{Heading: "--subcategories", Rows: cmdutil.ValueRows(options.Subcategories)},
	}
	sections = append(sections, xidActionSections(options.SuggestedActions)...)
	sections = append(sections,
		cmdutil.OptionSection{
			Heading: "--job-disruption  (boolean; pass --job-disruption=false to match false)",
			Rows:    boolRows(options.JobDisruption),
		},
		cmdutil.OptionSection{
			Heading: "--platform-disruption  (boolean; pass --platform-disruption=false to match false)",
			Rows:    boolRows(options.JobDisruptionDueToPlatformIssue),
		},
	)

	for _, section := range sections {
		if err := cmdutil.WriteOptionSection(w, section.Heading, section.Rows); err != nil {
			return err
		}
	}
	return nil
}

// Splits suggested actions into the four persona/type flags that accept them.
// The API omits persona when it has already reduced actions to one persona,
// which for this endpoint means a tenant key, so a blank persona is read as
// tenant. Actions whose persona or type the CLI has no flag for are listed
// separately rather than guessed into a flag or dropped, so a backend that
// grows a persona still shows the codes its filters accept.
func xidActionSections(actions []nvfleetint.SuggestedAction) []cmdutil.OptionSection {
	buckets := map[string][][]string{}
	var unclassified [][]string
	for _, action := range actions {
		persona := action.Persona
		if persona == "" {
			persona = nvfleetint.ActionPersonaTenant
		}
		row := []string{action.Code, action.Action}
		if !knownActionPersona(persona) || !knownActionType(action.Type) {
			unclassified = append(unclassified, row)
			continue
		}
		buckets[persona+"/"+action.Type] = append(buckets[persona+"/"+action.Type], row)
	}

	ordered := []struct {
		key  string
		flag string
	}{
		{nvfleetint.ActionPersonaTenant + "/" + nvfleetint.ActionTypeImmediate, "--tenant-actions"},
		{nvfleetint.ActionPersonaTenant + "/" + nvfleetint.ActionTypeInvestigatory, "--tenant-investigations"},
		{nvfleetint.ActionPersonaDCAdmin + "/" + nvfleetint.ActionTypeImmediate, "--dc-admin-actions"},
		{nvfleetint.ActionPersonaDCAdmin + "/" + nvfleetint.ActionTypeInvestigatory, "--dc-admin-investigations"},
	}

	sections := make([]cmdutil.OptionSection, 0, len(ordered)+1)
	for _, entry := range ordered {
		sections = append(sections, cmdutil.OptionSection{
			Heading: entry.flag,
			Rows:    buckets[entry.key],
		})
	}
	if len(unclassified) > 0 {
		sections = append(sections, cmdutil.OptionSection{
			Heading: fmt.Sprintf("suggestedActions with no matching persona/type flag  (no flag on %s)", xidBurstOptionsConsumer),
			Rows:    unclassified,
		})
	}
	return sections
}

// Reports whether a suggested-action persona has flags on `xidburst list`.
func knownActionPersona(persona string) bool {
	return persona == nvfleetint.ActionPersonaTenant || persona == nvfleetint.ActionPersonaDCAdmin
}

// Reports whether a suggested-action type has flags on `xidburst list`.
func knownActionType(actionType string) bool {
	return actionType == nvfleetint.ActionTypeImmediate || actionType == nvfleetint.ActionTypeInvestigatory
}

// Converts XID numbers into single-column rows.
func xidNumberRows(numbers []int) [][]string {
	rows := make([][]string, 0, len(numbers))
	for _, number := range numbers {
		rows = append(rows, []string{strconv.Itoa(number)})
	}
	return rows
}

// Converts booleans into single-column rows.
func boolRows(values []bool) [][]string {
	rows := make([][]string, 0, len(values))
	for _, value := range values {
		rows = append(rows, []string{strconv.FormatBool(value)})
	}
	return rows
}

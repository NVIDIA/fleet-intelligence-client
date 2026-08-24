// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
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

// Stores local flag values for alert list
type alertListFlags struct {
	node      string
	component string
	state     string
	severity  string
}

// Stores local flag values for alert timeline
type alertTimelineFlags struct {
	active         bool
	view           string
	node           string
	hostname       string
	sortBy         string
	order          string
	gpuType        string
	nodeGroupIDs   string
	computeZoneIDs string
	alertState     string
	componentType  string
	withoutPSIRT   bool
}

// Stores local flag values for alert timeline options.
type alertTimelineOptionsFlags struct {
	view string
}

// Stores local flag values for alert describe
type alertDescribeFlags struct {
	node     string
	order    string
	page     int
	pageSize int
}

// Stores data ready for alert list rendering
type alertListOutput struct {
	Alerts    []nvfleetint.Alert
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Stores data ready for alert timeline rendering
type alertTimelineOutput struct {
	Nodes     []nvfleetint.AlertTimelineNode
	Alerts    []nvfleetint.AlertTimelineNodeAlert
	Mode      string
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Preserves level-1 cross-page aggregates in normalized --all JSON output
type alertTimelineNodesAllJSON struct {
	Items                    []json.RawMessage           `json:"items"`
	Pagination               clihelpers.MergedPagination `json:"pagination"`
	TotalCritical            int                         `json:"totalCritical"`
	TotalWarning             int                         `json:"totalWarning"`
	TotalResolved            int                         `json:"totalResolved"`
	DistinctGPUTypeCount     int                         `json:"distinctGpuTypeCount"`
	DistinctNodeGroupCount   int                         `json:"distinctNodeGroupCount"`
	DistinctComputeZoneCount int                         `json:"distinctComputeZoneCount"`
}

const (
	alertTimelineModeNodes  = "nodes"
	alertTimelineModeAlerts = "alerts"
)

// Creates the top-level alert command group
func newAlertCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "alert",
		Short: "Inspect and investigate fleet alerts",
		Long: "Inspect fleet alerts from aggregate impact through individual event history.\n\n" +
			"Workflow: summary → node → describe. Start with fleet impact, inspect one node's alerts, " +
			"then describe an alert for its complete event timeline.",
		Example: "  nvfleetint alert summary\n" +
			"  nvfleetint alert node <node-uuid>\n" +
			"  nvfleetint alert describe <alert-uuid> --node <node-uuid>\n" +
			"  nvfleetint alert list --severity Critical",
	}

	cmd.AddCommand(newAlertListCmd())
	cmd.AddCommand(newAlertSummaryCmd())
	cmd.AddCommand(newAlertNodeCmd())
	cmd.AddCommand(newAlertDescribeCmd())
	cmd.AddCommand(newAlertOptionsCmd())
	rejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the alert list command
func newAlertListCmd() *cobra.Command {
	flags := alertListFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List alerts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAlertList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.node, "node", "", "Node UUID to filter")
	cmd.Flags().StringVar(&flags.component, "component", "", "Component name to filter")
	cmd.Flags().StringVar(&flags.state, "state", "", "Alert state to filter: Detected, Triggered, or Resolved")
	cmd.Flags().StringVar(&flags.severity, "severity", "", "Alert severity to filter: Critical or Warning")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the canonical impacted-node summary command.
func newAlertSummaryCmd() *cobra.Command {
	flags := alertTimelineFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "summary",
		Short: "Summarize impacted nodes and their alert counts",
		Long: "Summarize impacted nodes and their alert counts.\n\n" +
			optionsHelpNote("nvfleetint alert options",
				"--gpu-type", "--nodegroup-ids", "--compute-zone-ids",
				"--alert-state", "--component-type", "--sort-by", "--order"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAlertSummary(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", "", "Alert view: active or historical (default: active)")
	cmd.Flags().StringVar(&flags.hostname, "hostname", "", "Hostname partial match")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field")
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order")
	cmd.Flags().StringVar(&flags.gpuType, "gpu-type", "", "Comma-separated GPU types to filter")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.alertState, "alert-state", "", "Comma-separated timeline states to filter")
	cmd.Flags().StringVar(&flags.componentType, "component-type", "", "Comma-separated component types to include")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the command that lists timeline alerts for one node.
func newAlertNodeCmd() *cobra.Command {
	flags := alertTimelineFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "node <nodeUUID>",
		Short: "List alerts for one node",
		// The options endpoint advertises `alert summary`'s sort columns only,
		// so --sort-by/--order are spelled out on the flags here rather than
		// pointed at a command that would list the wrong values.
		Long: "List alerts for one node.\n\n" +
			optionsHelpNote("nvfleetint alert options",
				"--gpu-type", "--nodegroup-ids", "--compute-zone-ids",
				"--alert-state", "--component-type"),
		Args: requireSingleArg("node UUID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAlertNode(cmd, args[0], flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", "", "Alert view: active or historical (default: active)")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field: startTime or lastUpdate (default: lastUpdate)")
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order: asc or desc (default: desc)")
	cmd.Flags().StringVar(&flags.gpuType, "gpu-type", "", "Comma-separated GPU types to filter")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.alertState, "alert-state", "", "Comma-separated timeline states to filter")
	cmd.Flags().StringVar(&flags.componentType, "component-type", "", "Comma-separated component types to include")
	cmd.Flags().BoolVar(&flags.withoutPSIRT, "without-psirt", false, "Exclude PSIRT alerts")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the top-level alert filter-options command.
func newAlertOptionsCmd() *cobra.Command {
	flags := alertTimelineOptionsFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "options",
		Short: "List available alert filters and sorting options",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAlertTimelineOptions(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", "", "Alert view: active or historical (default: active)")
	registerReadCommonFlags(cmd, common)
	return cmd
}

// Creates the alert describe command
func newAlertDescribeCmd() *cobra.Command {
	flags := alertDescribeFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "describe <alertUUID>",
		Short: "Describe an alert timeline",
		Args:  requireSingleArg("alert UUID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAlertDescribe(cmd, args[0], flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.node, "node", "", "Node UUID for the alert")
	cmd.Flags().StringVar(&flags.order, "order", "", "Timeline event order: asc or desc")
	cmd.Flags().IntVar(&flags.page, "page", 1, "Timeline event page to fetch (1-based; requires --page-size)")
	cmd.Flags().IntVar(&flags.pageSize, "page-size", 0, "Timeline events per page (1-100); omit for the full timeline")
	registerReadCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runAlertList(cmd *cobra.Command, flags alertListFlags, common resolvedCommonFlags) error {
	opts := nvfleetint.ListAlertsOptions{
		NodeUUID:  strings.TrimSpace(flags.node),
		Component: strings.TrimSpace(flags.component),
		State:     nvfleetint.AlertState(strings.TrimSpace(flags.state)),
		Severity:  nvfleetint.AlertSeverity(strings.TrimSpace(flags.severity)),
	}
	if err := opts.Validate(); err != nil {
		return renderOptionError(err, alertListFlagNames)
	}
	if err := validateListCommonFlags(common); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var alerts []nvfleetint.Alert
		result, err := clihelpers.FetchAllPages("alerts",
			func(pageNumber int) (nvfleetint.AlertsPage, error) {
				opts.Page = &pageNumber
				return client.ListAlerts(cmd.Context(), opts)
			},
			func(page nvfleetint.AlertsPage) { alerts = append(alerts, page.Alerts...) },
		)
		if err != nil {
			return err
		}
		return writeAlertListOutput(cmd.OutOrStdout(), common, alertListOutput{
			Alerts:    alerts,
			JSONValue: result,
		})
	}

	page, err := client.ListAlerts(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeAlertListOutput(cmd.OutOrStdout(), common, alertListOutput{
		Alerts:  page.Alerts,
		RawJSON: page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	})
}

// Validates summary flags, calls timeline level 1, and writes output.
func runAlertSummary(cmd *cobra.Command, flags alertTimelineFlags, common resolvedCommonFlags) error {
	active, err := resolveAlertView(flags.view)
	if err != nil {
		return err
	}
	flags.active = active
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if err := validateAlertTimelineFlags(flags); err != nil {
		return err
	}
	opts, err := alertTimelineNodesOptions(flags)
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	return runAlertTimelineNodes(cmd, client, opts, common)
}

// Validates node flags, calls timeline level 2, and writes output.
func runAlertNode(cmd *cobra.Command, nodeUUID string, flags alertTimelineFlags, common resolvedCommonFlags) error {
	active, err := resolveAlertView(flags.view)
	if err != nil {
		return err
	}
	flags.active = active
	flags.node = strings.TrimSpace(nodeUUID)
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if err := validateAlertTimelineFlags(flags); err != nil {
		return err
	}
	opts, err := nodeAlertTimelineOptions(flags)
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}
	return runNodeAlertTimeline(cmd, client, opts, common)
}

// Gets and renders the filters and sorting choices available for an alert timeline view.
func runAlertTimelineOptions(cmd *cobra.Command, flags alertTimelineOptionsFlags, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}
	active, err := resolveAlertView(flags.view)
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}
	options, err := client.GetAlertTimelineFilterOptions(cmd.Context(), active)
	if err != nil {
		return err
	}
	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), options.RawJSON)
	}
	return writeAlertTimelineOptionsTable(cmd.OutOrStdout(), options)
}

// Resolves an alert view, defaulting operational commands to active alerts.
func resolveAlertView(view string) (bool, error) {
	switch normalized := strings.ToLower(strings.TrimSpace(view)); normalized {
	case "":
		return true, nil
	case "active":
		return true, nil
	case "historical":
		return false, nil
	default:
		return false, fmt.Errorf("invalid view %q: expected active or historical", view)
	}
}

// Lists nodes with alert timeline history
func runAlertTimelineNodes(cmd *cobra.Command, client *nvfleetint.Client, opts nvfleetint.ListAlertTimelineNodesOptions, common resolvedCommonFlags) error {
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var nodes []nvfleetint.AlertTimelineNode
		var aggregates nvfleetint.AlertTimelineNodesPage
		haveAggregates := false
		result, err := clihelpers.FetchAllPages("nodes",
			func(pageNumber int) (nvfleetint.AlertTimelineNodesPage, error) {
				opts.Page = &pageNumber
				return client.ListAlertTimelineNodes(cmd.Context(), opts)
			},
			func(page nvfleetint.AlertTimelineNodesPage) {
				nodes = append(nodes, page.Nodes...)
				// The fleet-wide counts are the same on every page; keep the
				// first so they survive into the merged output.
				if !haveAggregates {
					aggregates = page
					haveAggregates = true
				}
			},
		)
		if err != nil {
			return err
		}
		pagination := result.Pagination
		pagination.Page++
		return writeAlertTimelineOutput(cmd.OutOrStdout(), common, alertTimelineOutput{
			Nodes: nodes,
			Mode:  alertTimelineModeNodes,
			JSONValue: alertTimelineNodesAllJSON{
				Items: result.Items, Pagination: pagination,
				TotalCritical: aggregates.TotalCritical, TotalWarning: aggregates.TotalWarning,
				TotalResolved: aggregates.TotalResolved, DistinctGPUTypeCount: aggregates.DistinctGPUTypeCount,
				DistinctNodeGroupCount:   aggregates.DistinctNodeGroupCount,
				DistinctComputeZoneCount: aggregates.DistinctComputeZoneCount,
			},
		})
	}

	page, err := client.ListAlertTimelineNodes(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeAlertTimelineOutput(cmd.OutOrStdout(), common, alertTimelineOutput{
		Nodes:   page.Nodes,
		Mode:    alertTimelineModeNodes,
		RawJSON: page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	})
}

// Lists alert timeline history for one node
func runNodeAlertTimeline(cmd *cobra.Command, client *nvfleetint.Client, opts nvfleetint.ListNodeAlertTimelineOptions, common resolvedCommonFlags) error {
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var alerts []nvfleetint.AlertTimelineNodeAlert
		result, err := clihelpers.FetchAllPages("alerts",
			func(pageNumber int) (nvfleetint.NodeAlertTimelinePage, error) {
				opts.Page = &pageNumber
				return client.ListNodeAlertTimeline(cmd.Context(), opts)
			},
			func(page nvfleetint.NodeAlertTimelinePage) { alerts = append(alerts, page.Alerts...) },
		)
		if err != nil {
			return err
		}
		return writeAlertTimelineOutput(cmd.OutOrStdout(), common, alertTimelineOutput{
			Alerts:    alerts,
			Mode:      alertTimelineModeAlerts,
			JSONValue: result,
		})
	}

	page, err := client.ListNodeAlertTimeline(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeAlertTimelineOutput(cmd.OutOrStdout(), common, alertTimelineOutput{
		Alerts:  page.Alerts,
		Mode:    alertTimelineModeAlerts,
		RawJSON: page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	})
}

// Validates args, calls the SDK, and writes output
func runAlertDescribe(cmd *cobra.Command, alertUUID string, flags alertDescribeFlags, common resolvedCommonFlags) error {
	if err := validateAlertDescribeFlags(cmd, flags, common); err != nil {
		return err
	}

	nodeUUID := strings.TrimSpace(flags.node)
	alertUUID = strings.TrimSpace(alertUUID)
	if nodeUUID == "" {
		return errors.New("--node is required")
	}
	if alertUUID == "" {
		return errors.New("alert UUID is required")
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	opts := nvfleetint.DescribeAlertTimelineOptions{
		Order: nvfleetint.AlertTimelineSortOrder(strings.TrimSpace(flags.order)),
	}
	if cmd.Flags().Changed("page") {
		page := flags.page - 1
		opts.Page = &page
	}
	if cmd.Flags().Changed("page-size") {
		pageSize := flags.pageSize
		opts.PageSize = &pageSize
	}
	details, err := client.DescribeAlertTimelineWithOptions(cmd.Context(), nodeUUID, alertUUID, opts)
	if err != nil {
		return err
	}
	if common.output == clioutput.FormatJSON {
		if opts.PageSize != nil {
			return clioutput.WriteRawJSON(cmd.OutOrStdout(), clihelpers.OneIndexRawPage(details.RawJSON))
		}
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), details.RawJSON)
	}
	if err := writeAlertDescribeTable(cmd.OutOrStdout(), details); err != nil {
		return err
	}
	if opts.PageSize == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(cmd.OutOrStdout(), clioutput.Pagination{
		Page: details.Page, PageSize: details.PageSize, Total: details.Total,
	})
}

// Checks the alert timeline rules that are about which flags were combined
// rather than about a value. Value checks live in the SDK, reached through the
// option builders below.
func validateAlertTimelineFlags(flags alertTimelineFlags) error {
	if strings.TrimSpace(flags.node) == "" {
		if flags.withoutPSIRT {
			return errors.New("--without-psirt requires --node")
		}
		return nil
	}
	if strings.TrimSpace(flags.hostname) != "" {
		return errors.New("--hostname cannot be used with --node")
	}
	return nil
}

// Names the flag carrying each alert timeline option. The two timeline levels
// accept different sort fields, so each names its own.
var (
	alertTimelineNodesFlagNames = map[string]optionFlagName{
		"order":      {flag: "order"},
		"alertState": {flag: "alert-state"},
		"sortBy": {
			flag:     "sort-by",
			expected: "hostname, alert, gpuType, nodeGroup, computeZone, or lastUpdate",
		},
	}
	nodeAlertTimelineFlagNames = map[string]optionFlagName{
		"order":      {flag: "order"},
		"alertState": {flag: "alert-state"},
		"sortBy":     {flag: "sort-by", expected: "startTime or lastUpdate"},
	}
)

// Reads the filters shared by both alert timeline levels
func alertTimelineFilters(flags alertTimelineFlags) (states []nvfleetint.AlertTimelineState, lists [4][]string, err error) {
	states, err = clihelpers.ParseTypedList[nvfleetint.AlertTimelineState]("alert-state", flags.alertState)
	if err != nil {
		return nil, lists, err
	}
	for index, filter := range [4]struct {
		name string
		raw  string
	}{
		{name: "component-type", raw: flags.componentType},
		{name: "gpu-type", raw: flags.gpuType},
		{name: "nodegroup-ids", raw: flags.nodeGroupIDs},
		{name: "compute-zone-ids", raw: flags.computeZoneIDs},
	} {
		values, parseErr := clihelpers.ParseCommaList(filter.raw)
		if parseErr != nil {
			return nil, lists, fmt.Errorf("invalid --%s: %w", filter.name, parseErr)
		}
		lists[index] = values
	}
	return states, lists, nil
}

// alertTimelineNodesOptions reads the level-1 timeline flags exactly once and
// hands the result to the SDK to validate.
func alertTimelineNodesOptions(flags alertTimelineFlags) (nvfleetint.ListAlertTimelineNodesOptions, error) {
	states, lists, err := alertTimelineFilters(flags)
	if err != nil {
		return nvfleetint.ListAlertTimelineNodesOptions{}, err
	}

	opts := nvfleetint.ListAlertTimelineNodesOptions{
		Active:         flags.active,
		Hostname:       strings.TrimSpace(flags.hostname),
		SortBy:         nvfleetint.AlertTimelineNodeSortBy(strings.TrimSpace(flags.sortBy)),
		Order:          nvfleetint.AlertTimelineSortOrder(strings.TrimSpace(flags.order)),
		AlertStates:    states,
		ComponentTypes: lists[0],
		GPUTypes:       lists[1],
		NodeGroupIDs:   lists[2],
		ComputeZoneIDs: lists[3],
	}
	if err := opts.Validate(); err != nil {
		return opts, renderOptionError(err, alertTimelineNodesFlagNames)
	}

	return opts, nil
}

// nodeAlertTimelineOptions reads the level-2 timeline flags exactly once and
// hands the result to the SDK to validate.
func nodeAlertTimelineOptions(flags alertTimelineFlags) (nvfleetint.ListNodeAlertTimelineOptions, error) {
	states, lists, err := alertTimelineFilters(flags)
	if err != nil {
		return nvfleetint.ListNodeAlertTimelineOptions{}, err
	}

	opts := nvfleetint.ListNodeAlertTimelineOptions{
		NodeUUID:       strings.TrimSpace(flags.node),
		Active:         flags.active,
		WithoutPSIRT:   flags.withoutPSIRT,
		SortBy:         nvfleetint.AlertTimelineAlertSortBy(strings.TrimSpace(flags.sortBy)),
		Order:          nvfleetint.AlertTimelineSortOrder(strings.TrimSpace(flags.order)),
		AlertStates:    states,
		ComponentTypes: lists[0],
		GPUTypes:       lists[1],
		NodeGroupIDs:   lists[2],
		ComputeZoneIDs: lists[3],
	}
	if err := opts.Validate(); err != nil {
		return opts, renderOptionError(err, nodeAlertTimelineFlagNames)
	}

	return opts, nil
}

// Checks alert describe flags
func validateAlertDescribeFlags(cmd *cobra.Command, flags alertDescribeFlags, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}
	if flags.order != "" && !nvfleetint.AlertTimelineSortOrder(strings.TrimSpace(flags.order)).Valid() {
		return fmt.Errorf("invalid order %q: expected asc or desc", flags.order)
	}
	if cmd.Flags().Changed("page") && !cmd.Flags().Changed("page-size") {
		return errors.New("--page requires --page-size")
	}
	if cmd.Flags().Changed("page") && flags.page < 1 {
		return errors.New("--page must be greater than or equal to 1")
	}
	if cmd.Flags().Changed("page-size") && (flags.pageSize < clihelpers.MinPageSize || flags.pageSize > clihelpers.MaxPageSize) {
		return fmt.Errorf("--page-size must be between %d and %d", clihelpers.MinPageSize, clihelpers.MaxPageSize)
	}
	return nil
}

// Names the flag carrying each alert list option, for rendering SDK validation
// errors against what the user typed.
var alertListFlagNames = map[string]optionFlagName{
	"state":    {flag: "state"},
	"severity": {flag: "severity"},
}

// Writes JSON or table output for alert list results
func writeAlertListOutput(w io.Writer, common resolvedCommonFlags, result alertListOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	if err := clioutput.WriteTable(w, []string{"UUID", "NODE UUID", "COMPONENT", "SEVERITY", "STATE", "FIRED-AT"}, alertRows(result.Alerts)); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Writes JSON or table output for alert timeline results
func writeAlertTimelineOutput(w io.Writer, common resolvedCommonFlags, result alertTimelineOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	var err error
	if result.Mode == alertTimelineModeAlerts {
		err = clioutput.WriteTable(w, []string{"ALERT UUID", "COMPONENT", "STATUS", "START TIME", "LAST EVENT TIME"}, alertTimelineAlertRows(result.Alerts))
	} else {
		err = clioutput.WriteTable(w, []string{
			"NODE UUID", "HOSTNAME", "CRITICAL", "WARNING", "RESOLVED", "GPU TYPE", "NODE GROUP", "COMPUTE ZONE", "LAST ALERT TIME",
		}, alertTimelineNodeRows(result.Nodes))
	}
	if err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Maps each filter field returned by the alert options endpoint to the flag on
// `alert summary` / `alert node` that consumes it.
var alertOptionsRenderer = optionsRenderer{
	consumers: []string{"alert summary", "alert node"},
	filters: map[string]optionFlag{
		"gpuTypes":       {name: "--gpu-type"},
		"nodeGroups":     {name: "--nodegroup-ids"},
		"computeZones":   {name: "--compute-zone-ids", promote: "--nodegroup-ids"},
		"componentTypes": {name: "--component-type"},
		"alertStates":    {name: "--alert-state"},
	},
	// The endpoint advertises only the Level 1 nodes-list columns, which are
	// `alert summary`'s, so the sorting block is labelled as that command's
	// alone. `alert node`'s own columns are not restated here — only what the
	// endpoint actually returns is shown; see `alert node --help` for those.
	sortAccepted: func(field string) bool {
		return nvfleetint.AlertTimelineNodeSortBy(field).Valid()
	},
	sortConsumers: []string{"alert summary"},
}

// Writes alert timeline filter values grouped by the flag that accepts them,
// followed by the sorting flags.
func writeAlertTimelineOptionsTable(w io.Writer, options nvfleetint.AlertTimelineFilterOptions) error {
	return alertOptionsRenderer.write(w, options)
}

// Renders alert timeline events as a table
// Maximum width for free-text alert timeline columns. Longer values are
// truncated with an ellipsis to keep the table aligned; the full text is
// available via -o json.
const alertMessageColumnWidth = 80

func writeAlertDescribeTable(w io.Writer, details nvfleetint.AlertTimelineDetails) error {
	rows := make([][]string, 0, len(details.Timeline))
	for _, event := range details.Timeline {
		rows = append(rows, []string{
			clioutput.DisplayString(event.EventTimestamp),
			clioutput.DisplayString(event.EventType),
			clioutput.DisplayString(event.AlertStatus),
			clioutput.Truncate(clioutput.DisplayString(event.Message), alertMessageColumnWidth),
			clioutput.Truncate(clioutput.DisplayString(event.Error), alertMessageColumnWidth),
		})
	}
	return clioutput.WriteTable(w, []string{"TIMESTAMP", "EVENT", "STATUS", "MESSAGE", "ERROR"}, rows)
}

// Converts alerts into table rows
func alertRows(alerts []nvfleetint.Alert) [][]string {
	rows := make([][]string, 0, len(alerts))
	for _, alert := range alerts {
		rows = append(rows, []string{
			clioutput.DisplayString(alert.UUID),
			clioutput.DisplayString(alert.NodeUUID),
			clioutput.DisplayString(alert.Component),
			clioutput.DisplayString(alert.Severity),
			clioutput.DisplayString(alert.State),
			clioutput.DisplayString(alert.FiredAt),
		})
	}
	return rows
}

// Converts alert timeline nodes into table rows
func alertTimelineNodeRows(nodes []nvfleetint.AlertTimelineNode) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.NodeUUID),
			clioutput.DisplayString(node.Hostname),
			fmt.Sprintf("%d", node.CriticalCount),
			fmt.Sprintf("%d", node.WarningCount),
			fmt.Sprintf("%d", node.ResolvedCount),
			clioutput.DisplayString(node.GPUType),
			clioutput.DisplayString(node.NodeGroup),
			clioutput.DisplayString(node.ComputeZone),
			clioutput.DisplayString(node.LastAlertTime),
		})
	}
	return rows
}

// Converts node alert timeline entries into table rows
func alertTimelineAlertRows(alerts []nvfleetint.AlertTimelineNodeAlert) [][]string {
	rows := make([][]string, 0, len(alerts))
	for _, alert := range alerts {
		rows = append(rows, []string{
			clioutput.DisplayString(alert.AlertUUID),
			clioutput.DisplayString(alert.Component),
			clioutput.DisplayString(alert.AlertStatus),
			clioutput.DisplayString(alert.StartTime),
			clioutput.DisplayString(alert.LastEventTime),
		})
	}
	return rows
}

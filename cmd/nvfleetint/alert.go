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
	active bool
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
		Short: "Inspect alerts and alert timelines",
	}

	cmd.AddCommand(newAlertListCmd())
	cmd.AddCommand(newAlertTimelineCmd())
	cmd.AddCommand(newAlertDescribeCmd())

	return cmd
}

// Creates the alert list command
func newAlertListCmd() *cobra.Command {
	flags := alertListFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List alerts",
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

// Creates the alert timeline command
func newAlertTimelineCmd() *cobra.Command {
	flags := alertTimelineFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "timeline",
		Short: "List alert timelines",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAlertTimeline(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().BoolVar(&flags.active, "active", false, "Show only currently active alerts")
	cmd.Flags().StringVar(&flags.node, "node", "", "Node UUID whose alert history should be listed")
	cmd.Flags().StringVar(&flags.hostname, "hostname", "", "Hostname partial match (impacted-node view only)")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field; available values depend on whether --node is used")
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order: asc or desc")
	cmd.Flags().StringVar(&flags.gpuType, "gpu-type", "", "Comma-separated GPU types to filter")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.alertState, "alert-state", "", "Comma-separated timeline states: Critical, Warning, or Resolved")
	cmd.Flags().StringVar(&flags.componentType, "component-type", "", "Comma-separated component types to include")
	cmd.Flags().BoolVar(&flags.withoutPSIRT, "without-psirt", false, "Exclude PSIRT alerts (requires --node)")
	registerListCommonFlags(cmd, common)
	cmd.AddCommand(newAlertTimelineOptionsCmd())

	return cmd
}

// Creates the alert timeline filter-options command.
func newAlertTimelineOptionsCmd() *cobra.Command {
	flags := alertTimelineOptionsFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "options",
		Short: "List available alert timeline filters and sorting options",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAlertTimelineOptions(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().BoolVar(&flags.active, "active", false, "Show options for currently active alerts")
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
	state, err := parseAlertState(flags.state)
	if err != nil {
		return err
	}
	severity, err := parseAlertSeverity(flags.severity)
	if err != nil {
		return err
	}
	if err := validateListCommonFlags(common); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	opts := nvfleetint.ListAlertsOptions{
		NodeUUID:  strings.TrimSpace(flags.node),
		Component: strings.TrimSpace(flags.component),
		State:     state,
		Severity:  severity,
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var alerts []nvfleetint.Alert
		result, err := clihelpers.FetchAllRawPages("alerts", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListAlerts(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			alerts = append(alerts, currentPage.Alerts...)
			hasMore := alertPageHasMore(currentPage)
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

// Validates flags, calls the SDK, and writes output
func runAlertTimeline(cmd *cobra.Command, flags alertTimelineFlags, common resolvedCommonFlags) error {
	if err := validateAlertTimelineFlags(flags, common); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	nodeUUID := strings.TrimSpace(flags.node)
	if nodeUUID != "" {
		return runNodeAlertTimeline(cmd, client, flags, nodeUUID, common)
	}
	return runAlertTimelineNodes(cmd, client, flags, common)
}

// Gets and renders the filters and sorting choices available for an alert timeline view.
func runAlertTimelineOptions(cmd *cobra.Command, flags alertTimelineOptionsFlags, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}
	options, err := client.GetAlertTimelineFilterOptions(cmd.Context(), flags.active)
	if err != nil {
		return err
	}
	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), options.RawJSON)
	}
	return writeAlertTimelineOptionsTable(cmd.OutOrStdout(), options)
}

// Lists nodes with alert timeline history
func runAlertTimelineNodes(cmd *cobra.Command, client *nvfleetint.Client, flags alertTimelineFlags, common resolvedCommonFlags) error {
	alertStates, err := parseAlertTimelineStateList(flags.alertState)
	if err != nil {
		return err
	}
	gpuTypes, err := clihelpers.ParseCommaList(flags.gpuType)
	if err != nil {
		return err
	}
	nodeGroupIDs, err := clihelpers.ParseCommaList(flags.nodeGroupIDs)
	if err != nil {
		return err
	}
	computeZoneIDs, err := clihelpers.ParseCommaList(flags.computeZoneIDs)
	if err != nil {
		return err
	}
	componentTypes, err := clihelpers.ParseCommaList(flags.componentType)
	if err != nil {
		return err
	}
	opts := nvfleetint.ListAlertTimelineNodesOptions{
		Active:         flags.active,
		Hostname:       strings.TrimSpace(flags.hostname),
		SortBy:         nvfleetint.AlertTimelineNodeSortBy(strings.TrimSpace(flags.sortBy)),
		Order:          nvfleetint.AlertTimelineSortOrder(strings.TrimSpace(flags.order)),
		GPUTypes:       gpuTypes,
		NodeGroupIDs:   nodeGroupIDs,
		ComputeZoneIDs: computeZoneIDs,
		AlertStates:    alertStates,
		ComponentTypes: componentTypes,
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var nodes []nvfleetint.AlertTimelineNode
		var aggregates nvfleetint.AlertTimelineNodesPage
		haveAggregates := false
		result, err := clihelpers.FetchAllRawPages("nodes", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListAlertTimelineNodes(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			nodes = append(nodes, currentPage.Nodes...)
			if !haveAggregates {
				aggregates = currentPage
				haveAggregates = true
			}
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
func runNodeAlertTimeline(cmd *cobra.Command, client *nvfleetint.Client, flags alertTimelineFlags, nodeUUID string, common resolvedCommonFlags) error {
	alertStates, err := parseAlertTimelineStateList(flags.alertState)
	if err != nil {
		return err
	}
	gpuTypes, err := clihelpers.ParseCommaList(flags.gpuType)
	if err != nil {
		return err
	}
	nodeGroupIDs, err := clihelpers.ParseCommaList(flags.nodeGroupIDs)
	if err != nil {
		return err
	}
	computeZoneIDs, err := clihelpers.ParseCommaList(flags.computeZoneIDs)
	if err != nil {
		return err
	}
	componentTypes, err := clihelpers.ParseCommaList(flags.componentType)
	if err != nil {
		return err
	}
	opts := nvfleetint.ListNodeAlertTimelineOptions{
		NodeUUID:       nodeUUID,
		Active:         flags.active,
		WithoutPSIRT:   flags.withoutPSIRT,
		SortBy:         nvfleetint.AlertTimelineAlertSortBy(strings.TrimSpace(flags.sortBy)),
		Order:          nvfleetint.AlertTimelineSortOrder(strings.TrimSpace(flags.order)),
		AlertStates:    alertStates,
		ComponentTypes: componentTypes,
		GPUTypes:       gpuTypes,
		NodeGroupIDs:   nodeGroupIDs,
		ComputeZoneIDs: computeZoneIDs,
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var alerts []nvfleetint.AlertTimelineNodeAlert
		result, err := clihelpers.FetchAllRawPages("alerts", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListNodeAlertTimeline(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			alerts = append(alerts, currentPage.Alerts...)
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

// Checks alert timeline flags
func validateAlertTimelineFlags(flags alertTimelineFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if _, err := parseAlertTimelineStateList(flags.alertState); err != nil {
		return err
	}
	for _, filter := range []struct {
		name string
		raw  string
	}{
		{name: "component-type", raw: flags.componentType},
		{name: "gpu-type", raw: flags.gpuType},
		{name: "nodegroup-ids", raw: flags.nodeGroupIDs},
		{name: "compute-zone-ids", raw: flags.computeZoneIDs},
	} {
		if _, err := clihelpers.ParseCommaList(filter.raw); err != nil {
			return fmt.Errorf("invalid --%s: %w", filter.name, err)
		}
	}
	if flags.order != "" && !nvfleetint.AlertTimelineSortOrder(strings.TrimSpace(flags.order)).Valid() {
		return fmt.Errorf("invalid order %q: expected asc or desc", flags.order)
	}
	nodeUUID := strings.TrimSpace(flags.node)
	if nodeUUID == "" {
		if flags.withoutPSIRT {
			return errors.New("--without-psirt requires --node")
		}
		sortBy := nvfleetint.AlertTimelineNodeSortBy(strings.TrimSpace(flags.sortBy))
		if sortBy != "" && !sortBy.Valid() {
			return fmt.Errorf("invalid sort-by %q for impacted nodes: expected hostname, alert, gpuType, nodeGroup, computeZone, or lastUpdate", flags.sortBy)
		}
		return nil
	}
	if strings.TrimSpace(flags.hostname) != "" {
		return errors.New("--hostname cannot be used with --node")
	}
	sortBy := nvfleetint.AlertTimelineAlertSortBy(strings.TrimSpace(flags.sortBy))
	if sortBy != "" && !sortBy.Valid() {
		return fmt.Errorf("invalid sort-by %q for node alerts: expected component, startTime, or lastUpdate", flags.sortBy)
	}
	return nil
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

// Converts comma-separated alert timeline states into API values
func parseAlertTimelineStateList(raw string) ([]nvfleetint.AlertTimelineState, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	states := make([]nvfleetint.AlertTimelineState, 0, len(values))
	for _, value := range values {
		state := nvfleetint.AlertTimelineState(value)
		if !state.Valid() {
			return nil, fmt.Errorf("invalid alert-state %q: expected Critical, Warning, or Resolved", value)
		}
		states = append(states, state)
	}
	return states, nil
}

// Converts a state flag into an API value
func parseAlertState(raw string) (nvfleetint.AlertState, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	state := nvfleetint.AlertState(raw)
	if !state.Valid() {
		return "", fmt.Errorf("invalid state %q: expected Detected, Triggered, or Resolved", raw)
	}
	return state, nil
}

// Converts a severity flag into an API value
func parseAlertSeverity(raw string) (nvfleetint.AlertSeverity, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	severity := nvfleetint.AlertSeverity(raw)
	if !severity.Valid() {
		return "", fmt.Errorf("invalid severity %q: expected Critical or Warning", raw)
	}
	return severity, nil
}

// Reports whether a list alert response has another page
func alertPageHasMore(page nvfleetint.AlertsPage) bool {
	if strings.TrimSpace(page.PageCursorNext) != "" {
		return true
	}
	if page.Page < 0 || page.PageSize <= 0 || page.Total <= 0 {
		return false
	}
	// Page is 0-indexed, so the first (page+1) pages have been seen so far.
	return (page.Page+1)*page.PageSize < page.Total
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

// Writes alert timeline filter values and sorting metadata as readable tables.
func writeAlertTimelineOptionsTable(w io.Writer, options nvfleetint.AlertTimelineFilterOptions) error {
	rows := make([][]string, 0)
	for _, field := range options.Filters.Fields {
		for _, option := range field.Options {
			rows = append(rows, []string{field.Name, option.ID, option.Value})
		}
	}
	if err := clioutput.WriteTable(w, []string{"FILTER", "ID", "VALUE"}, rows); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	return clioutput.WriteTable(w, []string{"SORTING", "VALUE"}, [][]string{
		{"FIELDS", strings.Join(options.Sorting.Fields, ", ")},
		{"ORDERS", strings.Join(options.Sorting.Orders, ", ")},
		{"DEFAULT FIELD", options.Sorting.Defaults.Field},
		{"DEFAULT ORDER", options.Sorting.Defaults.Order},
	})
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

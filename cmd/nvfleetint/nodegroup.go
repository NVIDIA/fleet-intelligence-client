// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Stores local flag values for nodegroup list
type nodeGroupListFlags struct {
	view             string
	includeMetrics   bool
	computeZoneIDs   string
	computeZoneNames string
	nodeGroupIDs     string
	health           string
	gpuType          string
	sortBy           string
	order            string
}

// Stores data ready for nodegroup list rendering
type nodeGroupListOutput struct {
	NodeGroups []nvfleetint.NodeGroup
	View       string
	JSONValue  any
	RawJSON    []byte
	Page       *clioutput.Pagination
}

// Creates the top-level nodegroup command group
func newNodeGroupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "nodegroup",
		Aliases: []string{"node-group"},
		Short:   "Inspect node groups",
	}

	cmd.AddCommand(newNodeGroupListCmd())
	cmd.AddCommand(newNodeGroupOptionsCmd())
	rejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the node group list command
func newNodeGroupListCmd() *cobra.Command {
	flags := nodeGroupListFlags{
		view:           string(nvfleetint.NodeGroupViewDetail),
		includeMetrics: true,
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List node groups",
		Long: "List node groups.\n\n" +
			optionsHelpNote("nvfleetint nodegroup options",
				"--compute-zone-ids", "--nodegroup-ids", "--health",
				"--gpu-type", "--sort-by", "--order"),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodeGroupList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "View mode: detail or basic")
	cmd.Flags().BoolVar(&flags.includeMetrics, "include-metrics", flags.includeMetrics, "Include metrics in detail view; use --include-metrics=false to omit")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.computeZoneNames, "compute-zone-names", "", "Comma-separated compute zone names to filter (partial match)")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.health, "health", "", "Comma-separated health states to filter")
	cmd.Flags().StringVar(&flags.gpuType, "gpu-type", "", "Comma-separated GPU types to filter")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field")
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order for --sort-by; node groups default --sort-by to health")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runNodeGroupList(cmd *cobra.Command, flags nodeGroupListFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	opts, err := nodeGroupListOptions(flags, cmd.Flags().Changed("include-metrics"))
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var groups []nvfleetint.NodeGroup
		result, err := clihelpers.FetchAllPages("nodeGroups",
			func(pageNumber int) (nvfleetint.NodeGroupsPage, error) {
				opts.Page = &pageNumber
				return client.ListNodeGroups(cmd.Context(), opts)
			},
			func(page nvfleetint.NodeGroupsPage) { groups = append(groups, page.NodeGroups...) },
		)
		if err != nil {
			return err
		}
		return writeNodeGroupListOutput(cmd.OutOrStdout(), common, nodeGroupListOutput{
			NodeGroups: groups,
			View:       flags.view,
			JSONValue:  result,
		})
	}

	page, err := client.ListNodeGroups(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeNodeGroupListOutput(cmd.OutOrStdout(), common, nodeGroupListOutput{
		NodeGroups: page.NodeGroups,
		View:       flags.view,
		RawJSON:    page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	})
}

// Names the flag carrying each node group list option, for rendering SDK
// validation errors against what the user typed.
var nodeGroupListFlagNames = map[string]optionFlagName{
	"view":   {flag: "view"},
	"health": {flag: "health"},
	"sortBy": {flag: "sort-by"},
	"order":  {flag: "order"},
}

// nodeGroupListOptions reads every node group list flag exactly once and hands
// the result to the SDK to validate. includeMetricsSet reports whether
// --include-metrics was given at all, which is the difference between asking
// for the backend default and asking for false.
func nodeGroupListOptions(flags nodeGroupListFlags, includeMetricsSet bool) (nvfleetint.ListNodeGroupsOptions, error) {
	opts := nvfleetint.ListNodeGroupsOptions{
		View:   nvfleetint.NodeGroupView(flags.view),
		SortBy: nvfleetint.NodeGroupSortBy(flags.sortBy),
		Order:  nvfleetint.NodeGroupSortOrder(flags.order),
	}
	if includeMetricsSet {
		opts.IncludeMetrics = &flags.includeMetrics
	}

	for _, list := range []struct {
		name string
		raw  string
		dest *[]string
	}{
		{name: "compute-zone-ids", raw: flags.computeZoneIDs, dest: &opts.ComputeZoneIDs},
		{name: "compute-zone-names", raw: flags.computeZoneNames, dest: &opts.ComputeZoneNames},
		{name: "nodegroup-ids", raw: flags.nodeGroupIDs, dest: &opts.NodeGroupIDs},
		{name: "gpu-type", raw: flags.gpuType, dest: &opts.GPUTypes},
	} {
		values, err := clihelpers.ParseCommaList(list.raw)
		if err != nil {
			return opts, fmt.Errorf("invalid %s: %w", list.name, err)
		}
		*list.dest = values
	}

	healthStatuses, err := clihelpers.ParseTypedList[nvfleetint.NodeGroupHealthStatus]("health", flags.health)
	if err != nil {
		return opts, err
	}
	opts.HealthStatuses = healthStatuses

	// The SDK enforces this too; naming the flag is the only reason it is
	// restated here, since the SDK sees a pointer rather than a flag.
	if opts.View == nvfleetint.NodeGroupViewBasic && includeMetricsSet {
		return opts, errors.New("basic node group view is incompatible with --include-metrics")
	}
	if err := opts.Validate(); err != nil {
		return opts, renderOptionError(err, nodeGroupListFlagNames)
	}

	return opts, nil
}

// Writes JSON or table output for node group list results
func writeNodeGroupListOutput(w io.Writer, common resolvedCommonFlags, result nodeGroupListOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	if err := writeNodeGroupTable(w, result.View, result.NodeGroups); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Renders node groups using the selected view columns
func writeNodeGroupTable(w io.Writer, view string, groups []nvfleetint.NodeGroup) error {
	if nvfleetint.NodeGroupView(view) == nvfleetint.NodeGroupViewBasic {
		return clioutput.WriteTable(w, []string{"ID", "NAME"}, basicNodeGroupRows(groups))
	}
	return clioutput.WriteTable(w, []string{"ID", "NAME", "COMPUTE ZONE", "HEALTH", "HEALTH PERCENTAGE", "NODE COUNT"}, detailNodeGroupRows(groups))
}

// Converts node groups into basic table rows
func basicNodeGroupRows(groups []nvfleetint.NodeGroup) [][]string {
	rows := make([][]string, 0, len(groups))
	for _, group := range groups {
		rows = append(rows, []string{clioutput.DisplayString(group.ID), clioutput.DisplayString(group.Name)})
	}
	return rows
}

// Converts node groups into detail table rows
func detailNodeGroupRows(groups []nvfleetint.NodeGroup) [][]string {
	rows := make([][]string, 0, len(groups))
	for _, group := range groups {
		rows = append(rows, []string{
			clioutput.DisplayString(group.ID),
			clioutput.DisplayString(group.Name),
			clioutput.FormatNameOrID(group.ComputeZoneName, group.ComputeZoneID),
			clioutput.DisplayString(group.Health),
			clioutput.FormatOptionalPercentage(group.HealthPercentage),
			clioutput.FormatOptionalInt(group.NodeCount),
		})
	}
	return rows
}

// Maps each filter field returned by the node group options endpoint to the
// flag on `nodegroup list` that consumes it. The endpoint is shared with
// nodes, so nested node group values under computeZones are promoted into the
// flag that accepts them.
var nodeGroupOptionsRenderer = optionsRenderer{
	consumers: []string{"nodegroup list"},
	filters: map[string]optionFlag{
		"computeZones":   {name: "--compute-zone-ids", promote: "--nodegroup-ids"},
		"nodeGroups":     {name: "--nodegroup-ids"},
		"gpuTypes":       {name: "--gpu-type"},
		"healthStatuses": {name: "--health"},
	},
	sortAccepted: func(field string) bool {
		return nvfleetint.NodeGroupSortBy(field).Valid()
	},
}

// Creates the node group options command
func newNodeGroupOptionsCmd() *cobra.Command {
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "options",
		Short: "List available node group filters and sorting options",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodeGroupOptions(cmd, resolveCommonFlags(cmd, common))
		},
	}

	registerReadCommonFlags(cmd, common)

	return cmd
}

// Gets and renders the filters and sorting choices available for node group queries.
func runNodeGroupOptions(cmd *cobra.Command, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}
	options, err := client.GetNodeGroupFilterOptions(cmd.Context())
	if err != nil {
		return err
	}
	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), options.RawJSON)
	}
	return nodeGroupOptionsRenderer.write(cmd.OutOrStdout(), options)
}

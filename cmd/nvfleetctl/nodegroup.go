// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"strings"

	clihelpers "github.com/NVIDIA/fleet-intelligence-client/cmd/nvfleetctl/helpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"

	"github.com/spf13/cobra"
)

// Stores local flag values for nodegroup list
type nodeGroupListFlags struct {
	view         string
	nodeGroupIDs string
	health       string
	gpuType      string
	sortBy       string
	order        string
}

// Stores data ready for nodegroup list rendering
type nodeGroupListOutput struct {
	NodeGroups []fleetintelligence.NodeGroup
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

	return cmd
}

// Creates the node group list command
func newNodeGroupListCmd() *cobra.Command {
	flags := nodeGroupListFlags{
		view: string(fleetintelligence.NodeGroupViewDetail),
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List node groups",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodeGroupList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "View mode: detail or basic")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.health, "health", "", "Comma-separated health states to filter: Healthy, Degraded, Unhealthy, or Unknown")
	cmd.Flags().StringVar(&flags.gpuType, "gpu-type", "", "Comma-separated GPU types to filter")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field: health or nodes")
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order for --sort-by: asc or desc; node groups default --sort-by to health")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runNodeGroupList(cmd *cobra.Command, flags nodeGroupListFlags, common resolvedCommonFlags) error {
	if err := validateNodeGroupListFlags(flags, common); err != nil {
		return err
	}

	nodeGroupIDs, err := clihelpers.ParseCommaList(flags.nodeGroupIDs)
	if err != nil {
		return err
	}
	healthStatuses, err := parseNodeGroupHealthList(flags.health)
	if err != nil {
		return err
	}
	gpuTypes, err := clihelpers.ParseCommaList(flags.gpuType)
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	opts := fleetintelligence.ListNodeGroupsOptions{
		View:         fleetintelligence.NodeGroupView(flags.view),
		NodeGroupIDs: nodeGroupIDs,
		SortBy:       fleetintelligence.NodeGroupSortBy(flags.sortBy),
		Order:        fleetintelligence.NodeGroupSortOrder(flags.order),
	}
	if fleetintelligence.NodeGroupView(flags.view) == fleetintelligence.NodeGroupViewDetail {
		opts.HealthStatuses = healthStatuses
		opts.GPUTypes = gpuTypes
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var groups []fleetintelligence.NodeGroup
		result, err := clihelpers.FetchAllRawPages("nodeGroups", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListNodeGroups(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			groups = append(groups, currentPage.NodeGroups...)
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

// Checks node group list flags
func validateNodeGroupListFlags(flags nodeGroupListFlags, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if !fleetintelligence.NodeGroupView(flags.view).Valid() {
		return fmt.Errorf("invalid view %q: expected basic or detail", flags.view)
	}
	if _, err := parseNodeGroupHealthList(flags.health); err != nil {
		return err
	}
	if flags.sortBy != "" && !fleetintelligence.NodeGroupSortBy(flags.sortBy).Valid() {
		return fmt.Errorf("invalid sort-by %q: expected health or nodes", flags.sortBy)
	}
	if flags.order != "" && !fleetintelligence.NodeGroupSortOrder(flags.order).Valid() {
		return fmt.Errorf("invalid order %q: expected asc or desc", flags.order)
	}
	if fleetintelligence.NodeGroupView(flags.view) == fleetintelligence.NodeGroupViewBasic {
		if strings.TrimSpace(flags.health) != "" || strings.TrimSpace(flags.gpuType) != "" {
			return errors.New("basic node group view is incompatible with health and gpu-type filters")
		}
		if strings.TrimSpace(flags.sortBy) != "" {
			return fmt.Errorf("basic node group view is incompatible with sort %q", flags.sortBy)
		}
	}
	return nil
}

// Converts comma-separated health filters into API values
func parseNodeGroupHealthList(raw string) ([]fleetintelligence.NodeGroupHealthStatus, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	statuses := make([]fleetintelligence.NodeGroupHealthStatus, 0, len(values))
	for _, value := range values {
		status := fleetintelligence.NodeGroupHealthStatus(value)
		if !status.Valid() {
			return nil, fmt.Errorf("invalid health %q: expected Healthy, Degraded, Unhealthy, or Unknown", value)
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
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
func writeNodeGroupTable(w io.Writer, view string, groups []fleetintelligence.NodeGroup) error {
	if fleetintelligence.NodeGroupView(view) == fleetintelligence.NodeGroupViewBasic {
		return clioutput.WriteTable(w, []string{"ID", "NAME"}, basicNodeGroupRows(groups))
	}
	return clioutput.WriteTable(w, []string{"ID", "NAME", "COMPUTE ZONE", "HEALTH", "HEALTH PERCENTAGE", "NODE COUNT"}, detailNodeGroupRows(groups))
}

// Converts node groups into basic table rows
func basicNodeGroupRows(groups []fleetintelligence.NodeGroup) [][]string {
	rows := make([][]string, 0, len(groups))
	for _, group := range groups {
		rows = append(rows, []string{clioutput.DisplayString(group.ID), clioutput.DisplayString(group.Name)})
	}
	return rows
}

// Converts node groups into detail table rows
func detailNodeGroupRows(groups []fleetintelligence.NodeGroup) [][]string {
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

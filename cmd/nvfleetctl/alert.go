// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// Stores local flag values for alert list
type alertListFlags struct {
	node      string
	component string
	state     string
	severity  string
}

// Stores local flag values for alert timeline
type alertTimelineFlags struct {
	active bool
	node   string
}

// Stores local flag values for alert describe
type alertDescribeFlags struct {
	node string
}

// Stores data ready for alert list rendering
type alertListOutput struct {
	Alerts    []fleetintelligence.Alert
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

// Stores data ready for alert timeline rendering
type alertTimelineOutput struct {
	Nodes     []fleetintelligence.AlertTimelineNode
	Alerts    []fleetintelligence.AlertTimelineNodeAlert
	Mode      string
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
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
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the alert describe command
func newAlertDescribeCmd() *cobra.Command {
	flags := alertDescribeFlags{}
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "describe <alertUUID>",
		Short: "Describe an alert timeline",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAlertDescribe(cmd, args[0], flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.node, "node", "", "Node UUID for the alert")
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

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	opts := fleetintelligence.ListAlertsOptions{
		NodeUUID:  strings.TrimSpace(flags.node),
		Component: strings.TrimSpace(flags.component),
		State:     state,
		Severity:  severity,
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var alerts []fleetintelligence.Alert
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
	if err := validateAlertTimelineFlags(common); err != nil {
		return err
	}

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	nodeUUID := strings.TrimSpace(flags.node)
	if nodeUUID != "" {
		return runNodeAlertTimeline(cmd, client, flags, nodeUUID, common)
	}
	return runAlertTimelineNodes(cmd, client, flags, common)
}

// Lists nodes with alert timeline history
func runAlertTimelineNodes(cmd *cobra.Command, client *fleetintelligence.Client, flags alertTimelineFlags, common resolvedCommonFlags) error {
	opts := fleetintelligence.ListAlertTimelineNodesOptions{
		Active: flags.active,
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var nodes []fleetintelligence.AlertTimelineNode
		result, err := clihelpers.FetchAllRawPages("nodes", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListAlertTimelineNodes(cmd.Context(), opts)
			if err != nil {
				return clihelpers.RawPage{}, err
			}
			nodes = append(nodes, currentPage.Nodes...)
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
			Nodes:     nodes,
			Mode:      alertTimelineModeNodes,
			JSONValue: result,
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
func runNodeAlertTimeline(cmd *cobra.Command, client *fleetintelligence.Client, flags alertTimelineFlags, nodeUUID string, common resolvedCommonFlags) error {
	opts := fleetintelligence.ListNodeAlertTimelineOptions{
		NodeUUID: nodeUUID,
		Active:   flags.active,
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var alerts []fleetintelligence.AlertTimelineNodeAlert
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
	if err := validateReadCommonFlags(common); err != nil {
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

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	details, err := client.DescribeAlertTimeline(cmd.Context(), nodeUUID, alertUUID)
	if err != nil {
		return err
	}
	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), details.RawJSON)
	}
	return writeAlertDescribeTable(cmd.OutOrStdout(), details)
}

// Checks alert timeline flags
func validateAlertTimelineFlags(common resolvedCommonFlags) error {
	return validateListCommonFlags(common)
}

// Converts a state flag into an API value
func parseAlertState(raw string) (fleetintelligence.AlertState, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	state := fleetintelligence.AlertState(raw)
	if !state.Valid() {
		return "", fmt.Errorf("invalid state %q: expected Detected, Triggered, or Resolved", raw)
	}
	return state, nil
}

// Converts a severity flag into an API value
func parseAlertSeverity(raw string) (fleetintelligence.AlertSeverity, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	severity := fleetintelligence.AlertSeverity(raw)
	if !severity.Valid() {
		return "", fmt.Errorf("invalid severity %q: expected Critical or Warning", raw)
	}
	return severity, nil
}

// Reports whether a list alert response has another page
func alertPageHasMore(page fleetintelligence.AlertsPage) bool {
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
		err = clioutput.WriteTable(w, []string{"ALERT UUID", "COMPONENT", "STATUS", "LAST EVENT TIME"}, alertTimelineAlertRows(result.Alerts))
	} else {
		err = clioutput.WriteTable(w, []string{"NODE UUID", "HOSTNAME", "NODE STATUS", "LAST ALERT TIME"}, alertTimelineNodeRows(result.Nodes))
	}
	if err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Renders alert timeline events as a table
func writeAlertDescribeTable(w io.Writer, details fleetintelligence.AlertTimelineDetails) error {
	rows := make([][]string, 0, len(details.Timeline))
	for _, event := range details.Timeline {
		rows = append(rows, []string{
			clioutput.DisplayString(event.EventTimestamp),
			clioutput.DisplayString(event.EventType),
			clioutput.DisplayString(event.AlertStatus),
			clioutput.DisplayString(event.Message),
			clioutput.DisplayString(event.Error),
		})
	}
	return clioutput.WriteTable(w, []string{"TIMESTAMP", "EVENT", "STATUS", "MESSAGE", "ERROR"}, rows)
}

// Converts alerts into table rows
func alertRows(alerts []fleetintelligence.Alert) [][]string {
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
func alertTimelineNodeRows(nodes []fleetintelligence.AlertTimelineNode) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.NodeUUID),
			clioutput.DisplayString(node.Hostname),
			clioutput.DisplayString(node.HostStatus),
			clioutput.DisplayString(node.LastAlertTime),
		})
	}
	return rows
}

// Converts node alert timeline entries into table rows
func alertTimelineAlertRows(alerts []fleetintelligence.AlertTimelineNodeAlert) [][]string {
	rows := make([][]string, 0, len(alerts))
	for _, alert := range alerts {
		rows = append(rows, []string{
			clioutput.DisplayString(alert.AlertUUID),
			clioutput.DisplayString(alert.Component),
			clioutput.DisplayString(alert.AlertStatus),
			clioutput.DisplayString(alert.LastEventTime),
		})
	}
	return rows
}

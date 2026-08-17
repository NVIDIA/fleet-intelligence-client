// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// User-facing spelling of the backend "integrityCheck" node sort field
const nodeSortByVerificationCheck = "verificationCheck"

// Lists the sort fields accepted by node list, using the user-facing
// "verificationCheck" spelling instead of the backend "integrityCheck"
const nodeSortByList = "hostname, nodeUUID, healthStatus, nodegroup, computezone, gpuType, gpuCount, " +
	nodeSortByVerificationCheck + ", agentStatus, agentVersion, kernelVersion, gpuDriverVersion, or gpuFirmwareVersions"

// Stores local flag values for node list
type nodeListFlags struct {
	view             string
	agentType        string
	nodeUUIDs        string
	health           string
	computeZoneIDs   string
	computeZoneNames string
	nodeGroupIDs     string
	nodeGroupNames   string
	gpuType          string
	gpuCount         string
	publicIP         string
	privateIP        string
	hostname         string
	bmcHostname      string
	agentStatus      string
	integrityCheck   string
	firmwareCheck    string
	sortBy           string
	order            string
}

// Stores data ready for node list rendering
type nodeListOutput struct {
	Nodes     []nvfleetint.Node
	View      string
	AgentType nvfleetint.NodeAgentType
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
}

type nodeListResult struct {
	Inband *nodeListOutput
	OOB    *nodeListOutput
}

type combinedNodeListJSON struct {
	Inband any `json:"inband"`
	OOB    any `json:"oob"`
}

type nodeDescribeFlags struct {
	agentType string
	sections  string
}

type nodeDescribeRenderOptions struct {
	showSummary bool
	sections    map[oobInventorySection]bool
}

type nodeDescribeResult struct {
	Inband *nvfleetint.NodeDetails
	OOB    *nvfleetint.NodeDetails
}

type combinedNodeDescribeJSON struct {
	Inband json.RawMessage `json:"inband,omitempty"`
	OOB    json.RawMessage `json:"oob,omitempty"`
}

type oobInventorySection string

const (
	oobInventorySectionManagers oobInventorySection = "managers"
	oobInventorySectionSystems  oobInventorySection = "systems"
	oobInventorySectionChassis  oobInventorySection = "chassis"
	oobInventorySectionFirmware oobInventorySection = "firmware"
)

var oobInventorySectionOrder = []oobInventorySection{
	oobInventorySectionManagers,
	oobInventorySectionSystems,
	oobInventorySectionChassis,
	oobInventorySectionFirmware,
}

// Creates the top-level node command group
func newNodeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Inspect nodes",
	}

	cmd.AddCommand(newNodeListCmd())
	cmd.AddCommand(newNodeDescribeCmd())
	cmd.AddCommand(newNodeHealthCmd())
	rejectUnknownSubcommands(cmd)

	return cmd
}

// Creates the node list command
func newNodeListCmd() *cobra.Command {
	flags := nodeListFlags{
		view: string(nvfleetint.NodeViewDetail),
	}
	common := newCommonFlags()

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List nodes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodeList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "View mode: detail or basic")
	cmd.Flags().StringVar(
		&flags.agentType,
		"agent-type",
		flags.agentType,
		"Agent type view: inband or oob (detail view defaults to both)",
	)
	cmd.Flags().StringVar(&flags.nodeUUIDs, "node-uuids", "", "Comma-separated node UUIDs to filter")
	cmd.Flags().StringVar(&flags.health, "health", "", "Comma-separated health states to filter: Healthy, Degraded, Unhealthy, or Unknown")
	cmd.Flags().StringVar(&flags.computeZoneIDs, "compute-zone-ids", "", "Comma-separated compute zone IDs to filter")
	cmd.Flags().StringVar(&flags.computeZoneNames, "compute-zone-names", "", "Comma-separated compute zone names to filter (partial match)")
	cmd.Flags().StringVar(&flags.nodeGroupIDs, "nodegroup-ids", "", "Comma-separated node group IDs to filter")
	cmd.Flags().StringVar(&flags.nodeGroupNames, "nodegroup-names", "", "Comma-separated node group names to filter (partial match)")
	cmd.Flags().StringVar(&flags.gpuType, "gpu-type", "", "Comma-separated GPU types to filter")
	cmd.Flags().StringVar(&flags.gpuCount, "gpu-count", "", "Comma-separated GPU counts to filter")
	cmd.Flags().StringVar(&flags.publicIP, "public-ip", "", "Comma-separated public IP addresses to filter")
	cmd.Flags().StringVar(&flags.privateIP, "private-ip", "", "Comma-separated private IP addresses to filter")
	cmd.Flags().StringVar(&flags.hostname, "hostname", "", "Hostname partial match")
	cmd.Flags().StringVar(&flags.bmcHostname, "bmc-hostname", "", "BMC hostname partial match (OOB view)")
	cmd.Flags().StringVar(&flags.agentStatus, "agent-status", "", "Comma-separated agent statuses to filter: Online, Offline, or Unknown")
	// User-facing "verification check" maps to the backend "integrity check" API field.
	cmd.Flags().StringVar(&flags.integrityCheck, "verification-check", "", "Comma-separated verification check statuses to filter: Verified, Unverified, Degraded, Pending, Unsupported, or Unknown")
	cmd.Flags().StringVar(&flags.firmwareCheck, "firmware-check", "", "Comma-separated firmware check statuses to filter: Passed, Failed, or Unknown")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field: "+nodeSortByList)
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order: asc or desc")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the node describe command
func newNodeDescribeCmd() *cobra.Command {
	common := newCommonFlags()
	flags := nodeDescribeFlags{}
	cmd := &cobra.Command{
		Use:   "describe <uuid>",
		Short: "Describe a node",
		Args:  requireSingleArg("node UUID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodeDescribe(cmd, args[0], flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(
		&flags.agentType,
		"agent-type",
		flags.agentType,
		"Agent type view: inband or oob (default: both)",
	)
	cmd.Flags().StringVar(
		&flags.sections,
		"section",
		"",
		"Comma-separated OOB inventory sections: managers, systems, chassis, firmware, or all",
	)
	registerReadCommonFlags(cmd, common)

	return cmd
}

// Validates flags, calls the SDK, and writes output
func runNodeList(cmd *cobra.Command, flags nodeListFlags, common resolvedCommonFlags) error {
	sortBy, err := normalizeNodeSortBy(flags.sortBy)
	if err != nil {
		return err
	}
	if err := validateNodeListFlags(flags, sortBy, common); err != nil {
		return err
	}

	nodeUUIDs, err := clihelpers.ParseCommaList(flags.nodeUUIDs)
	if err != nil {
		return err
	}
	healthStatuses, err := parseNodeHealthList(flags.health)
	if err != nil {
		return err
	}
	computeZoneIDs, err := clihelpers.ParseCommaList(flags.computeZoneIDs)
	if err != nil {
		return err
	}
	computeZoneNames, err := clihelpers.ParseCommaList(flags.computeZoneNames)
	if err != nil {
		return err
	}
	nodeGroupIDs, err := clihelpers.ParseCommaList(flags.nodeGroupIDs)
	if err != nil {
		return err
	}
	nodeGroupNames, err := clihelpers.ParseCommaList(flags.nodeGroupNames)
	if err != nil {
		return err
	}
	gpuTypes, err := clihelpers.ParseCommaList(flags.gpuType)
	if err != nil {
		return err
	}
	gpuCounts, err := parseNodeGPUCountList(flags.gpuCount)
	if err != nil {
		return err
	}
	publicIPs, err := clihelpers.ParseCommaList(flags.publicIP)
	if err != nil {
		return err
	}
	privateIPs, err := clihelpers.ParseCommaList(flags.privateIP)
	if err != nil {
		return err
	}
	agentStatuses, err := parseNodeAgentStatusList(flags.agentStatus)
	if err != nil {
		return err
	}
	integrityChecks, err := parseNodeIntegrityCheckList(flags.integrityCheck)
	if err != nil {
		return err
	}
	firmwareChecks, err := parseNodeFirmwareCheckList(flags.firmwareCheck)
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	opts := nvfleetint.ListNodesOptions{
		View:             nvfleetint.NodeView(flags.view),
		AgentType:        nvfleetint.NodeAgentType(flags.agentType),
		NodeUUIDs:        nodeUUIDs,
		HealthStatuses:   healthStatuses,
		ComputeZoneIDs:   computeZoneIDs,
		ComputeZoneNames: computeZoneNames,
		NodeGroupIDs:     nodeGroupIDs,
		NodeGroupNames:   nodeGroupNames,
		GPUTypes:         gpuTypes,
		GPUCounts:        gpuCounts,
		PublicIPs:        publicIPs,
		PrivateIPs:       privateIPs,
		Hostname:         strings.TrimSpace(flags.hostname),
		BMCHostname:      strings.TrimSpace(flags.bmcHostname),
		AgentStatuses:    agentStatuses,
		IntegrityChecks:  integrityChecks,
		FirmwareChecks:   firmwareChecks,
		SortBy:           sortBy,
		Order:            nvfleetint.NodeSortOrder(flags.order),
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })
	agentType := nvfleetint.NodeAgentType(flags.agentType)
	if nvfleetint.NodeView(flags.view) == nvfleetint.NodeViewDetail && agentType == "" {
		result, err := listNodeViews(cmd.Context(), client, opts, common)
		if err != nil {
			return err
		}
		return writeNodeListResult(cmd.OutOrStdout(), common, result)
	}

	result, err := fetchNodeList(cmd.Context(), client, opts, common)
	if err != nil {
		return err
	}
	return writeNodeListOutput(cmd.OutOrStdout(), common, result)
}

func listNodeViews(
	ctx context.Context,
	client *nvfleetint.Client,
	opts nvfleetint.ListNodesOptions,
	common resolvedCommonFlags,
) (nodeListResult, error) {
	var result nodeListResult
	var inbandOutput nodeListOutput
	var oobOutput nodeListOutput
	var inbandErr error
	var oobErr error
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	go func() {
		defer waitGroup.Done()
		inbandOptions := opts
		inbandOptions.AgentType = nvfleetint.NodeAgentTypeInband
		inbandOutput, inbandErr = fetchNodeList(ctx, client, inbandOptions, common)
	}()
	go func() {
		defer waitGroup.Done()
		oobOptions := opts
		oobOptions.AgentType = nvfleetint.NodeAgentTypeOOB
		oobOutput, oobErr = fetchNodeList(ctx, client, oobOptions, common)
	}()
	waitGroup.Wait()

	if inbandErr != nil {
		return nodeListResult{}, fmt.Errorf("fetch in-band node list: %w", inbandErr)
	}
	if oobErr != nil {
		return nodeListResult{}, fmt.Errorf("fetch OOB node list: %w", oobErr)
	}
	result.Inband = &inbandOutput
	result.OOB = &oobOutput
	return result, nil
}

func fetchNodeList(
	ctx context.Context,
	client *nvfleetint.Client,
	opts nvfleetint.ListNodesOptions,
	common resolvedCommonFlags,
) (nodeListOutput, error) {
	if common.all {
		var nodes []nvfleetint.Node
		result, err := clihelpers.FetchAllRawPages("nodes", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListNodes(ctx, opts)
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
			return nodeListOutput{}, err
		}
		return nodeListOutput{
			Nodes:     nodes,
			View:      string(opts.View),
			AgentType: opts.AgentType,
			JSONValue: result,
		}, nil
	}

	page, err := client.ListNodes(ctx, opts)
	if err != nil {
		return nodeListOutput{}, err
	}
	return nodeListOutput{
		Nodes:     page.Nodes,
		View:      string(opts.View),
		AgentType: opts.AgentType,
		RawJSON:   page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	}, nil
}

// Validates args, calls the SDK, and writes output
func runNodeDescribe(
	cmd *cobra.Command,
	nodeUUID string,
	flags nodeDescribeFlags,
	common resolvedCommonFlags,
) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}

	nodeUUID = strings.TrimSpace(nodeUUID)
	if nodeUUID == "" {
		return errors.New("node UUID is required")
	}
	agentType := nvfleetint.NodeAgentType(flags.agentType)
	if agentType != "" && !agentType.Valid() {
		return fmt.Errorf("invalid agent-type %q: expected inband or oob", agentType)
	}
	renderOptions, err := parseOOBInventorySections(flags, agentType, common.output)
	if err != nil {
		return err
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	result, err := describeNodeResult(cmd.Context(), client, nodeUUID, agentType)
	if err != nil {
		return err
	}

	if common.output == clioutput.FormatJSON {
		return writeNodeDescribeJSON(cmd.OutOrStdout(), result, agentType)
	}
	if !renderOptions.showSummary && (result.OOB == nil || result.OOB.OOBInventory == nil) {
		return fmt.Errorf("node %q does not have an OOB view for the requested section", nodeUUID)
	}
	return writeNodeDescribeResultTable(cmd.OutOrStdout(), result, agentType, renderOptions)
}

func describeNodeResult(
	ctx context.Context,
	client *nvfleetint.Client,
	nodeUUID string,
	agentType nvfleetint.NodeAgentType,
) (nodeDescribeResult, error) {
	if agentType != "" {
		node, err := client.DescribeNodeWithOptions(ctx, nodeUUID, nvfleetint.DescribeNodeOptions{
			AgentType: agentType,
		})
		if err != nil {
			return nodeDescribeResult{}, err
		}
		if agentType == nvfleetint.NodeAgentTypeOOB {
			return nodeDescribeResult{OOB: &node}, nil
		}
		return nodeDescribeResult{Inband: &node}, nil
	}

	var result nodeDescribeResult
	var inbandErr error
	var oobErr error
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		node, err := client.DescribeNodeWithOptions(ctx, nodeUUID, nvfleetint.DescribeNodeOptions{
			AgentType: nvfleetint.NodeAgentTypeInband,
		})
		if err != nil {
			inbandErr = err
			return
		}
		result.Inband = &node
	}()
	go func() {
		defer waitGroup.Done()
		node, err := client.DescribeNodeWithOptions(ctx, nodeUUID, nvfleetint.DescribeNodeOptions{
			AgentType: nvfleetint.NodeAgentTypeOOB,
		})
		if err != nil {
			oobErr = err
			return
		}
		result.OOB = &node
	}()
	waitGroup.Wait()

	inbandMissing := apiErrorHasStatus(inbandErr, http.StatusNotFound)
	oobMissing := apiErrorHasStatus(oobErr, http.StatusNotFound)
	switch {
	case inbandErr != nil && !inbandMissing:
		return nodeDescribeResult{}, fmt.Errorf("fetch in-band node view: %w", inbandErr)
	case oobErr != nil && !oobMissing:
		return nodeDescribeResult{}, fmt.Errorf("fetch OOB node view: %w", oobErr)
	case inbandMissing && oobMissing:
		return nodeDescribeResult{}, fmt.Errorf("node %q was not found in either the in-band or OOB view", nodeUUID)
	default:
		return result, nil
	}
}

func apiErrorHasStatus(err error, statusCode int) bool {
	var apiErr *nvfleetint.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == statusCode
}

func writeNodeDescribeJSON(
	w io.Writer,
	result nodeDescribeResult,
	agentType nvfleetint.NodeAgentType,
) error {
	if agentType == nvfleetint.NodeAgentTypeInband && result.Inband != nil {
		return clioutput.WriteRawJSON(w, result.Inband.RawJSON)
	}
	if agentType == nvfleetint.NodeAgentTypeOOB && result.OOB != nil {
		return clioutput.WriteRawJSON(w, result.OOB.RawJSON)
	}

	output := combinedNodeDescribeJSON{}
	if result.Inband != nil {
		output.Inband = json.RawMessage(result.Inband.RawJSON)
	}
	if result.OOB != nil {
		output.OOB = json.RawMessage(result.OOB.RawJSON)
	}
	return clioutput.WriteJSON(w, output)
}

func parseOOBInventorySections(
	flags nodeDescribeFlags,
	agentType nvfleetint.NodeAgentType,
	output string,
) (nodeDescribeRenderOptions, error) {
	values, err := clihelpers.ParseCommaList(flags.sections)
	if err != nil {
		return nodeDescribeRenderOptions{}, err
	}
	showAll := false
	for _, value := range values {
		if strings.EqualFold(value, "all") {
			showAll = true
		}
	}
	if showAll && len(values) != 1 {
		return nodeDescribeRenderOptions{}, errors.New("section all cannot be combined with other sections")
	}
	if len(values) > 0 &&
		agentType != "" &&
		agentType != nvfleetint.NodeAgentTypeOOB {
		return nodeDescribeRenderOptions{}, errors.New("--section requires the OOB view")
	}
	if len(values) > 0 && output == clioutput.FormatJSON {
		return nodeDescribeRenderOptions{}, errors.New(
			"--section cannot be used with --output json; JSON already includes the full inventory",
		)
	}

	options := nodeDescribeRenderOptions{
		showSummary: len(values) == 0 || showAll,
		sections:    make(map[oobInventorySection]bool, len(oobInventorySectionOrder)),
	}
	if showAll {
		for _, section := range oobInventorySectionOrder {
			options.sections[section] = true
		}
		return options, nil
	}
	for _, value := range values {
		section := oobInventorySection(strings.ToLower(value))
		switch section {
		case oobInventorySectionManagers,
			oobInventorySectionSystems,
			oobInventorySectionChassis,
			oobInventorySectionFirmware:
			options.sections[section] = true
		default:
			return nodeDescribeRenderOptions{}, fmt.Errorf(
				"invalid OOB inventory section %q: expected managers, systems, chassis, or firmware",
				value,
			)
		}
	}
	return options, nil
}

// Checks node list flags
func validateNodeListFlags(flags nodeListFlags, sortBy nvfleetint.NodeSortBy, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if !nvfleetint.NodeView(flags.view).Valid() {
		return fmt.Errorf("invalid view %q: expected basic or detail", flags.view)
	}
	if flags.agentType != "" && !nvfleetint.NodeAgentType(flags.agentType).Valid() {
		return fmt.Errorf("invalid agent-type %q: expected inband or oob", flags.agentType)
	}
	if _, err := parseNodeHealthList(flags.health); err != nil {
		return err
	}
	if _, err := parseNodeAgentStatusList(flags.agentStatus); err != nil {
		return err
	}
	if _, err := parseNodeIntegrityCheckList(flags.integrityCheck); err != nil {
		return err
	}
	if _, err := parseNodeFirmwareCheckList(flags.firmwareCheck); err != nil {
		return err
	}
	if _, err := parseNodeGPUCountList(flags.gpuCount); err != nil {
		return err
	}
	if sortBy != "" && !sortBy.Valid() {
		return fmt.Errorf("invalid sort-by %q: expected %s", flags.sortBy, nodeSortByList)
	}
	if flags.order != "" && !nvfleetint.NodeSortOrder(flags.order).Valid() {
		return fmt.Errorf("invalid order %q: expected asc or desc", flags.order)
	}
	if nvfleetint.NodeView(flags.view) == nvfleetint.NodeViewBasic {
		if strings.TrimSpace(flags.health) != "" || strings.TrimSpace(flags.agentStatus) != "" || strings.TrimSpace(flags.integrityCheck) != "" || strings.TrimSpace(flags.firmwareCheck) != "" {
			return errors.New("basic node view is incompatible with health, agent-status, verification-check, and firmware-check filters")
		}
		if sortBy != "" && !basicNodeSortCompatible(sortBy) {
			return fmt.Errorf("basic node view is incompatible with sort %q", flags.sortBy)
		}
	}
	return nil
}

// Reports whether a sort field works with basic view
func basicNodeSortCompatible(sortBy nvfleetint.NodeSortBy) bool {
	switch sortBy {
	case nvfleetint.NodeSortByHostname, nvfleetint.NodeSortByUUID, nvfleetint.NodeSortByBMCHostname:
		return true
	default:
		return false
	}
}

// Normalizes the raw sort-by flag into an API sort field.
// "verificationCheck" is the user-facing name for the backend "integrityCheck"
// sort field; the backend name stays accepted so existing scripts keep working.
func normalizeNodeSortBy(raw string) (nvfleetint.NodeSortBy, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == nodeSortByVerificationCheck {
		return nvfleetint.NodeSortByIntegrityCheck, nil
	}

	return nvfleetint.NodeSortBy(trimmed), nil
}

// Converts comma-separated health filters into API values
func parseNodeHealthList(raw string) ([]nvfleetint.NodeHealthStatus, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	statuses := make([]nvfleetint.NodeHealthStatus, 0, len(values))
	for _, value := range values {
		status := nvfleetint.NodeHealthStatus(value)
		if !status.Valid() {
			return nil, fmt.Errorf("invalid health %q: expected Healthy, Degraded, Unhealthy, or Unknown", value)
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// Converts comma-separated agent filters into API values
func parseNodeAgentStatusList(raw string) ([]nvfleetint.NodeAgentStatus, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	statuses := make([]nvfleetint.NodeAgentStatus, 0, len(values))
	for _, value := range values {
		status := nvfleetint.NodeAgentStatus(value)
		if !status.Valid() {
			return nil, fmt.Errorf("invalid agent-status %q: expected Online, Offline, or Unknown", value)
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// Converts comma-separated verification filters into API values.
// Verification check is the user-facing name for the backend integrity check.
func parseNodeIntegrityCheckList(raw string) ([]nvfleetint.NodeIntegrityCheck, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	checks := make([]nvfleetint.NodeIntegrityCheck, 0, len(values))
	for _, value := range values {
		check := nvfleetint.NodeIntegrityCheck(value)
		if !check.Valid() {
			return nil, fmt.Errorf("invalid verification-check %q: expected Verified, Unverified, Degraded, Pending, Unsupported, or Unknown", value)
		}
		checks = append(checks, check)
	}

	return checks, nil
}

// Converts comma-separated GPU count filters into API values
func parseNodeGPUCountList(raw string) ([]int, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	counts := make([]int, 0, len(values))
	for _, value := range values {
		count, err := strconv.Atoi(value)
		if err != nil {
			return nil, fmt.Errorf("invalid gpu-count %q: expected an integer", value)
		}
		if count < 0 {
			return nil, fmt.Errorf("invalid gpu-count %q: expected a non-negative integer", value)
		}
		counts = append(counts, count)
	}

	return counts, nil
}

// Converts comma-separated firmware filters into API values
func parseNodeFirmwareCheckList(raw string) ([]nvfleetint.NodeFirmwareCheck, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	checks := make([]nvfleetint.NodeFirmwareCheck, 0, len(values))
	for _, value := range values {
		check := nvfleetint.NodeFirmwareCheck(value)
		if !check.Valid() {
			return nil, fmt.Errorf("invalid firmware-check %q: expected Passed, Failed, or Unknown", value)
		}
		checks = append(checks, check)
	}

	return checks, nil
}

// Writes JSON or table output for node list results
func writeNodeListOutput(w io.Writer, common resolvedCommonFlags, result nodeListOutput) error {
	if common.output == clioutput.FormatJSON {
		return writePaginatedListJSON(w, result.RawJSON, result.JSONValue)
	}

	if err := writeNodeTable(w, result.View, result.AgentType, result.Nodes); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

func writeNodeListResult(w io.Writer, common resolvedCommonFlags, result nodeListResult) error {
	if common.output == clioutput.FormatJSON {
		return clioutput.WriteJSON(w, combinedNodeListJSON{
			Inband: nodeListJSONValue(result.Inband),
			OOB:    nodeListJSONValue(result.OOB),
		})
	}

	if result.Inband != nil {
		if err := writeNodeListSection(w, "In-band", *result.Inband, false); err != nil {
			return err
		}
	}
	if result.OOB != nil {
		if err := writeNodeListSection(w, "Out-of-band", *result.OOB, result.Inband != nil); err != nil {
			return err
		}
	}
	return nil
}

func writeNodeListSection(w io.Writer, title string, result nodeListOutput, leadingNewline bool) error {
	if leadingNewline {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	if err := writeNodeTable(w, result.View, result.AgentType, result.Nodes); err != nil {
		return err
	}
	if result.Page != nil {
		return clioutput.WritePaginationFooter(w, *result.Page)
	}
	return nil
}

func nodeListJSONValue(result *nodeListOutput) any {
	if result == nil {
		return nil
	}
	if result.RawJSON != nil {
		return json.RawMessage(clihelpers.OneIndexRawPage(result.RawJSON))
	}
	if merged, ok := result.JSONValue.(clihelpers.MergedJSONResult); ok {
		merged.Pagination.Page++
		return merged
	}
	return result.JSONValue
}

// Renders nodes using the selected view columns
func writeNodeTable(
	w io.Writer,
	view string,
	agentType nvfleetint.NodeAgentType,
	nodes []nvfleetint.Node,
) error {
	if nvfleetint.NodeView(view) == nvfleetint.NodeViewBasic {
		return clioutput.WriteTable(
			w,
			[]string{"UUID", "HOSTNAME", "BMC HOSTNAME", "BMC IP"},
			basicNodeRows(nodes),
		)
	}
	if agentType == nvfleetint.NodeAgentTypeOOB || nodeListIsOOB(nodes) {
		return clioutput.WriteTable(
			w,
			[]string{"UUID", "BMC HOSTNAME", "BMC IP", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "VERIFICATION CHECK", "AGENT STATUS"},
			oobDetailNodeRows(nodes),
		)
	}
	// "VERIFICATION CHECK" is the user-facing label for the backend integrityCheck field.
	return clioutput.WriteTable(w, []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "GPU TYPE", "GPU COUNT", "VERIFICATION CHECK", "FIRMWARE CHECK", "AGENT STATUS"}, detailNodeRows(nodes))
}

// Reports whether a node list contains OOB-view records
func nodeListIsOOB(nodes []nvfleetint.Node) bool {
	for _, node := range nodes {
		if node.AgentType == string(nvfleetint.NodeAgentTypeOOB) || node.BMCHostname != "" || node.BMCIP != "" {
			return true
		}
	}
	return false
}

// Converts nodes into basic table rows
func basicNodeRows(nodes []nvfleetint.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.UUID),
			clioutput.DisplayString(node.Hostname),
			clioutput.DisplayString(node.BMCHostname),
			clioutput.DisplayString(node.BMCIP),
		})
	}
	return rows
}

// Converts nodes into detail table rows
func detailNodeRows(nodes []nvfleetint.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.UUID),
			clioutput.DisplayString(node.Hostname),
			clioutput.DisplayString(node.ComputeZone),
			clioutput.DisplayString(node.NodeGroup),
			clioutput.DisplayString(node.Health),
			clioutput.DisplayString(node.GPUType),
			clioutput.FormatOptionalInt(node.GPUCount),
			clioutput.DisplayString(node.IntegrityCheck),
			clioutput.DisplayString(node.FirmwareCheck),
			clioutput.DisplayString(node.AgentStatus),
		})
	}
	return rows
}

// Converts OOB nodes into detail table rows
func oobDetailNodeRows(nodes []nvfleetint.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{
			clioutput.DisplayString(node.UUID),
			clioutput.DisplayString(node.BMCHostname),
			clioutput.DisplayString(node.BMCIP),
			clioutput.DisplayString(node.ComputeZone),
			clioutput.DisplayString(node.NodeGroup),
			clioutput.DisplayString(node.Health),
			clioutput.DisplayString(node.IntegrityCheck),
			clioutput.DisplayString(node.AgentStatus),
		})
	}
	return rows
}

// Renders node detail fields as a table
func writeNodeDescribeResultTable(
	w io.Writer,
	result nodeDescribeResult,
	agentType nvfleetint.NodeAgentType,
	options nodeDescribeRenderOptions,
) error {
	if agentType == nvfleetint.NodeAgentTypeInband && result.Inband != nil {
		return writeNodeDescribeTable(w, *result.Inband, options)
	}
	if agentType == nvfleetint.NodeAgentTypeOOB && result.OOB != nil {
		return writeNodeDescribeTable(w, *result.OOB, options)
	}

	if !options.showSummary {
		return writeOOBInventoryTables(w, result.OOB.OOBInventory, options)
	}

	wroteSection := false
	if result.Inband != nil {
		if err := writeNodeViewSection(w, "In-band", *result.Inband, wroteSection); err != nil {
			return err
		}
		wroteSection = true
	}
	if result.OOB != nil {
		if err := writeNodeViewSection(w, "Out-of-band", *result.OOB, wroteSection); err != nil {
			return err
		}
		if result.OOB.OOBInventory != nil {
			return writeOOBInventoryTables(w, result.OOB.OOBInventory, options)
		}
	}
	return nil
}

func writeNodeViewSection(
	w io.Writer,
	title string,
	node nvfleetint.NodeDetails,
	leadingNewline bool,
) error {
	if leadingNewline {
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, nodeDescribeSummaryRows(node))
}

func writeNodeDescribeTable(
	w io.Writer,
	node nvfleetint.NodeDetails,
	options nodeDescribeRenderOptions,
) error {
	if options.showSummary {
		if err := clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, nodeDescribeSummaryRows(node)); err != nil {
			return err
		}
	}
	if node.OOBInventory == nil {
		return nil
	}
	return writeOOBInventoryTables(w, node.OOBInventory, options)
}

func nodeDescribeSummaryRows(node nvfleetint.NodeDetails) [][]string {
	rows := nodeDescribeRows(node)
	if node.AgentType == string(nvfleetint.NodeAgentTypeOOB) || node.OOBInventory != nil {
		rows = oobNodeDescribeRows(node)
	}
	if node.OOBInventory == nil {
		return rows
	}

	rows = append(rows, oobInventorySummaryRows(node.OOBInventory)...)
	if node.OOBInventory.Source != nil {
		rows = append(rows, oobSourceRows(node.OOBInventory.Source)...)
	}
	for index, domainError := range node.OOBInventory.DomainErrors {
		parts := make([]string, 0, 3)
		for _, value := range []string{domainError.Domain, domainError.Resource, domainError.Message} {
			if value = strings.TrimSpace(value); value != "" {
				parts = append(parts, value)
			}
		}
		rows = append(rows, []string{
			fmt.Sprintf("INVENTORY DOMAIN ERROR %d", index+1),
			clioutput.DisplayString(strings.Join(parts, ": ")),
		})
	}
	return rows
}

func oobNodeDescribeRows(node nvfleetint.NodeDetails) [][]string {
	return [][]string{
		{"UUID", clioutput.DisplayString(node.UUID)},
		{"HEALTH", clioutput.DisplayString(node.Health)},
		{"COMPUTE ZONE", clioutput.FormatNameAndID(node.ComputeZone, node.ComputeZoneID)},
		{"NODE GROUP", clioutput.FormatNameAndID(node.NodeGroup, node.NodeGroupID)},
		{"AGENT STATUS", clioutput.DisplayString(node.AgentStatus)},
		{"VERIFICATION CHECK", clioutput.DisplayString(node.IntegrityCheck)},
		{"VERIFICATION REASON", clioutput.DisplayString(node.IntegrityCheckReason)},
		{"TAGS", clioutput.FormatStringList(node.Tags)},
		{"ENROLLED AT", clioutput.DisplayString(node.EnrolledAt)},
		{"LAST UPDATED", clioutput.DisplayString(node.LastUpdatedTime)},
		{"LAST VERIFICATION CHECK", clioutput.DisplayString(node.LastIntegrityCheckTime)},
		{"HEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.HealthyComponentCount)},
		{"DEGRADED COMPONENTS", clioutput.FormatOptionalInt(node.DegradedComponentCount)},
		{"UNHEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.UnhealthyComponentCount)},
		{"LOCATION", clioutput.FormatGeoLocation(node.GeoLocation)},
		{"BMC HOSTNAME", clioutput.DisplayString(node.BMCHostname)},
		{"BMC IP", clioutput.DisplayString(node.BMCIP)},
	}
}

// Converts node details into describe table rows
func nodeDescribeRows(node nvfleetint.NodeDetails) [][]string {
	rows := [][]string{
		{"UUID", clioutput.DisplayString(node.UUID)},
		{"HOSTNAME", clioutput.DisplayString(node.Hostname)},
		{"AGENT TYPE", clioutput.DisplayString(node.AgentType)},
		{"HEALTH", clioutput.DisplayString(node.Health)},
		{"COMPUTE ZONE", clioutput.FormatNameAndID(node.ComputeZone, node.ComputeZoneID)},
		{"NODE GROUP", clioutput.FormatNameAndID(node.NodeGroup, node.NodeGroupID)},
		{"GPU TYPE", clioutput.DisplayString(node.GPUType)},
		{"GPU COUNT", clioutput.FormatOptionalInt(node.GPUCount)},
		{"AGENT STATUS", clioutput.DisplayString(node.AgentStatus)},
		// Verification check/reason/last-check map to the backend integrityCheck* fields.
		{"VERIFICATION CHECK", clioutput.DisplayString(node.IntegrityCheck)},
		{"VERIFICATION REASON", clioutput.DisplayString(node.IntegrityCheckReason)},
		{"FIRMWARE CHECK", clioutput.DisplayString(node.FirmwareCheck)},
		{"PUBLIC IP", clioutput.DisplayString(node.PublicIP)},
		{"PRIVATE IP", clioutput.DisplayString(node.PrivateIP)},
		{"TAGS", clioutput.FormatStringList(node.Tags)},
		{"ENROLLED AT", clioutput.DisplayString(node.EnrolledAt)},
		{"LAST UPDATED", clioutput.DisplayString(node.LastUpdatedTime)},
		{"LAST VERIFICATION CHECK", clioutput.DisplayString(node.LastIntegrityCheckTime)},
		{"HEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.HealthyComponentCount)},
		{"DEGRADED COMPONENTS", clioutput.FormatOptionalInt(node.DegradedComponentCount)},
		{"UNHEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.UnhealthyComponentCount)},
		// "LOCATION" is the user-facing label for the backend geoLocation field.
		{"LOCATION", clioutput.FormatGeoLocation(node.GeoLocation)},
	}

	if node.BMCHostname != "" || node.BMCIP != "" {
		rows = append(rows,
			[]string{"BMC HOSTNAME", clioutput.DisplayString(node.BMCHostname)},
			[]string{"BMC IP", clioutput.DisplayString(node.BMCIP)},
		)
	}
	if node.Resources != nil {
		rows = append(rows, nodeResourceRows(node.Resources)...)
	}
	if node.SystemInfo != nil {
		rows = append(rows, systemInfoRows(node.SystemInfo)...)
	}
	return rows
}

// Renders OOB inventory using the same domain sections as the collector detail view
func writeOOBInventoryTables(
	w io.Writer,
	inventory *nvfleetint.OOBInventory,
	options nodeDescribeRenderOptions,
) error {
	if options.sections[oobInventorySectionManagers] {
		if err := writeOOBSection(w, "Managers",
			[]string{"ID", "UUID", "MODEL", "TYPE", "FIRMWARE", "STATUS", "HEALTH", "ROLLUP"},
			oobManagerRows(inventory.Managers)); err != nil {
			return err
		}
	}
	if options.sections[oobInventorySectionSystems] {
		if err := writeOOBSection(w, "Systems",
			[]string{
				"PRIMARY", "ID", "UUID", "MANUFACTURER", "MODEL", "SKU", "SERIAL", "BIOS",
				"HOSTNAME", "ASSET", "POWER", "STATUS", "HEALTH", "ROLLUP", "MEMORY GIB", "SECURE BOOT",
			},
			oobSystemRows(inventory)); err != nil {
			return err
		}
		cpuRows, gpuRows := oobProcessorRows(inventory.Systems)
		if len(cpuRows) > 0 {
			if err := writeOOBSection(w, "CPUs",
				[]string{
					"SYSTEM", "ID", "SOCKET", "ARCH", "MANUFACTURER", "MODEL",
					"MAX MHZ", "CORES", "THREADS", "STATUS", "HEALTH", "ROLLUP",
				},
				cpuRows); err != nil {
				return err
			}
		}
		if len(gpuRows) > 0 {
			if err := writeOOBSection(w, "GPUs",
				[]string{"SYSTEM", "ID", "MANUFACTURER", "MODEL", "MAX MHZ", "STATUS", "HEALTH", "ROLLUP"},
				gpuRows); err != nil {
				return err
			}
		}
	}
	if options.sections[oobInventorySectionChassis] {
		if err := writeOOBSection(w, "Chassis",
			[]string{
				"ID", "TYPE", "MANUFACTURER", "MODEL", "SKU", "SERIAL", "PART",
				"ASSET", "POWER", "STATUS", "HEALTH", "ROLLUP", "LOCATION",
			},
			oobChassisRows(inventory.Chassis)); err != nil {
			return err
		}
		pcieRows := oobPCIeDeviceRows(inventory.Chassis)
		if len(pcieRows) > 0 {
			if err := writeOOBSection(w, "PCIe Devices",
				[]string{
					"CHASSIS", "ID", "UUID", "TYPE", "MANUFACTURER", "MODEL",
					"SKU", "SERIAL", "PART", "FIRMWARE", "STATUS", "HEALTH", "ROLLUP",
				},
				pcieRows); err != nil {
				return err
			}
		}
	}
	if options.sections[oobInventorySectionFirmware] {
		if err := writeOOBSection(w, "Firmware",
			[]string{"ID", "NAME", "VERSION", "RELEASE DATE", "STATUS", "HEALTH", "ROLLUP"},
			oobFirmwareRows(inventory.Firmware)); err != nil {
			return err
		}
	}
	if !options.showSummary && len(inventory.DomainErrors) > 0 {
		return writeOOBSection(w, "Domain Errors", []string{"DOMAIN", "RESOURCE", "MESSAGE"}, oobDomainErrorRows(inventory.DomainErrors))
	}
	return nil
}

func writeOOBSection(w io.Writer, title string, headers []string, rows [][]string) error {
	if _, err := fmt.Fprintf(w, "\n%s\n", title); err != nil {
		return err
	}
	return clioutput.WriteTable(w, headers, rows)
}

func oobInventorySummaryRows(inventory *nvfleetint.OOBInventory) [][]string {
	return [][]string{
		{"INVENTORY SCHEMA VERSION", clioutput.DisplayString(inventory.SchemaVersion)},
		{"INVENTORY COLLECTED AT", clioutput.DisplayString(inventory.CollectedAt)},
		{"INVENTORY PRIMARY SYSTEM", clioutput.DisplayString(inventory.PrimarySystemID)},
		{"INVENTORY MANAGERS", strconv.Itoa(len(inventory.Managers))},
		{"INVENTORY SYSTEMS", strconv.Itoa(len(inventory.Systems))},
		{"INVENTORY CHASSIS", strconv.Itoa(len(inventory.Chassis))},
		{"INVENTORY FIRMWARE", strconv.Itoa(len(inventory.Firmware))},
		{"INVENTORY DOMAIN ERRORS", strconv.Itoa(len(inventory.DomainErrors))},
		{"INVENTORY TARGET ERROR", clioutput.DisplayString(inventory.TargetError)},
	}
}

func oobSourceRows(source *nvfleetint.OOBSource) [][]string {
	return [][]string{
		{"SOURCE TYPE", clioutput.DisplayString(source.SourceType)},
		{"SOURCE MAC", clioutput.DisplayString(source.MAC)},
		{"SOURCE ADDRESS", clioutput.DisplayString(source.Address)},
		{"SOURCE HOSTNAME", clioutput.DisplayString(source.Hostname)},
		{"SOURCE REDFISH VERSION", clioutput.DisplayString(source.RedfishVersion)},
		{"SOURCE SERVICE UUID", clioutput.DisplayString(source.ServiceUUID)},
		{"SOURCE VENDOR", clioutput.DisplayString(source.Vendor)},
	}
}

func oobManagerRows(managers []nvfleetint.OOBManager) [][]string {
	rows := make([][]string, 0, len(managers))
	for _, manager := range managers {
		rows = append(rows, []string{
			manager.ID, manager.UUID, manager.Model, manager.ManagerType, manager.FirmwareVersion,
			manager.StatusState, manager.Health, manager.HealthRollup,
		})
	}
	return rows
}

func oobSystemRows(inventory *nvfleetint.OOBInventory) [][]string {
	rows := make([][]string, 0, len(inventory.Systems))
	for _, system := range inventory.Systems {
		rows = append(rows, []string{
			strconv.FormatBool(system.ID == inventory.PrimarySystemID),
			system.ID,
			system.UUID,
			system.Manufacturer,
			system.Model,
			system.SKU,
			system.SerialNumber,
			system.BIOSVersion,
			system.Hostname,
			system.AssetTag,
			system.PowerState,
			system.StatusState,
			system.Health,
			system.HealthRollup,
			formatOOBFloat32(system.MemoryGiB),
			formatOOBBool(system.SecureBootEnabled),
		})
	}
	return rows
}

func oobProcessorRows(systems []nvfleetint.OOBSystem) ([][]string, [][]string) {
	cpuRows := [][]string{}
	gpuRows := [][]string{}
	for _, system := range systems {
		for _, processor := range system.Processors {
			if strings.EqualFold(strings.TrimSpace(processor.ProcessorType), "gpu") {
				gpuRows = append(gpuRows, []string{
					system.ID, processor.ID, processor.Manufacturer, processor.Model,
					formatOOBInt(processor.MaxSpeedMHz), processor.StatusState,
					processor.Health, processor.HealthRollup,
				})
				continue
			}
			cpuRows = append(cpuRows, []string{
				system.ID, processor.ID, processor.Socket, processor.ProcessorArchitecture,
				processor.Manufacturer, processor.Model,
				formatOOBInt(processor.MaxSpeedMHz), formatOOBInt(processor.TotalCores),
				formatOOBInt(processor.TotalThreads), processor.StatusState,
				processor.Health, processor.HealthRollup,
			})
		}
	}
	return cpuRows, gpuRows
}

func oobChassisRows(chassis []nvfleetint.OOBChassis) [][]string {
	rows := make([][]string, 0, len(chassis))
	for _, item := range chassis {
		rows = append(rows, []string{
			item.ID, item.ChassisType, item.Manufacturer, item.Model, item.SKU,
			item.SerialNumber, item.PartNumber, item.AssetTag, item.PowerState,
			item.StatusState, item.Health, item.HealthRollup, formatOOBLocation(item.Location),
		})
	}
	return rows
}

func oobPCIeDeviceRows(chassis []nvfleetint.OOBChassis) [][]string {
	rows := [][]string{}
	for _, item := range chassis {
		for _, device := range item.PCIeDevices {
			rows = append(rows, []string{
				item.ID, device.ID, device.UUID, device.DeviceType, device.Manufacturer,
				device.Model, device.SKU, device.SerialNumber, device.PartNumber,
				device.FirmwareVersion, device.StatusState, device.Health, device.HealthRollup,
			})
		}
	}
	return rows
}

func oobFirmwareRows(firmware []nvfleetint.OOBFirmware) [][]string {
	rows := make([][]string, 0, len(firmware))
	for _, item := range firmware {
		rows = append(rows, []string{
			item.ID, item.Name, item.Version, item.ReleaseDate,
			item.StatusState, item.Health, item.HealthRollup,
		})
	}
	return rows
}

func oobDomainErrorRows(domainErrors []nvfleetint.OOBDomainError) [][]string {
	rows := make([][]string, 0, len(domainErrors))
	for _, domainError := range domainErrors {
		rows = append(rows, []string{domainError.Domain, domainError.Resource, domainError.Message})
	}
	return rows
}

func formatOOBLocation(location *nvfleetint.OOBChassisLocation) string {
	if location == nil {
		return ""
	}
	parts := []string{}
	if location.ServiceLabel != "" {
		parts = append(parts, location.ServiceLabel)
	}
	if location.Rack != "" {
		rack := "rack " + location.Rack
		if location.RackOffset != nil {
			rack += fmt.Sprintf(" U%d", *location.RackOffset)
		}
		parts = append(parts, rack)
	} else if location.RackOffset != nil {
		parts = append(parts, fmt.Sprintf("U%d", *location.RackOffset))
	}
	if location.Row != "" {
		parts = append(parts, "row "+location.Row)
	}
	if location.Room != "" {
		parts = append(parts, "room "+location.Room)
	}
	return strings.Join(parts, ", ")
}

func formatOOBInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func formatOOBFloat32(value *float32) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(float64(*value), 'f', -1, 32)
}

func formatOOBBool(value *bool) string {
	if value == nil {
		return ""
	}
	return strconv.FormatBool(*value)
}

// Converts optional node resources into describe table rows
func nodeResourceRows(resources *nvfleetint.NodeResources) [][]string {
	rows := [][]string{}
	if resources.CPUInfo != nil {
		rows = append(rows,
			[]string{"CPU TYPE", clioutput.DisplayString(resources.CPUInfo.Type)},
			[]string{"CPU MANUFACTURER", clioutput.DisplayString(resources.CPUInfo.Manufacturer)},
			[]string{"CPU ARCHITECTURE", clioutput.DisplayString(resources.CPUInfo.Architecture)},
			[]string{"CPU LOGICAL CORES", clioutput.DisplayString(resources.CPUInfo.LogicalCores)},
		)
	}
	if resources.GPUInfo != nil {
		rows = append(rows,
			[]string{"GPU PRODUCT", clioutput.DisplayString(resources.GPUInfo.Product)},
			[]string{"GPU MANUFACTURER", clioutput.DisplayString(resources.GPUInfo.Manufacturer)},
			[]string{"GPU ARCHITECTURE", clioutput.DisplayString(resources.GPUInfo.Architecture)},
			[]string{"GPU MEMORY", clioutput.DisplayString(resources.GPUInfo.Memory)},
			[]string{"GPU DEVICES", fmt.Sprintf("%d", len(resources.GPUInfo.GPUs))},
		)
	}
	if resources.MemoryInfo != nil {
		rows = append(rows, []string{"MEMORY TOTAL BYTES", clioutput.DisplayString(resources.MemoryInfo.TotalBytes)})
	}
	if resources.DiskInfo != nil {
		rows = append(rows,
			[]string{"CONTAINER ROOT DISK", clioutput.DisplayString(resources.DiskInfo.ContainerRootDisk)},
			[]string{"DISK DEVICES", fmt.Sprintf("%d", len(resources.DiskInfo.BlockDevices))},
		)
	}
	if resources.NICInfo != nil {
		rows = append(rows, []string{"NIC INTERFACES", fmt.Sprintf("%d", len(resources.NICInfo.PrivateIPInterfaces))})
	}
	return rows
}

// Converts system details into describe table rows
func systemInfoRows(system *nvfleetint.SystemInfo) [][]string {
	return [][]string{
		{"OS", clioutput.DisplayString(system.OperatingSystem)},
		{"OS IMAGE", clioutput.DisplayString(system.OSImage)},
		{"KERNEL", clioutput.DisplayString(system.KernelVersion)},
		{"GPU DRIVER", clioutput.DisplayString(system.GPUDriverVersion)},
		{"CUDA", clioutput.DisplayString(system.CUDAVersion)},
		{"DCGM", clioutput.DisplayString(system.DCGMVersion)},
		{"AGENT VERSION", clioutput.DisplayString(system.AgentVersion)},
		{"CONTAINER RUNTIME", clioutput.DisplayString(system.ContainerRuntimeVersion)},
		{"SYSTEM UUID", clioutput.DisplayString(system.SystemUUID)},
		{"BOOT ID", clioutput.DisplayString(system.BootID)},
		{"STARTED AT", clioutput.DisplayString(system.StartedAt)},
	}
}

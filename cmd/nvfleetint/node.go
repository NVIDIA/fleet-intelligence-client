// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	clihelpers "github.com/NVIDIA/fleet-intelligence-client/cmd/nvfleetint/helpers"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"

	"github.com/spf13/cobra"
)

// Stores local flag values for node list
type nodeListFlags struct {
	view             string
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
	JSONValue any
	RawJSON   []byte
	Page      *clioutput.Pagination
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
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runNodeList(cmd, flags, resolveCommonFlags(cmd, common))
		},
	}

	cmd.Flags().StringVar(&flags.view, "view", flags.view, "View mode: detail or basic")
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
	cmd.Flags().StringVar(&flags.agentStatus, "agent-status", "", "Comma-separated agent statuses to filter: Online, Offline, or Unknown")
	// User-facing "verification check" maps to the backend "integrity check" API field.
	cmd.Flags().StringVar(&flags.integrityCheck, "verification-check", "", "Comma-separated verification check statuses to filter: Verified, Unverified, Degraded, Pending, Unsupported, or Unknown")
	cmd.Flags().StringVar(&flags.firmwareCheck, "firmware-check", "", "Comma-separated firmware check statuses to filter: Passed, Failed, or Unknown")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field: hostname, nodeUUID, healthStatus, nodegroup, computezone, gpuType, gpuCount, integrityCheck, agentStatus, agentVersion, kernelVersion, gpuDriverVersion, or gpuFirmwareVersions")
	cmd.Flags().StringVar(&flags.order, "order", "", "Sort order: asc or desc")
	registerListCommonFlags(cmd, common)

	return cmd
}

// Creates the node describe command
func newNodeDescribeCmd() *cobra.Command {
	common := newCommonFlags()
	cmd := &cobra.Command{
		Use:   "describe <uuid>",
		Short: "Describe a node",
		Args:  requireSingleArg("node UUID"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNodeDescribe(cmd, args[0], resolveCommonFlags(cmd, common))
		},
	}

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
		AgentStatuses:    agentStatuses,
		IntegrityChecks:  integrityChecks,
		FirmwareChecks:   firmwareChecks,
		SortBy:           sortBy,
		Order:            nvfleetint.NodeSortOrder(flags.order),
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var nodes []nvfleetint.Node
		result, err := clihelpers.FetchAllRawPages("nodes", 0, func(pageNumber int) (clihelpers.RawPage, error) {
			page := pageNumber
			opts.Page = &page
			currentPage, err := client.ListNodes(cmd.Context(), opts)
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
		return writeNodeListOutput(cmd.OutOrStdout(), common, nodeListOutput{
			Nodes:     nodes,
			View:      flags.view,
			JSONValue: result,
		})
	}

	page, err := client.ListNodes(cmd.Context(), opts)
	if err != nil {
		return err
	}
	return writeNodeListOutput(cmd.OutOrStdout(), common, nodeListOutput{
		Nodes:   page.Nodes,
		View:    flags.view,
		RawJSON: page.RawJSON,
		Page: &clioutput.Pagination{
			Page:     page.Page,
			PageSize: page.PageSize,
			Total:    page.Total,
		},
	})
}

// Validates args, calls the SDK, and writes output
func runNodeDescribe(cmd *cobra.Command, nodeUUID string, common resolvedCommonFlags) error {
	if err := validateReadCommonFlags(common); err != nil {
		return err
	}

	nodeUUID = strings.TrimSpace(nodeUUID)
	if nodeUUID == "" {
		return errors.New("node UUID is required")
	}

	client, err := newConfiguredClient(common)
	if err != nil {
		return err
	}

	node, err := client.DescribeNode(cmd.Context(), nodeUUID)
	if err != nil {
		return err
	}

	if common.output == clioutput.FormatJSON {
		return clioutput.WriteRawJSON(cmd.OutOrStdout(), node.RawJSON)
	}
	return writeNodeDescribeTable(cmd.OutOrStdout(), node)
}

// Checks node list flags
func validateNodeListFlags(flags nodeListFlags, sortBy nvfleetint.NodeSortBy, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if !nvfleetint.NodeView(flags.view).Valid() {
		return fmt.Errorf("invalid view %q: expected basic or detail", flags.view)
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
		return fmt.Errorf("invalid sort-by %q: expected hostname, nodeUUID, healthStatus, nodegroup, computezone, gpuType, gpuCount, integrityCheck, agentStatus, agentVersion, kernelVersion, gpuDriverVersion, or gpuFirmwareVersions", flags.sortBy)
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
	case nvfleetint.NodeSortByHostname, nvfleetint.NodeSortByUUID:
		return true
	default:
		return false
	}
}

// Normalizes the raw sort-by flag into an API sort field
func normalizeNodeSortBy(raw string) (nvfleetint.NodeSortBy, error) {
	return nvfleetint.NodeSortBy(strings.TrimSpace(raw)), nil
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

	if err := writeNodeTable(w, result.View, result.Nodes); err != nil {
		return err
	}
	if result.Page == nil {
		return nil
	}
	return clioutput.WritePaginationFooter(w, *result.Page)
}

// Renders nodes using the selected view columns
func writeNodeTable(w io.Writer, view string, nodes []nvfleetint.Node) error {
	if nvfleetint.NodeView(view) == nvfleetint.NodeViewBasic {
		return clioutput.WriteTable(w, []string{"UUID", "HOSTNAME"}, basicNodeRows(nodes))
	}
	// "VERIFICATION CHECK" is the user-facing label for the backend integrityCheck field.
	return clioutput.WriteTable(w, []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "GPU TYPE", "GPU COUNT", "VERIFICATION CHECK", "FIRMWARE CHECK", "AGENT STATUS"}, detailNodeRows(nodes))
}

// Converts nodes into basic table rows
func basicNodeRows(nodes []nvfleetint.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{clioutput.DisplayString(node.UUID), clioutput.DisplayString(node.Hostname)})
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

// Renders node detail fields as a table
func writeNodeDescribeTable(w io.Writer, node nvfleetint.NodeDetails) error {
	return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, nodeDescribeRows(node))
}

// Converts node details into describe table rows
func nodeDescribeRows(node nvfleetint.NodeDetails) [][]string {
	rows := [][]string{
		{"UUID", clioutput.DisplayString(node.UUID)},
		{"HOSTNAME", clioutput.DisplayString(node.Hostname)},
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

	if node.Resources != nil {
		rows = append(rows, nodeResourceRows(node.Resources)...)
	}
	if node.SystemInfo != nil {
		rows = append(rows, systemInfoRows(node.SystemInfo)...)
	}

	return rows
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

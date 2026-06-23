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

// Stores local flag values for node list
type nodeListFlags struct {
	view           string
	nodeUUIDs      string
	health         string
	hostname       string
	agentStatus    string
	integrityCheck string
	firmwareCheck  string
	sortBy         string
	order          string
}

// Stores data ready for node list rendering
type nodeListOutput struct {
	Nodes     []fleetintelligence.Node
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

	return cmd
}

// Creates the node list command
func newNodeListCmd() *cobra.Command {
	flags := nodeListFlags{
		view: string(fleetintelligence.NodeViewDetail),
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
	cmd.Flags().StringVar(&flags.hostname, "hostname", "", "Hostname partial match")
	cmd.Flags().StringVar(&flags.agentStatus, "agent-status", "", "Comma-separated agent statuses to filter: Online, Offline, or Unknown")
	cmd.Flags().StringVar(&flags.integrityCheck, "integrity-check", "", "Comma-separated integrity check statuses to filter: Verified, Unverified, Degraded, Pending, Unsupported, or Unknown")
	cmd.Flags().StringVar(&flags.firmwareCheck, "firmware-check", "", "Comma-separated firmware check statuses to filter: Passed, Failed, or Unknown")
	cmd.Flags().StringVar(&flags.sortBy, "sort-by", "", "Sort field: hostname, nodeUUID, health, healthStatus, nodeGroup, nodegroup, computeZone, computezone, gpuType, gpuCount, integrityCheck, or agentStatus")
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
		Args:  cobra.ExactArgs(1),
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

	client, err := newConfiguredClient(commonClientOptions(common)...)
	if err != nil {
		return err
	}

	opts := fleetintelligence.ListNodesOptions{
		View:            fleetintelligence.NodeView(flags.view),
		NodeUUIDs:       nodeUUIDs,
		HealthStatuses:  healthStatuses,
		Hostname:        strings.TrimSpace(flags.hostname),
		AgentStatuses:   agentStatuses,
		IntegrityChecks: integrityChecks,
		FirmwareChecks:  firmwareChecks,
		SortBy:          sortBy,
		Order:           fleetintelligence.NodeSortOrder(flags.order),
	}
	applyPagination(common, func(page *int) { opts.Page = page }, func(pageSize *int) { opts.PageSize = pageSize })

	if common.all {
		var nodes []fleetintelligence.Node
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
			HasMore:  page.HasMore,
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

	client, err := newConfiguredClient(commonClientOptions(common)...)
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
func validateNodeListFlags(flags nodeListFlags, sortBy fleetintelligence.NodeSortBy, common resolvedCommonFlags) error {
	if err := validateListCommonFlags(common); err != nil {
		return err
	}
	if !fleetintelligence.NodeView(flags.view).Valid() {
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
	if sortBy != "" && !sortBy.Valid() {
		return fmt.Errorf("invalid sort-by %q: expected hostname, nodeUUID, health, healthStatus, nodeGroup, nodegroup, computeZone, computezone, gpuType, gpuCount, integrityCheck, or agentStatus", flags.sortBy)
	}
	if flags.order != "" && !fleetintelligence.NodeSortOrder(flags.order).Valid() {
		return fmt.Errorf("invalid order %q: expected asc or desc", flags.order)
	}
	if fleetintelligence.NodeView(flags.view) == fleetintelligence.NodeViewBasic {
		if strings.TrimSpace(flags.health) != "" || strings.TrimSpace(flags.agentStatus) != "" || strings.TrimSpace(flags.integrityCheck) != "" || strings.TrimSpace(flags.firmwareCheck) != "" {
			return errors.New("basic node view is incompatible with health, agent-status, integrity-check, and firmware-check filters")
		}
		if sortBy != "" && !basicNodeSortCompatible(sortBy) {
			return fmt.Errorf("basic node view is incompatible with sort %q", flags.sortBy)
		}
	}
	return nil
}

// Reports whether a sort field works with basic view
func basicNodeSortCompatible(sortBy fleetintelligence.NodeSortBy) bool {
	switch sortBy {
	case fleetintelligence.NodeSortByHostname, fleetintelligence.NodeSortByUUID:
		return true
	default:
		return false
	}
}

// Maps friendly CLI aliases to API sort fields
func normalizeNodeSortBy(raw string) (fleetintelligence.NodeSortBy, error) {
	switch strings.TrimSpace(raw) {
	case "":
		return "", nil
	case "health":
		return fleetintelligence.NodeSortByHealthStatus, nil
	case "nodeGroup":
		return fleetintelligence.NodeSortByNodeGroup, nil
	case "computeZone":
		return fleetintelligence.NodeSortByComputeZone, nil
	default:
		return fleetintelligence.NodeSortBy(raw), nil
	}
}

// Converts comma-separated health filters into API values
func parseNodeHealthList(raw string) ([]fleetintelligence.NodeHealthStatus, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	statuses := make([]fleetintelligence.NodeHealthStatus, 0, len(values))
	for _, value := range values {
		status := fleetintelligence.NodeHealthStatus(value)
		if !status.Valid() {
			return nil, fmt.Errorf("invalid health %q: expected Healthy, Degraded, Unhealthy, or Unknown", value)
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// Converts comma-separated agent filters into API values
func parseNodeAgentStatusList(raw string) ([]fleetintelligence.NodeAgentStatus, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	statuses := make([]fleetintelligence.NodeAgentStatus, 0, len(values))
	for _, value := range values {
		status := fleetintelligence.NodeAgentStatus(value)
		if !status.Valid() {
			return nil, fmt.Errorf("invalid agent-status %q: expected Online, Offline, or Unknown", value)
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// Converts comma-separated integrity filters into API values
func parseNodeIntegrityCheckList(raw string) ([]fleetintelligence.NodeIntegrityCheck, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	checks := make([]fleetintelligence.NodeIntegrityCheck, 0, len(values))
	for _, value := range values {
		check := fleetintelligence.NodeIntegrityCheck(value)
		if !check.Valid() {
			return nil, fmt.Errorf("invalid integrity-check %q: expected Verified, Unverified, Degraded, Pending, Unsupported, or Unknown", value)
		}
		checks = append(checks, check)
	}

	return checks, nil
}

// Converts comma-separated firmware filters into API values
func parseNodeFirmwareCheckList(raw string) ([]fleetintelligence.NodeFirmwareCheck, error) {
	values, err := clihelpers.ParseCommaList(raw)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, nil
	}

	checks := make([]fleetintelligence.NodeFirmwareCheck, 0, len(values))
	for _, value := range values {
		check := fleetintelligence.NodeFirmwareCheck(value)
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
		if result.RawJSON != nil {
			return clioutput.WriteRawJSON(w, result.RawJSON)
		}
		return clioutput.WriteJSON(w, result.JSONValue)
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
func writeNodeTable(w io.Writer, view string, nodes []fleetintelligence.Node) error {
	if fleetintelligence.NodeView(view) == fleetintelligence.NodeViewBasic {
		return clioutput.WriteTable(w, []string{"UUID", "HOSTNAME"}, basicNodeRows(nodes))
	}
	return clioutput.WriteTable(w, []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "GPU TYPE", "GPU COUNT", "INTEGRITY CHECK", "FIRMWARE CHECK", "AGENT STATUS"}, detailNodeRows(nodes))
}

// Converts nodes into basic table rows
func basicNodeRows(nodes []fleetintelligence.Node) [][]string {
	rows := make([][]string, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, []string{clioutput.DisplayString(node.UUID), clioutput.DisplayString(node.Hostname)})
	}
	return rows
}

// Converts nodes into detail table rows
func detailNodeRows(nodes []fleetintelligence.Node) [][]string {
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
func writeNodeDescribeTable(w io.Writer, node fleetintelligence.NodeDetails) error {
	return clioutput.WriteTable(w, []string{"FIELD", "VALUE"}, nodeDescribeRows(node))
}

// Converts node details into describe table rows
func nodeDescribeRows(node fleetintelligence.NodeDetails) [][]string {
	rows := [][]string{
		{"UUID", clioutput.DisplayString(node.UUID)},
		{"HOSTNAME", clioutput.DisplayString(node.Hostname)},
		{"HEALTH", clioutput.DisplayString(node.Health)},
		{"COMPUTE ZONE", clioutput.FormatNameAndID(node.ComputeZone, node.ComputeZoneID)},
		{"NODE GROUP", clioutput.FormatNameAndID(node.NodeGroup, node.NodeGroupID)},
		{"GPU TYPE", clioutput.DisplayString(node.GPUType)},
		{"GPU COUNT", clioutput.FormatOptionalInt(node.GPUCount)},
		{"AGENT STATUS", clioutput.DisplayString(node.AgentStatus)},
		{"INTEGRITY CHECK", clioutput.DisplayString(node.IntegrityCheck)},
		{"INTEGRITY REASON", clioutput.DisplayString(node.IntegrityCheckReason)},
		{"FIRMWARE CHECK", clioutput.DisplayString(node.FirmwareCheck)},
		{"PUBLIC IP", clioutput.DisplayString(node.PublicIP)},
		{"PRIVATE IP", clioutput.DisplayString(node.PrivateIP)},
		{"TAGS", clioutput.FormatStringList(node.Tags)},
		{"ENROLLED AT", clioutput.DisplayString(node.EnrolledAt)},
		{"LAST UPDATED", clioutput.DisplayString(node.LastUpdatedTime)},
		{"LAST INTEGRITY CHECK", clioutput.DisplayString(node.LastIntegrityCheckTime)},
		{"HEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.HealthyComponentCount)},
		{"DEGRADED COMPONENTS", clioutput.FormatOptionalInt(node.DegradedComponentCount)},
		{"UNHEALTHY COMPONENTS", clioutput.FormatOptionalInt(node.UnhealthyComponentCount)},
		{"GEOLOCATION", clioutput.FormatGeoLocation(node.GeoLocation)},
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
func nodeResourceRows(resources *fleetintelligence.NodeResources) [][]string {
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
func systemInfoRows(system *fleetintelligence.SystemInfo) [][]string {
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

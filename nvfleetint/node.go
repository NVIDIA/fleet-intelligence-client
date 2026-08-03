// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

const (
	NodeViewDetail NodeView = "detail"
	NodeViewBasic  NodeView = "basic"

	NodeHealthHealthy   NodeHealthStatus = "Healthy"
	NodeHealthDegraded  NodeHealthStatus = "Degraded"
	NodeHealthUnhealthy NodeHealthStatus = "Unhealthy"
	NodeHealthUnknown   NodeHealthStatus = "Unknown"

	NodeIntegrityVerified    NodeIntegrityCheck = "Verified"
	NodeIntegrityUnverified  NodeIntegrityCheck = "Unverified"
	NodeIntegrityDegraded    NodeIntegrityCheck = "Degraded"
	NodeIntegrityPending     NodeIntegrityCheck = "Pending"
	NodeIntegrityUnsupported NodeIntegrityCheck = "Unsupported"
	NodeIntegrityUnknown     NodeIntegrityCheck = "Unknown"
	// Deprecated: use NodeIntegrityVerified
	NodeIntegrityPassed NodeIntegrityCheck = NodeIntegrityVerified
	// Deprecated: use NodeIntegrityUnverified
	NodeIntegrityFailed NodeIntegrityCheck = NodeIntegrityUnverified

	NodeFirmwarePassed  NodeFirmwareCheck = "Passed"
	NodeFirmwareFailed  NodeFirmwareCheck = "Failed"
	NodeFirmwareUnknown NodeFirmwareCheck = "Unknown"

	NodeAgentOnline  NodeAgentStatus = "Online"
	NodeAgentOffline NodeAgentStatus = "Offline"
	NodeAgentUnknown NodeAgentStatus = "Unknown"

	NodeSortByHostname            NodeSortBy = "hostname"
	NodeSortByUUID                NodeSortBy = "nodeUUID"
	NodeSortByHealthStatus        NodeSortBy = "healthStatus"
	NodeSortByNodeGroup           NodeSortBy = "nodegroup"
	NodeSortByComputeZone         NodeSortBy = "computezone"
	NodeSortByGPUType             NodeSortBy = "gpuType"
	NodeSortByGPUCount            NodeSortBy = "gpuCount"
	NodeSortByIntegrityCheck      NodeSortBy = "integrityCheck"
	NodeSortByAgentStatus         NodeSortBy = "agentStatus"
	NodeSortByAgentVersion        NodeSortBy = "agentVersion"
	NodeSortByKernelVersion       NodeSortBy = "kernelVersion"
	NodeSortByGPUDriverVersion    NodeSortBy = "gpuDriverVersion"
	NodeSortByGPUFirmwareVersions NodeSortBy = "gpuFirmwareVersions"

	NodeOrderAsc  NodeSortOrder = "asc"
	NodeOrderDesc NodeSortOrder = "desc"
)

// Represents supported response shapes for listing nodes
type NodeView string

// Reports whether the view is accepted by the API
func (view NodeView) Valid() bool {
	return fleetapi.GetV1NodesParamsView(view).Valid()
}

// Represents supported health filters for listing nodes
type NodeHealthStatus string

// Reports whether the health status is accepted by the API
func (status NodeHealthStatus) Valid() bool {
	return fleetapi.GetV1NodesParamsHealthStatuses(status).Valid()
}

// Represents supported integrity check filters for listing nodes.
// This retains the backend "integrity check" vocabulary; the CLI surfaces it
// to users as "verification check".
type NodeIntegrityCheck string

// Reports whether the integrity check status is accepted by the API
func (check NodeIntegrityCheck) Valid() bool {
	return fleetapi.GetV1NodesParamsIntegrityChecks(check).Valid()
}

// Represents supported firmware check filters for listing nodes
type NodeFirmwareCheck string

// Reports whether the firmware check status is accepted by the API
func (check NodeFirmwareCheck) Valid() bool {
	return fleetapi.GetV1NodesParamsFirmwareChecks(check).Valid()
}

// Represents supported agent status filters for listing nodes
type NodeAgentStatus string

// Reports whether the agent status is accepted by the API
func (status NodeAgentStatus) Valid() bool {
	return fleetapi.GetV1NodesParamsAgentStatuses(status).Valid()
}

// Represents supported sort fields for listing nodes
type NodeSortBy string

// Reports whether the sort field is accepted by the API
func (sortBy NodeSortBy) Valid() bool {
	return fleetapi.GetV1NodesParamsSortBy(sortBy).Valid()
}

// Represents supported sort orders for listing nodes
type NodeSortOrder string

// Reports whether the sort order is accepted by the API
func (order NodeSortOrder) Valid() bool {
	return fleetapi.GetV1NodesParamsOrder(order).Valid()
}

// Represents request options for listing nodes
type ListNodesOptions struct {
	View             NodeView
	NodeUUIDs        []string
	HealthStatuses   []NodeHealthStatus
	ComputeZoneIDs   []string
	ComputeZoneNames []string
	NodeGroupIDs     []string
	NodeGroupNames   []string
	GPUTypes         []string
	GPUCounts        []int
	PublicIPs        []string
	PrivateIPs       []string
	Hostname         string
	AgentStatuses    []NodeAgentStatus
	IntegrityChecks  []NodeIntegrityCheck
	FirmwareChecks   []NodeFirmwareCheck
	SortBy           NodeSortBy
	Order            NodeSortOrder
	Page             *int
	PageSize         *int
}

// Represents a paginated node response with the raw backend payload
type NodesPage struct {
	Nodes    []Node `json:"nodes"`
	HasMore  bool   `json:"hasMore"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	Total    int    `json:"total"`
	RawJSON  []byte `json:"-"`
}

// Represents a node
type Node struct {
	UUID                   string `json:"nodeUUID"`
	Hostname               string `json:"hostname"`
	ComputeZone            string `json:"computeZone,omitempty"`
	NodeGroup              string `json:"nodeGroup,omitempty"`
	Health                 string `json:"healthStatus,omitempty"`
	GPUType                string `json:"gpuType,omitempty"`
	GPUCount               *int   `json:"gpuCount,omitempty"`
	AgentStatus            string `json:"agentStatus,omitempty"`
	IntegrityCheck         string `json:"integrityCheck,omitempty"`
	IntegrityCheckReason   string `json:"integrityCheckReason,omitempty"`
	FirmwareCheck          string `json:"firmwareCheck,omitempty"`
	PublicIP               string `json:"publicIP,omitempty"`
	PrivateIP              string `json:"privateIP,omitempty"`
	LastIntegrityCheckTime string `json:"lastIntegrityCheckTS,omitempty"`
	LastUpdatedTime        string `json:"lastUpdatedTS,omitempty"`
}

// Represents detailed node metadata
type NodeDetails struct {
	Node
	ComputeZoneID           string         `json:"computeZoneId,omitempty"`
	NodeGroupID             string         `json:"nodeGroupId,omitempty"`
	EnrolledAt              string         `json:"enrolledAt,omitempty"`
	GeoLocation             *GeoLocation   `json:"geoLocation,omitempty"`
	HealthyComponentCount   *int           `json:"healthyComponentCount,omitempty"`
	DegradedComponentCount  *int           `json:"degradedComponentCount,omitempty"`
	UnhealthyComponentCount *int           `json:"unhealthyComponentCount,omitempty"`
	Resources               *NodeResources `json:"resources,omitempty"`
	SystemInfo              *SystemInfo    `json:"systemInfo,omitempty"`
	Tags                    []string       `json:"tags,omitempty"`
	RawJSON                 []byte         `json:"-"`
}

// Represents hardware and network resource metadata for a node
type NodeResources struct {
	CPUInfo    *CPUInfo    `json:"cpuInfo,omitempty"`
	DiskInfo   *DiskInfo   `json:"diskInfo,omitempty"`
	GPUInfo    *GPUInfo    `json:"gpuInfo,omitempty"`
	MemoryInfo *MemoryInfo `json:"memoryInfo,omitempty"`
	NICInfo    *NICInfo    `json:"nicInfo,omitempty"`
}

// Represents CPU metadata for a node
type CPUInfo struct {
	Architecture string `json:"architecture,omitempty"`
	LogicalCores string `json:"logicalCores,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Type         string `json:"type,omitempty"`
}

// Represents disk metadata for a node
type DiskInfo struct {
	BlockDevices      []BlockDevice `json:"blockDevices,omitempty"`
	ContainerRootDisk string        `json:"containerRootDisk,omitempty"`
}

// Represents a node disk block device
type BlockDevice struct {
	FSType     string   `json:"fsType,omitempty"`
	MountPoint string   `json:"mountPoint,omitempty"`
	Name       string   `json:"name,omitempty"`
	Parents    []string `json:"parents,omitempty"`
	PartUUID   string   `json:"partUUID,omitempty"`
	Size       *int     `json:"size,omitempty"`
	Type       string   `json:"type,omitempty"`
	Used       *int     `json:"used,omitempty"`
	WWN        string   `json:"wwn,omitempty"`
}

// Represents GPU metadata for a node
type GPUInfo struct {
	Architecture string      `json:"architecture,omitempty"`
	GPUs         []GPUDevice `json:"gpus,omitempty"`
	Manufacturer string      `json:"manufacturer,omitempty"`
	Memory       string      `json:"memory,omitempty"`
	Product      string      `json:"product,omitempty"`
}

// Represents a GPU device on a node
type GPUDevice struct {
	BoardID      *int   `json:"boardID,omitempty"`
	BusID        string `json:"busID,omitempty"`
	ChassisSN    string `json:"chassisSN,omitempty"`
	GPUIndex     string `json:"gpuIndex,omitempty"`
	MinorID      string `json:"minorID,omitempty"`
	SerialNumber string `json:"sn,omitempty"`
	UUID         string `json:"uuid,omitempty"`
	VBIOSVersion string `json:"vbiosVersion,omitempty"`
}

// Represents memory metadata for a node
type MemoryInfo struct {
	TotalBytes string `json:"totalBytes,omitempty"`
}

// Represents network interface metadata for a node
type NICInfo struct {
	PrivateIPInterfaces []NICInterface `json:"privateIPInterfaces,omitempty"`
}

// Represents private network interface metadata for a node
type NICInterface struct {
	Interface string `json:"interface,omitempty"`
	IP        string `json:"ip,omitempty"`
	MAC       string `json:"mac,omitempty"`
}

// Represents system metadata reported by the node agent
type SystemInfo struct {
	AgentVersion            string `json:"agentVersion,omitempty"`
	BootID                  string `json:"bootID,omitempty"`
	ContainerRuntimeVersion string `json:"containerRuntimeVersion,omitempty"`
	CUDAVersion             string `json:"cudaVersion,omitempty"`
	DCGMVersion             string `json:"dcgmVersion,omitempty"`
	GPUDriverVersion        string `json:"gpuDriverVersion,omitempty"`
	Hostname                string `json:"hostname,omitempty"`
	KernelVersion           string `json:"kernelVersion,omitempty"`
	OperatingSystem         string `json:"operatingSystem,omitempty"`
	OSImage                 string `json:"osImage,omitempty"`
	StartedAt               string `json:"startedAt,omitempty"`
	SystemUUID              string `json:"systemUUID,omitempty"`
}

// Lists nodes using the configured API client
func (c *Client) ListNodes(ctx context.Context, opts ListNodesOptions) (NodesPage, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	view, err := normalizeNodeView(opts.View)
	if err != nil {
		return NodesPage{}, err
	}
	if err := validateNodeOptions(view, opts); err != nil {
		return NodesPage{}, err
	}

	params := fleetapi.GetV1NodesParams{
		View: nodeViewParam(view),
	}
	if len(opts.NodeUUIDs) > 0 {
		nodeUUIDs := append([]string(nil), opts.NodeUUIDs...)
		params.NodeUUIDs = &nodeUUIDs
	}
	if opts.Hostname != "" {
		params.Hostname = &opts.Hostname
	}
	if len(opts.ComputeZoneIDs) > 0 {
		computeZoneIDs := append([]string(nil), opts.ComputeZoneIDs...)
		params.ComputeZoneIds = &computeZoneIDs
	}
	if len(opts.ComputeZoneNames) > 0 {
		computeZoneNames := append([]string(nil), opts.ComputeZoneNames...)
		params.ComputeZoneNames = &computeZoneNames
	}
	if len(opts.NodeGroupIDs) > 0 {
		nodeGroupIDs := append([]string(nil), opts.NodeGroupIDs...)
		params.NodeGroupIds = &nodeGroupIDs
	}
	if len(opts.NodeGroupNames) > 0 {
		nodeGroupNames := append([]string(nil), opts.NodeGroupNames...)
		params.NodeGroupNames = &nodeGroupNames
	}
	if len(opts.GPUTypes) > 0 {
		gpuTypes := append([]string(nil), opts.GPUTypes...)
		params.GpuTypes = &gpuTypes
	}
	if len(opts.GPUCounts) > 0 {
		gpuCounts := append([]int(nil), opts.GPUCounts...)
		params.GpuCounts = &gpuCounts
	}
	if len(opts.PublicIPs) > 0 {
		publicIPs := append([]string(nil), opts.PublicIPs...)
		params.PublicIPs = &publicIPs
	}
	if len(opts.PrivateIPs) > 0 {
		privateIPs := append([]string(nil), opts.PrivateIPs...)
		params.PrivateIPs = &privateIPs
	}
	if view == NodeViewDetail {
		if len(opts.HealthStatuses) > 0 {
			statuses := make([]fleetapi.GetV1NodesParamsHealthStatuses, 0, len(opts.HealthStatuses))
			for _, status := range opts.HealthStatuses {
				statuses = append(statuses, fleetapi.GetV1NodesParamsHealthStatuses(status))
			}
			params.HealthStatuses = &statuses
		}
		if len(opts.AgentStatuses) > 0 {
			statuses := make([]fleetapi.GetV1NodesParamsAgentStatuses, 0, len(opts.AgentStatuses))
			for _, status := range opts.AgentStatuses {
				statuses = append(statuses, fleetapi.GetV1NodesParamsAgentStatuses(status))
			}
			params.AgentStatuses = &statuses
		}
		if len(opts.IntegrityChecks) > 0 {
			checks := make([]fleetapi.GetV1NodesParamsIntegrityChecks, 0, len(opts.IntegrityChecks))
			for _, check := range opts.IntegrityChecks {
				checks = append(checks, fleetapi.GetV1NodesParamsIntegrityChecks(check))
			}
			params.IntegrityChecks = &checks
		}
		if len(opts.FirmwareChecks) > 0 {
			checks := make([]fleetapi.GetV1NodesParamsFirmwareChecks, 0, len(opts.FirmwareChecks))
			for _, check := range opts.FirmwareChecks {
				checks = append(checks, fleetapi.GetV1NodesParamsFirmwareChecks(check))
			}
			params.FirmwareChecks = &checks
		}
	}
	if opts.SortBy != "" {
		sortBy := fleetapi.GetV1NodesParamsSortBy(opts.SortBy)
		params.SortBy = &sortBy
	}
	if opts.Order != "" {
		order := fleetapi.GetV1NodesParamsOrder(opts.Order)
		params.Order = &order
	}
	if opts.Page != nil {
		params.Page = cloneInt(opts.Page)
	}
	if opts.PageSize != nil {
		params.PageSize = cloneInt(opts.PageSize)
	}

	resp, err := c.api.GetV1NodesWithResponse(ctx, &params)
	if err != nil {
		return NodesPage{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return NodesPage{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	if view == NodeViewBasic {
		return decodeBasicNodes(resp.Body)
	}

	return decodeDetailNodes(resp.Body)
}

// Retrieves detail for a single node using the configured API client
func (c *Client) DescribeNode(ctx context.Context, nodeUUID string) (NodeDetails, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if nodeUUID == "" {
		return NodeDetails{}, fmt.Errorf("node UUID is required")
	}

	resp, err := c.api.GetV1NodesNodeUuidWithResponse(ctx, nodeUUID)
	if err != nil {
		return NodeDetails{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return NodeDetails{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var data fleetapi.ModelsNodeDetailsResponse
	if err := json.Unmarshal(resp.Body, &data); err != nil {
		return NodeDetails{}, err
	}

	node := nodeDetailsFromGenerated(data)
	node.RawJSON = append([]byte(nil), resp.Body...)
	return node, nil
}

// Defaults an omitted view and rejects unsupported values
func normalizeNodeView(view NodeView) (NodeView, error) {
	if view == "" {
		return NodeViewDetail, nil
	}
	if !view.Valid() {
		return "", fmt.Errorf("invalid node view %q: expected basic or detail", view)
	}

	return view, nil
}

// Checks node list options before making the request
func validateNodeOptions(view NodeView, opts ListNodesOptions) error {
	for _, status := range opts.HealthStatuses {
		if !status.Valid() {
			return fmt.Errorf("invalid node health %q: expected Healthy, Degraded, Unhealthy, or Unknown", status)
		}
	}
	for _, status := range opts.AgentStatuses {
		if !status.Valid() {
			return fmt.Errorf("invalid node agent status %q: expected Online, Offline, or Unknown", status)
		}
	}
	for _, check := range opts.IntegrityChecks {
		if !check.Valid() {
			return fmt.Errorf("invalid node verification check %q: expected Verified, Unverified, Degraded, Pending, Unsupported, or Unknown", check)
		}
	}
	for _, check := range opts.FirmwareChecks {
		if !check.Valid() {
			return fmt.Errorf("invalid node firmware check %q: expected Passed, Failed, or Unknown", check)
		}
	}
	for _, count := range opts.GPUCounts {
		if count < 0 {
			return fmt.Errorf("invalid node GPU count %d: expected a non-negative integer", count)
		}
	}
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return fmt.Errorf("invalid node sort %q: expected hostname, nodeUUID, healthStatus, nodegroup, computezone, gpuType, gpuCount, integrityCheck, agentStatus, agentVersion, kernelVersion, gpuDriverVersion, or gpuFirmwareVersions", opts.SortBy)
	}
	if opts.Order != "" && !opts.Order.Valid() {
		return fmt.Errorf("invalid node order %q: expected asc or desc", opts.Order)
	}
	if view == NodeViewBasic {
		if len(opts.HealthStatuses) > 0 || len(opts.AgentStatuses) > 0 || len(opts.IntegrityChecks) > 0 || len(opts.FirmwareChecks) > 0 {
			return fmt.Errorf("basic node view is incompatible with health, agent-status, verification-check, and firmware-check filters")
		}
		if opts.SortBy != "" && !nodeBasicSortCompatible(opts.SortBy) {
			return fmt.Errorf("basic node view is incompatible with sort %q", opts.SortBy)
		}
	}

	return nil
}

// Reports whether a sort field works with basic view
func nodeBasicSortCompatible(sortBy NodeSortBy) bool {
	switch sortBy {
	case NodeSortByHostname, NodeSortByUUID:
		return true
	default:
		return false
	}
}

// Converts a normalized view into the generated parameter type
func nodeViewParam(view NodeView) *fleetapi.GetV1NodesParamsView {
	param := fleetapi.GetV1NodesParamsView(view)
	return &param
}

// Decodes detail responses and preserves the original payload
func decodeDetailNodes(data []byte) (NodesPage, error) {
	var resp fleetapi.ModelsNodesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return NodesPage{}, err
	}

	page := NodesPage{
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.Nodes != nil {
		page.Nodes = make([]Node, 0, len(*resp.Nodes))
		for _, node := range *resp.Nodes {
			page.Nodes = append(page.Nodes, nodeFromGenerated(node))
		}
	}

	return page, nil
}

// Decodes basic responses and preserves the original payload
func decodeBasicNodes(data []byte) (NodesPage, error) {
	var resp fleetapi.ModelsBasicNodesResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return NodesPage{}, err
	}

	page := NodesPage{
		HasMore:  boolValue(resp.HasMore),
		Page:     intValue(resp.Page),
		PageSize: intValue(resp.PageSize),
		Total:    intValue(resp.Total),
		RawJSON:  append([]byte(nil), data...),
	}
	if resp.Nodes != nil {
		page.Nodes = make([]Node, 0, len(*resp.Nodes))
		for _, node := range *resp.Nodes {
			page.Nodes = append(page.Nodes, nodeFromSimple(node))
		}
	}

	return page, nil
}

// Maps detail API models into SDK values
func nodeFromGenerated(node fleetapi.ModelsNode) Node {
	return Node{
		UUID:                   stringValue(node.NodeUUID),
		Hostname:               stringValue(node.Hostname),
		ComputeZone:            stringValue(node.ComputeZone),
		NodeGroup:              stringValue(node.NodeGroup),
		Health:                 enumStringValue(node.HealthStatus),
		GPUType:                stringValue(node.GpuType),
		GPUCount:               cloneInt(node.GpuCount),
		AgentStatus:            enumStringValue(node.AgentStatus),
		IntegrityCheck:         enumStringValue(node.IntegrityCheck),
		IntegrityCheckReason:   stringValue(node.IntegrityCheckReason),
		FirmwareCheck:          enumStringValue(node.FirmwareCheck),
		PublicIP:               stringValue(node.PublicIP),
		PrivateIP:              stringValue(node.PrivateIP),
		LastIntegrityCheckTime: stringValue(node.LastIntegrityCheckTS),
		LastUpdatedTime:        stringValue(node.LastUpdatedTS),
	}
}

// Maps basic API models into SDK values
func nodeFromSimple(node fleetapi.ModelsSimpleNode) Node {
	return Node{
		UUID:     stringValue(node.NodeUUID),
		Hostname: stringValue(node.Hostname),
	}
}

// Maps node detail API models into SDK values
func nodeDetailsFromGenerated(node fleetapi.ModelsNodeDetailsResponse) NodeDetails {
	return NodeDetails{
		Node: Node{
			UUID:                   stringValue(node.NodeUUID),
			Hostname:               stringValue(node.Hostname),
			ComputeZone:            stringValue(node.ComputeZone),
			NodeGroup:              stringValue(node.NodeGroup),
			Health:                 enumStringValue(node.HealthStatus),
			GPUType:                stringValue(node.GpuType),
			GPUCount:               cloneInt(node.GpuCount),
			AgentStatus:            enumStringValue(node.AgentStatus),
			IntegrityCheck:         enumStringValue(node.IntegrityCheck),
			IntegrityCheckReason:   stringValue(node.IntegrityCheckReason),
			FirmwareCheck:          enumStringValue(node.FirmwareCheck),
			PublicIP:               stringValue(node.PublicIP),
			PrivateIP:              stringValue(node.PrivateIP),
			LastIntegrityCheckTime: stringValue(node.LastIntegrityCheckTS),
			LastUpdatedTime:        stringValue(node.LastUpdatedTS),
		},
		ComputeZoneID:           stringValue(node.ComputeZoneId),
		NodeGroupID:             stringValue(node.NodeGroupId),
		EnrolledAt:              stringValue(node.EnrolledAt),
		GeoLocation:             geoLocationFromGenerated(node.GeoLocation),
		HealthyComponentCount:   cloneInt(node.HealthyComponentCount),
		DegradedComponentCount:  cloneInt(node.DegradedComponentCount),
		UnhealthyComponentCount: cloneInt(node.UnhealthyComponentCount),
		Resources:               nodeResourcesFromGenerated(node.Resources),
		SystemInfo:              systemInfoFromGenerated(node.SystemInfo),
		Tags:                    cloneStringSlice(node.Tags),
	}
}

// Maps resource API models into SDK values
func nodeResourcesFromGenerated(resources *fleetapi.ModelsNodeResources) *NodeResources {
	if resources == nil {
		return nil
	}
	return &NodeResources{
		CPUInfo:    cpuInfoFromGenerated(resources.CpuInfo),
		DiskInfo:   diskInfoFromGenerated(resources.DiskInfo),
		GPUInfo:    gpuInfoFromGenerated(resources.GpuInfo),
		MemoryInfo: memoryInfoFromGenerated(resources.MemoryInfo),
		NICInfo:    nicInfoFromGenerated(resources.NicInfo),
	}
}

// Maps CPU API models into SDK values
func cpuInfoFromGenerated(info *fleetapi.ModelsCPUInfo) *CPUInfo {
	if info == nil {
		return nil
	}
	return &CPUInfo{
		Architecture: stringValue(info.Architecture),
		LogicalCores: stringValue(info.LogicalCores),
		Manufacturer: stringValue(info.Manufacturer),
		Type:         stringValue(info.Type),
	}
}

// Maps disk API models into SDK values
func diskInfoFromGenerated(info *fleetapi.ModelsDiskInfo) *DiskInfo {
	if info == nil {
		return nil
	}
	out := &DiskInfo{
		ContainerRootDisk: stringValue(info.ContainerRootDisk),
	}
	if info.BlockDevices != nil {
		out.BlockDevices = make([]BlockDevice, 0, len(*info.BlockDevices))
		for _, device := range *info.BlockDevices {
			out.BlockDevices = append(out.BlockDevices, blockDeviceFromGenerated(device))
		}
	}
	return out
}

// Maps block device API models into SDK values
func blockDeviceFromGenerated(device fleetapi.ModelsBlockDevice) BlockDevice {
	return BlockDevice{
		FSType:     stringValue(device.FsType),
		MountPoint: stringValue(device.MountPoint),
		Name:       stringValue(device.Name),
		Parents:    cloneStringSlice(device.Parents),
		PartUUID:   stringValue(device.PartUUID),
		Size:       cloneInt(device.Size),
		Type:       stringValue(device.Type),
		Used:       cloneInt(device.Used),
		WWN:        stringValue(device.Wwn),
	}
}

// Maps GPU API models into SDK values
func gpuInfoFromGenerated(info *fleetapi.ModelsGPUInfo) *GPUInfo {
	if info == nil {
		return nil
	}
	out := &GPUInfo{
		Architecture: stringValue(info.Architecture),
		Manufacturer: stringValue(info.Manufacturer),
		Memory:       stringValue(info.Memory),
		Product:      stringValue(info.Product),
	}
	if info.Gpus != nil {
		out.GPUs = make([]GPUDevice, 0, len(*info.Gpus))
		for _, gpu := range *info.Gpus {
			out.GPUs = append(out.GPUs, gpuDeviceFromGenerated(gpu))
		}
	}
	return out
}

// Maps GPU device API models into SDK values
func gpuDeviceFromGenerated(device fleetapi.ModelsGPUDevice) GPUDevice {
	return GPUDevice{
		BoardID:      cloneInt(device.BoardID),
		BusID:        stringValue(device.BusID),
		ChassisSN:    stringValue(device.ChassisSN),
		GPUIndex:     stringValue(device.GpuIndex),
		MinorID:      stringValue(device.MinorID),
		SerialNumber: stringValue(device.Sn),
		UUID:         stringValue(device.Uuid),
		VBIOSVersion: stringValue(device.VbiosVersion),
	}
}

// Maps memory API models into SDK values
func memoryInfoFromGenerated(info *fleetapi.ModelsMemoryInfo) *MemoryInfo {
	if info == nil {
		return nil
	}
	return &MemoryInfo{
		TotalBytes: stringValue(info.TotalBytes),
	}
}

// Maps NIC API models into SDK values
func nicInfoFromGenerated(info *fleetapi.ModelsNicInfo) *NICInfo {
	if info == nil {
		return nil
	}
	out := &NICInfo{}
	if info.PrivateIPInterfaces != nil {
		out.PrivateIPInterfaces = make([]NICInterface, 0, len(*info.PrivateIPInterfaces))
		for _, iface := range *info.PrivateIPInterfaces {
			out.PrivateIPInterfaces = append(out.PrivateIPInterfaces, nicInterfaceFromGenerated(iface))
		}
	}
	return out
}

// Maps NIC interface API models into SDK values
func nicInterfaceFromGenerated(iface fleetapi.ModelsNicInterface) NICInterface {
	return NICInterface{
		Interface: stringValue(iface.Interface),
		IP:        stringValue(iface.Ip),
		MAC:       stringValue(iface.Mac),
	}
}

// Maps system API models into SDK values
func systemInfoFromGenerated(info *fleetapi.ModelsSystemInfo) *SystemInfo {
	if info == nil {
		return nil
	}
	return &SystemInfo{
		AgentVersion:            stringValue(info.AgentVersion),
		BootID:                  stringValue(info.BootID),
		ContainerRuntimeVersion: stringValue(info.ContainerRuntimeVersion),
		CUDAVersion:             stringValue(info.CudaVersion),
		DCGMVersion:             stringValue(info.DcgmVersion),
		GPUDriverVersion:        stringValue(info.GpuDriverVersion),
		Hostname:                stringValue(info.Hostname),
		KernelVersion:           stringValue(info.KernelVersion),
		OperatingSystem:         stringValue(info.OperatingSystem),
		OSImage:                 stringValue(info.OsImage),
		StartedAt:               stringValue(info.StartedAt),
		SystemUUID:              stringValue(info.SystemUUID),
	}
}

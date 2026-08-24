// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

const (
	NodeViewDetail NodeView = "detail"
	NodeViewBasic  NodeView = "basic"

	NodeAgentTypeInband NodeAgentType = "inband"
	NodeAgentTypeOOB    NodeAgentType = "oob"

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
	NodeSortByBMCHostname         NodeSortBy = "bmcHostname"

	NodeOrderAsc  NodeSortOrder = "asc"
	NodeOrderDesc NodeSortOrder = "desc"
)

// Represents supported response shapes for listing nodes
type NodeView string

// Reports whether the view is accepted by the API
func (view NodeView) Valid() bool {
	return fleetapi.GetV1NodesParamsView(view).Valid()
}

// Represents the agent source used to list or describe nodes
type NodeAgentType string

// Reports whether the agent type is accepted by the API
func (agentType NodeAgentType) Valid() bool {
	return fleetapi.GetV1NodesParamsAgentType(agentType).Valid()
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
	return fleetapi.ModelsIntegrityCheck(check).Valid()
}

// Represents supported firmware check filters for listing nodes
type NodeFirmwareCheck string

// Reports whether the firmware check status is accepted by the API
func (check NodeFirmwareCheck) Valid() bool {
	return fleetapi.ModelsFirmwareCheck(check).Valid()
}

// Represents supported agent status filters for listing nodes
type NodeAgentStatus string

// Reports whether the agent status is accepted by the API
func (status NodeAgentStatus) Valid() bool {
	return fleetapi.ModelsAgentStatus(status).Valid()
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
	AgentType        NodeAgentType
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
	BMCHostname      string
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

// PageInfo reports the pagination envelope of the response.
func (page NodesPage) PageInfo() PageInfo {
	hasMore := page.HasMore
	return PageInfo{
		Page:     page.Page,
		PageSize: page.PageSize,
		Total:    page.Total,
		HasMore:  &hasMore,
		RawJSON:  page.RawJSON,
	}
}

// Represents a node
type Node struct {
	UUID                    string                   `json:"nodeUUID"`
	Hostname                string                   `json:"hostname,omitempty"`
	AgentType               string                   `json:"agentType,omitempty"`
	AgentVersion            string                   `json:"agentVersion,omitempty"`
	BMCHostname             string                   `json:"bmcHostname,omitempty"`
	BMCIP                   string                   `json:"bmcIP,omitempty"`
	ComputeZone             string                   `json:"computeZone,omitempty"`
	NodeGroup               string                   `json:"nodeGroup,omitempty"`
	Health                  string                   `json:"healthStatus,omitempty"`
	GPUType                 string                   `json:"gpuType,omitempty"`
	GPUCount                *int                     `json:"gpuCount,omitempty"`
	AgentStatus             string                   `json:"agentStatus,omitempty"`
	IntegrityCheck          string                   `json:"integrityCheck,omitempty"`
	IntegrityCheckExtraInfo *IntegrityCheckExtraInfo `json:"integrityCheckExtraInfo,omitempty"`
	IntegrityCheckReason    string                   `json:"integrityCheckReason,omitempty"`
	FirmwareCheck           string                   `json:"firmwareCheck,omitempty"`
	GPUDriverVersion        string                   `json:"gpuDriverVersion,omitempty"`
	GPUFirmwareVersions     []GPUFirmwareVersion     `json:"gpuFirmwareVersions,omitempty"`
	KernelVersion           string                   `json:"kernelVersion,omitempty"`
	PublicIP                string                   `json:"publicIP,omitempty"`
	PrivateIP               string                   `json:"privateIP,omitempty"`
	LastIntegrityCheckTime  string                   `json:"lastIntegrityCheckTS,omitempty"`
	LastUpdatedTime         string                   `json:"lastUpdatedTS,omitempty"`
}

// Represents firmware reported by one GPU
type GPUFirmwareVersion struct {
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	GPUIndex        string `json:"gpuIndex,omitempty"`
}

// Represents structured details returned with an integrity check
type IntegrityCheckExtraInfo struct {
	ErrorDetails             string               `json:"errorDetails,omitempty"`
	GPUEvidencePresent       *bool                `json:"gpuEvidencePresent,omitempty"`
	GPUResults               map[string]JWTClaims `json:"gpuResults,omitempty"`
	NRASCallResult           string               `json:"nrasCallResult,omitempty"`
	OverallAttestationResult *bool                `json:"overallAttestationResult,omitempty"`
}

// Represents claims returned for a GPU attestation result
type JWTClaims struct {
	EATNonce                                    string            `json:"eat_nonce,omitempty"`
	ExpiresAt                                   *int              `json:"exp,omitempty"`
	HardwareModel                               string            `json:"hwmodel,omitempty"`
	IssuedAt                                    *int              `json:"iat,omitempty"`
	Issuer                                      string            `json:"iss,omitempty"`
	JWTID                                       string            `json:"jti,omitempty"`
	MeasurementResults                          string            `json:"measres,omitempty"`
	NotBefore                                   *int              `json:"nbf,omitempty"`
	Subject                                     string            `json:"sub,omitempty"`
	UEID                                        string            `json:"ueid,omitempty"`
	NVIDIAAttestationWarning                    string            `json:"x-nvidia-attestation-warning,omitempty"`
	NVIDIAErrorDetails                          []NRASErrorDetail `json:"x-nvidia-error-details,omitempty"`
	NVIDIAGPUArchitectureCheck                  *bool             `json:"x-nvidia-gpu-arch-check,omitempty"`
	NVIDIAGPUAttestationReportCertChain         *CertChainStatus  `json:"x-nvidia-gpu-attestation-report-cert-chain,omitempty"`
	NVIDIAGPUAttestationReportNonceMatch        *bool             `json:"x-nvidia-gpu-attestation-report-nonce-match,omitempty"`
	NVIDIAGPUAttestationReportParsed            *bool             `json:"x-nvidia-gpu-attestation-report-parsed,omitempty"`
	NVIDIAGPUAttestationReportSignatureVerified *bool             `json:"x-nvidia-gpu-attestation-report-signature-verified,omitempty"`
	NVIDIAGPUVBIOSRIMFetched                    *bool             `json:"x-nvidia-gpu-vbios-rim-fetched,omitempty"`
	NVIDIAGPUVBIOSRIMMeasurementsAvailable      *bool             `json:"x-nvidia-gpu-vbios-rim-measurements-available,omitempty"`
	NVIDIAGPUVBIOSVersion                       string            `json:"x-nvidia-gpu-vbios-version,omitempty"`
	NVIDIAMismatchMeasurementRecords            []any             `json:"x-nvidia-mismatch-measurement-records,omitempty"`
	NVIDIAOverallAttestationResult              *bool             `json:"x-nvidia-overall-att-result,omitempty"`
}

// Represents an NRAS error returned in attestation claims
type NRASErrorDetail struct {
	Code        *int   `json:"code,omitempty"`
	Description string `json:"description,omitempty"`
	FieldName   string `json:"fieldName,omitempty"`
	HTTPStatus  string `json:"httpStatus,omitempty"`
	Message     string `json:"message,omitempty"`
}

// Represents certificate-chain validation details returned in attestation claims
type CertChainStatus struct {
	ExpirationDate    string `json:"x-nvidia-cert-expiration-date,omitempty"`
	OCSPNonceMatches  *bool  `json:"x-nvidia-cert-ocsp-nonce-matches,omitempty"`
	OCSPResponseValid *bool  `json:"x-nvidia-cert-ocsp-response-valid,omitempty"`
	OCSPStatus        string `json:"x-nvidia-cert-ocsp-status,omitempty"`
	RevocationReason  string `json:"x-nvidia-cert-revocation-reason,omitempty"`
	Status            string `json:"x-nvidia-cert-status,omitempty"`
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
	OOBInventory            *OOBInventory  `json:"oobInventory,omitempty"`
	Tags                    []string       `json:"tags,omitempty"`
	RawJSON                 []byte         `json:"-"`
}

// Represents request options for describing a node
type DescribeNodeOptions struct {
	AgentType NodeAgentType
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

	view, err := opts.normalize()
	if err != nil {
		return NodesPage{}, err
	}

	params := fleetapi.GetV1NodesParams{
		View:             nodeViewParam(view),
		AgentType:        optionalEnum[fleetapi.GetV1NodesParamsAgentType](opts.AgentType),
		NodeUUIDs:        optionalSlice(opts.NodeUUIDs),
		Hostname:         optionalString(opts.Hostname),
		BmcHostname:      optionalString(opts.BMCHostname),
		ComputeZoneIds:   optionalSlice(opts.ComputeZoneIDs),
		ComputeZoneNames: optionalSlice(opts.ComputeZoneNames),
		NodeGroupIds:     optionalSlice(opts.NodeGroupIDs),
		NodeGroupNames:   optionalSlice(opts.NodeGroupNames),
		GpuTypes:         optionalSlice(opts.GPUTypes),
		GpuCounts:        optionalSlice(opts.GPUCounts),
		PublicIPs:        optionalSlice(opts.PublicIPs),
		PrivateIPs:       optionalSlice(opts.PrivateIPs),
		SortBy:           optionalEnum[fleetapi.GetV1NodesParamsSortBy](opts.SortBy),
		Order:            optionalEnum[fleetapi.GetV1NodesParamsOrder](opts.Order),
		Page:             cloneInt(opts.Page),
		PageSize:         cloneInt(opts.PageSize),
	}
	// Basic view rejects these four filters (validateNodeOptions enforces it),
	// so they are only ever sent for the detail view.
	if view == NodeViewDetail {
		params.HealthStatuses = optionalEnumSlice[fleetapi.GetV1NodesParamsHealthStatuses](opts.HealthStatuses)
		params.AgentStatuses = optionalEnumSlice[fleetapi.ModelsAgentStatus](opts.AgentStatuses)
		params.IntegrityChecks = optionalEnumSlice[fleetapi.ModelsIntegrityCheck](opts.IntegrityChecks)
		params.FirmwareChecks = optionalEnumSlice[fleetapi.ModelsFirmwareCheck](opts.FirmwareChecks)
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
	if opts.AgentType == NodeAgentTypeOOB {
		return decodeOOBDetailNodes(resp.Body)
	}

	return decodeInbandDetailNodes(resp.Body)
}

// Retrieves detail for a single node using the configured API client
func (c *Client) DescribeNode(ctx context.Context, nodeUUID string) (NodeDetails, error) {
	return c.DescribeNodeWithOptions(ctx, nodeUUID, DescribeNodeOptions{})
}

// Retrieves agent-specific detail for a single node using the configured API client
func (c *Client) DescribeNodeWithOptions(
	ctx context.Context,
	nodeUUID string,
	opts DescribeNodeOptions,
) (NodeDetails, error) {
	ctx, cancel := c.requestContext(ctx)
	defer cancel()

	if nodeUUID == "" {
		return NodeDetails{}, fmt.Errorf("node UUID is required")
	}
	if opts.AgentType != "" && !opts.AgentType.Valid() {
		return NodeDetails{}, invalidOption("agentType", "node agent type", string(opts.AgentType), nodeAgentTypeValues)
	}

	params := fleetapi.GetV1NodesNodeUuidParams{
		AgentType: optionalEnum[fleetapi.GetV1NodesNodeUuidParamsAgentType](opts.AgentType),
	}
	resp, err := c.api.GetV1NodesNodeUuidWithResponse(ctx, nodeUUID, &params)
	if err != nil {
		return NodeDetails{}, err
	}
	if resp.StatusCode() != http.StatusOK {
		return NodeDetails{}, newAPIError(resp.StatusCode(), resp.Status(), resp.Body)
	}

	var node NodeDetails
	if opts.AgentType == NodeAgentTypeOOB {
		var data fleetapi.ModelsOobNodeDetailsResponse
		if err := json.Unmarshal(resp.Body, &data); err != nil {
			return NodeDetails{}, err
		}
		node = oobNodeDetailsFromGenerated(data)
	} else {
		var data fleetapi.ModelsInbandNodeDetailsResponse
		if err := json.Unmarshal(resp.Body, &data); err != nil {
			return NodeDetails{}, err
		}
		node = inbandNodeDetailsFromGenerated(data)
	}

	node.RawJSON = append([]byte(nil), resp.Body...)
	return node, nil
}

// The accepted values named in each node option's error
const (
	nodeViewValues           = "basic or detail"
	nodeAgentTypeValues      = "inband or oob"
	nodeHealthValues         = "Healthy, Degraded, Unhealthy, or Unknown"
	nodeAgentStatusValues    = "Online, Offline, or Unknown"
	nodeIntegrityCheckValues = "Verified, Unverified, Degraded, Pending, Unsupported, or Unknown"
	nodeFirmwareCheckValues  = "Passed, Failed, or Unknown"
	nodeOrderValues          = "asc or desc"
)

// Validate reports whether the options describe a request the API accepts.
// ListNodes calls it, and a caller can call it first to reject a bad request
// without opening a connection.
func (opts ListNodesOptions) Validate() error {
	_, err := opts.normalize()
	return err
}

// Defaults an omitted view and checks every option against it
func (opts ListNodesOptions) normalize() (NodeView, error) {
	view := opts.View
	if view == "" {
		view = NodeViewDetail
	} else if !view.Valid() {
		return "", invalidOption("view", "node view", string(view), nodeViewValues)
	}

	if opts.AgentType != "" && !opts.AgentType.Valid() {
		return "", invalidOption("agentType", "node agent type", string(opts.AgentType), nodeAgentTypeValues)
	}
	for _, status := range opts.HealthStatuses {
		if !status.Valid() {
			return "", invalidOption("health", "node health", string(status), nodeHealthValues)
		}
	}
	for _, status := range opts.AgentStatuses {
		if !status.Valid() {
			return "", invalidOption("agentStatus", "node agent status", string(status), nodeAgentStatusValues)
		}
	}
	for _, check := range opts.IntegrityChecks {
		if !check.Valid() {
			return "", invalidOption("integrityCheck", "node verification check", string(check), nodeIntegrityCheckValues)
		}
	}
	for _, check := range opts.FirmwareChecks {
		if !check.Valid() {
			return "", invalidOption("firmwareCheck", "node firmware check", string(check), nodeFirmwareCheckValues)
		}
	}
	for _, count := range opts.GPUCounts {
		if count < 0 {
			return "", invalidOption("gpuCount", "node GPU count", strconv.Itoa(count), "a non-negative integer")
		}
	}
	if opts.SortBy != "" && !opts.SortBy.Valid() {
		return "", invalidOption("sortBy", "node sort", string(opts.SortBy), "")
	}
	if opts.Order != "" && !opts.Order.Valid() {
		return "", invalidOption("order", "node order", string(opts.Order), nodeOrderValues)
	}

	if view == NodeViewBasic {
		if len(opts.HealthStatuses) > 0 || len(opts.AgentStatuses) > 0 || len(opts.IntegrityChecks) > 0 || len(opts.FirmwareChecks) > 0 {
			return "", errors.New("basic node view is incompatible with health, agent-status, verification-check, and firmware-check filters")
		}
		// The rule is stated rather than echoing the rejected value, because
		// the value here is the backend spelling of a sort field a front end
		// may name differently, as the CLI does with integrityCheck.
		if opts.SortBy != "" && !nodeBasicSortCompatible(opts.SortBy) {
			return "", errors.New("basic node view supports sorting only by hostname, nodeUUID, or bmcHostname")
		}
	}

	return view, nil
}

// Reports whether a sort field works with basic view. Basic responses carry
// only the identity columns, so those are the only fields there is anything to
// sort on.
func nodeBasicSortCompatible(sortBy NodeSortBy) bool {
	switch sortBy {
	case NodeSortByHostname, NodeSortByUUID, NodeSortByBMCHostname:
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
func decodeInbandDetailNodes(data []byte) (NodesPage, error) {
	var resp fleetapi.ModelsInbandNodesResponse
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
			page.Nodes = append(page.Nodes, inbandNodeFromGenerated(node))
		}
	}

	return page, nil
}

// Decodes OOB detail responses and preserves the original payload
func decodeOOBDetailNodes(data []byte) (NodesPage, error) {
	var resp fleetapi.ModelsOobNodesResponse
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
			page.Nodes = append(page.Nodes, oobNodeFromGenerated(node))
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
func inbandNodeFromGenerated(node fleetapi.ModelsInbandNode) Node {
	return Node{
		UUID:                    node.NodeUUID,
		Hostname:                stringValue(node.Hostname),
		AgentType:               stringValue(node.AgentType),
		AgentVersion:            stringValue(node.AgentVersion),
		ComputeZone:             stringValue(node.ComputeZone),
		NodeGroup:               stringValue(node.NodeGroup),
		Health:                  enumStringValue(node.HealthStatus),
		GPUType:                 stringValue(node.GpuType),
		GPUCount:                cloneInt(node.GpuCount),
		AgentStatus:             enumStringValue(node.AgentStatus),
		IntegrityCheck:          enumStringValue(node.IntegrityCheck),
		IntegrityCheckExtraInfo: integrityCheckExtraInfoFromGenerated(node.IntegrityCheckExtraInfo),
		IntegrityCheckReason:    stringValue(node.IntegrityCheckReason),
		FirmwareCheck:           enumStringValue(node.FirmwareCheck),
		GPUDriverVersion:        stringValue(node.GpuDriverVersion),
		GPUFirmwareVersions:     gpuFirmwareVersionsFromGenerated(node.GpuFirmwareVersions),
		KernelVersion:           stringValue(node.KernelVersion),
		PublicIP:                stringValue(node.PublicIP),
		PrivateIP:               stringValue(node.PrivateIP),
		LastIntegrityCheckTime:  stringValue(node.LastIntegrityCheckTS),
		LastUpdatedTime:         stringValue(node.LastUpdatedTS),
	}
}

// Maps OOB list API models into SDK values
func oobNodeFromGenerated(node fleetapi.ModelsOobNode) Node {
	return Node{
		UUID:                    node.NodeUUID,
		AgentType:               stringValue(node.AgentType),
		AgentVersion:            stringValue(node.AgentVersion),
		BMCHostname:             stringValue(node.BmcHostname),
		BMCIP:                   stringValue(node.BmcIP),
		ComputeZone:             stringValue(node.ComputeZone),
		NodeGroup:               stringValue(node.NodeGroup),
		Health:                  enumStringValue(node.HealthStatus),
		AgentStatus:             enumStringValue(node.AgentStatus),
		IntegrityCheck:          enumStringValue(node.IntegrityCheck),
		IntegrityCheckExtraInfo: integrityCheckExtraInfoFromGenerated(node.IntegrityCheckExtraInfo),
		IntegrityCheckReason:    stringValue(node.IntegrityCheckReason),
		LastIntegrityCheckTime:  stringValue(node.LastIntegrityCheckTS),
		LastUpdatedTime:         stringValue(node.LastUpdatedTS),
	}
}

// Maps basic API models into SDK values
func nodeFromSimple(node fleetapi.ModelsSimpleNode) Node {
	return Node{
		UUID:        node.NodeUUID,
		Hostname:    stringValue(node.Hostname),
		BMCHostname: stringValue(node.BmcHostname),
		BMCIP:       stringValue(node.BmcIP),
	}
}

// Maps node detail API models into SDK values
func inbandNodeDetailsFromGenerated(node fleetapi.ModelsInbandNodeDetailsResponse) NodeDetails {
	return NodeDetails{
		Node: Node{
			UUID:                    node.NodeUUID,
			Hostname:                stringValue(node.Hostname),
			AgentType:               stringValue(node.AgentType),
			AgentVersion:            stringValue(node.AgentVersion),
			ComputeZone:             stringValue(node.ComputeZone),
			NodeGroup:               stringValue(node.NodeGroup),
			Health:                  enumStringValue(node.HealthStatus),
			GPUType:                 stringValue(node.GpuType),
			GPUCount:                cloneInt(node.GpuCount),
			AgentStatus:             enumStringValue(node.AgentStatus),
			IntegrityCheck:          enumStringValue(node.IntegrityCheck),
			IntegrityCheckExtraInfo: integrityCheckExtraInfoFromGenerated(node.IntegrityCheckExtraInfo),
			IntegrityCheckReason:    stringValue(node.IntegrityCheckReason),
			FirmwareCheck:           enumStringValue(node.FirmwareCheck),
			GPUDriverVersion:        stringValue(node.GpuDriverVersion),
			GPUFirmwareVersions:     gpuFirmwareVersionsFromGenerated(node.GpuFirmwareVersions),
			KernelVersion:           stringValue(node.KernelVersion),
			PublicIP:                stringValue(node.PublicIP),
			PrivateIP:               stringValue(node.PrivateIP),
			LastIntegrityCheckTime:  stringValue(node.LastIntegrityCheckTS),
			LastUpdatedTime:         stringValue(node.LastUpdatedTS),
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

// Maps OOB node detail API models into SDK values
func oobNodeDetailsFromGenerated(node fleetapi.ModelsOobNodeDetailsResponse) NodeDetails {
	return NodeDetails{
		Node: Node{
			UUID:                    node.NodeUUID,
			AgentType:               stringValue(node.AgentType),
			AgentVersion:            stringValue(node.AgentVersion),
			BMCHostname:             stringValue(node.BmcHostname),
			BMCIP:                   stringValue(node.BmcIP),
			ComputeZone:             stringValue(node.ComputeZone),
			NodeGroup:               stringValue(node.NodeGroup),
			Health:                  enumStringValue(node.HealthStatus),
			AgentStatus:             enumStringValue(node.AgentStatus),
			IntegrityCheck:          enumStringValue(node.IntegrityCheck),
			IntegrityCheckExtraInfo: integrityCheckExtraInfoFromGenerated(node.IntegrityCheckExtraInfo),
			IntegrityCheckReason:    stringValue(node.IntegrityCheckReason),
			LastIntegrityCheckTime:  stringValue(node.LastIntegrityCheckTS),
			LastUpdatedTime:         stringValue(node.LastUpdatedTS),
		},
		ComputeZoneID:           stringValue(node.ComputeZoneId),
		NodeGroupID:             stringValue(node.NodeGroupId),
		EnrolledAt:              stringValue(node.EnrolledAt),
		GeoLocation:             geoLocationFromGenerated(node.GeoLocation),
		HealthyComponentCount:   cloneInt(node.HealthyComponentCount),
		DegradedComponentCount:  cloneInt(node.DegradedComponentCount),
		UnhealthyComponentCount: cloneInt(node.UnhealthyComponentCount),
		OOBInventory:            oobInventoryFromGenerated(node.OobInventory),
		Tags:                    cloneStringSlice(node.Tags),
	}
}

func gpuFirmwareVersionsFromGenerated(versions *[]fleetapi.ModelsGPUFirmwareVersion) []GPUFirmwareVersion {
	if versions == nil {
		return nil
	}
	out := make([]GPUFirmwareVersion, 0, len(*versions))
	for _, version := range *versions {
		out = append(out, GPUFirmwareVersion{
			FirmwareVersion: stringValue(version.FirmwareVersion),
			GPUIndex:        stringValue(version.GpuIndex),
		})
	}
	return out
}

func integrityCheckExtraInfoFromGenerated(
	info *fleetapi.ModelsIntegrityCheckExtraInfo,
) *IntegrityCheckExtraInfo {
	if info == nil {
		return nil
	}
	out := &IntegrityCheckExtraInfo{
		ErrorDetails:             stringValue(info.ErrorDetails),
		GPUEvidencePresent:       cloneBool(info.GpuEvidencePresent),
		NRASCallResult:           stringValue(info.NrasCallResult),
		OverallAttestationResult: cloneBool(info.OverallAttestationResult),
	}
	if info.GpuResults != nil {
		out.GPUResults = make(map[string]JWTClaims, len(*info.GpuResults))
		for gpuID, claims := range *info.GpuResults {
			out.GPUResults[gpuID] = jwtClaimsFromGenerated(claims)
		}
	}
	return out
}

func jwtClaimsFromGenerated(claims fleetapi.ModelsJWTClaims) JWTClaims {
	return JWTClaims{
		EATNonce:                             stringValue(claims.EatNonce),
		ExpiresAt:                            cloneInt(claims.Exp),
		HardwareModel:                        stringValue(claims.Hwmodel),
		IssuedAt:                             cloneInt(claims.Iat),
		Issuer:                               stringValue(claims.Iss),
		JWTID:                                stringValue(claims.Jti),
		MeasurementResults:                   stringValue(claims.Measres),
		NotBefore:                            cloneInt(claims.Nbf),
		Subject:                              stringValue(claims.Sub),
		UEID:                                 stringValue(claims.Ueid),
		NVIDIAAttestationWarning:             stringValue(claims.XNvidiaAttestationWarning),
		NVIDIAErrorDetails:                   nrasErrorDetailsFromGenerated(claims.XNvidiaErrorDetails),
		NVIDIAGPUArchitectureCheck:           cloneBool(claims.XNvidiaGpuArchCheck),
		NVIDIAGPUAttestationReportCertChain:  certChainStatusFromGenerated(claims.XNvidiaGpuAttestationReportCertChain),
		NVIDIAGPUAttestationReportNonceMatch: cloneBool(claims.XNvidiaGpuAttestationReportNonceMatch),
		NVIDIAGPUAttestationReportParsed:     cloneBool(claims.XNvidiaGpuAttestationReportParsed),
		NVIDIAGPUAttestationReportSignatureVerified: cloneBool(claims.XNvidiaGpuAttestationReportSignatureVerified),
		NVIDIAGPUVBIOSRIMFetched:                    cloneBool(claims.XNvidiaGpuVbiosRimFetched),
		NVIDIAGPUVBIOSRIMMeasurementsAvailable:      cloneBool(claims.XNvidiaGpuVbiosRimMeasurementsAvailable),
		NVIDIAGPUVBIOSVersion:                       stringValue(claims.XNvidiaGpuVbiosVersion),
		NVIDIAMismatchMeasurementRecords:            cloneAnySlice(claims.XNvidiaMismatchMeasurementRecords),
		NVIDIAOverallAttestationResult:              cloneBool(claims.XNvidiaOverallAttResult),
	}
}

func nrasErrorDetailsFromGenerated(details *[]fleetapi.ModelsNRASErrorDetail) []NRASErrorDetail {
	if details == nil {
		return nil
	}
	out := make([]NRASErrorDetail, 0, len(*details))
	for _, detail := range *details {
		out = append(out, NRASErrorDetail{
			Code:        cloneInt(detail.Code),
			Description: stringValue(detail.Description),
			FieldName:   stringValue(detail.FieldName),
			HTTPStatus:  stringValue(detail.HttpStatus),
			Message:     stringValue(detail.Message),
		})
	}
	return out
}

func certChainStatusFromGenerated(status *fleetapi.ModelsCertChainStatus) *CertChainStatus {
	if status == nil {
		return nil
	}
	return &CertChainStatus{
		ExpirationDate:    stringValue(status.XNvidiaCertExpirationDate),
		OCSPNonceMatches:  cloneBool(status.XNvidiaCertOcspNonceMatches),
		OCSPResponseValid: cloneBool(status.XNvidiaCertOcspResponseValid),
		OCSPStatus:        stringValue(status.XNvidiaCertOcspStatus),
		RevocationReason:  stringValue(status.XNvidiaCertRevocationReason),
		Status:            stringValue(status.XNvidiaCertStatus),
	}
}

func cloneAnySlice(values *[]interface{}) []any {
	if values == nil {
		return nil
	}
	return append([]any(nil), (*values)...)
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

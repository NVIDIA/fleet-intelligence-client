// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Verifies detail list request construction and decoding
func TestListNodesDetailSendsAuthAndParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}

		query := r.URL.Query()
		if got := query.Get("view"); got != "detail" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query["nodeUUIDs"]; !slices.Equal(got, []string{"node-1", "node-2"}) {
			t.Fatalf("unexpected nodeUUIDs: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["healthStatuses"]; !slices.Equal(got, []string{"Healthy", "Degraded"}) {
			t.Fatalf("unexpected healthStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["agentStatuses"]; !slices.Equal(got, []string{"Online"}) {
			t.Fatalf("unexpected agentStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["verificationChecks"]; !slices.Equal(got, []string{"Verified"}) {
			t.Fatalf("unexpected verificationChecks: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["firmwareChecks"]; !slices.Equal(got, []string{"Unknown"}) {
			t.Fatalf("unexpected firmwareChecks: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1", "cz-2"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["computeZoneNames"]; !slices.Equal(got, []string{"East"}) {
			t.Fatalf("unexpected computeZoneNames: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1"}) {
			t.Fatalf("unexpected nodeGroupIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupNames"]; !slices.Equal(got, []string{"Training"}) {
			t.Fatalf("unexpected nodeGroupNames: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["gpuTypes"]; !slices.Equal(got, []string{"NVIDIA-H100"}) {
			t.Fatalf("unexpected gpuTypes: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["gpuCounts"]; !slices.Equal(got, []string{"8", "4"}) {
			t.Fatalf("unexpected gpuCounts: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["publicIPs"]; !slices.Equal(got, []string{"203.0.113.10"}) {
			t.Fatalf("unexpected publicIPs: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["privateIPs"]; !slices.Equal(got, []string{"10.0.0.10"}) {
			t.Fatalf("unexpected privateIPs: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("hostname"); got != "gpu" {
			t.Fatalf("unexpected hostname: %q", got)
		}
		if got := query.Get("sortBy"); got != "healthStatus" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		if got := query.Get("order"); got != "desc" {
			t.Fatalf("unexpected order: %q", got)
		}
		if got := query.Get("page"); got != "2" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := query.Get("pageSize"); got != "50" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","healthStatus":"Healthy","gpuType":"NVIDIA-H100","gpuCount":8,"agentStatus":"Online","verificationCheck":"Verified","firmwareCheck":"Unknown","lastUpdatedTS":"2026-05-01T00:00:00Z"}],"hasMore":true,"page":2,"pageSize":50,"total":99}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	page := 2
	pageSize := 50
	got, err := client.ListNodes(context.Background(), ListNodesOptions{
		View:               NodeViewDetail,
		NodeUUIDs:          []string{"node-1", "node-2"},
		HealthStatuses:     []NodeHealthStatus{NodeHealthHealthy, NodeHealthDegraded},
		ComputeZoneIDs:     []string{"cz-1", "cz-2"},
		ComputeZoneNames:   []string{"East"},
		NodeGroupIDs:       []string{"ng-1"},
		NodeGroupNames:     []string{"Training"},
		GPUTypes:           []string{"NVIDIA-H100"},
		GPUCounts:          []int{8, 4},
		PublicIPs:          []string{"203.0.113.10"},
		PrivateIPs:         []string{"10.0.0.10"},
		Hostname:           "gpu",
		AgentStatuses:      []NodeAgentStatus{NodeAgentOnline},
		VerificationChecks: []NodeVerificationCheck{NodeVerificationVerified},
		FirmwareChecks:     []NodeFirmwareCheck{NodeFirmwareUnknown},
		SortBy:             NodeSortByHealthStatus,
		Order:              NodeOrderDesc,
		Page:               &page,
		PageSize:           &pageSize,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !got.HasMore || got.Page != 2 || got.PageSize != 50 || got.Total != 99 {
		t.Fatalf("unexpected page metadata: %#v", got)
	}
	if len(got.Nodes) != 1 {
		t.Fatalf("unexpected node count: %d", len(got.Nodes))
	}
	node := got.Nodes[0]
	if node.UUID != "node-1" || node.Hostname != "gpu-001" || node.Health != "Healthy" || node.AgentStatus != "Online" {
		t.Fatalf("unexpected node: %#v", node)
	}
	if node.GPUCount == nil || *node.GPUCount != 8 {
		t.Fatalf("unexpected GPU count: %#v", node.GPUCount)
	}
	if !strings.Contains(string(got.RawJSON), `"nodes"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies basic view decoding and filter omission
func TestListNodesBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query["healthStatuses"]; len(got) != 0 {
			t.Fatalf("basic view sent healthStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		// Compute-zone/nodegroup/GPU/IP filters are basic-view compatible per the API.
		if got := query["gpuTypes"]; !slices.Equal(got, []string{"NVIDIA-H100"}) {
			t.Fatalf("basic view dropped gpuTypes: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1"}) {
			t.Fatalf("basic view dropped computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("sortBy"); got != "hostname" {
			t.Fatalf("unexpected sortBy: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","nodeName":"machine-001","bmcMac":"bmc-001","bmcIP":"192.0.2.10:443"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.ListNodes(context.Background(), ListNodesOptions{
		View:           NodeViewBasic,
		SortBy:         NodeSortByHostname,
		GPUTypes:       []string{"NVIDIA-H100"},
		ComputeZoneIDs: []string{"cz-1"},
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].UUID != "node-1" || got.Nodes[0].Hostname != "gpu-001" {
		t.Fatalf("unexpected nodes: %#v", got.Nodes)
	}
	if got.Nodes[0].BMCMAC != "bmc-001" || got.Nodes[0].BMCIP != "192.0.2.10:443" {
		t.Fatalf("unexpected basic BMC fields: %#v", got.Nodes[0])
	}
	if got.Nodes[0].NodeName != "machine-001" {
		t.Fatalf("unexpected basic node name: %#v", got.Nodes[0])
	}
	if got.Nodes[0].GPUCount != nil || got.Nodes[0].Health != "" {
		t.Fatalf("basic view should not set detail fields: %#v", got.Nodes[0])
	}
}

// Verifies detail request construction and nested decoding
func TestDescribeNodeSendsAuthAndDecodesDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/node-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","computeZoneId":"cz-1","nodeGroup":"Training","nodeGroupId":"ng-1","healthStatus":"Degraded","gpuType":"NVIDIA-H100","gpuCount":8,"agentStatus":"Online","verificationCheck":"Unverified","verificationCheckReason":"nonce mismatch","firmwareCheck":"Passed","publicIP":"203.0.113.10","privateIP":"10.0.0.10","healthyComponentCount":3,"degradedComponentCount":1,"unhealthyComponentCount":0,"resources":{"cpuInfo":{"manufacturer":"Intel","type":"Xeon","logicalCores":"96"},"diskInfo":{"containerRootDisk":"/dev/sda1","blockDevices":[{"name":"sda1","mountPoint":"/","fsType":"ext4","parents":["sda"],"size":1024,"used":512,"type":"disk","wwn":"wwn-1"}]},"gpuInfo":{"product":"NVIDIA H100","memory":"80GB","gpus":[{"uuid":"GPU-1"}]},"memoryInfo":{"totalBytes":"1099511627776"},"nicInfo":{"privateIPInterfaces":[{"interface":"eth0","ip":"10.0.0.10","mac":"00:11:22:33:44:55"}]}},"systemInfo":{"agentVersion":"1.2.3","gpuDriverVersion":"550.54.14","cudaVersion":"12.4"},"tags":["prod","h100"]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.DescribeNode(context.Background(), "node-1")
	if err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if got.UUID != "node-1" || got.ComputeZoneID != "cz-1" || got.NodeGroupID != "ng-1" || got.Health != "Degraded" || got.VerificationCheckReason != "nonce mismatch" {
		t.Fatalf("unexpected node details: %#v", got)
	}
	if got.Resources == nil || got.Resources.GPUInfo == nil || len(got.Resources.GPUInfo.GPUs) != 1 || got.Resources.GPUInfo.GPUs[0].UUID != "GPU-1" {
		t.Fatalf("unexpected resources: %#v", got.Resources)
	}
	if got.Resources.DiskInfo == nil || len(got.Resources.DiskInfo.BlockDevices) != 1 || got.Resources.DiskInfo.BlockDevices[0].Name != "sda1" {
		t.Fatalf("unexpected disk info: %#v", got.Resources.DiskInfo)
	}
	if got.Resources.NICInfo == nil || len(got.Resources.NICInfo.PrivateIPInterfaces) != 1 || got.Resources.NICInfo.PrivateIPInterfaces[0].Interface != "eth0" {
		t.Fatalf("unexpected NIC info: %#v", got.Resources.NICInfo)
	}
	if got.SystemInfo == nil || got.SystemInfo.AgentVersion != "1.2.3" || got.SystemInfo.CUDAVersion != "12.4" {
		t.Fatalf("unexpected system info: %#v", got.SystemInfo)
	}
	if !slices.Equal(got.Tags, []string{"prod", "h100"}) {
		t.Fatalf("unexpected tags: %#v", got.Tags)
	}
	if !strings.Contains(string(got.RawJSON), `"nodeUUID"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies OOB detail selection and nested inventory decoding
func TestDescribeNodeOOBDecodesInventory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("dataSource"); got != "oob" {
			t.Fatalf("unexpected dataSource: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nodeUUID":"node-oob-1",
			"hostname":"host-001",
			"nodeName":"machine-001",
			"nodeKind":"nvswitch",
			"dataSource":"oob",
			"bmcMac":"bmc-001",
			"bmcIP":"192.0.2.10",
			"oobInventory":{
				"collectedAt":"2026-07-30T20:00:00Z",
				"schemaVersion":"inventory.v1alpha1",
				"source":{"sourceType":"redfish","vendor":"Dell","redfishVersion":"1.17.0","mac":"02:00:0a:00:00:0a"},
				"systems":[{"id":"System.Embedded.1","model":"PowerEdge XE9680","processorSummary":{"count":2},"memorySummary":{"totalSystemMemoryGiB":2048},"secureBootEnabled":true,"processors":[{"id":"CPU.Socket.1","totalCores":56}]}],
				"managers":[{"id":"iDRAC.Embedded.1","firmwareVersion":"7.10.00.00"}],
				"chassis":[{"id":"System.Embedded.1","pcieDevices":[{"id":"GPU.Slot.1","model":"NVIDIA H100"}]}],
				"firmware":[{"id":"BIOS","name":"System BIOS","serviceId":"fw-service","version":"1.2.3","status":{"state":"Enabled","health":"OK","healthRollup":"Warning"}}],
				"domainErrors":[{"domain":"firmware","message":"partial collection"}]
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	got, err := client.DescribeNodeWithOptions(context.Background(), "node-oob-1", DescribeNodeOptions{
		DataSource: DataSourceOOB,
	})
	if err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if got.DataSource != "oob" || got.NodeName != "machine-001" || got.NodeKind != NodeKindNVSwitch ||
		got.BMCMAC != "bmc-001" || got.BMCIP != "192.0.2.10" {
		t.Fatalf("unexpected OOB node fields: %#v", got.Node)
	}
	if got.OOBInventory == nil || got.OOBInventory.SchemaVersion != "inventory.v1alpha1" {
		t.Fatalf("unexpected inventory: %#v", got.OOBInventory)
	}
	if got.OOBInventory.Source == nil || got.OOBInventory.Source.MAC != "02:00:0a:00:00:0a" {
		t.Fatalf("unexpected inventory source: %#v", got.OOBInventory.Source)
	}
	if len(got.OOBInventory.Systems) != 1 || got.OOBInventory.Systems[0].ProcessorSummary == nil || got.OOBInventory.Systems[0].ProcessorSummary.Count == nil ||
		*got.OOBInventory.Systems[0].ProcessorSummary.Count != 2 {
		t.Fatalf("unexpected systems: %#v", got.OOBInventory.Systems)
	}
	if len(got.OOBInventory.Chassis) != 1 || len(got.OOBInventory.Chassis[0].PCIeDevices) != 1 {
		t.Fatalf("unexpected chassis: %#v", got.OOBInventory.Chassis)
	}
	if len(got.OOBInventory.Firmware) != 1 || got.OOBInventory.Firmware[0].ServiceID != "fw-service" ||
		got.OOBInventory.Firmware[0].Status == nil || got.OOBInventory.Firmware[0].Status.State != "Enabled" || got.OOBInventory.Firmware[0].Status.Health != "OK" ||
		got.OOBInventory.Firmware[0].Status.HealthRollup != "Warning" {
		t.Fatalf("unexpected firmware: %#v", got.OOBInventory.Firmware)
	}
}

// Verifies the OOB node list query and BMC fields
func TestListNodesOOB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("dataSource") != "oob" || query.Get("nodeName") != "machine" || query.Get("bmcMac") != "bmc" || query.Get("sortBy") != "nodeName" {
			t.Fatalf("unexpected query: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-oob-1","hostname":"host-001","nodeName":"machine-001","nodeKind":"nvswitch","dataSource":"oob","bmcMac":"bmc-001","bmcIP":"192.0.2.10"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	got, err := client.ListNodes(context.Background(), ListNodesOptions{
		DataSource: DataSourceOOB,
		NodeName:   "machine",
		BMCMAC:     "bmc",
		SortBy:     NodeSortByNodeName,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(got.Nodes) != 1 || got.Nodes[0].NodeName != "machine-001" || got.Nodes[0].NodeKind != NodeKindNVSwitch ||
		got.Nodes[0].BMCMAC != "bmc-001" || got.Nodes[0].BMCIP != "192.0.2.10" {
		t.Fatalf("unexpected OOB nodes: %#v", got.Nodes)
	}
}

// Verifies API errors are structured
func TestListNodesReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid filter parameters","details":"bad node uuid"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListNodes(context.Background(), ListNodesOptions{})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "400 Bad Request") || !strings.Contains(err.Error(), "bad node uuid") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies describe API errors are structured
func TestDescribeNodeReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Not found","details":"node does not exist"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.DescribeNode(context.Background(), "node-1")
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "404 Not Found") || !strings.Contains(err.Error(), "node does not exist") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies local option validation
func TestListNodesRejectsInvalidOptions(t *testing.T) {
	client, err := NewClient("https://fleet.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	tests := []struct {
		name string
		opts ListNodesOptions
		want string
	}{
		{name: "view", opts: ListNodesOptions{View: "wide"}, want: "invalid node view"},
		{name: "data source", opts: ListNodesOptions{DataSource: "sideband"}, want: "invalid data source"},
		{name: "health", opts: ListNodesOptions{HealthStatuses: []NodeHealthStatus{"Broken"}}, want: "invalid node health"},
		{name: "agent", opts: ListNodesOptions{AgentStatuses: []NodeAgentStatus{"Missing"}}, want: "invalid node agent status"},
		{name: "verification", opts: ListNodesOptions{VerificationChecks: []NodeVerificationCheck{"Missing"}}, want: "invalid node verification check"},
		{name: "firmware", opts: ListNodesOptions{FirmwareChecks: []NodeFirmwareCheck{"Missing"}}, want: "invalid node firmware check"},
		{name: "gpu count", opts: ListNodesOptions{GPUCounts: []int{8, -1}}, want: "invalid node GPU count"},
		{name: "sort", opts: ListNodesOptions{SortBy: "bad"}, want: "invalid node sort"},
		{name: "order", opts: ListNodesOptions{Order: "up"}, want: "invalid node order"},
		{name: "basic health", opts: ListNodesOptions{View: NodeViewBasic, HealthStatuses: []NodeHealthStatus{NodeHealthHealthy}}, want: "basic node view is incompatible"},
		{name: "basic sort", opts: ListNodesOptions{View: NodeViewBasic, SortBy: NodeSortByHealthStatus}, want: "basic node view supports sorting only by"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ListNodes(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies supported verification check filters match the API vocabulary
func TestNodeVerificationCheckValid(t *testing.T) {
	tests := []NodeVerificationCheck{
		NodeVerificationVerified,
		NodeVerificationUnverified,
		NodeVerificationDegraded,
		NodeVerificationPending,
		NodeVerificationUnsupported,
		NodeVerificationUnknown,
		NodeVerificationPassed,
		NodeVerificationFailed,
	}

	for _, check := range tests {
		if !check.Valid() {
			t.Fatalf("expected %q to be valid", check)
		}
	}
}

// Verifies supported sort fields match the API vocabulary
func TestNodeSortByValid(t *testing.T) {
	tests := []NodeSortBy{
		NodeSortByHostname,
		NodeSortByUUID,
		NodeSortByHealthStatus,
		NodeSortByNodeGroup,
		NodeSortByComputeZone,
		NodeSortByGPUType,
		NodeSortByGPUCount,
		NodeSortByVerificationCheck,
		NodeSortByAgentStatus,
		NodeSortByAgentVersion,
		NodeSortByKernelVersion,
		NodeSortByGPUDriverVersion,
		NodeSortByGPUFirmwareVersions,
		NodeSortByNodeName,
		NodeSortByBMCMAC,
	}

	for _, sortBy := range tests {
		if !sortBy.Valid() {
			t.Fatalf("expected %q to be valid", sortBy)
		}
	}
}

// Verifies describe validation
func TestDescribeNodeRejectsMissingUUID(t *testing.T) {
	client, err := NewClient("https://fleet.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	_, err = client.DescribeNode(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "node UUID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies the options validate without a client, so a front end can reject a
// bad request before resolving credentials
func TestListNodesOptionsValidate(t *testing.T) {
	tests := []struct {
		name string
		opts ListNodesOptions
		want string
	}{
		{name: "valid", opts: ListNodesOptions{HealthStatuses: []NodeHealthStatus{NodeHealthHealthy}}},
		{name: "view", opts: ListNodesOptions{View: "wide"}, want: "invalid node view"},
		{name: "health", opts: ListNodesOptions{HealthStatuses: []NodeHealthStatus{"Broken"}}, want: "invalid node health"},
		{name: "gpu count", opts: ListNodesOptions{GPUCounts: []int{-1}}, want: "invalid node GPU count"},
		{
			name: "basic filter",
			opts: ListNodesOptions{View: NodeViewBasic, HealthStatuses: []NodeHealthStatus{NodeHealthHealthy}},
			want: "basic node view is incompatible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want %q", err, tt.want)
			}
		})
	}
}

// Verifies basic view accepts identity and inventory sorting, but not health sorting.
func TestListNodesOptionsBasicSortCompatibility(t *testing.T) {
	accepted := []NodeSortBy{NodeSortByHostname, NodeSortByNodeName, NodeSortByUUID, NodeSortByBMCMAC,
		NodeSortByGPUType, NodeSortByGPUCount, NodeSortByNodeGroup, NodeSortByComputeZone}
	rejected := []NodeSortBy{
		NodeSortByHealthStatus, NodeSortByVerificationCheck, NodeSortByAgentStatus,
	}

	for _, sortBy := range accepted {
		if err := (ListNodesOptions{View: NodeViewBasic, SortBy: sortBy}).Validate(); err != nil {
			t.Fatalf("sort %q should be accepted by basic view: %v", sortBy, err)
		}
	}
	for _, sortBy := range rejected {
		err := (ListNodesOptions{View: NodeViewBasic, SortBy: sortBy}).Validate()
		if err == nil || !strings.Contains(err.Error(), "basic node view supports sorting only by") {
			t.Fatalf("sort %q should be rejected by basic view, got %v", sortBy, err)
		}
		// A front end may spell the field differently, so the message states
		// the rule instead of quoting back the backend name.
		if strings.Contains(err.Error(), string(sortBy)) {
			t.Fatalf("sort %q should not appear in the rejection: %v", sortBy, err)
		}
	}
}

// Verifies a rejected value carries the structured detail a front end needs to
// re-render the message against its own name for the option
func TestInvalidOptionErrorCarriesOption(t *testing.T) {
	err := (ListNodesOptions{VerificationChecks: []NodeVerificationCheck{"Missing"}}).Validate()

	var optionErr *InvalidOptionError
	if !errors.As(err, &optionErr) {
		t.Fatalf("expected InvalidOptionError, got %T", err)
	}
	if optionErr.Option != "verificationCheck" || optionErr.Value != "Missing" {
		t.Fatalf("unexpected option error: %#v", optionErr)
	}
	if !strings.Contains(optionErr.Expected, "Verified") {
		t.Fatalf("expected accepted values, got %q", optionErr.Expected)
	}
}

func TestNodeUUIDFilters(t *testing.T) {
	for _, view := range []NodeView{NodeViewBasic, NodeViewDetail} {
		for _, source := range []DataSource{"", DataSourceInband, DataSourceOOB} {
			for _, populated := range []bool{false, true} {
				t.Run(string(view)+"/"+string(source)+"/"+map[bool]string{false: "omitted", true: "filtered"}[populated], func(t *testing.T) {
					opts := ListNodesOptions{View: view, DataSource: source}
					if populated {
						opts.SystemUUIDs = []string{"system-A", "system-B"}
						if source != DataSourceOOB {
							opts.AgentUUIDs = []string{"agent-A", "agent-B"}
						}
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						q := r.URL.Query()
						if !slices.Equal(q["agentUUIDs"], opts.AgentUUIDs) || !slices.Equal(q["systemUUIDs"], opts.SystemUUIDs) {
							t.Errorf("incorrect repeated UUID filters: %s", r.URL)
						}
						if q.Get("dataSource") != string(source) || q.Get("view") != string(view) {
							t.Errorf("incorrect scope: %s", r.URL)
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","agentUUID":"agent-A"}],"total":1}`))
					}))
					defer server.Close()
					client, err := NewClient(server.URL, "test-key")
					if err != nil {
						t.Fatal(err)
					}
					page, err := client.ListNodes(context.Background(), opts)
					if err != nil {
						t.Fatal(err)
					}
					if source != DataSourceOOB && (len(page.Nodes) != 1 || page.Nodes[0].AgentUUID != "agent-A") {
						t.Fatalf("missing agent identity: %#v", page.Nodes)
					}
					if !strings.Contains(string(page.RawJSON), `"agentUUID":"agent-A"`) {
						t.Fatal("raw JSON changed")
					}
				})
			}
		}
	}
	for _, view := range []NodeView{NodeViewBasic, NodeViewDetail} {
		if err := (ListNodesOptions{View: view, DataSource: DataSourceOOB, AgentUUIDs: []string{"agent-A"}}).Validate(); err == nil {
			t.Fatal("OOB agent UUID filter accepted")
		}
	}
}

func TestDescribeNodeUUIDIdentities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","agentUUID":"agent-1","systemInfo":{"systemUUID":"system-1"},"resources":{"gpuInfo":{"gpus":[{"uuid":"gpu-1","cliqueID":0,"clusterUUID":"cluster-1"}]}}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	node, err := client.DescribeNode(context.Background(), "node-1")
	if err != nil {
		t.Fatal(err)
	}
	if node.AgentUUID != "agent-1" || node.SystemInfo == nil || node.SystemInfo.SystemUUID != "system-1" {
		t.Fatalf("missing node identities: %#v", node)
	}
	if node.Resources == nil || node.Resources.GPUInfo == nil || len(node.Resources.GPUInfo.GPUs) != 1 {
		t.Fatalf("missing GPUs: %#v", node.Resources)
	}
	gpu := node.Resources.GPUInfo.GPUs[0]
	if gpu.UUID != "gpu-1" || gpu.ClusterUUID != "cluster-1" || gpu.CliqueID == nil || *gpu.CliqueID != 0 {
		t.Fatalf("missing GPU identity: %#v", gpu)
	}
}

func TestVerificationPolicyAndEAR(t *testing.T) {
	var source fleetapi.ModelsVerificationCheckExtraInfo
	payload := []byte(`{"overallAttestationResult":false,"policy":{"approved":false,"tier":3,"reason":"verification rejected","policyVersion":"v1","denials":["measurement mismatch"],"gpus":{"gpu-1":{"approved":false,"tier":3,"reason":"mismatch","denials":["digest mismatch"]},"gpu-2":{}}},"ear":{"submods":{"gpu-1":{"status":"failed","scores":[0,false,null]}}}}`)
	if err := json.Unmarshal(payload, &source); err != nil {
		t.Fatal(err)
	}
	got := verificationCheckExtraInfoFromGenerated(&source)
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var wantJSON, gotJSON any
	if err := json.Unmarshal(payload, &wantJSON); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &gotJSON); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatalf("verification fields changed: %s", data)
	}
	if got.Policy.Tier == nil || *got.Policy.Tier != VerificationPolicyTierRejected || got.Policy.Approved == nil || *got.Policy.Approved {
		t.Fatalf("lost false or tier: %#v", got.Policy)
	}
	(*source.Policy.Denials)[0] = "changed"
	verdict := (*source.Policy.Gpus)["gpu-1"]
	(*verdict.Denials)[0] = "changed"
	*verdict.Approved = true
	(*source.Ear)["submods"].(map[string]any)["gpu-1"].(map[string]any)["scores"].([]any)[0] = 99
	cloned, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(cloned) != string(data) {
		t.Fatal("SDK verification data aliases the generated response")
	}
	if got := verificationCheckExtraInfoFromGenerated(&fleetapi.ModelsVerificationCheckExtraInfo{}); got.Policy != nil || got.EAR != nil {
		t.Fatal("absent policy/EAR acquired values")
	}
	if verificationCheckExtraInfoFromGenerated(nil) != nil {
		t.Fatal("absent extra info acquired values")
	}
}

func TestVerificationStatusConstants(t *testing.T) {
	for _, status := range []NodeVerificationCheck{NodeVerificationFullAttestation, NodeVerificationPartialAttestation, NodeVerificationVerificationFailed} {
		if !status.Valid() {
			t.Fatalf("invalid new status: %s", status)
		}
		for _, source := range []DataSource{DataSourceInband, DataSourceOOB} {
			if err := (ListNodesOptions{DataSource: source, VerificationChecks: []NodeVerificationCheck{status}}).Validate(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if NodeVerificationFailed != NodeVerificationUnverified {
		t.Fatal("legacy alias changed meaning")
	}
}

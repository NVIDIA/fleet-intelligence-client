// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
	"github.com/spf13/cobra"
)

const completeOOBNodeJSON = `{
 "nodeUUID":"node-1", "dataSource":"oob", "nodeKind":"nvswitch",
 "oobInventory":{
  "schemaVersion":"inventory.v1alpha1", "collectedAt":"2026-10-28T12:00:00Z",
  "source":{"sourceType":"redfish","address":"192.0.2.10","mac":"02:00:0a:00:00:0a"},
  "systems":[{
   "id":"system-1","status":{"state":"Enabled","health":"Critical","healthRollup":"Warning"},
   "memorySummary":{"totalSystemMemoryGiB":0},
   "processorSummary":{"count":0,"coreCount":0,"model":"Grace"},
   "processors":[{"id":"gpu-1","processorType":"GPU","status":{"health":"Warning"},"nvidiaGpu":{"totalNvLinks":18,"migModeEnabled":false,"nvlinkTopology":{"hostId":0,"ibGuid":"0x1234","traySerialNumber":"NA"}},"location":{"partLocation":{"serviceLabel":"GPU slot 1"}}},
    {"id":"cpu-1","processorType":"CPU","status":{"health":"Warning"}}],
   "memory":[{"id":"dimm-1","systemId":"system-1","deviceLocator":"DIMM A1","capacityMib":65536,"status":{"health":"OK"},"location":{"partLocation":{"serviceLabel":"memory slot 1"}}}],
   "storage":[{"id":"storage-1","systemId":"system-1","drives":[{"id":"drive-1","storageId":"storage-1","systemId":"system-1","capacityBytes":4000000000000,"protocol":"NVMe","status":{"health":"OK"}}]}],
   "links":{"Chassis":[{"odataId":"/redfish/v1/Chassis/chassis-1"}]}
  }],
  "managers":[{"id":"manager-1","status":{"health":"OK"}}],
  "chassis":[{
   "id":"chassis-1","status":{"health":"Warning"},
   "location":{"placement":{"rack":"R01","rackOffset":0,"rackOffsetUnits":"EIA_310"},"partLocation":{"serviceLabel":"compute tray"}},
   "pcieDevices":[{"id":"pcie-1","status":{"health":"OK"}}],
   "powerSupplies":[{"id":"psu-1","chassisId":"chassis-1","powerCapacityWatts":2400,"status":{"health":"OK"}}]
  }],
  "fabrics":[{"id":"fabric-1","name":"NVLink Fabric","fabricType":"NVLink","status":{"health":"OK"},"switches":[
   {"id":"switch-1","name":"NVSwitch","status":{"health":"Warning"},"location":{"placement":{"rack":"R02","rackOffset":10}},"nvidiaSwitch":{"pcieReferenceClockEnabled":false,"deviceId":"0x1234"}}
  ]}],
  "firmware":[{"id":"firmware-1","name":"Firmware","serviceId":"service-1","status":{"health":"OK"},"relatedItem":[{"odataId":"/redfish/v1/Systems/system-1"}]}],
  "futureField":{"preserved":true}
 }
}`

// newRootCmd builds a root command carrying only this package's commands, so
// the tests drive them through the same argument path a user types.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nvfleetint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(NewCmd())
	return root
}

// Verifies local output flags and sort field pass-through
func TestNodeListLocalJSONAndSort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	raw := `{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","healthStatus":"Healthy"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`
	// JSON output presents the page 1-based, which re-serializes top-level keys.
	want := `{"hasMore":false,"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","healthStatus":"Healthy"}],"page":1,"pageSize":20,"total":1}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("sortBy"); got != "healthStatus" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"node", "list", "--data-source", "inband", "--output", "json", "--sort-by", "healthStatus",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != want {
		t.Fatalf("unexpected JSON: got %q want %q", strings.TrimSpace(out.String()), want)
	}
}

// Verifies detail-only sort fields pass through to the API unchanged
func TestNodeListDetailSortFields(t *testing.T) {
	for _, field := range []string{"agentVersion", "kernelVersion", "gpuDriverVersion", "gpuFirmwareVersions"} {
		t.Run(field, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("sortBy"); got != field {
					t.Fatalf("unexpected sortBy: got %q want %q", got, field)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"nodes":[],"hasMore":false,"page":0,"pageSize":20,"total":0}`))
			}))
			defer server.Close()

			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{
				"node", "list", "--data-source", "inband", "--output", "json", "--sort-by", field,
			})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
		})
	}
}

// Verifies the verificationCheck sort field reaches the API unchanged, and that
// the retired "integrityCheck" spelling is never sent in its place
func TestNodeListSortVerificationCheck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sortBy"); got != "verificationCheck" {
			t.Fatalf("unexpected sortBy: got %q want %q", got, "verificationCheck")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[],"hasMore":false,"page":0,"pageSize":20,"total":0}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--data-source", "inband", "--output", "json", "--sort-by", "verificationCheck"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

// Verifies node list help and sort errors name "verificationCheck" and never
// the retired "integrityCheck" alias
func TestNodeListSortHelpUsesVerificationCheck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	usage := newNodeListCmd().Flags().Lookup("sort-by").Usage
	if !strings.Contains(usage, "verificationCheck") {
		t.Fatalf("sort-by usage missing verificationCheck: %q", usage)
	}
	if strings.Contains(usage, "integrityCheck") {
		t.Fatalf("sort-by usage still mentions integrityCheck: %q", usage)
	}

	_, err := nodeListOptions(nodeListFlags{view: "detail", sortBy: "bogus"})
	if err == nil {
		t.Fatal("expected invalid sort-by error")
	}
	if !strings.Contains(err.Error(), "verificationCheck") || strings.Contains(err.Error(), "integrityCheck") {
		t.Fatalf("unexpected sort-by error: %v", err)
	}

	_, err = nodeListOptions(nodeListFlags{view: "basic", sortBy: "verificationCheck"})
	if err == nil {
		t.Fatal("expected basic view sort error")
	}
	if strings.Contains(err.Error(), "integrityCheck") {
		t.Fatalf("basic view sort error mentions integrityCheck: %v", err)
	}
}

func TestNodeListOptionsSupportsNodeName(t *testing.T) {
	opts, err := nodeListOptions(nodeListFlags{
		view:     "detail",
		nodeName: " machine-001 ",
		sortBy:   "nodeName",
	})
	if err != nil {
		t.Fatalf("node list options failed: %v", err)
	}
	if opts.NodeName != "machine-001" || opts.SortBy != nvfleetint.NodeSortByNodeName {
		t.Fatalf("unexpected node-name options: %#v", opts)
	}
}

// Verifies table output and filter translation
func TestNodeListTableFiltersAndSort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
		if got := query.Get("hostname"); got != "gpu" {
			t.Fatalf("unexpected hostname: %q", got)
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
		if got := query.Get("sortBy"); got != "computezone" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		if got := query.Get("order"); got != "desc" {
			t.Fatalf("unexpected order: %q", got)
		}
		if got := query.Get("pageSize"); got != "10" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","healthStatus":"Healthy","gpuType":"NVIDIA-H100","gpuCount":8,"verificationCheck":"Verified","firmwareCheck":"Unknown","agentStatus":"Online"}],"hasMore":false,"page":0,"pageSize":10,"total":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--data-source", "inband", "--node-uuids", "node-1,node-2", "--health", "Healthy,Degraded", "--hostname", "gpu", "--agent-status", "Online", "--verification-check", "Verified", "--firmware-check", "Unknown", "--compute-zone-ids", "cz-1,cz-2", "--compute-zone-names", "East", "--nodegroup-ids", "ng-1", "--nodegroup-names", "Training", "--gpu-type", "NVIDIA-H100", "--gpu-count", "8,4", "--public-ip", "203.0.113.10", "--private-ip", "10.0.0.10", "--sort-by", "computezone", "--order", "desc", "--page-size", "10"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "GPU TYPE", "ATTESTATION", "FIRMWARE CHECK", "AGENT STATUS", "node-1", "gpu-001", "East", "Training", "Verified", "Unknown", "Online", "Page: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies detail lists combine independent in-band and OOB API views by default
func TestNodeListCombinedDetailViews(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("view"); got != "detail" {
			t.Errorf("unexpected view: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("dataSource") {
		case "inband":
			_, _ = w.Write([]byte(`{
				"nodes":[{"nodeUUID":"node-inband","hostname":"gpu-001","dataSource":"inband","healthStatus":"Healthy"}],
				"hasMore":false,"page":0,"pageSize":20,"total":1
			}`))
		case "oob":
			_, _ = w.Write([]byte(`{
				"nodes":[{"nodeUUID":"node-oob","nodeName":"machine-001","nodeKind":"nvswitch","dataSource":"oob","bmcMac":"bmc-001","bmcIP":"192.0.2.10:443","healthStatus":"Degraded"}],
				"hasMore":false,"page":0,"pageSize":20,"total":1
			}`))
		default:
			t.Errorf("unexpected dataSource: %q", r.URL.Query().Get("dataSource"))
			http.Error(w, "missing dataSource", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined table command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"In-band", "node-inband", "gpu-001", "GPU TYPE",
		"Out-of-band", "node-oob", "machine-001", "NODE NAME", "nvswitch", "NODE KIND", "bmc-001", "BMC MAC",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("combined table missing %q: %q", want, got)
		}
	}

}

// Verifies all-page JSON output
func TestNodeListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := r.URL.Query().Get("pageSize"); got != "1" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2","hostname":"gpu-002"}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--view", "basic", "--all", "--output", "json", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			Page         int  `json:"page"`
			PageSize     int  `json:"pageSize"`
			Total        int  `json:"total"`
			HasMore      bool `json:"hasMore"`
			PagesFetched int  `json:"pagesFetched"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if requests != 2 {
		t.Fatalf("unexpected request count: %d", requests)
	}
	if len(got.Items) != 2 || got.Items[0]["nodeUUID"] != "node-1" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 1 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies --all defaults the page size to the max when --page-size is omitted
func TestNodeListAllDefaultsPageSize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("pageSize"); got != "100" {
			t.Fatalf("unexpected pageSize: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1"}],"hasMore":false,"page":0,"pageSize":100,"total":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--data-source", "inband", "--all", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

// Verifies --all retries only a transiently failing page instead of restarting
// pagination and duplicating already collected nodes.
func TestNodeListAllRetriesFailedPage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var firstPageCalls atomic.Int32
	var secondPageCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "0":
			firstPageCalls.Add(1)
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1"}],"hasMore":true,"page":0,"pageSize":100,"total":2}`))
		case "1":
			if secondPageCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
				return
			}
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2"}],"hasMore":false,"page":1,"pageSize":100,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"node", "list", "--data-source", "inband", "--all", "--output", "json", "--timeout", "5s",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	var got struct {
		Items []struct {
			NodeUUID string `json:"nodeUUID"`
		} `json:"items"`
		Pagination struct {
			PagesFetched int `json:"pagesFetched"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(got.Items) != 2 ||
		got.Items[0].NodeUUID != "node-1" ||
		got.Items[1].NodeUUID != "node-2" ||
		got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected merged output: %#v", got)
	}
	if firstPageCalls.Load() != 1 || secondPageCalls.Load() != 2 {
		t.Fatalf("unexpected page calls: first=%d second=%d", firstPageCalls.Load(), secondPageCalls.Load())
	}
}

// Verifies node describe table output
func TestNodeDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/node-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","computeZoneId":"cz-1","nodeGroup":"Training","nodeGroupId":"ng-1","healthStatus":"Degraded","gpuType":"NVIDIA-H100","gpuCount":8,"agentStatus":"Online","resources":{"gpuInfo":{"product":"NVIDIA H100","gpus":[{"uuid":"GPU-1"}]}},"systemInfo":{"agentVersion":"1.2.3","cudaVersion":"12.4"},"tags":["prod","h100"]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1", "--data-source", "inband"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"FIELD", "VALUE", "UUID", "node-1", "COMPUTE ZONE", "East (cz-1)", "GPU DEVICES", "1", "CUDA", "12.4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies node describe combines independent in-band and OOB views by default
func TestNodeDescribeCombinedViews(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("dataSource") {
		case "inband":
			_, _ = w.Write([]byte(`{
				"nodeUUID":"node-1",
				"hostname":"gpu-001",
				"dataSource":"inband",
				"healthStatus":"Healthy",
				"agentStatus":"Online"
			}`))
		case "oob":
			_, _ = w.Write([]byte(`{
				"nodeUUID":"node-1",
				"dataSource":"oob",
				"bmcMac":"bmc-001",
				"bmcIP":"192.0.2.10:443",
				"healthStatus":"Degraded",
				"oobInventory":{
					"schemaVersion":"inventory.v1alpha1",
					"source":{"sourceType":"redfish","vendor":"NVIDIA"}
				}
			}`))
		default:
			t.Errorf("unexpected dataSource: %q", r.URL.Query().Get("dataSource"))
			http.Error(w, "missing dataSource", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined table command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"In-band", "gpu-001", "Online",
		"Out-of-band", "bmc-001", "192.0.2.10:443",
		"INVENTORY SCHEMA VERSION", "inventory.v1alpha1",
		"SOURCE TYPE", "redfish", "SOURCE VENDOR", "NVIDIA",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("combined output missing %q: %q", want, got)
		}
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined JSON command failed: %v", err)
	}

	var combined struct {
		Inband map[string]any `json:"inband"`
		OOB    map[string]any `json:"oob"`
	}
	if err := json.Unmarshal(out.Bytes(), &combined); err != nil {
		t.Fatalf("decode combined JSON: %v", err)
	}
	if combined.Inband["hostname"] != "gpu-001" {
		t.Fatalf("unexpected in-band JSON: %#v", combined.Inband)
	}
	if combined.OOB["bmcMac"] != "bmc-001" {
		t.Fatalf("unexpected OOB JSON: %#v", combined.OOB)
	}
}

// Verifies a missing agent-specific view does not hide the available view
func TestNodeDescribeCombinedViewAllowsNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataSource") == "oob" {
			http.Error(w, `{"error":"node does not have an OOB agent"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nodeUUID":"node-1",
			"hostname":"gpu-001",
			"dataSource":"inband"
		}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined command failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "In-band") || !strings.Contains(got, "gpu-001") {
		t.Fatalf("available in-band view is missing: %q", got)
	}
	if strings.Contains(got, "Out-of-band") {
		t.Fatalf("missing OOB view should not be rendered: %q", got)
	}
}

// Verifies OOB node describe defaults to a summary and supports domain drill-downs
func TestNodeDescribeOOBTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("dataSource"); got != "oob" {
			t.Fatalf("unexpected dataSource: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nodeUUID":"node-oob-1",
			"nodeKind":"nvswitch",
			"hostname":"host-001",
			"dataSource":"oob",
			"bmcMac":"bmc-001",
			"bmcIP":"192.0.2.10",
			"oobInventory":{
				"collectedAt":"2026-07-30T20:00:00Z",
				"schemaVersion":"inventory.v1alpha1",
				"source":{"sourceType":"redfish","vendor":"Dell","address":"192.0.2.10:443","mac":"00:11:22:33:44:55","redfishVersion":"1.17.0"},
				"systems":[{"id":"System.Embedded.1","uuid":"system-uuid","manufacturer":"Dell","model":"PowerEdge XE9680","sku":"sku-1","serialNumber":"serial-1","biosVersion":"2.1.0","hostName":"host-001","assetTag":"asset-1","powerState":"on","status":{"state":"Enabled","health":"Critical","healthRollup":"Warning","conditions":[{"messageId":"ResourceEvent.1.0.ResourceErrorsDetected","severity":"Critical","message":"nested status condition"}]},"processorSummary":{"count":2},"memorySummary":{"totalSystemMemoryGiB":2048},"secureBootEnabled":true,"processors":[{"id":"CPU.Socket.1","socket":"CPU 1","processorType":"cpu","processorArchitecture":"x86","manufacturer":"Intel","model":"Xeon","maxSpeedMhz":3800,"totalCores":56,"totalThreads":112,"status":{"state":"Enabled","health":"OK","healthRollup":"OK"}},{"id":"GPU.Slot.1","processorType":"gpu","manufacturer":"NVIDIA","model":"NVIDIA H100","status":{"state":"Enabled","health":"OK","healthRollup":"OK"}}]}],
				"managers":[{"id":"iDRAC.Embedded.1","uuid":"manager-uuid","model":"iDRAC","managerType":"bmc","firmwareVersion":"7.10.00.00","status":{"state":"Enabled","health":"OK","healthRollup":"OK"}}],
				"chassis":[{"id":"System.Embedded.1","chassisType":"rack_mount","manufacturer":"Dell","model":"XE9680","sku":"sku-1","serialNumber":"serial-1","partNumber":"part-1","assetTag":"asset-1","powerState":"on","status":{"state":"Enabled","health":"OK","healthRollup":"Warning"},"pcieDevices":[{"id":"GPU.Slot.1","uuid":"gpu-uuid","deviceType":"single_function","manufacturer":"NVIDIA","model":"NVIDIA H100","sku":"gpu-sku","serialNumber":"gpu-serial","partNumber":"gpu-part","firmwareVersion":"96.00.00","status":{"state":"Enabled","health":"OK","healthRollup":"OK"}}]}],
				"firmware":[{"id":"BIOS","name":"System BIOS","serviceId":"fw-service","version":"1.2.3","releaseDate":"2026-01-01","status":{"state":"Enabled","health":"OK","healthRollup":"Warning"}}],
				"domainErrors":[{"domain":"storage","resource":"Disk.1","message":"collection failed"}]
			}
		}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-oob-1", "--data-source", "oob"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"FIELD", "VALUE",
		"BMC MAC", "bmc-001",
		"INVENTORY SCHEMA VERSION", "inventory.v1alpha1",
		"NODE KIND", "nvswitch",
		"SOURCE ADDRESS", "192.0.2.10:443", "SOURCE MAC", "00:11:22:33:44:55", "SOURCE VENDOR", "Dell",
		"INVENTORY DOMAIN ERROR 1", "storage: Disk.1: collection failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
	for _, unwanted := range []string{
		"\nHOSTNAME ", "\nAGENT TYPE ",
		"\nFIRMWARE CHECK ", "\nPUBLIC IP ", "\nPRIVATE IP ",
		"nested status condition", "ResourceEvent.1.0.ResourceErrorsDetected",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("OOB summary unexpectedly contains %q: %q", unwanted, got)
		}
	}
	for _, unwanted := range []string{"\nManagers\n", "\nSystems\n", "\nChassis\n", "\nFirmware\n"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("default output unexpectedly contains %q: %q", unwanted, got)
		}
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"node", "describe", "node-oob-1", "--data-source", "oob",
		"--section", "systems,chassis",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("section command failed: %v", err)
	}

	got = out.String()
	for _, want := range []string{
		"\nSystems\n", "PowerEdge XE9680", "2048",
		"system-uuid", "sku-1", "2.1.0", "asset-1", "SECURE BOOT", "true", "ROLLUP", "Warning",
		"\nCPUs\n", "CPU.Socket.1", "x86", "3800", "56", "112",
		"\nGPUs\n", "GPU.Slot.1", "NVIDIA H100",
		"\nChassis\n", "serial-1",
		"part-1", "\nPCIe Devices\n", "gpu-uuid", "single_function", "gpu-part", "96.00.00",
		"Domain Errors", "collection failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("section output missing %q: %q", want, got)
		}
	}
	for _, unwanted := range []string{"\nManagers\n", "\nFirmware\n", "ODATA ID", "HEALTH ROLLUP"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("section output unexpectedly contains %q: %q", unwanted, got)
		}
	}
	for _, unwanted := range []string{
		"FIELD", "INVENTORY SCHEMA VERSION", "SOURCE ADDRESS", "BMC MAC",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("section output unexpectedly contains summary value %q: %q", unwanted, got)
		}
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"node", "describe", "node-oob-1", "--data-source", "oob", "--section", "all",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("all sections command failed: %v", err)
	}

	got = out.String()
	for _, want := range []string{
		"FIELD", "INVENTORY SCHEMA VERSION", "SOURCE ADDRESS",
		"NODE KIND", "nvswitch",
		"\nManagers\n", "iDRAC.Embedded.1", "manager-uuid", "7.10.00.00",
		"\nSystems\n", "\nCPUs\n", "\nGPUs\n",
		"\nChassis\n", "\nPCIe Devices\n",
		"\nFirmware\n", "System BIOS", "1.2.3", "2026-01-01", "Enabled", "OK", "Warning",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("all sections output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "nested status condition") ||
		strings.Contains(got, "ResourceEvent.1.0.ResourceErrorsDetected") {
		t.Fatalf("table output unexpectedly contains nested status details: %q", got)
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-oob-1", "--data-source", "oob", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("JSON command failed: %v", err)
	}
	jsonOutput := out.String()
	for _, want := range []string{
		`"nodeKind":"nvswitch"`,
		`"status":{`,
		`"messageId":"ResourceEvent.1.0.ResourceErrorsDetected"`,
		`"message":"nested status condition"`,
	} {
		if !strings.Contains(jsonOutput, want) {
			t.Fatalf("JSON output missing %q: %q", want, jsonOutput)
		}
	}
}

// Verifies describe drill-down flags are validated before making an API request
func TestNodeDescribeRejectsInvalidSectionFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "unknown section",
			args: []string{"node", "describe", "node-1", "--data-source", "oob", "--section", "storage"},
			want: "invalid OOB inventory section",
		},
		{
			name: "all with another section",
			args: []string{
				"node", "describe", "node-1", "--data-source", "oob",
				"--section", "all,systems",
			},
			want: "section all cannot be combined with other sections",
		},
		{
			name: "inband section",
			args: []string{
				"node", "describe", "node-1", "--data-source", "inband",
				"--section", "systems",
			},
			want: "--section requires the OOB view",
		},
		{
			name: "json section",
			args: []string{
				"node", "describe", "node-1", "--data-source", "oob",
				"--section", "systems", "--output", "json",
			},
			want: "--section cannot be used with --output json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies basic node rows
func TestWriteNodeBasicTable(t *testing.T) {
	var out bytes.Buffer
	err := writeNodeTable(&out, string(nvfleetint.NodeViewBasic), "", []nvfleetint.Node{
		{UUID: "node-1", Hostname: "gpu-001", NodeName: "machine-001", BMCMAC: "bmc-001", BMCIP: "192.0.2.10:443"},
	})
	if err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"UUID", "HOSTNAME", "NODE NAME", "BMC MAC", "BMC IP",
		"node-1", "gpu-001", "machine-001", "bmc-001", "192.0.2.10:443",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestOOBDetailNodeRowsOmitInbandHostname(t *testing.T) {
	rows := oobDetailNodeRows([]nvfleetint.Node{{
		UUID:     "node-1",
		Hostname: "inband-hostname",
		NodeName: "machine-001",
		NodeKind: nvfleetint.NodeKindNVSwitch,
		BMCMAC:   "bmc-001",
		BMCIP:    "192.0.2.10:443",
	}})

	if len(rows) != 1 || len(rows[0]) != 12 {
		t.Fatalf("unexpected OOB row shape: %#v", rows)
	}
	if slices.Contains(rows[0], "inband-hostname") {
		t.Fatalf("OOB row contains in-band hostname: %#v", rows[0])
	}
	if rows[0][1] != "machine-001" || rows[0][2] != "nvswitch" || rows[0][3] != "bmc-001" || rows[0][4] != "192.0.2.10:443" {
		t.Fatalf("unexpected OOB identity columns: %#v", rows[0])
	}
}

func TestOOBNodeDescribeRowsOmitInbandFields(t *testing.T) {
	rows := oobNodeDescribeRows(nvfleetint.NodeDetails{
		Node: nvfleetint.Node{
			UUID:          "node-1",
			Hostname:      "inband-hostname",
			NodeName:      "machine-001",
			DataSource:    "oob",
			GPUType:       "NVIDIA H100",
			FirmwareCheck: "Passed",
			PublicIP:      "192.0.2.20",
			PrivateIP:     "10.0.0.20",
		},
	})

	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row[0])
	}
	for _, unwanted := range []string{
		"HOSTNAME", "AGENT TYPE",
		"FIRMWARE CHECK", "PUBLIC IP", "PRIVATE IP",
	} {
		if slices.Contains(labels, unwanted) {
			t.Fatalf("OOB summary contains in-band field %q: %#v", unwanted, labels)
		}
	}
	if !slices.Contains(labels, "NODE NAME") {
		t.Fatalf("OOB summary missing node name: %#v", labels)
	}
}

// Verifies local flag validation
func TestNodeListRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "health", args: []string{"node", "list", "--health", "Broken"}, want: "invalid health"},
		{name: "agent", args: []string{"node", "list", "--agent-status", "Missing"}, want: "invalid agent-status"},
		{name: "verification", args: []string{"node", "list", "--verification-check", "Missing"}, want: "invalid verification-check"},
		{name: "firmware", args: []string{"node", "list", "--firmware-check", "Missing"}, want: "invalid firmware-check"},
		{name: "gpu-count", args: []string{"node", "list", "--gpu-count", "eight"}, want: "invalid gpu-count"},
		{name: "negative gpu-count", args: []string{"node", "list", "--gpu-count", "8,-1"}, want: "invalid gpu-count"},
		{name: "sort", args: []string{"node", "list", "--sort-by", "bad"}, want: "invalid sort-by"},
		{name: "order", args: []string{"node", "list", "--order", "up"}, want: "invalid order"},
		{name: "basic filter", args: []string{"node", "list", "--view", "basic", "--health", "Healthy"}, want: "basic node view is incompatible"},
		{name: "basic sort", args: []string{"node", "list", "--view", "basic", "--sort-by", "healthStatus"}, want: "basic node view supports sorting only by"},
		{name: "filter without data-source", args: []string{"node", "list", "--hostname", "gpu"}, want: "--data-source is required when using --hostname"},
		{name: "sort-by without data-source", args: []string{"node", "list", "--sort-by", "hostname"}, want: "--data-source is required when using --sort-by"},
		{name: "order without data-source", args: []string{"node", "list", "--order", "asc"}, want: "--data-source is required when using --order"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies shared list pagination validation
func TestListAllRejectsPage(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"node", "list", "--all", "--page", "1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --all and --page error")
	}
	if !strings.Contains(err.Error(), "--page cannot be used with --all") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies node options render nested compute zones under the flags that accept
// them, and that JSON output stays the raw backend payload.
func TestNodeOptionsOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	body := `{"filters":{"fields":[` +
		`{"name":"computeZones","options":[{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}]},` +
		`{"name":"gpuTypes","options":["NVIDIA-H100"]},` +
		`{"name":"brandNewFilter","options":["surprise"]}` +
		`]},"sorting":{"fields":["hostname","nodeGroup","computeZone","verificationCheck","integrityCheck"],"orders":["asc","desc"],"defaults":{"field":"integrityCheck","order":"asc"}}}`

	var dataSources []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		dataSources = append(dataSources, r.URL.Query().Get("dataSource"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var tableOut bytes.Buffer
	tableCmd := newRootCmd()
	tableCmd.SetOut(&tableOut)
	tableCmd.SetArgs([]string{"node", "options", "--data-source", "oob"})
	if err := tableCmd.Execute(); err != nil {
		t.Fatalf("node options command failed: %v", err)
	}

	got := tableOut.String()
	for _, want := range []string{
		"Filters for 'node list'",
		"--compute-zone-ids",
		// Nested node groups are promoted into their own section, tagged with
		// the compute zone they belong to.
		"\n--nodegroup-ids\n  ng-1  Training  (in East)\n",
		"cz-1", "East",
		"--gpu-type",
		"brandNewFilter  (no flag on 'node list')",
		// The backend spells two sort fields differently from the CLI.
		"nodegroup", "computezone", "verificationCheck",
		// The backend's default sort field is the deprecated "integrityCheck"
		// alias; naming it as the default would offer it as a choice.
		"\n--sort-by\n", "(default: asc)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q:\n%s", want, got)
		}
	}
	// integrityCheck is a deprecated alias the API still advertises; offering it
	// would point new scripts at a field that is about to be removed, and it
	// must not be named as the --sort-by default either.
	for _, unwanted := range []string{"nodeGroup,", "computeZone,", "integrityCheck"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("table output should not offer %q:\n%s", unwanted, got)
		}
	}
	// The compute zone section lists only zones; the node group is not left
	// nested beneath its parent.
	if strings.Contains(got, "\n    ng-1") {
		t.Fatalf("node group should be promoted, not nested:\n%s", got)
	}

	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"node", "options", "--data-source", "inband", "--output", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("node options JSON command failed: %v", err)
	}
	if strings.TrimSpace(jsonOut.String()) != body {
		t.Fatalf("JSON output is not the raw payload:\n%s", jsonOut.String())
	}
	if len(dataSources) != 2 || dataSources[0] != "oob" || dataSources[1] != "inband" {
		t.Fatalf("unexpected dataSource values: %#v", dataSources)
	}
}

// Verifies an unsupported data source is rejected before any request.
func TestNodeOptionsRejectsDataSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"node", "options", "--data-source", "bmc"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid data-source") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no requests, got %d", requests)
	}
}

// Verifies a backend that returns the child list as its own field wins over
// promotion, so one flag never gets two sections, and that nested values under
// a field with no promotion target are still shown rather than dropped.
func TestNodeOptionsPromotionEdgeCases(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	body := `{"filters":{"fields":[` +
		`{"name":"computeZones","options":[{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}]},` +
		`{"name":"nodeGroups","options":[{"id":"ng-1","value":"Training"},{"id":"ng-2","value":"Serving"}]},` +
		`{"name":"brandNewFilter","options":[{"id":"p-1","value":"Parent","options":[{"id":"c-1","value":"Child"}]}]}` +
		`]},"sorting":{"fields":["hostname"],"orders":["asc"],"defaults":{"field":"hostname","order":"asc"}}}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "options", "--data-source", "inband"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("node options command failed: %v", err)
	}

	got := out.String()
	if count := strings.Count(got, "\n--nodegroup-ids\n"); count != 1 {
		t.Fatalf("expected exactly one --nodegroup-ids section, got %d:\n%s", count, got)
	}
	// The flat field supplied it, so the section carries ng-2 and no membership
	// column promoted from the compute zone.
	if !strings.Contains(got, "ng-2") || strings.Contains(got, "(in East)") {
		t.Fatalf("flat nodeGroups field should win over promotion:\n%s", got)
	}
	// A nested value with nowhere to be promoted stays indented under its parent.
	if !strings.Contains(got, "\n    c-1") {
		t.Fatalf("unpromotable nested value was dropped:\n%s", got)
	}
	// The node groups nested under the compute zone belong to --nodegroup-ids,
	// so they must not be listed under --compute-zone-ids, where passing one
	// silently matches nothing. The section points at the flag instead.
	computeZones := cmdtest.SectionBody(t, got, "--compute-zone-ids")
	if strings.Contains(computeZones, "ng-1") {
		t.Fatalf("node group listed under --compute-zone-ids:\n%s", computeZones)
	}
	if !strings.Contains(computeZones, "Values nested under these are listed under --nodegroup-ids.") {
		t.Fatalf("--compute-zone-ids section does not point at --nodegroup-ids:\n%s", computeZones)
	}
}

func TestNodeDescribeCompleteOOBInventory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataSource") != "oob" {
			t.Errorf("unexpected source: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(completeOOBNodeJSON))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, tc := range []struct {
		name, section string
		want, absent  []string
	}{
		{"summary", "", []string{"INVENTORY MEMORY MODULES", "INVENTORY STORAGE", "INVENTORY DRIVES", "INVENTORY POWER SUPPLIES", "INVENTORY FABRICS", "INVENTORY SWITCHES"}, []string{"\nSystems\n", "\nFabrics\n"}},
		{"systems", "systems", []string{"\nMemory Modules\n", "DIMM A1", "65536", "\nStorage\n", "\nDrives\n", "drive-1", "4000000000000", "CPU COUNT", "Grace", "Critical"}, []string{"\nPower Supplies\n", "\nFabrics\n"}},
		{"chassis", "chassis", []string{"\nPower Supplies\n", "psu-1", "2400"}, []string{"\nSystems\n", "\nFabrics\n"}},
		{"fabrics", "fabrics", []string{"\nFabrics\n", "\nSwitches\n", "fabric-1", "switch-1", "NVSwitch", "Warning"}, []string{"\nSystems\n", "\nChassis\n"}},
		{"all", "all", []string{"\nSystems\n", "\nMemory Modules\n", "\nDrives\n", "\nPower Supplies\n", "\nFabrics\n", "\nSwitches\n", "\nFirmware\n", "INVENTORY SWITCHES"}, []string{"PRIMARY"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			args := []string{"node", "describe", "node-1", "--data-source", "oob"}
			if tc.section != "" {
				args = append(args, "--section", tc.section)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q in %s", want, &out)
				}
			}
			for _, absent := range append(tc.absent, "  LOCATION", "GPU slot 1", "memory slot 1", "compute tray", "rack R01 U0", "rack R02 U10") {
				if strings.Contains(out.String(), absent) {
					t.Errorf("unexpected %q in %s", absent, &out)
				}
			}
		})
	}
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1", "--data-source", "oob", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(completeOOBNodeJSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("raw JSON changed, including unknown fields or references")
	}
}

// Absent nested fields stay empty; explicit zero values remain visible.
func TestOOBTablesHandleAbsentAndZeroNestedFields(t *testing.T) {
	var inventory nvfleetint.OOBInventory
	if err := json.Unmarshal([]byte(`{"systems":[{"id":"system-1"}]}`), &inventory); err != nil {
		t.Fatal(err)
	}
	row := oobSystemRows(&inventory)[0]
	for _, column := range []int{10, 11, 12, 13, 15, 16, 17} {
		if row[column] != "" {
			t.Fatalf("absent nested column %d = %q", column, row[column])
		}
	}
	zeroMemory, zeroCount := float32(0), 0
	inventory.Systems[0].Status = &nvfleetint.OOBInventoryStatus{Health: "OK"}
	inventory.Systems[0].MemorySummary = &nvfleetint.OOBMemorySummary{TotalSystemMemoryGiB: &zeroMemory}
	inventory.Systems[0].ProcessorSummary = &nvfleetint.OOBProcessorSummary{Count: &zeroCount}
	row = oobSystemRows(&inventory)[0]
	if row[11] != "OK" || row[13] != "0" || row[15] != "0" || row[10] != "" || row[12] != "" || row[16] != "" || row[17] != "" {
		t.Fatalf("partial canonical values rendered incorrectly: %v", row)
	}
}

func TestOOBInventorySummaryCounts(t *testing.T) {
	inventory := nvfleetint.OOBInventory{
		Systems: []nvfleetint.OOBSystem{
			{Memory: make([]nvfleetint.OOBMemory, 2), Storage: []nvfleetint.OOBStorage{{Drives: make([]nvfleetint.OOBDrive, 2)}, {}}},
			{Memory: make([]nvfleetint.OOBMemory, 3), Storage: []nvfleetint.OOBStorage{{Drives: make([]nvfleetint.OOBDrive, 4)}}},
		},
		Chassis: []nvfleetint.OOBChassis{{PowerSupplies: make([]nvfleetint.OOBPowerSupply, 2)}, {PowerSupplies: make([]nvfleetint.OOBPowerSupply, 1)}},
		Fabrics: []nvfleetint.OOBFabric{{Switches: make([]nvfleetint.OOBSwitch, 2)}, {Switches: make([]nvfleetint.OOBSwitch, 4)}},
	}
	got := map[string]string{}
	for _, row := range oobInventorySummaryRows(&inventory) {
		got[row[0]] = row[1]
	}
	for name, want := range map[string]string{"MEMORY MODULES": "5", "STORAGE": "3", "DRIVES": "6", "POWER SUPPLIES": "3", "FABRICS": "2", "SWITCHES": "6"} {
		if got["INVENTORY "+name] != want {
			t.Errorf("%s count = %s, want %s", name, got["INVENTORY "+name], want)
		}
	}
}

func TestOOBEmptyFabricSection(t *testing.T) {
	options, err := parseOOBInventorySections(nodeDescribeFlags{sections: "fabrics"}, nvfleetint.DataSourceOOB, "table")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := writeOOBInventoryTables(&out, &nvfleetint.OOBInventory{}, options); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Fabrics") || strings.Contains(out.String(), "\nSwitches\n") {
		t.Fatalf("unexpected empty section: %s", &out)
	}
	for _, tc := range []struct {
		source nvfleetint.DataSource
		output string
	}{{nvfleetint.DataSourceInband, "table"}, {nvfleetint.DataSourceOOB, "json"}} {
		if _, err := parseOOBInventorySections(nodeDescribeFlags{sections: "fabrics"}, tc.source, tc.output); err == nil {
			t.Fatal("fabric section accepted invalid source/output combination")
		}
	}
}

func TestNodeUUIDFilterFlagsAndPagination(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if !slices.Equal(q["agentUUIDs"], []string{"agent-A", "agent-B"}) || !slices.Equal(q["systemUUIDs"], []string{"system-A", "system-B"}) {
			t.Errorf("UUID filters lost: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		if q.Get("page") == "0" {
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","agentUUID":"agent-A"}],"page":0,"pageSize":1,"hasMore":true,"total":2}`))
		} else {
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2","agentUUID":"agent-B"}],"page":1,"pageSize":1,"hasMore":false,"total":2}`))
		}
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, scope := range [][]string{{"--view", "basic"}, {"--data-source", "inband"}} {
		var out bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		args := []string{"node", "list", "--agent-uuids", "agent-A,agent-B", "--system-uuids", "system-A,system-B", "--all", "--page-size", "1", "--output", "json"}
		cmd.SetArgs(append(args, scope...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), `"agentUUID":"agent-A"`) || !strings.Contains(out.String(), `"agentUUID":"agent-B"`) {
			t.Fatalf("missing agent identities: %s", &out)
		}
	}
	if requests != 4 {
		t.Fatalf("wanted two pages per query, got %d requests", requests)
	}
}

func TestNodeUUIDFilterValidationBeforeAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	for _, args := range [][]string{
		{"--data-source", "oob", "--agent-uuids", "agent-A"},
		{"--view", "basic", "--data-source", "oob", "--agent-uuids", "agent-A"},
		{"--agent-uuids", "agent-A"}, {"--system-uuids", "system-A"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(append([]string{"node", "list"}, args...))
		err := cmd.Execute()
		if err == nil || (!strings.Contains(err.Error(), "agent-uuids") && !strings.Contains(err.Error(), "system-uuids")) {
			t.Fatalf("unexpected validation: %v", err)
		}
	}
}

func TestNodeVerificationPolicyStatusFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	statuses := []string{"Full Attestation", "Partial Attestation", "Verification Failed"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !slices.Equal(r.URL.Query()["verificationChecks"], statuses) {
			t.Errorf("incorrect verification filters: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","verificationCheck":"Full Attestation"}],"total":1}`))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, source := range []string{"inband", "oob"} {
		var out bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"node", "list", "--data-source", source, "--verification-check", strings.Join(statuses, ",")})
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Full Attestation") {
			t.Fatalf("missing status: %s", &out)
		}
	}
	var help bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&help)
	cmd.SetArgs([]string{"node", "list", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, status := range statuses {
		if !strings.Contains(help.String(), status) {
			t.Fatalf("missing help status %s", status)
		}
	}
}

// These cases protect combined versus source-specific detail behavior for
// Node describe source selection, including optional OOB data (docs/cli.md).
func TestNodeDescribeSourceFailuresAndJSON(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		statuses   map[string]int
		counts     map[string]int
		wantErrors []string
		jsonMode   string
	}{
		{name: "combined inband forbidden", statuses: map[string]int{"inband": 403, "oob": 200}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{"fetch in-band node view", "source access denied"}},
		{name: "combined oob forbidden", statuses: map[string]int{"inband": 200, "oob": 403}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{"fetch OOB node view", "source access denied"}},
		{name: "both missing", statuses: map[string]int{"inband": 404, "oob": 404}, counts: map[string]int{"inband": 1, "oob": 1}, wantErrors: []string{`node "node-1" was not found in either the in-band or OOB view`}},
		{name: "explicit inband forbidden", args: []string{"--data-source", "inband"}, statuses: map[string]int{"inband": 403}, counts: map[string]int{"inband": 1, "oob": 0}, wantErrors: []string{"source access denied"}},
		{name: "oob section without inventory", args: []string{"--data-source", "oob", "--section", "systems"}, statuses: map[string]int{"oob": 200}, counts: map[string]int{"inband": 0, "oob": 1}, wantErrors: []string{"does not have an OOB view for the requested section"}},
		{name: "combined json with only oob", args: []string{"--output", "json"}, statuses: map[string]int{"inband": 404, "oob": 200}, counts: map[string]int{"inband": 1, "oob": 1}, jsonMode: "combined"},
		{name: "explicit inband json", args: []string{"--data-source", "inband", "--output", "json"}, statuses: map[string]int{"inband": 200}, counts: map[string]int{"inband": 1, "oob": 0}, jsonMode: "inband"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			counts := map[string]int{"inband": 0, "oob": 0}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				sources := r.URL.Query()["dataSource"]
				if len(sources) != 1 || (sources[0] != "inband" && sources[0] != "oob") {
					t.Errorf("unexpected dataSource query: %q", sources)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				source := sources[0]
				mu.Lock()
				counts[source]++
				mu.Unlock()
				status, ok := tc.statuses[source]
				if !ok {
					t.Errorf("unexpected source request: %s", source)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				switch status {
				case http.StatusForbidden:
					_, _ = w.Write([]byte(`{"error":"source access denied"}`))
				case http.StatusNotFound:
					_, _ = w.Write([]byte(`{"error":"source not found"}`))
				default:
					if source == "inband" {
						_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"inband","hostname":"gpu-001","futureField":{"preserved":true}}`))
					} else {
						_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"oob"}`))
					}
				}
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"node", "describe", "node-1"}, tc.args...))
			err := cmd.Execute()
			if len(tc.wantErrors) > 0 {
				if err == nil {
					t.Fatal("expected command error")
				}
				for _, want := range tc.wantErrors {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				if out.Len() != 0 {
					t.Errorf("expected empty stdout, got %q", out.String())
				}
			} else if err != nil {
				t.Errorf("command failed: %v", err)
			} else {
				var got map[string]any
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Errorf("decode output: %v", err)
				} else if tc.jsonMode == "combined" {
					if _, exists := got["inband"]; exists {
						t.Errorf("unexpected inband object: %#v", got)
					}
					oob, ok := got["oob"].(map[string]any)
					if !ok || oob["nodeUUID"] != "node-1" || oob["dataSource"] != "oob" {
						t.Errorf("unexpected OOB object: %#v", got["oob"])
					}
				} else {
					future, ok := got["futureField"].(map[string]any)
					if got["nodeUUID"] != "node-1" || got["hostname"] != "gpu-001" || !ok || future["preserved"] != true {
						t.Errorf("unexpected inband JSON: %#v", got)
					}
					for _, key := range []string{"inband", "oob"} {
						if _, exists := got[key]; exists {
							t.Errorf("unexpected %s wrapper: %#v", key, got)
						}
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for _, source := range []string{"inband", "oob"} {
				if counts[source] != tc.counts[source] {
					t.Errorf("%s requests = %d, want %d", source, counts[source], tc.counts[source])
				}
			}
		})
	}
}

// nodeRenderWriter records actual writes so failures can target a boundary
// even when tabwriter emits a table in multiple writes.
type nodeRenderWriter struct {
	output bytes.Buffer
	writes []string
	failAt int
	err    error
	after  int
}

func (w *nodeRenderWriter) Write(p []byte) (int, error) {
	if w.failAt >= 0 && len(w.writes) >= w.failAt {
		if len(w.writes) > w.failAt {
			w.after++
		}
		w.writes = append(w.writes, string(p))
		return 0, w.err
	}
	w.writes = append(w.writes, string(p))
	return w.output.Write(p)
}

func TestNodeListTableRenderingBoundaries(t *testing.T) {
	result := nodeListOutput{
		View:  string(nvfleetint.NodeViewBasic),
		Nodes: []nvfleetint.Node{{UUID: "node-list-1", Hostname: "host-list-1"}},
		Page:  &clioutput.Pagination{Page: 0, PageSize: 1, Total: 2},
	}
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	control := &nodeRenderWriter{failAt: -1}
	if err := writeNodeListOutput(control, common, result); err != nil {
		t.Fatalf("render list control: %v", err)
	}
	if !strings.Contains(control.output.String(), "node-list-1") || !strings.Contains(control.output.String(), "Page: 1") {
		t.Fatalf("control missing node or footer: %q", control.output.String())
	}
	if len(control.writes) == 0 || strings.Contains(control.writes[0], "Page:") {
		t.Fatalf("first write is not a table boundary: %q", control.writes)
	}
	sentinel := errors.New("list table write failed")
	w := &nodeRenderWriter{failAt: 0, err: sentinel}
	if err := writeNodeListOutput(w, common, result); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want sentinel", err)
	}
	if w.after != 0 || len(w.writes) != 1 || w.output.Len() != 0 {
		t.Errorf("writes after table failure: writes=%q accepted=%q", w.writes, w.output.String())
	}

	result.Page = nil
	var noPage bytes.Buffer
	if err := writeNodeListOutput(&noPage, common, result); err != nil {
		t.Fatalf("render list without page: %v", err)
	}
	if !strings.Contains(noPage.String(), "node-list-1") || strings.Contains(noPage.String(), "Page:") {
		t.Errorf("unexpected unpaginated output: %q", noPage.String())
	}
}

func TestNodeListDetailInfersOOBColumns(t *testing.T) {
	result := nodeListOutput{
		View:  "detail",
		Nodes: []nvfleetint.Node{{UUID: "node-oob-1", NodeName: "bmc-node-1"}},
	}
	var out bytes.Buffer
	if err := writeNodeListOutput(&out, cmdutil.Resolved{Output: clioutput.FormatTable}, result); err != nil {
		t.Fatalf("render inferred OOB list: %v", err)
	}
	for _, want := range []string{"NODE NAME", "NODE KIND", "BMC MAC", "BMC IP", "node-oob-1", "bmc-node-1"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output missing %q: %q", want, out.String())
		}
	}
	if strings.Contains(out.String(), "FIRMWARE CHECK") || strings.Contains(out.String(), "HOSTNAME") {
		t.Errorf("in-band columns in OOB output: %q", out.String())
	}
}

func TestNodeDescribeSummaryWriteBoundaries(t *testing.T) {
	inband := nvfleetint.NodeDetails{Node: nvfleetint.Node{UUID: "inband-node", DataSource: "inband"}}
	oob := nvfleetint.NodeDetails{Node: nvfleetint.Node{UUID: "oob-node", DataSource: "oob"}}
	result := nodeDescribeResult{Inband: &inband, OOB: &oob}
	options := nodeDescribeRenderOptions{showSummary: true}
	control := &nodeRenderWriter{failAt: -1}
	if err := writeNodeDescribeResultTable(control, result, "", options); err != nil {
		t.Fatalf("render combined control: %v", err)
	}
	full := control.output.String()
	positions := []int{strings.Index(full, "In-band\n"), strings.Index(full, "inband-node"), strings.Index(full, "\nOut-of-band\n"), strings.Index(full, "oob-node")}
	if positions[0] != 0 || positions[1] <= positions[0] || positions[2] <= positions[1] || positions[3] <= positions[2] {
		t.Fatalf("unexpected combined section order: %q", full)
	}
	for _, tc := range []struct {
		name   string
		offset int
		write  string
	}{
		{"in-band heading", 0, "In-band\n"},
		{"separator", positions[2], "\n"},
		{"out-of-band heading", positions[2] + 1, "Out-of-band\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, offset := -1, 0
			for i, part := range control.writes {
				if offset == tc.offset && part == tc.write {
					index = i
					break
				}
				offset += len(part)
			}
			if index < 0 {
				t.Fatalf("boundary %q at %d not found in writes %q", tc.write, tc.offset, control.writes)
			}
			sentinel := errors.New("describe section write failed")
			w := &nodeRenderWriter{failAt: index, err: sentinel}
			if err := writeNodeDescribeResultTable(w, result, "", options); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if w.after != 0 || len(w.writes) != index+1 || w.output.String() != full[:tc.offset] {
				t.Errorf("unexpected writes after failure: accepted=%q writes=%q", w.output.String(), w.writes)
			}
		})
	}

	t.Run("explicit inband summary table", func(t *testing.T) {
		var expected bytes.Buffer
		if err := writeNodeDescribeResultTable(&expected, result, nvfleetint.DataSourceInband, options); err != nil {
			t.Fatalf("render explicit control: %v", err)
		}
		if !strings.Contains(expected.String(), "inband-node") || strings.Contains(expected.String(), "oob-node") {
			t.Fatalf("unexpected explicit output: %q", expected.String())
		}
		sentinel := errors.New("summary table write failed")
		w := &nodeRenderWriter{failAt: 0, err: sentinel}
		if err := writeNodeDescribeResultTable(w, result, nvfleetint.DataSourceInband, options); !errors.Is(err, sentinel) {
			t.Errorf("error = %v, want sentinel", err)
		}
		if w.after != 0 || len(w.writes) != 1 || w.output.Len() != 0 {
			t.Errorf("unexpected writes after summary failure: %q", w.writes)
		}
	})
}

type nodeSectionFailureWriter struct {
	accepted bytes.Buffer
	heading  string
	err      error
	reached  bool
	after    int
}

func (w *nodeSectionFailureWriter) Write(p []byte) (int, error) {
	if w.reached {
		w.after++
		return 0, w.err
	}
	if bytes.Contains(p, []byte(w.heading)) {
		w.reached = true
		return 0, w.err
	}
	return w.accepted.Write(p)
}

func TestNodeDescribeOOBSectionWriteErrors(t *testing.T) {
	var oob nvfleetint.NodeDetails
	if err := json.Unmarshal([]byte(completeOOBNodeJSON), &oob); err != nil {
		t.Fatalf("decode OOB fixture: %v", err)
	}
	if oob.OOBInventory == nil {
		t.Fatal("OOB fixture has no inventory")
	}
	options, err := parseOOBInventorySections(nodeDescribeFlags{sections: "all"}, nvfleetint.DataSourceOOB, "table")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}

	titles := []string{"Managers", "Systems", "CPUs", "GPUs", "Memory Modules", "Chassis", "PCIe Devices", "Power Supplies", "Fabrics", "Switches", "Firmware"}
	result := nodeDescribeResult{OOB: &oob}
	var control bytes.Buffer
	if err := writeNodeDescribeResultTable(&control, result, nvfleetint.DataSourceOOB, options); err != nil {
		t.Fatalf("render OOB control: %v", err)
	}
	for _, want := range append(append([]string{}, titles...), "manager-1", "system-1", "cpu-1", "gpu-1", "dimm-1", "chassis-1", "pcie-1", "psu-1", "fabric-1", "switch-1", "firmware-1") {
		if !strings.Contains(control.String(), want) {
			t.Errorf("OOB control missing %q", want)
		}
	}

	checkFailure := func(t *testing.T, result nodeDescribeResult, source nvfleetint.DataSource, expected, title string) {
		t.Helper()
		heading := "\n" + title + "\n"
		index := strings.Index(expected, heading)
		if index < 0 {
			t.Fatalf("control missing heading %q", heading)
		}
		sentinel := errors.New("section write failed")
		writer := &nodeSectionFailureWriter{heading: heading, err: sentinel}
		err := writeNodeDescribeResultTable(writer, result, source, options)
		if !errors.Is(err, sentinel) {
			t.Errorf("error = %v, want sentinel", err)
		}
		if !writer.reached {
			t.Errorf("writer did not reach heading %q", heading)
		}
		if writer.after != 0 {
			t.Errorf("writer received %d writes after failure", writer.after)
		}
		if got := writer.accepted.String(); got != expected[:index] {
			t.Errorf("accepted output differs from control prefix before %q: got %q", title, got)
		}
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			checkFailure(t, result, nvfleetint.DataSourceOOB, control.String(), title)
		})
	}

	t.Run("combined sources", func(t *testing.T) {
		inband := nvfleetint.NodeDetails{}
		inband.DataSource = string(nvfleetint.DataSourceInband)
		combined := nodeDescribeResult{Inband: &inband, OOB: &oob}
		var combinedControl bytes.Buffer
		if err := writeNodeDescribeResultTable(&combinedControl, combined, "", options); err != nil {
			t.Fatalf("render combined control: %v", err)
		}
		checkFailure(t, combined, "", combinedControl.String(), "Switches")
	})
}

// Protect accurate Node list results and pagination error reporting when a
// request fails, including after a page has already been fetched.
func TestNodeListHTTPFailures(t *testing.T) {
	cases := []struct {
		name       string
		args       []string
		wantPages  []string
		wantErrors []string
	}{
		{name: "single page", args: []string{"--page", "1"}, wantPages: []string{"0"}, wantErrors: []string{"403"}},
		{name: "all pages after first page", args: []string{"--all", "--page-size", "1"}, wantPages: []string{"0", "1"}, wantErrors: []string{"403", "page 1", "after 1 completed pages"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			var pages []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				pages = append(pages, r.URL.Query().Get("page"))
				requestNumber := len(pages)
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if len(tc.wantPages) == 2 && requestNumber == 1 {
					if got := r.URL.Query().Get("pageSize"); got != "1" {
						t.Errorf("pageSize = %q, want 1", got)
					}
					_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-first"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"node access denied"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"node", "list", "--view", "basic", "--output", "json"}, tc.args...))
			err := cmd.Execute()
			if err == nil {
				t.Error("expected command error")
			} else {
				for _, want := range tc.wantErrors {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty output, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if len(pages) != len(tc.wantPages) {
				t.Errorf("requested pages = %q, want %q", pages, tc.wantPages)
			} else {
				for i, want := range tc.wantPages {
					if pages[i] != want {
						t.Errorf("requested pages = %q, want %q", pages, tc.wantPages)
						break
					}
				}
			}
		})
	}
}

func TestNodeRejectsEmptyCommaListEntries(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantFlag string
	}{
		{name: "health", args: []string{"node", "list", "--data-source", "inband", "--health", "Healthy,,Degraded"}, wantFlag: "health"},
		{name: "agent status", args: []string{"node", "list", "--data-source", "inband", "--agent-status", "Online,,Offline"}, wantFlag: "agent-status"},
		{name: "verification check", args: []string{"node", "list", "--data-source", "inband", "--verification-check", "Verified,,Pending"}, wantFlag: "verification-check"},
		{name: "firmware check", args: []string{"node", "list", "--data-source", "inband", "--firmware-check", "Passed,,Failed"}, wantFlag: "firmware-check"},
		{name: "node UUIDs", args: []string{"node", "list", "--data-source", "inband", "--node-uuids", "node-1,,node-2"}, wantFlag: "node-uuids"},
		{name: "sections", args: []string{"node", "describe", "node-1", "--data-source", "oob", "--section", "systems,,chassis"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Error("expected command error")
			} else {
				if !strings.Contains(err.Error(), "empty values are not allowed") {
					t.Errorf("error %q does not identify empty list values", err)
				}
				if tc.wantFlag != "" && !strings.Contains(err.Error(), tc.wantFlag) {
					t.Errorf("error %q does not identify flag %q", err, tc.wantFlag)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 0 {
				t.Errorf("server received %d requests, want none", requests)
			}
		})
	}
}

func TestNodeDescribeInbandResourceSummary(t *testing.T) {
	resourceRows := map[string]string{
		"CPU TYPE":            "Xeon-Example",
		"CPU MANUFACTURER":    "ExampleCorp",
		"CPU ARCHITECTURE":    "arm64",
		"CPU LOGICAL CORES":   "96",
		"MEMORY TOTAL BYTES":  "1099511627776",
		"CONTAINER ROOT DISK": "/dev/example0",
		"DISK DEVICES":        "2",
		"NIC INTERFACES":      "3",
	}
	cases := []struct {
		name      string
		response  string
		populated bool
	}{
		{
			name:      "populated resources",
			response:  `{"nodeUUID":"node-1","dataSource":"inband","resources":{"cpuInfo":{"type":"Xeon-Example","manufacturer":"ExampleCorp","architecture":"arm64","logicalCores":"96"},"memoryInfo":{"totalBytes":"1099511627776"},"diskInfo":{"containerRootDisk":"/dev/example0","blockDevices":[{"name":"disk0"},{"name":"disk1"}]},"nicInfo":{"privateIPInterfaces":[{"interface":"eth0"},{"interface":"eth1"},{"interface":"eth2"}]}}}`,
			populated: true,
		},
		{name: "absent resources", response: `{"nodeUUID":"node-1","dataSource":"inband"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				if sources := r.URL.Query()["dataSource"]; len(sources) != 1 || sources[0] != "inband" {
					t.Errorf("dataSource = %q, want [inband]", sources)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.response))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "describe", "node-1", "--data-source", "inband", "--output", "table"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			if gotRequests != 1 {
				t.Errorf("requests = %d, want 1", gotRequests)
			}

			rowValue := func(label string) (string, bool) {
				for _, line := range strings.Split(out.String(), "\n") {
					if strings.HasPrefix(line, label+"  ") {
						return strings.TrimSpace(strings.TrimPrefix(line, label)), true
					}
				}
				return "", false
			}
			if got, ok := rowValue("UUID"); !ok || got != "node-1" {
				t.Errorf("UUID row = %q (present %t), want node-1; output: %s", got, ok, out.String())
			}
			for label, want := range resourceRows {
				got, ok := rowValue(label)
				if tc.populated && (!ok || got != want) {
					t.Errorf("%s row = %q (present %t), want %q; output: %s", label, got, ok, want, out.String())
				} else if !tc.populated && ok {
					t.Errorf("unexpected %s row = %q", label, got)
				}
			}
		})
	}
}

func TestNodeHealthValidationAndHTTPFailure(t *testing.T) {
	cases := []struct {
		name     string
		uuid     string
		flags    []string
		want     []string
		requests int
	}{
		{name: "invalid output", uuid: "node-1", flags: []string{"--output", "yaml"}, want: []string{`invalid output "yaml": expected table or json`}},
		{name: "zero timeout", uuid: "node-1", flags: []string{"--timeout", "0s"}, want: []string{"--timeout must be greater than 0"}},
		{name: "blank UUID", uuid: " \t ", want: []string{"node UUID is required"}},
		{name: "missing profile", uuid: "node-1", flags: []string{"--profile", "nonexistent"}, want: []string{"profile not found", "nonexistent"}},
		{name: "forbidden", uuid: "node-1", want: []string{"403", "health access denied"}, requests: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"health access denied"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			args := []string{"node", "health", tc.uuid, "--start", "2026-04-07T00:00:00Z", "--end", "2026-04-14T00:00:00Z"}
			cmd.SetArgs(append(args, tc.flags...))
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected command error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != tc.requests {
				t.Errorf("requests = %d, want %d", requests, tc.requests)
			}
		})
	}
}

func TestNodeHealthEmptyHistoryRendering(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		summary bool
	}{
		{name: "empty summary", body: `{"healthSummary":{},"machineStatus":[]}`, summary: true},
		{name: "absent summary", body: `{"machineStatus":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "health", "node-1", "--start", "2026-04-07T00:00:00Z", "--end", "2026-04-14T00:00:00Z"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			if gotRequests != 1 {
				t.Errorf("requests = %d, want 1", gotRequests)
			}
			got := out.String()
			if strings.Contains(got, "Health Summary") != tc.summary {
				t.Errorf("unexpected summary heading: %q", got)
			}
			for _, want := range []string{"Machine Status", "No health timeline segments in the requested window."} {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %q", want, got)
				}
			}
			if tc.summary {
				for _, state := range []string{"Healthy", "Degraded", "Unhealthy"} {
					found := false
					for _, line := range strings.Split(got, "\n") {
						fields := strings.Fields(line)
						if len(fields) == 3 && fields[0] == state && fields[1] == "-" && fields[2] == "-" {
							found = true
						}
					}
					if !found {
						t.Errorf("missing %s row with absent percentage and duration: %q", state, got)
					}
				}
			}
		})
	}
}

func TestNodeHealthTableWriteBoundaries(t *testing.T) {
	var history nvfleetint.NodeHealthHistory
	if err := json.Unmarshal([]byte(nodeHealthBody), &history); err != nil {
		t.Fatalf("decode health fixture: %v", err)
	}
	history.Segments = nil
	if history.HealthSummary == nil || history.HealthSummary.HealthyDurationSeconds == nil {
		t.Fatal("fixture has no populated summary")
	}
	control := &nodeRenderWriter{failAt: -1}
	if err := writeNodeHealthTable(control, "node-1", history); err != nil {
		t.Fatalf("render control: %v", err)
	}
	full := control.output.String()
	summary := strings.Index(full, "\nHealth Summary\n")
	machine := strings.Index(full, "\nMachine Status\n")
	empty := strings.Index(full, "No health timeline segments in the requested window.\n")
	if summary <= 0 || machine <= summary || empty <= machine {
		t.Fatalf("unexpected section order: %q", full)
	}
	for _, tc := range []struct {
		name   string
		offset int
		prefix string
	}{
		{name: "metadata table", offset: 0, prefix: "FIELD"},
		{name: "summary heading", offset: summary, prefix: "\nHealth Summary\n"},
		{name: "summary table", offset: summary + len("\nHealth Summary\n"), prefix: "STATE"},
		{name: "machine heading", offset: machine, prefix: "\nMachine Status\n"},
		{name: "empty timeline message", offset: empty, prefix: "No health timeline segments in the requested window.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, offset := -1, 0
			for i, part := range control.writes {
				if offset == tc.offset && strings.HasPrefix(part, tc.prefix) {
					index = i
					break
				}
				offset += len(part)
			}
			if index < 0 {
				t.Fatalf("boundary at %d beginning %q not found in writes %q", tc.offset, tc.prefix, control.writes)
			}
			sentinel := errors.New("health write failed")
			w := &nodeRenderWriter{failAt: index, err: sentinel}
			if err := writeNodeHealthTable(w, "node-1", history); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if w.after != 0 || len(w.writes) != index+1 || w.output.String() != full[:tc.offset] {
				t.Errorf("unexpected writes after failure: accepted=%q writes=%q", w.output.String(), w.writes)
			}
		})
	}
}

func TestNodeCommandEarlyValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "blank describe UUID", args: []string{"node", "describe", " \t "}, want: []string{"node UUID is required"}},
		{name: "invalid describe source", args: []string{"node", "describe", "node-1", "--data-source", "bmc"}, want: []string{"data-source", "bmc"}},
		{name: "zero options timeout", args: []string{"node", "options", "--timeout", "0"}, want: []string{"--timeout must be greater than 0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected validation error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 0 {
				t.Errorf("requests = %d, want none", requests)
			}
		})
	}
}

func TestNodeCommandsRequireCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"node", "list"}},
		{name: "describe", args: []string{"node", "describe", "node-1"}},
		{name: "options", args: []string{"node", "options"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("NVFLEETINT_API_KEY", "")
			t.Setenv("NVFLEETINT_API_URL", "")
			t.Setenv("NVFLEETINT_SERVICE_KEY", "")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected missing credential error")
			}
			for _, want := range []string{"no profile configured", "auth add", "NVFLEETINT_API_KEY"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
		})
	}
}

func TestNodeOptionsSingleSourceHTTPFailures(t *testing.T) {
	for _, tc := range []struct {
		name       string
		args       []string
		wantView   string
		wantSource string
	}{
		{name: "explicit inband", args: []string{"node", "options", "--data-source", "inband"}, wantView: "detail", wantSource: "inband"},
		{name: "combined basic", args: []string{"node", "options", "--view", "basic"}, wantView: "basic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			type request struct {
				method, path, view, source string
				hasSource                  bool
			}
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, q.Get("view"), q.Get("dataSource"), q.Has("dataSource")})
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"distinctive options denial"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "distinctive options denial") {
				t.Errorf("error = %v, want server denial", err)
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			want := request{http.MethodGet, "/v1/nodes/options", tc.wantView, tc.wantSource, tc.wantSource != ""}
			if len(requests) != 1 || requests[0] != want {
				t.Errorf("requests = %+v, want [%+v]", requests, want)
			}
		})
	}
}

func TestNodeDescribeCombinedSelectedInventorySections(t *testing.T) {
	for _, tc := range []struct {
		name         string
		inbandStatus int
	}{
		{name: "inband present", inbandStatus: http.StatusOK},
		{name: "inband missing", inbandStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			counts := map[string]int{"inband": 0, "oob": 0}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				sources := r.URL.Query()["dataSource"]
				if len(sources) != 1 || (sources[0] != "inband" && sources[0] != "oob") {
					t.Errorf("unexpected dataSource query: %q", sources)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				source := sources[0]
				mu.Lock()
				counts[source]++
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if source == "oob" {
					_, _ = w.Write([]byte(completeOOBNodeJSON))
				} else if tc.inbandStatus == http.StatusNotFound {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":"source not found"}`))
				} else {
					_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"inband","hostname":"distinctive-inband-host"}`))
				}
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "describe", "node-1", "--section", "systems,chassis"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			for _, source := range []string{"inband", "oob"} {
				if counts[source] != 1 {
					t.Errorf("%s requests = %d, want 1", source, counts[source])
				}
			}
			mu.Unlock()
			got := out.String()
			for _, want := range []string{"Systems", "system-1", "CPUs", "cpu-1", "GPUs", "gpu-1", "Chassis", "chassis-1"} {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %q", want, got)
				}
			}
			for _, unwanted := range []string{"distinctive-inband-host", "In-band", "Out-of-band"} {
				if strings.Contains(got, unwanted) {
					t.Errorf("output contains %q: %q", unwanted, got)
				}
			}
			for _, line := range strings.Split(got, "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 && fields[0] == "FIELD" {
					t.Errorf("output contains summary header: %q", line)
				}
				for _, heading := range []string{"Managers", "Firmware", "Fabrics"} {
					if strings.TrimSpace(line) == heading {
						t.Errorf("output contains unselected heading %q", heading)
					}
				}
			}
		})
	}
}

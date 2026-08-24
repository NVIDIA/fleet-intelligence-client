// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies local output flags and sort field pass-through
func TestNodeListLocalJSONAndSort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"node", "list", "--agent-type", "inband", "--output", "json", "--sort-by", "healthStatus",
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

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("sortBy"); got != field {
					t.Fatalf("unexpected sortBy: got %q want %q", got, field)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"nodes":[],"hasMore":false,"page":0,"pageSize":20,"total":0}`))
			}))
			defer server.Close()

			saveTestConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{
				"node", "list", "--agent-type", "inband", "--output", "json", "--sort-by", field,
			})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
		})
	}
}

// Verifies both the user-facing "verificationCheck" sort field and the legacy
// backend "integrityCheck" spelling reach the API as "integrityCheck"
func TestNodeListSortVerificationCheck(t *testing.T) {
	for _, sortBy := range []string{"verificationCheck", "integrityCheck"} {
		t.Run(sortBy, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("sortBy"); got != "integrityCheck" {
					t.Fatalf("unexpected sortBy: got %q want %q", got, "integrityCheck")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"nodes":[],"hasMore":false,"page":0,"pageSize":20,"total":0}`))
			}))
			defer server.Close()

			saveTestConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "list", "--output", "json", "--sort-by", sortBy})

			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
		})
	}
}

// Verifies node list help and sort errors use the user-facing "verificationCheck"
func TestNodeListSortHelpUsesVerificationCheck(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	// Basic view rejects the sort after the CLI has translated it, so this is
	// the path where the backend spelling could leak back to the user.
	_, err = nodeListOptions(nodeListFlags{view: "basic", sortBy: nodeSortByVerificationCheck})
	if err == nil {
		t.Fatal("expected basic view sort error")
	}
	if strings.Contains(err.Error(), "integrityCheck") {
		t.Fatalf("basic view sort error mentions integrityCheck: %v", err)
	}
}

// Verifies table output and filter translation
func TestNodeListTableFiltersAndSort(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
		if got := query["integrityChecks"]; !slices.Equal(got, []string{"Verified"}) {
			t.Fatalf("unexpected integrityChecks: %#v raw query %q", got, r.URL.RawQuery)
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
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","healthStatus":"Healthy","gpuType":"NVIDIA-H100","gpuCount":8,"integrityCheck":"Verified","firmwareCheck":"Unknown","agentStatus":"Online"}],"hasMore":false,"page":0,"pageSize":10,"total":1}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--agent-type", "inband", "--node-uuids", "node-1,node-2", "--health", "Healthy,Degraded", "--hostname", "gpu", "--agent-status", "Online", "--verification-check", "Verified", "--firmware-check", "Unknown", "--compute-zone-ids", "cz-1,cz-2", "--compute-zone-names", "East", "--nodegroup-ids", "ng-1", "--nodegroup-names", "Training", "--gpu-type", "NVIDIA-H100", "--gpu-count", "8,4", "--public-ip", "203.0.113.10", "--private-ip", "10.0.0.10", "--sort-by", "computezone", "--order", "desc", "--page-size", "10"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "GPU TYPE", "VERIFICATION CHECK", "FIRMWARE CHECK", "AGENT STATUS", "node-1", "gpu-001", "East", "Training", "Verified", "Unknown", "Online", "Page: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies detail lists combine independent in-band and OOB API views by default
func TestNodeListCombinedDetailViews(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("view"); got != "detail" {
			t.Errorf("unexpected view: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("agentType") {
		case "inband":
			_, _ = w.Write([]byte(`{
				"nodes":[{"nodeUUID":"node-inband","hostname":"gpu-001","agentType":"inband","healthStatus":"Healthy"}],
				"hasMore":false,"page":0,"pageSize":20,"total":1
			}`))
		case "oob":
			_, _ = w.Write([]byte(`{
				"nodes":[{"nodeUUID":"node-oob","agentType":"oob","bmcHostname":"bmc-001","bmcIP":"192.0.2.10:443","healthStatus":"Degraded"}],
				"hasMore":false,"page":0,"pageSize":20,"total":1
			}`))
		default:
			t.Errorf("unexpected agentType: %q", r.URL.Query().Get("agentType"))
			http.Error(w, "missing agentType", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

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
		"Out-of-band", "node-oob", "bmc-001", "BMC HOSTNAME",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("combined table missing %q: %q", want, got)
		}
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("combined JSON command failed: %v", err)
	}

	var combined struct {
		Inband struct {
			Nodes []map[string]any `json:"nodes"`
			Page  int              `json:"page"`
		} `json:"inband"`
		OOB struct {
			Nodes []map[string]any `json:"nodes"`
			Page  int              `json:"page"`
		} `json:"oob"`
	}
	if err := json.Unmarshal(out.Bytes(), &combined); err != nil {
		t.Fatalf("decode combined JSON: %v", err)
	}
	if len(combined.Inband.Nodes) != 1 || combined.Inband.Nodes[0]["nodeUUID"] != "node-inband" {
		t.Fatalf("unexpected in-band JSON: %#v", combined.Inband)
	}
	if len(combined.OOB.Nodes) != 1 || combined.OOB.Nodes[0]["nodeUUID"] != "node-oob" {
		t.Fatalf("unexpected OOB JSON: %#v", combined.OOB)
	}
	if combined.Inband.Page != 1 || combined.OOB.Page != 1 {
		t.Fatalf("combined JSON pages should be 1-based: %#v", combined)
	}
}

// Verifies all-page JSON output
func TestNodeListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	saveTestConfig(t, server.URL, "test-key")

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

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--agent-type", "inband", "--all", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

// Verifies --all retries only a transiently failing page instead of restarting
// pagination and duplicating already collected nodes.
func TestNodeListAllRetriesFailedPage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	saveTestConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"node", "list", "--agent-type", "inband", "--all", "--output", "json", "--timeout", "5s",
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/node-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","computeZoneId":"cz-1","nodeGroup":"Training","nodeGroupId":"ng-1","healthStatus":"Degraded","gpuType":"NVIDIA-H100","gpuCount":8,"agentStatus":"Online","resources":{"gpuInfo":{"product":"NVIDIA H100","gpus":[{"uuid":"GPU-1"}]}},"systemInfo":{"agentVersion":"1.2.3","cudaVersion":"12.4"},"tags":["prod","h100"]}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1", "--agent-type", "inband"})

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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("agentType") {
		case "inband":
			_, _ = w.Write([]byte(`{
				"nodeUUID":"node-1",
				"hostname":"gpu-001",
				"agentType":"inband",
				"healthStatus":"Healthy",
				"agentStatus":"Online"
			}`))
		case "oob":
			_, _ = w.Write([]byte(`{
				"nodeUUID":"node-1",
				"agentType":"oob",
				"bmcHostname":"bmc-001",
				"bmcIP":"192.0.2.10:443",
				"healthStatus":"Degraded",
				"oobInventory":{
					"schemaVersion":"inventory.v1alpha1",
					"source":{"sourceType":"redfish","vendor":"NVIDIA"}
				}
			}`))
		default:
			t.Errorf("unexpected agentType: %q", r.URL.Query().Get("agentType"))
			http.Error(w, "missing agentType", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

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
	if combined.OOB["bmcHostname"] != "bmc-001" {
		t.Fatalf("unexpected OOB JSON: %#v", combined.OOB)
	}
}

// Verifies a missing agent-specific view does not hide the available view
func TestNodeDescribeCombinedViewAllowsNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("agentType") == "oob" {
			http.Error(w, `{"error":"node does not have an OOB agent"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nodeUUID":"node-1",
			"hostname":"gpu-001",
			"agentType":"inband"
		}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("agentType"); got != "oob" {
			t.Fatalf("unexpected agentType: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nodeUUID":"node-oob-1",
			"hostname":"host-001",
			"agentType":"oob",
			"bmcHostname":"bmc-001",
			"bmcIP":"192.0.2.10",
			"oobInventory":{
				"collectedAt":"2026-07-30T20:00:00Z",
				"schemaVersion":"inventory.v1alpha1",
				"source":{"sourceType":"redfish","vendor":"Dell","address":"192.0.2.10:443","mac":"00:11:22:33:44:55","redfishVersion":"1.17.0"},
				"systems":[{"id":"System.Embedded.1","uuid":"system-uuid","manufacturer":"Dell","model":"PowerEdge XE9680","sku":"sku-1","serialNumber":"serial-1","biosVersion":"2.1.0","hostName":"host-001","assetTag":"asset-1","powerState":"on","statusState":"Enabled","health":"OK","healthRollup":"Warning","cpuCount":2,"memoryGib":2048,"secureBootEnabled":true,"processors":[{"id":"CPU.Socket.1","socket":"CPU 1","processorType":"cpu","processorArchitecture":"x86","manufacturer":"Intel","model":"Xeon","maxSpeedMhz":3800,"totalCores":56,"totalThreads":112,"statusState":"Enabled","health":"OK","healthRollup":"OK"},{"id":"GPU.Slot.1","processorType":"gpu","manufacturer":"NVIDIA","model":"NVIDIA H100","statusState":"Enabled","health":"OK","healthRollup":"OK"}]}],
				"managers":[{"id":"iDRAC.Embedded.1","uuid":"manager-uuid","model":"iDRAC","managerType":"bmc","firmwareVersion":"7.10.00.00","statusState":"Enabled","health":"OK","healthRollup":"OK"}],
				"chassis":[{"id":"System.Embedded.1","chassisType":"rack_mount","manufacturer":"Dell","model":"XE9680","sku":"sku-1","serialNumber":"serial-1","partNumber":"part-1","assetTag":"asset-1","powerState":"on","statusState":"Enabled","health":"OK","healthRollup":"Warning","pcieDevices":[{"id":"GPU.Slot.1","uuid":"gpu-uuid","deviceType":"single_function","manufacturer":"NVIDIA","model":"NVIDIA H100","sku":"gpu-sku","serialNumber":"gpu-serial","partNumber":"gpu-part","firmwareVersion":"96.00.00","statusState":"Enabled","health":"OK","healthRollup":"OK"}]}],
				"firmware":[{"id":"BIOS","name":"System BIOS","serviceId":"fw-service","version":"1.2.3","releaseDate":"2026-01-01","statusState":"Enabled","health":"OK","healthRollup":"Warning"}],
				"domainErrors":[{"domain":"storage","resource":"Disk.1","message":"collection failed"}]
			}
		}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-oob-1", "--agent-type", "oob"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"FIELD", "VALUE",
		"BMC HOSTNAME", "bmc-001",
		"INVENTORY SCHEMA VERSION", "inventory.v1alpha1",
		"SOURCE ADDRESS", "192.0.2.10:443", "SOURCE MAC", "00:11:22:33:44:55", "SOURCE VENDOR", "Dell",
		"INVENTORY DOMAIN ERROR 1", "storage: Disk.1: collection failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
	for _, unwanted := range []string{
		"\nHOSTNAME ", "\nAGENT TYPE ", "\nGPU TYPE ", "\nGPU COUNT ",
		"\nFIRMWARE CHECK ", "\nPUBLIC IP ", "\nPRIVATE IP ",
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
		"node", "describe", "node-oob-1", "--agent-type", "oob",
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
		"FIELD", "INVENTORY SCHEMA VERSION", "SOURCE ADDRESS", "BMC HOSTNAME",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("section output unexpectedly contains summary value %q: %q", unwanted, got)
		}
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"node", "describe", "node-oob-1", "--agent-type", "oob", "--section", "all",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("all sections command failed: %v", err)
	}

	got = out.String()
	for _, want := range []string{
		"FIELD", "INVENTORY SCHEMA VERSION", "SOURCE ADDRESS",
		"\nManagers\n", "iDRAC.Embedded.1", "manager-uuid", "7.10.00.00",
		"\nSystems\n", "\nCPUs\n", "\nGPUs\n",
		"\nChassis\n", "\nPCIe Devices\n",
		"\nFirmware\n", "System BIOS", "1.2.3", "2026-01-01", "Enabled", "OK", "Warning",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("all sections output missing %q: %q", want, got)
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
			args: []string{"node", "describe", "node-1", "--agent-type", "oob", "--section", "storage"},
			want: "invalid OOB inventory section",
		},
		{
			name: "all with another section",
			args: []string{
				"node", "describe", "node-1", "--agent-type", "oob",
				"--section", "all,systems",
			},
			want: "section all cannot be combined with other sections",
		},
		{
			name: "inband section",
			args: []string{
				"node", "describe", "node-1", "--agent-type", "inband",
				"--section", "systems",
			},
			want: "--section requires the OOB view",
		},
		{
			name: "json section",
			args: []string{
				"node", "describe", "node-1", "--agent-type", "oob",
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
		{UUID: "node-1", Hostname: "gpu-001", BMCHostname: "bmc-001", BMCIP: "192.0.2.10:443"},
	})
	if err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"UUID", "HOSTNAME", "BMC HOSTNAME", "BMC IP",
		"node-1", "gpu-001", "bmc-001", "192.0.2.10:443",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

func TestOOBDetailNodeRowsOmitInbandHostname(t *testing.T) {
	rows := oobDetailNodeRows([]nvfleetint.Node{{
		UUID:        "node-1",
		Hostname:    "inband-hostname",
		BMCHostname: "bmc-001",
		BMCIP:       "192.0.2.10:443",
	}})

	if len(rows) != 1 || len(rows[0]) != 8 {
		t.Fatalf("unexpected OOB row shape: %#v", rows)
	}
	if slices.Contains(rows[0], "inband-hostname") {
		t.Fatalf("OOB row contains in-band hostname: %#v", rows[0])
	}
	if rows[0][1] != "bmc-001" || rows[0][2] != "192.0.2.10:443" {
		t.Fatalf("unexpected OOB identity columns: %#v", rows[0])
	}
}

func TestOOBNodeDescribeRowsOmitInbandFields(t *testing.T) {
	rows := oobNodeDescribeRows(nvfleetint.NodeDetails{
		Node: nvfleetint.Node{
			UUID:          "node-1",
			Hostname:      "inband-hostname",
			AgentType:     "oob",
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
		"HOSTNAME", "AGENT TYPE", "GPU TYPE", "GPU COUNT",
		"FIRMWARE CHECK", "PUBLIC IP", "PRIVATE IP",
	} {
		if slices.Contains(labels, unwanted) {
			t.Fatalf("OOB summary contains in-band field %q: %#v", unwanted, labels)
		}
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

	body := `{"filters":{"fields":[` +
		`{"name":"computeZones","options":[{"id":"cz-1","value":"East","options":[{"id":"ng-1","value":"Training"}]}]},` +
		`{"name":"gpuTypes","options":["NVIDIA-H100"]},` +
		`{"name":"brandNewFilter","options":["surprise"]}` +
		`]},"sorting":{"fields":["hostname","nodeGroup","computeZone","integrityCheck"],"orders":["asc","desc"],"defaults":{"field":"hostname","order":"asc"}}}`

	var agentTypes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		agentTypes = append(agentTypes, r.URL.Query().Get("agentType"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var tableOut bytes.Buffer
	tableCmd := newRootCmd()
	tableCmd.SetOut(&tableOut)
	tableCmd.SetArgs([]string{"node", "options", "--agent-type", "oob"})
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
		// The backend spells three sort fields differently from the CLI.
		"nodegroup", "computezone", "verificationCheck",
		"(default: hostname)", "(default: asc)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"nodeGroup,", "computeZone,", "integrityCheck"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("table output should not offer backend spelling %q:\n%s", unwanted, got)
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
	jsonCmd.SetArgs([]string{"node", "options", "--output", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("node options JSON command failed: %v", err)
	}
	if strings.TrimSpace(jsonOut.String()) != body {
		t.Fatalf("JSON output is not the raw payload:\n%s", jsonOut.String())
	}
	if len(agentTypes) != 2 || agentTypes[0] != "oob" || agentTypes[1] != "" {
		t.Fatalf("unexpected agentType values: %#v", agentTypes)
	}
}

// Verifies an unsupported agent type is rejected before any request.
func TestNodeOptionsRejectsAgentType(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"node", "options", "--agent-type", "bmc"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid agent-type") {
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

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "options"})
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
	computeZones := sectionBody(t, got, "--compute-zone-ids")
	if strings.Contains(computeZones, "ng-1") {
		t.Fatalf("node group listed under --compute-zone-ids:\n%s", computeZones)
	}
	if !strings.Contains(computeZones, "Values nested under these are listed under --nodegroup-ids.") {
		t.Fatalf("--compute-zone-ids section does not point at --nodegroup-ids:\n%s", computeZones)
	}
}

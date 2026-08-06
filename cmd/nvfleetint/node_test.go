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
	cmd.SetArgs([]string{"node", "list", "--output", "json", "--sort-by", "healthStatus"})

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
			cmd.SetArgs([]string{"node", "list", "--output", "json", "--sort-by", field})

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

	common := resolvedCommonFlags{output: "table", pageSize: 20, timeout: nvfleetint.DefaultTimeout}
	err := validateNodeListFlags(nodeListFlags{view: "detail", sortBy: "bogus"}, "bogus", common)
	if err == nil {
		t.Fatal("expected invalid sort-by error")
	}
	if !strings.Contains(err.Error(), "verificationCheck") || strings.Contains(err.Error(), "integrityCheck") {
		t.Fatalf("unexpected sort-by error: %v", err)
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
	cmd.SetArgs([]string{"node", "list", "--node-uuids", "node-1,node-2", "--health", "Healthy,Degraded", "--hostname", "gpu", "--agent-status", "Online", "--verification-check", "Verified", "--firmware-check", "Unknown", "--compute-zone-ids", "cz-1,cz-2", "--compute-zone-names", "East", "--nodegroup-ids", "ng-1", "--nodegroup-names", "Training", "--gpu-type", "NVIDIA-H100", "--gpu-count", "8,4", "--public-ip", "203.0.113.10", "--private-ip", "10.0.0.10", "--sort-by", "computezone", "--order", "desc", "--page-size", "10"})

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
	cmd.SetArgs([]string{"node", "list", "--all", "--output", "json"})

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
	cmd.SetArgs([]string{"node", "list", "--all", "--output", "json", "--timeout", "5s"})
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
	cmd.SetArgs([]string{"node", "describe", "node-1"})

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

// Verifies basic node rows
func TestWriteNodeBasicTable(t *testing.T) {
	var out bytes.Buffer
	err := writeNodeTable(&out, string(nvfleetint.NodeViewBasic), []nvfleetint.Node{
		{UUID: "node-1", Hostname: "gpu-001"},
	})
	if err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "node-1", "gpu-001"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
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
		{name: "basic sort", args: []string{"node", "list", "--view", "basic", "--sort-by", "healthStatus"}, want: "basic node view is incompatible"},
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

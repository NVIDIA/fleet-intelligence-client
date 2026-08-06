// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verifies all-page alert JSON output
func TestAlertListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.URL.Query().Get("nodeUUID"); got != "node-1" {
			t.Fatalf("unexpected nodeUUID: %q", got)
		}
		if got := r.URL.Query().Get("severity"); got != "Critical" {
			t.Fatalf("unexpected severity: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"gpu","severity":"Critical","state":"Triggered","triggeredAt":"2026-05-01T00:00:00Z","extra":"kept"}],"page":1,"pageSize":1,"total":2}`))
		case "2":
			requests++
			_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-2","nodeUUID":"node-1","component":"memory","severity":"Critical","state":"Triggered","triggeredAt":"2026-05-01T00:01:00Z"}],"page":2,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "list", "--node", "node-1", "--severity", "Critical", "--all", "--output", "json", "--page-size", "1"})

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
	if len(got.Items) != 2 || got.Items[0]["alertUUID"] != "alert-1" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 1 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies table alert rows and derived has-more metadata
func TestAlertListTableAndHasMore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		// --page 1 (1-based) maps to the SDK's 0-based page 0, which the alerts
		// API sees as its 1-based page 1.
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := r.URL.Query().Get("component"); got != "gpu" {
			t.Fatalf("unexpected component: %q", got)
		}
		if got := r.URL.Query().Get("state"); got != "Triggered" {
			t.Fatalf("unexpected state: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"gpu","severity":"Warning","state":"Detected","detectedAt":"2026-05-01T00:00:00Z"}],"page":1,"pageSize":1,"total":2}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "list", "--page", "1", "--component", "gpu", "--state", "Triggered"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "NODE UUID", "COMPONENT", "SEVERITY", "STATE", "FIRED-AT", "alert-1", "Warning", "Page: 1  Total Pages: 2  Page Size: 1  Total Entries: 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies fleet and node timeline table paths
func TestAlertTimelineTables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/alert_timeline/nodes":
			query := r.URL.Query()
			if got := query.Get("active"); got != "true" {
				t.Fatalf("unexpected active: %q", got)
			}
			for name, want := range map[string]string{"hostname": "gpu", "sortBy": "alert", "order": "desc"} {
				if got := query.Get(name); got != want {
					t.Fatalf("unexpected %s: %q", name, got)
				}
			}
			if got := query["componentTypes"]; len(got) != 2 || got[0] != "gpu" || got[1] != "memory" {
				t.Fatalf("unexpected componentTypes: %#v", got)
			}
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUuid":"node-1","hostname":"gpu-001","criticalCount":2,"warningCount":1,"gpuType":"H100","nodeGroup":"Training","computeZone":"East","lastAlertTime":"2026-05-01T00:00:00Z"}],"hasMore":false,"page":0,"pageSize":50,"total":1,"totalCritical":2,"totalWarning":1}`))
		case "/v1/alert_timeline/nodes/node-1/alerts":
			query := r.URL.Query()
			for name, want := range map[string]string{"withoutPsirt": "true", "sortBy": "startTime", "order": "asc"} {
				if got := query.Get(name); got != want {
					t.Fatalf("unexpected %s: %q", name, got)
				}
			}
			_, _ = w.Write([]byte(`{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-1","component":"gpu","alertStatus":"Resolved","startTime":"2026-04-30T00:00:00Z","lastEventTime":"2026-05-01T00:00:00Z"}],"hasMore":false,"page":0,"pageSize":50,"total":1}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "timeline", "--active", "--hostname", "gpu", "--sort-by", "alert", "--order", "desc", "--component-type", "gpu,memory"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("timeline command failed: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "NODE UUID") || !strings.Contains(got, "CRITICAL") || !strings.Contains(got, "WARNING") || !strings.Contains(got, "H100") || !strings.Contains(got, "Training") || !strings.Contains(got, "East") || !strings.Contains(got, "node-1") {
		t.Fatalf("timeline node output missing fields: %q", got)
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "timeline", "--node", "node-1", "--without-psirt", "--sort-by", "startTime", "--order", "asc"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("node timeline command failed: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "ALERT UUID") || !strings.Contains(got, "STATUS") || !strings.Contains(got, "START TIME") || strings.Contains(got, "SEVERITY") || !strings.Contains(got, "alert-1") || !strings.Contains(got, "Resolved") || !strings.Contains(got, "2026-04-30T00:00:00Z") {
		t.Fatalf("timeline alert output missing fields: %q", got)
	}
}

// Verifies level-1 --all JSON preserves backend cross-page aggregates
func TestAlertTimelineAllJSONPreservesAggregates(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUuid":"node-1","criticalCount":2,"warningCount":1}],"hasMore":false,"page":0,"pageSize":100,"total":1,"totalCritical":2,"totalWarning":1,"totalResolved":0,"distinctGpuTypeCount":1,"distinctNodeGroupCount":1,"distinctComputeZoneCount":1}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "timeline", "--active", "--all", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("timeline command failed: %v", err)
	}

	var got struct {
		Items                    []map[string]any `json:"items"`
		TotalCritical            int              `json:"totalCritical"`
		TotalWarning             int              `json:"totalWarning"`
		DistinctComputeZoneCount int              `json:"distinctComputeZoneCount"`
		Pagination               struct {
			Page int `json:"page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if len(got.Items) != 1 || got.TotalCritical != 2 || got.TotalWarning != 1 || got.DistinctComputeZoneCount != 1 || got.Pagination.Page != 1 {
		t.Fatalf("unexpected timeline JSON: %#v", got)
	}
}

// Verifies node-alert --all JSON uses the CLI's 1-based pagination contract.
func TestNodeAlertTimelineAllJSONNormalizesPagination(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-1","component":"gpu","alertStatus":"Critical"}],"hasMore":false,"page":0,"pageSize":100,"total":1}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "timeline", "--node", "node-1", "--active", "--all", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("timeline command failed: %v", err)
	}

	var got struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			Page int `json:"page"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if len(got.Items) != 1 || got.Items[0]["alertUuid"] != "alert-1" || got.Pagination.Page != 1 {
		t.Fatalf("unexpected timeline JSON: %#v", got)
	}
}

// Verifies the CLI exposes alert timeline filter options in JSON and table formats.
func TestAlertTimelineOptionsOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/filter_options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requests++
		wantActive := requests == 1
		if got := r.URL.Query().Get("active"); got != map[bool]string{true: "true", false: "false"}[wantActive] {
			t.Fatalf("unexpected active value on request %d: %q", requests, got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"filters":{"fields":[{"name":"gpuTypes","options":["H100"]},{"name":"nodeGroups","options":[{"id":"ng-1","value":"Training"}]}]},"sorting":{"fields":["alert","hostname"],"orders":["asc","desc"],"defaults":{"field":"alert","order":"desc"}}}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")
	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"alert", "timeline", "options", "--active", "--output", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("timeline options JSON command failed: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(jsonOut.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, jsonOut.String())
	}
	if got["filters"] == nil || got["sorting"] == nil {
		t.Fatalf("unexpected options JSON: %#v", got)
	}

	var tableOut bytes.Buffer
	tableCmd := newRootCmd()
	tableCmd.SetOut(&tableOut)
	tableCmd.SetArgs([]string{"alert", "timeline", "options"})
	if err := tableCmd.Execute(); err != nil {
		t.Fatalf("timeline options table command failed: %v", err)
	}
	for _, want := range []string{"FILTER", "gpuTypes", "H100", "nodeGroups", "ng-1", "Training", "SORTING", "DEFAULT FIELD", "alert"} {
		if !strings.Contains(tableOut.String(), want) {
			t.Fatalf("table output missing %q:\n%s", want, tableOut.String())
		}
	}
}

// Verifies alert describe timeline output
func TestAlertDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("order") != "asc" || query.Get("page") != "1" || query.Get("pageSize") != "1" {
			t.Fatalf("unexpected query: %v", query)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","page":1,"pageSize":1,"total":3,"hasMore":true,"timeline":[{"eventType":"triggered","alertStatus":"Critical","eventTimestamp":"2026-05-01T00:00:00Z","message":"GPU critical"}]}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1", "--order", "asc", "--page", "2", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "TIMESTAMP") || !strings.Contains(got, "triggered") || !strings.Contains(got, "GPU critical") || !strings.Contains(got, "Page: 2  Total Pages: 3") {
		t.Fatalf("describe output missing fields: %q", got)
	}
}

// Verifies long free-text columns are truncated in the table while JSON keeps
// the full payload
func TestAlertDescribeTruncatesLongMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	longMessage := strings.Repeat("x", 200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","timeline":[{"eventType":"triggered","alertStatus":"Critical","eventTimestamp":"2026-05-01T00:00:00Z","message":"` + longMessage + `"}]}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var table bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&table)
	cmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if got := table.String(); strings.Contains(got, longMessage) {
		t.Fatalf("expected truncated message in table, got %q", got)
	}
	if got := table.String(); !strings.Contains(got, "…") {
		t.Fatalf("expected ellipsis in truncated table, got %q", got)
	}

	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1", "-o", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("describe json failed: %v", err)
	}
	if got := jsonOut.String(); !strings.Contains(got, longMessage) {
		t.Fatalf("expected full message in json output, got %q", got)
	}
}

// Verifies node context is required
func TestAlertDescribeRequiresNode(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "describe", "alert-1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing node error")
	}
	if !strings.Contains(err.Error(), "--node is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies alert flag validation
func TestAlertListRejectsInvalidSeverity(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "list", "--severity", "Fatal"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid severity error")
	}
	if !strings.Contains(err.Error(), "invalid severity") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies alert state flag validation
func TestAlertListRejectsInvalidState(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "list", "--state", "Pending"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid state error")
	}
	if !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies alert list uses the shared 1-based page rule and rejects out-of-range
func TestAlertListRejectsNegativePage(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "list", "--page=0"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected page error")
	}
	if !strings.Contains(err.Error(), "--page must be greater than or equal to 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies mode-specific timeline flags are rejected before a request
func TestAlertTimelineRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "node sort", args: []string{"alert", "timeline", "--sort-by", "startTime"}, want: "invalid sort-by"},
		{name: "alert sort", args: []string{"alert", "timeline", "--node", "node-1", "--sort-by", "alert"}, want: "invalid sort-by"},
		{name: "without psirt", args: []string{"alert", "timeline", "--without-psirt"}, want: "requires --node"},
		{name: "hostname", args: []string{"alert", "timeline", "--node", "node-1", "--hostname", "gpu"}, want: "cannot be used with --node"},
		{name: "state", args: []string{"alert", "timeline", "--alert-state", "Triggered"}, want: "invalid alert-state"},
		{name: "describe page", args: []string{"alert", "describe", "alert-1", "--node", "node-1", "--page", "2"}, want: "requires --page-size"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

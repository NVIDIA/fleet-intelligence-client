package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab-master.nvidia.com/gpu-health/fleet-intelligence-client-go/internal/config"
)

// Verifies inventory report table output
func TestReportInventoryTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/inventory" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.URL.Query().Get("format"); got != "json" {
			t.Fatalf("unexpected format: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","gpuType":"NVIDIA-H100","gpuCount":8,"integrityCheck":"Verified","firmwareCheck":"Passed","publicIP":"203.0.113.10","privateIP":"10.0.0.10"}],"hasMore":false,"page":0,"pageSize":10,"total":1}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "GPU TYPE", "GPU COUNT", "INTEGRITY CHECK", "FIRMWARE CHECK", "node-1", "gpu-001", "NVIDIA-H100", "Page: 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies inventory report CSV output
func TestReportInventoryCSV(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	raw := "nodeUUID,hostname\nnode-1,gpu-001\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/inventory" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "text/csv" {
			t.Fatalf("unexpected accept header: %q", got)
		}
		if got := r.URL.Query().Get("format"); got != "csv" {
			t.Fatalf("unexpected format: %q", got)
		}

		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--format", "csv"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if out.String() != raw {
		t.Fatalf("unexpected csv output: %q", out.String())
	}
}

// Verifies an explicitly set --output is rejected with --format csv
func TestReportInventoryRejectsOutputWithCSV(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := config.Save(config.Config{APIURL: "http://example.invalid", ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	for _, output := range []string{"table", "json"} {
		t.Run(output, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs([]string{"report", "inventory", "--format", "csv", "--output", output})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "--output cannot be used with --format csv") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies all-page error list reports merge raw JSON items
func TestReportErrorListAllJSONMergesItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "list" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("groupBy"); got != "error" {
			t.Fatalf("unexpected groupBy: %q", got)
		}
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "24h" {
			t.Fatalf("unexpected window: %q", got)
		}
		if got := query.Get("pageSize"); got != "1" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch query.Get("page") {
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"errors":[{"name":"xid_154","count":1,"extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"errors":[{"name":"NVSwitch Fatal Error","count":2}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", query.Get("page"))
		}
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--all", "--output", "json", "--page-size", "1"})

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
	if len(got.Items) != 2 || got.Items[0]["name"] != "xid_154" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 0 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies overview error report table output and absolute time filters
func TestReportErrorOverviewTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	start := "2026-05-01T00:00:00Z"
	end := "2026-05-02T00:00:00Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "overview" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("timeMode"); got != "absolute" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("startTime"); got != start {
			t.Fatalf("unexpected startTime: %q", got)
		}
		if got := query.Get("endTime"); got != end {
			t.Fatalf("unexpected endTime: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalErrors":1250,"totalErrorNodes":42,"totalErrorTypes":15}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "overview", "--start", start, "--end", end})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"FIELD", "VALUE", "TOTAL ERRORS", "1250", "TOTAL ERROR NODES", "42", "TOTAL ERROR TYPES", "15"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies the error report defaults to the overview view when --view is omitted
func TestReportErrorDefaultsToOverview(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "overview" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("groupBy"); got != "" {
			t.Fatalf("unexpected groupBy: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"totalErrors":1250,"totalErrorNodes":42,"totalErrorTypes":15}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--window", "24h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if !strings.Contains(out.String(), "TOTAL ERRORS") {
		t.Fatalf("expected overview table output: %q", out.String())
	}
}

// Verifies seven-day hour windows pass through unchanged
func TestReportErrorGraphSevenDayHourWindow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "graph" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("groupBy"); got != "error" {
			t.Fatalf("unexpected groupBy: %q", got)
		}
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "168h" {
			t.Fatalf("unexpected window: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"label":{"error":"xid_154"},"values":[[1716153600,5]]}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-08T00:00:00Z"}}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "graph", "--window", "168h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ERROR", "VALUES", "START", "END", "xid_154", "[[1716153600,5]]", "2026-05-01T00:00:00Z", "2026-05-08T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies backend-style hour windows pass through unchanged
func TestReportErrorGraphThreeHourWindow(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "graph" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query.Get("groupBy"); got != "error" {
			t.Fatalf("unexpected groupBy: %q", got)
		}
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "3h" {
			t.Fatalf("unexpected window: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[],"timeRange":{"start":"2026-06-03T17:00:00Z","end":"2026-06-03T20:00:00Z"}}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "graph", "--window", "3h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(out.String(), "ERROR") {
		t.Fatalf("expected graph table header: %q", out.String())
	}
}

// Verifies local error report flag validation
func TestReportErrorRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default view needs time range", args: []string{"report", "error"}, want: "a time range is required"},
		{name: "empty view", args: []string{"report", "error", "--view", ""}, want: "--view is required"},
		{name: "missing group", args: []string{"report", "error", "--view", "list"}, want: "--group-by is required"},
		{name: "bad view", args: []string{"report", "error", "--view", "bad"}, want: "invalid view"},
		{name: "bad group", args: []string{"report", "error", "--view", "list", "--group-by", "bad"}, want: "invalid group-by"},
		{name: "graph node group", args: []string{"report", "error", "--view", "graph", "--group-by", "node"}, want: "only supports --group-by error"},
		{name: "missing time range", args: []string{"report", "error", "--view", "graph"}, want: "a time range is required"},
		{name: "missing time range list", args: []string{"report", "error", "--view", "list", "--group-by", "error"}, want: "a time range is required"},
		{name: "overview group", args: []string{"report", "error", "--view", "overview", "--group-by", "error"}, want: "--group-by cannot be used"},
		{name: "csv overview", args: []string{"report", "error", "--view", "overview", "--format", "csv"}, want: "--format csv is only supported"},
		{name: "csv with output table", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--format", "csv", "--output", "table"}, want: "--output cannot be used with --format csv"},
		{name: "csv with output json", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--format", "csv", "--output", "json"}, want: "--output cannot be used with --format csv"},
		{name: "bad window", args: []string{"report", "error", "--view", "overview", "--window", "soon"}, want: "invalid window"},
		{name: "day window", args: []string{"report", "error", "--view", "overview", "--window", "7d"}, want: "invalid window"},
		{name: "huge window", args: []string{"report", "error", "--view", "overview", "--window", "22342394090s"}, want: "duration is too large"},
		{name: "start alone", args: []string{"report", "error", "--view", "overview", "--start", "2026-05-01T00:00:00Z"}, want: "--start and --end"},
		{name: "bad start", args: []string{"report", "error", "--view", "overview", "--start", "yesterday", "--end", "2026-05-01T00:00:00Z"}, want: "--start must be RFC3339"},
		{name: "graph page", args: []string{"report", "error", "--view", "graph", "--page", "1"}, want: "pagination flags"},
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
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

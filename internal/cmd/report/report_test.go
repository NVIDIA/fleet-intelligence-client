// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"
)

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

// Changes the working directory for the duration of a test
func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore chdir failed: %v", err)
		}
	})
}

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
		query := r.URL.Query()
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1", "cz-2"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1"}) {
			t.Fatalf("unexpected nodeGroupIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["tags"]; !slices.Equal(got, []string{"prod", "h100"}) {
			t.Fatalf("unexpected tags: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("startTime"); got != "2026-05-01T00:00:00Z" {
			t.Fatalf("unexpected startTime: %q", got)
		}
		if got := query.Get("endTime"); got != "2026-05-02T00:00:00Z" {
			t.Fatalf("unexpected endTime: %q", got)
		}
		if got := query.Get("sortBy"); got != "hostname" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		if got := query.Get("order"); got != "asc" {
			t.Fatalf("unexpected order: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","gpuType":"NVIDIA-H100","gpuCount":8,"integrityCheck":"Verified","firmwareCheck":"Passed","publicIP":"203.0.113.10","privateIP":"10.0.0.10"}],"hasMore":false,"page":0,"pageSize":10,"total":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--compute-zone-ids", "cz-1,cz-2", "--nodegroup-ids", "ng-1", "--tags", "prod,h100", "--start", "2026-05-01T00:00:00Z", "--end", "2026-05-02T00:00:00Z", "--sort-by", "hostname", "--order", "asc"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "GPU TYPE", "GPU COUNT", "VERIFICATION CHECK", "FIRMWARE CHECK", "node-1", "gpu-001", "NVIDIA-H100", "Page: 1"} {
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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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

	cmdtest.SaveConfig(t, "https://example.invalid", "test-key")

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

// Verifies local inventory report flag validation
func TestReportInventoryRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "compute zone IDs", args: []string{"report", "inventory", "--compute-zone-ids", "cz-1,,cz-2"}, want: "empty values are not allowed"},
		{name: "start alone", args: []string{"report", "inventory", "--start", "2026-05-01T00:00:00Z"}, want: "--start and --end must be used together"},
		{name: "bad start", args: []string{"report", "inventory", "--start", "yesterday", "--end", "2026-05-02T00:00:00Z"}, want: "--start must be RFC3339"},
		{name: "bad sort", args: []string{"report", "inventory", "--sort-by", "name"}, want: "invalid sort-by"},
		{name: "bad order", args: []string{"report", "inventory", "--order", "up"}, want: "invalid order"},
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

// Verifies a signed inventory report is written to the current directory
func TestReportInventorySignedWritesToCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "application/zip" {
			t.Fatalf("unexpected accept header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("format"); got != "csv" {
			t.Fatalf("unexpected format: %q", got)
		}
		if got := query.Get("signed"); got != "true" {
			t.Fatalf("unexpected signed: %q", got)
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	dir := t.TempDir()
	chdir(t, dir)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--format", "csv", "--signed"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, "fleet-inventory.zip"))
	if err != nil {
		t.Fatalf("read written file failed: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("unexpected file contents: %q", string(written))
	}
	if !strings.Contains(out.String(), "fleet-inventory.zip") {
		t.Fatalf("output missing written path: %q", out.String())
	}
}

func TestReportInventorySignedJSONStatus(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	dir := t.TempDir()
	chdir(t, dir)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--format", "csv", "--signed", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got signedReportOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode signed report JSON failed: %v", err)
	}
	if got.Status != "written" || !strings.HasSuffix(got.Path, "fleet-inventory.zip") {
		t.Fatalf("unexpected signed report JSON: %#v", got)
	}
}

// Verifies --output-path directs the signed bundle to an explicit file
func TestReportInventorySignedOutputPath(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	dir := t.TempDir()
	target := filepath.Join(dir, "custom-name.zip")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--format", "csv", "--signed", "--output-path", target})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read written file failed: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("unexpected file contents: %q", string(written))
	}
}

// Verifies --output-path pointing at a directory keeps the suggested filename
func TestReportInventorySignedOutputPathDirectory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	dir := t.TempDir()

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "inventory", "--format", "csv", "--signed", "--output-path", dir})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	written, err := os.ReadFile(filepath.Join(dir, "fleet-inventory.zip"))
	if err != nil {
		t.Fatalf("read written file failed: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Fatalf("unexpected file contents: %q", string(written))
	}
}

// Verifies signed inventory flag combinations are rejected before any request
func TestReportInventorySignedValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cmdtest.SaveConfig(t, "https://example.invalid", "test-key")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "signed without csv", args: []string{"report", "inventory", "--signed"}, want: "--signed requires --format csv"},
		{name: "signed with json", args: []string{"report", "inventory", "--format", "json", "--signed"}, want: "--signed requires --format csv"},
		{name: "output-path without signed", args: []string{"report", "inventory", "--format", "csv", "--output-path", "out.zip"}, want: "--output-path can only be used with --signed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
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
		if got := query.Get("groupBy"); got != "node" {
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
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1"}) {
			t.Fatalf("unexpected nodeGroupIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["tags"]; !slices.Equal(got, []string{"prod"}) {
			t.Fatalf("unexpected tags: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["errors"]; !slices.Equal(got, []string{"xid_154"}) {
			t.Fatalf("unexpected errors: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["severities"]; !slices.Equal(got, []string{"Critical", "Fatal"}) {
			t.Fatalf("unexpected severities: %#v raw query %q", got, r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		switch query.Get("page") {
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","errors":["xid_154"],"extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2","hostname":"gpu-002","errors":["NVSwitch Fatal Error"]}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", query.Get("page"))
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "list", "--group-by", "node", "--window", "24h", "--compute-zone-ids", "cz-1", "--nodegroup-ids", "ng-1", "--tags", "prod", "--errors", "xid_154", "--severities", "Critical,Fatal", "--all", "--output", "json", "--page-size", "1"})

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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
		if got := query.Get("step"); got != "5m" {
			t.Fatalf("unexpected step: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"label":{"error":"xid_154"},"values":[[1716153600,5]]}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-08T00:00:00Z"}}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "error", "--view", "graph", "--window", "168h", "--step", "5m"})

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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
		{name: "bad compute zone IDs", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--compute-zone-ids", "cz-1,,cz-2"}, want: "empty values are not allowed"},
		{name: "bad severities", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--severities", "Critical,Broken"}, want: "invalid severity"},
		{name: "errors with group by error", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--errors", "xid_154"}, want: "--errors can only be used"},
		{name: "step overview", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--step", "5m"}, want: "--step can only be used with --view graph"},
		{name: "short step", args: []string{"report", "error", "--view", "graph", "--window", "24h", "--step", "30s"}, want: "expected at least 1m"},
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

// writeSignedFixture signs csv and writes the CSV, bundle, and public key into
// dir, returning their paths.
func writeSignedFixture(t *testing.T, dir string, csv []byte) (csvPath, bundlePath, keyPath string) {
	t.Helper()

	keypair, err := sign.NewEphemeralKeypair(nil)
	if err != nil {
		t.Fatalf("new keypair failed: %v", err)
	}
	pem, err := keypair.GetPublicKeyPem()
	if err != nil {
		t.Fatalf("get public key failed: %v", err)
	}
	pb, err := sign.Bundle(&sign.PlainData{Data: csv}, keypair, sign.BundleOptions{})
	if err != nil {
		t.Fatalf("sign bundle failed: %v", err)
	}
	signed, err := bundle.NewBundle(pb)
	if err != nil {
		t.Fatalf("wrap bundle failed: %v", err)
	}
	bundleJSON, err := signed.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal bundle failed: %v", err)
	}

	csvPath = filepath.Join(dir, "inventory.csv")
	bundlePath = filepath.Join(dir, "inventory.sig.bundle")
	keyPath = filepath.Join(dir, "signing-key.pub")
	for path, data := range map[string][]byte{csvPath: csv, bundlePath: bundleJSON, keyPath: []byte(pem)} {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("write fixture %s failed: %v", path, err)
		}
	}
	return csvPath, bundlePath, keyPath
}

// Verifies a signed report passes verification with an offline --key
func TestReportVerifySucceedsWithKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath, "--key", keyPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(out.String(), "Verified OK") {
		t.Fatalf("output missing success message: %q", out.String())
	}
}

func TestReportVerifySucceedsWithKeyJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath, "--key", keyPath, "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got commandStatusOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode verify JSON failed: %v", err)
	}
	if got.Status != "verified" {
		t.Fatalf("unexpected verify JSON: %#v", got)
	}
}

// Verifies a tampered CSV fails the verify command
func TestReportVerifyTamperedFails(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
	if err := os.WriteFile(csvPath, []byte("customer,issued_at\nevil,2026-06-15T00:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("tamper csv failed: %v", err)
	}

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath, "--key", keyPath})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected verification to fail for tampered csv")
	}
}

// Verifies the signing key is fetched from the API when --key is omitted
func TestReportVerifyFetchesKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read key fixture failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/signing-key.pub" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write(keyPEM)
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if !strings.Contains(out.String(), "Verified OK") {
		t.Fatalf("output missing success message: %q", out.String())
	}
}

// Verifies a clear message when a non-bundle file is passed to --bundle
func TestReportVerifyRejectsNonBundle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	csvPath, _, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	// Point --bundle at the CSV instead of the .sig.bundle file.
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", csvPath, "--key", keyPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not a valid signature bundle") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "proto:") {
		t.Fatalf("error leaks internal details: %v", err)
	}
}

// Verifies a clear message when the report does not match the signature
func TestReportVerifyMismatchMessage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
	// Point --csv at the bundle file: a valid bundle, but not the signed artifact.
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--csv", bundlePath, "--bundle", bundlePath, "--key", keyPath})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "verification failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(err.Error(), "ASN.1") {
		t.Fatalf("error leaks internal details: %v", err)
	}
	_ = csvPath
}

// Verifies a clear message when a flag points at a missing file
func TestReportVerifyMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	dir := t.TempDir()
	csvPath, bundlePath, _ := writeSignedFixture(t, dir, []byte("data\n"))

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath, "--key", "test-key"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `--key file "test-key" does not exist`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies required flags are enforced
func TestReportVerifyRequiresFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"missing csv", []string{"report", "verify", "--bundle", "b.sig.bundle"}, "--csv is required"},
		{"missing bundle", []string{"report", "verify", "--csv", "report.csv"}, "--bundle is required"},
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

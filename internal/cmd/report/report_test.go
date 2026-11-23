// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/sign"
	"github.com/spf13/cobra"
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","gpuType":"NVIDIA-H100","gpuCount":8,"verificationCheck":"Verified","firmwareCheck":"Passed","publicIP":"203.0.113.10","privateIP":"10.0.0.10"}],"hasMore":false,"page":0,"pageSize":10,"total":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--compute-zone-ids", "cz-1,cz-2", "--nodegroup-ids", "ng-1", "--tags", "prod,h100", "--start", "2026-05-01T00:00:00Z", "--end", "2026-05-02T00:00:00Z", "--sort-by", "hostname", "--order", "asc"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "GPU TYPE", "GPU COUNT", "ATTESTATION", "FIRMWARE CHECK", "node-1", "gpu-001", "NVIDIA-H100", "Page: 1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies inventory report CSV output
func TestReportInventoryCSV(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv"})

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	cmdtest.SaveConfig(t, "https://example.invalid", "test-key")

	for _, output := range []string{"table", "json"} {
		t.Run(output, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--output", output})

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

// Verifies sort validation happens before fetching, and shared sorts reach both sources.
func TestReportInventorySortSources(t *testing.T) {
	sorts := []struct {
		field  string
		source string
	}{
		{field: "hostname", source: "inband"},
		{field: "publicIP", source: "inband"},
		{field: "privateIP", source: "inband"},
		{field: "nodeName", source: "oob"},
		{field: "bmcMac", source: "oob"},
		{field: "bmcIP", source: "oob"},
		{field: "nodeUUID"},
		{field: "nodeGroup"},
		{field: "computeZone"},
		{field: "gpuType"},
		{field: "gpuCount"},
		{field: "verificationCheck"},
		{field: "location"},
		{field: ""},
	}
	for _, sort := range sorts {
		for _, explicit := range []bool{false, true} {
			if explicit && sort.source == "" {
				continue
			}
			name := sort.field + "/both"
			if explicit {
				name = sort.field + "/" + sort.source
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("USERPROFILE", os.Getenv("HOME"))
				requests := make(chan string, 2)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					query := r.URL.Query()
					requests <- query.Get("dataSource")
					if got := query.Get("sortBy"); got != sort.field {
						t.Errorf("sortBy = %q, want %q", got, sort.field)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"nodes":[],"hasMore":false,"total":0}`))
				}))
				defer server.Close()
				cmdtest.SaveConfig(t, server.URL, "test-key")
				args := []string{"report", "inventory"}
				if sort.field != "" {
					args = append(args, "--sort-by", sort.field)
				}
				if explicit {
					args = append(args, "--data-source", sort.source)
				}
				cmd := newRootCmd()
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetArgs(args)
				err := cmd.Execute()
				if sort.source != "" && !explicit {
					want := "--data-source is required for --sort-by " + sort.field
					if err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("error = %v, want %q", err, want)
					}
					if len(requests) != 0 {
						t.Fatal("source-specific sort fetched before validation")
					}
					return
				}
				if err != nil {
					t.Fatalf("command failed: %v", err)
				}
				wantSources := []string{"inband", "oob"}
				if explicit {
					wantSources = []string{sort.source}
				}
				if len(requests) != len(wantSources) {
					t.Fatalf("request count = %d, want %d", len(requests), len(wantSources))
				}
				var gotSources []string
				for range wantSources {
					gotSources = append(gotSources, <-requests)
				}
				slices.Sort(gotSources)
				if !slices.Equal(gotSources, wantSources) {
					t.Errorf("data sources = %v, want %v", gotSources, wantSources)
				}
			})
		}
	}
}

// Verifies a signed inventory report is written to the current directory
func TestReportInventorySignedWritesToCWD(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed"})

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output", "json"})

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output-path", target})

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output-path", dir})

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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

func TestInventoryVersionColumns(t *testing.T) {
	for _, source := range []nvfleetint.DataSource{nvfleetint.DataSourceInband, nvfleetint.DataSourceOOB} {
		for _, empty := range []bool{false, true} {
			var out bytes.Buffer
			report := nvfleetint.InventoryReport{DataSource: source}
			if !empty {
				report.Nodes = []nvfleetint.InventoryNode{{NodeUUID: "node-1", AgentVersion: "agent-1.2.3", CollectorVersion: "collector-2.3.4"}}
			}
			if err := writeReportInventoryTableForSource(&out, report); err != nil {
				t.Fatal(err)
			}
			header, value, absent := "AGENT VERSION", "agent-1.2.3", "COLLECTOR VERSION"
			if source == nvfleetint.DataSourceOOB {
				header, value, absent = "COLLECTOR VERSION", "collector-2.3.4", "AGENT VERSION"
			}
			if !strings.Contains(out.String(), header) || strings.Contains(out.String(), absent) || (!empty && !strings.Contains(out.String(), value)) {
				t.Fatalf("incorrect source version table: %s", &out)
			}
		}
	}
}

func TestErrorReportSeverityColumn(t *testing.T) {
	var out bytes.Buffer
	report := nvfleetint.ErrorReport{View: nvfleetint.ErrorReportViewList, GroupBy: nvfleetint.ErrorReportGroupByError, Errors: []nvfleetint.ErrorReportError{{Name: "XID 79", Severities: []nvfleetint.ErrorSeverity{nvfleetint.ErrorSeverityFatal, nvfleetint.ErrorSeverityWarning}}, {Name: "No severity"}}}
	if err := writeReportErrorTable(&out, report); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"SEVERITIES", "XID 79", "Fatal", "Warning", "No severity"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %s: %s", want, &out)
		}
	}
}

func TestReportAPIPermissionFailures(t *testing.T) {
	cases := []struct {
		name, report, format, output, accept string
		signed, all                          bool
	}{
		{name: "inventory table", report: "inventory", format: "json", output: "table", accept: "application/json"},
		{name: "inventory json", report: "inventory", format: "json", output: "json", accept: "application/json"},
		{name: "inventory csv", report: "inventory", format: "csv", output: "table", accept: "text/csv"},
		{name: "inventory signed csv", report: "inventory", format: "csv", output: "table", accept: "application/zip", signed: true},
		{name: "error table", report: "error", format: "json", output: "table", accept: "application/json"},
		{name: "error json", report: "error", format: "json", output: "json", accept: "application/json"},
		{name: "error csv", report: "error", format: "csv", output: "table", accept: "text/csv"},
		{name: "inventory all table", report: "inventory", format: "json", output: "table", accept: "application/json", all: true},
		{name: "inventory all json", report: "inventory", format: "json", output: "json", accept: "application/json", all: true},
		{name: "error all table", report: "error", format: "json", output: "table", accept: "application/json", all: true},
		{name: "error all json", report: "error", format: "json", output: "json", accept: "application/json", all: true},
	}
	type request struct {
		method, path, accept string
		query                url.Values
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, r.Header.Get("Accept"), r.URL.Query()})
				requestNumber := len(requests)
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if tc.all && requestNumber == 1 {
					if tc.report == "inventory" {
						_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"first-node"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`))
					} else {
						_, _ = w.Write([]byte(`{"errors":[{"name":"first-error","count":1}],"page":0,"pageSize":1,"total":2,"hasMore":true}`))
					}
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"Forbidden","details":"no report access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			args := []string{"report", tc.report, "--data-source", "inband", "--format", tc.format}
			if tc.format != "csv" || tc.signed {
				args = append(args, "--output", tc.output)
			}
			if tc.report == "error" {
				args = append(args, "--view", "list", "--group-by", "error", "--window", "24h")
			}
			if tc.signed {
				args = append(args, "--signed")
			}
			if tc.all {
				args = append(args, "--all", "--page-size", "1")
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			err := cmd.Execute()
			var apiErr *nvfleetint.APIError
			if !errors.As(err, &apiErr) {
				t.Errorf("error = %v, want *nvfleetint.APIError", err)
			} else if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "Forbidden" || apiErr.Details != "no report access" {
				t.Errorf("API error = %+v, want 403, Forbidden, no report access", apiErr)
			}
			if tc.all && (err == nil || !strings.Contains(err.Error(), "page 1 after 1 completed pages")) {
				t.Errorf("pagination error = %v, want page 1 after 1 completed pages", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}

			mu.Lock()
			defer mu.Unlock()
			wantCount := 1
			if tc.all {
				wantCount = 2
			}
			if len(requests) != wantCount {
				t.Errorf("request count = %d, want %d", len(requests), wantCount)
			}
			for i, req := range requests {
				if req.method != http.MethodGet || req.path != "/v1/reports/"+tc.report || req.accept != tc.accept {
					t.Errorf("request %d: %s %s Accept=%q", i, req.method, req.path, req.accept)
				}
				want := map[string]string{"dataSource": "inband", "format": tc.format}
				if tc.report == "error" {
					want["view"], want["groupBy"], want["window"] = "list", "error", "24h"
				}
				if tc.signed {
					want["signed"] = "true"
				} else {
					want["signed"] = ""
				}
				if tc.all {
					want["page"], want["pageSize"] = strconv.Itoa(i), "1"
				}
				for key, value := range want {
					if got := req.query.Get(key); got != value {
						t.Errorf("request %d query %s = %q, want %q (query %v)", i, key, got, value, req.query)
					}
				}
			}
		})
	}
}

func TestReportInventorySignedWriteFailure(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	type request struct {
		method, path, accept string
		query                url.Values
	}
	requests := make(chan request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- request{r.Method, r.URL.Path, r.Header.Get("Accept"), r.URL.Query()}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write([]byte("PK\x03\x04 signed-zip-bytes"))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	target := filepath.Join(t.TempDir(), "missing", "inventory.zip")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--signed", "--output", "json", "--output-path", target})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "write signed report") || !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error = %v, want write signed report wrapping os.ErrNotExist", err)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output after write failure: %q", out.String())
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("destination stat error = %v, want os.ErrNotExist", err)
	}
	select {
	case req := <-requests:
		if req.method != http.MethodGet || req.path != "/v1/reports/inventory" || req.accept != "application/zip" {
			t.Errorf("request: %s %s Accept=%q", req.method, req.path, req.accept)
		}
		for key, want := range map[string]string{"dataSource": "inband", "format": "csv", "signed": "true"} {
			if got := req.query.Get(key); got != want {
				t.Errorf("query %s = %q, want %q", key, got, want)
			}
		}
	default:
		t.Error("signed inventory request not received")
	}
}

func TestWriteSignedReportDefaultFilename(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("PK\x03\x04 known signed bundle")
	path, err := writeSignedReport(dir, "", payload)
	if err != nil {
		t.Fatalf("write signed report: %v", err)
	}
	if want := filepath.Join(dir, "inventory-report.zip"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read signed report: %v", err)
	}
	if !bytes.Equal(written, payload) {
		t.Errorf("contents = %q, want %q", written, payload)
	}
}

func TestReportVerifyArtifactAndKeyErrors(t *testing.T) {
	cases := []struct {
		name string
	}{
		{name: "missing csv"},
		{name: "missing bundle"},
		{name: "csv is a directory"},
		{name: "invalid local key"},
		{name: "invalid fetched key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			dir := t.TempDir()
			csvPath, bundlePath, keyPath := writeSignedFixture(t, dir, []byte("customer,issued_at\nacme,2026-06-15T00:00:00Z\n"))
			var want []string
			var requests chan string
			switch tc.name {
			case "missing csv":
				csvPath = filepath.Join(dir, "missing.csv")
				want = []string{"--csv", csvPath, "does not exist"}
			case "missing bundle":
				bundlePath = filepath.Join(dir, "missing.sig.bundle")
				want = []string{"--bundle", bundlePath, "does not exist"}
			case "csv is a directory":
				csvPath = dir
				want = []string{"cannot read --csv", csvPath}
			case "invalid local key":
				if err := os.WriteFile(keyPath, []byte("invalid PEM"), 0o644); err != nil {
					t.Fatalf("write invalid key: %v", err)
				}
				want = []string{keyPath, "not a valid PEM public key"}
			case "invalid fetched key":
				requests = make(chan string, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					select {
					case requests <- r.URL.Path:
					default:
					}
					_, _ = w.Write([]byte("invalid PEM"))
				}))
				defer server.Close()
				cmdtest.SaveConfig(t, server.URL, "test-key")
				keyPath = ""
				want = []string{"the signing key fetched from the API is not a valid PEM public key"}
			}

			args := []string{"report", "verify", "--csv", csvPath, "--bundle", bundlePath}
			if keyPath != "" {
				args = append(args, "--key", keyPath)
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("command succeeded, want an error")
			}
			for _, part := range want {
				if !strings.Contains(err.Error(), part) {
					t.Errorf("error = %q, want it to contain %q", err, part)
				}
			}
			if tc.name == "csv is a directory" && strings.Contains(err.Error(), "does not exist") {
				t.Errorf("directory reported as missing: %v", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
			if requests != nil {
				select {
				case path := <-requests:
					if path != "/.well-known/signing-key.pub" {
						t.Errorf("requested path = %q, want signing-key endpoint", path)
					}
				default:
					t.Error("signing-key endpoint was not requested")
				}
			}
		})
	}
}

func TestReportValidationBoundariesRejectBeforeRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "inventory csv page", args: []string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--page", "1"}, want: []string{"pagination flags cannot be used with --format csv"}},
		{name: "error csv page size", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--format", "csv", "--page-size", "1"}, want: []string{"pagination flags cannot be used with --format csv"}},
		{name: "error overview page", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--page", "1"}, want: []string{"pagination flags cannot be used with --view overview"}},
		{name: "error conflicting time range", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--start", "2026-05-01T00:00:00Z", "--end", "2026-05-02T00:00:00Z"}, want: []string{"--window cannot be used with --start or --end"}},
		{name: "inventory invalid end", args: []string{"report", "inventory", "--start", "2026-05-01T00:00:00Z", "--end", "yesterday"}, want: []string{"--end must be RFC3339"}},
		{name: "error invalid end", args: []string{"report", "error", "--view", "overview", "--start", "2026-05-01T00:00:00Z", "--end", "yesterday"}, want: []string{"--end must be RFC3339"}},
		{name: "error empty severity", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--severities", "Critical,,Fatal"}, want: []string{"invalid severity", "empty values are not allowed"}},
		{name: "inventory invalid page", args: []string{"report", "inventory", "--page", "0"}, want: []string{"--page must be greater than or equal to 1"}},
		{name: "error invalid page", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--page", "0"}, want: []string{"--page must be greater than or equal to 1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			for _, part := range tc.want {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Errorf("error = %v, want it to contain %q", err, part)
				}
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
			mu.Lock()
			count := requests
			mu.Unlock()
			if count != 0 {
				t.Errorf("request count = %d, want 0", count)
			}
		})
	}
}

func TestReportErrorOutputSuggestedActions(t *testing.T) {
	rows := []struct {
		name   string
		action *nvfleetint.SuggestedAction
		want   string
	}{
		{name: "nil-action", want: "-"},
		{name: "both-fields", action: &nvfleetint.SuggestedAction{Action: "  Drain node  ", Code: "  DRAIN  "}, want: "Drain node (DRAIN)"},
		{name: "action-only", action: &nvfleetint.SuggestedAction{Action: "Drain node"}, want: "Drain node"},
		{name: "code-only", action: &nvfleetint.SuggestedAction{Code: "DRAIN"}, want: "DRAIN"},
		{name: "blank-fields", action: &nvfleetint.SuggestedAction{Action: "  ", Code: "  "}, want: "-"},
	}
	report := nvfleetint.ErrorReport{View: nvfleetint.ErrorReportViewList, GroupBy: nvfleetint.ErrorReportGroupByError}
	for _, row := range rows {
		report.Errors = append(report.Errors, nvfleetint.ErrorReportError{Name: row.name, SuggestedAction: row.action})
	}
	var out bytes.Buffer
	if err := writeReportErrorOutput(&out, cmdutil.Resolved{Output: clioutput.FormatTable}, reportErrorOutput{Report: report}); err != nil {
		t.Fatalf("write error report: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != len(rows)+1 {
		t.Fatalf("got %d lines, want header and %d rows: %q", len(lines), len(rows), out.String())
	}
	for _, row := range rows {
		matches := 0
		for _, line := range lines[1:] {
			fields := strings.Fields(line)
			if len(fields) == 0 || fields[0] != row.name {
				continue
			}
			matches++
			// The final column is the action; compare all of it on this row.
			if len(fields) < 5 || strings.Join(fields[4:], " ") != row.want {
				t.Errorf("row %q = %q, want action %q", row.name, line, row.want)
			}
		}
		if matches != 1 {
			t.Errorf("row %q appeared %d times, want once", row.name, matches)
		}
	}
}

func TestReportErrorOutputEmptyViewsAndInvalidView(t *testing.T) {
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	for _, tc := range []struct {
		name    string
		view    nvfleetint.ErrorReportView
		headers []string
	}{
		{name: "nil graph", view: nvfleetint.ErrorReportViewGraph, headers: []string{"ERROR", "VALUES", "START", "END"}},
		{name: "nil overview", view: nvfleetint.ErrorReportViewOverview, headers: []string{"FIELD", "VALUE"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := writeReportErrorOutput(&out, common, reportErrorOutput{Report: nvfleetint.ErrorReport{View: tc.view}})
			if err != nil {
				t.Fatalf("write error report: %v", err)
			}
			lines := strings.Split(strings.TrimSpace(out.String()), "\n")
			if len(lines) != 1 || strings.Join(strings.Fields(lines[0]), "|") != strings.Join(tc.headers, "|") {
				t.Errorf("output = %q, want only headers %v", out.String(), tc.headers)
			}
		})
	}
	t.Run("invalid view", func(t *testing.T) {
		var out bytes.Buffer
		err := writeReportErrorOutput(&out, common, reportErrorOutput{Report: nvfleetint.ErrorReport{View: "unknown-view"}})
		if err == nil || !strings.Contains(err.Error(), "unknown-view") {
			t.Errorf("error = %v, want invalid view identified", err)
		}
		if out.Len() != 0 {
			t.Errorf("unexpected output for invalid view: %q", out.String())
		}
	})
}

type reportFailingWriter struct {
	err   error
	calls int
}

func (w *reportFailingWriter) Write(_ []byte) (int, error) {
	w.calls++
	return 0, w.err
}

func TestReportOutputTableWriteFailureStopsBeforeFooter(t *testing.T) {
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	page := &clioutput.Pagination{Page: 0, PageSize: 1, Total: 1}
	sentinel := errors.New("table write failed")
	cases := []struct {
		name  string
		write func(*reportFailingWriter) error
	}{
		{name: "in-band inventory", write: func(w *reportFailingWriter) error {
			return writeReportInventoryOutput(w, common, reportInventoryOutput{Report: nvfleetint.InventoryReport{DataSource: nvfleetint.DataSourceInband, Nodes: []nvfleetint.InventoryNode{{NodeUUID: "node-inband"}}}, Page: page})
		}},
		{name: "OOB inventory", write: func(w *reportFailingWriter) error {
			return writeReportInventoryOutput(w, common, reportInventoryOutput{Report: nvfleetint.InventoryReport{DataSource: nvfleetint.DataSourceOOB, Nodes: []nvfleetint.InventoryNode{{NodeUUID: "node-oob"}}}, Page: page})
		}},
		{name: "error list", write: func(w *reportFailingWriter) error {
			return writeReportErrorOutput(w, common, reportErrorOutput{Report: nvfleetint.ErrorReport{View: nvfleetint.ErrorReportViewList, GroupBy: nvfleetint.ErrorReportGroupByError, Errors: []nvfleetint.ErrorReportError{{Name: "error-one"}}}, Page: page})
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := &reportFailingWriter{err: sentinel}
			if err := tc.write(w); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if w.calls != 1 {
				t.Errorf("write calls = %d, want 1 table attempt and no footer", w.calls)
			}
		})
	}
}

func TestReportCommandsRequireCredentials(t *testing.T) {
	for _, name := range []string{"inventory", "error", "verify"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			var args []string
			switch name {
			case "inventory":
				args = []string{"report", "inventory", "--data-source", "inband"}
			case "error":
				args = []string{"report", "error", "--window", "24h"}
			case "verify":
				csv, bundle, _ := writeSignedFixture(t, t.TempDir(), []byte("customer\nacme\n"))
				args = []string{"report", "verify", "--csv", csv, "--bundle", bundle}
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			err := cmd.Execute()
			if !errors.Is(err, config.ErrNoProfile) {
				t.Errorf("error = %v, want ErrNoProfile", err)
			}
			if err == nil || !strings.Contains(err.Error(), "nvfleetint auth add <name>") || !strings.Contains(err.Error(), config.EnvAPIKey) {
				t.Errorf("error = %v, want credential setup hint", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
		})
	}
}

func TestReportVerifyTimeoutPrecedesArtifactAccess(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)
	dir := t.TempDir()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"report", "verify", "--timeout", "0", "--csv", filepath.Join(dir, "missing.csv"), "--bundle", filepath.Join(dir, "missing.bundle")})
	err := cmd.Execute()
	if err == nil || err.Error() != "--timeout must be greater than 0" {
		t.Errorf("error = %v, want timeout validation error", err)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output after failure: %q", out.String())
	}
}

func TestReportVerifyErrorPreservesUnexpectedError(t *testing.T) {
	sentinel := errors.New("unexpected verification error")
	wrapped := fmt.Errorf("verification context: %w", sentinel)
	got := reportVerifyError(reportVerifyFlags{}, wrapped)
	if got != wrapped || !errors.Is(got, sentinel) {
		t.Errorf("error = %v, want unchanged wrapped sentinel %v", got, wrapped)
	}
}

// Explicit --page is one-based; verify the zero-based backend request and
// one-based JSON pagination while preserving other response fields.
func TestRound2ExplicitPageContract(t *testing.T) {
	for _, cliPage := range []int{1, 3} {
		t.Run(fmt.Sprintf("page_%d", cliPage), func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			apiPage := cliPage - 1
			const pageSize = 7
			body := []byte(fmt.Sprintf(`{"errors":[{"name":"pagination-error","count":3,"nodeCount":1}],"page":%d,"pageSize":7,"total":%d,"hasMore":false,"opaquePaginationContract":"preserved"}`, apiPage, apiPage*pageSize+1))
			type request struct{ method, path, page, size string }
			requests := make(chan request, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				q := r.URL.Query()
				select {
				case requests <- request{r.Method, r.URL.Path, q.Get("page"), q.Get("pageSize")}:
				default:
					t.Error("unexpected extra API requests")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(body)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			args := []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h"}
			args = append(args, "--page", strconv.Itoa(cliPage), "--page-size", strconv.Itoa(pageSize), "--output", "json")
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			if got := len(requests); got != 1 {
				t.Fatalf("API request count = %d, want 1", got)
			}
			gotRequest := <-requests
			wantRequest := request{http.MethodGet, "/v1/reports/error", strconv.Itoa(apiPage), strconv.Itoa(pageSize)}
			if gotRequest != wantRequest {
				t.Fatalf("request = %+v, want %+v", gotRequest, wantRequest)
			}
			var got, want map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("invalid command JSON %q: %v", out.String(), err)
			}
			if err := json.Unmarshal(body, &want); err != nil {
				t.Fatal(err)
			}
			// The documented CLI contract normalizes API page 0 to CLI page 1.
			// See output.OneBasedPage and the existing command JSON tests.
			want["page"] = float64(cliPage)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("JSON result = %#v, want normalized CLI result %#v", got, want)
			}
		})
	}
}

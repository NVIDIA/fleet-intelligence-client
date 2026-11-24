// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

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

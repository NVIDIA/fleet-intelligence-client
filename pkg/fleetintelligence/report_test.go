// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetintelligence

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Verifies inventory report request construction and decoding
func TestGetInventoryReportSendsParamsAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/inventory" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Fatalf("unexpected accept header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("format"); got != "json" {
			t.Fatalf("unexpected format: %q", got)
		}
		if got := query.Get("page"); got != "2" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := query.Get("pageSize"); got != "25" {
			t.Fatalf("unexpected pageSize: %q", got)
		}
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1", "cz-2"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1"}) {
			t.Fatalf("unexpected nodeGroupIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["tags"]; !slices.Equal(got, []string{"prod"}) {
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
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","gpuType":"NVIDIA-H100","gpuCount":8,"integrityCheck":"Verified","firmwareCheck":"Passed","publicIP":"203.0.113.10","privateIP":"10.0.0.10","serialNumbers":["SN1"]}],"hasMore":false,"page":2,"pageSize":25,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	page := 2
	pageSize := 25
	got, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{
		ComputeZoneIDs: []string{"cz-1", "cz-2"},
		NodeGroupIDs:   []string{"ng-1"},
		Tags:           []string{"prod"},
		StartTime:      "2026-05-01T00:00:00Z",
		EndTime:        "2026-05-02T00:00:00Z",
		SortBy:         InventoryReportSortByHostname,
		Order:          InventoryReportOrderAsc,
		Page:           &page,
		PageSize:       &pageSize,
	})
	if err != nil {
		t.Fatalf("inventory report failed: %v", err)
	}
	if got.Page != 2 || got.PageSize != 25 || got.Total != 1 || len(got.Nodes) != 1 {
		t.Fatalf("unexpected report: %#v", got)
	}
	node := got.Nodes[0]
	if node.NodeUUID != "node-1" || node.Hostname != "gpu-001" || node.GPUCount == nil || *node.GPUCount != 8 || len(node.SerialNumbers) != 1 {
		t.Fatalf("unexpected node: %#v", node)
	}
	if !strings.Contains(string(got.RawJSON), `"nodes"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies inventory CSV report downloads
func TestGetInventoryReportCSV(t *testing.T) {
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
		_, _ = w.Write([]byte("nodeUUID,hostname\nnode-1,gpu-001\n"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{Format: ReportFormatCSV})
	if err != nil {
		t.Fatalf("inventory csv failed: %v", err)
	}
	if string(got.RawCSV) != "nodeUUID,hostname\nnode-1,gpu-001\n" {
		t.Fatalf("unexpected csv: %q", string(got.RawCSV))
	}
}

// Verifies signed inventory report downloads request a zip bundle
func TestGetInventoryReportSigned(t *testing.T) {
	payload := []byte("PK\x03\x04 signed-zip-bytes")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/inventory" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
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
		w.Header().Set("Content-Disposition", `attachment; filename="fleet-inventory-2026.zip"`)
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.GetInventoryReport(context.Background(), InventoryReportOptions{Format: ReportFormatCSV, Signed: true})
	if err != nil {
		t.Fatalf("inventory signed failed: %v", err)
	}
	if !bytes.Equal(got.RawSigned, payload) {
		t.Fatalf("unexpected signed bytes: %q", string(got.RawSigned))
	}
	if got.Filename != "fleet-inventory-2026.zip" {
		t.Fatalf("unexpected filename: %q", got.Filename)
	}
}

// Verifies signed inventory report downloads reject non-zip responses
func TestGetInventoryReportSignedRejectsNonZipContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/inventory" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/zip" {
			t.Fatalf("unexpected accept header: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.GetInventoryReport(context.Background(), InventoryReportOptions{Format: ReportFormatCSV, Signed: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `content type "application/json"`) {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "expected application/zip") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies signed inventory reports are rejected without csv format
func TestGetInventoryReportSignedRequiresCSV(t *testing.T) {
	client, err := NewClient("https://example.invalid", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.GetInventoryReport(context.Background(), InventoryReportOptions{Signed: true})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "signed inventory reports require csv format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies error report list request construction and decoding
func TestGetErrorReportListByError(t *testing.T) {
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
		if got := query.Get("format"); got != "json" {
			t.Fatalf("unexpected format: %q", got)
		}
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "24h" {
			t.Fatalf("unexpected window: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":[{"name":"NVSwitch Fatal Error","count":10,"nodeCount":5,"suggestedAction":{"action":"Drain node","code":"DRAIN","persona":"dc_admin","type":"immediate"}}],"hasMore":false,"page":0,"pageSize":10,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.GetErrorReport(context.Background(), ErrorReportOptions{
		View:     ErrorReportViewList,
		GroupBy:  ErrorReportGroupByError,
		TimeMode: ErrorReportTimeModeRelative,
		Window:   "24h",
	})
	if err != nil {
		t.Fatalf("error report failed: %v", err)
	}
	if len(got.Errors) != 1 || got.Errors[0].Name != "NVSwitch Fatal Error" || got.Errors[0].SuggestedAction == nil || got.Errors[0].SuggestedAction.Code != "DRAIN" {
		t.Fatalf("unexpected errors: %#v", got.Errors)
	}
}

// Verifies graph reports default to grouping by error
func TestGetErrorReportGraphDefaultsGroupByError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/reports/error" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("view"); got != "graph" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := r.URL.Query().Get("groupBy"); got != "error" {
			t.Fatalf("unexpected groupBy: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"label":{"error":"xid_154"},"values":[[1716153600,5]]}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-02T00:00:00Z"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.GetErrorReport(context.Background(), ErrorReportOptions{View: ErrorReportViewGraph})
	if err != nil {
		t.Fatalf("graph report failed: %v", err)
	}
	if got.Graph == nil || len(got.Graph.Result) != 1 || got.Graph.Result[0].Error != "xid_154" || got.Graph.Result[0].Values != "[[1716153600,5]]" || got.Graph.TimeRange.Start != "2026-05-01T00:00:00Z" {
		t.Fatalf("unexpected graph: %#v", got.Graph)
	}
}

// Verifies error report validation before requests
func TestGetErrorReportRejectsInvalidOptions(t *testing.T) {
	client, err := NewClient("https://fleet.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	tests := []struct {
		name string
		opts ErrorReportOptions
		want string
	}{
		{name: "missing view", opts: ErrorReportOptions{}, want: "view is required"},
		{name: "missing group", opts: ErrorReportOptions{View: ErrorReportViewList}, want: "group-by is required"},
		{name: "csv overview", opts: ErrorReportOptions{View: ErrorReportViewOverview, Format: ReportFormatCSV}, want: "csv format"},
		{name: "graph node group", opts: ErrorReportOptions{View: ErrorReportViewGraph, GroupBy: ErrorReportGroupByNode}, want: "graph view"},
		{name: "relative time missing window", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeRelative}, want: "relative time mode requires window"},
		{name: "absolute time missing end", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeAbsolute, StartTime: "2026-05-01T00:00:00Z"}, want: "absolute time mode requires start time and end time"},
		{name: "absolute time missing start", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeAbsolute, EndTime: "2026-05-02T00:00:00Z"}, want: "absolute time mode requires start time and end time"},
		{name: "relative time with start/end", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeRelative, Window: "24h", StartTime: "2026-05-01T00:00:00Z", EndTime: "2026-05-02T00:00:00Z"}, want: "relative time mode does not support start time or end time"},
		{name: "absolute time with window", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeAbsolute, Window: "24h", StartTime: "2026-05-01T00:00:00Z", EndTime: "2026-05-02T00:00:00Z"}, want: "absolute time mode does not support window"},
		{name: "malformed window", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeRelative, Window: "5 bananas"}, want: "invalid window"},
		{name: "zero window", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeRelative, Window: "0s"}, want: "invalid window"},
		{name: "malformed start", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeAbsolute, StartTime: "not-a-date", EndTime: "2026-05-02T00:00:00Z"}, want: "start time must be RFC3339"},
		{name: "malformed end", opts: ErrorReportOptions{View: ErrorReportViewList, GroupBy: ErrorReportGroupByError, TimeMode: ErrorReportTimeModeAbsolute, StartTime: "2026-05-01T00:00:00Z", EndTime: "not-a-date"}, want: "end time must be RFC3339"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.GetErrorReport(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

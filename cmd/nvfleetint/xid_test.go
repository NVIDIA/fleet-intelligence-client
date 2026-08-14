// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

const xidBurstsBody = `{"items":[{"burstId":"burst-1","nodeUuid":"node-1","hostname":"gpu-01","nodeGroup":"ng","nodeGroupId":"ng-1","computeZone":"cz","computeZoneId":"cz-1","startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T00:05:00Z","burstDurationSeconds":300,"xidCount":2,"xidNumbers":[{"xidNumber":48,"mnemonic":"DBE","description":"Double Bit ECC"},{"xidNumber":94,"mnemonic":"CE"}],"deviceIds":{"0000:0f:00.0":[48,94]},"jobDisruption":true,"category":"GPU","subcategory":"Memory","stickyXidsSuppressed":1,"suggestedActions":[{"action":"Drain the node","code":"DRAIN","persona":"tenant","type":"immediate"}]}],"page":0,"pageSize":20,"total":1}`

// Verifies xidburst list table output and filter translation
func TestXIDBurstListTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "24h" {
			t.Fatalf("unexpected window: %q", got)
		}
		if got := query["xidNumbers"]; len(got) != 2 || got[0] != "48" || got[1] != "94" {
			t.Fatalf("unexpected xidNumbers: %#v", got)
		}
		if got := query.Get("jobDisruption"); got != "true" {
			t.Fatalf("unexpected jobDisruption: %q", got)
		}
		if got := query.Get("sortBy"); got != "startTime" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"xidburst", "list", "--window", "24h", "--xid-numbers", "48,94",
		"--job-disruption", "--sort-by", "startTime",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"BURST ID", "HOSTNAME", "XIDS", "XID COUNT", "START TIME", "DURATION (S)",
		"JOB DISRUPTION", "NODE GROUP", "COMPUTE ZONE",
		"burst-1", "gpu-01", "48, 94", "2026-05-01T00:00:00Z", "300", "true",
		"Total Entries: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	// Detail-only fields stay out of the list table.
	for _, absent := range []string{"Double Bit ECC", "0000:0f:00.0", "Drain the node"} {
		if strings.Contains(got, absent) {
			t.Fatalf("output should not contain %q:\n%s", absent, got)
		}
	}
}

// Verifies xidburst list JSON output is the raw payload with a 1-based page
func TestXIDBurstListJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "list", "--window", "24h", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := strings.TrimSpace(out.String())
	if !strings.Contains(got, `"page":1`) {
		t.Fatalf("expected 1-based page in JSON output:\n%s", got)
	}
	// The raw backend payload is preserved, including the fields the table drops.
	for _, want := range []string{`"burstId":"burst-1"`, `"deviceIds"`, `"Double Bit ECC"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// Verifies xidburst list rejects invalid flags before any request
func TestXIDBurstListFlagValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	tests := []struct {
		name string
		args []string
	}{
		{"missing time range", []string{"xidburst", "list"}},
		{"window with start", []string{"xidburst", "list", "--window", "24h", "--start", "2026-05-01T00:00:00Z"}},
		{"bad sort field", []string{"xidburst", "list", "--window", "24h", "--sort-by", "burstId"}},
		{"bad order", []string{"xidburst", "list", "--window", "24h", "--order", "sideways"}},
		{"non-numeric xid", []string{"xidburst", "list", "--window", "24h", "--xid-numbers", "forty-eight"}},
		{"bad output", []string{"xidburst", "list", "--window", "24h", "--output", "yaml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err == nil {
				t.Fatalf("expected error for %v", tt.args)
			}
		})
	}
}

// Verifies xidburst describe renders the field/value table
func TestXIDBurstDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts/burst-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"burstId":"burst-1","nodeUuid":"node-1","hostname":"gpu-01","nodeGroup":"ng","nodeGroupId":"ng-1","computeZone":"cz","computeZoneId":"cz-1","startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T00:05:00Z","burstDurationSeconds":300,"xidCount":2,"xidNumbers":[{"xidNumber":48,"mnemonic":"DBE","description":"Double Bit ECC"}],"deviceIds":{"0000:0f:00.0":[48,94]},"jobDisruption":true,"category":"GPU","subcategory":"Memory","stickyXidsSuppressed":1,"suggestedActions":[{"action":"Drain the node","code":"DRAIN","persona":"tenant","type":"immediate"}]}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "describe", "burst-1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"FIELD", "VALUE", "BURST ID", "burst-1", "NODE UUID", "HOSTNAME", "gpu-01",
		"NODE GROUP", "COMPUTE ZONE", "DURATION (S)", "300", "XIDS", "48",
		"JOB DISRUPTION", "true", "CATEGORY", "GPU", "SUBCATEGORY", "Memory",
		"XID 48", "DBE: Double Bit ECC", "DEVICE 0000:0f:00.0", "48, 94",
		"TENANT IMMEDIATE", "DRAIN: Drain the node",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	// A field the backend omitted for this persona renders as "-", not "false".
	if !strings.Contains(got, "PLATFORM JOB DISRUPTION") || !strings.Contains(got, "PLATFORM JOB DISRUPTION  -") {
		t.Fatalf("expected omitted platform disruption to render as '-':\n%s", got)
	}
}

// Verifies xidburst describe JSON output is the raw backend payload
func TestXIDBurstDescribeJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	body := `{"burstId":"burst-1","hostname":"gpu-01","jobDisruption":true}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "describe", "burst-1", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != body {
		t.Fatalf("unexpected JSON:\ngot  %s\nwant %s", strings.TrimSpace(out.String()), body)
	}
}

// Verifies xidburst describe requires exactly one burst ID
func TestXIDBurstDescribeRequiresBurstID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid args")
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	for _, args := range [][]string{
		{"xidburst", "describe"},
		{"xidburst", "describe", "burst-1", "burst-2"},
	} {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

// Verifies --all merges pages using the derived has-more signal
func TestXIDBurstListAllFetchesEveryPage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page := r.URL.Query().Get("page")
		pages++
		w.Header().Set("Content-Type", "application/json")
		switch page {
		case "0":
			_, _ = w.Write([]byte(`{"items":[{"burstId":"burst-1"}],"page":0,"pageSize":1,"total":2}`))
		case "1":
			_, _ = w.Write([]byte(`{"items":[{"burstId":"burst-2"}],"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", page)
		}
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "list", "--window", "24h", "--all", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if pages != 2 {
		t.Fatalf("expected 2 API pages, got %d", pages)
	}
	got := out.String()
	if !strings.Contains(got, "burst-1") || !strings.Contains(got, "burst-2") {
		t.Fatalf("expected both pages in output:\n%s", got)
	}
}

// Verifies the derived has-more signal, which the endpoint does not report
func TestXIDBurstPageHasMore(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
		total    int
		want     bool
	}{
		{"more pages remain", 0, 20, 45, true},
		{"last page", 2, 20, 45, false},
		{"exact fit", 1, 20, 40, false},
		{"empty result", 0, 20, 0, false},
		{"unreported page size", 0, 0, 45, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := xidBurstPageHasMore(nvfleetint.XIDBurstsPage{
				Page:     tt.page,
				PageSize: tt.pageSize,
				Total:    tt.total,
			})
			if got != tt.want {
				t.Fatalf("hasMore = %v, want %v", got, tt.want)
			}
		})
	}
}

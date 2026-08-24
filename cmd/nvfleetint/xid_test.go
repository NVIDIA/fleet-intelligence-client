// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
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

// Verifies the exclusion filter flags reach the backend as query parameters
func TestXIDBurstListExclusionFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{
		"xidburst", "list", "--window", "24h",
		"--exclude-nodegroup-ids", "ng-1,ng-2",
		"--exclude-compute-zone-ids", "cz-1",
		"--sort-by", "xidCount",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	if got := query["excludeNodeGroupIds"]; !reflect.DeepEqual(got, []string{"ng-1", "ng-2"}) {
		t.Fatalf("unexpected excludeNodeGroupIds: %#v", got)
	}
	if got := query["excludeComputeZoneIds"]; !reflect.DeepEqual(got, []string{"cz-1"}) {
		t.Fatalf("unexpected excludeComputeZoneIds: %#v", got)
	}
	if got := query.Get("sortBy"); got != "xidCount" {
		t.Fatalf("unexpected sortBy: %q", got)
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
		{"nodegroup include with exclude", []string{
			"xidburst", "list", "--window", "24h",
			"--nodegroup-ids", "ng-1", "--exclude-nodegroup-ids", "ng-2",
		}},
		{"compute zone include with exclude", []string{
			"xidburst", "list", "--window", "24h",
			"--compute-zone-ids", "cz-1", "--exclude-compute-zone-ids", "cz-2",
		}},
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

// Verifies XID burst options split suggested actions across the four
// persona/type flags, default a missing persona to tenant, and set aside
// actions whose type the API did not report.
func TestXIDBurstOptionsOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	body := `{"xidNumbers":[43],"categories":["User-App"],"subcategories":["Illegal Memory Access"],` +
		`"jobDisruption":[true,false],"jobDisruptionDueToPlatformIssue":[true,false],"suggestedActions":[` +
		`{"code":"RESTART_APP","action":"Restart the application","persona":"tenant","type":"immediate"},` +
		`{"code":"CHECK_LOGS","action":"Inspect application logs","persona":"tenant","type":"investigatory"},` +
		`{"code":"PULL_FROM_SERVICE","action":"Pull the node","persona":"dc_admin","type":"immediate"},` +
		`{"code":"RUN_DIAGS","action":"Run diagnostics","persona":"dc_admin","type":"investigatory"},` +
		`{"code":"NO_PERSONA","action":"Persona omitted for tenant keys","type":"immediate"},` +
		`{"code":"NO_TYPE","action":"Type not reported"},` +
		`{"code":"NEW_PERSONA","action":"Persona the CLI has no flag for","persona":"operator","type":"immediate"}` +
		`]}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "options"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("xid burst options command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"Filters for 'xidburst list'",
		"--xid-numbers",
		"--categories",
		"--subcategories",
		"--tenant-actions",
		"--tenant-investigations",
		"--dc-admin-actions",
		"--dc-admin-investigations",
		"--job-disruption  (boolean; pass --job-disruption=false to match false)",
		"--platform-disruption",
		"RESTART_APP", "CHECK_LOGS", "PULL_FROM_SERVICE", "RUN_DIAGS",
		"suggestedActions with no matching persona/type flag  (no flag on 'xidburst list')",
		"NO_TYPE",
		"NEW_PERSONA",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("table output missing %q:\n%s", want, got)
		}
	}

	// A persona-less immediate action belongs to the tenant bucket, not dc-admin.
	tenantActions := sectionBody(t, got, "--tenant-actions")
	if !strings.Contains(tenantActions, "NO_PERSONA") || !strings.Contains(tenantActions, "RESTART_APP") {
		t.Fatalf("unexpected tenant action section:\n%s", tenantActions)
	}
	// A persona no flag covers is listed as unclassified rather than dropped or
	// folded into a persona it does not belong to.
	if strings.Contains(tenantActions, "NEW_PERSONA") {
		t.Fatalf("unknown persona leaked into the tenant section:\n%s", tenantActions)
	}
	unclassified := sectionBody(t, got, "suggestedActions with no matching persona/type flag  (no flag on 'xidburst list')")
	if !strings.Contains(unclassified, "NEW_PERSONA") || !strings.Contains(unclassified, "NO_TYPE") {
		t.Fatalf("unexpected unclassified action section:\n%s", unclassified)
	}
	if strings.Contains(sectionBody(t, got, "--dc-admin-actions"), "NO_PERSONA") {
		t.Fatalf("persona-less action leaked into the dc-admin section:\n%s", got)
	}
	if section := sectionBody(t, got, "--dc-admin-investigations"); !strings.Contains(section, "RUN_DIAGS") {
		t.Fatalf("unexpected dc-admin investigation section:\n%s", section)
	}

	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"xidburst", "options", "--output", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("xid burst options JSON command failed: %v", err)
	}
	if strings.TrimSpace(jsonOut.String()) != body {
		t.Fatalf("JSON output is not the raw payload:\n%s", jsonOut.String())
	}
}

// Verifies filters the backend returns empty are still listed, so a tenant key
// sees which flags exist rather than a silently shortened list.
func TestXIDBurstOptionsRendersEmptyFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jobDisruption":[true,false],"xidNumbers":[],"suggestedActions":[]}`))
	}))
	defer server.Close()

	saveTestConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "options"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("xid burst options command failed: %v", err)
	}
	got := out.String()
	// Every filter except jobDisruption is absent or empty in this payload.
	if strings.Count(got, "(none)") != 8 {
		t.Fatalf("expected every empty filter to render as (none):\n%s", got)
	}
	if !strings.Contains(got, "--dc-admin-actions") {
		t.Fatalf("empty sections should still name their flag:\n%s", got)
	}
}

// Returns the indented body of one rendered options section.
func sectionBody(t *testing.T, output, heading string) string {
	t.Helper()
	// A heading is either the bare flag or the flag plus a parenthesized note.
	start := strings.Index(output, "\n"+heading+"\n")
	if start < 0 {
		start = strings.Index(output, "\n"+heading+" ")
	}
	if start < 0 {
		t.Fatalf("section %q not found in:\n%s", heading, output)
	}
	rest := output[start+1:]
	if end := strings.Index(rest, "\n\n"); end >= 0 {
		return rest[:end]
	}
	return rest
}

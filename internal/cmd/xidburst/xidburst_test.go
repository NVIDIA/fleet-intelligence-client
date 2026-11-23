// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
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

const xidBurstsBody = `{"items":[{"burstId":"burst-1","nodeUuid":"node-1","hostname":"gpu-01","nodeGroup":"ng","nodeGroupId":"ng-1","computeZone":"cz","computeZoneId":"cz-1","startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T00:05:00Z","burstDurationSeconds":300,"xidCount":2,"xidNumbers":[{"xidNumber":48,"mnemonic":"DBE","description":"Double Bit ECC"},{"xidNumber":94,"mnemonic":"CE"}],"deviceIds":{"0000:0f:00.0":[48,94]},"jobDisruption":true,"category":"GPU","subcategory":"Memory","stickyXidsSuppressed":1,"suggestedActions":[{"action":"Drain the node","code":"DRAIN","persona":"tenant","type":"immediate"}]}],"page":0,"pageSize":20,"total":1}`

// Verifies xidburst list table output and filter translation
func TestXIDBurstListTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
		"BURST ID", "HOSTNAME", "XIDS", "START TIME", "DURATION (S)",
		"JOB DISRUPTED", "PLATFORM RELATED", "CATEGORY", "SUBCATEGORY", "GPU", "Memory",
		"burst-1", "gpu-01", "48, 94", "2026-05-01T00:00:00Z", "300", "true",
		"Total Entries: 1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	headers := strings.Fields(strings.SplitN(got, "\n", 2)[0])
	wantPrefix := strings.Fields("BURST ID XIDS JOB DISRUPTED PLATFORM RELATED CATEGORY SUBCATEGORY")
	if len(headers) < len(wantPrefix) || !reflect.DeepEqual(headers[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("unexpected column order: %v", headers)
	}
	values := strings.Fields(strings.SplitN(got, "\n", 3)[1])
	wantValues := []string{"burst-1", "48,", "94", "true", "-", "GPU", "Memory"}
	if len(values) < len(wantValues) || !reflect.DeepEqual(values[:len(wantValues)], wantValues) {
		t.Fatalf("unexpected leading values: %v", values)
	}
	// Detail-only fields stay out of the list table.
	for _, absent := range []string{"NODE GROUP", "COMPUTE ZONE", "XID COUNT", "Double Bit ECC", "0000:0f:00.0", "Drain the node"} {
		if strings.Contains(got, absent) {
			t.Fatalf("output should not contain %q:\n%s", absent, got)
		}
	}
}

// Verifies xidburst list JSON output is the raw payload with a 1-based page
func TestXIDBurstListJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts/burst-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"burstId":"burst-1","nodeUuid":"node-1","hostname":"gpu-01","nodeGroup":"ng","nodeGroupId":"ng-1","computeZone":"cz","computeZoneId":"cz-1","startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T00:05:00Z","burstDurationSeconds":300,"xidCount":2,"xidNumbers":[{"xidNumber":48,"mnemonic":"DBE","description":"Double Bit ECC"}],"deviceIds":{"0000:0f:00.0":[48,94]},"jobDisruption":true,"category":"GPU","subcategory":"Memory","stickyXidsSuppressed":1,"suggestedActions":[{"action":"Drain the node","code":"DRAIN","persona":"tenant","type":"immediate"}]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
		"TENANT IMMEDIATE", "DRAIN: Drain the node", "END TIME", "2026-05-01T00:05:00Z",
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	body := `{"burstId":"burst-1","hostname":"gpu-01","jobDisruption":true}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid args")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
// Verifies the scope flags reach the options endpoint as query parameters
func TestXIDBurstOptionsSendsScopeFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var query url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/xid/bursts/options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"xidNumbers":[43]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"xidburst", "options",
		"--window", "24h",
		"--nodegroup-ids", "ng-1,ng-2",
		"--exclude-compute-zone-ids", "cz-9",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("xid burst options command failed: %v", err)
	}

	if got := query.Get("timeMode"); got != "relative" {
		t.Fatalf("unexpected timeMode: %q", got)
	}
	if got := query.Get("window"); got != "24h" {
		t.Fatalf("unexpected window: %q", got)
	}
	if got := query["nodeGroupIds"]; !reflect.DeepEqual(got, []string{"ng-1", "ng-2"}) {
		t.Fatalf("unexpected nodeGroupIds: %#v", got)
	}
	if got := query["excludeComputeZoneIds"]; !reflect.DeepEqual(got, []string{"cz-9"}) {
		t.Fatalf("unexpected excludeComputeZoneIds: %#v", got)
	}
	if query.Has("computeZoneIds") || query.Has("excludeNodeGroupIds") {
		t.Fatalf("did not expect unset filters: %q", query.Encode())
	}
}

// Verifies scope flags are validated before a request, unlike 'xidburst list'
// the time range stays optional
func TestXIDBurstOptionsRejectsInvalidScopeFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"xidNumbers":[]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "window with start",
			args: []string{"--window", "24h", "--start", "2026-05-01T00:00:00Z"},
			want: "--window cannot be used with --start or --end",
		},
		{
			name: "start without end",
			args: []string{"--start", "2026-05-01T00:00:00Z"},
			want: "--start and --end must be used together",
		},
		{
			name: "node group include and exclude",
			args: []string{"--nodegroup-ids", "ng-1", "--exclude-nodegroup-ids", "ng-2"},
			want: "node group include and exclude filters cannot be combined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(append([]string{"xidburst", "options"}, tt.args...))
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}

	if requests != 0 {
		t.Fatalf("expected no requests to reach the server, got %d", requests)
	}
}

func TestXIDBurstOptionsOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	tenantActions := cmdtest.SectionBody(t, got, "--tenant-actions")
	if !strings.Contains(tenantActions, "NO_PERSONA") || !strings.Contains(tenantActions, "RESTART_APP") {
		t.Fatalf("unexpected tenant action section:\n%s", tenantActions)
	}
	// A persona no flag covers is listed as unclassified rather than dropped or
	// folded into a persona it does not belong to.
	if strings.Contains(tenantActions, "NEW_PERSONA") {
		t.Fatalf("unknown persona leaked into the tenant section:\n%s", tenantActions)
	}
	unclassified := cmdtest.SectionBody(t, got, "suggestedActions with no matching persona/type flag  (no flag on 'xidburst list')")
	if !strings.Contains(unclassified, "NEW_PERSONA") || !strings.Contains(unclassified, "NO_TYPE") {
		t.Fatalf("unexpected unclassified action section:\n%s", unclassified)
	}
	if strings.Contains(cmdtest.SectionBody(t, got, "--dc-admin-actions"), "NO_PERSONA") {
		t.Fatalf("persona-less action leaked into the dc-admin section:\n%s", got)
	}
	if section := cmdtest.SectionBody(t, got, "--dc-admin-investigations"); !strings.Contains(section, "RUN_DIAGS") {
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jobDisruption":[true,false],"xidNumbers":[],"suggestedActions":[]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "options"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("xid burst options command failed: %v", err)
	}
	got := out.String()
	// Every filter except jobDisruption is absent or empty in this payload.
	if strings.Count(got, "(none)") != 9 {
		t.Fatalf("expected every empty filter to render as (none):\n%s", got)
	}
	if !strings.Contains(got, "--dc-admin-actions") {
		t.Fatalf("empty sections should still name their flag:\n%s", got)
	}
}

func TestXIDBurstPlatformDisruptionFilterBoundaries(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	queries := make(chan url.Values, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case queries <- r.URL.Query():
		default:
			t.Error("unexpected extra API request while recorder is full")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tt := range []struct {
		name string
		flag string
		want []string
	}{
		{name: "omitted"},
		{name: "false", flag: "--platform-disruption=false", want: []string{"false"}},
		{name: "true", flag: "--platform-disruption=true", want: []string{"true"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			args := []string{"xidburst", "list", "--window", "24h"}
			if tt.flag != "" {
				args = append(args, tt.flag)
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			select {
			case query := <-queries:
				got, present := query["jobDisruptionDueToPlatformIssue"]
				if present != (tt.want != nil) || !reflect.DeepEqual(got, tt.want) {
					t.Fatalf("platform disruption query: got %v (present %t), want %v", got, present, tt.want)
				}
			default:
				t.Fatal("expected a request to the server")
			}
			if !strings.Contains(out.String(), "burst-1") {
				t.Fatalf("output missing burst-1: %s", out.String())
			}
		})
	}
}

func TestXIDBurstDescribeSparseDetails(t *testing.T) {
	number := 48
	burst := nvfleetint.XIDBurstDetails{XIDBurst: nvfleetint.XIDBurst{
		XIDNumbers:       []nvfleetint.XIDBurstXID{{}, {XIDNumber: &number, Description: "Memory error"}},
		DeviceIDs:        map[string][]int{"z-device": {94}, "a-device": {48}},
		SuggestedActions: []nvfleetint.SuggestedAction{{Code: "DRAIN", Action: "Drain node"}},
	}}
	rows := xidBurstDescribeRows(burst)
	if got := rows[13]; !reflect.DeepEqual(got, []string{"XIDS", "48"}) {
		t.Errorf("XIDS row = %v, want [XIDS 48]", got)
	}
	wantDetails := [][]string{
		{"XID 48", "Memory error"},
		{"DEVICE a-device", "48"},
		{"DEVICE z-device", "94"},
		{"SUGGESTED ACTION", "DRAIN: Drain node"},
	}
	if got := rows[19:]; !reflect.DeepEqual(got, wantDetails) {
		t.Errorf("detail rows = %v, want %v", got, wantDetails)
	}
	if got := xidBurstDeviceRows(map[string][]int{}); len(got) != 0 {
		t.Errorf("empty device map produced rows: %v", got)
	}
	var out bytes.Buffer
	if err := writeXIDBurstDescribeTable(&out, burst); err != nil {
		t.Fatalf("render describe table: %v", err)
	}
	for _, want := range []string{"XID 48", "Memory error", "DEVICE a-device", "DEVICE z-device", "SUGGESTED ACTION", "DRAIN: Drain node"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("describe output missing %q: %s", want, out.String())
		}
	}
}

type xidBurstFailingWriter struct {
	bytes.Buffer
	failAt int
	calls  int
	err    error
}

func (w *xidBurstFailingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}

func TestXIDBurstRenderingWriteFailures(t *testing.T) {
	sentinel := errors.New("output failure")
	t.Run("list table", func(t *testing.T) {
		w := &xidBurstFailingWriter{failAt: 1, err: sentinel}
		err := writeXIDBurstListOutput(w, cmdutil.Resolved{}, xidBurstListOutput{Bursts: []nvfleetint.XIDBurst{{BurstID: "burst-1"}}})
		if !errors.Is(err, sentinel) || w.calls != 1 || w.Len() != 0 {
			t.Fatalf("error = %v, calls = %d, accepted = %q", err, w.calls, w.String())
		}
	})
	preamble := "Filters for 'xidburst list'\n" + cmdutil.OptionsPreamble + "\n"
	for _, tc := range []struct {
		name   string
		failAt int
		prefix string
	}{
		{"preamble", 1, ""},
		{"data source", 2, preamble},
		{"first section", 3, preamble + "Data source: inband\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &xidBurstFailingWriter{failAt: tc.failAt, err: sentinel}
			err := writeXIDBurstOptionsTable(w, nvfleetint.XIDBurstFilterOptions{DataSource: "inband"})
			if !errors.Is(err, sentinel) || w.calls != tc.failAt || w.String() != tc.prefix {
				t.Fatalf("error = %v, calls = %d, accepted = %q; want sentinel, %d, %q", err, w.calls, w.String(), tc.failAt, tc.prefix)
			}
		})
	}
}

func TestXIDBurstReadFlagValidationBeforeRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"describe output", []string{"xidburst", "describe", "burst-1", "--output", "yaml"}, `invalid output "yaml": expected table or json`},
		{"describe timeout", []string{"xidburst", "describe", "burst-1", "--timeout", "0s"}, "--timeout must be greater than 0"},
		{"options output", []string{"xidburst", "options", "--output", "yaml"}, `invalid output "yaml": expected table or json`},
		{"options timeout", []string{"xidburst", "options", "--timeout", "0s"}, "--timeout must be greater than 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 || requests.Load() != 0 {
				t.Errorf("output = %q, requests = %d; want neither", out.String(), requests.Load())
			}
		})
	}
}

func TestXIDBurstRejectsMalformedFilterLists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tt := range []struct {
		name  string
		args  []string
		wants []string
	}{
		{"empty xid", []string{"xidburst", "list", "--window", "24h", "--xid-numbers", "48,,94"}, []string{"--xid-numbers", "empty values are not allowed"}},
		{"negative xid", []string{"xidburst", "list", "--window", "24h", "--xid-numbers=-1"}, []string{"xid-numbers", "-1", "non-negative"}},
		{"empty category", []string{"xidburst", "list", "--window", "24h", "--categories", "GPU,,Memory"}, []string{"--categories", "empty values are not allowed"}},
		{"empty options node group", []string{"xidburst", "options", "--nodegroup-ids", "ng-1,,ng-2"}, []string{"--nodegroup-ids", "empty values are not allowed"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("expected no requests, got %d", got)
			}
			if out.Len() != 0 {
				t.Errorf("expected no rendered result, got %q", out.String())
			}
		})
	}
}

func TestXIDBurstCommandAPIErrorPropagation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
		all  bool
	}{
		{name: "list", args: []string{"xidburst", "list", "--window", "24h"}, path: "/v1/xid/bursts"},
		{name: "list all", args: []string{"xidburst", "list", "--window", "24h", "--all", "--page-size", "1", "--output", "json"}, path: "/v1/xid/bursts", all: true},
		{name: "describe", args: []string{"xidburst", "describe", "burst-1"}, path: "/v1/xid/bursts/burst-1"},
		{name: "options", args: []string{"xidburst", "options"}, path: "/v1/xid/bursts/options"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			type request struct{ method, path, page, pageSize, authorization string }
			var mu sync.Mutex
			var requests []request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests = append(requests, request{r.Method, r.URL.Path, r.URL.Query().Get("page"), r.URL.Query().Get("pageSize"), r.Header.Get("Authorization")})
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if tc.all && r.URL.Query().Get("page") == "0" {
					_, _ = w.Write([]byte(`{"items":[{"burstId":"burst-1"}],"page":0,"pageSize":1,"total":2}`))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden","details":"no XID burst access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			t.Run("API error", func(t *testing.T) {
				var out bytes.Buffer
				cmd := newRootCmd()
				cmd.SetOut(&out)
				cmd.SetArgs(tc.args)
				err := cmd.Execute()
				var apiErr *nvfleetint.APIError
				if !errors.As(err, &apiErr) {
					t.Fatalf("expected APIError, got %T: %v", err, err)
				}
				if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "forbidden" || apiErr.Details != "no XID burst access" {
					t.Errorf("unexpected API error: %+v", apiErr)
				}
				if out.Len() != 0 {
					t.Errorf("unexpected command output: %q", out.String())
				}
				mu.Lock()
				observed := append([]request(nil), requests...)
				mu.Unlock()
				wantCount := 1
				if tc.all {
					wantCount = 2
				}
				if len(observed) != wantCount {
					t.Fatalf("got requests %+v, want %d requests", observed, wantCount)
				}
				var pages []string
				for _, got := range observed {
					if got.method != http.MethodGet || got.path != tc.path || got.authorization != "Bearer test-key" {
						t.Errorf("unexpected request: %+v; want GET %s with test key", got, tc.path)
					}
					if tc.all && got.pageSize != "1" {
						t.Errorf("pageSize = %q, want 1", got.pageSize)
					}
					pages = append(pages, got.page)
				}
				if tc.all && !reflect.DeepEqual(pages, []string{"0", "1"}) {
					t.Errorf("requested pages = %v, want [0 1]", pages)
				}
			})

			t.Run("missing profile", func(t *testing.T) {
				mu.Lock()
				requests = nil
				mu.Unlock()
				var out bytes.Buffer
				cmd := newRootCmd()
				cmd.SetOut(&out)
				args := append([]string(nil), tc.args...)
				cmd.SetArgs(append(args, "--profile", "nonexistent"))
				err := cmd.Execute()
				if !errors.Is(err, config.ErrProfileNotFound) {
					t.Errorf("expected profile not found error, got %v", err)
				}
				if out.Len() != 0 {
					t.Errorf("unexpected command output: %q", out.String())
				}
				mu.Lock()
				defer mu.Unlock()
				if len(requests) != 0 {
					t.Errorf("unexpected requests: %+v", requests)
				}
			})
		})
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
			body := []byte(fmt.Sprintf(`{"items":[{"burstId":"pagination-burst","nodeUuid":"node-1","xidCount":2}],"page":%d,"pageSize":7,"total":%d,"hasMore":false,"opaquePaginationContract":"preserved"}`, apiPage, apiPage*pageSize+1))
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
			args := []string{"xidburst", "list", "--window", "24h"}
			args = append(args, "--page", strconv.Itoa(cliPage), "--page-size", strconv.Itoa(pageSize), "--output", "json")
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			if got := len(requests); got != 1 {
				t.Fatalf("API request count = %d, want 1", got)
			}
			gotRequest := <-requests
			wantRequest := request{http.MethodGet, "/v1/xid/bursts", strconv.Itoa(apiPage), strconv.Itoa(pageSize)}
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

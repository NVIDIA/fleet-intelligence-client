// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

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

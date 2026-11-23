// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
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

// Verifies all-page alert JSON output
func TestAlertListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"gpu","severity":"Critical","state":"Triggered","triggeredAt":"2026-05-01T00:00:00Z","extra":"kept"}],"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-2","nodeUUID":"node-1","component":"memory","severity":"Critical","state":"Triggered","triggeredAt":"2026-05-01T00:01:00Z"}],"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("page"); got != "0" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := r.URL.Query().Get("component"); got != "gpu" {
			t.Fatalf("unexpected component: %q", got)
		}
		if got := r.URL.Query().Get("state"); got != "Triggered" {
			t.Fatalf("unexpected state: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"gpu","severity":"Warning","state":"Detected","detectedAt":"2026-05-01T00:00:00Z"}],"page":0,"pageSize":1,"total":2}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/alert_timeline/nodes":
			query := r.URL.Query()
			if got := query.Get("active"); got != "true" {
				t.Fatalf("unexpected active: %q", got)
			}
			for name, want := range map[string]string{"nameSearch": "gpu", "sortBy": "alert", "order": "desc"} {
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

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "summary", "--hostname", "gpu", "--sort-by", "alert", "--order", "desc", "--component-type", "gpu,memory"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("timeline command failed: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "NODE UUID") || !strings.Contains(got, "CRITICAL") || !strings.Contains(got, "WARNING") || !strings.Contains(got, "H100") || !strings.Contains(got, "Training") || !strings.Contains(got, "East") || !strings.Contains(got, "node-1") {
		t.Fatalf("timeline node output missing fields: %q", got)
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "node", "node-1", "--without-psirt", "--sort-by", "startTime", "--order", "asc"})
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUuid":"node-1","criticalCount":2,"warningCount":1}],"hasMore":false,"page":0,"pageSize":100,"total":1,"totalCritical":2,"totalWarning":1,"totalResolved":0,"distinctGpuTypeCount":1,"distinctNodeGroupCount":1,"distinctComputeZoneCount":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "summary", "--all", "--output", "json"})
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-1","component":"gpu","alertStatus":"Critical"}],"hasMore":false,"page":0,"pageSize":100,"total":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "node", "node-1", "--all", "--output", "json"})
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

// Verifies the CLI exposes alert options in JSON and table formats.
func TestAlertTimelineOptionsOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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

	cmdtest.SaveConfig(t, server.URL, "test-key")
	var jsonOut bytes.Buffer
	jsonCmd := newRootCmd()
	jsonCmd.SetOut(&jsonOut)
	jsonCmd.SetArgs([]string{"alert", "options", "--output", "json"})
	if err := jsonCmd.Execute(); err != nil {
		t.Fatalf("alert options JSON command failed: %v", err)
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
	tableCmd.SetArgs([]string{"alert", "options", "--view", "historical"})
	if err := tableCmd.Execute(); err != nil {
		t.Fatalf("alert options table command failed: %v", err)
	}
	for _, want := range []string{
		"--gpu-type", "H100",
		"--nodegroup-ids", "ng-1", "Training",
		"--sort-by", "hostname", "--order", "(default: alert)", "(default: desc)",
	} {
		if !strings.Contains(tableOut.String(), want) {
			t.Fatalf("table output missing %q:\n%s", want, tableOut.String())
		}
	}

	// The endpoint advertises the summary's columns only, so that is the one
	// sorting section rendered; `alert node`'s columns are documented on its
	// own flags rather than invented here.
	if !strings.Contains(tableOut.String(), "Sorting for 'alert summary'") {
		t.Fatalf("missing 'alert summary' sorting section:\n%s", tableOut.String())
	}
	if strings.Contains(tableOut.String(), "Sorting for 'alert node'") {
		t.Fatalf("'alert node' sorting is not advertised by the endpoint:\n%s", tableOut.String())
	}
	if strings.Contains(tableOut.String(), "startTime") {
		t.Fatalf("options table invents a sort column the endpoint never returned:\n%s", tableOut.String())
	}
}

// Verifies filter fields with no matching flag, empty option lists, and absent
// sorting defaults still render rather than being dropped.
func TestAlertTimelineOptionsTableFallbacks(t *testing.T) {
	var out bytes.Buffer
	err := writeAlertTimelineOptionsTable(&out, nvfleetint.AlertTimelineFilterOptions{
		Filters: nvfleetint.AlertTimelineFilters{Fields: []nvfleetint.AlertTimelineFilterField{
			{Name: "newBackendFilter", Options: []nvfleetint.AlertTimelineFilterOption{{Value: "somevalue"}}},
			{Name: "alertStates"},
		}},
		Sorting: nvfleetint.AlertTimelineSortingOptions{Fields: []string{"alert"}, Orders: []string{"asc"}},
	})
	if err != nil {
		t.Fatalf("render options table failed: %v", err)
	}
	for _, want := range []string{"newBackendFilter", "no flag on", "somevalue", "--alert-state", "(none)"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "default:") {
		t.Fatalf("absent sorting defaults should not render:\n%s", out.String())
	}
}

// Verifies `alert node` documents its own sort columns on its flags. The
// options endpoint advertises `alert summary`'s columns only, so --help is the
// sole discovery path for these two.
func TestAlertNodeHelpDocumentsSortColumns(t *testing.T) {
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "node", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("alert node --help failed: %v", err)
	}
	for _, want := range []string{"startTime, lastUpdate, or dataSource", "default: lastUpdate", "asc or desc", "default: desc"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("alert node help missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "--sort-by, and --order") {
		t.Fatalf("alert node help still points --sort-by at the options command:\n%s", out.String())
	}
}

// Verifies the visible alert workflow starts from the active summary view.
func TestAlertSummaryUX(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("active"); got != "true" {
			t.Fatalf("summary should default to active view, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[],"hasMore":false,"page":0,"pageSize":50,"total":0,"totalCritical":0,"totalWarning":0,"totalResolved":0,"distinctGpuTypeCount":0,"distinctNodeGroupCount":0,"distinctComputeZoneCount":0}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "summary", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("alert summary failed: %v", err)
	}

	var help bytes.Buffer
	helpCmd := newRootCmd()
	helpCmd.SetOut(&help)
	helpCmd.SetArgs([]string{"alert", "--help"})
	if err := helpCmd.Execute(); err != nil {
		t.Fatalf("alert help failed: %v", err)
	}
	for _, want := range []string{"summary", "node", "list", "describe", "options", "summary → node → describe"} {
		if !strings.Contains(help.String(), want) {
			t.Fatalf("alert help missing %q:\n%s", want, help.String())
		}
	}
	if strings.Contains(help.String(), "  timeline ") {
		t.Fatalf("removed timeline command should not be listed:\n%s", help.String())
	}
}

// Verifies the unreleased timeline command was removed instead of retained as an alias.
func TestAlertTimelineCommandRemoved(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "timeline"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected removed-command error, got %v", err)
	}
}

// Verifies alert views default to active and accept explicit historical selection.
func TestResolveAlertView(t *testing.T) {
	for _, test := range []struct {
		name    string
		view    string
		want    bool
		wantErr bool
	}{
		{name: "default", want: true},
		{name: "explicit active", view: "active", want: true},
		{name: "explicit historical", view: "historical", want: false},
		{name: "invalid", view: "recent", wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveAlertView(test.view)
			if (err != nil) != test.wantErr || got != test.want {
				t.Fatalf("resolveAlertView() = %v, %v; want %v, error=%v", got, err, test.want, test.wantErr)
			}
		})
	}
}

// Verifies alert describe timeline output
func TestAlertDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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

	cmdtest.SaveConfig(t, server.URL, "test-key")

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	longMessage := strings.Repeat("x", 200)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","timeline":[{"eventType":"triggered","alertStatus":"Critical","eventTimestamp":"2026-05-01T00:00:00Z","message":"` + longMessage + `"}]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

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

// Verifies mode-specific alert flags are rejected before a request
func TestAlertTimelineRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "summary sort", args: []string{"alert", "summary", "--sort-by", "startTime"}, want: "invalid sort-by"},
		{name: "node sort", args: []string{"alert", "node", "node-1", "--sort-by", "alert"}, want: "invalid sort-by"},
		// The API dropped the component sort; only startTime and lastUpdate remain.
		{name: "node component sort", args: []string{"alert", "node", "node-1", "--sort-by", "component"}, want: "expected startTime, lastUpdate, or dataSource"},
		{name: "missing node", args: []string{"alert", "node"}, want: "node UUID is required"},
		{name: "state", args: []string{"alert", "summary", "--alert-state", "Triggered"}, want: "invalid alert-state"},
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

type alertErrorWriter struct{ err error }

func (w alertErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestAlertTableOutputWriterErrors(t *testing.T) {
	sentinel := errors.New("table output failed")
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	for _, tc := range []struct {
		name  string
		write func() error
	}{
		{"list", func() error { return writeAlertListOutput(alertErrorWriter{sentinel}, common, alertListOutput{}) }},
		{"timeline nodes", func() error {
			return writeAlertTimelineOutput(alertErrorWriter{sentinel}, common, alertTimelineOutput{Mode: alertTimelineModeNodes})
		}},
		{"timeline alerts", func() error {
			return writeAlertTimelineOutput(alertErrorWriter{sentinel}, common, alertTimelineOutput{Mode: alertTimelineModeAlerts})
		}},
		{"options", func() error {
			return writeAlertTimelineOptionsTable(alertErrorWriter{sentinel}, nvfleetint.AlertTimelineFilterOptions{})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.write(); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
		})
	}
}

func TestAlertCommandsRejectMissingProfile(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"alert", "list"}},
		{name: "summary", args: []string{"alert", "summary"}},
		{name: "node", args: []string{"alert", "node", "node-1"}},
		{name: "options", args: []string{"alert", "options"}},
		{name: "describe", args: []string{"alert", "describe", "alert-1", "--node", "node-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "configured-key")
			t.Setenv(config.EnvAPIURL, server.URL)
			t.Setenv(config.EnvAPIKey, "environment-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append(append([]string(nil), tc.args...), "--profile", "missing"))
			err := cmd.Execute()
			if !errors.Is(err, config.ErrProfileNotFound) {
				t.Errorf("error = %v, want ErrProfileNotFound", err)
			}
			if err == nil || !strings.Contains(err.Error(), "nvfleetint auth list") {
				t.Errorf("error = %v, want auth list hint", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("got %d HTTP requests, want none", got)
			}
		})
	}
}

func TestAlertNodeWithoutPSIRTRejectsBlankNode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "node", "   ", "--without-psirt"})
	err := cmd.Execute()
	if err == nil || err.Error() != "--without-psirt requires --node" {
		t.Errorf("error = %v, want --without-psirt requires --node", err)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output: %q", out.String())
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("got %d HTTP requests, want none", got)
	}
}

func TestAlertTablesWithoutPagination(t *testing.T) {
	common := cmdutil.Resolved{Output: clioutput.FormatTable}
	var out bytes.Buffer
	if err := writeAlertListOutput(&out, common, alertListOutput{Alerts: []nvfleetint.Alert{{UUID: "alert-1", NodeUUID: "node-1", Component: "gpu", Severity: "critical"}}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"UUID", "NODE UUID", "COMPONENT", "SEVERITY", "alert-1", "node-1", "gpu", "critical"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list output %q missing %q", out.String(), want)
		}
	}
	if strings.Contains(out.String(), "Page:") {
		t.Errorf("unexpected pagination footer: %q", out.String())
	}

	out.Reset()
	node := nvfleetint.AlertTimelineNode{NodeUUID: "node-2", Hostname: "host-2", AgentStatus: &nvfleetint.AlertAgentStatuses{Inband: &nvfleetint.AlertAgentSourceStatus{Status: "online"}}, CriticalCount: 3}
	if err := writeAlertTimelineOutput(&out, common, alertTimelineOutput{Mode: alertTimelineModeNodes, Nodes: []nvfleetint.AlertTimelineNode{node}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"NODE UUID", "HOSTNAME", "INBAND STATUS", "OOB STATUS", "CRITICAL", "node-2", "host-2"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("node output %q missing %q", out.String(), want)
		}
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("node output has %d lines: %q", len(lines), out.String())
	}
	fields := strings.Fields(lines[1])
	if len(fields) != 13 || fields[4] != "online" || fields[5] != "-" || fields[6] != "3" {
		t.Errorf("node row = %v, want inband online, missing OOB -, and critical count 3", fields)
	}
	if strings.Contains(out.String(), "not_enrolled") || strings.Contains(out.String(), "Page:") {
		t.Errorf("unexpected status or pagination footer: %q", out.String())
	}

	out.Reset()
	if err := writeAlertTimelineOutput(&out, common, alertTimelineOutput{Mode: alertTimelineModeAlerts, Alerts: []nvfleetint.AlertTimelineNodeAlert{{AlertUUID: "alert-2", Component: "memory", AlertStatus: "Detected"}}}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ALERT UUID", "COMPONENT", "STATUS", "alert-2", "memory", "Detected"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("timeline output %q missing %q", out.String(), want)
		}
	}
	if strings.Contains(out.String(), "Page:") {
		t.Errorf("unexpected pagination footer: %q", out.String())
	}
}

func TestAlertDescribePropagatesTableWriterError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	type request struct{ method, path string }
	requests := make(chan request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requests <- request{r.Method, r.URL.Path}:
		default:
			t.Error("unexpected extra API request while recorder is full")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","timeline":[{"eventType":"triggered","message":"GPU critical"}]}`))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	sentinel := errors.New("describe output failed")
	cmd := newRootCmd()
	cmd.SetOut(alertErrorWriter{sentinel})
	cmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1"})
	if err := cmd.Execute(); !errors.Is(err, sentinel) {
		t.Errorf("error = %v, want sentinel", err)
	}
	select {
	case got := <-requests:
		if got.method != http.MethodGet || got.path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" {
			t.Errorf("unexpected request: %+v", got)
		}
	default:
		t.Error("expected detail request")
	}
	select {
	case extra := <-requests:
		t.Errorf("unexpected extra request: %+v", extra)
	default:
	}
}

func TestAlertCommandAPIErrorPropagation(t *testing.T) {
	const node = "node-1"
	const alertID = "alert-1"
	tests := []struct {
		name      string
		args      []string
		path      string
		firstPage string
		all       bool
	}{
		{name: "list", args: []string{"alert", "list"}, path: "/v1/alerts"},
		{name: "summary", args: []string{"alert", "summary"}, path: "/v1/alert_timeline/nodes"},
		{name: "node", args: []string{"alert", "node", node}, path: "/v1/alert_timeline/nodes/" + node + "/alerts"},
		{name: "options", args: []string{"alert", "options"}, path: "/v1/alert_timeline/filter_options"},
		{name: "describe", args: []string{"alert", "describe", alertID, "--node", node}, path: "/v1/alert_timeline/nodes/" + node + "/alerts/" + alertID},
		{name: "list all", args: []string{"alert", "list"}, path: "/v1/alerts", all: true, firstPage: `{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`},
		{name: "summary all", args: []string{"alert", "summary"}, path: "/v1/alert_timeline/nodes", all: true, firstPage: `{"nodes":[{"nodeUuid":"node-1"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`},
		{name: "node all", args: []string{"alert", "node", node}, path: "/v1/alert_timeline/nodes/" + node + "/alerts", all: true, firstPage: `{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-1"}],"page":0,"pageSize":1,"total":2,"hasMore":true}`},
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
					_, _ = w.Write([]byte(tc.firstPage))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden","details":"no alert access"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			args := append([]string(nil), tc.args...)
			if tc.all {
				args = append(args, "--all", "--page-size", "1", "--output", "json")
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			err := cmd.Execute()
			var apiErr *nvfleetint.APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("expected APIError, got %T: %v", err, err)
			}
			if apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "forbidden" || apiErr.Details != "no alert access" {
				t.Errorf("unexpected API error: %+v", apiErr)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected command output: %q", out.String())
			}

			mu.Lock()
			observed := append([]request(nil), requests...)
			mu.Unlock()
			wantPages := []string{""}
			if tc.all {
				wantPages = []string{"0", "1"}
			}
			if len(observed) != len(wantPages) {
				t.Fatalf("got requests %+v, want %d requests", observed, len(wantPages))
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
			if tc.all && !reflect.DeepEqual(pages, wantPages) {
				t.Errorf("requested pages = %v, want %v", pages, wantPages)
			}
		})
	}
}

func TestAlertDescribeJSONPaginationPreservesPayload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	const payload = `{"alertUuid":"alert-1","nodeUuid":"node-1","page":1,"pageSize":1,"total":3,"timeline":[{"eventType":"triggered","message":"GPU critical"}],"unknown":{"nested":{"value":"retained"}}}`
	type request struct {
		method, path   string
		page, pageSize []string
	}
	requests := make(chan request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		select {
		case requests <- request{r.Method, r.URL.Path, query["page"], query["pageSize"]}:
		default:
			t.Error("unexpected extra API request while recorder is full")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tc := range []struct {
		name           string
		flags          []string
		page, pageSize []string
		outputPage     float64
	}{
		{name: "paginated", flags: []string{"--page", "2", "--page-size", "1"}, page: []string{"1"}, pageSize: []string{"1"}, outputPage: 2},
		{name: "full timeline", outputPage: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"alert", "describe", "alert-1", "--node", "node-1", "--output", "json"}
			args = append(args, tc.flags...)
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("describe failed: %v", err)
			}
			var gotRequest request
			select {
			case gotRequest = <-requests:
			default:
				t.Fatal("expected a describe request")
			}
			if gotRequest.method != http.MethodGet || gotRequest.path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" || !reflect.DeepEqual(gotRequest.page, tc.page) || !reflect.DeepEqual(gotRequest.pageSize, tc.pageSize) {
				t.Errorf("unexpected request: %+v", gotRequest)
			}
			var got, want map[string]any
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode output %q: %v", out.String(), err)
			}
			if err := json.Unmarshal([]byte(payload), &want); err != nil {
				t.Fatal(err)
			}
			want["page"] = tc.outputPage
			if !reflect.DeepEqual(got, want) {
				t.Errorf("output = %#v, want %#v", got, want)
			}
		})
	}
	select {
	case extra := <-requests:
		t.Errorf("unexpected extra request: %+v", extra)
	default:
	}
}

func TestAlertDescribeRejectsInvalidInputsBeforeRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "blank UUID", args: []string{"   ", "--node", "node-1"}, want: "alert UUID is required"},
		{name: "invalid order", args: []string{"alert-1", "--node", "node-1", "--order", "invalid"}, want: "invalid order \"invalid\": expected asc or desc"},
		{name: "page zero", args: []string{"alert-1", "--node", "node-1", "--page", "0", "--page-size", "1"}, want: "--page must be greater than or equal to 1"},
		{name: "page size zero", args: []string{"alert-1", "--node", "node-1", "--page-size", "0"}, want: "--page-size must be between 1 and 100"},
		{name: "page size too large", args: []string{"alert-1", "--node", "node-1", "--page-size", "101"}, want: "--page-size must be between 1 and 100"},
		{name: "timeout zero", args: []string{"alert-1", "--node", "node-1", "--timeout", "0"}, want: "--timeout must be greater than 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"alert", "describe"}, tc.args...))
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("got %d HTTP requests, want none", got)
			}
		})
	}
}

func TestAlertTimelineRejectsInvalidInputsBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "summary invalid view", args: []string{"summary", "--view", "recent"}, want: []string{"invalid view"}},
		{name: "node invalid view", args: []string{"node", "node-1", "--view", "recent"}, want: []string{"invalid view"}},
		{name: "options invalid view", args: []string{"options", "--view", "recent"}, want: []string{"invalid view"}},
		{name: "summary empty alert state", args: []string{"summary", "--alert-state", "Detected,,Resolved"}, want: []string{"invalid alert-state", "empty values"}},
		{name: "node empty alert state", args: []string{"node", "node-1", "--alert-state", "Detected,,Resolved"}, want: []string{"invalid alert-state", "empty values"}},
		{name: "summary empty compute zone", args: []string{"summary", "--compute-zone-ids", "zone-1,,zone-2"}, want: []string{"compute-zone-ids", "empty values"}},
		{name: "node empty component type", args: []string{"node", "node-1", "--component-type", "gpu,,memory"}, want: []string{"component-type", "empty values"}},
		{name: "summary empty oob status", args: []string{"summary", "--oob-agent-status", "online,,offline"}, want: []string{"oob-agent-status", "empty values"}},
		{name: "node empty data source", args: []string{"node", "node-1", "--data-source", "inband,,oob"}, want: []string{"data-source", "empty values"}},
		{name: "summary page with all", args: []string{"summary", "--all", "--page", "1"}, want: []string{"--page cannot be used with --all"}},
		{name: "node page with all", args: []string{"node", "node-1", "--all", "--page", "1"}, want: []string{"--page cannot be used with --all"}},
		{name: "options zero timeout", args: []string{"options", "--timeout", "0"}, want: []string{"--timeout must be greater than 0"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"alert"}, tc.args...))
			err := cmd.Execute()
			for _, want := range tc.want {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("error = %v, want substring %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("got %d HTTP requests, want none", got)
			}
		})
	}
}

// Both timeline views must translate the user's 1-based page into the SDK's
// 0-based page, while preserving the requested size and displaying page 2.
func TestTimelineExplicitPageRoundTrip(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, itemKey, idKey, id string
		args                                 []string
	}{
		{
			name: "summary", path: "/v1/alert_timeline/nodes",
			args: []string{"alert", "summary"}, itemKey: "nodes", idKey: "nodeUuid", id: "node-2",
			body: `{"nodes":[{"nodeUuid":"node-2","hostname":"gpu-002","criticalCount":1}],"page":1,"pageSize":1,"total":3,"hasMore":true,"totalCritical":7}`,
		},
		{
			name: "node", path: "/v1/alert_timeline/nodes/node-1/alerts",
			args: []string{"alert", "node", "node-1"}, itemKey: "alerts", idKey: "alertUuid", id: "alert-2",
			body: `{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-2","component":"gpu","alertStatus":"Critical"}],"page":1,"pageSize":1,"total":3,"hasMore":true}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodGet || r.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", r.Method, r.URL.Path, tc.path)
				}
				if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("pageSize") != "1" {
					t.Errorf("pagination query = %s, want page=1 and pageSize=1", r.URL.RawQuery)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("request did not use the configured API key")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append(tc.args, "--page", "2", "--page-size", "1", "--output", "json"))
			if err := cmd.Execute(); err != nil {
				t.Fatalf("execute: %v", err)
			}
			var got map[string]json.RawMessage
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode output: %v; %s", err, out.String())
			}
			for key, want := range map[string]string{"page": "2", "pageSize": "1", "total": "3", "hasMore": "true"} {
				if string(got[key]) != want {
					t.Errorf("output %s = %s, want %s", key, got[key], want)
				}
			}
			var items []map[string]any
			if err := json.Unmarshal(got[tc.itemKey], &items); err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 || items[0][tc.idKey] != tc.id {
				t.Errorf("unexpected page items: %#v", items)
			}
			if requests.Load() != 1 {
				t.Errorf("requests = %d, want exactly one requested page", requests.Load())
			}
		})
	}
}

func TestRound2AlertSummaryTimelineFlagValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   alertTimelineFlags
		wantErr string
	}{
		{
			name:    "without PSIRT requires node",
			flags:   alertTimelineFlags{node: "  ", withoutPSIRT: true},
			wantErr: "--without-psirt requires --node",
		},
		{
			name:    "hostname conflicts with node",
			flags:   alertTimelineFlags{node: " node-1 ", hostname: " host-1 "},
			wantErr: "--hostname cannot be used with --node",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&out)
			err := runAlertSummary(cmd, tc.flags, cmdutil.Resolved{Output: "table", Timeout: time.Second})
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want empty", out.String())
			}
		})
	}

	if err := validateAlertTimelineFlags(alertTimelineFlags{node: " node-1 ", hostname: "   "}); err != nil {
		t.Errorf("blank hostname validation error = %v, want nil", err)
	}
}

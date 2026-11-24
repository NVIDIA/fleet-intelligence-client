// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

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

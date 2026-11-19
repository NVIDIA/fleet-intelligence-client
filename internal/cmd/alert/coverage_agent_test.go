// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	clioutput "github.com/NVIDIA/fleet-intelligence-client/internal/output"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

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
	type request struct{ method, path string }
	requests := make(chan request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- request{r.Method, r.URL.Path}
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
	const payload = `{"alertUuid":"alert-1","nodeUuid":"node-1","page":1,"pageSize":1,"total":3,"timeline":[{"eventType":"triggered","message":"GPU critical"}],"unknown":{"nested":{"value":"retained"}}}`
	type request struct {
		method, path   string
		page, pageSize []string
	}
	requests := make(chan request, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		requests <- request{r.Method, r.URL.Path, query["page"], query["pageSize"]}
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
			gotRequest := <-requests
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

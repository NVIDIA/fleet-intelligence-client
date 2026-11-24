// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

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

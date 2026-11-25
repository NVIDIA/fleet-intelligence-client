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

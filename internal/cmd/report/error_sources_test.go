// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestErrorReportNodeSources(t *testing.T) {
	for _, source := range []string{"", "inband", "oob"} {
		for _, output := range []string{"table", "json"} {
			for _, all := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/all=%v", source, output, all), func(t *testing.T) {
					t.Setenv("HOME", t.TempDir())
					requests := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests++
						q := r.URL.Query()
						if q.Get("dataSource") != source || q.Has("dataSource") != (source != "") || q.Has("agentType") {
							t.Errorf("incorrect source: %s", r.URL)
						}
						if q.Get("window") != "24h" || q.Get("groupBy") != "node" || q.Get("tags") != "prod" || q.Get("pageSize") != "1" {
							t.Errorf("lost filters: %s", r.URL)
						}
						page, _ := strconv.Atoi(q.Get("page"))
						identity := fmt.Sprintf(`"hostname":"gpu-%d"`, page)
						if source == "oob" {
							identity = fmt.Sprintf(`"nodeName":"machine-%d","bmcMac":"02:00:0a:00:00:%02x"`, page, page)
						}
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprintf(w, `{"nodes":[{"nodeUUID":"n%d",%s,"errors":["Xid"],"futureField":"retained"}],"page":%d,"pageSize":1,"total":2,"hasMore":%v}`, page, identity, page, page == 0)
					}))
					defer server.Close()
					cmdtest.SaveConfig(t, server.URL, "test-key")
					var out bytes.Buffer
					cmd := newRootCmd()
					cmd.SetOut(&out)
					cmd.SetErr(&bytes.Buffer{})
					args := []string{"report", "error", "--view", "list", "--group-by", "node", "--window", "24h", "--tags", "prod", "--page-size", "1", "--output", output}
					if source != "" {
						args = append(args, "--data-source", source)
					}
					if all {
						args = append(args, "--all")
					}
					cmd.SetArgs(args)
					if err := cmd.Execute(); err != nil {
						t.Fatal(err)
					}
					wantRequests := 1
					if all {
						wantRequests = 2
					}
					if requests != wantRequests {
						t.Fatalf("requests=%d, want %d", requests, wantRequests)
					}
					for page := 0; page < wantRequests; page++ {
						want := fmt.Sprintf("gpu-%d", page)
						if source == "oob" {
							want = fmt.Sprintf("02:00:0a:00:00:%02x", page)
							if !strings.Contains(out.String(), fmt.Sprintf("machine-%d", page)) {
								t.Fatalf("missing node name: %s", &out)
							}
						}
						if !strings.Contains(out.String(), want) {
							t.Fatalf("missing %s: %s", want, &out)
						}
					}
					if output == "json" {
						var payload map[string]json.RawMessage
						if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
							t.Fatal(err)
						}
						key := "nodes"
						if all {
							key = "items"
						}
						if len(payload[key]) == 0 || !strings.Contains(out.String(), `"futureField"`) || payload["oob"] != nil || payload["inband"] != nil {
							t.Fatalf("unexpected JSON shape: %s", &out)
						}
					} else if source == "oob" {
						if !strings.Contains(out.String(), "BMC MAC") || strings.Contains(out.String(), "HOSTNAME") {
							t.Fatalf("wrong OOB columns: %s", &out)
						}
					} else if !strings.Contains(out.String(), "HOSTNAME") || strings.Contains(out.String(), "BMC MAC") {
						t.Fatalf("wrong in-band columns: %s", &out)
					}
				})
			}
		}
	}
}

func TestErrorReportSourceOtherViewsAndCSV(t *testing.T) {
	for _, source := range []string{"", "inband", "oob"} {
		for _, tc := range []struct {
			name string
			args []string
			body string
		}{
			{"overview", nil, `{"totalErrors":2}`},
			{"graph", []string{"--view", "graph"}, `{"result":[]}`},
			{"errors", []string{"--view", "list", "--group-by", "error"}, `{"errors":[],"total":0}`},
			{"empty-nodes", []string{"--view", "list", "--group-by", "node"}, `{"nodes":[],"total":0}`},
			{"csv", []string{"--view", "list", "--group-by", "node", "--format", "csv"}, "Node UUID,Node Name,BMC MAC,Errors\nn1,machine-1,02:00:0a:00:00:0a,Xid\n"},
		} {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					if q := r.URL.Query(); q.Get("dataSource") != source || q.Has("dataSource") != (source != "") {
						t.Errorf("lost source: %s", r.URL)
					}
					contentType := "application/json"
					if tc.name == "csv" {
						contentType = "text/csv"
					}
					w.Header().Set("Content-Type", contentType)
					fmt.Fprint(w, tc.body)
				}))
				defer server.Close()
				cmdtest.SaveConfig(t, server.URL, "test-key")
				cmd := newRootCmd()
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(&bytes.Buffer{})
				args := append([]string{"report", "error", "--window", "24h"}, tc.args...)
				if source != "" {
					args = append(args, "--data-source", source)
				}
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if requests != 1 {
					t.Fatalf("requests=%d", requests)
				}
				if tc.name == "csv" && out.String() != tc.body {
					t.Fatalf("CSV changed: %q", out.String())
				}
				if tc.name == "empty-nodes" && source == "oob" && (!strings.Contains(out.String(), "BMC MAC") || strings.Contains(out.String(), "HOSTNAME")) {
					t.Fatalf("empty OOB report lost source columns: %s", &out)
				}
			})
		}
	}
}

func TestErrorReportInvalidSourceBeforeAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, source := range []string{"all", "inband,oob", "invalid"} {
		cmd := newRootCmd()
		cmd.SetArgs([]string{"report", "error", "--window", "24h", "--data-source", source})
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "data-source") {
			t.Fatalf("source %q: %v", source, err)
		}
	}
}

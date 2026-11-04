// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestAlertSourceStatusCommands(t *testing.T) {
	for _, view := range []string{"active", "historical"} {
		for _, output := range []string{"table", "json"} {
			for _, all := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/all=%v", view, output, all), func(t *testing.T) {
					t.Setenv("HOME", t.TempDir())
					requests := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests++
						q := r.URL.Query()
						if !slices.Equal(q["dataSources"], []string{"inband", "oob"}) || (q.Get("active") == "true") != (view == "active") {
							t.Errorf("lost source/view: %s", r.URL)
						}
						page := 0
						if q.Get("page") == "1" {
							page = 1
						}
						w.Header().Set("Content-Type", "application/json")
						if r.URL.Path == "/v1/alert_timeline/nodes" {
							if !slices.Equal(q["inbandAgentStatuses"], []string{"offline", "not_enrolled"}) || q.Get("oobAgentStatuses") != "online" || q.Get("sortBy") != "nodeName" || q.Get("nameSearch") != "machine" {
								t.Errorf("lost summary filters: %s", r.URL)
							}
							fmt.Fprintf(w, `{"nodes":[{"nodeUuid":"n%d","nodeName":"machine%d","bmcMac":"bmc%d","agentStatus":{"inband":{"status":"not_enrolled"},"oob":{"status":"online","lastSeenAt":"2026-10-01T12:00:00Z"}}}],"page":%d,"pageSize":1,"total":2,"hasMore":%v,"totalCritical":9}`, page, page, page, page, page == 0)
						} else {
							if q.Has("inbandAgentStatuses") || q.Has("oobAgentStatuses") || q.Get("sortBy") != "dataSource" {
								t.Errorf("wrong node filters: %s", r.URL)
							}
							fmt.Fprintf(w, `{"nodeUuid":"n1","alerts":[{"alertUuid":"a%d","component":"gpu","dataSource":"oob","alertStatus":"Critical"}],"page":%d,"pageSize":1,"total":2,"hasMore":%v}`, page, page, page == 0)
						}
					}))
					defer server.Close()
					cmdtest.SaveConfig(t, server.URL, "test-key")
					for _, command := range []string{"summary", "node"} {
						before := requests
						args := []string{"alert", command}
						if command == "summary" {
							args = append(args, "--hostname", "machine", "--sort-by", "nodeName", "--inband-agent-status", "offline,not_enrolled", "--oob-agent-status", "online")
						} else {
							args = append(args, "n1", "--sort-by", "dataSource")
						}
						args = append(args, "--data-source", "inband,oob", "--view", view, "--output", output, "--page-size", "1")
						if all {
							args = append(args, "--all")
						}
						var out bytes.Buffer
						cmd := newRootCmd()
						cmd.SetOut(&out)
						cmd.SetArgs(args)
						if err := cmd.Execute(); err != nil {
							t.Fatal(err)
						}
						wantRequests := 1
						if all {
							wantRequests = 2
						}
						if requests-before != wantRequests {
							t.Fatalf("wrong page count: %d", requests-before)
						}
						if output == "json" {
							var result map[string]json.RawMessage
							if err := json.Unmarshal(out.Bytes(), &result); err != nil {
								t.Fatal(err)
							}
							if all && result["items"] == nil {
								t.Fatal("missing merged items")
							}
							if command == "summary" && (!strings.Contains(out.String(), "lastSeenAt") || string(result["totalCritical"]) != "9") {
								t.Fatalf("lost status/aggregates: %s", &out)
							}
						} else {
							wants := []string{"DATA SOURCE", "oob", "a0"}
							if command == "summary" {
								wants = []string{"NODE NAME", "BMC MAC", "INBAND STATUS", "OOB STATUS", "machine0", "bmc0", "not_enrolled", "online"}
							}
							for _, want := range wants {
								if !strings.Contains(out.String(), want) {
									t.Fatalf("missing %q: %s", want, &out)
								}
							}
						}
						if all {
							want := "a1"
							if command == "summary" {
								want = "machine1"
							}
							if !strings.Contains(out.String(), want) {
								t.Fatalf("lost second page: %s", &out)
							}
						}
					}
				})
			}
		}
	}
}

func TestAlertSourceStatusOptionsAndValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/v1/alert_timeline/filter_options" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"filters":{"fields":[{"name":"dataSources","options":["inband","oob"]},{"name":"inbandAgentStatuses","options":["offline","not_enrolled"]},{"name":"oobAgentStatuses","options":["online"]}]},"sorting":{"fields":["nodeName","bmcMac","nodeUUID"]}}`)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "options"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--data-source", "--inband-agent-status", "--oob-agent-status", "'alert summary' only", "nodeName", "bmcMac", "nodeUUID", "not_enrolled"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("options missing %q: %s", want, &out)
		}
	}
	before := requests
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"summary", "--inband-agent-status", "Online"}, "invalid inband-agent-status"},
		{[]string{"summary", "--oob-agent-status", "Unknown"}, "invalid oob-agent-status"},
		{[]string{"summary", "--data-source", "all"}, "invalid data-source"},
		{[]string{"node", "n1", "--data-source", "bad"}, "invalid data-source"},
		{[]string{"summary", "--data-source", "inband,,oob"}, "empty values"},
		{[]string{"summary", "--inband-agent-status", "online,,offline"}, "empty values"},
		{[]string{"node", "n1", "--oob-agent-status", "online"}, "unknown flag"},
	} {
		cmd := newRootCmd()
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		cmd.SetArgs(append([]string{"alert"}, tc.args...))
		if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: want %q, got %v", tc.args, tc.want, err)
		}
	}
	if requests != before {
		t.Fatal("invalid flags made requests")
	}
}

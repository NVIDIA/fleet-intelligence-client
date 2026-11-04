// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestAlertSourceFiltersAndStatusDecoding(t *testing.T) {
	for _, active := range []bool{false, true} {
		for _, sources := range [][]DataSource{nil, {DataSourceInband}, {DataSourceOOB}, {DataSourceInband, DataSourceOOB}} {
			t.Run(fmt.Sprintf("active=%v/sources=%v", active, sources), func(t *testing.T) {
				requests := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests++
					q := r.URL.Query()
					var want []string
					for _, source := range sources {
						want = append(want, string(source))
					}
					if !slices.Equal(q["dataSources"], want) || (len(sources) == 0 && q.Has("dataSources")) {
						t.Errorf("unexpected source query: %s", r.URL)
					}
					if (q.Get("active") == "true") != active {
						t.Errorf("wrong view: %s", r.URL)
					}
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == "/v1/alert_timeline/nodes" {
						if !slices.Equal(q["inbandAgentStatuses"], []string{"offline", "not_enrolled"}) || !slices.Equal(q["oobAgentStatuses"], []string{"online"}) {
							t.Errorf("lost status filters: %s", r.URL)
						}
						if q.Get("sortBy") != "bmcMac" || q.Get("nameSearch") != "machine" {
							t.Errorf("lost sort/search: %s", r.URL)
						}
						fmt.Fprint(w, `{"nodes":[{"nodeUuid":"n1","nodeName":"machine","bmcMac":"bmc","gpuType":"H100","agentStatus":{"inband":{"status":"not_enrolled"},"oob":{"status":"online","lastSeenAt":"2026-10-01T12:00:00Z"}}},{"nodeUuid":"n2"},{"nodeUuid":"n3","agentStatus":{"inband":{"status":"offline"}}}],"total":3}`)
					} else {
						if q.Get("sortBy") != "dataSource" || q.Has("inbandAgentStatuses") || q.Has("oobAgentStatuses") {
							t.Errorf("unexpected node-alert query: %s", r.URL)
						}
						fmt.Fprint(w, `{"nodeUuid":"n1","alerts":[{"alertUuid":"a1","dataSource":"oob"}],"total":1}`)
					}
				}))
				defer server.Close()
				client, err := NewClient(server.URL, "test-key")
				if err != nil {
					t.Fatal(err)
				}
				page, err := client.ListAlertTimelineNodes(context.Background(), ListAlertTimelineNodesOptions{
					Active: active, Hostname: "machine", SortBy: AlertTimelineNodeSortByBMCMAC, DataSources: sources,
					InbandAgentStatuses: []AlertAgentStatus{AlertAgentOffline, AlertAgentNotEnrolled}, OOBAgentStatuses: []AlertAgentStatus{AlertAgentOnline},
				})
				if err != nil {
					t.Fatal(err)
				}
				if len(page.Nodes) != 3 {
					t.Fatalf("wrong nodes: %#v", page.Nodes)
				}
				n := page.Nodes[0]
				if n.NodeName != "machine" || n.BMCMAC != "bmc" || n.GPUType != "H100" || n.AgentStatus == nil || n.AgentStatus.Inband == nil || n.AgentStatus.OOB == nil {
					t.Fatalf("lost fields: %#v", n)
				}
				if n.AgentStatus.Inband.Status != AlertAgentNotEnrolled || n.AgentStatus.Inband.LastSeenAt != "" || n.AgentStatus.OOB.Status != AlertAgentOnline || n.AgentStatus.OOB.LastSeenAt != "2026-10-01T12:00:00Z" {
					t.Fatalf("wrong status data: %#v", n.AgentStatus)
				}
				if page.Nodes[1].AgentStatus != nil || page.Nodes[2].AgentStatus.OOB != nil {
					t.Fatal("missing status converted to a present value")
				}
				alerts, err := client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{NodeUUID: "n1", Active: active, DataSources: sources, SortBy: AlertTimelineAlertSortByDataSource})
				if err != nil {
					t.Fatal(err)
				}
				if len(alerts.Alerts) != 1 || alerts.Alerts[0].DataSource != "oob" {
					t.Fatalf("lost source: %#v", alerts)
				}
				if requests != 2 {
					t.Fatalf("requests = %d", requests)
				}
			})
		}
	}
}

func TestAlertSourceDefaultsAndInvalidValues(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		for _, key := range []string{"dataSources", "inbandAgentStatuses", "oobAgentStatuses"} {
			if r.URL.Query().Has(key) {
				t.Errorf("default sent %s", key)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"nodes":[],"alerts":[{"alertUUID":"a1","nodeUUID":"n1","dataSource":"inband"}],"total":0}`)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, sortBy := range []AlertTimelineNodeSortBy{AlertTimelineNodeSortByNodeName, AlertTimelineNodeSortByBMCMAC, AlertTimelineNodeSortByNodeUUID} {
		if _, err := client.ListAlertTimelineNodes(context.Background(), ListAlertTimelineNodesOptions{SortBy: sortBy}); err != nil {
			t.Fatal(err)
		}
	}
	flat, err := client.ListAlerts(context.Background(), ListAlertsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(flat.Alerts) != 1 || flat.Alerts[0].DataSource != "inband" {
		t.Fatalf("flat alert lost source: %#v", flat)
	}
	before := requests
	for _, opts := range []ListAlertTimelineNodesOptions{
		{DataSources: []DataSource{"all"}},
		{DataSources: []DataSource{DataSourceOOB, "bad"}},
		{InbandAgentStatuses: []AlertAgentStatus{"Online"}},
		{OOBAgentStatuses: []AlertAgentStatus{"Unknown"}},
	} {
		if _, err := client.ListAlertTimelineNodes(context.Background(), opts); err == nil || !strings.Contains(err.Error(), "invalid") {
			t.Fatalf("invalid options accepted: %#v: %v", opts, err)
		}
	}
	if _, err := client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{NodeUUID: "n1", DataSources: []DataSource{"all"}}); err == nil {
		t.Fatal("invalid source accepted")
	}
	if requests != before {
		t.Fatal("invalid request reached HTTP")
	}
}

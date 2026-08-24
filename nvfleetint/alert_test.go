// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Verifies alert list request construction and decoding
func TestListAlertsSendsAuthAndParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("nodeUUID"); got != "node-1" {
			t.Fatalf("unexpected nodeUUID: %q", got)
		}
		if got := query.Get("severity"); got != "Critical" {
			t.Fatalf("unexpected severity: %q", got)
		}
		if got := query.Get("page"); got != "1" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := query.Get("pageSize"); got != "50" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"gpu","severity":"Critical","state":"Triggered","triggeredAt":"2026-05-01T00:00:00Z"}],"page":1,"pageSize":50,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	page := 1
	pageSize := 50
	got, err := client.ListAlerts(context.Background(), ListAlertsOptions{
		NodeUUID: "node-1",
		Severity: AlertSeverityCritical,
		Page:     &page,
		PageSize: &pageSize,
	})
	if err != nil {
		t.Fatalf("list alerts failed: %v", err)
	}
	if got.Page != 1 || got.PageSize != 50 || got.Total != 1 || len(got.Alerts) != 1 {
		t.Fatalf("unexpected page: %#v", got)
	}
	alert := got.Alerts[0]
	if alert.UUID != "alert-1" || alert.FiredAt != "2026-05-01T00:00:00Z" || alert.Severity != "Critical" {
		t.Fatalf("unexpected alert: %#v", alert)
	}
	if !strings.Contains(string(got.RawJSON), `"alerts"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies the progressive timeline SDK methods
func TestAlertTimelineMethodsDecode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/alert_timeline/nodes":
			query := r.URL.Query()
			if got := query.Get("active"); got != "true" {
				t.Fatalf("unexpected active: %q", got)
			}
			if got := query.Get("hostname"); got != "gpu" {
				t.Fatalf("unexpected hostname: %q", got)
			}
			if got := query.Get("sortBy"); got != "alert" {
				t.Fatalf("unexpected sortBy: %q", got)
			}
			if got := query.Get("order"); got != "desc" {
				t.Fatalf("unexpected order: %q", got)
			}
			if got := query["gpuTypes"]; !slices.Equal(got, []string{"H100", "B200"}) {
				t.Fatalf("unexpected gpuTypes: %#v", got)
			}
			if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1"}) {
				t.Fatalf("unexpected nodeGroupIds: %#v", got)
			}
			if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1"}) {
				t.Fatalf("unexpected computeZoneIds: %#v", got)
			}
			if got := query["alertStates"]; !slices.Equal(got, []string{"Critical", "Warning"}) {
				t.Fatalf("unexpected alertStates: %#v", got)
			}
			if got := query["componentTypes"]; !slices.Equal(got, []string{"gpu", "memory"}) {
				t.Fatalf("unexpected componentTypes: %#v", got)
			}
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUuid":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","gpuType":"H100","criticalCount":4,"warningCount":2,"resolvedCount":1,"hostStatus":"Active","lastAlertTime":"2026-05-01T00:00:00Z"}],"hasMore":false,"page":0,"pageSize":50,"total":1,"totalCritical":4,"totalWarning":2,"totalResolved":1,"distinctGpuTypeCount":1,"distinctNodeGroupCount":1,"distinctComputeZoneCount":1}`))
		case "/v1/alert_timeline/nodes/node-1/alerts":
			query := r.URL.Query()
			for name, want := range map[string]string{"active": "true", "withoutPsirt": "true", "sortBy": "startTime", "order": "asc"} {
				if got := query.Get(name); got != want {
					t.Fatalf("unexpected %s: %q", name, got)
				}
			}
			if got := query["alertStates"]; !slices.Equal(got, []string{"Critical"}) {
				t.Fatalf("unexpected alertStates: %#v", got)
			}
			if got := query["componentTypes"]; !slices.Equal(got, []string{"gpu"}) {
				t.Fatalf("unexpected componentTypes: %#v", got)
			}
			if got := query["gpuTypes"]; !slices.Equal(got, []string{"H100"}) {
				t.Fatalf("unexpected gpuTypes: %#v", got)
			}
			if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1"}) {
				t.Fatalf("unexpected nodeGroupIds: %#v", got)
			}
			if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1"}) {
				t.Fatalf("unexpected computeZoneIds: %#v", got)
			}
			_, _ = w.Write([]byte(`{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-1","component":"gpu","alertStatus":"Critical","startTime":"2026-04-30T00:00:00Z","lastEventTime":"2026-05-01T00:00:00Z"}],"hasMore":false,"page":0,"pageSize":50,"total":1}`))
		case "/v1/alert_timeline/nodes/node-1/alerts/alert-1":
			query := r.URL.Query()
			if query.Get("order") != "asc" || query.Get("page") != "1" || query.Get("pageSize") != "10" {
				t.Fatalf("unexpected detail query: %v", query)
			}
			_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","alertStatus":"Critical","nodeGroup":"Training","computeZone":"East","customerID":"customer-1","isBackendComponent":true,"hasMore":false,"page":1,"pageSize":10,"total":1,"timeline":[{"eventType":"triggered","alertStatus":"Critical","eventTimestamp":"2026-05-01T00:00:00Z","message":"GPU critical","extraInfo":{"gpu":"0"},"incidents":[{"id":"incident-1"}],"suggestedActions":[{"action":"Drain node","code":"DRAIN","persona":"dc_admin","type":"immediate"}]}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	nodes, err := client.ListAlertTimelineNodes(context.Background(), ListAlertTimelineNodesOptions{
		Active: true, Hostname: "gpu", SortBy: AlertTimelineNodeSortByAlert, Order: AlertTimelineOrderDesc,
		GPUTypes: []string{"H100", "B200"}, NodeGroupIDs: []string{"ng-1"}, ComputeZoneIDs: []string{"cz-1"},
		AlertStates: []AlertTimelineState{AlertTimelineStateCritical, AlertTimelineStateWarning}, ComponentTypes: []string{"gpu", "memory"},
	})
	if err != nil {
		t.Fatalf("timeline nodes failed: %v", err)
	}
	if len(nodes.Nodes) != 1 || nodes.Nodes[0].NodeUUID != "node-1" || nodes.Nodes[0].CriticalCount != 4 || nodes.Nodes[0].ComputeZone != "East" {
		t.Fatalf("unexpected timeline nodes: %#v", nodes.Nodes)
	}
	if nodes.TotalCritical != 4 || nodes.TotalWarning != 2 || nodes.TotalResolved != 1 || nodes.DistinctGPUTypeCount != 1 || nodes.DistinctNodeGroupCount != 1 || nodes.DistinctComputeZoneCount != 1 {
		t.Fatalf("unexpected timeline aggregates: %#v", nodes)
	}

	alerts, err := client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{
		NodeUUID: "node-1", Active: true, WithoutPSIRT: true, SortBy: AlertTimelineAlertSortByStartTime, Order: AlertTimelineOrderAsc,
		AlertStates: []AlertTimelineState{AlertTimelineStateCritical}, ComponentTypes: []string{"gpu"}, GPUTypes: []string{"H100"},
		NodeGroupIDs: []string{"ng-1"}, ComputeZoneIDs: []string{"cz-1"},
	})
	if err != nil {
		t.Fatalf("node alert timeline failed: %v", err)
	}
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].AlertUUID != "alert-1" || alerts.Alerts[0].StartTime != "2026-04-30T00:00:00Z" {
		t.Fatalf("unexpected timeline alerts: %#v", alerts.Alerts)
	}

	page := 1
	pageSize := 10
	details, err := client.DescribeAlertTimelineWithOptions(context.Background(), "node-1", "alert-1", DescribeAlertTimelineOptions{
		Order: AlertTimelineOrderAsc, Page: &page, PageSize: &pageSize,
	})
	if err != nil {
		t.Fatalf("describe alert timeline failed: %v", err)
	}
	if details.AlertUUID != "alert-1" || details.AlertStatus != "Critical" || details.NodeGroup != "Training" || details.ComputeZone != "East" || !details.IsBackendComponent || details.Page != 1 || details.PageSize != 10 || details.Total != 1 || len(details.Timeline) != 1 || details.Timeline[0].EventType != "triggered" {
		t.Fatalf("unexpected details: %#v", details)
	}
	if details.Timeline[0].ExtraInfo["gpu"] != "0" || len(details.Timeline[0].Incidents) != 1 || len(details.Timeline[0].Actions) != 1 || details.Timeline[0].Actions[0].Code != "DRAIN" {
		t.Fatalf("unexpected timeline action details: %#v", details.Timeline[0])
	}
}

// Verifies alert timeline filter options support both simple and object values.
func TestGetAlertTimelineFilterOptions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/filter_options" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("active"); got != "true" {
			t.Fatalf("unexpected active value: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"filters":{"fields":[{"name":"gpuTypes","options":["H100"]},{"name":"nodeGroups","options":[{"id":"ng-1","value":"Training"}]}]},"sorting":{"fields":["alert","hostname"],"orders":["asc","desc"],"defaults":{"field":"alert","order":"desc"}}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}
	options, err := client.GetAlertTimelineFilterOptions(context.Background(), true)
	if err != nil {
		t.Fatalf("get filter options failed: %v", err)
	}
	if len(options.Filters.Fields) != 2 || options.Filters.Fields[0].Options[0].Value != "H100" {
		t.Fatalf("unexpected simple options: %#v", options.Filters.Fields)
	}
	nodeGroup := options.Filters.Fields[1].Options[0]
	if nodeGroup.ID != "ng-1" || nodeGroup.Value != "Training" {
		t.Fatalf("unexpected object option: %#v", nodeGroup)
	}
	if options.Sorting.Defaults.Field != "alert" || len(options.RawJSON) == 0 {
		t.Fatalf("unexpected sorting or raw JSON: %#v", options)
	}
}

// Verifies alert state enum validation
func TestAlertStateValidation(t *testing.T) {
	if !AlertStateTriggered.Valid() {
		t.Fatal("expected Triggered to be valid")
	}
	if AlertState("Broken").Valid() {
		t.Fatal("expected Broken to be invalid")
	}
}

// Verifies SDK validation before requests
func TestListAlertsRejectsInvalidSeverity(t *testing.T) {
	client, err := NewClient("https://fleet.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListAlerts(context.Background(), ListAlertsOptions{Severity: "Fatal"})
	if err == nil {
		t.Fatal("expected invalid severity error")
	}
	if !strings.Contains(err.Error(), "invalid alert severity") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies alert timeline options are validated before requests
func TestAlertTimelineRejectsInvalidOptions(t *testing.T) {
	client, err := NewClient("https://fleet.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.ListAlertTimelineNodes(context.Background(), ListAlertTimelineNodesOptions{SortBy: "bad"}); err == nil || !strings.Contains(err.Error(), "invalid alert timeline node sort") {
		t.Fatalf("unexpected node sort error: %v", err)
	}
	if _, err := client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{NodeUUID: "node-1", AlertStates: []AlertTimelineState{"Triggered"}}); err == nil || !strings.Contains(err.Error(), "invalid alert timeline state") {
		t.Fatalf("unexpected timeline state error: %v", err)
	}
	// The API dropped the component sort; only startTime and lastUpdate remain.
	if _, err := client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{NodeUUID: "node-1", SortBy: "component"}); err == nil || !strings.Contains(err.Error(), "invalid node alert timeline sort") {
		t.Fatalf("unexpected node alert sort error: %v", err)
	}
	page := 1
	if _, err := client.DescribeAlertTimelineWithOptions(context.Background(), "node-1", "alert-1", DescribeAlertTimelineOptions{Page: &page}); err == nil || !strings.Contains(err.Error(), "page requires page size") {
		t.Fatalf("unexpected detail pagination error: %v", err)
	}

	pageSize := 10
	negativePage := -1
	if _, err := client.DescribeAlertTimelineWithOptions(context.Background(), "node-1", "alert-1", DescribeAlertTimelineOptions{Page: &negativePage, PageSize: &pageSize}); err == nil || !strings.Contains(err.Error(), "page must be non-negative") {
		t.Fatalf("unexpected negative page error: %v", err)
	}

	for _, invalidPageSize := range []int{0, 101} {
		if _, err := client.DescribeAlertTimelineWithOptions(context.Background(), "node-1", "alert-1", DescribeAlertTimelineOptions{PageSize: &invalidPageSize}); err == nil || !strings.Contains(err.Error(), "page size must be between 1 and 100") {
			t.Fatalf("unexpected page size error for %d: %v", invalidPageSize, err)
		}
	}
}

// Verifies node alert timeline options reject requests before opening a connection
func TestListNodeAlertTimelineOptionsValidateRequiresNodeUUID(t *testing.T) {
	err := (ListNodeAlertTimelineOptions{}).Validate()
	if err == nil || !strings.Contains(err.Error(), "node UUID is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

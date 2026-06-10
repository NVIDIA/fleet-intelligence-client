package fleetintelligence

import (
	"context"
	"net/http"
	"net/http/httptest"
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
		// /v1/alerts is 1-indexed; the SDK's 0-indexed page 0 maps to API page 1.
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

	page := 0
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
	// The API's 1-indexed page 1 is normalized back to the SDK's 0-indexed page 0.
	if got.Page != 0 || got.PageSize != 50 || got.Total != 1 || len(got.Alerts) != 1 {
		t.Fatalf("unexpected page: %#v", got)
	}
	alert := got.Alerts[0]
	if alert.UUID != "alert-1" || alert.FiredAt != "2026-05-01T00:00:00Z" || alert.Severity != "Critical" {
		t.Fatalf("unexpected alert: %#v", alert)
	}
	if !strings.Contains(string(got.RawJSON), `"alerts"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
	// The preserved payload must also carry the normalized 0-indexed page.
	if !strings.Contains(string(got.RawJSON), `"page":0`) {
		t.Fatalf("raw JSON page not normalized: %q", string(got.RawJSON))
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
			if got := r.URL.Query().Get("active"); got != "true" {
				t.Fatalf("unexpected active: %q", got)
			}
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUuid":"node-1","hostname":"gpu-001","hostStatus":"Active","lastAlertTime":"2026-05-01T00:00:00Z"}],"hasMore":false,"page":0,"pageSize":50,"total":1}`))
		case "/v1/alert_timeline/nodes/node-1/alerts":
			_, _ = w.Write([]byte(`{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-1","component":"gpu","alertStatus":"Critical","lastEventTime":"2026-05-01T00:00:00Z"}],"hasMore":false,"page":0,"pageSize":50,"total":1}`))
		case "/v1/alert_timeline/nodes/node-1/alerts/alert-1":
			_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","timeline":[{"eventType":"triggered","alertStatus":"Critical","eventTimestamp":"2026-05-01T00:00:00Z","message":"GPU critical","extraInfo":{"gpu":"0"},"suggestedActions":[{"action":"Drain node","code":"DRAIN","persona":"dc_admin","type":"immediate"}]}]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	nodes, err := client.ListAlertTimelineNodes(context.Background(), ListAlertTimelineNodesOptions{Active: true})
	if err != nil {
		t.Fatalf("timeline nodes failed: %v", err)
	}
	if len(nodes.Nodes) != 1 || nodes.Nodes[0].NodeUUID != "node-1" || nodes.Nodes[0].HostStatus != "Active" {
		t.Fatalf("unexpected timeline nodes: %#v", nodes.Nodes)
	}

	alerts, err := client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{NodeUUID: "node-1"})
	if err != nil {
		t.Fatalf("node alert timeline failed: %v", err)
	}
	if len(alerts.Alerts) != 1 || alerts.Alerts[0].AlertUUID != "alert-1" {
		t.Fatalf("unexpected timeline alerts: %#v", alerts.Alerts)
	}

	details, err := client.DescribeAlertTimeline(context.Background(), "node-1", "alert-1")
	if err != nil {
		t.Fatalf("describe alert timeline failed: %v", err)
	}
	if details.AlertUUID != "alert-1" || len(details.Timeline) != 1 || details.Timeline[0].EventType != "triggered" {
		t.Fatalf("unexpected details: %#v", details)
	}
	if details.Timeline[0].ExtraInfo["gpu"] != "0" || len(details.Timeline[0].Actions) != 1 || details.Timeline[0].Actions[0].Code != "DRAIN" {
		t.Fatalf("unexpected timeline action details: %#v", details.Timeline[0])
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

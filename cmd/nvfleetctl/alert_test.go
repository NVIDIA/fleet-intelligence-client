package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

// Verifies all-page alert JSON output
func TestAlertListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"gpu","severity":"Critical","state":"Triggered","triggeredAt":"2026-05-01T00:00:00Z","extra":"kept"}],"page":1,"pageSize":1,"total":2}`))
		case "2":
			requests++
			_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-2","nodeUUID":"node-1","component":"memory","severity":"Critical","state":"Triggered","triggeredAt":"2026-05-01T00:01:00Z"}],"page":2,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

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
	if got.Pagination.Page != 0 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies table alert rows and derived has-more metadata
func TestAlertListTableAndHasMore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alerts" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		// 0-indexed --page 0 is translated to the 1-indexed API's page 1.
		if got := r.URL.Query().Get("page"); got != "1" {
			t.Fatalf("unexpected page: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1","component":"gpu","severity":"Warning","state":"Detected","detectedAt":"2026-05-01T00:00:00Z"}],"page":1,"pageSize":1,"total":2}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "list", "--page", "0"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "NODE UUID", "COMPONENT", "SEVERITY", "STATE", "FIRED-AT", "alert-1", "Warning", "Has More: true"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies fleet and node timeline table paths
func TestAlertTimelineTables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/alert_timeline/nodes":
			if got := r.URL.Query().Get("active"); got != "true" {
				t.Fatalf("unexpected active: %q", got)
			}
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUuid":"node-1","hostname":"gpu-001","hostStatus":"Active","lastAlertTime":"2026-05-01T00:00:00Z"}],"hasMore":false,"page":0,"pageSize":50,"total":1}`))
		case "/v1/alert_timeline/nodes/node-1/alerts":
			_, _ = w.Write([]byte(`{"nodeUuid":"node-1","alerts":[{"alertUuid":"alert-1","component":"gpu","alertStatus":"Resolved","lastEventTime":"2026-05-01T00:00:00Z"}],"hasMore":false,"page":0,"pageSize":50,"total":1}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "timeline", "--active"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("timeline command failed: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "NODE UUID") || !strings.Contains(got, "NODE STATUS") || !strings.Contains(got, "node-1") || !strings.Contains(got, "Active") {
		t.Fatalf("timeline node output missing fields: %q", got)
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "timeline", "--node", "node-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("node timeline command failed: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "ALERT UUID") || !strings.Contains(got, "STATUS") || strings.Contains(got, "SEVERITY") || !strings.Contains(got, "alert-1") || !strings.Contains(got, "Resolved") {
		t.Fatalf("timeline alert output missing fields: %q", got)
	}
}

// Verifies alert describe timeline output
func TestAlertDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","timeline":[{"eventType":"triggered","alertStatus":"Critical","eventTimestamp":"2026-05-01T00:00:00Z","message":"GPU critical"}]}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "describe", "alert-1", "--node", "node-1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("describe failed: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "TIMESTAMP") || !strings.Contains(got, "triggered") || !strings.Contains(got, "GPU critical") {
		t.Fatalf("describe output missing fields: %q", got)
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

// Verifies alert list uses the shared 0-indexed page rule and rejects negatives
func TestAlertListRejectsNegativePage(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "list", "--page=-1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected page error")
	}
	if !strings.Contains(err.Error(), "--page must be greater than or equal to 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

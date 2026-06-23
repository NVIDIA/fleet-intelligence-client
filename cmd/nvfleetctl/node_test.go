package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/pkg/fleetintelligence"
)

// Verifies local output flags and friendly sort aliases
func TestNodeListLocalJSONAndSortAlias(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	raw := `{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","healthStatus":"Healthy"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("sortBy"); got != "healthStatus" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(raw))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--output", "json", "--sort-by", "health"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != raw {
		t.Fatalf("raw JSON changed: got %q want %q", strings.TrimSpace(out.String()), raw)
	}
}

// Verifies table output and filter translation
func TestNodeListTableFiltersAndSortAliases(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "detail" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query["nodeUUIDs"]; !slices.Equal(got, []string{"node-1", "node-2"}) {
			t.Fatalf("unexpected nodeUUIDs: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["healthStatuses"]; !slices.Equal(got, []string{"Healthy", "Degraded"}) {
			t.Fatalf("unexpected healthStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("hostname"); got != "gpu" {
			t.Fatalf("unexpected hostname: %q", got)
		}
		if got := query["agentStatuses"]; !slices.Equal(got, []string{"Online"}) {
			t.Fatalf("unexpected agentStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["integrityChecks"]; !slices.Equal(got, []string{"Verified"}) {
			t.Fatalf("unexpected integrityChecks: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["firmwareChecks"]; !slices.Equal(got, []string{"Unknown"}) {
			t.Fatalf("unexpected firmwareChecks: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("sortBy"); got != "computezone" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		if got := query.Get("order"); got != "desc" {
			t.Fatalf("unexpected order: %q", got)
		}
		if got := query.Get("pageSize"); got != "10" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","nodeGroup":"Training","healthStatus":"Healthy","gpuType":"NVIDIA-H100","gpuCount":8,"integrityCheck":"Verified","firmwareCheck":"Unknown","agentStatus":"Online"}],"hasMore":false,"page":0,"pageSize":10,"total":1}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--node-uuids", "node-1,node-2", "--health", "Healthy,Degraded", "--hostname", "gpu", "--agent-status", "Online", "--integrity-check", "Verified", "--firmware-check", "Unknown", "--sort-by", "computeZone", "--order", "desc", "--page-size", "10"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "COMPUTE ZONE", "NODE GROUP", "HEALTH", "GPU TYPE", "INTEGRITY CHECK", "FIRMWARE CHECK", "AGENT STATUS", "node-1", "gpu-001", "East", "Training", "Verified", "Unknown", "Online", "Page: 0"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies all-page JSON output
func TestNodeListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := r.URL.Query().Get("pageSize"); got != "1" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2","hostname":"gpu-002"}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
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
	cmd.SetArgs([]string{"node", "list", "--view", "basic", "--all", "--output", "json", "--page-size", "1"})

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
	if len(got.Items) != 2 || got.Items[0]["nodeUUID"] != "node-1" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 0 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies --all defaults the page size to the max when --page-size is omitted
func TestNodeListAllDefaultsPageSize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("pageSize"); got != "100" {
			t.Fatalf("unexpected pageSize: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1"}],"hasMore":false,"page":0,"pageSize":100,"total":1}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--all", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

// Verifies node describe table output
func TestNodeDescribeTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/node-1" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeUUID":"node-1","hostname":"gpu-001","computeZone":"East","computeZoneId":"cz-1","nodeGroup":"Training","nodeGroupId":"ng-1","healthStatus":"Degraded","gpuType":"NVIDIA-H100","gpuCount":8,"agentStatus":"Online","resources":{"gpuInfo":{"product":"NVIDIA H100","gpus":[{"uuid":"GPU-1"}]}},"systemInfo":{"agentVersion":"1.2.3","cudaVersion":"12.4"},"tags":["prod","h100"]}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"FIELD", "VALUE", "UUID", "node-1", "COMPUTE ZONE", "East (cz-1)", "GPU DEVICES", "1", "CUDA", "12.4"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies basic node rows
func TestWriteNodeBasicTable(t *testing.T) {
	var out bytes.Buffer
	err := writeNodeTable(&out, string(fleetintelligence.NodeViewBasic), []fleetintelligence.Node{
		{UUID: "node-1", Hostname: "gpu-001"},
	})
	if err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"UUID", "HOSTNAME", "node-1", "gpu-001"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies local flag validation
func TestNodeListRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "health", args: []string{"node", "list", "--health", "Broken"}, want: "invalid health"},
		{name: "agent", args: []string{"node", "list", "--agent-status", "Missing"}, want: "invalid agent-status"},
		{name: "integrity", args: []string{"node", "list", "--integrity-check", "Missing"}, want: "invalid integrity-check"},
		{name: "firmware", args: []string{"node", "list", "--firmware-check", "Missing"}, want: "invalid firmware-check"},
		{name: "sort", args: []string{"node", "list", "--sort-by", "bad"}, want: "invalid sort-by"},
		{name: "order", args: []string{"node", "list", "--order", "up"}, want: "invalid order"},
		{name: "basic filter", args: []string{"node", "list", "--view", "basic", "--health", "Healthy"}, want: "basic node view is incompatible"},
		{name: "basic sort", args: []string{"node", "list", "--view", "basic", "--sort-by", "health"}, want: "basic node view is incompatible"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies shared list pagination validation
func TestListAllRejectsPage(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"node", "list", "--all", "--page", "1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --all and --page error")
	}
	if !strings.Contains(err.Error(), "--page cannot be used with --all") {
		t.Fatalf("unexpected error: %v", err)
	}
}

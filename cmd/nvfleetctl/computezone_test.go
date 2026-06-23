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

// Verifies table output and request filters
func TestComputeZoneListTableAndFilters(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/computezones" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("view"); got != "detail" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query["computeZoneIds"]; !slices.Equal(got, []string{"cz-1", "cz-2"}) {
			t.Fatalf("unexpected computeZoneIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("page"); got != "2" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := query.Get("pageSize"); got != "50" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter","geoLocation":{"city":"Santa Clara","country":"US"},"nodesCount":7}],"hasMore":false,"page":2,"pageSize":50,"total":1}`))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"computezone", "list", "--zone-ids", "cz-1,cz-2", "--page", "2", "--page-size", "50"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "NAME", "TYPE", "GEOLOCATION", "NODE COUNT", "cz-1", "East", "datacenter", "Santa Clara, US", "Page: 2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies all-page JSON output
func TestComputeZoneListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/computezones" {
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
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-2","name":"West"}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
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
	cmd.SetArgs([]string{"computezone", "list", "--view", "basic", "--all", "--output", "json", "--page-size", "1"})

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
	if len(got.Items) != 2 || got.Items[0]["id"] != "cz-1" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 0 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies basic compute zone rows
func TestWriteComputeZoneBasicTable(t *testing.T) {
	var out bytes.Buffer
	err := writeComputeZoneTable(&out, string(fleetintelligence.ComputeZoneViewBasic), []fleetintelligence.ComputeZone{
		{ID: "cz-1", Name: "East"},
	})
	if err != nil {
		t.Fatalf("write table failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{"ID", "NAME", "cz-1", "East"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
}

// Verifies local flag validation
func TestComputeZoneListRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "view", args: []string{"computezone", "list", "--view", "wide"}, want: "invalid view"},
		{name: "zone ids", args: []string{"computezone", "list", "--zone-ids", "cz-1,,cz-2"}, want: "empty values are not allowed"},
		{name: "page size", args: []string{"computezone", "list", "--page-size", "0"}, want: "--page-size must be between"},
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

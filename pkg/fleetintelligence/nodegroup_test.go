// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetintelligence

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

// Verifies detail list request construction and decoding
func TestListNodeGroupsDetailSendsAuthAndParams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodegroups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}

		query := r.URL.Query()
		if got := query.Get("view"); got != "detail" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query["nodeGroupIds"]; !slices.Equal(got, []string{"ng-1", "ng-2"}) {
			t.Fatalf("unexpected nodeGroupIds: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["healthStatuses"]; !slices.Equal(got, []string{"Healthy", "Degraded"}) {
			t.Fatalf("unexpected healthStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["gpuTypes"]; !slices.Equal(got, []string{"NVIDIA-H100", "NVIDIA-A100"}) {
			t.Fatalf("unexpected gpuTypes: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query.Get("sortBy"); got != "health" {
			t.Fatalf("unexpected sortBy: %q", got)
		}
		if got := query.Get("order"); got != "asc" {
			t.Fatalf("unexpected order: %q", got)
		}
		if got := query.Get("page"); got != "2" {
			t.Fatalf("unexpected page: %q", got)
		}
		if got := query.Get("pageSize"); got != "50" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeGroups":[{"id":"ng-1","name":"Training","computeZoneId":"cz-1","computeZoneName":"East","healthState":"Healthy","healthPercentage":95.5,"nodesCount":8}],"hasMore":true,"page":2,"pageSize":50,"total":99}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	page := 2
	pageSize := 50
	got, err := client.ListNodeGroups(context.Background(), ListNodeGroupsOptions{
		View:           NodeGroupViewDetail,
		NodeGroupIDs:   []string{"ng-1", "ng-2"},
		HealthStatuses: []NodeGroupHealthStatus{NodeGroupHealthHealthy, NodeGroupHealthDegraded},
		GPUTypes:       []string{"NVIDIA-H100", "NVIDIA-A100"},
		SortBy:         NodeGroupSortByHealth,
		Order:          NodeGroupOrderAsc,
		Page:           &page,
		PageSize:       &pageSize,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !got.HasMore || got.Page != 2 || got.PageSize != 50 || got.Total != 99 {
		t.Fatalf("unexpected page metadata: %#v", got)
	}
	if len(got.NodeGroups) != 1 {
		t.Fatalf("unexpected group count: %d", len(got.NodeGroups))
	}
	group := got.NodeGroups[0]
	if group.ID != "ng-1" || group.Name != "Training" || group.ComputeZoneName != "East" || group.Health != "Healthy" {
		t.Fatalf("unexpected group: %#v", group)
	}
	if group.HealthPercentage == nil || *group.HealthPercentage != 95.5 {
		t.Fatalf("unexpected health percentage: %#v", group.HealthPercentage)
	}
	if group.NodeCount == nil || *group.NodeCount != 8 {
		t.Fatalf("unexpected node count: %#v", group.NodeCount)
	}
	if !strings.Contains(string(got.RawJSON), `"nodeGroups"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies basic view decoding
func TestListNodeGroupsBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := query["healthStatuses"]; len(got) != 0 {
			t.Fatalf("basic view sent healthStatuses: %#v raw query %q", got, r.URL.RawQuery)
		}
		if got := query["gpuTypes"]; len(got) != 0 {
			t.Fatalf("basic view sent gpuTypes: %#v raw query %q", got, r.URL.RawQuery)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodeGroups":[{"id":"ng-1","name":"Training","computeZoneId":"cz-1","computeZoneName":"East"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.ListNodeGroups(context.Background(), ListNodeGroupsOptions{View: NodeGroupViewBasic})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(got.NodeGroups) != 1 || got.NodeGroups[0].ID != "ng-1" || got.NodeGroups[0].ComputeZoneName != "East" {
		t.Fatalf("unexpected groups: %#v", got.NodeGroups)
	}
	if got.NodeGroups[0].NodeCount != nil {
		t.Fatalf("basic view should not set node count: %#v", got.NodeGroups[0].NodeCount)
	}
}

// Verifies API errors are structured
func TestListNodeGroupsReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid filter parameters","details":"bad node group id"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListNodeGroups(context.Background(), ListNodeGroupsOptions{})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "400 Bad Request") || !strings.Contains(err.Error(), "bad node group id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies local option validation
func TestListNodeGroupsRejectsInvalidOptions(t *testing.T) {
	client, err := NewClient("https://fleet.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	tests := []struct {
		name string
		opts ListNodeGroupsOptions
		want string
	}{
		{name: "view", opts: ListNodeGroupsOptions{View: "wide"}, want: "invalid node group view"},
		{name: "health", opts: ListNodeGroupsOptions{HealthStatuses: []NodeGroupHealthStatus{"Broken"}}, want: "invalid node group health"},
		{name: "sort", opts: ListNodeGroupsOptions{SortBy: "name"}, want: "invalid node group sort"},
		{name: "order", opts: ListNodeGroupsOptions{Order: "up"}, want: "invalid node group order"},
		{name: "basic health", opts: ListNodeGroupsOptions{View: NodeGroupViewBasic, HealthStatuses: []NodeGroupHealthStatus{NodeGroupHealthHealthy}}, want: "basic node group view is incompatible"},
		{name: "basic GPU type", opts: ListNodeGroupsOptions{View: NodeGroupViewBasic, GPUTypes: []string{"NVIDIA-H100"}}, want: "basic node group view is incompatible"},
		{name: "basic health sort", opts: ListNodeGroupsOptions{View: NodeGroupViewBasic, SortBy: NodeGroupSortByHealth}, want: "basic node group view is incompatible with sort"},
		{name: "basic nodes sort", opts: ListNodeGroupsOptions{View: NodeGroupViewBasic, SortBy: NodeGroupSortByNodes}, want: "basic node group view is incompatible with sort"},
		{name: "basic order", opts: ListNodeGroupsOptions{View: NodeGroupViewBasic, Order: NodeGroupOrderAsc}, want: "basic node group view is incompatible with sort order"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ListNodeGroups(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies sort field validation tracks the generated API enum
func TestNodeGroupSortByValid(t *testing.T) {
	valid := []NodeGroupSortBy{NodeGroupSortByHealth, NodeGroupSortByNodes}
	for _, sortBy := range valid {
		if !sortBy.Valid() {
			t.Errorf("expected %q to be a valid node group sort field", sortBy)
		}
	}
	for _, sortBy := range []NodeGroupSortBy{"", "name", "gpuUtil", "gpuutil"} {
		if sortBy.Valid() {
			t.Errorf("expected %q to be an invalid node group sort field", sortBy)
		}
	}
}

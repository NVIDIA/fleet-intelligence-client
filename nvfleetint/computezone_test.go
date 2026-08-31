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

// Verifies detail list request construction and decoding
func TestListComputeZonesDetailSendsAuthAndParams(t *testing.T) {
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
		if got := query.Get("includeMetrics"); got != "false" {
			t.Fatalf("unexpected includeMetrics: %q", got)
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
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East","type":"datacenter","location":{"region":"us-east-1"},"nodesCount":7}],"hasMore":true,"page":2,"pageSize":50,"total":99}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	page := 2
	pageSize := 50
	includeMetrics := false
	got, err := client.ListComputeZones(context.Background(), ListComputeZonesOptions{
		View:           ComputeZoneViewDetail,
		IncludeMetrics: &includeMetrics,
		ZoneIDs:        []string{"cz-1", "cz-2"},
		Page:           &page,
		PageSize:       &pageSize,
	})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if !got.HasMore || got.Page != 2 || got.PageSize != 50 || got.Total != 99 {
		t.Fatalf("unexpected page metadata: %#v", got)
	}
	if len(got.ComputeZones) != 1 {
		t.Fatalf("unexpected zone count: %d", len(got.ComputeZones))
	}
	zone := got.ComputeZones[0]
	if zone.ID != "cz-1" || zone.Name != "East" || zone.Type != "datacenter" {
		t.Fatalf("unexpected zone: %#v", zone)
	}
	if zone.NodeCount == nil || *zone.NodeCount != 7 {
		t.Fatalf("unexpected node count: %#v", zone.NodeCount)
	}
	if zone.Location == nil || zone.Location.Region != "us-east-1" {
		t.Fatalf("unexpected location: %#v", zone.Location)
	}
	if !strings.Contains(string(got.RawJSON), `"computezones"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies basic view decoding
func TestListComputeZonesBasic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"computezones":[{"id":"cz-1","name":"East"}],"hasMore":false,"page":0,"pageSize":20,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.ListComputeZones(context.Background(), ListComputeZonesOptions{View: ComputeZoneViewBasic})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(got.ComputeZones) != 1 || got.ComputeZones[0].ID != "cz-1" || got.ComputeZones[0].Name != "East" {
		t.Fatalf("unexpected zones: %#v", got.ComputeZones)
	}
	if got.ComputeZones[0].NodeCount != nil {
		t.Fatalf("basic view should not set node count: %#v", got.ComputeZones[0].NodeCount)
	}
}

// Verifies API errors are structured
func TestListComputeZonesReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Invalid filter parameters","details":"bad zone id"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListComputeZones(context.Background(), ListComputeZonesOptions{})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "400 Bad Request") || !strings.Contains(err.Error(), "bad zone id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies local option validation
func TestListComputeZonesRejectsInvalidOptions(t *testing.T) {
	client, err := NewClient("https://fleet.example.com", "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	includeMetrics := false
	tests := []struct {
		name string
		opts ListComputeZonesOptions
		want string
	}{
		{name: "view", opts: ListComputeZonesOptions{View: "wide"}, want: "invalid compute zone view"},
		{name: "basic include metrics", opts: ListComputeZonesOptions{View: ComputeZoneViewBasic, IncludeMetrics: &includeMetrics}, want: "basic compute zone view is incompatible with include metrics"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ListComputeZones(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("expected invalid options error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

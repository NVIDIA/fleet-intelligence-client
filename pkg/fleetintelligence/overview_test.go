// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetintelligence

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verifies overview request construction and decoding
func TestGetOverviewSendsAuthAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/overview" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.URL.Query().Get("includeMetrics"); got != "false" {
			t.Fatalf("unexpected includeMetrics: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodesCount":10,"healthNodeCount":7,"degradedNodeCount":2,"unhealthyNodeCount":1,"unknownNodeCount":0,"healthPercentage":70,"nodeGroupCount":3,"computeZoneCount":2,"gpusCount":80,"cpuCoresCount":960,"metrics":[{"name":"gpu_utilization","description":"Average GPU utilization","unit":"%","value":42.5,"aggregation":"average","lastUpdated":"2026-07-14T00:00:00Z"}]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.GetOverview(context.Background(), OverviewOptions{IncludeMetrics: boolPointer(false)})
	if err != nil {
		t.Fatalf("overview failed: %v", err)
	}
	if got.NodesCount == nil || *got.NodesCount != 10 {
		t.Fatalf("unexpected nodes count: %#v", got.NodesCount)
	}
	if got.HealthPercentage == nil || *got.HealthPercentage != 70 {
		t.Fatalf("unexpected health percentage: %#v", got.HealthPercentage)
	}
	if len(got.Metrics) != 1 || got.Metrics[0].Name != "gpu_utilization" || got.Metrics[0].Aggregation != "average" {
		t.Fatalf("unexpected metrics: %#v", got.Metrics)
	}
	if got.Metrics[0].Value == nil || *got.Metrics[0].Value != 42.5 {
		t.Fatalf("unexpected metric value: %#v", got.Metrics[0].Value)
	}
	if !strings.Contains(string(got.RawJSON), `"nodesCount"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies the includeMetrics param is omitted when unset
func TestGetOverviewOmitsIncludeMetricsWhenUnset(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Has("includeMetrics") {
			t.Fatalf("expected includeMetrics to be omitted, got %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodesCount":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.GetOverview(context.Background(), OverviewOptions{}); err != nil {
		t.Fatalf("overview failed: %v", err)
	}
}

// Verifies overview API errors are structured
func TestGetOverviewReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"boom","details":"backend unavailable"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.GetOverview(context.Background(), OverviewOptions{})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "backend unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

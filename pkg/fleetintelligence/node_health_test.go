// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package fleetintelligence

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verifies health history request construction and decoding
func TestNodeHealthHistorySendsAuthAndDecodes(t *testing.T) {
	const nodeUUID = "1e9c0d2a-0000-4a1b-9c3d-000000000001"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if want := "/v1/nodes/" + nodeUUID + "/health_history"; r.URL.Path != want {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		if got := r.URL.Query().Get("startTime"); got != "2026-04-07T00:00:00Z" {
			t.Fatalf("unexpected startTime: %q", got)
		}
		if got := r.URL.Query().Get("endTime"); got != "2026-04-14T00:00:00Z" {
			t.Fatalf("unexpected endTime: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"enrolledAt":"2026-01-01T00:00:00Z","healthSummary":{"healthyPercentage":99.5,"degradedPercentage":0.5,"unhealthyPercentage":0,"healthyDurationSeconds":600000,"degradedDurationSeconds":3000,"unhealthyDurationSeconds":0},"machineStatus":[{"status":"Healthy","startTime":"2026-04-07T00:00:00Z","endTime":"2026-04-13T00:00:00Z"},{"status":"Degraded","startTime":"2026-04-13T00:00:00Z","endTime":"2026-04-14T00:00:00Z"}]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	got, err := client.NodeHealthHistory(context.Background(), nodeUUID, NodeHealthHistoryOptions{
		StartTime: "2026-04-07T00:00:00Z",
		EndTime:   "2026-04-14T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("health history failed: %v", err)
	}
	if got.EnrolledAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("unexpected enrolledAt: %q", got.EnrolledAt)
	}
	if got.HealthSummary == nil {
		t.Fatal("expected health summary")
	}
	if got.HealthSummary.HealthyPercentage == nil || *got.HealthSummary.HealthyPercentage != 99.5 {
		t.Fatalf("unexpected healthy percentage: %#v", got.HealthSummary.HealthyPercentage)
	}
	if got.HealthSummary.DegradedDurationSeconds == nil || *got.HealthSummary.DegradedDurationSeconds != 3000 {
		t.Fatalf("unexpected degraded duration: %#v", got.HealthSummary.DegradedDurationSeconds)
	}
	if len(got.Segments) != 2 {
		t.Fatalf("unexpected segments: %#v", got.Segments)
	}
	if got.Segments[0].Status != "Healthy" || got.Segments[0].StartTime != "2026-04-07T00:00:00Z" {
		t.Fatalf("unexpected first segment: %#v", got.Segments[0])
	}
	if got.Segments[1].Status != "Degraded" {
		t.Fatalf("unexpected second segment: %#v", got.Segments[1])
	}
	if !strings.Contains(string(got.RawJSON), `"machineStatus"`) {
		t.Fatalf("raw JSON not preserved: %q", string(got.RawJSON))
	}
}

// Verifies missing node UUID and time window are rejected before any request
func TestNodeHealthHistoryValidatesInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid input")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	tests := []struct {
		name string
		node string
		opts NodeHealthHistoryOptions
		want string
	}{
		{
			name: "empty node UUID",
			node: "",
			opts: NodeHealthHistoryOptions{StartTime: "2026-04-07T00:00:00Z", EndTime: "2026-04-14T00:00:00Z"},
			want: "node UUID is required",
		},
		{
			name: "missing start time",
			node: "node-1",
			opts: NodeHealthHistoryOptions{StartTime: "", EndTime: "2026-04-14T00:00:00Z"},
			want: "start and end times are required",
		},
		{
			name: "missing end time",
			node: "node-1",
			opts: NodeHealthHistoryOptions{StartTime: "2026-04-07T00:00:00Z", EndTime: ""},
			want: "start and end times are required",
		},
		{
			name: "malformed start time",
			node: "node-1",
			opts: NodeHealthHistoryOptions{StartTime: "yesterday", EndTime: "2026-04-14T00:00:00Z"},
			want: "node health start time must be RFC3339",
		},
		{
			name: "malformed end time",
			node: "node-1",
			opts: NodeHealthHistoryOptions{StartTime: "2026-04-07T00:00:00Z", EndTime: "tomorrow"},
			want: "node health end time must be RFC3339",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.NodeHealthHistory(context.Background(), tt.node, tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies health history API errors are structured
func TestNodeHealthHistoryReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found","details":"node not found"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.NodeHealthHistory(context.Background(), "node-1", NodeHealthHistoryOptions{
		StartTime: "2026-04-07T00:00:00Z",
		EndTime:   "2026-04-14T00:00:00Z",
	})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "node not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

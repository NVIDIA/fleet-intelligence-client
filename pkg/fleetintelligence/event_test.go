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

// Verifies event list request construction and decoding in absolute mode
func TestListEventsAbsoluteSendsParamsAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("unexpected auth header: %q", got)
		}
		query := r.URL.Query()
		if got := query.Get("timeMode"); got != "absolute" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("startTime"); got != "2026-05-01T00:00:00Z" {
			t.Fatalf("unexpected startTime: %q", got)
		}
		if got := query.Get("endTime"); got != "2026-05-08T00:00:00Z" {
			t.Fatalf("unexpected endTime: %q", got)
		}
		if got := query.Get("nodeUUID"); got != "node-1" {
			t.Fatalf("unexpected nodeUUID: %q", got)
		}
		if got := query.Get("component"); got != "GPU" {
			t.Fatalf("unexpected component: %q", got)
		}
		if query.Has("window") {
			t.Fatalf("did not expect window in absolute mode: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[{"eventId":"e1","nodeUUID":"node-1","component":"GPU","type":"error","name":"xid","message":"boom","timestamp":"2026-05-01T00:00:00Z","suggestedActions":[{"action":"reboot","code":"R1"}]}],"hasMore":false,"page":0,"pageSize":50,"total":1}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	page, err := client.ListEvents(context.Background(), EventListOptions{
		NodeUUID:  "node-1",
		Component: "GPU",
		StartTime: "2026-05-01T00:00:00Z",
		EndTime:   "2026-05-08T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("list events failed: %v", err)
	}
	if len(page.Events) != 1 {
		t.Fatalf("unexpected events: %#v", page.Events)
	}
	event := page.Events[0]
	if event.EventID != "e1" || event.NodeUUID != "node-1" || event.Component != "GPU" || event.Type != "error" || event.Name != "xid" || event.Message != "boom" {
		t.Fatalf("unexpected event fields: %#v", event)
	}
	if len(event.SuggestedActions) != 1 || event.SuggestedActions[0].Action != "reboot" || event.SuggestedActions[0].Code != "R1" {
		t.Fatalf("unexpected suggested actions: %#v", event.SuggestedActions)
	}
	if page.Total != 1 || page.PageSize != 50 || page.HasMore {
		t.Fatalf("unexpected pagination: %#v", page)
	}
	if !strings.Contains(string(page.RawJSON), `"events"`) {
		t.Fatalf("raw JSON not preserved: %q", string(page.RawJSON))
	}
}

// Verifies a relative window sets relative mode and omits start/end
func TestListEventsRelativeWindow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "24h" {
			t.Fatalf("unexpected window: %q", got)
		}
		if query.Has("startTime") || query.Has("endTime") {
			t.Fatalf("did not expect start/end in relative mode: %q", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"events":[],"hasMore":false,"page":0,"pageSize":50,"total":0}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	if _, err := client.ListEvents(context.Background(), EventListOptions{Window: "24h"}); err != nil {
		t.Fatalf("list events failed: %v", err)
	}
}

// Verifies conflicting or malformed time options are rejected before any request
func TestListEventsTimeValidation(t *testing.T) {
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
		opts EventListOptions
		want string
	}{
		{"no time range", EventListOptions{}, "a time range is required"},
		{"window with start", EventListOptions{Window: "24h", StartTime: "2026-05-01T00:00:00Z"}, "window cannot be combined"},
		{"start without end", EventListOptions{StartTime: "2026-05-01T00:00:00Z"}, "must be provided together"},
		{"malformed window", EventListOptions{Window: "3 days"}, "invalid window"},
		{"malformed start", EventListOptions{StartTime: "yesterday", EndTime: "2026-05-08T00:00:00Z"}, "event start time must be RFC3339"},
		{"malformed end", EventListOptions{StartTime: "2026-05-01T00:00:00Z", EndTime: "tomorrow"}, "event end time must be RFC3339"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := client.ListEvents(context.Background(), tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies event list API errors are structured
func TestListEventsReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad","details":"invalid range"}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	_, err = client.ListEvents(context.Background(), EventListOptions{Window: "24h"})
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies event buckets request construction and decoding
func TestGetEventBucketsSendsParamsAndDecodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/events/buckets" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("timeMode"); got != "relative" {
			t.Fatalf("unexpected timeMode: %q", got)
		}
		if got := query.Get("window"); got != "168h" {
			t.Fatalf("unexpected window: %q", got)
		}
		if got := query.Get("maxBuckets"); got != "50" {
			t.Fatalf("unexpected maxBuckets: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"bucketInterval":"1h","buckets":[{"startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T01:00:00Z","firstEventTime":"2026-05-01T00:12:00Z","count":3}]}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	maxBuckets := 50
	buckets, err := client.GetEventBuckets(context.Background(), EventBucketsOptions{Window: "168h", MaxBuckets: &maxBuckets})
	if err != nil {
		t.Fatalf("get event buckets failed: %v", err)
	}
	if buckets.BucketInterval != "1h" {
		t.Fatalf("unexpected interval: %q", buckets.BucketInterval)
	}
	if len(buckets.Buckets) != 1 {
		t.Fatalf("unexpected buckets: %#v", buckets.Buckets)
	}
	if buckets.Buckets[0].Count == nil || *buckets.Buckets[0].Count != 3 {
		t.Fatalf("unexpected count: %#v", buckets.Buckets[0].Count)
	}
	if !strings.Contains(string(buckets.RawJSON), `"bucketInterval"`) {
		t.Fatalf("raw JSON not preserved: %q", string(buckets.RawJSON))
	}
}

// Verifies out-of-range max buckets is rejected before any request
func TestGetEventBucketsValidatesMaxBuckets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid input")
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatalf("new client failed: %v", err)
	}

	for _, value := range []int{0, MaxEventBuckets + 1} {
		maxBuckets := value
		if _, err := client.GetEventBuckets(context.Background(), EventBucketsOptions{Window: "24h", MaxBuckets: &maxBuckets}); err == nil {
			t.Fatalf("expected error for max buckets %d", value)
		}
	}
}

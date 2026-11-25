// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestNodeHealthHistoryMalformedResponse(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{!"))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.NodeHealthHistory(context.Background(), "node-1", NodeHealthHistoryOptions{
		StartTime: "2026-04-07T00:00:00Z", EndTime: "2026-04-14T00:00:00Z",
	})
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, NodeHealthHistory{}) || calls.Load() != 1 {
		t.Fatalf("got history %#v, error %v, and %d calls; want zero history and JSON syntax error", got, err, calls.Load())
	}
}

func TestNodeHealthHistoryWithoutSummary(t *testing.T) {
	const payload = `{"enrolledAt":"2026-01-01T00:00:00Z","machineStatus":[{"status":"Degraded","startTime":"2026-04-07T00:00:00Z","endTime":"2026-04-14T00:00:00Z"}]}`
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.NodeHealthHistory(context.Background(), "node-1", NodeHealthHistoryOptions{
		StartTime: "2026-04-07T00:00:00Z", EndTime: "2026-04-14T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSegments := []NodeHealthSegment{{Status: "Degraded", StartTime: "2026-04-07T00:00:00Z", EndTime: "2026-04-14T00:00:00Z"}}
	if got.HealthSummary != nil || got.EnrolledAt != "2026-01-01T00:00:00Z" || !reflect.DeepEqual(got.Segments, wantSegments) || string(got.RawJSON) != payload || calls.Load() != 1 {
		t.Fatalf("unexpected history %#v after %d calls", got, calls.Load())
	}
}

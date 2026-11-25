// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestDescribeAlertTimelineConvenience(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/alert_timeline/nodes/node-1/alerts/alert-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		for _, key := range []string{"page", "pageSize", "order"} {
			if _, ok := r.URL.Query()[key]; ok {
				t.Errorf("unexpected query parameter %s: %v", key, r.URL.Query())
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alertUuid":"alert-1","nodeUuid":"node-1","timeline":[]}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.DescribeAlertTimeline(context.Background(), "node-1", "alert-1")
	if err != nil || got.AlertUUID != "alert-1" {
		t.Fatalf("got detail %#v and error %v; want alert-1", got, err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("server received %d requests; want 1", got)
	}
}

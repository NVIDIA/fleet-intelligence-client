// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestEventBucketsAbsoluteRangeAndInvalidRange(t *testing.T) {
	const start = "2026-05-01T00:00:00Z"
	const end = "2026-05-01T01:00:00Z"
	const payload = `{"bucketInterval":"1h","buckets":[{"startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T01:00:00Z","count":3}]}`
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/v1/events/buckets" || q.Get("timeMode") != "absolute" || q.Get("startTime") != start || q.Get("endTime") != end || q.Get("nodeUUID") != "node-1" || q.Get("component") != "gpu" || q.Has("window") {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.GetEventBuckets(context.Background(), EventBucketsOptions{StartTime: start, EndTime: end, NodeUUID: "  node-1  ", Component: "  gpu  "})
	if err != nil {
		t.Fatal(err)
	}
	if got.BucketInterval != "1h" || string(got.RawJSON) != payload || len(got.Buckets) != 1 {
		t.Fatalf("unexpected buckets: %#v", got)
	}
	bucket := got.Buckets[0]
	if bucket.StartTime != start || bucket.EndTime != end || bucket.Count == nil || *bucket.Count != 3 {
		t.Fatalf("unexpected bucket: %#v", bucket)
	}
	if requests.Load() != 1 {
		t.Fatalf("got %d requests; want 1", requests.Load())
	}
	got, err = client.GetEventBuckets(context.Background(), EventBucketsOptions{StartTime: "invalid", EndTime: end})
	if err == nil || err.Error() != "event start time must be RFC3339" || !reflect.DeepEqual(got, EventBuckets{}) {
		t.Fatalf("got buckets %#v and error %v; want zero result and invalid start time error", got, err)
	}
	if requests.Load() != 1 {
		t.Errorf("invalid range made a request; got %d requests", requests.Load())
	}
}

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

func TestEventResponseBoundaries(t *testing.T) {
	endpoints := []struct {
		name string
		path string
		zero any
		call func(context.Context, *Client) (any, error)
	}{
		{"list", "/v1/events/page", EventsPage{}, func(ctx context.Context, c *Client) (any, error) {
			return c.ListEvents(ctx, EventListOptions{Window: "24h"})
		}},
		{"buckets", "/v1/events/buckets", EventBuckets{}, func(ctx context.Context, c *Client) (any, error) {
			return c.GetEventBuckets(ctx, EventBucketsOptions{Window: "24h"})
		}},
		{"filter options", "/v1/events/options", EventFilterOptions{}, func(ctx context.Context, c *Client) (any, error) {
			return c.GetEventFilterOptions(ctx)
		}},
	}
	for _, endpoint := range endpoints {
		for _, scenario := range []string{"canceled", "malformed", "forbidden"} {
			if scenario == "forbidden" && endpoint.name == "list" {
				continue
			}
			t.Run(endpoint.name+"/"+scenario, func(t *testing.T) {
				var requests atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Method != http.MethodGet || r.URL.Path != endpoint.path {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
					}
					if scenario == "forbidden" {
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusForbidden)
						_, _ = w.Write([]byte(`{"error":"denied","details":"event access forbidden"}`))
					} else {
						w.Header().Set("Content-Type", "text/plain")
						_, _ = w.Write([]byte(`{"broken":`))
					}
				}))
				defer server.Close()
				client, err := NewClient(server.URL, "test-key")
				if err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if scenario == "canceled" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithCancel(ctx)
					cancel()
				}
				got, err := endpoint.call(ctx, client)
				if !reflect.DeepEqual(got, endpoint.zero) {
					t.Errorf("got result %#v; want zero result %#v", got, endpoint.zero)
				}
				switch scenario {
				case "canceled":
					if !errors.Is(err, context.Canceled) || requests.Load() != 0 {
						t.Fatalf("got error %v and %d requests; want cancellation and no request", err, requests.Load())
					}
				case "malformed":
					var syntaxErr *json.SyntaxError
					if !errors.As(err, &syntaxErr) || requests.Load() != 1 {
						t.Fatalf("got error %v and %d requests; want JSON syntax error and one request", err, requests.Load())
					}
				case "forbidden":
					var apiErr *APIError
					if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Message != "denied" || apiErr.Details != "event access forbidden" || requests.Load() != 1 {
						t.Fatalf("got error %v and %d requests; want 403 with server details and one request", err, requests.Load())
					}
				}
			})
		}
	}
}

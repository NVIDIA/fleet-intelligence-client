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
	"strings"
	"sync/atomic"
	"testing"
)

func TestAlertResponseErrors(t *testing.T) {
	for _, endpoint := range alertBoundaryCalls() {
		for _, response := range []struct {
			name, contentType, body string
			status                  int
		}{
			{"forbidden", "application/json", `{"message":"permission denied"}`, http.StatusForbidden},
			{"malformed", "text/plain", `{`, http.StatusOK},
		} {
			t.Run(endpoint.name+"/"+response.name, func(t *testing.T) {
				var requests atomic.Int64
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					if r.Method != http.MethodGet || r.URL.Path != endpoint.path {
						t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
					}
					w.Header().Set("Content-Type", response.contentType)
					w.WriteHeader(response.status)
					_, _ = w.Write([]byte(response.body))
				}))
				defer server.Close()
				client, err := NewClient(server.URL, "test-key")
				if err != nil {
					t.Fatal(err)
				}
				got, err := endpoint.call(context.Background(), client)
				if !reflect.DeepEqual(got, endpoint.zero) {
					t.Errorf("got result %#v; want zero value %#v", got, endpoint.zero)
				}
				if response.status == http.StatusForbidden {
					var apiErr *APIError
					if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden || apiErr.Message != response.body || !strings.Contains(apiErr.Error(), "permission denied") {
						t.Errorf("got error %v; want 403 APIError preserving the permission denied response body", err)
					}
				} else {
					var syntaxErr *json.SyntaxError
					if !errors.As(err, &syntaxErr) {
						t.Errorf("got error %v; want JSON syntax error", err)
					}
				}
				if got := requests.Load(); got != 1 {
					t.Errorf("server received %d requests; want 1", got)
				}
			})
		}
	}
}

func TestAlertCanceledContexts(t *testing.T) {
	calls := alertBoundaryCalls()
	calls = append(calls, struct {
		name string
		path string
		zero any
		call func(context.Context, *Client) (any, error)
	}{"filter options", "", AlertTimelineFilterOptions{}, func(ctx context.Context, c *Client) (any, error) {
		return c.GetAlertTimelineFilterOptions(ctx, false)
	}})
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			got, err := tc.call(ctx, client)
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, tc.zero) {
				t.Errorf("got result %#v and error %v; want zero value and context cancellation", got, err)
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("server received %d requests; want none", got)
			}
		})
	}
}

func TestAlertTimelineValidationBoundaries(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "test-key")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, identifierError string
		zero                  any
		call                  func() (any, error)
	}{
		{"node timeline missing node", "node UUID is required", NodeAlertTimelinePage{}, func() (any, error) {
			return client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{})
		}},
		{"detail missing node", "node UUID is required", AlertTimelineDetails{}, func() (any, error) {
			return client.DescribeAlertTimeline(context.Background(), "", "alert-1")
		}},
		{"detail missing alert", "alert UUID is required", AlertTimelineDetails{}, func() (any, error) {
			return client.DescribeAlertTimeline(context.Background(), "node-1", "")
		}},
		{"nodes invalid order", "", AlertTimelineNodesPage{}, func() (any, error) {
			return client.ListAlertTimelineNodes(context.Background(), ListAlertTimelineNodesOptions{Order: "sideways"})
		}},
		{"node timeline invalid order", "", NodeAlertTimelinePage{}, func() (any, error) {
			return client.ListNodeAlertTimeline(context.Background(), ListNodeAlertTimelineOptions{NodeUUID: "node-1", Order: "sideways"})
		}},
		{"detail invalid order", "", AlertTimelineDetails{}, func() (any, error) {
			return client.DescribeAlertTimelineWithOptions(context.Background(), "node-1", "alert-1", DescribeAlertTimelineOptions{Order: "sideways"})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.call()
			if !reflect.DeepEqual(got, tc.zero) {
				t.Errorf("got result %#v; want zero value %#v", got, tc.zero)
			}
			if tc.identifierError != "" {
				if err == nil || err.Error() != tc.identifierError {
					t.Errorf("got error %v; want %q", err, tc.identifierError)
				}
			} else {
				assertReportOptionError(t, err, "order", "sideways")
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("server received %d requests; want none", got)
			}
		})
	}
}

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

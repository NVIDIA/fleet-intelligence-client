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

func TestListTagsResourceScopes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		opts  TagListOptions
		field string
	}{
		{name: "node", opts: TagListOptions{Prefix: "  rack  ", NodeUUID: "  node-1  "}, field: "nodeUUID"},
		{name: "compute zone", opts: TagListOptions{Prefix: "  rack  ", ComputeZoneID: "  zone-1  "}, field: "computeZoneId"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const payload = `{"tags":["rack-blue","rack-red"]}`
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				q := r.URL.Query()
				wantID := "node-1"
				if tc.field == "computeZoneId" {
					wantID = "zone-1"
				}
				if r.Method != http.MethodGet || r.URL.Path != "/v1/tags" || q.Get("prefix") != "rack" || q.Get(tc.field) != wantID {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.String())
				}
				for _, field := range []string{"nodeUUID", "nodeGroupId", "computeZoneId"} {
					if field != tc.field && q.Has(field) {
						t.Errorf("unexpected resource filter %q in %s", field, r.URL.RawQuery)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.ListTags(context.Background(), tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Tags, []string{"rack-blue", "rack-red"}) || string(got.RawJSON) != payload || requests.Load() != 1 {
				t.Fatalf("got %#v after %d requests", got, requests.Load())
			}
		})
	}
}

func TestTagMethodsMalformedSuccessResponses(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		set         bool
	}{
		{name: "list text", contentType: "text/plain"},
		{name: "list json", contentType: "application/json"},
		{name: "set text", contentType: "text/plain", set: true},
		{name: "set json", contentType: "application/json", set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", tc.contentType)
				_, _ = w.Write([]byte(`{"tags":`))
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			if tc.set {
				got, err := client.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{Tags: []string{"rack-blue"}})
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, NodeTags{}) {
					t.Fatalf("got result %#v and error %v; want zero result and JSON syntax error", got, err)
				}
			} else {
				got, err := client.ListTags(context.Background(), TagListOptions{NodeUUID: "node-1"})
				var syntaxErr *json.SyntaxError
				if !errors.As(err, &syntaxErr) || !reflect.DeepEqual(got, TagList{}) {
					t.Fatalf("got result %#v and error %v; want zero result and JSON syntax error", got, err)
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("got %d requests; want 1", requests.Load())
			}
		})
	}
}

func TestTagMethodsCanceledContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  bool
	}{
		{name: "list"},
		{name: "set", set: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if tc.set {
				got, err := client.SetNodeTags(ctx, "node-1", SetNodeTagsOptions{Tags: []string{"rack-blue"}})
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, NodeTags{}) {
					t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
				}
			} else {
				got, err := client.ListTags(ctx, TagListOptions{NodeUUID: "node-1"})
				if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, TagList{}) {
					t.Fatalf("got result %#v and error %v; want zero result and context cancellation", got, err)
				}
			}
			if requests.Load() != 0 {
				t.Fatalf("got %d requests; want none", requests.Load())
			}
		})
	}
}

func TestTagMethodsInterruptedResponseBody(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
		zero   any
		call   func(*Client) (any, error)
	}{
		{"list", http.MethodGet, "/v1/tags", TagList{}, func(c *Client) (any, error) {
			return c.ListTags(context.Background(), TagListOptions{NodeGroupID: "group-1"})
		}},
		{"set", http.MethodPut, "/v1/nodes/node-1/tags", NodeTags{}, func(c *Client) (any, error) {
			return c.SetNodeTags(context.Background(), "node-1", SetNodeTagsOptions{Tags: []string{"rack-blue"}})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("interrupted response body")
			body := &interruptedTagBody{partial: strings.NewReader(`{"tags":["rack-blue"`), err: sentinel}
			requests := 0
			transport := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Errorf("got request %s %s; want %s %s", req.Method, req.URL.String(), tc.method, tc.path)
				}
				if tc.method == http.MethodGet {
					if req.URL.Query().Get("nodeGroupId") != "group-1" {
						t.Errorf("got query %s; want nodeGroupId=group-1", req.URL.RawQuery)
					}
				} else {
					var payload struct {
						Tags []string `json:"tags"`
					}
					if err := json.NewDecoder(req.Body).Decode(&payload); err != nil || !reflect.DeepEqual(payload.Tags, []string{"rack-blue"}) {
						t.Errorf("got request body %#v and error %v; want replacement tags", payload, err)
					}
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: body, Request: req}, nil
			})
			client, err := NewClient("https://example.com", "test-key", WithHTTPClient(&http.Client{Transport: transport}))
			if err != nil {
				t.Fatal(err)
			}
			got, err := tc.call(client)
			if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, tc.zero) {
				t.Errorf("got result %#v and error %v; want zero result and sentinel error", got, err)
			}
			if requests != 1 || !body.closed {
				t.Errorf("got %d requests and body closed %t; want one request and closed body", requests, body.closed)
			}
		})
	}
}

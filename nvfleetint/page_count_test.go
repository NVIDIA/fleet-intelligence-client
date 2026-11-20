// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPageCountListsShareFrozenScope(t *testing.T) {
	for _, scenario := range []struct {
		name, root, pageBody string
		list                 func(*Client) (int, []byte, error)
	}{
		{"events", "/v1/events", `{"events":[],"page":4,"pageSize":20,"hasMore":false,"extra":"preserved"}`,
			func(c *Client) (int, []byte, error) {
				p, err := c.ListEvents(context.Background(), EventListOptions{Window: "24h", NodeUUID: "node-1", DataSources: []DataSource{DataSourceOOB}, EventNames: []string{"XID", "Others"}, Page: ptr(4), PageSize: ptr(20)})
				return p.Total, p.RawJSON, err
			}},
		{"bursts", "/v1/xid/bursts", `{"items":[],"page":4,"pageSize":20,"hasMore":false,"extra":"preserved"}`,
			func(c *Client) (int, []byte, error) {
				p, err := c.ListXIDBursts(context.Background(), ListXIDBurstsOptions{Window: "24h", NodeUUID: "node-1", DataSource: DataSourceOOB, XIDNumbers: []int{48, 94}, Page: ptr(4), PageSize: ptr(20)})
				return p.Total, p.RawJSON, err
			}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			const expectedTotal = 81
			var queries []url.Values
			var paths []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				paths = append(paths, r.URL.Path)
				queries = append(queries, r.URL.Query())
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case scenario.root + "/page":
					_, _ = w.Write([]byte(scenario.pageBody))
				case scenario.root + "/count":
					_ = json.NewEncoder(w).Encode(CountResult{Total: expectedTotal})
				default:
					t.Errorf("legacy or unexpected request: %s", r.URL)
					w.WriteHeader(http.StatusForbidden)
				}
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "test-key")
			if err != nil {
				t.Fatal(err)
			}
			total, raw, err := scenario.list(client)
			if err != nil {
				t.Fatal(err)
			}
			if total != expectedTotal || len(paths) != 2 {
				t.Fatalf("total=%d paths=%v", total, paths)
			}
			for _, q := range queries {
				if q.Get("timeMode") != "absolute" || q.Has("window") {
					t.Fatalf("not frozen: %v", q)
				}
			}
			start, err := time.Parse(time.RFC3339Nano, queries[0].Get("startTime"))
			if err != nil {
				t.Fatal(err)
			}
			end, err := time.Parse(time.RFC3339Nano, queries[0].Get("endTime"))
			if err != nil {
				t.Fatal(err)
			}
			if end.Sub(start) != 24*time.Hour {
				t.Fatal("wrong relative window duration")
			}
			for _, q := range queries {
				delete(q, "page")
				delete(q, "pageSize")
			}
			if !reflect.DeepEqual(queries[0], queries[1]) {
				t.Fatalf("scope differs: %v vs %v", queries[0], queries[1])
			}
			var envelope map[string]any
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope["total"] != float64(expectedTotal) || envelope["extra"] != "preserved" || envelope["hasMore"] != false {
				t.Fatalf("bad merged envelope: %s", raw)
			}
		})
	}
}

func TestPageCountFailureDoesNotFallbackOrReturnPartialResults(t *testing.T) {
	for _, scenario := range []struct {
		name, body string
		status     int
	}{
		{"forbidden", `{"error":"Forbidden"}`, http.StatusForbidden},
		{"missing_total", `{}`, http.StatusOK},
		{"negative_total", `{"total":-1}`, http.StatusOK},
		{"invalid_json", `not-json`, http.StatusOK},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/events/page" {
					_, _ = w.Write([]byte(`{"events":[],"page":0,"pageSize":20,"hasMore":false}`))
					return
				}
				if r.URL.Path != "/v1/events/count" {
					t.Errorf("unexpected fallback: %s", r.URL.Path)
				}
				w.WriteHeader(scenario.status)
				_, _ = w.Write([]byte(scenario.body))
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, "test-key")
			page, err := client.ListEvents(context.Background(), EventListOptions{Window: "1h"})
			if err == nil || page.RawJSON != nil || requests != 2 {
				t.Fatalf("partial result or fallback: page=%v err=%v requests=%d", page, err, requests)
			}
		})
	}
}

func TestSkipTotalUsesOnlyPageAndPreservesHasMore(t *testing.T) {
	for _, resource := range []string{"events", "xid/bursts"} {
		t.Run(resource, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if r.URL.Path != "/v1/"+resource+"/page" {
					t.Errorf("unexpected request: %s", r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"events":[],"items":[],"page":0,"pageSize":1,"hasMore":true}`))
			}))
			defer server.Close()
			client, _ := NewClient(server.URL, "test-key")
			var info PageInfo
			if resource == "events" {
				p, err := client.ListEvents(context.Background(), EventListOptions{Window: "1h", SkipTotal: true})
				if err != nil {
					t.Fatal(err)
				}
				info = p.PageInfo()
			} else {
				p, err := client.ListXIDBursts(context.Background(), ListXIDBurstsOptions{Window: "1h", SkipTotal: true})
				if err != nil {
					t.Fatal(err)
				}
				info = p.PageInfo()
			}
			if requests != 1 || info.HasMore == nil || !*info.HasMore || strings.Contains(string(info.RawJSON), `"total"`) {
				t.Fatalf("unexpected page info: %+v", info)
			}
		})
	}
}

func ptr(value int) *int { return &value }

// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies all-page JSON output
func TestNodeListAllJSONMergesRawItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("view"); got != "basic" {
			t.Fatalf("unexpected view: %q", got)
		}
		if got := r.URL.Query().Get("pageSize"); got != "1" {
			t.Fatalf("unexpected pageSize: %q", got)
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "0":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","hostname":"gpu-001","extra":"kept"}],"hasMore":true,"page":0,"pageSize":1,"total":2}`))
		case "1":
			requests++
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2","hostname":"gpu-002"}],"hasMore":false,"page":1,"pageSize":1,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--view", "basic", "--all", "--output", "json", "--page-size", "1"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	var got struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			Page         int  `json:"page"`
			PageSize     int  `json:"pageSize"`
			Total        int  `json:"total"`
			HasMore      bool `json:"hasMore"`
			PagesFetched int  `json:"pagesFetched"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if requests != 2 {
		t.Fatalf("unexpected request count: %d", requests)
	}
	if len(got.Items) != 2 || got.Items[0]["nodeUUID"] != "node-1" || got.Items[0]["extra"] != "kept" {
		t.Fatalf("unexpected merged items: %#v", got.Items)
	}
	if got.Pagination.Page != 1 || got.Pagination.PageSize != 1 || got.Pagination.Total != 2 || got.Pagination.HasMore || got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected pagination: %#v", got.Pagination)
	}
}

// Verifies --all defaults the page size to the max when --page-size is omitted
func TestNodeListAllDefaultsPageSize(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("pageSize"); got != "100" {
			t.Fatalf("unexpected pageSize: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1"}],"hasMore":false,"page":0,"pageSize":100,"total":1}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "list", "--data-source", "inband", "--all", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
}

// Verifies --all retries only a transiently failing page instead of restarting
// pagination and duplicating already collected nodes.
func TestNodeListAllRetriesFailedPage(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var firstPageCalls atomic.Int32
	var secondPageCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "0":
			firstPageCalls.Add(1)
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1"}],"hasMore":true,"page":0,"pageSize":100,"total":2}`))
		case "1":
			if secondPageCalls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":"temporarily unavailable"}`))
				return
			}
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2"}],"hasMore":false,"page":1,"pageSize":100,"total":2}`))
		default:
			t.Fatalf("unexpected page: %q", r.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"node", "list", "--data-source", "inband", "--all", "--output", "json", "--timeout", "5s",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	var got struct {
		Items []struct {
			NodeUUID string `json:"nodeUUID"`
		} `json:"items"`
		Pagination struct {
			PagesFetched int `json:"pagesFetched"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if len(got.Items) != 2 ||
		got.Items[0].NodeUUID != "node-1" ||
		got.Items[1].NodeUUID != "node-2" ||
		got.Pagination.PagesFetched != 2 {
		t.Fatalf("unexpected merged output: %#v", got)
	}
	if firstPageCalls.Load() != 1 || secondPageCalls.Load() != 2 {
		t.Fatalf("unexpected page calls: first=%d second=%d", firstPageCalls.Load(), secondPageCalls.Load())
	}
}

func TestNodeUUIDFilterFlagsAndPagination(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		q := r.URL.Query()
		if !slices.Equal(q["agentUUIDs"], []string{"agent-A", "agent-B"}) || !slices.Equal(q["systemUUIDs"], []string{"system-A", "system-B"}) {
			t.Errorf("UUID filters lost: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		if q.Get("page") == "0" {
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-1","agentUUID":"agent-A"}],"page":0,"pageSize":1,"hasMore":true,"total":2}`))
		} else {
			_, _ = w.Write([]byte(`{"nodes":[{"nodeUUID":"node-2","agentUUID":"agent-B"}],"page":1,"pageSize":1,"hasMore":false,"total":2}`))
		}
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, scope := range [][]string{{"--view", "basic"}, {"--data-source", "inband"}} {
		var out bytes.Buffer
		cmd := newRootCmd()
		cmd.SetOut(&out)
		args := []string{"node", "list", "--agent-uuids", "agent-A,agent-B", "--system-uuids", "system-A,system-B", "--all", "--page-size", "1", "--output", "json"}
		cmd.SetArgs(append(args, scope...))
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), `"agentUUID":"agent-A"`) || !strings.Contains(out.String(), `"agentUUID":"agent-B"`) {
			t.Fatalf("missing agent identities: %s", &out)
		}
	}
	if requests != 4 {
		t.Fatalf("wanted two pages per query, got %d requests", requests)
	}
}

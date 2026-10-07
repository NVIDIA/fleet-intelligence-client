// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestNodeSearchAcrossViewsAndPages(t *testing.T) {
	for _, tc := range []struct{ view, source, identityFlag, identityParam string }{
		{"basic", "", "--hostname", "hostname"},
		{"basic", "inband", "--hostname", "hostname"},
		{"basic", "oob", "--node-name", "nodeName"},
		{"detail", "inband", "--hostname", "hostname"},
		{"detail", "oob", "--bmc-hostname", "bmcHostname"},
	} {
		for _, all := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/all=%v", tc.view, tc.source, all), func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					q := r.URL.Query()
					if r.URL.Path != "/v1/nodes" || q.Get("search") != "node-01" || q.Get(tc.identityParam) != "exact-filter" || q.Get("view") != tc.view || q.Get("dataSource") != tc.source || q.Has("nameSearch") {
						t.Errorf("unexpected filters: %s", r.URL)
					}
					if all && q.Get("page") != fmt.Sprint(calls) {
						t.Errorf("page=%s; want %d", q.Get("page"), calls)
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprintf(w, `{"nodes":[{"nodeUUID":"n%d"}],"page":%d,"pageSize":1,"total":2,"hasMore":%t}`, calls, calls, calls == 0)
					calls++
				}))
				defer server.Close()
				cmdtest.SaveConfig(t, server.URL, "test-key")
				args := []string{"node", "list", "--view", tc.view, "--search", " node-01 ", tc.identityFlag, "exact-filter", "--page-size", "1", "--output", "json"}
				if tc.source != "" {
					args = append(args, "--data-source", tc.source)
				}
				if all {
					args = append(args, "--all")
				}
				cmd := newRootCmd()
				cmd.SetOut(new(bytes.Buffer))
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				want := 1
				if all {
					want = 2
				}
				if calls != want {
					t.Fatalf("requests=%d; want %d", calls, want)
				}
			})
		}
	}
}

func TestNodeDetailSearchRequiresSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmd := newRootCmd()
	cmd.SetOut(new(bytes.Buffer))
	cmd.SetArgs([]string{"node", "list", "--search", "node-01"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "--data-source is required when using --search") {
		t.Fatalf("expected explicit source requirement, got %v", err)
	}
}

// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNodeSearchRequest(t *testing.T) {
	for _, view := range []NodeView{NodeViewBasic, NodeViewDetail} {
		for _, source := range []DataSource{"", DataSourceInband, DataSourceOOB} {
			for _, search := range []string{" bmc-01 ", "", "   "} {
				t.Run(string(view)+"/"+string(source)+"/"+search, func(t *testing.T) {
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						q := r.URL.Query()
						want := strings.TrimSpace(search)
						if r.URL.Path != "/v1/nodes" || q.Get("search") != want || q.Has("search") != (want != "") {
							t.Errorf("unexpected search: %s", r.URL)
						}
						if q.Get("view") != string(view) || q.Get("dataSource") != string(source) || q.Has("nameSearch") {
							t.Errorf("unexpected scope: %s", r.URL)
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write([]byte(`{"nodes":[],"total":0}`))
					}))
					defer server.Close()
					client, err := NewClient(server.URL, "test-key")
					if err != nil {
						t.Fatal(err)
					}
					if _, err := client.ListNodes(context.Background(), ListNodesOptions{View: view, DataSource: source, Search: search}); err != nil {
						t.Fatal(err)
					}
				})
			}
		}
	}
}

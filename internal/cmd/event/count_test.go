// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package event

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestEventAllUsesTwoPagesAndOneCount(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	pages, counts := 0, 0
	var start, end string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if start == "" {
			start, end = q.Get("startTime"), q.Get("endTime")
		}
		if start == "" || end == "" || q.Get("startTime") != start || q.Get("endTime") != end || q.Has("window") {
			t.Errorf("time scope changed: %s", r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/events/page":
			if q.Get("page") != fmt.Sprint(pages) {
				t.Errorf("page=%s; want %d", q.Get("page"), pages)
			}
			_, _ = fmt.Fprintf(w, `{"events":[{"eventId":"event-1"}],"page":%d,"pageSize":1,"hasMore":%t}`, pages, pages == 0)
			pages++
		case "/v1/events/count":
			counts++
			_, _ = w.Write([]byte(`{"total":2}`))
		default:
			t.Errorf("unexpected legacy request: %s", r.URL)
		}
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"event", "list", "--window", "24h", "--all", "--page-size", "1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if pages != 2 || counts != 1 {
		t.Fatalf("pages=%d counts=%d", pages, counts)
	}
}

func TestEventCountUsesOnlyCountAPI(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				if r.URL.Path != "/v1/events/count" || q.Get("dataSources") != "oob" || q.Get("components") != "gpu" || q.Has("page") {
					t.Errorf("unexpected request: %s", r.URL)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"total":42}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"event", "count", "--window", "24h", "--data-source", "oob", "--components", "gpu", "--output", format})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !strings.Contains(out.String(), "42") {
				t.Fatalf("calls=%d output=%s", calls, out.String())
			}
		})
	}
}

// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestXIDBurstAllUsesTwoPagesAndOneCount(t *testing.T) {
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
		case "/v1/xid/bursts/page":
			if q.Get("page") != fmt.Sprint(pages) {
				t.Errorf("page=%s; want %d", q.Get("page"), pages)
			}
			_, _ = fmt.Fprintf(w, `{"items":[{"burstId":"burst-1"}],"page":%d,"pageSize":1,"hasMore":%t}`, pages, pages == 0)
			pages++
		case "/v1/xid/bursts/count":
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
	cmd.SetArgs([]string{"xidburst", "list", "--window", "24h", "--all", "--page-size", "1", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if pages != 2 || counts != 1 {
		t.Fatalf("pages=%d counts=%d", pages, counts)
	}
}

func TestXIDBurstCountUsesOnlyCountAPI(t *testing.T) {
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				q := r.URL.Query()
				if r.URL.Path != "/v1/xid/bursts/count" || q.Get("dataSource") != "oob" || q.Get("xidNumbers") != "48" || q.Has("page") {
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
			cmd.SetArgs([]string{"xidburst", "count", "--window", "24h", "--data-source", "oob", "--xid-numbers", "48", "--output", format})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !strings.Contains(out.String(), "42") {
				t.Fatalf("calls=%d output=%s", calls, out.String())
			}
		})
	}
}

func TestXIDBurstCountRejectsListOnlyFlags(t *testing.T) {
	for _, scenario := range []struct {
		flag  string
		value []string
	}{
		{"sort-by", []string{"startTime"}},
		{"order", []string{"desc"}},
		{"page", []string{"2"}},
		{"page-size", []string{"10"}},
		{"all", nil},
	} {
		t.Run(scenario.flag, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			args := []string{"xidburst", "count", "--window", "24h", "--" + scenario.flag}
			cmd.SetArgs(append(args, scenario.value...))
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "unknown flag: --"+scenario.flag) {
				t.Fatalf("expected flag rejection before authentication, got %v", err)
			}
		})
	}
}

func TestXIDBurstCountHelpOmitsSortingFlags(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"xidburst", "count", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--sort-by", "--order", "--page", "--all"} {
		if strings.Contains(out.String(), flag) {
			t.Errorf("count help contains list-only flag %q", flag)
		}
	}
	list := newXIDBurstListCmd()
	for _, flag := range []string{"sort-by", "order", "page", "page-size", "all"} {
		if list.Flags().Lookup(flag) == nil {
			t.Errorf("list lost flag %q", flag)
		}
	}
}

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

func TestEventCountOnlyUsesOnlyCountAPI(t *testing.T) {
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
			cmd.SetArgs([]string{"event", "list", "--count-only", "--window", "24h", "--data-source", "oob", "--components", "gpu", "--output", format})
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if calls != 1 || !strings.Contains(out.String(), "42") {
				t.Fatalf("calls=%d output=%s", calls, out.String())
			}
		})
	}
}

func TestEventCountOnlyRejectsListOnlyFlags(t *testing.T) {
	for _, scenario := range []struct {
		flag  string
		value []string
	}{
		{"sort-by", []string{"timestamp"}},
		{"order", []string{"desc"}},
		{"page", []string{"2"}},
		{"page-size", []string{"10"}},
		{"all", nil},
	} {
		t.Run(scenario.flag, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetErr(new(bytes.Buffer))
			args := []string{"event", "list", "--count-only", "--window", "24h", "--" + scenario.flag}
			cmd.SetArgs(append(args, scenario.value...))
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "--count-only cannot be used with --"+scenario.flag) {
				t.Fatalf("expected flag rejection before authentication, got %v", err)
			}
		})
	}
}

func TestEventListHelpIncludesCountOnly(t *testing.T) {
	cmd := newRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"event", "list", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--count-only", "--sort-by", "--order", "--page", "--all"} {
		if !strings.Contains(out.String(), flag) {
			t.Errorf("list help missing %q", flag)
		}
	}
	for _, command := range NewCmd().Commands() {
		if command.Name() == "count" {
			t.Error("standalone count command remains registered")
		}
	}
}

func TestEventCountOnlyModes(t *testing.T) {
	for _, scenario := range []struct {
		name      string
		args      []string
		failCount bool
		wantPages int
		wantError bool
	}{
		{name: "explicit false lists rows", args: []string{"--count-only=false", "--page", "1"}, wantPages: 1},
		{name: "count failure has no output", args: []string{"--count-only"}, failCount: true, wantError: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			pages, counts := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/events/page":
					pages++
					_, _ = w.Write([]byte(`{"events":[],"page":0,"pageSize":10,"hasMore":false}`))
				case "/v1/events/count":
					counts++
					if scenario.failCount {
						http.Error(w, "count unavailable", http.StatusBadRequest)
						return
					}
					_, _ = w.Write([]byte(`{"total":0}`))
				default:
					t.Errorf("unexpected request: %s", r.URL)
				}
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			cmd := newRootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(new(bytes.Buffer))
			args := []string{"event", "list", "--window", "24h", "--output", "json"}
			cmd.SetArgs(append(args, scenario.args...))
			err := cmd.Execute()
			if (err != nil) != scenario.wantError {
				t.Fatalf("error=%v; wantError=%t", err, scenario.wantError)
			}
			if pages != scenario.wantPages || counts != 1 {
				t.Fatalf("pages=%d counts=%d", pages, counts)
			}
			if scenario.wantError && out.Len() != 0 {
				t.Fatalf("partial output: %s", out.String())
			}
		})
	}
}

// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

var nameSearchCommands = []struct {
	name      string
	args      []string
	path      string
	body      string
	countPath string
	requests  int
}{
	{"xidburst list", []string{"xidburst", "list", "--window", "24h"}, "/v1/xid/bursts/page", `{"items":[],"hasMore":false,"page":0,"pageSize":20}`, "/v1/xid/bursts/count", 2},
	{"alert summary", []string{"alert", "summary"}, "/v1/alert_timeline/nodes", `{"nodes":[],"total":0}`, "", 1},
	{"event list", []string{"event", "list", "--window", "24h"}, "/v1/events/page", `{"events":[],"hasMore":false,"page":0,"pageSize":50}`, "/v1/events/count", 2},
	{"event buckets", []string{"event", "buckets", "--window", "24h"}, "/v1/events/buckets", `{"buckets":[]}`, "", 1},
}

func TestNameSearchAliasesSendAPIParameter(t *testing.T) {
	for _, tc := range nameSearchCommands {
		for _, flag := range []string{"--name-search", "--hostname"} {
			t.Run(tc.name+"/"+flag, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("USERPROFILE", os.Getenv("HOME"))
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					q := r.URL.Query()
					if (r.URL.Path != tc.path && r.URL.Path != tc.countPath) || q.Get("nameSearch") != "bmc-01" || q.Has("hostname") || q.Has("hostnameSearch") {
						t.Errorf("unexpected request: %s", r.URL)
					}
					w.Header().Set("Content-Type", "application/json")
					if r.URL.Path == tc.countPath {
						_, _ = w.Write([]byte(`{"total":0}`))
						return
					}
					_, _ = w.Write([]byte(tc.body))
				}))
				defer server.Close()
				cmdtest.SaveConfig(t, server.URL, "test-key")
				cmd := newTestRootCmd()
				cmd.SetOut(new(bytes.Buffer))
				cmd.SetErr(new(bytes.Buffer))
				args := append([]string{}, tc.args...)
				args = append(args, flag, " bmc-01 ", "--output", "json")
				cmd.SetArgs(args)
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				if calls != tc.requests {
					t.Fatalf("requests=%d; want %d", calls, tc.requests)
				}
			})
		}
	}
}

func TestNameSearchAliasesAreMutuallyExclusive(t *testing.T) {
	for _, tc := range nameSearchCommands {
		for _, flags := range [][]string{
			{"--name-search", "a", "--hostname", "b"},
			{"--hostname", "same", "--name-search", "same"},
			{"--name-search=", "--hostname="},
		} {
			t.Run(tc.name+strings.Join(flags, " "), func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				t.Setenv("USERPROFILE", os.Getenv("HOME"))
				cmd := newTestRootCmd()
				cmd.SetOut(new(bytes.Buffer))
				cmd.SetErr(new(bytes.Buffer))
				args := append([]string{}, tc.args...)
				cmd.SetArgs(append(args, flags...))
				err := cmd.Execute()
				if err == nil || !strings.Contains(err.Error(), "name-search") || !strings.Contains(err.Error(), "hostname") || !strings.Contains(err.Error(), "none of the others can be") {
					t.Fatalf("expected mutually exclusive flags error before authentication, got %v", err)
				}
			})
		}
	}
}

func TestNodeRetainsHostnameFilter(t *testing.T) {
	cmd, _, err := newTestRootCmd().Find([]string{"node", "list"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Flags().Lookup("hostname") == nil || cmd.Flags().Lookup("name-search") != nil {
		t.Fatal("node filters must retain OpenAPI hostname naming")
	}
}

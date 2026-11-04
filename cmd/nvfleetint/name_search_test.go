// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

var nameSearchCommands = []struct {
	name string
	args []string
	path string
	body string
}{
	{"xidburst list", []string{"xidburst", "list", "--window", "24h"}, "/v1/xid/bursts", `{"items":[],"total":0}`},
	{"alert summary", []string{"alert", "summary"}, "/v1/alert_timeline/nodes", `{"nodes":[],"total":0}`},
	{"event list", []string{"event", "list", "--window", "24h"}, "/v1/events", `{"events":[],"total":0}`},
	{"event buckets", []string{"event", "buckets", "--window", "24h"}, "/v1/events/buckets", `{"buckets":[]}`},
}

func TestNameSearchAliasesSendAPIParameter(t *testing.T) {
	for _, tc := range nameSearchCommands {
		for _, flag := range []string{"--name-search", "--hostname"} {
			t.Run(tc.name+"/"+flag, func(t *testing.T) {
				t.Setenv("HOME", t.TempDir())
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					q := r.URL.Query()
					if r.URL.Path != tc.path || q.Get("nameSearch") != "bmc-01" || q.Has("hostname") || q.Has("hostnameSearch") {
						t.Errorf("unexpected request: %s", r.URL)
					}
					w.Header().Set("Content-Type", "application/json")
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
				if calls != 1 {
					t.Fatalf("requests=%d; want 1", calls)
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

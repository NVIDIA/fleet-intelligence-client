// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestNodeHealthValidationAndHTTPFailure(t *testing.T) {
	cases := []struct {
		name     string
		uuid     string
		flags    []string
		want     []string
		requests int
	}{
		{name: "invalid output", uuid: "node-1", flags: []string{"--output", "yaml"}, want: []string{`invalid output "yaml": expected table or json`}},
		{name: "zero timeout", uuid: "node-1", flags: []string{"--timeout", "0s"}, want: []string{"--timeout must be greater than 0"}},
		{name: "blank UUID", uuid: " \t ", want: []string{"node UUID is required"}},
		{name: "missing profile", uuid: "node-1", flags: []string{"--profile", "nonexistent"}, want: []string{"profile not found", "nonexistent"}},
		{name: "forbidden", uuid: "node-1", want: []string{"403", "health access denied"}, requests: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"health access denied"}`))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			args := []string{"node", "health", tc.uuid, "--start", "2026-04-07T00:00:00Z", "--end", "2026-04-14T00:00:00Z"}
			cmd.SetArgs(append(args, tc.flags...))
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected command error")
			}
			for _, want := range tc.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != tc.requests {
				t.Errorf("requests = %d, want %d", requests, tc.requests)
			}
		})
	}
}

func TestNodeHealthEmptyHistoryRendering(t *testing.T) {
	for _, tc := range []struct {
		name    string
		body    string
		summary bool
	}{
		{name: "empty summary", body: `{"healthSummary":{},"machineStatus":[]}`, summary: true},
		{name: "absent summary", body: `{"machineStatus":[]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1/health_history" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "health", "node-1", "--start", "2026-04-07T00:00:00Z", "--end", "2026-04-14T00:00:00Z"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			gotRequests := requests
			mu.Unlock()
			if gotRequests != 1 {
				t.Errorf("requests = %d, want 1", gotRequests)
			}
			got := out.String()
			if strings.Contains(got, "Health Summary") != tc.summary {
				t.Errorf("unexpected summary heading: %q", got)
			}
			for _, want := range []string{"Machine Status", "No health timeline segments in the requested window."} {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %q", want, got)
				}
			}
			if tc.summary {
				for _, state := range []string{"Healthy", "Degraded", "Unhealthy"} {
					found := false
					for _, line := range strings.Split(got, "\n") {
						fields := strings.Fields(line)
						if len(fields) == 3 && fields[0] == state && fields[1] == "-" && fields[2] == "-" {
							found = true
						}
					}
					if !found {
						t.Errorf("missing %s row with absent percentage and duration: %q", state, got)
					}
				}
			}
		})
	}
}

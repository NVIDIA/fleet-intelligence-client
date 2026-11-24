// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

const nodeHealthBody = `{"enrolledAt":"2026-01-01T00:00:00Z","healthSummary":{"healthyPercentage":99.5,"degradedPercentage":0.5,"unhealthyPercentage":0,"healthyDurationSeconds":600000,"degradedDurationSeconds":3000,"unhealthyDurationSeconds":0},"machineStatus":[{"status":"Healthy","startTime":"2026-04-07T00:00:00Z","endTime":"2026-04-13T00:00:00Z"},{"status":"Degraded","startTime":"2026-04-13T00:00:00Z","endTime":"2026-04-14T00:00:00Z"}]}`

// Verifies the health table renders summary and timeline sections
func TestNodeHealthTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/nodes/node-1/health_history" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("startTime"); got != "2026-04-07T00:00:00Z" {
			t.Fatalf("unexpected startTime: %q", got)
		}
		if got := r.URL.Query().Get("endTime"); got != "2026-04-14T00:00:00Z" {
			t.Fatalf("unexpected endTime: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nodeHealthBody))
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

	got := out.String()
	for _, want := range []string{
		"UUID", "ENROLLED AT", "2026-01-01T00:00:00Z",
		"Health Summary",
		"STATE", "PERCENTAGE", "DURATION", "99.5%", "166h40m0s",
		"Machine Status",
		"STATUS", "START TIME", "END TIME",
		"Healthy", "Degraded", "Unhealthy",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// Verifies JSON output emits the raw backend payload
func TestNodeHealthJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nodeHealthBody))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "health", "node-1", "--start", "2026-04-07T00:00:00Z", "--end", "2026-04-14T00:00:00Z", "--output", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != nodeHealthBody {
		t.Fatalf("unexpected JSON:\n%s", out.String())
	}
}

// Verifies flag validation rejects missing or malformed time windows before any request
func TestNodeHealthRequiresValidTimeRange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	cases := [][]string{
		{"node", "health", "node-1", "--start", "2026-04-07T00:00:00Z"},
		{"node", "health", "node-1", "--end", "2026-04-14T00:00:00Z"},
		{"node", "health", "node-1", "--start", "not-a-time", "--end", "2026-04-14T00:00:00Z"},
		{"node", "health", "node-1", "--start", "2026-04-07T00:00:00Z", "--end", "not-a-time"},
	}
	for _, args := range cases {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected error for args %v", args)
		}
	}
}

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

func TestNodeHealthTableWriteBoundaries(t *testing.T) {
	var history nvfleetint.NodeHealthHistory
	if err := json.Unmarshal([]byte(nodeHealthBody), &history); err != nil {
		t.Fatalf("decode health fixture: %v", err)
	}
	history.Segments = nil
	if history.HealthSummary == nil || history.HealthSummary.HealthyDurationSeconds == nil {
		t.Fatal("fixture has no populated summary")
	}
	control := &nodeRenderWriter{failAt: -1}
	if err := writeNodeHealthTable(control, "node-1", history); err != nil {
		t.Fatalf("render control: %v", err)
	}
	full := control.output.String()
	summary := strings.Index(full, "\nHealth Summary\n")
	machine := strings.Index(full, "\nMachine Status\n")
	empty := strings.Index(full, "No health timeline segments in the requested window.\n")
	if summary <= 0 || machine <= summary || empty <= machine {
		t.Fatalf("unexpected section order: %q", full)
	}
	for _, tc := range []struct {
		name   string
		offset int
		prefix string
	}{
		{name: "metadata table", offset: 0, prefix: "FIELD"},
		{name: "summary heading", offset: summary, prefix: "\nHealth Summary\n"},
		{name: "summary table", offset: summary + len("\nHealth Summary\n"), prefix: "STATE"},
		{name: "machine heading", offset: machine, prefix: "\nMachine Status\n"},
		{name: "empty timeline message", offset: empty, prefix: "No health timeline segments in the requested window.\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			index, offset := -1, 0
			for i, part := range control.writes {
				if offset == tc.offset && strings.HasPrefix(part, tc.prefix) {
					index = i
					break
				}
				offset += len(part)
			}
			if index < 0 {
				t.Fatalf("boundary at %d beginning %q not found in writes %q", tc.offset, tc.prefix, control.writes)
			}
			sentinel := errors.New("health write failed")
			w := &nodeRenderWriter{failAt: index, err: sentinel}
			if err := writeNodeHealthTable(w, "node-1", history); !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if w.after != 0 || len(w.writes) != index+1 || w.output.String() != full[:tc.offset] {
				t.Errorf("unexpected writes after failure: accepted=%q writes=%q", w.output.String(), w.writes)
			}
		})
	}
}

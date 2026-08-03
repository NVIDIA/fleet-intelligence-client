// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

const nodeHealthBody = `{"enrolledAt":"2026-01-01T00:00:00Z","healthSummary":{"healthyPercentage":99.5,"degradedPercentage":0.5,"unhealthyPercentage":0,"healthyDurationSeconds":600000,"degradedDurationSeconds":3000,"unhealthyDurationSeconds":0},"machineStatus":[{"status":"Healthy","startTime":"2026-04-07T00:00:00Z","endTime":"2026-04-13T00:00:00Z"},{"status":"Degraded","startTime":"2026-04-13T00:00:00Z","endTime":"2026-04-14T00:00:00Z"}]}`

// Verifies the health table renders summary and timeline sections
func TestNodeHealthTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(nodeHealthBody))
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

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

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	if err := config.Save(config.Config{APIURL: server.URL, ServiceKey: "test-key"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

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

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

// Verifies describe drill-down flags are validated before making an API request
func TestNodeDescribeRejectsInvalidSectionFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "unknown section",
			args: []string{"node", "describe", "node-1", "--data-source", "oob", "--section", "storage"},
			want: "invalid OOB inventory section",
		},
		{
			name: "all with another section",
			args: []string{
				"node", "describe", "node-1", "--data-source", "oob",
				"--section", "all,systems",
			},
			want: "section all cannot be combined with other sections",
		},
		{
			name: "inband section",
			args: []string{
				"node", "describe", "node-1", "--data-source", "inband",
				"--section", "systems",
			},
			want: "--section requires the OOB view",
		},
		{
			name: "json section",
			args: []string{
				"node", "describe", "node-1", "--data-source", "oob",
				"--section", "systems", "--output", "json",
			},
			want: "--section cannot be used with --output json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies local flag validation
func TestNodeListRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "health", args: []string{"node", "list", "--health", "Broken"}, want: "invalid health"},
		{name: "agent", args: []string{"node", "list", "--agent-status", "Missing"}, want: "invalid agent-status"},
		{name: "verification", args: []string{"node", "list", "--verification-check", "Missing"}, want: "invalid verification-check"},
		{name: "firmware", args: []string{"node", "list", "--firmware-check", "Missing"}, want: "invalid firmware-check"},
		{name: "gpu-count", args: []string{"node", "list", "--gpu-count", "eight"}, want: "invalid gpu-count"},
		{name: "negative gpu-count", args: []string{"node", "list", "--gpu-count", "8,-1"}, want: "invalid gpu-count"},
		{name: "sort", args: []string{"node", "list", "--sort-by", "bad"}, want: "invalid sort-by"},
		{name: "order", args: []string{"node", "list", "--order", "up"}, want: "invalid order"},
		{name: "basic filter", args: []string{"node", "list", "--view", "basic", "--health", "Healthy"}, want: "basic node view is incompatible"},
		{name: "basic sort", args: []string{"node", "list", "--view", "basic", "--sort-by", "healthStatus"}, want: "basic node view supports sorting only by"},
		{name: "filter without data-source", args: []string{"node", "list", "--hostname", "gpu"}, want: "--data-source is required when using --hostname"},
		{name: "sort-by without data-source", args: []string{"node", "list", "--sort-by", "hostname"}, want: "--data-source is required when using --sort-by"},
		{name: "order without data-source", args: []string{"node", "list", "--order", "asc"}, want: "--data-source is required when using --order"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tt.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}
}

// Verifies shared list pagination validation
func TestListAllRejectsPage(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"node", "list", "--all", "--page", "1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected --all and --page error")
	}
	if !strings.Contains(err.Error(), "--page cannot be used with --all") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies an unsupported data source is rejected before any request.
func TestNodeOptionsRejectsDataSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"node", "options", "--data-source", "bmc"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid data-source") {
		t.Fatalf("unexpected error: %v", err)
	}
	if requests != 0 {
		t.Fatalf("expected no requests, got %d", requests)
	}
}

func TestNodeUUIDFilterValidationBeforeAuth(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	for _, args := range [][]string{
		{"--data-source", "oob", "--agent-uuids", "agent-A"},
		{"--view", "basic", "--data-source", "oob", "--agent-uuids", "agent-A"},
		{"--agent-uuids", "agent-A"}, {"--system-uuids", "system-A"},
	} {
		cmd := newRootCmd()
		cmd.SetArgs(append([]string{"node", "list"}, args...))
		err := cmd.Execute()
		if err == nil || (!strings.Contains(err.Error(), "agent-uuids") && !strings.Contains(err.Error(), "system-uuids")) {
			t.Fatalf("unexpected validation: %v", err)
		}
	}
}

func TestNodeRejectsEmptyCommaListEntries(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantFlag string
	}{
		{name: "health", args: []string{"node", "list", "--data-source", "inband", "--health", "Healthy,,Degraded"}, wantFlag: "health"},
		{name: "agent status", args: []string{"node", "list", "--data-source", "inband", "--agent-status", "Online,,Offline"}, wantFlag: "agent-status"},
		{name: "verification check", args: []string{"node", "list", "--data-source", "inband", "--verification-check", "Verified,,Pending"}, wantFlag: "verification-check"},
		{name: "firmware check", args: []string{"node", "list", "--data-source", "inband", "--firmware-check", "Passed,,Failed"}, wantFlag: "firmware-check"},
		{name: "node UUIDs", args: []string{"node", "list", "--data-source", "inband", "--node-uuids", "node-1,,node-2"}, wantFlag: "node-uuids"},
		{name: "sections", args: []string{"node", "describe", "node-1", "--data-source", "oob", "--section", "systems,,chassis"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Error("expected command error")
			} else {
				if !strings.Contains(err.Error(), "empty values are not allowed") {
					t.Errorf("error %q does not identify empty list values", err)
				}
				if tc.wantFlag != "" && !strings.Contains(err.Error(), tc.wantFlag) {
					t.Errorf("error %q does not identify flag %q", err, tc.wantFlag)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 0 {
				t.Errorf("server received %d requests, want none", requests)
			}
		})
	}
}

func TestNodeCommandEarlyValidation(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "blank describe UUID", args: []string{"node", "describe", " \t "}, want: []string{"node UUID is required"}},
		{name: "invalid describe source", args: []string{"node", "describe", "node-1", "--data-source", "bmc"}, want: []string{"data-source", "bmc"}},
		{name: "zero options timeout", args: []string{"node", "options", "--timeout", "0"}, want: []string{"--timeout must be greater than 0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				http.Error(w, "unexpected request", http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected validation error")
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
			if requests != 0 {
				t.Errorf("requests = %d, want none", requests)
			}
		})
	}
}

func TestNodeCommandsRequireCredentials(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"node", "list"}},
		{name: "describe", args: []string{"node", "describe", "node-1"}},
		{name: "options", args: []string{"node", "options"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			t.Setenv("NVFLEETINT_API_KEY", "")
			t.Setenv("NVFLEETINT_API_URL", "")
			t.Setenv("NVFLEETINT_SERVICE_KEY", "")
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected missing credential error")
			}
			for _, want := range []string{"no profile configured", "auth add", "NVFLEETINT_API_KEY"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("expected empty stdout, got %q", out.String())
			}
		})
	}
}

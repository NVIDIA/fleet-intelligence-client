// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

// Verifies xidburst list rejects invalid flags before any request
func TestXIDBurstListFlagValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	tests := []struct {
		name string
		args []string
	}{
		{"missing time range", []string{"xidburst", "list"}},
		{"window with start", []string{"xidburst", "list", "--window", "24h", "--start", "2026-05-01T00:00:00Z"}},
		{"bad sort field", []string{"xidburst", "list", "--window", "24h", "--sort-by", "burstId"}},
		{"bad order", []string{"xidburst", "list", "--window", "24h", "--order", "sideways"}},
		{"non-numeric xid", []string{"xidburst", "list", "--window", "24h", "--xid-numbers", "forty-eight"}},
		{"bad output", []string{"xidburst", "list", "--window", "24h", "--output", "yaml"}},
		{"nodegroup include with exclude", []string{
			"xidburst", "list", "--window", "24h",
			"--nodegroup-ids", "ng-1", "--exclude-nodegroup-ids", "ng-2",
		}},
		{"compute zone include with exclude", []string{
			"xidburst", "list", "--window", "24h",
			"--compute-zone-ids", "cz-1", "--exclude-compute-zone-ids", "cz-2",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(new(bytes.Buffer))
			cmd.SetArgs(tt.args)
			if err := cmd.Execute(); err == nil {
				t.Fatalf("expected error for %v", tt.args)
			}
		})
	}
}

// Verifies xidburst describe requires exactly one burst ID
func TestXIDBurstDescribeRequiresBurstID(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid args")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, args := range [][]string{
		{"xidburst", "describe"},
		{"xidburst", "describe", "burst-1", "burst-2"},
	} {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected error for %v", args)
		}
	}
}

// Verifies scope flags are validated before a request, unlike 'xidburst list'
// the time range stays optional
func TestXIDBurstOptionsRejectsInvalidScopeFlags(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"xidNumbers":[]}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "window with start",
			args: []string{"--window", "24h", "--start", "2026-05-01T00:00:00Z"},
			want: "--window cannot be used with --start or --end",
		},
		{
			name: "start without end",
			args: []string{"--start", "2026-05-01T00:00:00Z"},
			want: "--start and --end must be used together",
		},
		{
			name: "node group include and exclude",
			args: []string{"--nodegroup-ids", "ng-1", "--exclude-nodegroup-ids", "ng-2"},
			want: "node group include and exclude filters cannot be combined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs(append([]string{"xidburst", "options"}, tt.args...))
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: got %v want %q", err, tt.want)
			}
		})
	}

	if requests != 0 {
		t.Fatalf("expected no requests to reach the server, got %d", requests)
	}
}

func TestXIDBurstReadFlagValidationBeforeRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"describe output", []string{"xidburst", "describe", "burst-1", "--output", "yaml"}, `invalid output "yaml": expected table or json`},
		{"describe timeout", []string{"xidburst", "describe", "burst-1", "--timeout", "0s"}, "--timeout must be greater than 0"},
		{"options output", []string{"xidburst", "options", "--output", "yaml"}, `invalid output "yaml": expected table or json`},
		{"options timeout", []string{"xidburst", "options", "--timeout", "0s"}, "--timeout must be greater than 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 || requests.Load() != 0 {
				t.Errorf("output = %q, requests = %d; want neither", out.String(), requests.Load())
			}
		})
	}
}

func TestXIDBurstRejectsMalformedFilterLists(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(xidBurstsBody))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tt := range []struct {
		name  string
		args  []string
		wants []string
	}{
		{"empty xid", []string{"xidburst", "list", "--window", "24h", "--xid-numbers", "48,,94"}, []string{"--xid-numbers", "empty values are not allowed"}},
		{"negative xid", []string{"xidburst", "list", "--window", "24h", "--xid-numbers=-1"}, []string{"xid-numbers", "-1", "non-negative"}},
		{"empty category", []string{"xidburst", "list", "--window", "24h", "--categories", "GPU,,Memory"}, []string{"--categories", "empty values are not allowed"}},
		{"empty options node group", []string{"xidburst", "options", "--nodegroup-ids", "ng-1,,ng-2"}, []string{"--nodegroup-ids", "empty values are not allowed"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected an error")
			}
			for _, want := range tt.wants {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q does not contain %q", err, want)
				}
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("expected no requests, got %d", got)
			}
			if out.Len() != 0 {
				t.Errorf("expected no rendered result, got %q", out.String())
			}
		})
	}
}

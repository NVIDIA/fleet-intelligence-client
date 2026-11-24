// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/spf13/cobra"
)

// Verifies node context is required
func TestAlertDescribeRequiresNode(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "describe", "alert-1"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected missing node error")
	}
	if !strings.Contains(err.Error(), "--node is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies alert flag validation
func TestAlertListRejectsInvalidSeverity(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "list", "--severity", "Fatal"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid severity error")
	}
	if !strings.Contains(err.Error(), "invalid severity") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies alert state flag validation
func TestAlertListRejectsInvalidState(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "list", "--state", "Pending"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid state error")
	}
	if !strings.Contains(err.Error(), "invalid state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies alert list uses the shared 1-based page rule and rejects out-of-range
func TestAlertListRejectsNegativePage(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"alert", "list", "--page=0"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected page error")
	}
	if !strings.Contains(err.Error(), "--page must be greater than or equal to 1") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Verifies mode-specific alert flags are rejected before a request
func TestAlertTimelineRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "summary sort", args: []string{"alert", "summary", "--sort-by", "startTime"}, want: "invalid sort-by"},
		{name: "node sort", args: []string{"alert", "node", "node-1", "--sort-by", "alert"}, want: "invalid sort-by"},
		// The API dropped the component sort; only startTime and lastUpdate remain.
		{name: "node component sort", args: []string{"alert", "node", "node-1", "--sort-by", "component"}, want: "expected startTime, lastUpdate, or dataSource"},
		{name: "missing node", args: []string{"alert", "node"}, want: "node UUID is required"},
		{name: "state", args: []string{"alert", "summary", "--alert-state", "Triggered"}, want: "invalid alert-state"},
		{name: "describe page", args: []string{"alert", "describe", "alert-1", "--node", "node-1", "--page", "2"}, want: "requires --page-size"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(test.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestAlertCommandsRejectMissingProfile(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "list", args: []string{"alert", "list"}},
		{name: "summary", args: []string{"alert", "summary"}},
		{name: "node", args: []string{"alert", "node", "node-1"}},
		{name: "options", args: []string{"alert", "options"}},
		{name: "describe", args: []string{"alert", "describe", "alert-1", "--node", "node-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "configured-key")
			t.Setenv(config.EnvAPIURL, server.URL)
			t.Setenv(config.EnvAPIKey, "environment-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append(append([]string(nil), tc.args...), "--profile", "missing"))
			err := cmd.Execute()
			if !errors.Is(err, config.ErrProfileNotFound) {
				t.Errorf("error = %v, want ErrProfileNotFound", err)
			}
			if err == nil || !strings.Contains(err.Error(), "nvfleetint auth list") {
				t.Errorf("error = %v, want auth list hint", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("got %d HTTP requests, want none", got)
			}
		})
	}
}

func TestAlertNodeWithoutPSIRTRejectsBlankNode(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"alert", "node", "   ", "--without-psirt"})
	err := cmd.Execute()
	if err == nil || err.Error() != "--without-psirt requires --node" {
		t.Errorf("error = %v, want --without-psirt requires --node", err)
	}
	if out.Len() != 0 {
		t.Errorf("unexpected output: %q", out.String())
	}
	if got := requests.Load(); got != 0 {
		t.Errorf("got %d HTTP requests, want none", got)
	}
}

func TestAlertDescribeRejectsInvalidInputsBeforeRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{name: "blank UUID", args: []string{"   ", "--node", "node-1"}, want: "alert UUID is required"},
		{name: "invalid order", args: []string{"alert-1", "--node", "node-1", "--order", "invalid"}, want: "invalid order \"invalid\": expected asc or desc"},
		{name: "page zero", args: []string{"alert-1", "--node", "node-1", "--page", "0", "--page-size", "1"}, want: "--page must be greater than or equal to 1"},
		{name: "page size zero", args: []string{"alert-1", "--node", "node-1", "--page-size", "0"}, want: "--page-size must be between 1 and 100"},
		{name: "page size too large", args: []string{"alert-1", "--node", "node-1", "--page-size", "101"}, want: "--page-size must be between 1 and 100"},
		{name: "timeout zero", args: []string{"alert-1", "--node", "node-1", "--timeout", "0"}, want: "--timeout must be greater than 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"alert", "describe"}, tc.args...))
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("got %d HTTP requests, want none", got)
			}
		})
	}
}

func TestAlertTimelineRejectsInvalidInputsBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "summary invalid view", args: []string{"summary", "--view", "recent"}, want: []string{"invalid view"}},
		{name: "node invalid view", args: []string{"node", "node-1", "--view", "recent"}, want: []string{"invalid view"}},
		{name: "options invalid view", args: []string{"options", "--view", "recent"}, want: []string{"invalid view"}},
		{name: "summary empty alert state", args: []string{"summary", "--alert-state", "Detected,,Resolved"}, want: []string{"invalid alert-state", "empty values"}},
		{name: "node empty alert state", args: []string{"node", "node-1", "--alert-state", "Detected,,Resolved"}, want: []string{"invalid alert-state", "empty values"}},
		{name: "summary empty compute zone", args: []string{"summary", "--compute-zone-ids", "zone-1,,zone-2"}, want: []string{"compute-zone-ids", "empty values"}},
		{name: "node empty component type", args: []string{"node", "node-1", "--component-type", "gpu,,memory"}, want: []string{"component-type", "empty values"}},
		{name: "summary empty oob status", args: []string{"summary", "--oob-agent-status", "online,,offline"}, want: []string{"oob-agent-status", "empty values"}},
		{name: "node empty data source", args: []string{"node", "node-1", "--data-source", "inband,,oob"}, want: []string{"data-source", "empty values"}},
		{name: "summary page with all", args: []string{"summary", "--all", "--page", "1"}, want: []string{"--page cannot be used with --all"}},
		{name: "node page with all", args: []string{"node", "node-1", "--all", "--page", "1"}, want: []string{"--page cannot be used with --all"}},
		{name: "options zero timeout", args: []string{"options", "--timeout", "0"}, want: []string{"--timeout must be greater than 0"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(append([]string{"alert"}, tc.args...))
			err := cmd.Execute()
			for _, want := range tc.want {
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Errorf("error = %v, want substring %q", err, want)
				}
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("got %d HTTP requests, want none", got)
			}
		})
	}
}

func TestAlertSummaryTimelineFlagValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   alertTimelineFlags
		wantErr string
	}{
		{
			name:    "without PSIRT requires node",
			flags:   alertTimelineFlags{node: "  ", withoutPSIRT: true},
			wantErr: "--without-psirt requires --node",
		},
		{
			name:    "hostname conflicts with node",
			flags:   alertTimelineFlags{node: " node-1 ", hostname: " host-1 "},
			wantErr: "--hostname cannot be used with --node",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&out)
			err := runAlertSummary(cmd, tc.flags, cmdutil.Resolved{Output: "table", Timeout: time.Second})
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want empty", out.String())
			}
		})
	}

	if err := validateAlertTimelineFlags(alertTimelineFlags{node: " node-1 ", hostname: "   "}); err != nil {
		t.Errorf("blank hostname validation error = %v, want nil", err)
	}
}

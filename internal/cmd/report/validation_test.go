// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

// Verifies an explicitly set --output is rejected with --format csv
func TestReportInventoryRejectsOutputWithCSV(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	cmdtest.SaveConfig(t, "https://example.invalid", "test-key")

	for _, output := range []string{"table", "json"} {
		t.Run(output, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs([]string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--output", output})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), "--output cannot be used with --format csv") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies local inventory report flag validation
func TestReportInventoryRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "compute zone IDs", args: []string{"report", "inventory", "--compute-zone-ids", "cz-1,,cz-2"}, want: "empty values are not allowed"},
		{name: "start alone", args: []string{"report", "inventory", "--start", "2026-05-01T00:00:00Z"}, want: "--start and --end must be used together"},
		{name: "bad start", args: []string{"report", "inventory", "--start", "yesterday", "--end", "2026-05-02T00:00:00Z"}, want: "--start must be RFC3339"},
		{name: "bad sort", args: []string{"report", "inventory", "--sort-by", "name"}, want: "invalid sort-by"},
		{name: "bad order", args: []string{"report", "inventory", "--order", "up"}, want: "invalid order"},
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

// Verifies signed inventory flag combinations are rejected before any request
func TestReportInventorySignedValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	cmdtest.SaveConfig(t, "https://example.invalid", "test-key")

	cases := []struct {
		name string
		args []string
		want string
	}{
		{name: "signed without csv", args: []string{"report", "inventory", "--signed"}, want: "--signed requires --format csv"},
		{name: "signed with json", args: []string{"report", "inventory", "--format", "json", "--signed"}, want: "--signed requires --format csv"},
		{name: "output-path without signed", args: []string{"report", "inventory", "--format", "csv", "--output-path", "out.zip"}, want: "--output-path can only be used with --signed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetArgs(tc.args)

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Verifies local error report flag validation
func TestReportErrorRejectsInvalidFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "default view needs time range", args: []string{"report", "error"}, want: "a time range is required"},
		{name: "empty view", args: []string{"report", "error", "--view", ""}, want: "--view is required"},
		{name: "missing group", args: []string{"report", "error", "--view", "list"}, want: "--group-by is required"},
		{name: "bad view", args: []string{"report", "error", "--view", "bad"}, want: "invalid view"},
		{name: "bad group", args: []string{"report", "error", "--view", "list", "--group-by", "bad"}, want: "invalid group-by"},
		{name: "graph node group", args: []string{"report", "error", "--view", "graph", "--group-by", "node"}, want: "only supports --group-by error"},
		{name: "missing time range", args: []string{"report", "error", "--view", "graph"}, want: "a time range is required"},
		{name: "missing time range list", args: []string{"report", "error", "--view", "list", "--group-by", "error"}, want: "a time range is required"},
		{name: "overview group", args: []string{"report", "error", "--view", "overview", "--group-by", "error"}, want: "--group-by cannot be used"},
		{name: "csv overview", args: []string{"report", "error", "--view", "overview", "--format", "csv"}, want: "--format csv is only supported"},
		{name: "csv with output table", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--format", "csv", "--output", "table"}, want: "--output cannot be used with --format csv"},
		{name: "csv with output json", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--format", "csv", "--output", "json"}, want: "--output cannot be used with --format csv"},
		{name: "bad window", args: []string{"report", "error", "--view", "overview", "--window", "soon"}, want: "invalid window"},
		{name: "day window", args: []string{"report", "error", "--view", "overview", "--window", "7d"}, want: "invalid window"},
		{name: "huge window", args: []string{"report", "error", "--view", "overview", "--window", "22342394090s"}, want: "duration is too large"},
		{name: "bad compute zone IDs", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--compute-zone-ids", "cz-1,,cz-2"}, want: "empty values are not allowed"},
		{name: "bad severities", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--severities", "Critical,Broken"}, want: "invalid severity"},
		{name: "errors with group by error", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--errors", "xid_154"}, want: "--errors can only be used"},
		{name: "step overview", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--step", "5m"}, want: "--step can only be used with --view graph"},
		{name: "short step", args: []string{"report", "error", "--view", "graph", "--window", "24h", "--step", "30s"}, want: "expected at least 1m"},
		{name: "start alone", args: []string{"report", "error", "--view", "overview", "--start", "2026-05-01T00:00:00Z"}, want: "--start and --end"},
		{name: "bad start", args: []string{"report", "error", "--view", "overview", "--start", "yesterday", "--end", "2026-05-01T00:00:00Z"}, want: "--start must be RFC3339"},
		{name: "graph page", args: []string{"report", "error", "--view", "graph", "--page", "1"}, want: "pagination flags"},
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
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestReportValidationBoundariesRejectBeforeRequest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	var mu sync.Mutex
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")

	cases := []struct {
		name string
		args []string
		want []string
	}{
		{name: "inventory csv page", args: []string{"report", "inventory", "--data-source", "inband", "--format", "csv", "--page", "1"}, want: []string{"pagination flags cannot be used with --format csv"}},
		{name: "error csv page size", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--format", "csv", "--page-size", "1"}, want: []string{"pagination flags cannot be used with --format csv"}},
		{name: "error overview page", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--page", "1"}, want: []string{"pagination flags cannot be used with --view overview"}},
		{name: "error conflicting time range", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--start", "2026-05-01T00:00:00Z", "--end", "2026-05-02T00:00:00Z"}, want: []string{"--window cannot be used with --start or --end"}},
		{name: "inventory invalid end", args: []string{"report", "inventory", "--start", "2026-05-01T00:00:00Z", "--end", "yesterday"}, want: []string{"--end must be RFC3339"}},
		{name: "error invalid end", args: []string{"report", "error", "--view", "overview", "--start", "2026-05-01T00:00:00Z", "--end", "yesterday"}, want: []string{"--end must be RFC3339"}},
		{name: "error empty severity", args: []string{"report", "error", "--view", "overview", "--window", "24h", "--severities", "Critical,,Fatal"}, want: []string{"invalid severity", "empty values are not allowed"}},
		{name: "inventory invalid page", args: []string{"report", "inventory", "--page", "0"}, want: []string{"--page must be greater than or equal to 1"}},
		{name: "error invalid page", args: []string{"report", "error", "--view", "list", "--group-by", "error", "--window", "24h", "--page", "0"}, want: []string{"--page must be greater than or equal to 1"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			for _, part := range tc.want {
				if err == nil || !strings.Contains(err.Error(), part) {
					t.Errorf("error = %v, want it to contain %q", err, part)
				}
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
			mu.Lock()
			count := requests
			mu.Unlock()
			if count != 0 {
				t.Errorf("request count = %d, want 0", count)
			}
		})
	}
}

func TestReportCommandsRequireCredentials(t *testing.T) {
	for _, name := range []string{"inventory", "error", "verify"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			var args []string
			switch name {
			case "inventory":
				args = []string{"report", "inventory", "--data-source", "inband"}
			case "error":
				args = []string{"report", "error", "--window", "24h"}
			case "verify":
				csv, bundle, _ := writeSignedFixture(t, t.TempDir(), []byte("customer\nacme\n"))
				args = []string{"report", "verify", "--csv", csv, "--bundle", bundle}
			}
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetArgs(args)
			err := cmd.Execute()
			if !errors.Is(err, config.ErrNoProfile) {
				t.Errorf("error = %v, want ErrNoProfile", err)
			}
			if err == nil || !strings.Contains(err.Error(), "nvfleetint auth add <name>") || !strings.Contains(err.Error(), config.EnvAPIKey) {
				t.Errorf("error = %v, want credential setup hint", err)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output after failure: %q", out.String())
			}
		})
	}
}

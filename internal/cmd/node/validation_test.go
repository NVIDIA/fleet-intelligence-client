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

// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package report

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

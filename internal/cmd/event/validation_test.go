// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package event

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

// Verifies event buckets rejects an out-of-range max-buckets value
func TestEventBucketsMaxBucketsValidation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("did not expect a request for invalid flags")
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	for _, value := range []string{"0", "2000"} {
		cmd := newRootCmd()
		cmd.SetOut(new(bytes.Buffer))
		cmd.SetArgs([]string{"event", "buckets", "--window", "24h", "--max-buckets", value})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("expected error for --max-buckets %s", value)
		}
	}
}

func TestEventCommandValidationBeforeRequest(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"list pagination conflict", []string{"event", "list", "--window", "24h", "--all", "--page", "1"}, "--page cannot be used with --all"},
		{"buckets invalid output", []string{"event", "buckets", "--window", "24h", "--output", "yaml"}, `invalid output "yaml": expected table or json`},
		{"buckets missing time range", []string{"event", "buckets"}, "a time range is required: use --window"},
		{"options invalid timeout", []string{"event", "options", "--timeout", "0s"}, "--timeout must be greater than 0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want %q", err, tc.want)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected output: %q", out.String())
			}
			mu.Lock()
			defer mu.Unlock()
			if requests != 0 {
				t.Errorf("requests = %d, want 0", requests)
			}
		})
	}
}

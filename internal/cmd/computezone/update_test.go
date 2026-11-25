// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package computezone

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestComputeZoneUpdateEarlyValidation(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "invalid timeout", args: []string{"computezone", "update", "cz-1", "--type", "datacenter", "--yes", "--timeout", "0s"}, want: "--timeout must be greater than 0"},
		{name: "whitespace ID", args: []string{"computezone", "update", "   ", "--type", "datacenter", "--yes"}, want: "compute zone ID is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var requests atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err == nil || err.Error() != tt.want {
				t.Errorf("error = %v, want %q", err, tt.want)
			}
			if out.Len() != 0 {
				t.Errorf("unexpected result output: %q", out.String())
			}
			if got := requests.Load(); got != 0 {
				t.Errorf("HTTP requests = %d, want 0", got)
			}
		})
	}
}

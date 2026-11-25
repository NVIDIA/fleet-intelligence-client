// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
)

func TestAuthStatusReportsUnsuccessfulConnectionsJSON(t *testing.T) {
	tests := []struct {
		name             string
		code             int
		body, connection string
	}{
		{"unauthenticated", http.StatusOK, `{"authenticated":false}`, "unauthenticated"},
		{"forbidden", http.StatusForbidden, `{"error":"forbidden"}`, "unauthorized"},
		{"server error", http.StatusInternalServerError, `{"error":"server error"}`, "error:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			server := newAuthStatusServer(t, tt.code, tt.body)
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, apiKey)
			out := mustRunAuth(t, "status", "--output", "json")
			var got authStatusOutput
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("decode status JSON: %v", err)
			}
			if got.Profile != cmdtest.Profile || got.APIURL != server.URL || !got.APIKeyConfigured {
				t.Fatalf("unexpected profile status: %#v", got)
			}
			if tt.code == http.StatusInternalServerError {
				if !strings.HasPrefix(got.Connection, tt.connection) || !strings.Contains(got.Connection, "500") {
					t.Fatalf("unexpected connection: %q", got.Connection)
				}
			} else if got.Connection != tt.connection {
				t.Fatalf("connection = %q, want %q", got.Connection, tt.connection)
			}
			if strings.Contains(out, apiKey) {
				t.Fatalf("status exposed the key: %q", out)
			}
		})
	}
}

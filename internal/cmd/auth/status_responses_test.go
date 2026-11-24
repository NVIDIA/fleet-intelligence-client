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
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

// The warnings are part of the answer, so JSON consumers get them too.
func TestAuthStatusJSONIncludesWarnings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	if err := config.Save(config.Config{CurrentProfile: "gone"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var status authStatusOutput
	if err := json.Unmarshal([]byte(mustRunAuth(t, "status", "-o", "json")), &status); err != nil {
		t.Fatalf("decode status failed: %v", err)
	}
	if len(status.Warnings) == 0 {
		t.Fatal("expected a warning about the stale current profile")
	}
	if !strings.Contains(strings.Join(status.Warnings, "\n"), `"gone"`) {
		t.Fatalf("warning did not name the stale profile: %v", status.Warnings)
	}
}

func TestAuthStatusReportsUnauthorizedOnRejectedKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := newAuthStatusServer(t, http.StatusUnauthorized, `{"error":"unauthorized"}`)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, apiKey)

	// A rejected key is a reportable status, not a command failure.
	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "Connection: unauthorized") {
		t.Fatalf("status missing unauthorized state: %q", got)
	}
}

func TestAuthStatusJSONUsesEnvFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	t.Setenv(config.EnvAPIURL, server.URL)
	t.Setenv(config.EnvAPIKey, apiKey)

	out := mustRunAuth(t, "status", "--output", "json")
	var got authStatusOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode status JSON failed: %v", err)
	}
	if got.APIURL != server.URL || !got.APIKeyConfigured || got.Connection != "ok" {
		t.Fatalf("unexpected status JSON: %#v", got)
	}
	if got.APIKeySource != string(config.SourceEnvironment) {
		t.Fatalf("expected the environment to be named as the source: %#v", got)
	}
	if strings.Contains(out, apiKey) {
		t.Fatalf("status printed secret: %q", out)
	}
}

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

func TestAuthStatusReportsInsecureURLJSON(t *testing.T) {
	const name = "selected"
	const secret = "status-insecure-url-secret"
	const insecureURL = "http://example.com"
	tests := []struct {
		name, urlSource, keySource, hint string
		environment                      bool
	}{
		{"stored profile", string(config.SourceProfile), string(config.SourceProfile), "nvfleetint auth add " + name, false},
		{"environment override", string(config.SourceEnvironment), string(config.SourceEnvironment), "update " + config.EnvAPIURL, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			storedURL := insecureURL
			if tt.environment {
				storedURL = "https://stored.example.com"
			}
			if err := config.Save(config.Config{CurrentProfile: name, Profiles: map[string]config.Profile{
				name: {APIURL: storedURL, APIKey: secret},
			}}); err != nil {
				t.Fatalf("save config: %v", err)
			}
			if tt.environment {
				t.Setenv(config.EnvAPIURL, insecureURL)
				t.Setenv(config.EnvAPIKey, secret)
			}
			out := mustRunAuth(t, "status", "--output", "json")
			var got authStatusOutput
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("decode status JSON: %v", err)
			}
			if got.Profile != name || got.APIURL != insecureURL || got.APIURLSource != tt.urlSource || !got.APIKeyConfigured || got.APIKeySource != tt.keySource {
				t.Fatalf("unexpected resolved status: %#v", got)
			}
			if !strings.HasPrefix(got.Connection, "error:") || !strings.Contains(got.Connection, "insecure base URL: https is required") || !strings.Contains(got.Connection, tt.hint) {
				t.Fatalf("unexpected connection diagnostic: %q", got.Connection)
			}
			if strings.Contains(out, secret) {
				t.Fatalf("status exposed the key: %q", out)
			}
		})
	}
}

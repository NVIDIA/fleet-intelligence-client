// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

// The profile-naming commands take exactly one <name>; a second word is a typo
// (an unquoted name, a stray flag value) and must not be silently ignored.
func TestAuthSubcommandsRejectExtraProfileNames(t *testing.T) {
	tests := [][]string{
		{"add", "prod", "extra"},
		{"remove", "prod", "extra", "--yes"},
		{"use", "prod", "extra"},
	}

	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)

			err := runAuthErr(t, args...)
			if !strings.Contains(err.Error(), "only one profile name may be given") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Bare `auth` is still a help request, not an error.
func TestAuthWithoutSubcommandPrintsHelp(t *testing.T) {
	got := mustRunAuth(t)
	if !strings.Contains(got, "Available Commands:") || !strings.Contains(got, "add") {
		t.Fatalf("expected help output: %q", got)
	}
}

// list and status act on the configuration as a whole, so they take no name.
func TestAuthSubcommandsRejectPositionalArgs(t *testing.T) {
	tests := [][]string{
		{"list", "extra"},
		{"status", "extra"},
	}

	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
			err := runAuthErr(t, args...)
			if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthMutationsRejectInvalidProfileName(t *testing.T) {
	for _, action := range []string{"remove", "use"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			cmdtest.SaveConfig(t, apiURL, apiKey)

			var stdout, stderr bytes.Buffer
			cmd := newRootCmd()
			cmd.SetIn(strings.NewReader("yes\n"))
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"auth", action, "has space"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), `invalid profile name "has space"`) {
				t.Fatalf("expected invalid profile name error, got %v", err)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected output or prompt: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("reload config: %v", err)
			}
			if cfg.CurrentProfile != cmdtest.Profile || len(cfg.Profiles) != 1 || cfg.Profiles[cmdtest.Profile] != (config.Profile{APIURL: apiURL, APIKey: apiKey}) {
				t.Fatalf("profile or selection changed: %#v", cfg)
			}
		})
	}
}

func TestAuthMutationsRejectMalformedConfigBeforePrompt(t *testing.T) {
	for _, action := range []string{"add", "remove"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			path, err := config.Path()
			if err != nil {
				t.Fatalf("config path: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			malformed := []byte("profiles: [\n")
			if err := os.WriteFile(path, malformed, 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			var stdout, stderr bytes.Buffer
			cmd := newRootCmd()
			cmd.SetIn(strings.NewReader("yes\nhttps://example.com\n"))
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"auth", action, "prod"})
			err = cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "read config") || !strings.Contains(err.Error(), path) {
				t.Fatalf("expected config read error identifying %q, got %v", path, err)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected output or prompt: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			if !bytes.Equal(got, malformed) {
				t.Fatalf("malformed config changed: got %q, want %q", got, malformed)
			}
		})
	}
}

func TestAuthReadCommandsRejectInvalidOutput(t *testing.T) {
	for _, action := range []string{"list", "status"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, apiKey)

			var stdout, stderr bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"auth", action, "--output", "xml"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), `invalid output "xml"`) {
				t.Fatalf("expected invalid output error, got %v", err)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected result output: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("made %d API requests, want none", got)
			}
		})
	}
}

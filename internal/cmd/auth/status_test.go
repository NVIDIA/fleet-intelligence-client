// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

func TestAuthStatusChecksConnectionAndDoesNotPrintSecret(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "Profile: "+cmdtest.Profile) {
		t.Fatalf("status missing profile: %q", got)
	}
	if !strings.Contains(got, "API URL: "+server.URL) {
		t.Fatalf("status missing API URL: %q", got)
	}
	if !strings.Contains(got, "API key: configured") {
		t.Fatalf("status missing API key state: %q", got)
	}
	if !strings.Contains(got, "Connection: ok") {
		t.Fatalf("status missing connection state: %q", got)
	}
	if strings.Contains(got, apiKey) {
		t.Fatalf("status printed secret: %q", got)
	}
}

// `auth status` is the command that explains credential resolution, so it has
// to run — and say what happened — when current_profile points at nothing.
func TestAuthStatusReportsGoneCurrentProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	if err := config.Save(config.Config{CurrentProfile: "gone"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	t.Setenv(config.EnvAPIURL, server.URL)
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, `current profile "gone" is no longer stored`) {
		t.Fatalf("status did not report the stale selection: %q", got)
	}
	if !strings.Contains(got, "Connection: ok") {
		t.Fatalf("status did not fall back to the environment: %q", got)
	}
}

// An unreadable config that resolution recovered from must not look like an
// absent one: `auth list` fails on the same file, so hiding it here is worse
// than useless.
func TestAuthStatusReportsUnreadableConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	path, err := config.Path()
	if err != nil {
		t.Fatalf("config path failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles: [\n"), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "read config") {
		t.Fatalf("status hid the unreadable config: %q", got)
	}
	if !strings.Contains(got, "API key: configured (from environment)") {
		t.Fatalf("status did not resolve from the environment: %q", got)
	}
}

// `auth status --profile` must report the named profile, not the current one.
func TestAuthStatusChecksNamedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustAddProfile(t, "prod", "prod-key", "")
	mustAddProfile(t, "dev", apiKey, server.URL)

	got := mustRunAuth(t, "status", "--profile", "dev")
	if !strings.Contains(got, "Profile: dev") {
		t.Fatalf("status checked the wrong profile: %q", got)
	}
	if !strings.Contains(got, "Connection: ok") {
		t.Fatalf("status missing connection state: %q", got)
	}
}

// An explicit --profile ignores the credential environment, and says so.
func TestAuthStatusReportsIgnoredEnvironment(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustAddProfile(t, "dev", apiKey, server.URL)
	t.Setenv(config.EnvAPIKey, "stale-key")

	got := mustRunAuth(t, "status", "--profile", "dev")
	if !strings.Contains(got, "Connection: ok") {
		t.Fatalf("environment leaked into an explicit profile: %q", got)
	}
	if !strings.Contains(got, "ignored because a profile was selected explicitly") {
		t.Fatalf("status did not report the ignored environment: %q", got)
	}
}

// The ignored-environment note must name only the variables actually set.
// Listing both when one is set sends the user hunting for an export that isn't
// there.
func TestAuthStatusIgnoredEnvironmentNamesOnlySetVariables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustAddProfile(t, "dev", apiKey, server.URL)
	t.Setenv(config.EnvAPIKey, "stale-key")

	got := mustRunAuth(t, "status", "--profile", "dev")
	if !strings.Contains(got, config.EnvAPIKey+" is set but ignored") {
		t.Fatalf("expected a singular note naming only the set variable: %q", got)
	}
	if strings.Contains(got, config.EnvAPIURL) {
		t.Fatalf("status named an environment variable that was never set: %q", got)
	}
}

// Same for the shadowing note: one variable reads "overrides the value".
func TestAuthStatusShadowedProfileNamesOnlySetVariables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "stored-key")
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, config.EnvAPIKey+" overrides the value stored in profile") {
		t.Fatalf("expected a singular shadowing note: %q", got)
	}
	if strings.Contains(got, config.EnvAPIURL) {
		t.Fatalf("status named an environment variable that was never set: %q", got)
	}
}

// Without an explicit --profile the environment still wins, so status has to
// name the profile it is shadowing or the override is undiagnosable.
func TestAuthStatusReportsShadowedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	cmdtest.SaveConfig(t, "https://stored.example.com", "stored-key")
	t.Setenv(config.EnvAPIURL, server.URL)
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "API URL: "+server.URL+" (from environment)") {
		t.Fatalf("expected the environment to win: %q", got)
	}
	if !strings.Contains(got, config.EnvAPIURL) || !strings.Contains(got, config.EnvAPIKey) {
		t.Fatalf("status did not name the shadowing variables: %q", got)
	}
	if !strings.Contains(got, `profile "`+cmdtest.Profile+`"`) {
		t.Fatalf("status did not name the shadowed profile: %q", got)
	}
}

func TestAuthStatusRejectsUnknownProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	err := runAuthErr(t, "status", "--profile", "nope")
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthStatusWithoutConfigExitsZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "API URL: "+config.DefaultAPIURL) {
		t.Fatalf("status missing default API URL: %q", got)
	}
	if !strings.Contains(got, "API key: not configured") {
		t.Fatalf("status missing not configured state: %q", got)
	}
	if !strings.Contains(got, "Profile: none") {
		t.Fatalf("status should report no profile: %q", got)
	}
}

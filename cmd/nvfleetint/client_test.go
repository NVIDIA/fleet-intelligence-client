// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// testCommonFlags builds the resolved flags a command would hand to
// newConfiguredClient, optionally selecting a profile explicitly.
func testCommonFlags(profile string) resolvedCommonFlags {
	return resolvedCommonFlags{
		output:     "table",
		timeout:    nvfleetint.DefaultTimeout,
		profile:    profile,
		profileSet: profile != "",
	}
}

// Verifies the current profile is converted to an SDK client
func TestNewConfiguredClientBuildsSDKClient(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveTestConfig(t, "https://fleet.example.com", "test-key")

	client, err := newConfiguredClient(testCommonFlags(""))
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://fleet.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}
	if !client.APIKeyConfigured() {
		t.Fatal("expected API key to be configured")
	}
}

// Verifies --profile picks the named profile rather than the current one
func TestNewConfiguredClientUsesNamedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	var cfg config.Config
	for name, url := range map[string]string{
		"prod": "https://prod.example.com",
		"dev":  "https://dev.example.com",
	} {
		if err := cfg.AddProfile(name, config.Profile{APIURL: url, APIKey: name + "-key"}); err != nil {
			t.Fatalf("add %s failed: %v", name, err)
		}
	}
	if err := cfg.UseProfile("prod"); err != nil {
		t.Fatalf("use failed: %v", err)
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	client, err := newConfiguredClient(testCommonFlags("dev"))
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://dev.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}
}

// Verifies an unknown --profile fails with a discoverable hint
func TestNewConfiguredClientRejectsUnknownProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveTestConfig(t, "https://fleet.example.com", "test-key")

	_, err := newConfiguredClient(testCommonFlags("nope"))
	if !errors.Is(err, config.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth list") {
		t.Fatalf("expected the error to point at `auth list`, got %v", err)
	}
}

// Verifies a keyless profile fails clearly
func TestNewConfiguredClientRequiresAPIKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveTestConfig(t, "https://fleet.example.com", "")

	_, err := newConfiguredClient(testCommonFlags(""))
	if err == nil {
		t.Fatal("expected API key error")
	}
	if !strings.Contains(err.Error(), "has no API key") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "auth add "+testProfile) {
		t.Fatalf("expected the error to name the profile to fix, got %v", err)
	}
}

// Verifies the no-config case names the command that fixes it
func TestNewConfiguredClientRequiresAProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	_, err := newConfiguredClient(testCommonFlags(""))
	if !errors.Is(err, config.ErrNoProfile) {
		t.Fatalf("expected ErrNoProfile, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth add <name>") {
		t.Fatalf("expected the error to point at `auth add`, got %v", err)
	}
}

func TestNewConfiguredClientSuggestsUseWhenProfilesExist(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	cfg := config.Config{
		Profiles: map[string]config.Profile{
			"dev":  {APIKey: "dev-key"},
			"prod": {APIKey: "prod-key"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	_, err := newConfiguredClient(testCommonFlags(""))
	if !errors.Is(err, config.ErrNoProfile) {
		t.Fatalf("expected ErrNoProfile, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth use <name>") {
		t.Fatalf("expected the error to point at `auth use`, got %v", err)
	}
	if strings.Contains(err.Error(), "auth add") {
		t.Fatalf("error should not ask for a duplicate key: %v", err)
	}
}

// A current_profile whose profile was removed must not lock the user out when
// the environment carries a working key.
func TestNewConfiguredClientFallsBackWhenCurrentProfileIsGone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	if err := config.Save(config.Config{CurrentProfile: "gone"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	t.Setenv(config.EnvAPIURL, "https://env-fleet.example.com")
	t.Setenv(config.EnvAPIKey, "env-test-key")

	client, err := newConfiguredClient(testCommonFlags(""))
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://env-fleet.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}
}

// Without an environment key there is nothing to fall back to, so the error
// has to name the stale selection and the command that clears it.
func TestNewConfiguredClientNamesGoneCurrentProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	if err := config.Save(config.Config{CurrentProfile: "gone"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	_, err := newConfiguredClient(testCommonFlags(""))
	if !errors.Is(err, config.ErrNoProfile) {
		t.Fatalf("expected ErrNoProfile, got %v", err)
	}
	if !strings.Contains(err.Error(), `"gone"`) {
		t.Fatalf("expected the error to name the stale profile, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth use <name>") {
		t.Fatalf("expected the error to point at `auth use`, got %v", err)
	}
}

// NVFLEETINT_SERVICE_KEY was renamed; a job that still exports it otherwise
// fails as if it had set no credentials at all.
func TestNewConfiguredClientNamesRenamedEnvironmentVariable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)
	t.Setenv(config.EnvLegacyAPIKey, "legacy-key")

	_, err := newConfiguredClient(testCommonFlags(""))
	if !errors.Is(err, config.ErrNoProfile) {
		t.Fatalf("expected ErrNoProfile, got %v", err)
	}
	if !strings.Contains(err.Error(), config.EnvLegacyAPIKey) {
		t.Fatalf("expected the error to name %s, got %v", config.EnvLegacyAPIKey, err)
	}
	if !strings.Contains(err.Error(), config.EnvAPIKey) {
		t.Fatalf("expected the error to name the new variable, got %v", err)
	}
}

// Once the new variable is set the old one is leftovers, not the cause.
func TestNewConfiguredClientOmitsRenameHintWhenNewVariableIsSet(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveTestConfig(t, "https://fleet.example.com", "")
	t.Setenv(config.EnvLegacyAPIKey, "legacy-key")
	t.Setenv(config.EnvAPIKey, "")

	// A profile with no key, so the error path runs while the legacy note
	// stays suppressed by the profile's own remedy.
	_, err := newConfiguredClient(testCommonFlags(""))
	if err == nil {
		t.Fatal("expected API key error")
	}
	if !strings.Contains(err.Error(), config.EnvLegacyAPIKey) {
		t.Fatalf("expected the rename hint while %s is unset, got %v", config.EnvAPIKey, err)
	}

	t.Setenv(config.EnvAPIKey, "env-test-key")
	if note := legacyAPIKeyEnvNote(); note != "" {
		t.Fatalf("expected no rename note once %s is set, got %q", config.EnvAPIKey, note)
	}
}

func TestNewConfiguredClientUsesEnvFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.EnvAPIURL, "https://env-fleet.example.com")
	t.Setenv(config.EnvAPIKey, "env-test-key")

	client, err := newConfiguredClient(testCommonFlags(""))
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://env-fleet.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}
	if !client.APIKeyConfigured() {
		t.Fatal("expected API key to be configured")
	}
}

func TestNewConfiguredClientEnvOverridesCurrentProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveTestConfig(t, "https://file-fleet.example.com", "file-test-key")
	t.Setenv(config.EnvAPIURL, "https://env-fleet.example.com")
	t.Setenv(config.EnvAPIKey, "env-test-key")

	client, err := newConfiguredClient(testCommonFlags(""))
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://env-fleet.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}
}

// An explicitly named profile must not be contaminated by a stale environment
// override: that is how one tenant's key would reach another tenant's endpoint.
func TestNewConfiguredClientIgnoresEnvForNamedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveTestConfig(t, "https://file-fleet.example.com", "file-test-key")
	t.Setenv(config.EnvAPIURL, "https://env-fleet.example.com")
	t.Setenv(config.EnvAPIKey, "env-test-key")

	client, err := newConfiguredClient(testCommonFlags(testProfile))
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://file-fleet.example.com" {
		t.Fatalf("environment leaked into an explicit profile: %q", client.BaseURL())
	}
}

// --profile selects a profile other than the stored current one.
func TestNewConfiguredClientHonorsProfileFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	var cfg config.Config
	if err := cfg.AddProfile("prod", config.Profile{APIURL: "https://prod.example.com", APIKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", config.Profile{APIURL: "https://dev.example.com", APIKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	// AddProfile selected "prod" as current; the flag must override that.
	client, err := newConfiguredClient(testCommonFlags("dev"))
	if err != nil {
		t.Fatalf("new configured client failed: %v", err)
	}
	if client.BaseURL() != "https://dev.example.com" {
		t.Fatalf("unexpected base URL: %q", client.BaseURL())
	}
}

// The env override never passes through `auth add`, so this is the path that
// would otherwise smuggle a plaintext endpoint past validation.
func TestNewConfiguredClientRejectsInsecureEnvAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(config.EnvAPIURL, "http://evil.example.com")
	t.Setenv(config.EnvAPIKey, "env-test-key")

	_, err := newConfiguredClient(testCommonFlags(""))
	if err == nil {
		t.Fatal("expected insecure URL error")
	}
	if !errors.Is(err, nvfleetint.ErrInsecureBaseURL) {
		t.Fatalf("expected ErrInsecureBaseURL, got %v", err)
	}
	// The message must name where the bad value came from.
	if !strings.Contains(err.Error(), config.EnvAPIURL) {
		t.Fatalf("expected error to mention %s, got %v", config.EnvAPIURL, err)
	}
}

// A hand-edited config file bypasses `auth add` too.
func TestNewConfiguredClientRejectsInsecureProfileAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveTestConfig(t, "http://evil.example.com", "test-key")

	_, err := newConfiguredClient(testCommonFlags(""))
	if !errors.Is(err, nvfleetint.ErrInsecureBaseURL) {
		t.Fatalf("expected ErrInsecureBaseURL, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth add "+testProfile) {
		t.Fatalf("expected the error to name the profile to fix, got %v", err)
	}
}

// Verifies config load failures are returned
func TestNewConfiguredClientReturnsLoadError(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	clearCredentialEnv(t)

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles: [oops\n"), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	_, err := newConfiguredClient(testCommonFlags(""))
	if err == nil {
		t.Fatal("expected config load error")
	}
}

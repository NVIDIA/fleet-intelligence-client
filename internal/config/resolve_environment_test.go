// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveEnvironmentOnlySurvivesMalformedConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clearEnv(t)
	t.Setenv(EnvAPIURL, "https://env.example.com")
	t.Setenv(EnvAPIKey, "env-key")

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles: [oops\n"), fileMode); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	resolved, err := Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.APIURL != "https://env.example.com" || resolved.APIURLSource != SourceEnvironment {
		t.Fatalf("unexpected API URL resolution: %#v", resolved)
	}
	if resolved.APIKey != "env-key" || resolved.APIKeySource != SourceEnvironment {
		t.Fatalf("unexpected API key resolution: %#v", resolved)
	}
}

func TestResolveExplicitProfileIgnoresEnvironment(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAPIURL, "https://env.example.com")
	t.Setenv(EnvAPIKey, "env-key")

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", APIKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIURL: "https://dev.example.com", APIKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}

	resolved, err := cfg.Resolve("dev")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Profile != "dev" {
		t.Fatalf("unexpected profile: %q", resolved.Profile)
	}
	if resolved.APIURL != "https://dev.example.com" || resolved.APIKey != "dev-key" {
		t.Fatalf("environment leaked into an explicit profile: %#v", resolved)
	}
	if resolved.APIURLSource != SourceProfile || resolved.APIKeySource != SourceProfile {
		t.Fatalf("unexpected sources: %#v", resolved)
	}
	if len(resolved.EnvIgnored) != 2 {
		t.Fatalf("expected EnvIgnored to name both skipped variables, got %#v", resolved.EnvIgnored)
	}
}

func TestResolveCurrentProfileWithEnvOverlay(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAPIKey, "env-key")

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", APIKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}

	resolved, err := cfg.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Profile != "prod" {
		t.Fatalf("unexpected profile: %q", resolved.Profile)
	}
	if resolved.APIURL != "https://prod.example.com" || resolved.APIURLSource != SourceProfile {
		t.Fatalf("expected the profile URL to survive: %#v", resolved)
	}
	if resolved.APIKey != "env-key" || resolved.APIKeySource != SourceEnvironment {
		t.Fatalf("expected the environment key to win: %#v", resolved)
	}
	if len(resolved.EnvIgnored) != 0 {
		t.Fatalf("environment credentials were applied, not ignored: %#v", resolved.EnvIgnored)
	}
}

func TestResolveEnvironmentOnly(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAPIKey, "env-key")

	resolved, err := Config{}.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Profile != "" {
		t.Fatalf("expected no profile, got %q", resolved.Profile)
	}
	if resolved.APIKey != "env-key" || resolved.APIKeySource != SourceEnvironment {
		t.Fatalf("unexpected key: %#v", resolved)
	}
	if resolved.APIURL != DefaultAPIURL || resolved.APIURLSource != SourceDefault {
		t.Fatalf("expected the default API URL: %#v", resolved)
	}
}

func TestResolveDanglingCurrentProfileFallsBackToEnvironment(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAPIKey, "env-key")
	t.Setenv(EnvAPIURL, "https://env.example.com")

	cfg := Config{CurrentProfile: "gone"}
	resolved, err := cfg.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.MissingCurrentProfile != "gone" {
		t.Fatalf("expected MissingCurrentProfile %q, got %q", "gone", resolved.MissingCurrentProfile)
	}
	if resolved.APIKey != "env-key" || resolved.APIKeySource != SourceEnvironment {
		t.Fatalf("expected the environment key, got %q from %q", resolved.APIKey, resolved.APIKeySource)
	}
	if resolved.APIURL != "https://env.example.com" || resolved.APIURLSource != SourceEnvironment {
		t.Fatalf("expected the environment URL, got %q from %q", resolved.APIURL, resolved.APIURLSource)
	}
}

// A corrupt file with a usable environment resolves, but must say so: it is
// otherwise indistinguishable from having no config at all.
func TestResolveReportsUnreadableConfigWhenEnvironmentSucceeds(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	t.Setenv(EnvAPIKey, "env-key")

	path, err := Path()
	if err != nil {
		t.Fatalf("path failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles: [\n"), fileMode); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	resolved, err := Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.ConfigError == nil {
		t.Fatal("expected the config read error to be reported")
	}
	if !strings.Contains(resolved.ConfigError.Error(), "read config") {
		t.Fatalf("expected a read config error, got %v", resolved.ConfigError)
	}
	if resolved.APIKey != "env-key" {
		t.Fatalf("expected the environment key, got %q", resolved.APIKey)
	}
}

// Without an environment key there is nothing to fall back to, so the read
// failure must stay a failure rather than become an empty credential set.
func TestResolveUnreadableConfigFailsWithoutEnvironmentKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	path, err := Path()
	if err != nil {
		t.Fatalf("path failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles: [\n"), fileMode); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := Resolve(""); err == nil {
		t.Fatal("expected an error for an unreadable config")
	}
}

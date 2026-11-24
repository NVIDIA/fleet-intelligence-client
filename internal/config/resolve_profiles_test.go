// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"os"
	"testing"
)

func TestResolveBlankProfileAPIURLReportsDefaultSource(t *testing.T) {
	clearEnv(t)

	cfg := Config{
		CurrentProfile: "staging",
		Profiles: map[string]Profile{
			"staging": {APIKey: "staging-key"},
		},
	}

	resolved, err := cfg.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.APIURL != DefaultAPIURL {
		t.Fatalf("unexpected API URL: %q", resolved.APIURL)
	}
	if resolved.APIURLSource != SourceDefault {
		t.Fatalf("expected default source, got %#v", resolved)
	}
}

// The profile flag selects a profile other than the stored current one.
func TestResolveFlagBeatsCurrentProfile(t *testing.T) {
	clearEnv(t)

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", APIKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIURL: "https://dev.example.com", APIKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}

	// AddProfile selected "prod" as current; the flag must override that.
	resolved, err := cfg.Resolve("dev")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.Profile != "dev" || resolved.APIKey != "dev-key" {
		t.Fatalf("expected the flag to win, got %#v", resolved)
	}
}

func TestResolveWithoutAnythingConfigured(t *testing.T) {
	clearEnv(t)

	resolved, err := Config{}.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.APIKey != "" {
		t.Fatalf("expected no API key, got %q", resolved.APIKey)
	}
	if resolved.APIURL != DefaultAPIURL {
		t.Fatalf("expected the default API URL, got %q", resolved.APIURL)
	}
}

func TestResolveUnknownProfileFails(t *testing.T) {
	clearEnv(t)

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	if _, err := cfg.Resolve("nope"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

// A selection left behind in the file must not fail the command: nobody named
// that profile, and failing takes down `auth status` too.
func TestResolveDanglingCurrentProfileReportsInsteadOfFailing(t *testing.T) {
	clearEnv(t)

	cfg := Config{CurrentProfile: "gone"}
	resolved, err := cfg.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.MissingCurrentProfile != "gone" {
		t.Fatalf("expected MissingCurrentProfile %q, got %q", "gone", resolved.MissingCurrentProfile)
	}
	if resolved.Profile != "" {
		t.Fatalf("expected no resolved profile, got %q", resolved.Profile)
	}
	if resolved.APIKey != "" {
		t.Fatalf("expected no API key, got %q", resolved.APIKey)
	}
}

// An explicitly named profile still fails — the user named it, so silently
// using something else is the wrong recovery.
func TestResolveExplicitMissingProfileStillFails(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAPIKey, "env-key")

	cfg := Config{CurrentProfile: "gone"}
	if _, err := cfg.Resolve("nope"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestResolveLoadsFromDisk(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", APIKey: "prod-key"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	resolved, err := Resolve("prod")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.APIKey != "prod-key" {
		t.Fatalf("unexpected key: %q", resolved.APIKey)
	}
}

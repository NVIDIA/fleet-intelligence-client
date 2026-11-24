// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

func TestAuthAddStoresProfileAndSelectsIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	got := mustAddProfile(t, "prod", apiKey, "")
	if !strings.Contains(got, `Profile "prod" added.`) {
		t.Fatalf("unexpected output: %q", got)
	}
	if !strings.Contains(got, "is now the current profile") {
		t.Fatalf("expected the first profile to be selected: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.CurrentProfile != "prod" {
		t.Fatalf("unexpected current profile: %q", cfg.CurrentProfile)
	}
	profile := cfg.Profiles["prod"]
	if profile.APIURL != config.DefaultAPIURL {
		t.Fatalf("unexpected API URL: %q", profile.APIURL)
	}
	if profile.APIKey != apiKey {
		t.Fatalf("unexpected API key: %q", profile.APIKey)
	}
}

func TestAuthAddSavesExplicitAPIURLAndKeepsCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, "")
	got := mustAddProfile(t, "dev", "dev-key", apiURL)
	if strings.Contains(got, "is now the current profile") {
		t.Fatalf("adding a second profile must not steal the selection: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.CurrentProfile != "prod" {
		t.Fatalf("unexpected current profile: %q", cfg.CurrentProfile)
	}
	if cfg.Profiles["dev"].APIURL != apiURL {
		t.Fatalf("unexpected API URL: %q", cfg.Profiles["dev"].APIURL)
	}
}

// `add` on an existing name changes it in place — this is the key-rotation
// path. The output has to say "updated", since the name is the only thing
// distinguishing a new profile from an overwritten one.
func TestAuthAddUpdatesExistingProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, apiURL)

	got := mustAddProfile(t, "prod", "rotated-key", "", "--yes")
	if !strings.Contains(got, `Profile "prod" updated.`) {
		t.Fatalf("expected an overwrite to be reported as an update: %q", got)
	}
	if strings.Contains(got, "added") {
		t.Fatalf("an existing profile must not report as added: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	profile := cfg.Profiles["prod"]
	if profile.APIKey != "rotated-key" {
		t.Fatalf("unexpected API key: %q", profile.APIKey)
	}
	// An omitted --api-url must not reset the endpoint to the default.
	if profile.APIURL != apiURL {
		t.Fatalf("rotating a key wiped the custom API URL: %q", profile.APIURL)
	}
}

// Keeping both offered values is a legitimate answer, not a failure: it is what
// pressing Enter twice does. Nothing is written and nothing is destroyed.
func TestAuthAddReportsAnUnchangedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, apiURL)

	got := mustAddProfile(t, "prod", "", "")
	if !strings.Contains(got, `Profile "prod" unchanged.`) {
		t.Fatalf("unexpected output: %q", got)
	}
	if strings.Contains(got, "updated") {
		t.Fatalf("a no-op must not report a write: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got := cfg.Profiles["prod"]; got.APIKey != apiKey || got.APIURL != apiURL {
		t.Fatalf("a no-op must not disturb the profile: %#v", got)
	}
}

// Updating a profile must not steal the current-profile selection.
func TestAuthAddExistingProfileKeepsCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, "")
	mustAddProfile(t, "dev", "dev-key", "")

	got := mustAddProfile(t, "dev", "rotated-key", "", "--yes")
	if strings.Contains(got, "is now the current profile") {
		t.Fatalf("updating a profile must not change the selection: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.CurrentProfile != "prod" {
		t.Fatalf("unexpected current profile: %q", cfg.CurrentProfile)
	}
}

// Omitting the name is the single-tenant path: it targets a profile called
// "default" so first-time setup is one command with nothing to invent.
func TestAuthAddWithoutNameUsesDefaultProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	got := mustAddProfile(t, "", apiKey, "")
	if !strings.Contains(got, `Profile "`+config.DefaultProfileName+`" added.`) {
		t.Fatalf("unexpected output: %q", got)
	}
	if !strings.Contains(got, "is now the current profile") {
		t.Fatalf("expected the first profile to be selected: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.CurrentProfile != config.DefaultProfileName {
		t.Fatalf("unexpected current profile: %q", cfg.CurrentProfile)
	}
	profile := cfg.Profiles[config.DefaultProfileName]
	if profile.APIKey != apiKey {
		t.Fatalf("unexpected API key: %q", profile.APIKey)
	}
	if profile.APIURL != config.DefaultAPIURL {
		t.Fatalf("unexpected API URL: %q", profile.APIURL)
	}
}

// The unnamed form is an upsert too, so it is also how a single-tenant user
// rotates a key.
func TestAuthAddWithoutNameUpdatesDefaultProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "", apiKey, apiURL)

	got := mustAddProfile(t, "", "rotated-key", "", "--yes")
	if !strings.Contains(got, `Profile "`+config.DefaultProfileName+`" updated.`) {
		t.Fatalf("expected an update: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	profile := cfg.Profiles[config.DefaultProfileName]
	if profile.APIKey != "rotated-key" {
		t.Fatalf("unexpected API key: %q", profile.APIKey)
	}
	if profile.APIURL != apiURL {
		t.Fatalf("rotating a key wiped the custom API URL: %q", profile.APIURL)
	}
}

// Loopback stays usable so the local mock-server workflow keeps working.
func TestAuthAddAcceptsLoopbackHTTPAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	const loopbackURL = "http://127.0.0.1:9999"
	mustAddProfile(t, "local", apiKey, loopbackURL)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Profiles["local"].APIURL != loopbackURL {
		t.Fatalf("unexpected API URL: %q", cfg.Profiles["local"].APIURL)
	}
}

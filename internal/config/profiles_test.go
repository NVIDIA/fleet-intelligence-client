// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestAddProfileRejectsDuplicate(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	err := cfg.AddProfile("prod", Profile{APIKey: "b"})
	if !errors.Is(err, ErrProfileExists) {
		t.Fatalf("expected ErrProfileExists, got %v", err)
	}
	if got := cfg.Profiles["prod"].APIKey; got != "a" {
		t.Fatalf("expected the stored profile to be untouched, got %q", got)
	}
}

func TestAddProfileDefaultsAPIURL(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	if got := cfg.Profiles["prod"].APIURL; got != DefaultAPIURL {
		t.Fatalf("unexpected API URL: got %q want %q", got, DefaultAPIURL)
	}
}

func TestAddProfileTrimsName(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile(" dev ", Profile{APIKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if _, ok := cfg.Profiles["dev"]; !ok {
		t.Fatalf("expected trimmed profile name, got %#v", cfg.Profiles)
	}
	if cfg.CurrentProfile != "dev" {
		t.Fatalf("expected trimmed current profile, got %q", cfg.CurrentProfile)
	}
}

func TestAddProfileKeepsExistingCurrent(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIKey: "a"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIKey: "b"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}

	if cfg.CurrentProfile != "prod" {
		t.Fatalf("expected prod to stay current, got %q", cfg.CurrentProfile)
	}
}

func TestAddProfileRepairsDanglingCurrent(t *testing.T) {
	cfg := Config{
		CurrentProfile: "gone",
		Profiles: map[string]Profile{
			"dev": {APIKey: "dev-key"},
		},
	}

	if err := cfg.AddProfile("prod", Profile{APIKey: "prod-key"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if cfg.CurrentProfile != "prod" {
		t.Fatalf("expected added profile to repair dangling current profile, got %q", cfg.CurrentProfile)
	}
}

func TestUpdateProfileRequiresExisting(t *testing.T) {
	var cfg Config

	err := cfg.UpdateProfile("prod", Profile{APIKey: "a"})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}

	if err := cfg.AddProfile("prod", Profile{APIKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := cfg.UpdateProfile("prod", Profile{APIURL: "https://new.example.com", APIKey: "b"}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := cfg.Profiles["prod"]; got.APIKey != "b" || got.APIURL != "https://new.example.com" {
		t.Fatalf("unexpected profile after update: %#v", got)
	}
}

func TestRemoveProfileClearsCurrent(t *testing.T) {
	t.Run("last profile clears current", func(t *testing.T) {
		var cfg Config
		if err := cfg.AddProfile("prod", Profile{APIKey: "a"}); err != nil {
			t.Fatalf("add failed: %v", err)
		}
		if err := cfg.RemoveProfile("prod"); err != nil {
			t.Fatalf("remove failed: %v", err)
		}
		if cfg.CurrentProfile != "" {
			t.Fatalf("expected current profile to be cleared, got %q", cfg.CurrentProfile)
		}
	})

	t.Run("single survivor is not auto-selected", func(t *testing.T) {
		var cfg Config
		if err := cfg.AddProfile("prod", Profile{APIKey: "a"}); err != nil {
			t.Fatalf("add prod failed: %v", err)
		}
		if err := cfg.AddProfile("dev", Profile{APIKey: "b"}); err != nil {
			t.Fatalf("add dev failed: %v", err)
		}
		if err := cfg.RemoveProfile("prod"); err != nil {
			t.Fatalf("remove failed: %v", err)
		}
		if cfg.CurrentProfile != "" {
			t.Fatalf("expected current profile to be cleared, got %q", cfg.CurrentProfile)
		}
	})

	t.Run("removing a non-current profile keeps the selection", func(t *testing.T) {
		var cfg Config
		for _, name := range []string{"prod", "dev", "qa"} {
			if err := cfg.AddProfile(name, Profile{APIKey: "k"}); err != nil {
				t.Fatalf("add %s failed: %v", name, err)
			}
		}
		if err := cfg.RemoveProfile("dev"); err != nil {
			t.Fatalf("remove failed: %v", err)
		}
		if cfg.CurrentProfile != "prod" {
			t.Fatalf("expected prod to stay current, got %q", cfg.CurrentProfile)
		}
	})

	t.Run("unknown profile", func(t *testing.T) {
		var cfg Config
		if err := cfg.RemoveProfile("nope"); !errors.Is(err, ErrProfileNotFound) {
			t.Fatalf("expected ErrProfileNotFound, got %v", err)
		}
	})
}

func TestUseProfile(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIKey: "a"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIKey: "b"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}

	if err := cfg.UseProfile("dev"); err != nil {
		t.Fatalf("use failed: %v", err)
	}
	if cfg.CurrentProfile != "dev" {
		t.Fatalf("expected dev to be current, got %q", cfg.CurrentProfile)
	}
	if err := cfg.UseProfile("nope"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestProfileNamesAreSorted(t *testing.T) {
	var cfg Config
	for _, name := range []string{"zeta", "alpha", "m1"} {
		if err := cfg.AddProfile(name, Profile{APIKey: "k"}); err != nil {
			t.Fatalf("add %s failed: %v", name, err)
		}
	}

	want := []string{"alpha", "m1", "zeta"}
	if got := cfg.ProfileNames(); !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected names: got %v want %v", got, want)
	}
}

func TestValidateProfileName(t *testing.T) {
	valid := []string{"prod", "dev-2", "us_west", "tenant.a", "A1", " dev "}
	for _, name := range valid {
		if err := ValidateProfileName(name); err != nil {
			t.Errorf("expected %q to be valid: %v", name, err)
		}
	}

	invalid := []string{
		"", "   ", "has space", "slash/name", "quote\"name",
		// Must start with a letter or digit, so a name is never mistaken for a
		// flag or a hidden file.
		"-lead", "_lead", ".lead",
		// `none` is what `auth status` prints when no profile is in use.
		"none", "NONE",
		strings.Repeat("a", maxProfileNameLength+1),
	}
	for _, name := range invalid {
		if err := ValidateProfileName(name); err == nil {
			t.Errorf("expected %q to be rejected", name)
		}
	}
}

func TestEditInvalidProfileNamesPreserveFile(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config) error
	}{
		{"add", func(cfg *Config) error { return cfg.AddProfile("bad name", Profile{APIKey: "replacement"}) }},
		{"update", func(cfg *Config) error { return cfg.UpdateProfile("bad name", Profile{APIKey: "replacement"}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			clearEnv(t)
			want := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
				"dev": {APIURL: "https://fleet.example.com", APIKey: "original-key"},
			}}
			if err := Save(want); err != nil {
				t.Fatalf("save: %v", err)
			}
			path, err := Path()
			if err != nil {
				t.Fatalf("path: %v", err)
			}
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read original: %v", err)
			}
			wantErr := ValidateProfileName("bad name")
			_, err = Edit(tt.mutate)
			if err == nil || wantErr == nil || err.Error() != wantErr.Error() {
				t.Fatalf("edit error: got %v, want %v", err, wantErr)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read after edit: %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatalf("config bytes changed: got %q, want %q", after, before)
			}
			loaded, err := Load()
			if err != nil || !reflect.DeepEqual(loaded, want) {
				t.Fatalf("config after edit: got %#v, want %#v, err=%v", loaded, want, err)
			}
			if _, err := os.Lstat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("lock remains or cannot be inspected: %v", err)
			}
		})
	}
}

func TestEditRemoveProfilesWithNoCurrentSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clearEnv(t)

	survivor := Profile{APIURL: "https://survivor.example.com", APIKey: "survivor-key"}
	initial := Config{CurrentProfile: "first", Profiles: map[string]Profile{
		"first":    {APIURL: "https://first.example.com", APIKey: "first-key"},
		"second":   {APIURL: "https://second.example.com", APIKey: "second-key"},
		"survivor": survivor,
	}}
	if err := Save(initial); err != nil {
		t.Fatalf("save initial config: %v", err)
	}
	if _, err := Edit(func(cfg *Config) error { return cfg.RemoveProfile("first") }); err != nil {
		t.Fatalf("remove current profile: %v", err)
	}
	if _, err := Edit(func(cfg *Config) error {
		if cfg.CurrentProfile != "" {
			t.Fatalf("current profile before second removal: got %q, want empty", cfg.CurrentProfile)
		}
		return cfg.RemoveProfile("second")
	}); err != nil {
		t.Fatalf("remove second profile: %v", err)
	}

	want := Config{Profiles: map[string]Profile{"survivor": survivor}}
	loaded, err := Load()
	if err != nil || !reflect.DeepEqual(loaded, want) {
		t.Fatalf("config after removals: got %#v, want %#v, err=%v", loaded, want, err)
	}
	resolved, err := Resolve("")
	if err != nil {
		t.Fatalf("resolve without selection: %v", err)
	}
	if resolved.Profile != "" || resolved.APIKey != "" || resolved.APIURL != DefaultAPIURL ||
		resolved.APIURLSource != SourceDefault || resolved.APIKeySource != SourceDefault || !resolved.ProfilesConfigured {
		t.Fatalf("resolve without selection: unexpected result %#v", resolved)
	}

	added := Profile{APIURL: "https://new.example.com", APIKey: "new-key"}
	if _, err := Edit(func(cfg *Config) error { return cfg.AddProfile("new", added) }); err != nil {
		t.Fatalf("add profile: %v", err)
	}
	want = Config{CurrentProfile: "new", Profiles: map[string]Profile{
		"survivor": survivor,
		"new":      added,
	}}
	loaded, err = Load()
	if err != nil || !reflect.DeepEqual(loaded, want) {
		t.Fatalf("config after adding profile: got %#v, want %#v, err=%v", loaded, want, err)
	}
}

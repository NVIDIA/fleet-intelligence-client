// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadMissingConfigReturnsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if cfg.CurrentProfile != "" {
		t.Fatalf("expected no current profile, got %q", cfg.CurrentProfile)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected no profiles, got %d", len(cfg.Profiles))
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var want Config
	if err := want.AddProfile("prod", Profile{APIURL: "https://fleet.example.com", APIKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := want.AddProfile("dev", Profile{APIURL: "https://dev.example.com", APIKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}

	if err := Save(want); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config mismatch: got %#v want %#v", got, want)
	}
	if got.CurrentProfile != "prod" {
		t.Fatalf("expected the first added profile to be current, got %q", got.CurrentProfile)
	}
}

func TestSaveUsesExpectedPathAndMode(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIKey: "test-key"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("path failed: %v", err)
	}
	if want := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml"); path != want {
		t.Fatalf("unexpected path: got %q want %q", path, want)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != fileMode {
		t.Fatalf("unexpected file mode: got %o want %o", perm, fileMode)
	}
}

// The file is rewritten on every auth mutation, so an unstable key order would
// churn it (and any diff a user keeps on it) for no reason.
func TestSaveIsDeterministic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var cfg Config
	for _, name := range []string{"zeta", "alpha", "prod", "dev", "m1"} {
		if err := cfg.AddProfile(name, Profile{APIURL: "https://" + name + ".example.com", APIKey: "key"}); err != nil {
			t.Fatalf("add %s failed: %v", name, err)
		}
	}

	path, err := Path()
	if err != nil {
		t.Fatalf("path failed: %v", err)
	}

	var first string
	for range 5 {
		if err := Save(cfg); err != nil {
			t.Fatalf("save failed: %v", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read failed: %v", err)
		}
		if first == "" {
			first = string(data)
			continue
		}
		if string(data) != first {
			t.Fatalf("config file is not written deterministically:\n%s\n---\n%s", first, data)
		}
	}

	if !strings.Contains(first, "current_profile: zeta") {
		t.Fatalf("expected the current profile in the file, got:\n%s", first)
	}
}

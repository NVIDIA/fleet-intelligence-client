// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// clearEnv removes the credential environment variables so a test starts from a
// known state regardless of the developer's shell.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvAPIURL, "")
	t.Setenv(EnvServiceKey, "")
	t.Setenv(EnvProfile, "")
}

func TestLoadMissingConfigReturnsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

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

	var want Config
	if err := want.AddProfile("prod", Profile{APIURL: "https://fleet.example.com", ServiceKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := want.AddProfile("dev", Profile{APIURL: "https://dev.example.com", ServiceKey: "dev-key"}); err != nil {
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

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{ServiceKey: "test-key"}); err != nil {
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

func TestSaveWritesThroughSymlinkedConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	target := filepath.Join(homeDir, "dotfiles", "nvfleetint.yaml")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("mkdir target dir failed: %v", err)
	}
	if err := os.WriteFile(target, []byte("profiles: {}\n"), fileMode); err != nil {
		t.Fatalf("write target failed: %v", err)
	}

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir config dir failed: %v", err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink failed: %v", err)
	}

	var cfg Config
	if err := cfg.AddProfile("dev", Profile{ServiceKey: "dev-key"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("lstat failed: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected config path to remain a symlink, got mode %s", info.Mode())
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target failed: %v", err)
	}
	if !strings.Contains(string(data), "dev-key") {
		t.Fatalf("symlink target was not updated:\n%s", data)
	}
}

func TestEditConcurrentAddsPreserveProfiles(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearEnv(t)

	const count = 20
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("p%d", i)
			_, err := Edit(func(cfg *Config) error {
				return cfg.AddProfile(name, Profile{ServiceKey: "key-" + name})
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("edit failed: %v", err)
		}
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.Profiles) != count {
		t.Fatalf("expected %d profiles, got %d: %#v", count, len(cfg.Profiles), cfg)
	}
}

// The file is rewritten on every auth mutation, so an unstable key order would
// churn it (and any diff a user keeps on it) for no reason.
func TestSaveIsDeterministic(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var cfg Config
	for _, name := range []string{"zeta", "alpha", "prod", "dev", "m1"} {
		if err := cfg.AddProfile(name, Profile{APIURL: "https://" + name + ".example.com", ServiceKey: "key"}); err != nil {
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

// Profiles landed before the first release, so the old flat config is
// deliberately not migrated: it decodes to zero profiles.
func TestLoadFlatLegacyConfigYieldsNoProfiles(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	legacy := "api_url: \"https://fleet.example.com\"\nservice_key: \"legacy-key\"\n"
	if err := os.WriteFile(path, []byte(legacy), fileMode); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.Profiles) != 0 || cfg.CurrentProfile != "" {
		t.Fatalf("expected legacy config to decode to nothing, got %#v", cfg)
	}
}

func TestLoadInvalidYAMLFails(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles: [oops\n"), fileMode); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("expected invalid YAML to fail")
	} else if !strings.Contains(err.Error(), path) {
		t.Fatalf("expected error to include path %q, got %v", path, err)
	}
}

func TestResolveEnvironmentOnlySurvivesMalformedConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	clearEnv(t)
	t.Setenv(EnvAPIURL, "https://env.example.com")
	t.Setenv(EnvServiceKey, "env-key")

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
	if resolved.ServiceKey != "env-key" || resolved.ServiceKeySource != SourceEnvironment {
		t.Fatalf("unexpected service key resolution: %#v", resolved)
	}
}

func TestAddProfileRejectsDuplicate(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile("prod", Profile{ServiceKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	err := cfg.AddProfile("prod", Profile{ServiceKey: "b"})
	if !errors.Is(err, ErrProfileExists) {
		t.Fatalf("expected ErrProfileExists, got %v", err)
	}
	if got := cfg.Profiles["prod"].ServiceKey; got != "a" {
		t.Fatalf("expected the stored profile to be untouched, got %q", got)
	}
}

func TestAddProfileDefaultsAPIURL(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile("prod", Profile{ServiceKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	if got := cfg.Profiles["prod"].APIURL; got != DefaultAPIURL {
		t.Fatalf("unexpected API URL: got %q want %q", got, DefaultAPIURL)
	}
}

func TestAddProfileTrimsName(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile(" dev ", Profile{ServiceKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if _, ok := cfg.Profiles["dev"]; !ok {
		t.Fatalf("expected trimmed profile name, got %#v", cfg.Profiles)
	}
	if cfg.CurrentProfile != "dev" {
		t.Fatalf("expected trimmed current profile, got %q", cfg.CurrentProfile)
	}
}

func TestResolveBlankProfileAPIURLReportsDefaultSource(t *testing.T) {
	clearEnv(t)

	cfg := Config{
		CurrentProfile: "staging",
		Profiles: map[string]Profile{
			"staging": {ServiceKey: "staging-key"},
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

func TestAddProfileKeepsExistingCurrent(t *testing.T) {
	var cfg Config
	if err := cfg.AddProfile("prod", Profile{ServiceKey: "a"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{ServiceKey: "b"}); err != nil {
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
			"dev": {ServiceKey: "dev-key"},
		},
	}

	if err := cfg.AddProfile("prod", Profile{ServiceKey: "prod-key"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if cfg.CurrentProfile != "prod" {
		t.Fatalf("expected added profile to repair dangling current profile, got %q", cfg.CurrentProfile)
	}
}

func TestUpdateProfileRequiresExisting(t *testing.T) {
	var cfg Config

	err := cfg.UpdateProfile("prod", Profile{ServiceKey: "a"})
	if !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}

	if err := cfg.AddProfile("prod", Profile{ServiceKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := cfg.UpdateProfile("prod", Profile{APIURL: "https://new.example.com", ServiceKey: "b"}); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if got := cfg.Profiles["prod"]; got.ServiceKey != "b" || got.APIURL != "https://new.example.com" {
		t.Fatalf("unexpected profile after update: %#v", got)
	}
}

func TestRemoveProfileClearsCurrent(t *testing.T) {
	t.Run("last profile clears current", func(t *testing.T) {
		var cfg Config
		if err := cfg.AddProfile("prod", Profile{ServiceKey: "a"}); err != nil {
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
		if err := cfg.AddProfile("prod", Profile{ServiceKey: "a"}); err != nil {
			t.Fatalf("add prod failed: %v", err)
		}
		if err := cfg.AddProfile("dev", Profile{ServiceKey: "b"}); err != nil {
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
			if err := cfg.AddProfile(name, Profile{ServiceKey: "k"}); err != nil {
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
	if err := cfg.AddProfile("prod", Profile{ServiceKey: "a"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{ServiceKey: "b"}); err != nil {
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
		if err := cfg.AddProfile(name, Profile{ServiceKey: "k"}); err != nil {
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

// The file holds every stored service key, so a failed write must not be able
// to leave a truncated or half-written config behind.
func TestSaveReplacesFileAtomically(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", ServiceKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIURL: "https://dev.example.com", ServiceKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("second save failed: %v", err)
	}

	// The rename must leave the directory holding exactly the config file: no
	// temp file with credentials in it survives a successful write.
	entries, err := os.ReadDir(filepath.Join(homeDir, ".config", "nvfleetint"))
	if err != nil {
		t.Fatalf("read dir failed: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != fileName {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("unexpected directory contents: %v", names)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(got.Profiles) != 2 {
		t.Fatalf("expected both profiles to survive the rewrite, got %#v", got)
	}
}

func TestResolveExplicitProfileIgnoresEnvironment(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvAPIURL, "https://env.example.com")
	t.Setenv(EnvServiceKey, "env-key")

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", ServiceKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIURL: "https://dev.example.com", ServiceKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}

	resolved, err := cfg.Resolve("dev")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Profile != "dev" {
		t.Fatalf("unexpected profile: %q", resolved.Profile)
	}
	if resolved.APIURL != "https://dev.example.com" || resolved.ServiceKey != "dev-key" {
		t.Fatalf("environment leaked into an explicit profile: %#v", resolved)
	}
	if resolved.APIURLSource != SourceProfile || resolved.ServiceKeySource != SourceProfile {
		t.Fatalf("unexpected sources: %#v", resolved)
	}
	if len(resolved.EnvIgnored) != 2 {
		t.Fatalf("expected EnvIgnored to name both skipped variables, got %#v", resolved.EnvIgnored)
	}
}

func TestResolveEnvProfileSelectsProfile(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvProfile, "dev")
	t.Setenv(EnvServiceKey, "env-key")

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", ServiceKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIURL: "https://dev.example.com", ServiceKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}

	resolved, err := cfg.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.Profile != "dev" || resolved.ServiceKey != "dev-key" {
		t.Fatalf("unexpected resolution: %#v", resolved)
	}
}

func TestResolveFlagBeatsEnvProfile(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvProfile, "dev")

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", ServiceKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIURL: "https://dev.example.com", ServiceKey: "dev-key"}); err != nil {
		t.Fatalf("add dev failed: %v", err)
	}

	resolved, err := cfg.Resolve("prod")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.Profile != "prod" {
		t.Fatalf("expected the flag to win, got %q", resolved.Profile)
	}
}

func TestResolveCurrentProfileWithEnvOverlay(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvServiceKey, "env-key")

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", ServiceKey: "prod-key"}); err != nil {
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
	if resolved.ServiceKey != "env-key" || resolved.ServiceKeySource != SourceEnvironment {
		t.Fatalf("expected the environment key to win: %#v", resolved)
	}
	if len(resolved.EnvIgnored) != 0 {
		t.Fatalf("environment credentials were applied, not ignored: %#v", resolved.EnvIgnored)
	}
}

func TestResolveEnvironmentOnly(t *testing.T) {
	clearEnv(t)
	t.Setenv(EnvServiceKey, "env-key")

	resolved, err := Config{}.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}

	if resolved.Profile != "" {
		t.Fatalf("expected no profile, got %q", resolved.Profile)
	}
	if resolved.ServiceKey != "env-key" || resolved.ServiceKeySource != SourceEnvironment {
		t.Fatalf("unexpected key: %#v", resolved)
	}
	if resolved.APIURL != DefaultAPIURL || resolved.APIURLSource != SourceDefault {
		t.Fatalf("expected the default API URL: %#v", resolved)
	}
}

func TestResolveWithoutAnythingConfigured(t *testing.T) {
	clearEnv(t)

	resolved, err := Config{}.Resolve("")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.ServiceKey != "" {
		t.Fatalf("expected no service key, got %q", resolved.ServiceKey)
	}
	if resolved.APIURL != DefaultAPIURL {
		t.Fatalf("expected the default API URL, got %q", resolved.APIURL)
	}
}

func TestResolveUnknownProfileFails(t *testing.T) {
	clearEnv(t)

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{ServiceKey: "a"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	if _, err := cfg.Resolve("nope"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestResolveDanglingCurrentProfileFails(t *testing.T) {
	clearEnv(t)

	cfg := Config{CurrentProfile: "gone"}
	if _, err := cfg.Resolve(""); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestResolveLoadsFromDisk(t *testing.T) {
	clearEnv(t)
	t.Setenv("HOME", t.TempDir())

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", ServiceKey: "prod-key"}); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	resolved, err := Resolve("prod")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.ServiceKey != "prod-key" {
		t.Fatalf("unexpected key: %q", resolved.ServiceKey)
	}
}

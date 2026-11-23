// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// clearEnv removes the credential environment variables so a test starts from a
// known state regardless of the developer's shell.
func clearEnv(t *testing.T) {
	t.Helper()
	t.Setenv(EnvAPIURL, "")
	t.Setenv(EnvAPIKey, "")
}

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

func TestSaveWritesThroughSymlinkedConfig(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	if err := cfg.AddProfile("dev", Profile{APIKey: "dev-key"}); err != nil {
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
				return cfg.AddProfile(name, Profile{APIKey: "key-" + name})
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

func TestLoadInvalidYAMLFails(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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

// The file holds every stored API key, so a failed write must not be able
// to leave a truncated or half-written config behind.
func TestSaveReplacesFileAtomically(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	var cfg Config
	if err := cfg.AddProfile("prod", Profile{APIURL: "https://prod.example.com", APIKey: "prod-key"}); err != nil {
		t.Fatalf("add prod failed: %v", err)
	}
	if err := Save(cfg); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if err := cfg.AddProfile("dev", Profile{APIURL: "https://dev.example.com", APIKey: "dev-key"}); err != nil {
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

func TestEditRecoversStaleLockThroughRelativeSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clearEnv(t)

	path, err := Path()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	target := filepath.Join(home, "dotfiles", "nvfleetint.yaml")
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	initial := []byte("current_profile: missing\nprofiles:\n  dev:\n    api_url: https://old.example.com\n    api_key: old-key\n")
	if err := os.WriteFile(target, initial, fileMode); err != nil {
		t.Fatalf("write target: %v", err)
	}
	relativeTarget, err := filepath.Rel(filepath.Dir(path), target)
	if err != nil {
		t.Fatalf("relative target: %v", err)
	}
	if err := os.Symlink(relativeTarget, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	lockPath := path + ".lock"
	if err := os.WriteFile(lockPath, []byte("stale"), lockMode); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	stale := time.Now().Add(-lockStaleAfter - time.Minute)
	if err := os.Chtimes(lockPath, stale, stale); err != nil {
		t.Fatalf("age lock: %v", err)
	}

	want := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
		"dev": {APIURL: "https://new.example.com", APIKey: "new-key"},
	}}
	got, err := Edit(func(cfg *Config) error {
		return cfg.UpdateProfile("dev", want.Profiles["dev"])
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("edited config: got %#v, want %#v", got, want)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !reflect.DeepEqual(loaded, want) {
		t.Fatalf("reloaded config: got %#v, want %#v", loaded, want)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config path is not a symlink: info=%v err=%v", info, err)
	}
	targetConfig, err := loadFromPath(target)
	if err != nil || !reflect.DeepEqual(targetConfig, want) {
		t.Fatalf("target config: got %#v, want %#v, err=%v", targetConfig, want, err)
	}
	info, err = os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	wantMode := os.FileMode(fileMode)
	if runtime.GOOS == "windows" {
		// Windows exposes writable regular files as 0666; Chmod applies only
		// the owner write bit (see os.fileStat.mode in types_windows.go).
		wantMode = 0666
	}
	if info.Mode().Perm() != wantMode {
		t.Fatalf("target permissions: got %o, want %o", info.Mode().Perm(), wantMode)
	}
	if _, err := os.Lstat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock remains or cannot be inspected: %v", err)
	}
}

func TestEditReadErrorDoesNotMutateAndRemovesLock(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clearEnv(t)
	path, err := Path()
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	if err := os.MkdirAll(path, dirMode); err != nil {
		t.Fatalf("mkdir config path: %v", err)
	}
	called := false
	_, err = Edit(func(*Config) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "read config") || !strings.Contains(err.Error(), path) {
		t.Fatalf("expected read-config error identifying %q, got %v", path, err)
	}
	if called {
		t.Fatal("mutation callback called after read error")
	}
	if _, err := os.Lstat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock remains or cannot be inspected: %v", err)
	}
}

func TestEditAtomicRenameFailureCleansUpCredentials(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	clearEnv(t)
	initial := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
		"dev": {APIURL: "https://fleet.example.com", APIKey: "original-key"},
	}}
	if err := Save(initial); err != nil {
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
	backup := path + ".backup"
	edited, err := Edit(func(cfg *Config) error {
		if err := cfg.UpdateProfile("dev", Profile{APIURL: "https://new.example.com", APIKey: "replacement-key"}); err != nil {
			t.Fatalf("update profile: %v", err)
		}
		if err := os.Rename(path, backup); err != nil {
			t.Fatalf("back up config: %v", err)
		}
		if err := os.Mkdir(path, dirMode); err != nil {
			t.Fatalf("create destination directory: %v", err)
		}
		return nil
	})
	var renameErr *os.LinkError
	if !errors.As(err, &renameErr) || renameErr.Op != "rename" || renameErr.New != path {
		t.Fatalf("Edit error: got %v, want rename failure at %q", err, path)
	}
	if !reflect.DeepEqual(edited, Config{}) {
		t.Errorf("Edit config: got %#v, want zero Config", edited)
	}
	after, err := os.ReadFile(backup)
	if err != nil || !bytes.Equal(after, before) {
		t.Errorf("backup changed: got %q, want %q, err=%v", after, before, err)
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		t.Errorf("destination is not a directory: info=%v err=%v", info, err)
	}
	if _, err := os.Lstat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("lock remains or cannot be inspected: %v", err)
	}
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".config-*.yaml"))
	if err != nil || len(temps) != 0 {
		t.Errorf("temporary config files remain or cannot be inspected: %v, err=%v", temps, err)
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

func TestConfigWriteDirectoryFailuresPreserveData(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, home string, cfg Config)
	}{
		{"lock directory", func(t *testing.T, home string, cfg Config) {
			blocker := filepath.Join(home, ".config")
			original := []byte("existing data")
			if err := os.WriteFile(blocker, original, fileMode); err != nil {
				t.Fatalf("write blocker: %v", err)
			}
			if err := Save(cfg); err == nil {
				t.Error("Save succeeded with a blocked config directory")
			}
			called := false
			if _, err := Edit(func(*Config) error { called = true; return nil }); err == nil {
				t.Error("Edit succeeded with a blocked config directory")
			}
			if called {
				t.Error("Edit invoked callback after lock directory failure")
			}
			assertBlockerUnchanged(t, blocker, original)
		}},
		{"resolved target directory", func(t *testing.T, home string, cfg Config) {
			path, err := Path()
			if err != nil {
				t.Fatalf("path: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
				t.Fatalf("mkdir config: %v", err)
			}
			blocker := filepath.Join(home, "blocker")
			original := []byte("existing data")
			if err := os.WriteFile(blocker, original, fileMode); err != nil {
				t.Fatalf("write blocker: %v", err)
			}
			target := filepath.Join(blocker, "child", fileName)
			if err := os.Symlink(target, path); err != nil {
				t.Fatalf("symlink: %v", err)
			}
			// Compare the complete tree so neither a temporary file nor any other
			// new file containing the profile key can be left behind.
			tree := func() []string {
				var paths []string
				err := filepath.WalkDir(home, func(path string, _ os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					paths = append(paths, path)
					return nil
				})
				if err != nil {
					t.Fatalf("walk home: %v", err)
				}
				return paths
			}
			before := tree()
			if err := Save(cfg); err == nil {
				t.Error("Save succeeded with a blocked symlink target directory")
			}
			info, err := os.Lstat(path)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("config path is not a symlink: info=%v err=%v", info, err)
			}
			if got, err := os.Readlink(path); err != nil || got != target {
				t.Errorf("symlink target: got %q, want %q, err=%v", got, target, err)
			}
			assertBlockerUnchanged(t, blocker, original)
			if _, err := os.Lstat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("lock remains or cannot be inspected: %v", err)
			}
			if after := tree(); !reflect.DeepEqual(after, before) {
				t.Errorf("home tree changed: got %v, want %v", after, before)
			}
		}},
		{"saveToPath directory", func(t *testing.T, home string, cfg Config) {
			blocker := filepath.Join(home, "blocker")
			original := []byte("existing data")
			if err := os.WriteFile(blocker, original, fileMode); err != nil {
				t.Fatalf("write blocker: %v", err)
			}
			if err := saveToPath(filepath.Join(blocker, "child", fileName), cfg); err == nil {
				t.Error("saveToPath succeeded with a blocked directory")
			}
			assertBlockerUnchanged(t, blocker, original)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			clearEnv(t)
			cfg := Config{CurrentProfile: "dev", Profiles: map[string]Profile{
				"dev": {APIURL: "https://fleet.example.com", APIKey: "distinct-test-key"},
			}}
			tt.run(t, home, cfg)
		})
	}
}

func assertBlockerUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("blocking file changed: got %q, want %q, err=%v", got, want, err)
	}
}

func TestConfigFilesystemResolutionAndLockErrors(t *testing.T) {
	t.Run("atomic write path resolution", func(t *testing.T) {
		dir := t.TempDir()
		blocker := filepath.Join(dir, "blocker")
		original := []byte("distinctive existing data\n")
		if err := os.WriteFile(blocker, original, fileMode); err != nil {
			t.Fatalf("write blocker: %v", err)
		}
		path := filepath.Join(blocker, fileName)
		err := writeFileAtomic(path, []byte("api_key: temporary-test-key\n"))
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) || pathErr.Op != "lstat" || pathErr.Path != path || errors.Is(err, os.ErrNotExist) {
			t.Errorf("writeFileAtomic error: got %v, want non-NotExist lstat error for %q", err, path)
		}
		assertBlockerUnchanged(t, blocker, original)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read temporary directory: %v", err)
		}
		if len(entries) != 1 || entries[0].Name() != "blocker" {
			t.Errorf("unexpected files after atomic write: %v", entries)
		}
	})

	t.Run("lock directory absent", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "removed")
		lockPath := filepath.Join(dir, "config.lock")
		file, err := acquireLock(lockPath)
		if file != nil {
			_ = file.Close()
			t.Error("acquireLock returned a file for an absent directory")
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("acquireLock error: got %v, want os.ErrNotExist", err)
		}
		for _, path := range []string{dir, lockPath} {
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("path %q exists or cannot be inspected: %v", path, err)
			}
		}
	})
}

func TestWriteAndCloseFailuresPreserveCredentials(t *testing.T) {
	tests := []struct {
		name        string
		closeBefore bool
		operation   string
	}{
		{name: "chmod on closed file", closeBefore: true, operation: "chmod"},
		{name: "write on read-only file", operation: "write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credentials")
			original := []byte("api_key: original-test-credential\n")
			if err := os.WriteFile(path, original, fileMode); err != nil {
				t.Fatalf("write original credentials: %v", err)
			}
			file, err := os.Open(path)
			if err != nil {
				t.Fatalf("open credentials: %v", err)
			}
			t.Cleanup(func() { _ = file.Close() })
			if tt.closeBefore {
				if err := file.Close(); err != nil {
					t.Fatalf("close credentials: %v", err)
				}
			}

			err = writeAndClose(file, []byte("api_key: replacement-test-credential\n"))
			if err == nil {
				t.Fatal("writeAndClose succeeded, want an error")
			}
			if tt.closeBefore && !errors.Is(err, os.ErrClosed) {
				t.Errorf("error: got %v, want os.ErrClosed", err)
			}
			var pathErr *os.PathError
			if !errors.As(err, &pathErr) || pathErr.Op != tt.operation {
				t.Errorf("error: got %v, want %s operation", err, tt.operation)
			}
			if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Errorf("file remains open or cannot be checked: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Errorf("credentials changed: got %q, want %q, err=%v", got, original, err)
			}
		})
	}
}

func TestWriteAndClosePipeSyncError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux pipe sync semantics; not executed on this platform")
	}

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	t.Cleanup(func() { _ = writer.Close() })

	payload := []byte("api_key: distinctive-pipe-test-credential\n")
	err = writeAndClose(writer, payload)
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) || pathErr.Op != "sync" || !errors.Is(err, syscall.EINVAL) {
		t.Errorf("writeAndClose error: got %v, want sync error wrapping EINVAL", err)
	}
	if _, err := writer.Stat(); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("pipe write end remains open or cannot be checked: %v", err)
	}
	got, err := io.ReadAll(reader)
	if err != nil || !bytes.Equal(got, payload) {
		t.Errorf("pipe payload: got %q, want %q, err=%v", got, payload, err)
	}
}

func TestConfigInvalidHomeErrorPropagation(t *testing.T) {
	homeVar := "HOME"
	switch runtime.GOOS {
	case "windows":
		homeVar = "USERPROFILE"
	case "plan9":
		homeVar = "home"
	}

	tests := []struct {
		name string
		home string
	}{
		{name: "whitespace", home: " \t\n"},
	}
	if runtime.GOOS != "android" && runtime.GOOS != "ios" {
		tests = append(tests, struct {
			name string
			home string
		}{name: "empty", home: ""})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(homeVar, tt.home)
			clearEnv(t)

			path, pathErr := Path()
			if path != "" || pathErr == nil {
				t.Fatalf("Path: got %q, %v; want empty path and error", path, pathErr)
			}
			if tt.name == "whitespace" && pathErr.Error() != "home directory is required" {
				t.Fatalf("Path error: got %q, want home directory is required", pathErr)
			}
			checkError := func(operation string, err error) {
				t.Helper()
				if err == nil || err.Error() != pathErr.Error() {
					t.Errorf("%s error: got %v, want %v", operation, err, pathErr)
				}
			}

			loaded, err := Load()
			checkError("Load", err)
			if !reflect.DeepEqual(loaded, Config{}) {
				t.Errorf("Load config: got %#v, want zero Config", loaded)
			}
			checkError("Save", Save(Config{CurrentProfile: "named"}))
			called := false
			edited, err := Edit(func(*Config) error {
				called = true
				return nil
			})
			checkError("Edit", err)
			if !reflect.DeepEqual(edited, Config{}) || called {
				t.Errorf("Edit: got config %#v and callback called=%t; want zero Config and no callback", edited, called)
			}

			t.Setenv(EnvAPIKey, "test-environment-key")
			resolved, err := Resolve("")
			if err != nil {
				t.Fatalf("Resolve from environment: %v", err)
			}
			if resolved.APIKey != "test-environment-key" || resolved.APIKeySource != SourceEnvironment ||
				resolved.APIURL != DefaultAPIURL || resolved.APIURLSource != SourceDefault {
				t.Errorf("Resolve from environment: unexpected credentials or sources: %#v", resolved)
			}
			checkError("Resolve ConfigError", resolved.ConfigError)
			named, err := Resolve("named")
			checkError("Resolve named", err)
			if !reflect.DeepEqual(named, Resolved{}) {
				t.Errorf("Resolve named: got %#v, want zero Resolved", named)
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

func TestRound2ConfigLockNameTooLong(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("requires Linux or macOS filename limits")
	}

	dir := t.TempDir()
	name := strings.Repeat("a", 255)
	path := filepath.Join(dir, name)
	initial := []byte("distinctive original config contents\n")
	if err := os.WriteFile(path, initial, 0600); err != nil {
		if errors.Is(err, syscall.ENAMETOOLONG) {
			t.Skipf("filesystem does not support 255-byte filenames: %v", err)
		}
		t.Fatalf("write original config: %v", err)
	}

	called := false
	err := withConfigLock(path, func() error {
		called = true
		return nil
	})
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("lock error = %v, want *os.PathError", err)
	}
	if pathErr.Op != "open" || pathErr.Path != path+".lock" || !errors.Is(pathErr.Err, syscall.ENAMETOOLONG) {
		t.Errorf("lock error = %#v, want open of %q with ENAMETOOLONG", pathErr, path+".lock")
	}
	if called {
		t.Error("callback ran after lock acquisition failed")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read original config: %v", err)
	}
	if !bytes.Equal(got, initial) {
		t.Errorf("config contents = %q, want %q", got, initial)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != name || !entries[0].Type().IsRegular() {
		t.Errorf("directory entries = %v, want only original regular file %q", entries, name)
	}
}

func TestRound2ConfigAtomicTempPathTooLong(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("requires Linux PATH_MAX behavior")
	}

	const dirLength = 4088
	dir := t.TempDir()
	if !filepath.IsAbs(dir) || len(dir) > dirLength-2 {
		t.Fatalf("cannot construct %d-byte directory from temporary directory %q", dirLength, dir)
	}
	for len(dir) < dirLength {
		remaining := dirLength - len(dir)
		componentLength := remaining - 1 // Account for the path separator.
		if componentLength > 200 {
			componentLength = 200
			if remaining-componentLength-1 == 1 {
				componentLength--
			}
		}
		if componentLength < 1 {
			t.Fatalf("cannot finish %d-byte directory path from %q", dirLength, dir)
		}
		dir = filepath.Join(dir, strings.Repeat("d", componentLength))
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatalf("create fixture directory of length %d: %v", len(dir), err)
		}
	}
	if len(dir) != dirLength {
		t.Fatalf("fixture directory length = %d, want %d", len(dir), dirLength)
	}

	target := filepath.Join(dir, "c")
	original := []byte("original credentials: secret-original\n")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatalf("create original config: %v", err)
	}

	err := writeFileAtomic(target, []byte("replacement credentials: secret-new\n"))
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Fatalf("atomic write error = %v, want *os.PathError", err)
	}
	if pathErr.Op != "open" || filepath.Dir(pathErr.Path) != dir ||
		!strings.HasPrefix(filepath.Base(pathErr.Path), ".config-") ||
		!strings.HasSuffix(pathErr.Path, ".yaml") || !errors.Is(pathErr, syscall.ENAMETOOLONG) {
		t.Errorf("atomic write error = %#v, want temporary-file open with ENAMETOOLONG in %q", pathErr, dir)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read original config: %v", err)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("config contents = %q, want %q", got, original)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read config directory: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "c" || !entries[0].Type().IsRegular() {
		t.Errorf("directory entries = %v, want only original regular file %q", entries, "c")
	}
}

// A live competing writer must time out without invoking the edit callback or
// deleting the other process's lock/configuration. Only this test's temp files
// are involved; the production 30-second acquisition deadline is exercised.
func TestRound2CompetingConfigLockPreservesOwner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	original := []byte("original configuration\n")
	owner := []byte("another live writer owns this lock\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".lock", owner, 0600); err != nil {
		t.Fatal(err)
	}
	called := false
	err := withConfigLock(path, func() error {
		called = true
		return os.WriteFile(path, []byte("unexpected edit"), 0600)
	})
	if err == nil || !strings.Contains(err.Error(), "config is locked by another nvfleetint process") || !strings.Contains(err.Error(), path+".lock") {
		t.Fatalf("lock error = %v, want competing-writer error naming the lock", err)
	}
	if called {
		t.Error("edit callback ran while another writer owned the lock")
	}
	for name, want := range map[string][]byte{path: original, path + ".lock": owner} {
		got, readErr := os.ReadFile(name)
		if readErr != nil || !bytes.Equal(got, want) {
			t.Errorf("file %q changed: contents=%q error=%v", name, got, readErr)
		}
	}
}

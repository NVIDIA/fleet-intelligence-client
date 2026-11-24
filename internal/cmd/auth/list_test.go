// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

func TestAuthListShowsProfilesWithoutPrintingKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, apiURL)
	mustAddProfile(t, "dev", "dev-key", "")

	got := mustRunAuth(t, "list")
	for _, want := range []string{"NAME", "API KEY", "ACTIVE", "prod", "dev", apiURL, "configured"} {
		if !strings.Contains(got, want) {
			t.Fatalf("list output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, apiKey) || strings.Contains(got, "dev-key") {
		t.Fatalf("list printed a secret: %q", got)
	}
}

func TestAuthListJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, apiURL)

	out := mustRunAuth(t, "list", "--output", "json")
	var got authListOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode list JSON failed: %v", err)
	}
	if got.CurrentProfile != "prod" {
		t.Fatalf("unexpected current profile: %q", got.CurrentProfile)
	}
	if len(got.Profiles) != 1 {
		t.Fatalf("unexpected profile count: %d", len(got.Profiles))
	}
	profile := got.Profiles[0]
	if profile.Name != "prod" || profile.APIURL != apiURL || !profile.APIKeyConfigured || !profile.Current {
		t.Fatalf("unexpected profile JSON: %#v", profile)
	}
	if strings.Contains(out, apiKey) {
		t.Fatalf("list printed a secret: %q", out)
	}
}

func TestAuthListMarksSelectedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", "prod-key", "")
	mustAddProfile(t, "dev", "dev-key", "")
	mustRunAuth(t, "use", "dev")

	out := mustRunAuth(t, "list", "--output", "json")
	var got authListOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode list JSON failed: %v", err)
	}
	if got.CurrentProfile != "dev" {
		t.Fatalf("expected the selected profile, got %#v", got)
	}
	for _, profile := range got.Profiles {
		if profile.Name == "dev" && !profile.Current {
			t.Fatalf("expected dev to be marked current: %#v", got)
		}
		if profile.Name == "prod" && profile.Current {
			t.Fatalf("expected prod not to be marked current: %#v", got)
		}
	}
}

func TestAuthListWarnsAboutDanglingCurrentProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	cfg := config.Config{
		CurrentProfile: "gone",
		Profiles: map[string]config.Profile{
			"dev": {APIKey: "dev-key"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	got := mustRunAuth(t, "list")
	if !strings.Contains(got, `current profile "gone" is not configured`) {
		t.Fatalf("expected dangling-current warning: %q", got)
	}
}

func TestAuthListWithoutProfilesExplainsSetup(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	got := mustRunAuth(t, "list")
	if !strings.Contains(got, "No profiles configured") {
		t.Fatalf("unexpected output: %q", got)
	}
	if !strings.Contains(got, "auth add <name>") {
		t.Fatalf("expected a setup hint: %q", got)
	}
}

func TestAuthListReportsEnvironmentOverrides(t *testing.T) {
	const storedKey = "stored-secret-for-list"
	const envKey = "environment-secret-for-list"
	tests := []struct {
		name, url, key, note string
	}{
		{"url", "https://env.example.com", "", config.EnvAPIURL + " is"},
		{"key", "", envKey, config.EnvAPIKey + " is"},
		{"both", "https://env.example.com", envKey, config.EnvAPIURL + " / " + config.EnvAPIKey + " are"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)
			cfg := config.Config{CurrentProfile: "selected", Profiles: map[string]config.Profile{
				"selected": {APIURL: apiURL, APIKey: storedKey},
				"keyless":  {APIURL: "https://keyless.example.com"},
			}}
			if err := config.Save(cfg); err != nil {
				t.Fatalf("save config: %v", err)
			}
			t.Setenv(config.EnvAPIURL, tt.url)
			t.Setenv(config.EnvAPIKey, tt.key)
			out := mustRunAuth(t, "list")
			rows := make(map[string]string)
			for _, line := range strings.Split(out, "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 {
					rows[fields[0]] = strings.Join(fields, " ")
				}
			}
			if rows["selected"] != "selected "+apiURL+" configured *" {
				t.Fatalf("unexpected selected row: %q", rows["selected"])
			}
			if rows["keyless"] != "keyless https://keyless.example.com not configured -" {
				t.Fatalf("unexpected keyless row: %q", rows["keyless"])
			}
			wantNote := tt.note + ` overriding values stored in profile "selected"; run ` + "`nvfleetint auth status`" + " for effective credentials."
			if !strings.Contains(out, wantNote) {
				t.Fatalf("missing override note %q in %q", wantNote, out)
			}
			if strings.Contains(out, storedKey) || strings.Contains(out, envKey) {
				t.Fatalf("list exposed a key: %q", out)
			}
		})
	}
}

func TestAuthListPropagatesOutputError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)
	cmdtest.SaveConfig(t, apiURL, apiKey)
	sentinel := errors.New("list output failed")
	cmd := newRootCmd()
	cmd.SetOut(authListErrorWriter{err: sentinel})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"auth", "list"})
	if err := cmd.Execute(); !errors.Is(err, sentinel) {
		t.Fatalf("Execute error = %v, want output error", err)
	}
}

func TestAuthListRejectsMalformedConfig(t *testing.T) {
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
	if err := os.WriteFile(path, []byte("profiles: [\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	err = runAuthErr(t, "list")
	if !strings.Contains(err.Error(), "read config") || !strings.Contains(err.Error(), path) {
		t.Fatalf("expected config read/parse error, got %v", err)
	}
}

func TestProfileListHint(t *testing.T) {
	if got := withProfileListHint(nil); got != nil {
		t.Fatalf("nil error changed: %v", got)
	}

	unrelated := errors.New("configuration unavailable")
	if got := withProfileListHint(unrelated); got != unrelated {
		t.Fatalf("unrelated error changed: got %v, want original error", got)
	}

	missing := fmt.Errorf("lookup: %w", config.ErrProfileNotFound)
	got := withProfileListHint(missing)
	if !errors.Is(got, config.ErrProfileNotFound) || !errors.Is(got, missing) {
		t.Fatalf("missing-profile error identity was lost: %v", got)
	}
	want := missing.Error() + "; run `nvfleetint auth list` to see the configured profiles"
	if got.Error() != want {
		t.Fatalf("hint = %q, want %q", got.Error(), want)
	}
}

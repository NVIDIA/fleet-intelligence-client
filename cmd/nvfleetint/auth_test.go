// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

const (
	serviceKey = "test-key"
	apiURL     = "https://fleet.example.com"
)

// newAuthStatusServer starts a test server that mimics GET /v1/auth/status,
// asserting the bearer token and replying with the given status and body.
func newAuthStatusServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+serviceKey {
			t.Errorf("unexpected auth header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_, _ = w.Write([]byte(body))
	}))
}

// mustRunAuth executes an auth subcommand that is expected to succeed.
func mustRunAuth(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"auth"}, args...))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth %s failed: %v", strings.Join(args, " "), err)
	}

	return out.String()
}

// runAuthErr executes an auth subcommand that is expected to fail.
func runAuthErr(t *testing.T, args ...string) error {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(append([]string{"auth"}, args...))

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected auth %s to fail", strings.Join(args, " "))
	}

	return err
}

func TestAuthSubcommandsRejectPositionalArgs(t *testing.T) {
	tests := [][]string{
		{"add", "--profile", "prod", "--key", serviceKey, "extra"},
		{"update", "--profile", "prod", "--key", serviceKey, "extra"},
		{"remove", "--profile", "prod", "--yes", "extra"},
		{"use", "--profile", "prod", "extra"},
		{"list", "extra"},
		{"status", "extra"},
	}

	for _, args := range tests {
		t.Run(strings.Join(args[:1], " "), func(t *testing.T) {
			err := runAuthErr(t, args...)
			if !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthAddStoresProfileAndSelectsIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	got := mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)
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
	if profile.ServiceKey != serviceKey {
		t.Fatalf("unexpected service key: %q", profile.ServiceKey)
	}
}

func TestAuthAddSavesExplicitAPIURLAndKeepsCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)
	got := mustRunAuth(t, "add", "--profile", "dev", "--key", "dev-key", "--api-url", apiURL)
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

func TestAuthAddRejectsDuplicateProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)

	err := runAuthErr(t, "add", "--profile", "prod", "--key", "other-key")
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	// The error must point at the command that actually changes a profile.
	if !strings.Contains(err.Error(), "auth update --profile prod") {
		t.Fatalf("expected an update hint: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Profiles["prod"].ServiceKey != serviceKey {
		t.Fatal("a rejected add must not overwrite the stored key")
	}
}

func TestAuthAddRequiresProfileName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthErr(t, "add", "--key", serviceKey)
	if !strings.Contains(err.Error(), "profile name is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddRejectsInvalidProfileName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthErr(t, "add", "--profile", "has space", "--key", serviceKey)
	if !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddRequiresKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthErr(t, "add", "--profile", "prod")
	if !strings.Contains(err.Error(), "service key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddRejectsInvalidAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthErr(t, "add", "--profile", "prod", "--key", serviceKey, "--api-url", "example.com")
	if !strings.Contains(err.Error(), "absolute https URL is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A plaintext remote endpoint would put the service key on the wire, so add
// must refuse it and leave no config behind.
func TestAuthAddRejectsInsecureAPIURL(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	err := runAuthErr(t, "add", "--profile", "prod", "--key", serviceKey, "--api-url", "http://api.example.com")
	if !strings.Contains(err.Error(), "https is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected no config file to be written, stat returned %v", statErr)
	}
}

// Loopback stays usable so the local mock-server workflow keeps working.
func TestAuthAddAcceptsLoopbackHTTPAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	const loopbackURL = "http://127.0.0.1:9999"
	mustRunAuth(t, "add", "--profile", "local", "--key", serviceKey, "--api-url", loopbackURL)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Profiles["local"].APIURL != loopbackURL {
		t.Fatalf("unexpected API URL: %q", cfg.Profiles["local"].APIURL)
	}
}

func TestAuthUpdateChangesOnlySuppliedFields(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey, "--api-url", apiURL)
	mustRunAuth(t, "update", "--profile", "prod", "--key", "rotated-key")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	profile := cfg.Profiles["prod"]
	if profile.ServiceKey != "rotated-key" {
		t.Fatalf("unexpected service key: %q", profile.ServiceKey)
	}
	// An omitted --api-url must leave the endpoint alone.
	if profile.APIURL != apiURL {
		t.Fatalf("unexpected API URL: %q", profile.APIURL)
	}
}

// An empty value is rejected rather than treated as "clear this credential":
// `--key "$KEY"` with KEY unset must not silently wipe a stored key.
func TestAuthUpdateRejectsEmptyValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "key", args: []string{"--key", ""}, want: "--key cannot be empty"},
		{name: "api-url", args: []string{"--api-url", ""}, want: "--api-url cannot be empty"},
		{name: "whitespace key", args: []string{"--key", "   "}, want: "--key cannot be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			clearCredentialEnv(t)
			mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey, "--api-url", apiURL)

			err := runAuthErr(t, append([]string{"update", "--profile", "prod"}, tt.args...)...)
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}

			cfg, loadErr := config.Load()
			if loadErr != nil {
				t.Fatalf("load failed: %v", loadErr)
			}
			if got := cfg.Profiles["prod"]; got.ServiceKey != serviceKey || got.APIURL != apiURL {
				t.Fatalf("a rejected update must not change the profile: %#v", got)
			}
		})
	}
}

func TestAuthUpdateRequiresSomethingToChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)

	err := runAuthErr(t, "update", "--profile", "prod")
	if !strings.Contains(err.Error(), "nothing to update") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthUpdateRejectsUnknownProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	err := runAuthErr(t, "update", "--profile", "nope", "--key", serviceKey)
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "auth list") {
		t.Fatalf("expected a discovery hint: %v", err)
	}
}

func TestAuthRemoveCurrentProfileClearsSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)
	mustRunAuth(t, "add", "--profile", "dev", "--key", "dev-key")

	got := mustRunAuth(t, "remove", "--profile", "prod", "--yes")
	if !strings.Contains(got, `Profile "prod" removed.`) {
		t.Fatalf("unexpected output: %q", got)
	}
	if !strings.Contains(got, "No current profile") {
		t.Fatalf("expected no profile to be selected: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if _, ok := cfg.Profiles["prod"]; ok {
		t.Fatal("expected prod to be removed")
	}
	if cfg.CurrentProfile != "" {
		t.Fatalf("unexpected current profile: %q", cfg.CurrentProfile)
	}
}

func TestAuthRemoveOtherProfileKeepsSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)
	mustRunAuth(t, "add", "--profile", "dev", "--key", "dev-key")

	got := mustRunAuth(t, "remove", "--profile", "dev", "--yes")
	if !strings.Contains(got, "Current profile: prod") {
		t.Fatalf("expected prod to stay current: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.CurrentProfile != "prod" {
		t.Fatalf("unexpected current profile: %q", cfg.CurrentProfile)
	}
}

func TestAuthRemoveLastProfileExplainsRecovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)

	got := mustRunAuth(t, "remove", "--profile", "prod", "--yes")
	if !strings.Contains(got, "No profiles remain") {
		t.Fatalf("unexpected output: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(cfg.Profiles) != 0 || cfg.CurrentProfile != "" {
		t.Fatalf("expected an empty config, got %#v", cfg)
	}
}

func TestAuthRemoveRejectsUnknownProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	err := runAuthErr(t, "remove", "--profile", "nope", "--yes")
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Removing a profile destroys a service key that cannot be recovered, so it
// confirms first. The prompt goes to stderr and defaults to No.
func TestAuthRemovePromptsForConfirmation(t *testing.T) {
	tests := []struct {
		name    string
		answer  string
		removed bool
	}{
		{name: "y", answer: "y\n", removed: true},
		{name: "yes", answer: "YES\n", removed: true},
		{name: "n", answer: "n\n", removed: false},
		{name: "empty defaults to no", answer: "\n", removed: false},
		{name: "eof defaults to no", answer: "", removed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			clearCredentialEnv(t)
			mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)

			var out, errOut bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetIn(strings.NewReader(tt.answer))
			cmd.SetArgs([]string{"auth", "remove", "--profile", "prod"})

			err := cmd.Execute()

			// The prompt must stay off stdout so `-o json` remains parseable.
			if !strings.Contains(errOut.String(), "Are you sure? [y/N]") {
				t.Fatalf("prompt missing from stderr: %q", errOut.String())
			}
			if strings.Contains(out.String(), "Are you sure?") {
				t.Fatalf("prompt leaked to stdout: %q", out.String())
			}

			cfg, loadErr := config.Load()
			if loadErr != nil {
				t.Fatalf("load failed: %v", loadErr)
			}
			_, stillThere := cfg.Profiles["prod"]

			if tt.removed {
				if err != nil {
					t.Fatalf("remove failed: %v", err)
				}
				if stillThere {
					t.Fatal("expected the profile to be removed")
				}
				return
			}
			if !errors.Is(err, errAborted) {
				t.Fatalf("expected errAborted, got %v", err)
			}
			if !stillThere {
				t.Fatal("a declined prompt must leave the profile alone")
			}
		})
	}
}

// A cron or CI run gets a clear error instead of blocking or silently aborting.
// /dev/null is the case a character-device check gets wrong: it looks like a
// terminal by mode, so `nvfleetint auth remove </dev/null` would read EOF and
// abort with a confusing "aborted" instead of naming --yes.
func TestAuthRemoveRefusesToPromptWithoutATerminal(t *testing.T) {
	tests := []struct {
		name string
		open func(t *testing.T) *os.File
	}{
		{
			name: "closed pipe",
			open: func(t *testing.T) *os.File {
				t.Helper()
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatalf("pipe failed: %v", err)
				}
				t.Cleanup(func() { _ = reader.Close() })
				_ = writer.Close()
				return reader
			},
		},
		{
			name: "dev null",
			open: func(t *testing.T) *os.File {
				t.Helper()
				file, err := os.Open(os.DevNull)
				if err != nil {
					t.Fatalf("open %s failed: %v", os.DevNull, err)
				}
				t.Cleanup(func() { _ = file.Close() })
				return file
			},
		},
		{
			name: "regular file",
			open: func(t *testing.T) *os.File {
				t.Helper()
				path := filepath.Join(t.TempDir(), "stdin")
				if err := os.WriteFile(path, []byte("y\n"), 0o600); err != nil {
					t.Fatalf("write failed: %v", err)
				}
				file, err := os.Open(path)
				if err != nil {
					t.Fatalf("open failed: %v", err)
				}
				t.Cleanup(func() { _ = file.Close() })
				return file
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			clearCredentialEnv(t)
			mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)

			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetIn(tt.open(t))
			cmd.SetArgs([]string{"auth", "remove", "--profile", "prod"})

			err := cmd.Execute()
			if err == nil {
				t.Fatal("expected a refusal to prompt")
			}
			if !strings.Contains(err.Error(), "--yes") {
				t.Fatalf("expected the error to name --yes, got %v", err)
			}

			cfg, loadErr := config.Load()
			if loadErr != nil {
				t.Fatalf("load failed: %v", loadErr)
			}
			if _, ok := cfg.Profiles["prod"]; !ok {
				t.Fatal("expected the profile to survive")
			}
		})
	}
}

// The prompt is about a real deletion, so a typo'd name must fail before it.
func TestAuthRemoveUnknownProfileDoesNotPrompt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)
	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)

	var errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"auth", "remove", "--profile", "nope"})

	if err := cmd.Execute(); !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(errOut.String(), "Are you sure?") {
		t.Fatalf("unexpected prompt: %q", errOut.String())
	}
}

func TestAuthUseSelectsProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)
	mustRunAuth(t, "add", "--profile", "dev", "--key", "dev-key")

	got := mustRunAuth(t, "use", "--profile", "dev")
	if !strings.Contains(got, "Current profile: dev") {
		t.Fatalf("unexpected output: %q", got)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.CurrentProfile != "dev" {
		t.Fatalf("unexpected current profile: %q", cfg.CurrentProfile)
	}
}

func TestAuthUseRejectsUnknownProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey)

	err := runAuthErr(t, "use", "--profile", "nope")
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// "nothing is configured" needs a different remedy than "that name is wrong".
func TestAuthUseWithoutAnyProfilesErrorsDistinctly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	err := runAuthErr(t, "use", "--profile", "prod")
	if !errors.Is(err, config.ErrNoProfile) {
		t.Fatalf("expected ErrNoProfile, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth add --profile prod") {
		t.Fatalf("expected a setup hint naming the profile: %v", err)
	}
}

func TestAuthListShowsProfilesWithoutPrintingKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey, "--api-url", apiURL)
	mustRunAuth(t, "add", "--profile", "dev", "--key", "dev-key")

	got := mustRunAuth(t, "list")
	for _, want := range []string{"NAME", "SERVICE KEY", "ACTIVE", "prod", "dev", apiURL, "configured"} {
		if !strings.Contains(got, want) {
			t.Fatalf("list output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, serviceKey) || strings.Contains(got, "dev-key") {
		t.Fatalf("list printed a secret: %q", got)
	}
}

func TestAuthListJSON(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", serviceKey, "--api-url", apiURL)

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
	if profile.Name != "prod" || profile.APIURL != apiURL || !profile.ServiceKeyConfigured || !profile.Current {
		t.Fatalf("unexpected profile JSON: %#v", profile)
	}
	if strings.Contains(out, serviceKey) {
		t.Fatalf("list printed a secret: %q", out)
	}
}

func TestAuthListMarksEnvSelectedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--profile", "prod", "--key", "prod-key")
	mustRunAuth(t, "add", "--profile", "dev", "--key", "dev-key")
	t.Setenv(config.EnvProfile, "dev")

	out := mustRunAuth(t, "list", "--output", "json")
	var got authListOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode list JSON failed: %v", err)
	}
	if got.CurrentProfile != "dev" {
		t.Fatalf("expected env-selected profile, got %#v", got)
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
	clearCredentialEnv(t)

	cfg := config.Config{
		CurrentProfile: "gone",
		Profiles: map[string]config.Profile{
			"dev": {ServiceKey: "dev-key"},
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
	clearCredentialEnv(t)

	got := mustRunAuth(t, "list")
	if !strings.Contains(got, "No profiles configured") {
		t.Fatalf("unexpected output: %q", got)
	}
	if !strings.Contains(got, "auth add --profile") {
		t.Fatalf("expected a setup hint: %q", got)
	}
}

func TestAuthStatusChecksConnectionAndDoesNotPrintSecret(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	saveTestConfig(t, server.URL, serviceKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "Profile: "+testProfile) {
		t.Fatalf("status missing profile: %q", got)
	}
	if !strings.Contains(got, "API URL: "+server.URL) {
		t.Fatalf("status missing API URL: %q", got)
	}
	if !strings.Contains(got, "Service key: configured") {
		t.Fatalf("status missing service key state: %q", got)
	}
	if !strings.Contains(got, "Connection: ok") {
		t.Fatalf("status missing connection state: %q", got)
	}
	if strings.Contains(got, serviceKey) {
		t.Fatalf("status printed secret: %q", got)
	}
}

// `auth status --profile` must report the named profile, not the current one.
func TestAuthStatusChecksNamedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustRunAuth(t, "add", "--profile", "prod", "--key", "prod-key")
	mustRunAuth(t, "add", "--profile", "dev", "--key", serviceKey, "--api-url", server.URL)

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
	clearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustRunAuth(t, "add", "--profile", "dev", "--key", serviceKey, "--api-url", server.URL)
	t.Setenv(config.EnvServiceKey, "stale-key")

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
	clearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustRunAuth(t, "add", "--profile", "dev", "--key", serviceKey, "--api-url", server.URL)
	t.Setenv(config.EnvServiceKey, "stale-key")

	got := mustRunAuth(t, "status", "--profile", "dev")
	if !strings.Contains(got, config.EnvServiceKey+" is set but ignored") {
		t.Fatalf("expected a singular note naming only the set variable: %q", got)
	}
	if strings.Contains(got, config.EnvAPIURL) {
		t.Fatalf("status named an environment variable that was never set: %q", got)
	}
}

// Same for the shadowing note: one variable reads "overrides the value".
func TestAuthStatusShadowedProfileNamesOnlySetVariables(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	saveTestConfig(t, server.URL, "stored-key")
	t.Setenv(config.EnvServiceKey, serviceKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, config.EnvServiceKey+" overrides the value stored in profile") {
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

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	saveTestConfig(t, "https://stored.example.com", "stored-key")
	t.Setenv(config.EnvAPIURL, server.URL)
	t.Setenv(config.EnvServiceKey, serviceKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "API URL: "+server.URL+" (from environment)") {
		t.Fatalf("expected the environment to win: %q", got)
	}
	if !strings.Contains(got, config.EnvAPIURL) || !strings.Contains(got, config.EnvServiceKey) {
		t.Fatalf("status did not name the shadowing variables: %q", got)
	}
	if !strings.Contains(got, `profile "`+testProfile+`"`) {
		t.Fatalf("status did not name the shadowed profile: %q", got)
	}
}

func TestAuthStatusRejectsUnknownProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	err := runAuthErr(t, "status", "--profile", "nope")
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthStatusReportsUnauthorizedOnRejectedKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := newAuthStatusServer(t, http.StatusUnauthorized, `{"error":"unauthorized"}`)
	defer server.Close()

	saveTestConfig(t, server.URL, serviceKey)

	// A rejected key is a reportable status, not a command failure.
	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "Connection: unauthorized") {
		t.Fatalf("status missing unauthorized state: %q", got)
	}
}

func TestAuthStatusWithoutConfigExitsZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "API URL: "+config.DefaultAPIURL) {
		t.Fatalf("status missing default API URL: %q", got)
	}
	if !strings.Contains(got, "Service key: not configured") {
		t.Fatalf("status missing not configured state: %q", got)
	}
	if !strings.Contains(got, "Profile: none") {
		t.Fatalf("status should report no profile: %q", got)
	}
}

func TestAuthStatusJSONUsesEnvFallback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	t.Setenv(config.EnvAPIURL, server.URL)
	t.Setenv(config.EnvServiceKey, serviceKey)

	out := mustRunAuth(t, "status", "--output", "json")
	var got authStatusOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode status JSON failed: %v", err)
	}
	if got.APIURL != server.URL || !got.ServiceKeyConfigured || got.Connection != "ok" {
		t.Fatalf("unexpected status JSON: %#v", got)
	}
	if got.ServiceKeySource != string(config.SourceEnvironment) {
		t.Fatalf("expected the environment to be named as the source: %#v", got)
	}
	if strings.Contains(out, serviceKey) {
		t.Fatalf("status printed secret: %q", out)
	}
}

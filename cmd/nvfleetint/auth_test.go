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

	"github.com/NVIDIA/fleet-intelligence-client/internal/clihelpers"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

const (
	apiKey = "test-key"
	apiURL = "https://fleet.example.com"
)

// newAuthStatusServer starts a test server that mimics GET /v1/auth/status,
// asserting the bearer token and replying with the given status and body.
func newAuthStatusServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+apiKey {
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

// The profile-naming commands take exactly one <name>; a second word is a typo
// (an unquoted name, a stray flag value) and must not be silently ignored.
func TestAuthSubcommandsRejectExtraProfileNames(t *testing.T) {
	tests := [][]string{
		{"add", "prod", "extra", "--api-key", apiKey},
		{"remove", "prod", "extra", "--yes"},
		{"use", "prod", "extra"},
	}

	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			clearCredentialEnv(t)

			err := runAuthErr(t, args...)
			if !strings.Contains(err.Error(), "only one profile name may be given") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// `auth update` was removed in favor of `add`. A removed subcommand must fail
// loudly: cobra's default for a parent with no RunE is to print help and exit
// 0, which would let a key-rotation script report success having rotated
// nothing.
func TestAuthRejectsRemovedUpdateSubcommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)
	mustRunAuth(t, "add", "prod", "--api-key", apiKey)

	err := runAuthErr(t, "update", "prod")
	if !strings.Contains(err.Error(), `unknown command "update"`) {
		t.Fatalf("unexpected error: %v", err)
	}

	// The profile must be untouched, and reachable through the command that
	// replaced it.
	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load failed: %v", loadErr)
	}
	if cfg.Profiles["prod"].APIKey != apiKey {
		t.Fatal("a rejected command must not change the profile")
	}
}

// Bare `auth` is still a help request, not an error.
func TestAuthWithoutSubcommandPrintsHelp(t *testing.T) {
	got := mustRunAuth(t)
	if !strings.Contains(got, "Available Commands:") || !strings.Contains(got, "add") {
		t.Fatalf("expected help output: %q", got)
	}
}

// list and status act on the configuration as a whole, so they take no name.
func TestAuthSubcommandsRejectPositionalArgs(t *testing.T) {
	tests := [][]string{
		{"list", "extra"},
		{"status", "extra"},
	}

	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
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

	got := mustRunAuth(t, "add", "prod", "--api-key", apiKey)
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
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "prod", "--api-key", apiKey)
	got := mustRunAuth(t, "add", "dev", "--api-key", "dev-key", "--api-url", apiURL)
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
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "prod", "--api-key", apiKey, "--api-url", apiURL)

	got := mustRunAuth(t, "add", "prod", "--api-key", "rotated-key", "--yes")
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

// Overwriting a profile destroys an API key that cannot be recovered, so it
// confirms first. The prompt goes to stderr and defaults to No.
func TestAuthAddPromptsBeforeOverwritingProfile(t *testing.T) {
	tests := []struct {
		name      string
		answer    string
		overwrote bool
	}{
		{name: "y", answer: "y\n", overwrote: true},
		{name: "yes", answer: "YES\n", overwrote: true},
		{name: "n", answer: "n\n", overwrote: false},
		{name: "empty defaults to no", answer: "\n", overwrote: false},
		{name: "eof defaults to no", answer: "", overwrote: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			clearCredentialEnv(t)
			mustRunAuth(t, "add", "prod", "--api-key", apiKey, "--api-url", apiURL)

			var out, errOut bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetIn(strings.NewReader(tt.answer))
			cmd.SetArgs([]string{"auth", "add", "prod", "--api-key", "rotated-key"})

			err := cmd.Execute()

			// The prompt must stay off stdout so `-o json` remains parseable.
			if !strings.Contains(errOut.String(), "Are you sure? [y/N]") {
				t.Fatalf("prompt missing from stderr: %q", errOut.String())
			}
			if strings.Contains(out.String(), "Are you sure?") {
				t.Fatalf("prompt leaked to stdout: %q", out.String())
			}
			// The prompt describes the overwrite; it must not print either key.
			if strings.Contains(errOut.String(), apiKey) || strings.Contains(errOut.String(), "rotated-key") {
				t.Fatalf("prompt printed a secret: %q", errOut.String())
			}

			cfg, loadErr := config.Load()
			if loadErr != nil {
				t.Fatalf("load failed: %v", loadErr)
			}
			stored := cfg.Profiles["prod"].APIKey

			if tt.overwrote {
				if err != nil {
					t.Fatalf("add failed: %v", err)
				}
				if stored != "rotated-key" {
					t.Fatalf("expected the key to be replaced, got %q", stored)
				}
				return
			}
			if !errors.Is(err, clihelpers.ErrAborted) {
				t.Fatalf("expected ErrAborted, got %v", err)
			}
			if stored != apiKey {
				t.Fatalf("a declined prompt must leave the key alone, got %q", stored)
			}
		})
	}
}

// Creating a profile destroys nothing, so it must not ask.
func TestAuthAddDoesNotPromptForNewProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)
	mustRunAuth(t, "add", "prod", "--api-key", apiKey)

	var errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"auth", "add", "dev", "--api-key", "dev-key"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if strings.Contains(errOut.String(), "Are you sure?") {
		t.Fatalf("unexpected prompt for a new profile: %q", errOut.String())
	}
}

// Supplying the first key for a profile that has none takes nothing away, so it
// must neither prompt nor claim a stored key is being lost.
func TestAuthAddDoesNotPromptWhenProfileHasNoStoredKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	saveTestConfig(t, apiURL, "")

	var errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	// An answerable stdin, so a prompt would abort rather than be refused.
	cmd.SetIn(strings.NewReader("n\n"))
	cmd.SetArgs([]string{"auth", "add", testProfile, "--api-key", apiKey})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if strings.Contains(errOut.String(), "Are you sure?") {
		t.Fatalf("unexpected prompt: %q", errOut.String())
	}
	if strings.Contains(errOut.String(), "cannot be recovered") {
		t.Fatalf("warned about losing a key that was never stored: %q", errOut.String())
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got := cfg.Profiles[testProfile].APIKey; got != apiKey {
		t.Fatalf("unexpected API key: %q", got)
	}
}

// The remediation hints the CLI prints name an existing profile — `auth add
// <profile> --api-key` for a keyless profile, `auth add <profile> --api-url` for a
// rejected endpoint — and neither mentions --yes. Both therefore have to run
// with no terminal attached, or the printed fix is a dead end in cron and CI.
func TestAuthAddRunsHintedFixesWithoutATerminal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// No key and an endpoint the SDK rejects: the state both hints repair.
	saveTestConfig(t, "http://fleet.example.com", "")

	_, err := newConfiguredClient(testCommonFlags(""))
	if err == nil || !strings.Contains(err.Error(), "auth add "+testProfile+" --api-key") {
		t.Fatalf("expected the missing-key hint, got %v", err)
	}
	mustRunAuth(t, "add", testProfile, "--api-key", apiKey)

	_, err = newConfiguredClient(testCommonFlags(""))
	if !errors.Is(err, nvfleetint.ErrInsecureBaseURL) {
		t.Fatalf("expected ErrInsecureBaseURL, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth add "+testProfile+" --api-url") {
		t.Fatalf("expected the API URL hint, got %v", err)
	}
	mustRunAuth(t, "add", testProfile, "--api-url", apiURL)

	if _, err := newConfiguredClient(testCommonFlags("")); err != nil {
		t.Fatalf("the hinted fixes left the profile unusable: %v", err)
	}
}

// A cron or CI job rotating a key gets a clear error naming --yes rather than
// blocking, and the stored key survives until it re-runs with --yes.
func TestAuthAddRefusesToPromptWithoutATerminal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)
	mustRunAuth(t, "add", "prod", "--api-key", apiKey)

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s failed: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devNull.Close() })

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(devNull)
	cmd.SetArgs([]string{"auth", "add", "prod", "--api-key", "rotated-key"})

	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected a refusal to prompt")
	}
	if !strings.Contains(execErr.Error(), "--yes") {
		t.Fatalf("expected the error to name --yes, got %v", execErr)
	}

	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load failed: %v", loadErr)
	}
	if cfg.Profiles["prod"].APIKey != apiKey {
		t.Fatal("the stored key must survive a refused prompt")
	}
}

// A command that is going to fail anyway must not ask first — the prompt would
// imply the overwrite was about to happen.
func TestAuthAddDoesNotPromptWhenNothingWouldChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)
	mustRunAuth(t, "add", "prod", "--api-key", apiKey)

	var errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"auth", "add", "prod"})

	if err := cmd.Execute(); !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(errOut.String(), "Are you sure?") {
		t.Fatalf("unexpected prompt: %q", errOut.String())
	}
}

// The other half of the partial update: --api-url alone keeps the stored key.
// It also must not prompt — mustRunAuth leaves stdin as the test binary's,
// which is not a terminal, so a prompt here would fail the command outright.
func TestAuthAddChangesAPIURLWithoutKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "dev", "--api-key", apiKey)
	mustRunAuth(t, "add", "dev", "--api-url", apiURL)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	profile := cfg.Profiles["dev"]
	if profile.APIURL != apiURL {
		t.Fatalf("unexpected API URL: %q", profile.APIURL)
	}
	if profile.APIKey != apiKey {
		t.Fatalf("changing the API URL wiped the stored key: %q", profile.APIKey)
	}
}

// Re-adding an existing profile with no values is a typo, not a request to
// reset it to defaults — the stored key must survive.
func TestAuthAddExistingProfileRequiresSomethingToChange(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "prod", "--api-key", apiKey, "--api-url", apiURL)

	err := runAuthErr(t, "add", "prod")
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(err.Error(), "--api-key") {
		t.Fatalf("expected the error to name the flags that change it: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got := cfg.Profiles["prod"]; got.APIKey != apiKey || got.APIURL != apiURL {
		t.Fatalf("a rejected add must not disturb the profile: %#v", got)
	}
}

// Updating a profile must not steal the current-profile selection.
func TestAuthAddExistingProfileKeepsCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "prod", "--api-key", apiKey)
	mustRunAuth(t, "add", "dev", "--api-key", "dev-key")

	got := mustRunAuth(t, "add", "dev", "--api-key", "rotated-key", "--yes")
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
	clearCredentialEnv(t)

	got := mustRunAuth(t, "add", "--api-key", apiKey)
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
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "--api-key", apiKey, "--api-url", apiURL)

	got := mustRunAuth(t, "add", "--api-key", "rotated-key", "--yes")
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

// The name is optional; the key is not. Without either there is nothing to
// store, so a bare `auth add` must not create an empty default profile.
func TestAuthAddWithoutNameStillRequiresKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	err := runAuthErr(t, "add")
	if !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load failed: %v", loadErr)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected no profile to be created: %#v", cfg.Profiles)
	}
}

// Only `add` names the profile for you. Defaulting a delete or a switch would
// act on a profile the user never named.
func TestAuthRemoveAndUseStillRequireProfileName(t *testing.T) {
	for _, name := range []string{"remove", "use"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			clearCredentialEnv(t)
			mustRunAuth(t, "add", "--api-key", apiKey)

			err := runAuthErr(t, name)
			if !strings.Contains(err.Error(), "profile name is required") {
				t.Fatalf("unexpected error: %v", err)
			}

			cfg, loadErr := config.Load()
			if loadErr != nil {
				t.Fatalf("load failed: %v", loadErr)
			}
			if _, ok := cfg.Profiles[config.DefaultProfileName]; !ok {
				t.Fatal("the default profile must survive")
			}
		})
	}
}

func TestAuthAddRejectsInvalidProfileName(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthErr(t, "add", "has space", "--api-key", apiKey)
	if !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddRequiresKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthErr(t, "add", "prod")
	if !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddRejectsInvalidAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthErr(t, "add", "prod", "--api-key", apiKey, "--api-url", "example.com")
	if !strings.Contains(err.Error(), "absolute https URL is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A plaintext remote endpoint would put the API key on the wire, so add
// must refuse it and leave no config behind.
func TestAuthAddRejectsInsecureAPIURL(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	err := runAuthErr(t, "add", "prod", "--api-key", apiKey, "--api-url", "http://api.example.com")
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
	mustRunAuth(t, "add", "local", "--api-key", apiKey, "--api-url", loopbackURL)

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if cfg.Profiles["local"].APIURL != loopbackURL {
		t.Fatalf("unexpected API URL: %q", cfg.Profiles["local"].APIURL)
	}
}

// An empty value is rejected rather than treated as "clear this credential":
// `--api-key "$KEY"` with KEY unset must not silently wipe a stored key.
func TestAuthAddRejectsEmptyValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "key", args: []string{"--api-key", ""}, want: "--api-key cannot be empty"},
		{name: "api-url", args: []string{"--api-url", ""}, want: "--api-url cannot be empty"},
		{name: "whitespace key", args: []string{"--api-key", "   "}, want: "--api-key cannot be empty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			clearCredentialEnv(t)
			mustRunAuth(t, "add", "prod", "--api-key", apiKey, "--api-url", apiURL)

			err := runAuthErr(t, append([]string{"add", "prod"}, tt.args...)...)
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("unexpected error: %v", err)
			}

			cfg, loadErr := config.Load()
			if loadErr != nil {
				t.Fatalf("load failed: %v", loadErr)
			}
			if got := cfg.Profiles["prod"]; got.APIKey != apiKey || got.APIURL != apiURL {
				t.Fatalf("a rejected add must not change the profile: %#v", got)
			}
		})
	}
}

// A brand-new profile needs a key; only an existing one can be changed by
// --api-url alone.
func TestAuthAddNewProfileRequiresKeyEvenWithAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	err := runAuthErr(t, "add", "nope", "--api-url", apiURL)
	if !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load failed: %v", loadErr)
	}
	if _, ok := cfg.Profiles["nope"]; ok {
		t.Fatal("a keyless profile must not be created")
	}
}

func TestAuthRemoveCurrentProfileClearsSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "prod", "--api-key", apiKey)
	mustRunAuth(t, "add", "dev", "--api-key", "dev-key")

	got := mustRunAuth(t, "remove", "prod", "--yes")
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

	mustRunAuth(t, "add", "prod", "--api-key", apiKey)
	mustRunAuth(t, "add", "dev", "--api-key", "dev-key")

	got := mustRunAuth(t, "remove", "dev", "--yes")
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

	mustRunAuth(t, "add", "prod", "--api-key", apiKey)

	got := mustRunAuth(t, "remove", "prod", "--yes")
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

	err := runAuthErr(t, "remove", "nope", "--yes")
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Removing a profile destroys an API key that cannot be recovered, so it
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
			mustRunAuth(t, "add", "prod", "--api-key", apiKey)

			var out, errOut bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)
			cmd.SetIn(strings.NewReader(tt.answer))
			cmd.SetArgs([]string{"auth", "remove", "prod"})

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
			if !errors.Is(err, clihelpers.ErrAborted) {
				t.Fatalf("expected ErrAborted, got %v", err)
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
			mustRunAuth(t, "add", "prod", "--api-key", apiKey)

			cmd := newRootCmd()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			cmd.SetIn(tt.open(t))
			cmd.SetArgs([]string{"auth", "remove", "prod"})

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
	mustRunAuth(t, "add", "prod", "--api-key", apiKey)

	var errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("y\n"))
	cmd.SetArgs([]string{"auth", "remove", "nope"})

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

	mustRunAuth(t, "add", "prod", "--api-key", apiKey)
	mustRunAuth(t, "add", "dev", "--api-key", "dev-key")

	got := mustRunAuth(t, "use", "dev")
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

	mustRunAuth(t, "add", "prod", "--api-key", apiKey)

	err := runAuthErr(t, "use", "nope")
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// "nothing is configured" needs a different remedy than "that name is wrong".
func TestAuthUseWithoutAnyProfilesErrorsDistinctly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	err := runAuthErr(t, "use", "prod")
	if !errors.Is(err, config.ErrNoProfile) {
		t.Fatalf("expected ErrNoProfile, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth add prod") {
		t.Fatalf("expected a setup hint naming the profile: %v", err)
	}
}

func TestAuthListShowsProfilesWithoutPrintingKeys(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "prod", "--api-key", apiKey, "--api-url", apiURL)
	mustRunAuth(t, "add", "dev", "--api-key", "dev-key")

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
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "prod", "--api-key", apiKey, "--api-url", apiURL)

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

func TestAuthListMarksEnvSelectedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	mustRunAuth(t, "add", "prod", "--api-key", "prod-key")
	mustRunAuth(t, "add", "dev", "--api-key", "dev-key")
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
	clearCredentialEnv(t)

	got := mustRunAuth(t, "list")
	if !strings.Contains(got, "No profiles configured") {
		t.Fatalf("unexpected output: %q", got)
	}
	if !strings.Contains(got, "auth add <name>") {
		t.Fatalf("expected a setup hint: %q", got)
	}
}

func TestAuthStatusChecksConnectionAndDoesNotPrintSecret(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	saveTestConfig(t, server.URL, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "Profile: "+testProfile) {
		t.Fatalf("status missing profile: %q", got)
	}
	if !strings.Contains(got, "API URL: "+server.URL) {
		t.Fatalf("status missing API URL: %q", got)
	}
	if !strings.Contains(got, "API key: configured") {
		t.Fatalf("status missing API key state: %q", got)
	}
	if !strings.Contains(got, "Connection: ok") {
		t.Fatalf("status missing connection state: %q", got)
	}
	if strings.Contains(got, apiKey) {
		t.Fatalf("status printed secret: %q", got)
	}
}

// `auth status` is the command that explains credential resolution, so it has
// to run — and say what happened — when current_profile points at nothing.
func TestAuthStatusReportsGoneCurrentProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	if err := config.Save(config.Config{CurrentProfile: "gone"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}
	t.Setenv(config.EnvAPIURL, server.URL)
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, `current profile "gone" is no longer stored`) {
		t.Fatalf("status did not report the stale selection: %q", got)
	}
	if !strings.Contains(got, "Connection: ok") {
		t.Fatalf("status did not fall back to the environment: %q", got)
	}
}

// An unreadable config that resolution recovered from must not look like an
// absent one: `auth list` fails on the same file, so hiding it here is worse
// than useless.
func TestAuthStatusReportsUnreadableConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	path, err := config.Path()
	if err != nil {
		t.Fatalf("config path failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("profiles: [\n"), 0o600); err != nil {
		t.Fatalf("write config failed: %v", err)
	}
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "read config") {
		t.Fatalf("status hid the unreadable config: %q", got)
	}
	if !strings.Contains(got, "API key: configured (from environment)") {
		t.Fatalf("status did not resolve from the environment: %q", got)
	}
}

// The warnings are part of the answer, so JSON consumers get them too.
func TestAuthStatusJSONIncludesWarnings(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	if err := config.Save(config.Config{CurrentProfile: "gone"}); err != nil {
		t.Fatalf("save config failed: %v", err)
	}

	var status authStatusOutput
	if err := json.Unmarshal([]byte(mustRunAuth(t, "status", "-o", "json")), &status); err != nil {
		t.Fatalf("decode status failed: %v", err)
	}
	if len(status.Warnings) == 0 {
		t.Fatal("expected a warning about the stale current profile")
	}
	if !strings.Contains(strings.Join(status.Warnings, "\n"), `"gone"`) {
		t.Fatalf("warning did not name the stale profile: %v", status.Warnings)
	}
}

// `auth status --profile` must report the named profile, not the current one.
func TestAuthStatusChecksNamedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	clearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustRunAuth(t, "add", "prod", "--api-key", "prod-key")
	mustRunAuth(t, "add", "dev", "--api-key", apiKey, "--api-url", server.URL)

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

	mustRunAuth(t, "add", "dev", "--api-key", apiKey, "--api-url", server.URL)
	t.Setenv(config.EnvAPIKey, "stale-key")

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

	mustRunAuth(t, "add", "dev", "--api-key", apiKey, "--api-url", server.URL)
	t.Setenv(config.EnvAPIKey, "stale-key")

	got := mustRunAuth(t, "status", "--profile", "dev")
	if !strings.Contains(got, config.EnvAPIKey+" is set but ignored") {
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
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, config.EnvAPIKey+" overrides the value stored in profile") {
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
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "API URL: "+server.URL+" (from environment)") {
		t.Fatalf("expected the environment to win: %q", got)
	}
	if !strings.Contains(got, config.EnvAPIURL) || !strings.Contains(got, config.EnvAPIKey) {
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

	saveTestConfig(t, server.URL, apiKey)

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
	if !strings.Contains(got, "API key: not configured") {
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
	t.Setenv(config.EnvAPIKey, apiKey)

	out := mustRunAuth(t, "status", "--output", "json")
	var got authStatusOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("decode status JSON failed: %v", err)
	}
	if got.APIURL != server.URL || !got.APIKeyConfigured || got.Connection != "ok" {
		t.Fatalf("unexpected status JSON: %#v", got)
	}
	if got.APIKeySource != string(config.SourceEnvironment) {
		t.Fatalf("expected the environment to be named as the source: %#v", got)
	}
	if strings.Contains(out, apiKey) {
		t.Fatalf("status printed secret: %q", out)
	}
}

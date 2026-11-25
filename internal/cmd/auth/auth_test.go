// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

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

	"github.com/spf13/cobra"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// testCommonFlags is the resolved common flags a client is built from when the
// test is checking credential resolution rather than a command's own flags.
func testCommonFlags(profile string) cmdutil.Resolved {
	return cmdutil.Resolved{
		Timeout:    nvfleetint.DefaultTimeout,
		Profile:    profile,
		ProfileSet: profile != "",
	}
}

// newRootCmd builds a root command carrying only this package's commands, so
// the tests drive them through the same argument path a user types.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nvfleetint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(NewCmd())
	return root
}

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
	return runAuthInputErr(t, "", args...)
}

// runAuthInputErr executes an auth subcommand with input on stdin, expecting
// it to fail.
func runAuthInputErr(t *testing.T, input string, args ...string) error {
	t.Helper()
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader(input))
	cmd.SetArgs(append([]string{"auth"}, args...))

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("expected auth %s to fail", strings.Join(args, " "))
	}

	return err
}

// addAnswers is what `auth add` reads from a piped stdin: the API key, then
// the API URL. An empty answer keeps the stored (or default) value.
func addAnswers(apiKey, apiURL string) string {
	return apiKey + "\n" + apiURL + "\n"
}

// mustAddProfile stores a profile through `auth add`, answering its prompts on
// stdin. The reader is not a terminal, so this exercises the piped form —
// which is also why replacing a stored key here needs an explicit "--yes".
func mustAddProfile(t *testing.T, name, apiKey, apiURL string, extra ...string) string {
	t.Helper()

	args := []string{"auth", "add"}
	if name != "" {
		args = append(args, name)
	}
	args = append(args, extra...)

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(addAnswers(apiKey, apiURL)))
	cmd.SetArgs(args)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth add %s failed: %v", strings.Join(args[2:], " "), err)
	}

	return out.String()
}

// The profile-naming commands take exactly one <name>; a second word is a typo
// (an unquoted name, a stray flag value) and must not be silently ignored.
func TestAuthSubcommandsRejectExtraProfileNames(t *testing.T) {
	tests := [][]string{
		{"add", "prod", "extra"},
		{"remove", "prod", "extra", "--yes"},
		{"use", "prod", "extra"},
	}

	for _, args := range tests {
		t.Run(args[0], func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmdtest.ClearCredentialEnv(t)

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
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, "")

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

// The credentials are no longer flags. Accepting them again would put the key
// back in shell history, and silently ignoring the flag would store the wrong
// thing, so both have to be rejected outright.
func TestAuthAddRejectsCredentialFlags(t *testing.T) {
	tests := [][]string{
		{"add", "prod", "--api-key", "secret"},
		{"add", "prod", "--api-url", apiURL},
	}

	for _, args := range tests {
		t.Run(args[2], func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmdtest.ClearCredentialEnv(t)

			err := runAuthErr(t, args...)
			if !strings.Contains(err.Error(), "unknown flag: "+args[2]) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// The questions belong on stderr, and the answers must never be echoed back:
// the whole point of reading the key from stdin is that it leaves no trace.
func TestAuthAddPromptsOnStderrWithoutEchoingTheKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)

	var out, errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(addAnswers(apiKey, "")))
	cmd.SetArgs([]string{"auth", "add", "prod"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	for _, want := range []string{"API key:", "API URL [production: " + config.DefaultAPIURL + "]:"} {
		if !strings.Contains(errOut.String(), want) {
			t.Fatalf("prompt %q missing from stderr: %q", want, errOut.String())
		}
	}
	if strings.Contains(out.String(), "API key:") {
		t.Fatalf("prompt leaked to stdout: %q", out.String())
	}
	if strings.Contains(out.String()+errOut.String(), apiKey) {
		t.Fatalf("the key was echoed back: %q / %q", out.String(), errOut.String())
	}
}

// Pressing Enter at the key prompt keeps the stored key, so changing an
// endpoint never means re-entering — or seeing — the current credential.
func TestAuthAddKeepsStoredKeyOnEmptyAnswer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, apiURL)

	var errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(addAnswers("", "https://other.example.com")))
	cmd.SetArgs([]string{"auth", "add", "prod"})

	// Keeping the key destroys nothing, so this must not need --yes.
	if err := cmd.Execute(); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if !strings.Contains(errOut.String(), "keep the stored key") {
		t.Fatalf("expected the prompt to offer keeping the key: %q", errOut.String())
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	profile := cfg.Profiles["prod"]
	if profile.APIKey != apiKey {
		t.Fatalf("an empty answer wiped the stored key: %q", profile.APIKey)
	}
	if profile.APIURL != "https://other.example.com" {
		t.Fatalf("unexpected API URL: %q", profile.APIURL)
	}
}

// With input piped in nobody sees the warning, so replacing a stored key needs
// --yes. The key must survive the refusal.
func TestAuthAddPipedOverwriteRequiresYes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, apiURL)

	err := runAuthInputErr(t, addAnswers("rotated-key", ""), "add", "prod")
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("expected the error to name --yes, got %v", err)
	}

	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load failed: %v", loadErr)
	}
	if cfg.Profiles["prod"].APIKey != apiKey {
		t.Fatal("the stored key must survive a refused overwrite")
	}
}

// Creating a profile destroys nothing, so it must not need --yes.
func TestAuthAddNewProfileNeedsNoConfirmation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, "")

	var errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(addAnswers("dev-key", "")))
	cmd.SetArgs([]string{"auth", "add", "dev"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if strings.Contains(errOut.String(), "already exists") {
		t.Fatalf("unexpected overwrite warning for a new profile: %q", errOut.String())
	}
}

// Supplying the first key for a profile that has none takes nothing away, so it
// must neither need --yes nor claim a stored key is being lost.
func TestAuthAddDoesNotWarnWhenProfileHasNoStoredKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.SaveConfig(t, apiURL, "")

	var errOut bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(addAnswers(apiKey, "")))
	cmd.SetArgs([]string{"auth", "add", cmdtest.Profile})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("add failed: %v", err)
	}
	if strings.Contains(errOut.String(), "cannot be recovered") {
		t.Fatalf("warned about losing a key that was never stored: %q", errOut.String())
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if got := cfg.Profiles[cmdtest.Profile].APIKey; got != apiKey {
		t.Fatalf("unexpected API key: %q", got)
	}
}

// The remediation hints the CLI prints name an existing profile — `auth add
// <profile>` for a keyless profile and for a rejected endpoint — and neither
// mentions --yes. Both therefore have to run with input piped in, or the
// printed fix is a dead end in cron and CI.
func TestAuthAddRunsHintedFixesWithoutATerminal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// No key and an endpoint the SDK rejects: the state both hints repair.
	cmdtest.SaveConfig(t, "http://fleet.example.com", "")

	_, err := cmdutil.New(testCommonFlags(""))
	if err == nil || !strings.Contains(err.Error(), "auth add "+cmdtest.Profile) {
		t.Fatalf("expected the missing-key hint, got %v", err)
	}
	// The stored endpoint is the one the SDK rejects, so accepting it as the
	// offered value has to fail rather than write it back.
	keyErr := runAuthInputErr(t, addAnswers(apiKey, ""), "add", cmdtest.Profile)
	if !errors.Is(keyErr, nvfleetint.ErrInsecureBaseURL) {
		t.Fatalf("expected the stored endpoint to be re-validated, got %v", keyErr)
	}
	mustAddProfile(t, cmdtest.Profile, apiKey, apiURL)

	if _, err := cmdutil.New(testCommonFlags("")); err != nil {
		t.Fatalf("the hinted fix left the profile unusable: %v", err)
	}
}

// A cron or CI job rotating a key gets a clear error naming --yes rather than
// blocking, and the stored key survives until it re-runs with --yes. /dev/null
// is the case a character-device check gets wrong: it looks like a terminal by
// mode, so `auth add prod </dev/null` would be treated as a conversation.
func TestAuthAddRefusesToReplaceStoredKeyWithoutATerminal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, "")

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	t.Cleanup(func() { _ = reader.Close() })
	if _, err := writer.WriteString(addAnswers("rotated-key", "")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	_ = writer.Close()

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(reader)
	cmd.SetArgs([]string{"auth", "add", "prod"})

	execErr := cmd.Execute()
	if execErr == nil {
		t.Fatal("expected a refusal to replace the stored key")
	}
	if !strings.Contains(execErr.Error(), "--yes") {
		t.Fatalf("expected the error to name --yes, got %v", execErr)
	}

	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load failed: %v", loadErr)
	}
	if cfg.Profiles["prod"].APIKey != apiKey {
		t.Fatal("the stored key must survive a refused replacement")
	}
}

// Keeping both offered values is a legitimate answer, not a failure: it is what
// pressing Enter twice does. Nothing is written and nothing is destroyed.
func TestAuthAddReportsAnUnchangedProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

// A closed stdin answers nothing, so an existing profile is left alone rather
// than reset to defaults or stripped of its key.
func TestAuthAddWithClosedStdinLeavesProfileAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, apiURL)

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open %s failed: %v", os.DevNull, err)
	}
	t.Cleanup(func() { _ = devNull.Close() })

	cmd := newRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(devNull)
	cmd.SetArgs([]string{"auth", "add", "prod"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("add failed: %v", err)
	}

	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load failed: %v", loadErr)
	}
	if got := cfg.Profiles["prod"]; got.APIKey != apiKey || got.APIURL != apiURL {
		t.Fatalf("an unanswered add must not disturb the profile: %#v", got)
	}
}

// The other half of the partial update: a new URL alone keeps the stored key.
func TestAuthAddChangesAPIURLWithoutKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "dev", apiKey, "")
	mustAddProfile(t, "dev", "", apiURL)

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

// Updating a profile must not steal the current-profile selection.
func TestAuthAddExistingProfileKeepsCurrent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

// The name is optional; the key is not. Without either there is nothing to
// store, so a bare `auth add` must not create an empty default profile.
func TestAuthAddWithoutNameStillRequiresKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)

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
			cmdtest.ClearCredentialEnv(t)
			mustAddProfile(t, "", apiKey, "")

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

// An invalid name is caught before anything is asked: a question about a
// profile that cannot exist is worse than no question at all.
func TestAuthAddRejectsInvalidProfileNameBeforePrompting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cmd := newRootCmd()
	var errOut bytes.Buffer
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(addAnswers(apiKey, "")))
	cmd.SetArgs([]string{"auth", "add", "has space"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid profile name") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(errOut.String(), "API key:") {
		t.Fatalf("asked for a key it could not store: %q", errOut.String())
	}
}

func TestAuthAddRequiresKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthInputErr(t, "\n\n", "add", "prod")
	if !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddRejectsInvalidAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	err := runAuthInputErr(t, addAnswers(apiKey, "example.com"), "add", "prod")
	if !strings.Contains(err.Error(), "absolute https URL is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// A plaintext remote endpoint would put the API key on the wire, so add
// must refuse it and leave no config behind.
func TestAuthAddRejectsInsecureAPIURL(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	err := runAuthInputErr(t, addAnswers(apiKey, "http://api.example.com"), "add", "prod")
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

// An answer of nothing but whitespace is an empty answer — `echo "$KEY" | ...`
// with KEY unset keeps the stored key rather than wiping it.
func TestAuthAddTreatsWhitespaceAnswersAsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, apiURL)

	got := mustAddProfile(t, "prod", "   ", "  ")
	if !strings.Contains(got, `Profile "prod" unchanged.`) {
		t.Fatalf("unexpected output: %q", got)
	}

	cfg, loadErr := config.Load()
	if loadErr != nil {
		t.Fatalf("load failed: %v", loadErr)
	}
	if got := cfg.Profiles["prod"]; got.APIKey != apiKey || got.APIURL != apiURL {
		t.Fatalf("a blank answer must not change the profile: %#v", got)
	}
}

// A brand-new profile needs a key; only an existing one has a key to keep.
func TestAuthAddNewProfileRequiresKeyEvenWithAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)

	err := runAuthInputErr(t, addAnswers("", apiURL), "add", "nope")
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
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, "")
	mustAddProfile(t, "dev", "dev-key", "")

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
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, "")
	mustAddProfile(t, "dev", "dev-key", "")

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
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, "")

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
	cmdtest.ClearCredentialEnv(t)

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
			cmdtest.ClearCredentialEnv(t)
			mustAddProfile(t, "prod", apiKey, "")

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
			if !errors.Is(err, cmdutil.ErrAborted) {
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
			cmdtest.ClearCredentialEnv(t)
			mustAddProfile(t, "prod", apiKey, "")

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
	cmdtest.ClearCredentialEnv(t)
	mustAddProfile(t, "prod", apiKey, "")

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
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, "")
	mustAddProfile(t, "dev", "dev-key", "")

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
	cmdtest.ClearCredentialEnv(t)

	mustAddProfile(t, "prod", apiKey, "")

	err := runAuthErr(t, "use", "nope")
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// "nothing is configured" needs a different remedy than "that name is wrong".
func TestAuthUseWithoutAnyProfilesErrorsDistinctly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)

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
	cmdtest.ClearCredentialEnv(t)

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

	cmdtest.SaveConfig(t, server.URL, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "Profile: "+cmdtest.Profile) {
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
	cmdtest.ClearCredentialEnv(t)

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
	cmdtest.ClearCredentialEnv(t)

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
	cmdtest.ClearCredentialEnv(t)

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
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustAddProfile(t, "prod", "prod-key", "")
	mustAddProfile(t, "dev", apiKey, server.URL)

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
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustAddProfile(t, "dev", apiKey, server.URL)
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
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	mustAddProfile(t, "dev", apiKey, server.URL)
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
	cmdtest.ClearCredentialEnv(t)

	server := newAuthStatusServer(t, http.StatusOK, `{"authenticated":true}`)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "stored-key")
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

	cmdtest.SaveConfig(t, "https://stored.example.com", "stored-key")
	t.Setenv(config.EnvAPIURL, server.URL)
	t.Setenv(config.EnvAPIKey, apiKey)

	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "API URL: "+server.URL+" (from environment)") {
		t.Fatalf("expected the environment to win: %q", got)
	}
	if !strings.Contains(got, config.EnvAPIURL) || !strings.Contains(got, config.EnvAPIKey) {
		t.Fatalf("status did not name the shadowing variables: %q", got)
	}
	if !strings.Contains(got, `profile "`+cmdtest.Profile+`"`) {
		t.Fatalf("status did not name the shadowed profile: %q", got)
	}
}

func TestAuthStatusRejectsUnknownProfile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)

	err := runAuthErr(t, "status", "--profile", "nope")
	if !strings.Contains(err.Error(), "profile not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthStatusReportsUnauthorizedOnRejectedKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	server := newAuthStatusServer(t, http.StatusUnauthorized, `{"error":"unauthorized"}`)
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, apiKey)

	// A rejected key is a reportable status, not a command failure.
	got := mustRunAuth(t, "status")
	if !strings.Contains(got, "Connection: unauthorized") {
		t.Fatalf("status missing unauthorized state: %q", got)
	}
}

func TestAuthStatusWithoutConfigExitsZero(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)

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
	cmdtest.ClearCredentialEnv(t)

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

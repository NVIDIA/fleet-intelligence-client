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
	"sync/atomic"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

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

func TestAuthMutationsRejectInvalidProfileName(t *testing.T) {
	for _, action := range []string{"remove", "use"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmdtest.ClearCredentialEnv(t)
			cmdtest.SaveConfig(t, apiURL, apiKey)

			var stdout, stderr bytes.Buffer
			cmd := newRootCmd()
			cmd.SetIn(strings.NewReader("yes\n"))
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"auth", action, "has space"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), `invalid profile name "has space"`) {
				t.Fatalf("expected invalid profile name error, got %v", err)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected output or prompt: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("reload config: %v", err)
			}
			if cfg.CurrentProfile != cmdtest.Profile || len(cfg.Profiles) != 1 || cfg.Profiles[cmdtest.Profile] != (config.Profile{APIURL: apiURL, APIKey: apiKey}) {
				t.Fatalf("profile or selection changed: %#v", cfg)
			}
		})
	}
}

func TestAuthMutationsRejectMalformedConfigBeforePrompt(t *testing.T) {
	for _, action := range []string{"add", "remove"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmdtest.ClearCredentialEnv(t)
			path, err := config.Path()
			if err != nil {
				t.Fatalf("config path: %v", err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			malformed := []byte("profiles: [\n")
			if err := os.WriteFile(path, malformed, 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}

			var stdout, stderr bytes.Buffer
			cmd := newRootCmd()
			cmd.SetIn(strings.NewReader("yes\nhttps://example.com\n"))
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"auth", action, "prod"})
			err = cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), "read config") || !strings.Contains(err.Error(), path) {
				t.Fatalf("expected config read error identifying %q, got %v", path, err)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected output or prompt: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read config: %v", err)
			}
			if !bytes.Equal(got, malformed) {
				t.Fatalf("malformed config changed: got %q, want %q", got, malformed)
			}
		})
	}
}

func TestAuthReadCommandsRejectInvalidOutput(t *testing.T) {
	for _, action := range []string{"list", "status"} {
		t.Run(action, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmdtest.ClearCredentialEnv(t)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, apiKey)

			var stdout, stderr bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"auth", action, "--output", "xml"})
			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), `invalid output "xml"`) {
				t.Fatalf("expected invalid output error, got %v", err)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("unexpected result output: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if got := requests.Load(); got != 0 {
				t.Fatalf("made %d API requests, want none", got)
			}
		})
	}
}

func TestAuthStatusReportsUnsuccessfulConnectionsJSON(t *testing.T) {
	tests := []struct {
		name             string
		code             int
		body, connection string
	}{
		{"unauthenticated", http.StatusOK, `{"authenticated":false}`, "unauthenticated"},
		{"forbidden", http.StatusForbidden, `{"error":"forbidden"}`, "unauthorized"},
		{"server error", http.StatusInternalServerError, `{"error":"server error"}`, "error:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmdtest.ClearCredentialEnv(t)
			server := newAuthStatusServer(t, tt.code, tt.body)
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, apiKey)
			out := mustRunAuth(t, "status", "--output", "json")
			var got authStatusOutput
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("decode status JSON: %v", err)
			}
			if got.Profile != cmdtest.Profile || got.APIURL != server.URL || !got.APIKeyConfigured {
				t.Fatalf("unexpected profile status: %#v", got)
			}
			if tt.code == http.StatusInternalServerError {
				if !strings.HasPrefix(got.Connection, tt.connection) || !strings.Contains(got.Connection, "500") {
					t.Fatalf("unexpected connection: %q", got.Connection)
				}
			} else if got.Connection != tt.connection {
				t.Fatalf("connection = %q, want %q", got.Connection, tt.connection)
			}
			if strings.Contains(out, apiKey) {
				t.Fatalf("status exposed the key: %q", out)
			}
		})
	}
}

type authListErrorWriter struct{ err error }

func (w authListErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestAuthListPropagatesOutputError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

// authFirstRead invokes the change after the command's initial config read,
// but before it receives any prompt answers.
type authFirstRead struct {
	reader *strings.Reader
	once   func()
}

func (r *authFirstRead) Read(p []byte) (int, error) {
	if r.once != nil {
		callback := r.once
		r.once = nil
		callback()
	}
	return r.reader.Read(p)
}

func TestAuthProfileChangesDuringPrompt(t *testing.T) {
	const name = "target"
	const storedKey = "concurrently-stored-secret"
	const newKey = "new-input-secret"
	const storedURL = "https://stored.example.com"
	const newURL = "https://changed.example.com"

	tests := []struct {
		name, action, input string
		initial             bool
		change              func(*config.Config) error
	}{
		{
			name: "key added while adding", action: "add", input: addAnswers(newKey, newURL),
			change: func(cfg *config.Config) error {
				return cfg.AddProfile(name, config.Profile{APIKey: storedKey, APIURL: storedURL})
			},
		},
		{
			name: "profile removed while adding", action: "add", input: addAnswers("", newURL), initial: true,
			change: func(cfg *config.Config) error { return cfg.RemoveProfile(name) },
		},
		{
			name: "profile removed while confirming removal", action: "remove", input: "yes\n", initial: true,
			change: func(cfg *config.Config) error { return cfg.RemoveProfile(name) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmdtest.ClearCredentialEnv(t)
			if tt.initial {
				if err := config.Save(config.Config{CurrentProfile: name, Profiles: map[string]config.Profile{
					name: {APIKey: storedKey, APIURL: storedURL},
				}}); err != nil {
					t.Fatalf("save initial config: %v", err)
				}
			}

			calls := 0
			input := &authFirstRead{reader: strings.NewReader(tt.input), once: func() {
				calls++
				if _, err := config.Edit(tt.change); err != nil {
					t.Fatalf("edit config during prompt: %v", err)
				}
			}}
			var stdout, stderr bytes.Buffer
			cmd := newRootCmd()
			cmd.SetIn(input)
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs([]string{"auth", tt.action, name})
			err := cmd.Execute()
			if calls != 1 {
				t.Fatalf("prompt callback ran %d times, want 1", calls)
			}
			if err == nil {
				t.Fatal("expected command to fail")
			}
			switch tt.name {
			case "key added while adding":
				if !strings.Contains(err.Error(), "gained an API key") || !strings.Contains(err.Error(), "confirm replacing it") {
					t.Fatalf("expected confirmation error, got %v", err)
				}
			case "profile removed while adding":
				if !strings.Contains(err.Error(), "API key is required") {
					t.Fatalf("expected API key required error, got %v", err)
				}
			case "profile removed while confirming removal":
				if !errors.Is(err, config.ErrProfileNotFound) || !strings.Contains(err.Error(), "nvfleetint auth list") {
					t.Fatalf("expected profile-not-found error with list hint, got %v", err)
				}
			}
			output := stdout.String() + stderr.String()
			for _, forbidden := range []string{" added.", " updated.", " removed.", storedKey, newKey} {
				if strings.Contains(output, forbidden) {
					t.Fatalf("unexpected success or secret in output: %q", output)
				}
			}
			cfg, err := config.Load()
			if err != nil {
				t.Fatalf("reload config: %v", err)
			}
			profile, err := cfg.Profile(name)
			if !tt.initial {
				if err != nil || profile.APIKey != storedKey || profile.APIURL != storedURL {
					t.Fatalf("concurrent profile changed: profile = %#v, error = %v", profile, err)
				}
			} else if !errors.Is(err, config.ErrProfileNotFound) {
				t.Fatalf("expected profile to remain absent, got profile = %#v, error = %v", profile, err)
			}
		})
	}
}

func TestAuthStatusReportsInsecureURLJSON(t *testing.T) {
	const name = "selected"
	const secret = "status-insecure-url-secret"
	const insecureURL = "http://example.com"
	tests := []struct {
		name, urlSource, keySource, hint string
		environment                      bool
	}{
		{"stored profile", string(config.SourceProfile), string(config.SourceProfile), "nvfleetint auth add " + name, false},
		{"environment override", string(config.SourceEnvironment), string(config.SourceEnvironment), "update " + config.EnvAPIURL, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			cmdtest.ClearCredentialEnv(t)
			storedURL := insecureURL
			if tt.environment {
				storedURL = "https://stored.example.com"
			}
			if err := config.Save(config.Config{CurrentProfile: name, Profiles: map[string]config.Profile{
				name: {APIURL: storedURL, APIKey: secret},
			}}); err != nil {
				t.Fatalf("save config: %v", err)
			}
			if tt.environment {
				t.Setenv(config.EnvAPIURL, insecureURL)
				t.Setenv(config.EnvAPIKey, secret)
			}
			out := mustRunAuth(t, "status", "--output", "json")
			var got authStatusOutput
			if err := json.Unmarshal([]byte(out), &got); err != nil {
				t.Fatalf("decode status JSON: %v", err)
			}
			if got.Profile != name || got.APIURL != insecureURL || got.APIURLSource != tt.urlSource || !got.APIKeyConfigured || got.APIKeySource != tt.keySource {
				t.Fatalf("unexpected resolved status: %#v", got)
			}
			if !strings.HasPrefix(got.Connection, "error:") || !strings.Contains(got.Connection, "insecure base URL: https is required") || !strings.Contains(got.Connection, tt.hint) {
				t.Fatalf("unexpected connection diagnostic: %q", got.Connection)
			}
			if strings.Contains(out, secret) {
				t.Fatalf("status exposed the key: %q", out)
			}
		})
	}
}

func TestAuthRemoveConfigBecomesMalformedDuringConfirmation(t *testing.T) {
	const name = "target"
	const secret = "removal-malformed-config-secret"
	t.Setenv("HOME", t.TempDir())
	cmdtest.ClearCredentialEnv(t)
	if err := config.Save(config.Config{CurrentProfile: name, Profiles: map[string]config.Profile{
		name: {APIURL: "https://stored.example.com", APIKey: secret},
	}}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	path, err := config.Path()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	malformed := []byte("profiles: [\n")
	calls := 0
	input := &authFirstRead{reader: strings.NewReader("yes\n"), once: func() {
		calls++
		if err := os.WriteFile(path, malformed, 0o600); err != nil {
			t.Fatalf("replace config during confirmation: %v", err)
		}
	}}
	var stdout, stderr bytes.Buffer
	cmd := newRootCmd()
	cmd.SetIn(input)
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"auth", "remove", name})
	err = cmd.Execute()
	if calls != 1 {
		t.Fatalf("confirmation callback ran %d times, want 1", calls)
	}
	if err == nil || !strings.Contains(err.Error(), "read config") || !strings.Contains(err.Error(), path) || errors.Is(err, config.ErrProfileNotFound) || strings.Contains(err.Error(), "nvfleetint auth list") {
		t.Fatalf("expected config read error without profile-not-found hint, got %v", err)
	}
	output := stdout.String() + stderr.String() + err.Error()
	if strings.Contains(output, " removed.") || strings.Contains(output, secret) {
		t.Fatalf("unexpected success or secret in output: %q", output)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !bytes.Equal(got, malformed) {
		t.Fatalf("malformed config changed: got %q, want %q", got, malformed)
	}
}

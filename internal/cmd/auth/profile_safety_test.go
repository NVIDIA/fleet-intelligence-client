// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

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
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

func TestAuthMutationsRejectMalformedConfigBeforePrompt(t *testing.T) {
	for _, action := range []string{"add", "remove"} {
		t.Run(action, func(t *testing.T) {
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

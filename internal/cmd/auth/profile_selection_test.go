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
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

// `auth update` was removed in favor of `add`. A removed subcommand must fail
// loudly: cobra's default for a parent with no RunE is to print help and exit
// 0, which would let a key-rotation script report success having rotated
// nothing.
func TestAuthRejectsRemovedUpdateSubcommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

// Only `add` names the profile for you. Defaulting a delete or a switch would
// act on a profile the user never named.
func TestAuthRemoveAndUseStillRequireProfileName(t *testing.T) {
	for _, name := range []string{"remove", "use"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

func TestAuthRemoveCurrentProfileClearsSelection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	cmdtest.ClearCredentialEnv(t)

	err := runAuthErr(t, "use", "prod")
	if !errors.Is(err, config.ErrNoProfile) {
		t.Fatalf("expected ErrNoProfile, got %v", err)
	}
	if !strings.Contains(err.Error(), "auth add prod") {
		t.Fatalf("expected a setup hint naming the profile: %v", err)
	}
}

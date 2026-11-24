// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

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
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			cmdtest.ClearCredentialEnv(t)

			err := runAuthErr(t, args...)
			if !strings.Contains(err.Error(), "unknown flag: "+args[2]) {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// With input piped in nobody sees the warning, so replacing a stored key needs
// --yes. The key must survive the refusal.
func TestAuthAddPipedOverwriteRequiresYes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

// A cron or CI job rotating a key gets a clear error naming --yes rather than
// blocking, and the stored key survives until it re-runs with --yes. /dev/null
// is the case a character-device check gets wrong: it looks like a terminal by
// mode, so `auth add prod </dev/null` would be treated as a conversation.
func TestAuthAddRefusesToReplaceStoredKeyWithoutATerminal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

// The name is optional; the key is not. Without either there is nothing to
// store, so a bare `auth add` must not create an empty default profile.
func TestAuthAddWithoutNameStillRequiresKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

// An invalid name is caught before anything is asked: a question about a
// profile that cannot exist is worse than no question at all.
func TestAuthAddRejectsInvalidProfileNameBeforePrompting(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	err := runAuthInputErr(t, "\n\n", "add", "prod")
	if !strings.Contains(err.Error(), "API key is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuthAddRejectsInvalidAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	err := runAuthInputErr(t, addAnswers(apiKey, "http://api.example.com"), "add", "prod")
	if !strings.Contains(err.Error(), "https is required") {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(homeDir, ".config", "nvfleetint", "config.yaml")
	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Fatalf("expected no config file to be written, stat returned %v", statErr)
	}
}

// A brand-new profile needs a key; only an existing one has a key to keep.
func TestAuthAddNewProfileRequiresKeyEvenWithAPIURL(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

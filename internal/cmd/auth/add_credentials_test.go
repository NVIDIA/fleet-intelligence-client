// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// The questions belong on stderr, and the answers must never be echoed back:
// the whole point of reading the key from stdin is that it leaves no trace.
func TestAuthAddPromptsOnStderrWithoutEchoingTheKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

// Supplying the first key for a profile that has none takes nothing away, so it
// must neither need --yes nor claim a stored key is being lost.
func TestAuthAddDoesNotWarnWhenProfileHasNoStoredKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

// A closed stdin answers nothing, so an existing profile is left alone rather
// than reset to defaults or stripped of its key.
func TestAuthAddWithClosedStdinLeavesProfileAlone(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

// An answer of nothing but whitespace is an empty answer — `echo "$KEY" | ...`
// with KEY unset keeps the stored key rather than wiping it.
func TestAuthAddTreatsWhitespaceAnswersAsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
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

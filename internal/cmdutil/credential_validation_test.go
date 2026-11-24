// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
)

func TestCredentialWarningsLegacyKeyAndCurrentKey(t *testing.T) {
	const legacySecret = "legacy-test-secret"
	t.Setenv(config.EnvLegacyAPIKey, legacySecret)
	t.Setenv(config.EnvAPIKey, "")
	resolved := config.Resolved{
		ConfigError:           errors.New("configuration could not be read"),
		MissingCurrentProfile: "stale-profile",
	}
	want := []string{
		"configuration could not be read; continuing with credentials from the environment",
		"current profile \"stale-profile\" is no longer stored; run `nvfleetint auth use <name>`",
		"NVFLEETINT_SERVICE_KEY is set but no longer read; it was renamed to NVFLEETINT_API_KEY",
	}
	check := func(got, expected []string) {
		t.Helper()
		if len(got) != len(expected) {
			t.Errorf("warnings = %q, want %q", got, expected)
		}
		for i, warning := range got {
			if strings.Contains(warning, legacySecret) {
				t.Errorf("warning %d exposed legacy secret", i)
			}
			if i < len(expected) && warning != expected[i] {
				t.Errorf("warning %d = %q, want %q", i, warning, expected[i])
			}
		}
	}
	check(CredentialWarnings(resolved), want)
	t.Setenv(config.EnvAPIKey, "current-test-key")
	check(CredentialWarnings(resolved), want[:2])
}

func TestPromptsPropagateInputErrors(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "no input"},
		{name: "partial answer", input: "yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sentinel := errors.New("controlled prompt read failure")
			t.Run("APIKey", func(t *testing.T) {
				var out bytes.Buffer
				prompt := NewCredentialPrompt(&promptErrorReader{input: tc.input, err: sentinel}, &out)
				key, set, err := prompt.APIKey(true)
				if !errors.Is(err, sentinel) || key != "" || set {
					t.Errorf("APIKey = (%q, %t, %v), want empty key, false, and sentinel error", key, set, err)
				}
			})
			t.Run("APIURL", func(t *testing.T) {
				var out bytes.Buffer
				prompt := NewCredentialPrompt(&promptErrorReader{input: tc.input, err: sentinel}, &out)
				url, err := prompt.APIURL("https://example.com")
				if !errors.Is(err, sentinel) || url != "" {
					t.Errorf("APIURL = (%q, %v), want empty URL and sentinel error", url, err)
				}
			})
			t.Run("Confirm", func(t *testing.T) {
				var out bytes.Buffer
				const summary = "Delete the selected resource"
				err := Confirm(&promptErrorReader{input: tc.input, err: sentinel}, &out, summary)
				if !errors.Is(err, sentinel) || errors.Is(err, ErrAborted) {
					t.Errorf("Confirm error = %v, want sentinel and not ErrAborted", err)
				}
				if !strings.Contains(out.String(), summary) || !strings.Contains(out.String(), "Are you sure? [y/N]: ") {
					t.Errorf("Confirm output missing summary or question: %q", out.String())
				}
			})
		})
	}
}

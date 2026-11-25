// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package cmdutil

import (
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

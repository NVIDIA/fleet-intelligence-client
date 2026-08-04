// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/config"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Builds an SDK client from the credentials selected by the common flags
func newConfiguredClient(common resolvedCommonFlags) (*nvfleetint.Client, error) {
	resolved, err := resolveCredentials(common.profile)
	if err != nil {
		return nil, err
	}

	return clientFromResolved(resolved, common)
}

func clientFromResolved(resolved config.Resolved, common resolvedCommonFlags) (*nvfleetint.Client, error) {
	if strings.TrimSpace(resolved.APIKey) == "" {
		return nil, missingAPIKeyError(resolved)
	}

	client, err := nvfleetint.NewClient(resolved.APIURL, resolved.APIKey, commonClientOptions(common)...)
	if err != nil {
		// The URL can come from a profile or the environment, so name the place
		// the user actually needs to fix.
		if errors.Is(err, nvfleetint.ErrInsecureBaseURL) {
			return nil, fmt.Errorf("%w; %s", err, fixAPIURLHint(resolved))
		}
		return nil, err
	}

	return client, nil
}

// Resolves credentials, adding CLI hints to the config package's errors
func resolveCredentials(profile string) (config.Resolved, error) {
	resolved, err := config.Resolve(profile)
	if err != nil {
		if errors.Is(err, config.ErrProfileNotFound) {
			return config.Resolved{}, fmt.Errorf("%w; run `nvfleetint auth list` to see the configured profiles", err)
		}
		return config.Resolved{}, err
	}

	return resolved, nil
}

// Explains which profile (if any) is missing an API key
func missingAPIKeyError(resolved config.Resolved) error {
	hint := legacyAPIKeyEnvHint()

	switch {
	case resolved.MissingCurrentProfile != "":
		// The selection survived the profile it points at, so neither "no
		// profile is selected" nor "this profile has no key" describes it.
		return fmt.Errorf(
			"%w; current profile %q is no longer stored; run `nvfleetint auth use <name>`, or pass --profile <name>%s",
			config.ErrNoProfile, resolved.MissingCurrentProfile, hint,
		)
	case resolved.Profile == "" && resolved.ProfilesConfigured:
		return fmt.Errorf(
			"%w; run `nvfleetint auth use <name>`, or pass --profile <name>%s",
			config.ErrNoProfile, hint,
		)
	case resolved.Profile == "":
		return fmt.Errorf(
			"%w; run `nvfleetint auth add <name> --api-key <api-key>`, or set %s%s",
			config.ErrNoProfile, config.EnvAPIKey, hint,
		)
	}

	return fmt.Errorf(
		"profile %q has no API key; run `nvfleetint auth add %s --api-key <api-key>`%s",
		resolved.Profile, resolved.Profile, hint,
	)
}

// legacyAPIKeyEnvNote reports an exported NVFLEETINT_SERVICE_KEY that nothing
// reads any more. The variable was renamed to NVFLEETINT_API_KEY, so a CI job
// that still exports the old name fails as if it had set no credentials at
// all; naming it is the only thing that makes the failure diagnosable. It
// stays quiet once NVFLEETINT_API_KEY is set, since then the old name is
// leftovers rather than the cause.
func legacyAPIKeyEnvNote() string {
	if strings.TrimSpace(os.Getenv(config.EnvLegacyAPIKey)) == "" {
		return ""
	}
	if strings.TrimSpace(os.Getenv(config.EnvAPIKey)) != "" {
		return ""
	}

	return fmt.Sprintf("%s is set but no longer read; it was renamed to %s",
		config.EnvLegacyAPIKey, config.EnvAPIKey)
}

// legacyAPIKeyEnvHint is legacyAPIKeyEnvNote as a clause to append to an error
func legacyAPIKeyEnvHint() string {
	note := legacyAPIKeyEnvNote()
	if note == "" {
		return ""
	}

	return "; " + note
}

// credentialWarnings lists the conditions that did not stop credential
// resolution but changed its outcome. `auth status` exists to explain how
// credentials resolved, so these belong in its output rather than in a
// silently degraded result.
func credentialWarnings(resolved config.Resolved) []string {
	var warnings []string
	if resolved.ConfigError != nil {
		warnings = append(warnings, fmt.Sprintf(
			"%v; continuing with credentials from the environment", resolved.ConfigError))
	}
	if resolved.MissingCurrentProfile != "" {
		warnings = append(warnings, fmt.Sprintf(
			"current profile %q is no longer stored; run `nvfleetint auth use <name>`",
			resolved.MissingCurrentProfile))
	}
	if note := legacyAPIKeyEnvNote(); note != "" {
		warnings = append(warnings, note)
	}

	return warnings
}

// Names the place a rejected API URL came from
func fixAPIURLHint(resolved config.Resolved) string {
	if resolved.APIURLSource == config.SourceEnvironment {
		return "update " + config.EnvAPIURL
	}
	if resolved.Profile != "" {
		return fmt.Sprintf("run `nvfleetint auth add %s --api-url <https-url>`", resolved.Profile)
	}

	return "run `nvfleetint auth add <name> --api-key <api-key> --api-url <https-url>`"
}

// Builds the SDK options implied by resolved common flags
func commonClientOptions(common resolvedCommonFlags) []nvfleetint.Option {
	return []nvfleetint.Option{
		nvfleetint.WithTimeout(common.timeout),
	}
}
